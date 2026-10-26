import { test } from 'node:test';
import { deepEqual, equal, notEqual, ok } from 'node:assert/strict';
import type { CustomerContractRecord } from './api';
import { emptyDeliveryConditionDraft } from './delivery-condition-section';
import { payloadKey } from './publication-draft-flow';
import {
  customerContractDraftFromRecord,
  customerContractFieldPaths,
  customerContractLocalProblems,
  emptyBindingDraft,
  emptyCustomerContractDraft,
  payloadOf,
  type CustomerContractDraft,
} from './customer-contract-form';

// 本文件钉的是客户合同表单草稿 → 载荷那一层纯函数（票 admin-write-faces/10）：
//   1. 一格两层，键名 = Go `CustomerContractBodyPayload` 的 json 标签（contractContent / preAcceptanceControl）；
//   2. 二选一控件只决定送哪一格，**不代判**：没选的行、没选的要求原样送上去，答回来的是构造门的拒绝；
//   3. 策略侧没有「无控制」取值——「不适用」是行的一格，不是策略选单里的一项（ADR-0115 Decision 一）；
//   4. 规则包同时进壳上的指名引用与正文，两处必须相等由服务端核。
//   5. 第三层（票 admin-write-faces/25）：合同层交付条件一格都没填整节缺席，填了任一格整节送、tightens 两格空着也送。
// 表单不算摘要、不裁任何门（伞票 07 硬句）：这里没有任何领域校验。

function filled(over: Partial<CustomerContractDraft> = {}): CustomerContractDraft {
  return {
    ...emptyCustomerContractDraft(),
    objectId: 'SYN-CONTRACT-02',
    version: 'v1',
    scope: 'SYN-SCOPE-01',
    effectiveStartsAt: '2026-10-01T00:00',
    customerAccount: 'SYN-ACCOUNT-02',
    rulePackage: 'SYN-RULEPKG-01',
    bindings: [
      { chargeScope: 'SYN-CHARGE-PREPAID', mode: 'policy', policy: 'SYN-FIN-CONTROL-01', inapplicabilityBasis: '' },
      { chargeScope: 'SYN-CHARGE-COD', mode: 'inapplicable', policy: '', inapplicabilityBasis: 'SYN-BASIS-COD-NA-01' },
    ],
    controlRequirement: 'REQUIRED',
    controlNotApplicableBasis: '',
    ...over,
  };
}

test('两层齐全的草稿组出的载荷键名镜像 Go 侧 json 标签，规则包同时进壳引用与正文', () => {
  deepEqual(payloadOf(filled()), {
    kind: 'CUSTOMER_CONTRACT',
    objectId: 'SYN-CONTRACT-02',
    version: 'v1',
    scope: 'SYN-SCOPE-01',
    effectiveStartsAt: '2026-10-01T00:00:00Z',
    references: { ACCEPTANCE_RULE_PACKAGE: 'SYN-RULEPKG-01' },
    customerContract: {
      contractContent: {
        customerAccount: 'SYN-ACCOUNT-02',
        rulePackage: 'SYN-RULEPKG-01',
        bindings: [
          { chargeScope: 'SYN-CHARGE-PREPAID', policy: 'SYN-FIN-CONTROL-01' },
          { chargeScope: 'SYN-CHARGE-COD', inapplicabilityBasis: 'SYN-BASIS-COD-NA-01' },
        ],
      },
      preAcceptanceControl: { requirement: 'REQUIRED' },
    },
  });
});

test('可缺的格留空即不进载荷：区间上界、指名引用、约定表；载荷里没有身份也没有摘要', () => {
  const payload = payloadOf(filled({ effectiveEndsAt: '', rulePackage: '', bindings: [] }));
  equal('effectiveEndsAt' in payload, false);
  equal('references' in payload, false);
  equal('bindings' in payload.customerContract!.contractContent, false);
  equal(payload.customerContract!.contractContent.rulePackage, '');
  // 客户账户是必填格（ADR-0177 决定一）：留空也照送空串，由服务端点名这一格，表单不代填、不从别处推。
  equal(payloadOf(filled({ customerAccount: '' })).customerContract!.contractContent.customerAccount, '');
  for (const forbidden of ['tenantId', 'submitter', 'contentDigest', 'approval']) {
    equal(forbidden in payload, false, `${forbidden} 不该出现在载荷里`);
  }
  const bounded = payloadOf(filled({ effectiveEndsAt: '2027-01-01T00:00' }));
  equal(bounded.effectiveEndsAt, '2027-01-01T00:00:00Z');
});

