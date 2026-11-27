import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import { buildGuidedPayload, type GuidedFormValues } from '../guided-forms';
import { workFactPrefillFor, workFactSpecs } from '../work-fact';
import type { PackOrder } from './pack-order';
import {
  checkFactValues,
  emptyParcel,
  firstUnpacked,
  judgeMeasure,
  judgePack,
  judgeParcel,
  packBalance,
  packFactValues,
  parcelQuantity,
  withMaterial,
  type PackagingMaterial,
  type ParcelDraft,
} from './pack-parcels';
import { prepareWorkFact } from './work-fact-submit';

const order: PackOrder = {
  requestId: 'OBR-1',
  number: 'DOC-001',
  ownerAccount: 'SYN-OWNER-01',
  warehouse: 'SYN-WH-01',
  dispatch: 'PRODUCT_NETWORK',
  status: 'CHECKED',
  goods: [
    { goodsCode: 'G-A', unit: '件', quantity: 3 },
    { goodsCode: 'G-B', unit: '件', quantity: 2 },
  ],
  lastMismatch: null,
};

const BOX: PackagingMaterial = {
  code: 'BOX-S',
  name: '小号纸箱',
  length: '30',
  width: '20',
  height: '15',
  dimensionUnit: 'CM',
  weightUnit: 'KG',
  deactivated: false,
};

/** 填好的一个包裹：用 BOX-S，称 1.25 千克，尺寸取包材的外廓。 */
function filled(key: number, goodsCode: string, quantity: number): ParcelDraft {
  return { ...withMaterial(emptyParcel(key, { goodsCode, unit: '件' }, quantity), BOX), weight: '1.25', weightUnit: 'KG' };
}

// —— 数值格 ——

test('数值格：不带符号的十进制数、整数最多 9 位、小数最多 6 位、大于 0，才算填对（与旧作业台表单同写法，再加「大于 0」）', () => {
  for (const good of ['1', '1.25', '0.5', '0.000001', '123456789', '123456789.123456', ' 12 ']) ok(judgeMeasure(good), good);
  for (const bad of ['', ' ', '0', '0.0', '0.000000', '-1', '+1', '1e3', '1.', '.5', '1,5', '1234567890', '1.1234567', 'abc', '１２']) {
    ok(!judgeMeasure(bad), `${JSON.stringify(bad)} 不该算填对`);
  }
});

test('装几件：正整数，别的都不算', () => {
  equal(parcelQuantity({ quantity: '3' }), 3);
  equal(parcelQuantity({ quantity: ' 3 ' }), 3);
  for (const bad of ['', '0', '00', '-1', '1.5', 'a', '1e2', '99999999999999999999']) {
    equal(parcelQuantity({ quantity: bad }), null, JSON.stringify(bad));
  }
});

// —— 空包裹与选包材 ——

test('新包裹除了货与件数什么都不预填：包材、重量、重量单位、尺寸、尺寸单位都是空的', () => {
  deepEqual(emptyParcel(7, { goodsCode: 'G-A', unit: '件' }, 3), {
    key: 7,
    goodsCode: 'G-A',
    unit: '件',
    quantity: '3',
    packaging: '',
    packagingName: '',
    packagingWeightUnit: '',
    weight: '',
    weightUnit: '',
    length: '',
    width: '',
    height: '',
    dimensionUnit: '',
    dimsEdited: false,
  });
  equal(emptyParcel(1, { goodsCode: 'G-A', unit: '件' }, 0).quantity, '', '没有数就留空，不填 0');
});

test('选了包材：尺寸取包材登记的外廓，重量与重量单位一概不填；包材的重量单位只当提示记着', () => {
  const next = withMaterial(emptyParcel(1, { goodsCode: 'G-A', unit: '件' }, 3), BOX);
  equal(next.packaging, 'BOX-S');
  equal(next.packagingName, '小号纸箱');
  equal(next.packagingWeightUnit, 'KG');
  deepEqual([next.length, next.width, next.height, next.dimensionUnit], ['30', '20', '15', 'CM']);
  equal(next.weight, '');
  equal(next.weightUnit, '', '重量单位不替人填，哪怕包材登记了一个');
});

