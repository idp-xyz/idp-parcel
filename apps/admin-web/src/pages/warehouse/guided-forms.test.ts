import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import {
  internalOpsActionSpecs,
  outboundWaveActionSpecs,
  pricingActionSpecs,
  receivingActionSpecs,
} from './actions';
import { buildGuidedPayload, initialGuidedValues } from './guided-forms';
import {
  allWarehouseGuidedForms,
  catalogueGuidedForms,
  internalOpsGuidedForms,
  outboundWaveGuidedForms,
  pricingGuidedForms,
  receivingGuidedForms,
  stockImportGuidedForms,
} from './guided-schemas';
import { catalogueFamilies } from './rows';

test('仓配六页全部写动作都有引导式表单定义，没有漏网的纯 JSON 动作', () => {
  for (const family of catalogueFamilies) {
    ok((catalogueGuidedForms[family]?.fields.length ?? 0) > 0, `名册 ${family} 缺少引导表单`);
  }
  for (const id of ['submit', 'approve', 'reject']) {
    ok((stockImportGuidedForms[id]?.fields.length ?? 0) > 0, `建账导入 ${id} 缺少引导表单`);
  }
  const allSpecs = [
    ...receivingActionSpecs,
    ...internalOpsActionSpecs,
    ...outboundWaveActionSpecs,
    ...pricingActionSpecs,
  ];
  for (const spec of allSpecs) {
    ok((allWarehouseGuidedForms[spec.id]?.fields.length ?? 0) > 0, `动作 ${spec.id} 缺少引导表单`);
  }
  equal(Object.keys(allWarehouseGuidedForms).length, 5 + 3 + allSpecs.length);
});

test('仓与包材表单按嵌套路径拼出精确报文，可选空项不写入且数值保持十进制字符串', () => {
  const wh = buildGuidedPayload(catalogueGuidedForms.warehouse, {
    warehouseCode: 'SYN-WH-01',
    revision: '2',
    name: '深圳一号仓',
    kind: 'SELF_OPERATED',
    'address.country': 'CN',
    'address.text': '深圳市宝安区',
    businessTimeZone: 'Asia/Shanghai',
    colocatedNode: '',
    deactivated: false,
  });
  equal(wh.ok, true);
  if (wh.ok) {
    deepEqual(wh.payload, {
      warehouseCode: 'SYN-WH-01',
      revision: 2,
      name: '深圳一号仓',
      kind: 'SELF_OPERATED',
      address: { country: 'CN', text: '深圳市宝安区' },
      businessTimeZone: 'Asia/Shanghai',
    });
  }

  const pkg = buildGuidedPayload(catalogueGuidedForms.packaging, {
    packagingMaterialCode: 'SYN-BOX-01',
    revision: '1',
    name: '一号纸箱',
    'dimensions.length': '30',
    'dimensions.width': '20',
    'dimensions.height': '15.50',
    'dimensions.unit': 'CM',
    'weight.value': '0.25',
    'weight.unit': 'KG',
    deactivated: true,
  });
  equal(pkg.ok, true);
  if (pkg.ok) {
    deepEqual(pkg.payload, {
      packagingMaterialCode: 'SYN-BOX-01',
      revision: 1,
      name: '一号纸箱',
      dimensions: { length: '30', width: '20', height: '15.50', unit: 'CM' },
      weight: { value: '0.25', unit: 'KG' },
      deactivated: true,
    });
  }
});

test('列表行预填值直接落进对应字段，其余字段不编造租户默认值', () => {
  const seeded = initialGuidedValues(outboundWaveGuidedForms['wave-release'].fields, {
    warehouse: 'SYN-WH-01',
    requests: 'OBR-1',
  });
  equal(seeded.warehouse, 'SYN-WH-01');
  equal(seeded.requests, 'OBR-1');
  equal(seeded.waveId, '');
  equal(seeded.prefetch, false);

  const built = buildGuidedPayload(outboundWaveGuidedForms['wave-release'], {
    ...seeded,
    waveId: 'WAVE-1',
    requests: 'OBR-1, OBR-2',
    prefetch: true,
    packagingMaterial: 'SYN-BOX-01',
  });
  equal(built.ok, true);
  if (built.ok) {
    deepEqual(built.payload, {
      waveId: 'WAVE-1',
      warehouse: 'SYN-WH-01',
      requests: ['OBR-1', 'OBR-2'],
      prefetch: true,
      packagingMaterial: 'SYN-BOX-01',
    });
  }
});

