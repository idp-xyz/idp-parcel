import { test } from 'node:test';
import { deepEqual } from 'node:assert/strict';
import { unrenderedClaimedPaths } from './publication-form-shared';
import {
  acceptanceRulePackageFieldPaths,
  acceptanceRulePackageRenderedPaths,
  emptyAcceptanceRulePackageDraft,
  emptyAmendmentRuleDraft,
  emptyAsOfPolicyDraft,
  emptyAssembledRuleDraft,
  emptyFinalRuleDraft,
  type AcceptanceRulePackageDraft,
} from './acceptance-rule-package-form';
import {
  authorizationRuleFieldPaths,
  authorizationRuleRenderedPaths,
  emptyAuthorizationRuleDraft,
  emptyCancellationRowDraft,
} from './authorization-rule-form';
import { creditPolicyFieldPaths, creditPolicyRenderedPaths } from './credit-policy-form';
import {
  customerContractFieldPaths,
  customerContractRenderedPaths,
  emptyBindingDraft,
  emptyCustomerContractDraft,
  type CustomerContractDraft,
} from './customer-contract-form';
import {
  emptyServiceRuleDraft,
  serviceRuleFieldPaths,
  serviceRuleRenderedPaths,
  withClaimDeadlineRowAdded,
  withMinimumMaterialsRowAdded,
  withMinimumMaterialsRowPatched,
  type ServiceRuleDraft,
} from './customer-service-rule-form';
import {
  controlPolicyFieldPaths,
  controlPolicyRenderedPaths,
  emptyControlPolicyDraft,
  withControlRowAdded,
} from './pre-acceptance-financial-control-policy-form';
import { pricePolicyFieldPaths, pricePolicyRenderedPaths } from './price-policy-form';
import { emptyServiceProductDraft, serviceProductFieldPaths, serviceProductRenderedPaths } from './service-product-form';
import { settlementPolicyFieldPaths, settlementPolicyRenderedPaths } from './settlement-policy-form';
import { supplierAgreementFieldPaths, supplierAgreementRenderedPaths } from './supplier-agreement-form';

// 本文件钉的是票 admin-write-faces/22 判据 3：每张发布表单交给 PublicationDraftFlow 的认领表（*FieldPaths）里的每一条路径，
// 组件都有一处显它的 Problems（*RenderedPaths）。认领了却没处显，服务端点到那一格的话会被静默吞掉——公共半边按「已认领」
// 不单列，表单又没地方显（票 17 评审点名的 'authorizationRule' 节根就是这样漏的）。
//
// 为什么「渲染了哪些路径」是各 *-form.ts 里手抄 JSX 的一份纯声明，而不是从组件渲染结果取证：本包的测试只编 *.test.ts 与
// 它们引到的 .ts（tsconfig.test.json），.tsx 会连带拖进 react 与 @idpxyz/ui-primitives，node --test 下跑不起来；而认领表本来
// 就是一份「表单渲染了哪几条路径」的声明，第二份从 JSX 那一侧抄出来、放在它旁边，两份对不上就是有一条路径两边说法不一。
// 这条测试守的是两份声明一致，JSX 与声明一致仍要靠改 JSX 的人同步改声明（各声明的注释都写了这一句）与非作者评审对照。
//
// 空草稿与带行的草稿各比一次：行按行数长、组按选中数长、条件格按选项显隐，认领表里这些分叉各走一遍。

function assertEveryClaimedPathRendered(form: string, claimed: readonly string[], rendered: readonly string[]) {
  deepEqual(unrenderedClaimedPaths(claimed, rendered), [], `${form}：认领了却没处显 Problems 的路径`);
}

test('service product：壳五格、引用表每行（含空行）与交付条件一节（含方式每项）都有处显', () => {
  const empty = emptyServiceProductDraft();
  assertEveryClaimedPathRendered('SERVICE_PRODUCT 空草稿', serviceProductFieldPaths(empty), serviceProductRenderedPaths(empty));
  const withRows = {
    ...empty,
    references: [
      { kind: 'ACCEPTANCE_RULE_PACKAGE', objectId: 'SYN-RP-01' },
      { kind: '', objectId: '' },
      { kind: 'CUSTOMER_CONTRACT', objectId: '' },
    ],
    deliveryConditions: { ...empty.deliveryConditions, methodsText: 'SYN-METHOD-A\n\nSYN-METHOD-B', recipientScopeRule: 'SYN-RULE-01' },
  };
  assertEveryClaimedPathRendered('SERVICE_PRODUCT 带行带交付条件', serviceProductFieldPaths(withRows), serviceProductRenderedPaths(withRows));
});