// Covers: 票 party-commercial-uiux/10——时刻格是日期时间控件交出的本地墙钟，按显示时区换成 UTC；只有空白的止点同空着。
test('时刻格的本地墙钟按显示时区换成 UTC', () => {
  const payload = payloadOf(filled({ effectiveStartsAt: ' 2026-10-01T08:00 ', effectiveEndsAt: '2027-01-01T00:00:30' }), 'Asia/Shanghai');
  equal(payload.effectiveStartsAt, '2026-10-01T00:00:00Z');
  equal(payload.effectiveEndsAt, '2026-12-31T16:00:30Z');
  equal('effectiveEndsAt' in payloadOf(filled({ effectiveEndsAt: '  ' }), 'Asia/Shanghai'), false);
});

// Covers: 票 party-commercial-uiux/10——时刻格换不成时刻是本地问题，落在那一格路径上，挡住预览；空着不是。
test('时刻格换不成时刻才是本地问题，落在该格路径上', () => {
  deepEqual(customerContractLocalProblems(filled({ effectiveStartsAt: '2026-10-01', effectiveEndsAt: 'tomorrow' }), 'Asia/Shanghai'), {
    effectiveStartsAt: ['时间格式不对，请用日期时间选择器重新选'],
    effectiveEndsAt: ['时间格式不对，请用日期时间选择器重新选'],
  });
  deepEqual(customerContractLocalProblems(filled(), 'Asia/Shanghai'), {});
  deepEqual(customerContractLocalProblems(emptyCustomerContractDraft(), 'Asia/Shanghai'), {});
});

test('二选一控件只决定送哪一格，不代判：没选模式的行只送范围，选了模式但没填的行同样只送范围', () => {
  const payload = payloadOf(
    filled({
      bindings: [
        { chargeScope: 'SYN-CHARGE-A', mode: '', policy: 'ignored', inapplicabilityBasis: 'ignored' },
        { chargeScope: 'SYN-CHARGE-B', mode: 'policy', policy: '', inapplicabilityBasis: 'stale' },
        { chargeScope: 'SYN-CHARGE-C', mode: 'inapplicable', policy: 'stale', inapplicabilityBasis: '' },
      ],
    }),
  );
  deepEqual(payload.customerContract!.contractContent.bindings, [
    { chargeScope: 'SYN-CHARGE-A' },
    { chargeScope: 'SYN-CHARGE-B' },
    { chargeScope: 'SYN-CHARGE-C' },
  ]);
});

test('合同级声明一节永远在场：没选要求就送空要求让服务端答；依据只随「不适用」送', () => {
  const unselected = payloadOf(filled({ controlRequirement: '', controlNotApplicableBasis: 'typed-early' }));
  deepEqual(unselected.customerContract!.preAcceptanceControl, { requirement: '' });

  const notApplicable = payloadOf(
    filled({ controlRequirement: 'NOT_APPLICABLE', controlNotApplicableBasis: 'CONTRACT-CLAUSE/NO-CONTROL' }),
  );
  deepEqual(notApplicable.customerContract!.preAcceptanceControl, {
    requirement: 'NOT_APPLICABLE',
    notApplicableBasis: 'CONTRACT-CLAUSE/NO-CONTROL',
  });
  // 「不适用」却没写依据：照样送上去，缺依据由领域构造门答，表单不补一句。
  const bare = payloadOf(filled({ controlRequirement: 'NOT_APPLICABLE', controlNotApplicableBasis: '' }));
  deepEqual(bare.customerContract!.preAcceptanceControl, { requirement: 'NOT_APPLICABLE' });
  // 切回「要求控制」后早先填的依据不随行：要求控制不得带依据是领域门，但一格看不见的值送上去会让人找不到成因。
  const switchedBack = payloadOf(filled({ controlRequirement: 'REQUIRED', controlNotApplicableBasis: 'typed-early' }));
  deepEqual(switchedBack.customerContract!.preAcceptanceControl, { requirement: 'REQUIRED' });
});