test('跨字段与明细行校验在提交前拦住不完整输入', () => {
  const emptyImport = buildGuidedPayload(stockImportGuidedForms.submit, {
    importId: 'IMP-1',
    ownerAccount: 'SYN-OWNER-01',
    warehouseCode: 'SYN-WH-01',
    reason: '期初',
    goodsLines: [],
    cartonLines: [],
  });
  equal(emptyImport.ok, false);
  if (!emptyImport.ok) ok(emptyImport.formError?.includes('至少填写一组'));

  const badNetwork = buildGuidedPayload(outboundWaveGuidedForms['outbound-request'], {
    decisionId: 'DEC-1',
    ownerAccount: 'SYN-OWNER-01',
    basis: 'BASIS-1',
    warehouse: 'SYN-WH-01',
    dispatch: 'PRODUCT_NETWORK',
    service: '',
    recipient: '张三',
    address: '深圳',
    lines: [{ goodsCode: 'SYN-GOODS-01', unit: '件', stockState: 'GOOD', quantity: '2' }],
  });
  equal(badNetwork.ok, false);
  if (!badNetwork.ok) ok(badNetwork.formError?.includes('服务产品'));

  const badConfirmCorrect = buildGuidedPayload(outboundWaveGuidedForms['confirmation-correction'], {
    factId: 'FACT-1',
    occurredAt: '2026-11-24T08:00:00Z',
    invalidate: true,
  });
  equal(badConfirmCorrect.ok, false);
  if (!badConfirmCorrect.ok) ok(badConfirmCorrect.formError?.includes('二选一'));

  const badFreeze = buildGuidedPayload(internalOpsGuidedForms['stock-freeze'], {
    decisionId: 'DEC-FRZ',
    ownerAccount: 'SYN-OWNER-01',
    warehouse: 'SYN-WH-01',
    reason: '抽检',
    cartonNumber: 'BOX-1',
    locationCode: 'LOC-1',
  });
  equal(badFreeze.ok, false);
  if (!badFreeze.ok) ok(badFreeze.formError?.includes('整箱冻结'));

  const okGoods = buildGuidedPayload(receivingGuidedForms['goods-on-behalf'], {
    ownerAccount: 'SYN-OWNER-01',
    decisionId: 'DEC-G1',
    evidenceReference: 'EVID-1',
    goodsCode: 'SYN-GOODS-01',
    basisRevision: '0',
    name: '商品一',
    baseUnit: '件',
    trackBatch: false,
    trackExpiry: true,
  });
  equal(okGoods.ok, true);
  if (okGoods.ok) {
    equal(okGoods.payload.basisRevision, 0);
    equal(okGoods.payload.trackBatch, false);
    equal(okGoods.payload.trackExpiry, true);
  }

  const okDayClose = buildGuidedPayload(pricingGuidedForms['storage-day-close'], {
    ownerAccount: 'SYN-OWNER-01',
    warehouse: 'SYN-WH-01',
    day: '2026-11-24',
  });
  equal(okDayClose.ok, true);
  if (okDayClose.ok) {
    deepEqual(okDayClose.payload, {
      ownerAccount: 'SYN-OWNER-01',
      warehouse: 'SYN-WH-01',
      day: '2026-11-24',
    });
  }
});

test('增强后的仓配表单自动附带名册下拉（lookup）与可读单据号前缀（autoIdPrefix），且不覆盖列表预填', () => {
  const waveDef = allWarehouseGuidedForms['wave-release'];
  const waveIdField = waveDef.fields.find((f) => f.path === 'waveId');
  const whField = waveDef.fields.find((f) => f.path === 'warehouse');
  const pkgField = waveDef.fields.find((f) => f.path === 'packagingMaterial');
  equal(waveIdField?.autoIdPrefix, 'WAVE');
  equal(whField?.lookup, 'warehouse');
  equal(pkgField?.lookup, 'packaging');

  const generated = initialGuidedValues(waveDef.fields, { warehouse: 'SYN-WH-01', requests: 'OBR-1' });
  ok(typeof generated.waveId === 'string' && generated.waveId.startsWith('WAVE-'), '未指定波次号时自动生成可读波次号');
  equal(generated.warehouse, 'SYN-WH-01');

  const explicit = initialGuidedValues(waveDef.fields, { waveId: 'CUSTOM-W1', warehouse: 'SYN-WH-01' });
  equal(explicit.waveId, 'CUSTOM-W1');

  // 新建仓/库区/库位自身编码不挂 lookup，防止建档时只能选旧码
  equal(allWarehouseGuidedForms.warehouse.fields.find((f) => f.path === 'warehouseCode')?.lookup, undefined);
  equal(allWarehouseGuidedForms.zone.fields.find((f) => f.path === 'warehouseCode')?.lookup, 'warehouse');
  equal(allWarehouseGuidedForms.zone.fields.find((f) => f.path === 'zoneCode')?.lookup, undefined);
  equal(allWarehouseGuidedForms.location.fields.find((f) => f.path === 'zoneCode')?.lookup, 'zone');
  equal(allWarehouseGuidedForms.location.fields.find((f) => f.path === 'locationCode')?.lookup, undefined);
});
