import { test } from 'node:test';
import { deepEqual, equal, notEqual, ok } from 'node:assert/strict';
import { payloadKey } from './publication-draft-flow';
import {
  customerContractFieldPaths,
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
// 表单不算摘要、不裁任何门（伞票 07 硬句）：这里没有任何领域校验。

function filled(over: Partial<CustomerContractDraft> = {}): CustomerContractDraft {
  return {
    ...emptyCustomerContractDraft(),
    objectId: 'SYN-CONTRACT-02',
    version: 'v1',
    scope: 'SYN-SCOPE-01',
    effectiveStartsAt: '2026-10-01T00:00:00Z',
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
  for (const forbidden of ['tenantId', 'submitter', 'contentDigest', 'approval']) {
    equal(forbidden in payload, false, `${forbidden} 不该出现在载荷里`);
  }
  const bounded = payloadOf(filled({ effectiveEndsAt: '2027-01-01T00:00:00Z' }));
  equal(bounded.effectiveEndsAt, '2027-01-01T00:00:00Z');
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
});