test('包材没登记外廓（缺一边、不是正数、没有单位）：尺寸留着让人填，不编默认值', () => {
  const base = emptyParcel(1, { goodsCode: 'G-A', unit: '件' }, 3);
  for (const material of [
    { ...BOX, length: '' },
    { ...BOX, width: '0' },
    { ...BOX, height: 'abc' },
    { ...BOX, dimensionUnit: '' },
    { ...BOX, length: '', width: '', height: '', dimensionUnit: '' },
  ]) {
    const next = withMaterial(base, material);
    equal(next.packaging, 'BOX-S');
    deepEqual([next.length, next.width, next.height, next.dimensionUnit], ['', '', '', '']);
  }
});

test('人改过尺寸之后再换包材，不覆盖他改的尺寸', () => {
  const edited = { ...emptyParcel(1, { goodsCode: 'G-A', unit: '件' }, 3), length: '31', width: '21', height: '16', dimensionUnit: 'CM', dimsEdited: true };
  const next = withMaterial(edited, { ...BOX, code: 'BOX-L', length: '60' });
  equal(next.packaging, 'BOX-L');
  deepEqual([next.length, next.width, next.height], ['31', '21', '16']);
  const untouched = withMaterial(withMaterial(emptyParcel(1, { goodsCode: 'G-A', unit: '件' }, 3), BOX), { ...BOX, code: 'BOX-L', length: '60' });
  equal(untouched.length, '60', '没改过尺寸的，换包材就取新包材的外廓');
});

// —— 全单装得齐不齐 ——

test('各种货装进包裹的件数合计，与该装的对照', () => {
  deepEqual(packBalance(order, [filled(1, 'G-A', 2), filled(2, 'G-A', 1), filled(3, 'G-B', 1)]), [
    { goodsCode: 'G-A', unit: '件', expected: 3, assigned: 3 },
    { goodsCode: 'G-B', unit: '件', expected: 2, assigned: 1 },
  ]);
  const bad = { ...filled(4, 'G-A', 1), quantity: 'x' };
  equal(packBalance(order, [bad])[0].assigned, 0, '件数填不对的不计');
});

test('新包裹先装第一种还没装满的货，差几件装几件；都装满了答没有', () => {
  deepEqual(firstUnpacked(order, []), { goods: order.goods[0], remaining: 3 });
  deepEqual(firstUnpacked(order, [filled(1, 'G-A', 1)]), { goods: order.goods[0], remaining: 2 });
  deepEqual(firstUnpacked(order, [filled(1, 'G-A', 3)]), { goods: order.goods[1], remaining: 2 });
  equal(firstUnpacked(order, [filled(1, 'G-A', 3), filled(2, 'G-B', 2)]), null);
});

// —— 一个包裹填得对不对：次序就是人填的次序 ——

test('一个包裹没填好时先指第一处：货、件数、包材、重量、重量单位、长宽高、尺寸单位', () => {
  const base = emptyParcel(1, { goodsCode: 'G-A', unit: '件' }, 3);
  const problem = (parcel: ParcelDraft) => judgeParcel(order, parcel, 2);

  equal(problem({ ...base, goodsCode: 'G-X' })?.field, 'goods');
  equal(problem({ ...base, quantity: '' })?.field, 'quantity');
  const packed = withMaterial(base, { ...BOX, length: '', width: '', height: '', dimensionUnit: '' });
  equal(problem(base)?.field, 'packaging');
  equal(problem(packed)?.field, 'weight');
  equal(problem({ ...packed, weight: '0' })?.field, 'weight');
  equal(problem({ ...packed, weight: '1.5' })?.field, 'weightUnit');
  equal(problem({ ...packed, weight: '1.5', weightUnit: 'KG' })?.field, 'dimensions');
  equal(problem({ ...packed, weight: '1.5', weightUnit: 'KG', length: '3', width: '2', height: '0' })?.field, 'dimensions');
  equal(problem({ ...packed, weight: '1.5', weightUnit: 'KG', length: '3', width: '2', height: '1' })?.field, 'dimensionUnit');
  equal(problem({ ...packed, weight: '1.5', weightUnit: 'KG', length: '3', width: '2', height: '1', dimensionUnit: 'CM' }), null);
  ok(problem(base)?.text.startsWith('包裹 2：'), '话里写明是第几个包裹');
});