test('改任何一格载荷键就变（预览作废由流程组件按键判）；换约定行序也算改动——归一是服务端的事', () => {
  const base = payloadKey(payloadOf(filled()));
  notEqual(payloadKey(payloadOf(filled({ controlRequirement: 'NOT_APPLICABLE' }))), base);
  notEqual(payloadKey(payloadOf(filled({ bindings: [filled().bindings[1]!, filled().bindings[0]!] }))), base);
  equal(payloadKey(payloadOf(filled())), base);
});

test('表单认领的 JSON 路径随约定行数长：每行三格 + 行本身；壳与两层的固定格都在', () => {
  const paths = customerContractFieldPaths(filled());
  for (const want of [
    'objectId',
    'version',
    'scope',
    'effectiveStartsAt',
    'effectiveEndsAt',
    'references.ACCEPTANCE_RULE_PACKAGE',
    'customerContract.contractContent',
    'customerContract.contractContent.customerAccount',
    'customerContract.contractContent.rulePackage',
    'customerContract.contractContent.bindings[0]',
    'customerContract.contractContent.bindings[0].chargeScope',
    'customerContract.contractContent.bindings[0].policy',
    'customerContract.contractContent.bindings[0].inapplicabilityBasis',
    'customerContract.contractContent.bindings[1]',
    'customerContract.preAcceptanceControl.requirement',
    'customerContract.preAcceptanceControl.notApplicableBasis',
  ]) {
    ok(paths.includes(want), `缺路径 ${want}`);
  }
  ok(!paths.includes('customerContract.contractContent.bindings[2]'));
  ok(!paths.includes('kind'), 'kind 是固定的，不是表单的一格');
});

test('空草稿有一行空约定等人填，两层都未选；空行的默认模式是未选而不是任一格', () => {
  const draft = emptyCustomerContractDraft();
  equal(draft.bindings.length, 1);
  deepEqual(draft.bindings[0], emptyBindingDraft());
  equal(emptyBindingDraft().mode, '');
  equal(draft.controlRequirement, '');
  deepEqual(draft.deliveryConditions, emptyDeliveryConditionDraft(), '第三层空着：这一版不声明合同层交付条件');
});

test('第三层：交付条件一格都没填整节缺席；填了任一格整节进 customerContract.deliveryConditions 且 tightens 两格永远送', () => {
  equal('deliveryConditions' in payloadOf(filled()).customerContract!, false);

  const declared = payloadOf(
    filled({
      deliveryConditions: {
        ...emptyDeliveryConditionDraft(),
        methodsText: 'METHOD/in-person',
        recipientScopeRule: 'RULE/recipient-scope-1',
        proofOfDeliveryRule: 'RULE/proof-contract-1',
        tightensObjectId: 'SYN-PROD-01',
        tightensVersion: 'v2',
      },
    }),
  );
  deepEqual(declared.customerContract!.deliveryConditions, {
    methods: ['METHOD/in-person'],
    recipientScopeRule: 'RULE/recipient-scope-1',
    proofOfDeliveryRule: 'RULE/proof-contract-1',
    tightens: { objectId: 'SYN-PROD-01', version: 'v2' },
  });
  // 只填了方式、没指名所收紧的产品版本：tightens 两格空着也送，让服务端点名 tightens.objectId / tightens.version，
  // 而不是缺键落成一句不指格的「未受理」。
  const untightened = payloadOf(filled({ deliveryConditions: { ...emptyDeliveryConditionDraft(), methodsText: 'METHOD/in-person' } }));
  deepEqual(untightened.customerContract!.deliveryConditions?.tightens, { objectId: '', version: '' });

  const paths = customerContractFieldPaths(filled({ deliveryConditions: { ...emptyDeliveryConditionDraft(), methodsText: 'a\nb' } }));
  for (const want of [
    'customerContract.deliveryConditions',
    'customerContract.deliveryConditions.methods[0]',
    'customerContract.deliveryConditions.methods[1]',
    'customerContract.deliveryConditions.recipientScopeRule',
    'customerContract.deliveryConditions.proofOfDeliveryRule',
    'customerContract.deliveryConditions.tightens',
    'customerContract.deliveryConditions.tightens.objectId',
    'customerContract.deliveryConditions.tightens.version',
  ]) {
    ok(paths.includes(want), `缺路径 ${want}`);
  }
});

