import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { SupplierAgreementRecord } from './api';
import {
  emptySupplierAgreementDraft,
  planReferenceOf,
  supplierAgreementDraftFromRecord,
  supplierAgreementFieldPaths,
  supplierAgreementLocalProblems,
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
  effectiveStartsAt: '2026-10-01T08:00',
  effectiveEndsAt: '',
  supplier: 'supplier-1',
  legalEntity: 'legal-1 ',
  agreementScope: 'scope-procurement',
  purchasePlan: 'plan-buy-1@v3',
  agreementEffectiveStartsAt: '2026-10-01T08:00:00',
  agreementEffectiveEndsAt: '2027-01-01T00:00',
};

// 时刻格是日期时间控件的本地墙钟，按显示时区换成 UTC，与登记表单同一条换算（票 party-commercial-uiux/10）。
test('草稿组成载荷：kind 固定 SUPPLIER_AGREEMENT，各格去首尾空白，本地墙钟按显示时区换成 UTC', () => {
  deepEqual(supplierAgreementPayloadOf(filled, 'Asia/Shanghai'), {
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
      effectiveEndsAt: '2026-12-31T16:00:00Z',
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

  const bounded = supplierAgreementPayloadOf({ ...filled, effectiveEndsAt: '2027-01-01T00:00' }, 'Asia/Shanghai');
  equal(bounded.effectiveEndsAt, '2026-12-31T16:00:00Z');
});

// 票 party-commercial-uiux/10：时刻格换不成时刻是本地问题，落在那一格路径上，挡住预览；空着不是。
test('时刻格换不成时刻才是本地问题，落在该格路径上', () => {
  deepEqual(
    supplierAgreementLocalProblems({ ...filled, effectiveStartsAt: '2026-10-01', agreementEffectiveEndsAt: 'tomorrow' }, 'Asia/Shanghai'),
    {
      effectiveStartsAt: ['时间格式不对，请用日期时间选择器重新选'],
      'supplierAgreement.effectiveEndsAt': ['时间格式不对，请用日期时间选择器重新选'],
    },
  );
  deepEqual(supplierAgreementLocalProblems(filled, 'Asia/Shanghai'), {});
  deepEqual(supplierAgreementLocalProblems(emptySupplierAgreementDraft(), 'Asia/Shanghai'), {});
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
  const payload = supplierAgreementPayloadOf({ ...filled, effectiveEndsAt: '2027-01-01T00:00' });
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
    effectiveEndsAt: '2027-01-01T00:00',
    agreementScope: 'stale',
    agreementEffectiveStartsAt: 'stale',
    agreementEffectiveEndsAt: 'stale',
  });
  equal(copied.agreementScope, 'SYN-SCOPE-01');
  equal(copied.agreementEffectiveStartsAt, '2026-10-01T08:00');
  equal(copied.agreementEffectiveEndsAt, '2027-01-01T00:00');
  equal(copied.supplier, 'supplier-1');
  equal(copied.purchasePlan, 'plan-buy-1@v3');
});

test('空草稿十一格全空', () => {
  const draft = emptySupplierAgreementDraft();
  equal(Object.keys(draft).length, 11);
  for (const [key, value] of Object.entries(draft)) equal(value, '', `${key} 不为空`);
});

// 目录里一版供应商协议的读面行：必填字段写全，正文各键由各用例按需加。
function agreementRecord(over: Partial<SupplierAgreementRecord> = {}): SupplierAgreementRecord {
  return {
    objectId: 'SYN-SUPPLIER-TRUNK-CN-SG-01',
    version: 'v2',
    scope: 'SYN-SCOPE-01',
    status: 'PUBLISHED',
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    effectiveEndsAt: '2026-12-31T00:00:00Z',
    publishedAt: '2026-01-01T00:00:00Z',
    contentRegistered: true,
    supplier: 'supplier-1',
    legalEntity: 'legal-1',
    purchasePlan: 'plan-buy-1@v3',
    agreementScope: 'scope-procurement',
    agreementEffectiveStartsAt: '2026-01-01T00:00:00Z',
    agreementEffectiveEndsAt: '2026-12-31T00:00:00Z',
    registeredAt: '2026-01-01T00:00:00Z',
    ...over,
  };
}

