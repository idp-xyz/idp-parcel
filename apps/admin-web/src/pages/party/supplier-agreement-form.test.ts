import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import {
  emptySupplierAgreementDraft,
  planReferenceOf,
  supplierAgreementFieldPaths,
  supplierAgreementPayloadOf,
  withShellCopiedIntoAgreement,
  type SupplierAgreementDraft,
} from './supplier-agreement-form';

// 票 admin-write-faces/11：供应商协议逐字段表单的纯逻辑。钉的是「草稿 → 载荷」这一步与它对公共半边
// （publication-draft-flow）的两个承诺：载荷形状逐格镜像 Go 的 CommercialPublicationPayload.supplierAgreement，
// 表单认领的 JSON 路径覆盖它自己能产出的每一条键。

const filled: SupplierAgreementDraft = {
  objectId: ' SYN-SUPPLIER-TRUNK-CN-SG-01 ',
  version: 'v2',
  scope: 'SYN-SCOPE-01',
  effectiveStartsAt: '2026-10-01',
  effectiveEndsAt: '',
  supplier: 'supplier-1',
  legalEntity: 'legal-1 ',
  agreementScope: 'scope-procurement',
  purchasePlan: 'plan-buy-1@v3',
  agreementEffectiveStartsAt: '2026-10-01T00:00:00Z',
  agreementEffectiveEndsAt: '2026-12-31',
};

test('草稿组成载荷：kind 固定 SUPPLIER_AGREEMENT，各格去首尾空白，只到天的日期补成当天零点 UTC', () => {
  deepEqual(supplierAgreementPayloadOf(filled), {
    kind: 'SUPPLIER_AGREEMENT',
    objectId: 'SYN-SUPPLIER-TRUNK-CN-SG-01',
    version: 'v2',
    scope: 'SYN-SCOPE-01',
    effectiveStartsAt: '2026-10-01T00:00:00Z',
    supplierAgreement: {
      supplier: 'supplier-1',
      legalEntity: 'legal-1',
      scope: 'scope-procurement',
      purchasePlan: 'plan-buy-1@v3',
      effectiveStartsAt: '2026-10-01T00:00:00Z',
      effectiveEndsAt: '2026-12-31T00:00:00Z',
    },
  });
});

// 服务端按键在场与否分辨「没有上界」，空串会被当成一个填了空的时刻送进构造门；壳与正文两级同一条规矩。
test('区间上界留空即键缺席，不送空串；壳上不带指名引用、不带方向', () => {
  const payload = supplierAgreementPayloadOf({ ...filled, agreementEffectiveEndsAt: '  ' });
  ok(!('effectiveEndsAt' in payload));
  ok(!('effectiveEndsAt' in payload.supplierAgreement!));
  ok(!('references' in payload));
  ok(!('direction' in payload.supplierAgreement!));

  const bounded = supplierAgreementPayloadOf({ ...filled, effectiveEndsAt: '2027-01-01' });
  equal(bounded.effectiveEndsAt, '2027-01-01T00:00:00Z');
});

// 伞票 07 硬句：表单不算摘要、不收也不送批准人；租户与录入者由操作者信封给。载荷里出现这些键会被服务端
// 按未知键拒，所以这里钉死它们一个都不在。
test('载荷里没有身份也没有摘要', () => {
  const text = JSON.stringify(supplierAgreementPayloadOf(filled));
  for (const forbidden of ['tenant', 'submitter', 'approver', 'contentDigest', 'approval', 'canonicalization']) {
    ok(!text.includes(`"${forbidden}`), `载荷不该带 ${forbidden}`);
  }
});

// 公共半边把服务端点名、表单没认领的路径单列为「未认领」。表单自己能产出的每一条键都必须已认领，
// 否则本册自己的格出了问题会被显示成别处的问题。
test('表单认领的 JSON 路径覆盖载荷能产出的每一条键', () => {
  const payload = supplierAgreementPayloadOf({ ...filled, effectiveEndsAt: '2027-01-01' });
  const produced: string[] = [];
  for (const [key, value] of Object.entries(payload)) {
    if (key === 'kind') continue;
    if (value !== null && typeof value === 'object') {
      for (const nested of Object.keys(value)) produced.push(`${key}.${nested}`);
    } else {
      produced.push(key);
    }
  }
  for (const path of produced) {
    ok(supplierAgreementFieldPaths.includes(path), `路径 ${path} 未被表单认领`);
  }
  // 反向：认领表里没有载荷产不出的路径——多认领一条会把别处的问题吞进本表单。
  for (const path of supplierAgreementFieldPaths) {
    ok(produced.includes(path), `认领了载荷产不出的路径 ${path}`);
  }
});

// 采购方案是 parcel-pricing 方案版本的引用串，从价卡目录行选出；写法与 settlement-accounting 拼方案版本
// 引用同一条（planId@planVersion），表单不读方案内容。
test('价卡目录行 → 方案版本引用串', () => {
  equal(planReferenceOf({ planId: 'PLAN-CN-SG-COST', planVersion: 'v1' }), 'PLAN-CN-SG-COST@v1');
});

test('「从版本壳带入」只抄范围与区间两样，其余各格不动', () => {
  const copied = withShellCopiedIntoAgreement({
    ...filled,
    effectiveEndsAt: '2027-01-01',
    agreementScope: 'stale',
    agreementEffectiveStartsAt: 'stale',
    agreementEffectiveEndsAt: 'stale',
  });
  equal(copied.agreementScope, 'SYN-SCOPE-01');
  equal(copied.agreementEffectiveStartsAt, '2026-10-01');
  equal(copied.agreementEffectiveEndsAt, '2027-01-01');
  equal(copied.supplier, 'supplier-1');
  equal(copied.purchasePlan, 'plan-buy-1@v3');
});

test('空草稿十一格全空', () => {
  const draft = emptySupplierAgreementDraft();
  equal(Object.keys(draft).length, 11);
  for (const [key, value] of Object.entries(draft)) equal(value, '', `${key} 不为空`);
});