// 目录里一版客户合同的读面行：必填字段写全，可缺的键由各用例按需加。
function contractRecord(over: Partial<CustomerContractRecord> = {}): CustomerContractRecord {
  return {
    objectId: 'SYN-CONTRACT-02',
    version: 'v3',
    scope: 'SYN-SCOPE-01',
    status: 'PUBLISHED',
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    effectiveEndsAt: '2026-12-31T00:00:00Z',
    publishedAt: '2026-01-01T00:00:00Z',
    contentRegistered: true,
    rulePackageId: 'SYN-RULEPKG-01',
    declaredAt: '2026-01-01T00:00:00Z',
    bindings: [
      { chargeScope: 'SYN-CHARGE-PREPAID', policyId: 'SYN-FIN-CONTROL-01' },
      { chargeScope: 'SYN-CHARGE-COD', inapplicabilityBasis: 'SYN-BASIS-COD-NA-01' },
    ],
    deliveryConditions: {
      tightens: { objectId: 'SYN-PROD-01', version: 'v2' },
      methods: ['METHOD/in-person', 'METHOD/locker'],
      recipientScopeRule: 'RULE/recipient-scope-1',
      proofOfDeliveryRule: 'RULE/proof-contract-1',
      declaredAt: '2026-01-01T00:00:00Z',
    },
    ...over,
  };
}

// Covers: 票 party-commercial-uiux/14——基于此版本发新版本：对象标识、版本适用范围、规则包、两种约定行（指名策略 / 显式不适用）、
// 合同层交付条件（含所收紧的产品版本）照抄；版本号、生效区间、货主客户账户、合同级控制声明读面没有或要人填，留空。
test('基于此版本预填：抄正文与交付条件，留版本号、区间、客户账户与合同级控制声明', () => {
  deepEqual(customerContractDraftFromRecord(contractRecord()), {
    ...emptyCustomerContractDraft(),
    objectId: 'SYN-CONTRACT-02',
    scope: 'SYN-SCOPE-01',
    rulePackage: 'SYN-RULEPKG-01',
    bindings: [
      { chargeScope: 'SYN-CHARGE-PREPAID', mode: 'policy', policy: 'SYN-FIN-CONTROL-01', inapplicabilityBasis: '' },
      { chargeScope: 'SYN-CHARGE-COD', mode: 'inapplicable', policy: '', inapplicabilityBasis: 'SYN-BASIS-COD-NA-01' },
    ],
    deliveryConditions: {
      methodsText: 'METHOD/in-person\nMETHOD/locker',
      recipientScopeRule: 'RULE/recipient-scope-1',
      proofOfDeliveryRule: 'RULE/proof-contract-1',
      tightensObjectId: 'SYN-PROD-01',
      tightensVersion: 'v2',
    },
  });
});