// Covers: 票 party-commercial-uiux/14——基于此版本发新版本：对象标识、版本适用范围，以及正文里的供应商、责任法人、采购方案、协议
// 适用范围照抄；版本号、版本区间、协议区间一律留空（抄上一版的区间会让新旧两版重叠，解析时答「适用冲突」）。
test('基于此版本预填：抄对象、两层适用范围与三个引用，留版本号与两对区间', () => {
  deepEqual(supplierAgreementDraftFromRecord(agreementRecord()), {
    ...emptySupplierAgreementDraft(),
    objectId: 'SYN-SUPPLIER-TRUNK-CN-SG-01',
    scope: 'SYN-SCOPE-01',
    supplier: 'supplier-1',
    legalEntity: 'legal-1',
    agreementScope: 'scope-procurement',
    purchasePlan: 'plan-buy-1@v3',
  });
});

// Covers: 票 party-commercial-uiux/14——还原是组载荷的反向：人补上版本号与区间后，载荷正文里的供应商、法人、方案与协议范围
// 与记录所读一致。
test('基于此版本预填：人补齐留空的格后，载荷正文与记录一致', () => {
  const payload = supplierAgreementPayloadOf(
    {
      ...supplierAgreementDraftFromRecord(agreementRecord()),
      version: 'v3',
      effectiveStartsAt: '2027-01-01T00:00',
      agreementEffectiveStartsAt: '2027-01-01T00:00',
    },
    'Asia/Shanghai',
  );
  deepEqual(payload.supplierAgreement, {
    supplier: 'supplier-1',
    legalEntity: 'legal-1',
    scope: 'scope-procurement',
    purchasePlan: 'plan-buy-1@v3',
    effectiveStartsAt: '2026-12-31T16:00:00Z',
  });
  equal(payload.scope, 'SYN-SCOPE-01');
  ok(!('effectiveEndsAt' in payload), '上一版的止点没有被带过来');
});

// Covers: 票 party-commercial-uiux/14——contentRegistered 为假时正文各键缺席：只抄对象标识与版本适用范围，正文各格留空，
// 不拿记录上残留的键硬塞，也不崩。
test('基于此版本预填：未登记正文时只抄对象标识与版本适用范围', () => {
  deepEqual(
    supplierAgreementDraftFromRecord(
      agreementRecord({
        contentRegistered: false,
        supplier: undefined,
        legalEntity: undefined,
        purchasePlan: undefined,
        agreementScope: undefined,
        agreementEffectiveStartsAt: undefined,
        agreementEffectiveEndsAt: undefined,
        registeredAt: undefined,
      }),
    ),
    { ...emptySupplierAgreementDraft(), objectId: 'SYN-SUPPLIER-TRUNK-CN-SG-01', scope: 'SYN-SCOPE-01' },
  );
  // 布尔为假时即便记录上带着正文键（响应不合契约）也不抄：先看布尔，不拿键的有无去推。
  const stale = supplierAgreementDraftFromRecord(agreementRecord({ contentRegistered: false }));
  equal(stale.supplier, '');
  equal(stale.purchasePlan, '');
});

// Covers: 票 party-commercial-uiux/14——登记了正文但某个可缺的键缺席时不崩也不硬塞：缺的那一格空着，其余照抄。
test('基于此版本预填：正文里个别键缺席时，缺的格留空、其余照抄', () => {
  const draft = supplierAgreementDraftFromRecord(agreementRecord({ purchasePlan: undefined, agreementScope: undefined }));
  equal(draft.purchasePlan, '');
  equal(draft.agreementScope, '');
  equal(draft.supplier, 'supplier-1');
  equal(draft.legalEntity, 'legal-1');
});