test('customer contract：合同级声明、约定行两格无论显隐、第三层交付条件（含 tightens 两格与方式每项）都有处显', () => {
  const empty = emptyCustomerContractDraft();
  assertEveryClaimedPathRendered('CUSTOMER_CONTRACT 空草稿', customerContractFieldPaths(empty), customerContractRenderedPaths(empty));
  const modes: CustomerContractDraft['controlRequirement'][] = ['', 'REQUIRED', 'NOT_APPLICABLE'];
  for (const controlRequirement of modes) {
    const draft: CustomerContractDraft = {
      ...empty,
      controlRequirement,
      bindings: [
        { ...emptyBindingDraft(), mode: 'policy', policy: 'SYN-CTRL-01' },
        { ...emptyBindingDraft(), mode: 'inapplicable', inapplicabilityBasis: 'SYN-BASIS-01' },
        emptyBindingDraft(),
      ],
      deliveryConditions: { ...empty.deliveryConditions, methodsText: 'SYN-METHOD-A\nSYN-METHOD-B', tightensObjectId: 'SYN-PROD-01' },
    };
    assertEveryClaimedPathRendered(
      `CUSTOMER_CONTRACT 要求=${controlRequirement || '未选'} 带三种模式的行`,
      customerContractFieldPaths(draft),
      customerContractRenderedPaths(draft),
    );
  }
});

test('authorization rule：正文根、目录根与每行都有处显', () => {
  const empty = emptyAuthorizationRuleDraft();
  assertEveryClaimedPathRendered('AUTHORIZATION_RULE 空草稿', authorizationRuleFieldPaths(empty), authorizationRuleRenderedPaths(empty));
  const withRows = { ...empty, rows: [emptyCancellationRowDraft(), { party: 'CUSTOMER', rule: 'SYN-RULE-01' }] };
  assertEveryClaimedPathRendered('AUTHORIZATION_RULE 带行', authorizationRuleFieldPaths(withRows), authorizationRuleRenderedPaths(withRows));
});

test('acceptance rule package：五个节根、各行、勾选组每项都有处显', () => {
  const empty = emptyAcceptanceRulePackageDraft();
  assertEveryClaimedPathRendered(
    'ACCEPTANCE_RULE_PACKAGE 空草稿',
    acceptanceRulePackageFieldPaths(empty),
    acceptanceRulePackageRenderedPaths(empty),
  );
  const filled: AcceptanceRulePackageDraft = {
    ...empty,
    body: { ...empty.body, rules: [emptyAssembledRuleDraft(), emptyAssembledRuleDraft()] },
    asOfPolicies: [emptyAsOfPolicyDraft()],
    acceptanceContent: { applicableGroups: ['SYN-GROUP-A', 'SYN-GROUP-B'], manualReview: 'REQUIRED' },
    intakeQualification: { sources: ['SYN-SOURCE-A'], qualifications: ['SYN-QUAL-A', 'SYN-QUAL-B'] },
    finalRules: [emptyFinalRuleDraft(), emptyFinalRuleDraft()],
    finalRuleValidity: { anchor: 'SYN-ANCHOR', duration: 'P7D' },
    sourceDataAmendment: { closed: 'false', rules: [emptyAmendmentRuleDraft()] },
  };
  assertEveryClaimedPathRendered(
    'ACCEPTANCE_RULE_PACKAGE 六节都有内容',
    acceptanceRulePackageFieldPaths(filled),
    acceptanceRulePackageRenderedPaths(filled),
  );
});

test('customer service rule：适用对象两种选法、期限行、材料行与清单每项都有处显', () => {
  const empty = emptyServiceRuleDraft();
  assertEveryClaimedPathRendered('CUSTOMER_SERVICE_RULE 空草稿', serviceRuleFieldPaths(empty), serviceRuleRenderedPaths(empty));
  const appliesTos: ServiceRuleDraft['appliesTo'][] = ['serviceProduct', 'customerContract'];
  for (const appliesTo of appliesTos) {
    let draft: ServiceRuleDraft = { ...empty, appliesTo, appliesToId: 'SYN-OBJ-01' };
    draft = withClaimDeadlineRowAdded(withClaimDeadlineRowAdded(draft));
    draft = withMinimumMaterialsRowAdded(draft);
    draft = withMinimumMaterialsRowPatched(draft, 0, { materials: 'SYN-DOC-A\nSYN-DOC-B\n\nSYN-DOC-C' });
    assertEveryClaimedPathRendered(
      `CUSTOMER_SERVICE_RULE 适用对象=${appliesTo} 带两张表`,
      serviceRuleFieldPaths(draft),
      serviceRuleRenderedPaths(draft),
    );
  }
});

test('pre-acceptance financial control policy：联合通过条件与每条控制行都有处显', () => {
  const empty = emptyControlPolicyDraft();
  assertEveryClaimedPathRendered('PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY 空草稿', controlPolicyFieldPaths(empty), controlPolicyRenderedPaths(empty));
  const withRows = withControlRowAdded(withControlRowAdded(empty));
  assertEveryClaimedPathRendered(
    'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY 带行',
    controlPolicyFieldPaths(withRows),
    controlPolicyRenderedPaths(withRows),
  );
});

test('credit / price / supplier agreement / settlement policy：认领表是常量，逐条都有处显', () => {
  assertEveryClaimedPathRendered('CREDIT_POLICY', creditPolicyFieldPaths, creditPolicyRenderedPaths);
  assertEveryClaimedPathRendered('PRICE_POLICY', pricePolicyFieldPaths, pricePolicyRenderedPaths);
  assertEveryClaimedPathRendered('SUPPLIER_AGREEMENT', supplierAgreementFieldPaths, supplierAgreementRenderedPaths);
  assertEveryClaimedPathRendered('SETTLEMENT_POLICY', settlementPolicyFieldPaths, settlementPolicyRenderedPaths);
});