// Covers: 票 party-commercial-uiux/14——还原是组载荷的反向：人补上版本号、起点与客户账户、选了合同级声明后，载荷里的规则包、
// 两种约定行与交付条件与记录所读一致，不多一格也不少一格。
test('基于此版本预填：人补齐留空的格后，载荷里的正文与记录一致', () => {
  const restored = customerContractDraftFromRecord(contractRecord());
  const payload = payloadOf(
    {
      ...restored,
      version: 'v4',
      effectiveStartsAt: '2027-01-01T00:00',
      customerAccount: 'SYN-ACCOUNT-02',
      controlRequirement: 'REQUIRED',
    },
    'Asia/Shanghai',
  );
  deepEqual(payload.references, { ACCEPTANCE_RULE_PACKAGE: 'SYN-RULEPKG-01' });
  deepEqual(payload.customerContract!.contractContent.bindings, [
    { chargeScope: 'SYN-CHARGE-PREPAID', policy: 'SYN-FIN-CONTROL-01' },
    { chargeScope: 'SYN-CHARGE-COD', inapplicabilityBasis: 'SYN-BASIS-COD-NA-01' },
  ]);
  deepEqual(payload.customerContract!.deliveryConditions, {
    methods: ['METHOD/in-person', 'METHOD/locker'],
    recipientScopeRule: 'RULE/recipient-scope-1',
    proofOfDeliveryRule: 'RULE/proof-contract-1',
    tightens: { objectId: 'SYN-PROD-01', version: 'v2' },
  });
  // 不预填就发是发不出去的：留空的版本号与起点由服务端点名，本地没有替人抄一份上一版的区间。
  const bare = payloadOf(restored, 'Asia/Shanghai');
  equal(bare.version, '');
  equal('effectiveEndsAt' in bare, false);
});

// Covers: 票 party-commercial-uiux/14——contentRegistered 为假时正文不在（零约定与未登记在读面上撞成同一个空数组，由布尔分开）：
// 只还原对象标识、版本适用范围与交付条件；规则包、约定行都按空草稿，不拿记录上残留的键硬塞。
test('基于此版本预填：未登记正文时只抄对象标识、适用范围与交付条件', () => {
  const draft = customerContractDraftFromRecord(
    contractRecord({ contentRegistered: false, rulePackageId: 'SYN-STALE', bindings: [{ chargeScope: 'SYN-STALE', policyId: 'SYN-STALE' }] }),
  );
  equal(draft.objectId, 'SYN-CONTRACT-02');
  equal(draft.scope, 'SYN-SCOPE-01');
  equal(draft.rulePackage, '');
  deepEqual(draft.bindings, [emptyBindingDraft()]);
  equal(draft.deliveryConditions.tightensObjectId, 'SYN-PROD-01');
  equal(draft.deliveryConditions.methodsText, 'METHOD/in-person\nMETHOD/locker');
});

// Covers: 票 party-commercial-uiux/14——读面上可缺的键缺席时不崩也不硬塞：没有交付条件即整节空草稿（不由 contentRegistered 推）；
// 规则包缺席即空；登记了正文但零约定，照空草稿预开一行；约定行两格都缺或都在（不合「恰一」）时不替人选模式，也不抄值。
test('基于此版本预填：可缺的键缺席时不崩也不硬塞', () => {
  const bare = customerContractDraftFromRecord(
    contractRecord({ rulePackageId: undefined, bindings: [], deliveryConditions: undefined, declaredAt: undefined }),
  );
  equal(bare.rulePackage, '');
  deepEqual(bare.bindings, [emptyBindingDraft()], '零约定照空草稿预开一行等人填');
  deepEqual(bare.deliveryConditions, emptyDeliveryConditionDraft(), '键缺席即这一版没有交付条件');
  equal('deliveryConditions' in payloadOf(bare).customerContract!, false, '空的一节不进载荷');

  const noTightens = customerContractDraftFromRecord(
    contractRecord({
      deliveryConditions: { methods: [], recipientScopeRule: 'RULE/r', proofOfDeliveryRule: 'RULE/p', declaredAt: '2026-01-01T00:00:00Z' },
    }),
  );
  deepEqual(noTightens.deliveryConditions, {
    ...emptyDeliveryConditionDraft(),
    recipientScopeRule: 'RULE/r',
    proofOfDeliveryRule: 'RULE/p',
  });

  const odd = customerContractDraftFromRecord(
    contractRecord({
      bindings: [
        { chargeScope: 'SYN-CHARGE-NONE' },
        { chargeScope: 'SYN-CHARGE-BOTH', policyId: 'SYN-FIN-CONTROL-01', inapplicabilityBasis: 'SYN-BASIS-01' },
      ],
    }),
  );
  deepEqual(odd.bindings, [
    { ...emptyBindingDraft(), chargeScope: 'SYN-CHARGE-NONE' },
    { ...emptyBindingDraft(), chargeScope: 'SYN-CHARGE-BOTH' },
  ]);
});