// —— 能不能提交 ——

test('提交前：至少一个包裹，每个都填好，每种货装进包裹的件数都等于已拣的件数', () => {
  const none = judgePack(order, []);
  ok(!none.ok && none.reason.includes('再加一个包裹'));

  const rawSecond = emptyParcel(2, { goodsCode: 'G-B', unit: '件' }, 2);
  const unfinished = judgePack(order, [filled(1, 'G-A', 3), rawSecond]);
  ok(!unfinished.ok);
  deepEqual(!unfinished.ok && unfinished.at, { position: 2, field: 'packaging' });

  const short = judgePack(order, [filled(1, 'G-A', 3), filled(2, 'G-B', 1)]);
  ok(!short.ok && short.reason.includes('G-B') && short.reason.includes('还有 1 件'));
  equal(!short.ok && short.at, undefined, '装不齐不属于哪一个包裹');

  const over = judgePack(order, [filled(1, 'G-A', 3), filled(2, 'G-B', 3)]);
  ok(!over.ok && over.reason.includes('G-B') && over.reason.includes('多 1 件'));

  ok(judgePack(order, [filled(1, 'G-A', 3), filled(2, 'G-B', 2)]).ok);
  ok(judgePack(order, [filled(1, 'G-A', 1), filled(2, 'G-A', 2), filled(3, 'G-B', 2)]).ok, '一种货可以拆进几个包裹');
});

test('提交前：合计对得上总数、各种货分别对不上，也不算装齐（比后端只对总数更严一步）', () => {
  // 总数 5 对上了，但 G-A 装了 4、G-B 装了 1
  const skewed = judgePack(order, [filled(1, 'G-A', 4), filled(2, 'G-B', 1)]);
  ok(!skewed.ok);
});

// —— 提交的报文：与现有作业台那两条定义装出来的逐键相同 ——

const device = 'SYN-PDA-WH01-01';
const at = new Date('2026-11-25T08:30:15.123Z');

function legacy(specId: string, values: GuidedFormValues) {
  const spec = workFactSpecs.find((candidate) => candidate.id === specId);
  ok(spec !== undefined, `缺 ${specId} 这条定义`);
  if (spec === undefined) throw new Error('unreachable');
  // 旧作业台页的做法：事实号与仓按定义补，和人在表单里填的格一起过 buildGuidedPayload，再叠固定载荷
  const built = buildGuidedPayload(spec.form, { ...workFactPrefillFor(spec, device, 'SYN-WH-01', 9, at), ...values });
  ok(built.ok, JSON.stringify(built));
  if (!built.ok) throw new Error('unreachable');
  return { spec, payload: { ...(spec.fixedPayload ?? {}), ...built.payload } };
}

test('复核的报文与旧作业台页用同一条 outbound-check 定义装出来的逐键相同（后端用封闭载荷解码，多一个键、错一个字就是 400）', () => {
  for (const observed of [5, 4, 0, 12]) {
    const old = legacy('outbound-check', { requestId: 'OBR-1', observed: String(observed) });
    const prepared = prepareWorkFact({
      spec: old.spec,
      deviceId: device,
      warehouse: 'SYN-WH-01',
      sequence: 9,
      values: checkFactValues(order, observed),
      now: at,
    });
    ok(prepared.ok);
    if (!prepared.ok) return;
    deepEqual(prepared.attempt.payload, old.payload);
    deepEqual(prepared.attempt.payload, { factId: 'PDA-SYN-PDA-WH01-01-20261125083015-9', warehouse: 'SYN-WH-01', requestId: 'OBR-1', observed });
    equal(prepared.attempt.endpoint, '/warehouse-fulfillment-outbound-checks');
    deepEqual(prepared.attempt.headers, { deviceId: device, scopeKind: 'WAREHOUSE', scopeReference: 'SYN-WH-01' });
  }
});

