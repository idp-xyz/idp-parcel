import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import { emptyGoodsLine } from './goods-form';
import {
  emptyFbaBox,
  emptyFbaForm,
  emptyFbaHead,
  fbaDraftOf,
  validateFba,
  withMeasurementsFromFirstBox,
  withSequence,
  type FbaForm,
  type FbaHeadForm,
} from './fba-form';
import { fillSameEachBox } from './packing-form';

// 本文件钉平台货件下单页的组装(ADR-0374,票 declared-goods-lines/05):一个平台货件一份委托、一箱一个包裹;收件资料是平台仓;
// 不设 FBA 标志,页面不把平台货件号当必填,只拦服务端必拒的形状。

const head: FbaHeadForm = {
  ...emptyFbaHead,
  customerShipmentReference: 'SYN-FBA-0001',
  requestedServiceProduct: 'SYN-FBA-EXPRESS',
  senderRelation: 'SYN 深圳仓',
  senderAddress: '深圳市宝安区 SYN 路 1 号',
  platform: 'AMAZON',
  shipmentId: 'FBA15SYN0001',
  warehouseCode: 'SYN-ONT8',
  warehouseAddress: '24300 Nandina Ave, Moreno Valley, CA',
  destinationServiceScope: 'US',
  weightUnit: 'KG',
  dimensionsUnit: 'CM',
};

const thirtyBoxes = withMeasurementsFromFirstBox(
  withSequence(
    withSequence(
      Array.from({ length: 30 }, (_, index) =>
        index === 0 ? { ...emptyFbaBox, weight: '12.5', length: '60', width: '40', height: '40' } : { ...emptyFbaBox },
      ),
      'customerParcelReference',
      'SYN-CTN-',
      1,
      3,
    ),
    'platformBoxLabel',
    'FBA15SYN0001U',
    1,
    6,
  ),
);

const form = (change: Partial<FbaForm> = {}): FbaForm => ({ ...emptyFbaForm, head, boxes: thirtyBoxes, lines: [], ...change });

test('序号填法只填空格,补零到给定位数;其余箱照第 1 箱填重量与尺寸的空格', () => {
  const boxes = withSequence(
    [{ ...emptyFbaBox }, { ...emptyFbaBox, customerParcelReference: 'MINE' }, { ...emptyFbaBox }],
    'customerParcelReference',
    'C-',
    7,
    3,
  );
  deepEqual(
    boxes.map((box) => box.customerParcelReference),
    ['C-007', 'MINE', 'C-009'],
  );
  const filled = withMeasurementsFromFirstBox([{ ...emptyFbaBox, weight: '10', length: '50' }, { ...emptyFbaBox, weight: '9' }]);
  equal(filled[1].weight, '9');
  equal(filled[1].length, '50');
  equal(thirtyBoxes[29].platformBoxLabel, 'FBA15SYN0001U000030');
  equal(thirtyBoxes[29].height, '40');
});

test('三十箱、一行货每箱 24 件:收件资料是平台仓,单位逐箱带上,每箱带箱标号与装箱分配', () => {
  const lines = [{ ...emptyGoodsLine, descriptionEn: 'T-shirt', quantity: '720', unit: 'PCS' }];
  const filled = form({ lines, contentCategory: 'SALE_OF_GOODS', packing: fillSameEachBox([], 0, '24', 0, 29) });
  deepEqual(validateFba(filled), {});
  const draft = fbaDraftOf(filled);
  equal(draft.recipientRelation, 'SYN-ONT8');
  equal(draft.recipientAddress, '24300 Nandina Ave, Moreno Valley, CA');
  equal(draft.requestEffectiveAt, undefined);
  deepEqual(draft.platformShipment, { platform: 'AMAZON', shipmentId: 'FBA15SYN0001' });
  equal(draft.parcels.length, 30);
  deepEqual(draft.parcels[0], {
    customerParcelReference: 'SYN-CTN-001',
    declaredWeightValue: '12.5',
    declaredWeightUnit: 'KG',
    declaredLength: '60',
    declaredWidth: '40',
    declaredHeight: '40',
    declaredDimensionsUnit: 'CM',
    platformBoxLabel: 'FBA15SYN0001U000001',
    packing: [{ line: 1, quantity: '24' }],
  });
  deepEqual(draft.goods, {
    contentCategory: 'SALE_OF_GOODS',
    lines: [{ descriptions: [{ lang: 'en', text: 'T-shirt' }], quantity: '720', unit: 'PCS' }],
  });
});

test('平台货件号不当必填(由接单规则说);但平台与货件号同在同缺,报了箱标号就要有平台货件', () => {
  const noPlatform = { ...head, platform: '', shipmentId: '' };
  const plainBoxes = thirtyBoxes.map((box) => ({ ...box, platformBoxLabel: '' }));
  deepEqual(validateFba(form({ head: noPlatform, boxes: plainBoxes })), {});
  equal(fbaDraftOf(form({ head: noPlatform, boxes: plainBoxes })).platformShipment, undefined);

  const labelsOnly = validateFba(form({ head: noPlatform }));
  equal(labelsOnly.platform, '报了箱标号就要填平台货件');
  equal(labelsOnly.shipmentId, '报了箱标号就要填平台货件');
  equal(validateFba(form({ head: { ...noPlatform, referenceId: 'SYN-REF' }, boxes: plainBoxes })).platform, '平台与平台货件号要一起填');
});

test('一票里箱标号与客户包裹号不重复:撞号的几箱都标上', () => {
  const boxes = thirtyBoxes.map((box, index) =>
    index === 4 ? { ...box, platformBoxLabel: thirtyBoxes[1].platformBoxLabel, customerParcelReference: ' SYN-CTN-003 ' } : box,
  );
  const errors = validateFba(form({ boxes }));
  equal(errors['b1.platformBoxLabel'], '重号');
  equal(errors['b4.platformBoxLabel'], '重号');
  equal(errors['b2.customerParcelReference'], '重号');
  equal(errors['b4.customerParcelReference'], '重号');
  equal(errors['b3.platformBoxLabel'], undefined);
});

test('开始时间按显示时区折成 RFC 3339 交;填不全在提交前拦', () => {
  equal(fbaDraftOf(form({ head: { ...head, requestEffectiveAt: '2026-11-10T10:00' } }), 'Asia/Shanghai').requestEffectiveAt, '2026-11-10T02:00:00Z');
  equal(validateFba(form({ head: { ...head, requestEffectiveAt: '2026-11' } })).requestEffectiveAt, '日期和时刻要填全');
});

test('装箱报了没报全,提交前就说;箱的重量与尺寸按领域构造期的形状拦', () => {
  const lines = [{ ...emptyGoodsLine, descriptionEn: 'T-shirt', quantity: '720' }];
  const errors = validateFba(form({ lines, packing: fillSameEachBox([], 0, '24', 0, 9) }));
  ok(errors.packing?.startsWith('第 1 行报了 720,各箱加起来是 240'));
  ok(errors.packing?.includes('等 20 箱什么都没装'));

  const badBoxes = [{ ...emptyFbaBox, customerParcelReference: 'X', weight: '0', length: '10' }];
  const boxErrors = validateFba(form({ head: { ...head, dimensionsUnit: '' }, boxes: badBoxes }));
  equal(boxErrors['b0.weight'], '大于 0 的数');
  equal(boxErrors['b0.width'], '长宽高一起填');
  equal(boxErrors.dimensionsUnit, '请填写');
});