test('复核要填的格：出库请求号与实点总数（文本）；写进载荷的是请求号，不是屏上写的单据号', () => {
  deepEqual(checkFactValues(order, 5), { requestId: 'OBR-1', observed: '5' });
});

test('打包的报文与旧作业台页用同一条 pack 定义装出来的逐键相同；一个包裹一项、序号从 1 起、数量是数、重量与尺寸是文本', () => {
  const one = [{ ...filled(1, 'G-A', 3), packaging: 'BOX-S' }, filled(2, 'G-B', 2)];
  const many = [filled(1, 'G-A', 1), filled(2, 'G-A', 2), filled(3, 'G-B', 2)];
  for (const parcels of [one, many]) {
    const values = packFactValues(order, parcels);
    const old = legacy('pack', values);
    const prepared = prepareWorkFact({ spec: old.spec, deviceId: device, warehouse: 'SYN-WH-01', sequence: 9, values, now: at });
    ok(prepared.ok);
    if (!prepared.ok) return;
    deepEqual(prepared.attempt.payload, old.payload);
    equal(prepared.attempt.endpoint, '/warehouse-fulfillment-packs');
  }

  const prepared = prepareWorkFact({
    spec: workFactSpecs.find((candidate) => candidate.id === 'pack') as (typeof workFactSpecs)[number],
    deviceId: device,
    warehouse: 'SYN-WH-01',
    sequence: 9,
    values: packFactValues(order, many),
    now: at,
  });
  ok(prepared.ok);
  if (!prepared.ok) return;
  deepEqual(prepared.attempt.payload, {
    factId: 'PDA-SYN-PDA-WH01-01-20261125083015-9',
    warehouse: 'SYN-WH-01',
    requestId: 'OBR-1',
    parcels: [
      { sequence: 1, packagingMaterial: 'BOX-S', weight: '1.25', weightUnit: 'KG', length: '30', width: '20', height: '15', dimensionUnit: 'CM', goodsCode: 'G-A', unit: '件', quantity: 1 },
      { sequence: 2, packagingMaterial: 'BOX-S', weight: '1.25', weightUnit: 'KG', length: '30', width: '20', height: '15', dimensionUnit: 'CM', goodsCode: 'G-A', unit: '件', quantity: 2 },
      { sequence: 3, packagingMaterial: 'BOX-S', weight: '1.25', weightUnit: 'KG', length: '30', width: '20', height: '15', dimensionUnit: 'CM', goodsCode: 'G-B', unit: '件', quantity: 2 },
    ],
  });
});

test('打包的报文：各格首尾空白去掉再交，序号按包裹的次序从 1 数，与草稿里的本地 key 无关', () => {
  const parcels = [
    { ...filled(41, 'G-A', 3), weight: ' 2.5 ', weightUnit: ' KG ', packaging: ' BOX-S ' },
    { ...filled(7, 'G-B', 2), quantity: ' 2 ' },
  ];
  const values = packFactValues(order, parcels);
  const rows = values.parcels as Array<Record<string, string>>;
  deepEqual(
    rows.map((row) => row.sequence),
    ['1', '2'],
  );
  equal(rows[0].weight, '2.5');
  equal(rows[0].weightUnit, 'KG');
  equal(rows[0].packagingMaterial, 'BOX-S');
  equal(rows[1].quantity, '2');
});

test('跟踪序列号的号按包裹顺序分给同一种货，没号的包裹不带这一格', () => {
  const parcels = [filled(1, 'G-A', 1), filled(2, 'G-A', 2), filled(3, 'G-B', 2)];
  const values = packFactValues(order, parcels, { 'G-A\u0000件': ['SN-1', 'SN-2', 'SN-3'] });
  const rows = values.parcels as Array<Record<string, unknown>>;
  deepEqual(rows[0].serials, ['SN-1']);
  deepEqual(rows[1].serials, ['SN-2', 'SN-3']);
  equal('serials' in rows[2], false);
});
