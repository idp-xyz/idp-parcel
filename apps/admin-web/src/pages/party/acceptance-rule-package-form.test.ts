import { test } from 'node:test';
import { deepEqual, equal, notEqual, ok } from 'node:assert/strict';
import { payloadKey } from './publication-draft-flow';
import {
  acceptanceRulePackageFieldPaths,
  declarationSections,
  emptyAcceptanceRulePackageDraft,
  emptyAmendmentRuleDraft,
  emptyAsOfPolicyDraft,
  emptyAssembledRuleDraft,
  emptyFinalRuleDraft,
  payloadOf,
  sectionDeclared,
  setOptionsOf,
  vocabularyStateOf,
  type AcceptanceRulePackageDraft,
} from './acceptance-rule-package-form';

// 本文件钉的是接单规则包表单草稿 → 载荷那一层纯函数（票 admin-write-faces/12）：
//   1. 一格分节，键名 = Go `AcceptanceRulePackageBodyPayload` 的 json 标签（rulePackageBody / asOfPolicies /
//      acceptanceContent / intakeQualification / finalRules / finalRuleValidity / sourceDataAmendment）；
//   2. **某一节整节留空 = 该通道未声明**：载荷里不带那一节；节里只要有一格有字或有一行，整节照原样上送——
//      没选的码、空的行都不代判，答回来的是服务端的拒绝；
//   3. 各节封闭集的码只从词表读口来（票 20），403 是可辨的「未配置」显占位，这里没有内置码、没有默认选中；
//   4. `sourceDataAmendment.closed` 没选就不送这一格（服务端答「须在场」），不替登记方在 true / false 里挑。
// 表单不算摘要、不裁任何门（伞票 07 硬句）：这里没有任何领域校验。

function filled(over: Partial<AcceptanceRulePackageDraft> = {}): AcceptanceRulePackageDraft {
  return {
    ...emptyAcceptanceRulePackageDraft(),
    objectId: 'SYN-RULEPKG-02',
    version: 'v1',
    scope: 'SYN-SCOPE-01',
    effectiveStartsAt: '2026-10-01T00:00:00Z',
    body: {
      serviceProduct: 'SYN-PROD-CN-SG-EXPRESS',
      contract: 'SYN-CONTRACT-01',
      legalEntity: 'SYN-LE-CN-01',
      scope: 'SYN-SCOPE-01',
      effectiveStartsAt: '2026-10-01T00:00:00Z',
      effectiveEndsAt: '',
      rules: [
        { category: 'MINIMUM_INGRESS_IDENTITY', reference: 'RULE/ingress-identity' },
        { category: 'REGULATORY_SOURCE_DOCUMENT', reference: 'RULE/customs-doc' },
      ],
    },
    asOfPolicies: [{ judgment: 'NETWORK_REACHABILITY', semantics: 'AT_SUBMISSION', policyVersion: 'asof-policy/v1' }],
    acceptanceContent: { applicableGroups: ['REQUIRED_DOCUMENT', 'CUSTOMER_RELATIONSHIP'], manualReview: 'REQUIRED' },
    intakeQualification: { sources: ['NODE_INTAKE', 'OFFSITE_PICKUP'], qualifications: ['INTAKE-QUAL/realname'] },
    finalRules: [{ outcome: 'EFFECTIVE_DELIVERY', finalKind: 'FINAL/delivery' }],
    finalRuleValidity: { anchor: 'CHANNEL_RESULT_OBSERVED', duration: 'P3DT12H' },
    sourceDataAmendment: {
      closed: 'false',
      rules: [{ dataGroup: 'consignee.address', stage: 'ACCEPTED_NOT_YET_RECEIVED', intent: 'CORRECTION', allowance: 'ALLOWED' }],
    },
    ...over,
  };
}

test('齐全的草稿组出的载荷键名镜像 Go 的 json 标签：正文一节 + 六节声明，行照原样', () => {
  deepEqual(payloadOf(filled()), {
    kind: 'ACCEPTANCE_RULE_PACKAGE',
    objectId: 'SYN-RULEPKG-02',
    version: 'v1',
    scope: 'SYN-SCOPE-01',
    effectiveStartsAt: '2026-10-01T00:00:00Z',
    acceptanceRulePackage: {
      rulePackageBody: {
        serviceProduct: 'SYN-PROD-CN-SG-EXPRESS',
        contract: 'SYN-CONTRACT-01',
        legalEntity: 'SYN-LE-CN-01',
        scope: 'SYN-SCOPE-01',
        effectiveStartsAt: '2026-10-01T00:00:00Z',
        rules: [
          { category: 'MINIMUM_INGRESS_IDENTITY', reference: 'RULE/ingress-identity' },
          { category: 'REGULATORY_SOURCE_DOCUMENT', reference: 'RULE/customs-doc' },
        ],
      },
      asOfPolicies: [{ judgment: 'NETWORK_REACHABILITY', semantics: 'AT_SUBMISSION', policyVersion: 'asof-policy/v1' }],
      acceptanceContent: { applicableGroups: ['REQUIRED_DOCUMENT', 'CUSTOMER_RELATIONSHIP'], manualReview: 'REQUIRED' },
      intakeQualification: { sources: ['NODE_INTAKE', 'OFFSITE_PICKUP'], qualifications: ['INTAKE-QUAL/realname'] },
      finalRules: [{ outcome: 'EFFECTIVE_DELIVERY', finalKind: 'FINAL/delivery' }],
      finalRuleValidity: { anchor: 'CHANNEL_RESULT_OBSERVED', duration: 'P3DT12H' },
      sourceDataAmendment: {
        closed: false,
        rules: [{ dataGroup: 'consignee.address', stage: 'ACCEPTED_NOT_YET_RECEIVED', intent: 'CORRECTION', allowance: 'ALLOWED' }],
      },
    },
  });
});

test('可缺的格留空即不进载荷：壳与正文各自的区间上界；载荷里没有身份、摘要，也没有壳上的指名引用', () => {
  const open = payloadOf(filled());
  equal('effectiveEndsAt' in open, false);
  equal('effectiveEndsAt' in open.acceptanceRulePackage!.rulePackageBody, false);
  equal('references' in open, false);
  for (const forbidden of ['tenantId', 'submitter', 'contentDigest', 'approval']) {
    equal(forbidden in open, false, `${forbidden} 不该出现在载荷里`);
  }
  const bounded = payloadOf(
    filled({
      effectiveEndsAt: '2027-01-01T00:00:00Z',
      body: { ...filled().body, effectiveEndsAt: '2026-12-31T00:00:00Z' },
    }),
  );
  equal(bounded.effectiveEndsAt, '2027-01-01T00:00:00Z');
  equal(bounded.acceptanceRulePackage!.rulePackageBody.effectiveEndsAt, '2026-12-31T00:00:00Z');
});

test('空节即未声明：六节全留空时载荷只有正文一节；空草稿的六节都是未声明', () => {
  const bodyOnly = payloadOf(
    filled({
      asOfPolicies: [],
      acceptanceContent: { applicableGroups: [], manualReview: '' },
      intakeQualification: { sources: [], qualifications: [] },
      finalRules: [],
      finalRuleValidity: { anchor: '', duration: '' },
      sourceDataAmendment: { closed: '', rules: [] },
    }),
  );
  deepEqual(Object.keys(bodyOnly.acceptanceRulePackage!), ['rulePackageBody']);
  for (const section of declarationSections) {
    equal(sectionDeclared(emptyAcceptanceRulePackageDraft(), section), false, `${section} 在空草稿上不该算已声明`);
    equal(sectionDeclared(filled(), section), true, `${section} 在齐全草稿上该算已声明`);
  }
});

test('节里只要有一格有字或有一行，整节就照原样上送——空行、没选的码都不代判、不补默认', () => {
  const empty = emptyAcceptanceRulePackageDraft();

  const oneEmptyAnchor = payloadOf(filled({ ...empty, asOfPolicies: [emptyAsOfPolicyDraft()] }));
  deepEqual(oneEmptyAnchor.acceptanceRulePackage!.asOfPolicies, [{ judgment: '', semantics: '', policyVersion: '' }]);

  const reviewOnly = payloadOf(filled({ ...empty, acceptanceContent: { applicableGroups: [], manualReview: 'REQUIRED' } }));
  deepEqual(reviewOnly.acceptanceRulePackage!.acceptanceContent, { applicableGroups: [], manualReview: 'REQUIRED' });
  equal('intakeQualification' in reviewOnly.acceptanceRulePackage!, false);

  const groupsOnly = payloadOf(filled({ ...empty, acceptanceContent: { applicableGroups: ['REQUIRED_DOCUMENT'], manualReview: '' } }));
  deepEqual(groupsOnly.acceptanceRulePackage!.acceptanceContent, { applicableGroups: ['REQUIRED_DOCUMENT'], manualReview: '' });

  // 真没有硬资格也要声明来源允许：来源在、硬资格显式为空，整节在场。
  const sourcesOnly = payloadOf(filled({ ...empty, intakeQualification: { sources: ['NODE_INTAKE'], qualifications: [] } }));
  deepEqual(sourcesOnly.acceptanceRulePackage!.intakeQualification, { sources: ['NODE_INTAKE'], qualifications: [] });

  const emptyFinalRow = payloadOf(filled({ ...empty, finalRules: [emptyFinalRuleDraft()] }));
  deepEqual(emptyFinalRow.acceptanceRulePackage!.finalRules, [{ outcome: '', finalKind: '' }]);

  // 只填了时长没选起算种类：整格照送，起算种类那一格由服务端点名；只有有效期没有终局行同样由服务端整项拒。
  const durationOnly = payloadOf(filled({ ...empty, finalRuleValidity: { anchor: '', duration: 'P7D' } }));
  deepEqual(durationOnly.acceptanceRulePackage!.finalRuleValidity, { anchor: '', duration: 'P7D' });
  equal('finalRules' in durationOnly.acceptanceRulePackage!, false);
});

test('资料修订允许一节：closed 没选就不送这一格让服务端答「须在场」；选了 true / false 才进载荷；closed=true 零行照送', () => {
  const empty = emptyAcceptanceRulePackageDraft();
  const rowsWithoutClosed = payloadOf(
    filled({ ...empty, sourceDataAmendment: { closed: '', rules: [emptyAmendmentRuleDraft()] } }),
  );
  deepEqual(rowsWithoutClosed.acceptanceRulePackage!.sourceDataAmendment, {
    rules: [{ dataGroup: '', stage: '', intent: '', allowance: '' }],
  });
  const closedNoRows = payloadOf(filled({ ...empty, sourceDataAmendment: { closed: 'true', rules: [] } }));
  deepEqual(closedNoRows.acceptanceRulePackage!.sourceDataAmendment, { closed: true, rules: [] });
  const openNoRows = payloadOf(filled({ ...empty, sourceDataAmendment: { closed: 'false', rules: [] } }));
  deepEqual(openNoRows.acceptanceRulePackage!.sourceDataAmendment, { closed: false, rules: [] });
});

test('空草稿没有任何一格带默认值：正文一行空规则等人填，六节声明都留空', () => {
  const draft = emptyAcceptanceRulePackageDraft();
  for (const name of ['objectId', 'version', 'scope', 'effectiveStartsAt', 'effectiveEndsAt'] as const) {
    equal(draft[name], '', `${name} 不该有默认值`);
  }
  for (const name of ['serviceProduct', 'contract', 'legalEntity', 'scope', 'effectiveStartsAt', 'effectiveEndsAt'] as const) {
    equal(draft.body[name], '', `body.${name} 不该有默认值`);
  }
  deepEqual(draft.body.rules, [emptyAssembledRuleDraft()]);
  deepEqual(emptyAssembledRuleDraft(), { category: '', reference: '' });
  deepEqual(draft.asOfPolicies, []);
  deepEqual(draft.acceptanceContent, { applicableGroups: [], manualReview: '' });
  deepEqual(draft.intakeQualification, { sources: [], qualifications: [] });
  deepEqual(draft.finalRules, []);
  deepEqual(draft.finalRuleValidity, { anchor: '', duration: '' });
  deepEqual(draft.sourceDataAmendment, { closed: '', rules: [] });
});

test('改任何一格载荷键就变（预览作废由流程组件按键判）；同一份草稿两次同键', () => {
  const base = payloadKey(payloadOf(filled()));
  equal(payloadKey(payloadOf(filled())), base);
  notEqual(payloadKey(payloadOf(filled({ finalRuleValidity: { anchor: 'CHANNEL_RESULT_OBSERVED', duration: 'P7D' } }))), base);
  notEqual(payloadKey(payloadOf(filled({ sourceDataAmendment: { ...filled().sourceDataAmendment, closed: 'true' } }))), base);
  notEqual(payloadKey(payloadOf(filled({ asOfPolicies: [] }))), base);
});

test('表单认领的路径覆盖壳、正文与每一节：行按行数长、组按选中数长，不多认领不存在的行', () => {
  const paths = acceptanceRulePackageFieldPaths(filled());
  for (const expected of [
    'objectId',
    'version',
    'scope',
    'effectiveStartsAt',
    'effectiveEndsAt',
    'acceptanceRulePackage.rulePackageBody',
    'acceptanceRulePackage.rulePackageBody.serviceProduct',
    'acceptanceRulePackage.rulePackageBody.contract',
    'acceptanceRulePackage.rulePackageBody.legalEntity',
    'acceptanceRulePackage.rulePackageBody.scope',
    'acceptanceRulePackage.rulePackageBody.effectiveStartsAt',
    'acceptanceRulePackage.rulePackageBody.effectiveEndsAt',
    'acceptanceRulePackage.rulePackageBody.rules[0]',
    'acceptanceRulePackage.rulePackageBody.rules[0].category',
    'acceptanceRulePackage.rulePackageBody.rules[1].reference',
    'acceptanceRulePackage.asOfPolicies[0]',
    'acceptanceRulePackage.asOfPolicies[0].judgment',
    'acceptanceRulePackage.asOfPolicies[0].semantics',
    'acceptanceRulePackage.asOfPolicies[0].policyVersion',
    'acceptanceRulePackage.acceptanceContent',
    'acceptanceRulePackage.acceptanceContent.applicableGroups[0]',
    'acceptanceRulePackage.acceptanceContent.applicableGroups[1]',
    'acceptanceRulePackage.acceptanceContent.manualReview',
    'acceptanceRulePackage.intakeQualification',
    'acceptanceRulePackage.intakeQualification.sources[1]',
    'acceptanceRulePackage.intakeQualification.qualifications[0]',
    'acceptanceRulePackage.finalRules[0]',
    'acceptanceRulePackage.finalRules[0].outcome',
    'acceptanceRulePackage.finalRules[0].finalKind',
    'acceptanceRulePackage.finalRuleValidity',
    'acceptanceRulePackage.finalRuleValidity.anchor',
    'acceptanceRulePackage.finalRuleValidity.duration',
    'acceptanceRulePackage.sourceDataAmendment',
    'acceptanceRulePackage.sourceDataAmendment.closed',
    'acceptanceRulePackage.sourceDataAmendment.rules[0]',
    'acceptanceRulePackage.sourceDataAmendment.rules[0].dataGroup',
    'acceptanceRulePackage.sourceDataAmendment.rules[0].stage',
    'acceptanceRulePackage.sourceDataAmendment.rules[0].intent',
    'acceptanceRulePackage.sourceDataAmendment.rules[0].allowance',
  ]) {
    ok(paths.includes(expected), `缺 ${expected}`);
  }
  ok(!paths.includes('acceptanceRulePackage.rulePackageBody.rules[2]'), '多认领了不存在的规则行');
  ok(!paths.includes('acceptanceRulePackage.asOfPolicies[1]'), '多认领了不存在的时点锚行');
  ok(!paths.includes('acceptanceRulePackage.acceptanceContent.applicableGroups[2]'), '多认领了没选的组');
  ok(!paths.includes('kind'), 'kind 是固定的，不是表单的一格');
});

test('词表判读：未读到是 loading，403 是可辨的未配置，别的失败是读不到；读到了按集名成表', () => {
  deepEqual(vocabularyStateOf(null), { kind: 'loading' });
  deepEqual(vocabularyStateOf({ kind: 'unconfigured' }), { kind: 'unconfigured' });
  deepEqual(vocabularyStateOf({ kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' }), { kind: 'unavailable' });
  deepEqual(vocabularyStateOf({ kind: 'transport', message: 'down' }), { kind: 'unavailable' });
  deepEqual(
    vocabularyStateOf({
      kind: 'outcome',
      status: 200,
      body: {
        outcome: 'PUBLICATION_VOCABULARY_LISTED',
        kind: 'ACCEPTANCE_RULE_PACKAGE',
        sets: [
          { name: 'stage', codes: ['ACCEPTED_NOT_YET_RECEIVED', 'CUSTOMS_SUBMITTED'] },
          { name: 'allowance', codes: ['ALLOWED', 'DISALLOWED'] },
        ],
      },
    }),
    { kind: 'listed', sets: { stage: ['ACCEPTED_NOT_YET_RECEIVED', 'CUSTOMS_SUBMITTED'], allowance: ['ALLOWED', 'DISALLOWED'] } },
  );
});

test('某一集的下拉选项 = 服务端码 × 本页中文，顺序照服务端，集外原样；答复里没这一集就是读不到；三态都没有内置码', () => {
  const listed = vocabularyStateOf({
    kind: 'outcome',
    status: 200,
    body: {
      outcome: 'PUBLICATION_VOCABULARY_LISTED',
      kind: 'ACCEPTANCE_RULE_PACKAGE',
      sets: [
        { name: 'allowance', codes: ['ALLOWED', 'DISALLOWED', 'MAYBE'] },
        { name: 'manualReview', codes: ['NOT_REQUIRED', 'REQUIRED'] },
        { name: 'sources', codes: ['NODE_INTAKE', 'OFFSITE_PICKUP'] },
      ],
    },
  });
  deepEqual(setOptionsOf(listed, 'allowance'), {
    kind: 'options',
    options: [
      { value: 'ALLOWED', label: '允许' },
      { value: 'DISALLOWED', label: '不允许' },
      { value: 'MAYBE', label: 'MAYBE' },
    ],
  });
  deepEqual(setOptionsOf(listed, 'manualReview'), {
    kind: 'options',
    options: [
      { value: 'NOT_REQUIRED', label: '不要求人工复核' },
      { value: 'REQUIRED', label: '要求人工复核' },
    ],
  });
  // 收寄来源沿用读面既有的中文（presentation.ts），同一个词不因换到写签而换名。
  deepEqual(setOptionsOf(listed, 'sources'), {
    kind: 'options',
    options: [
      { value: 'NODE_INTAKE', label: '节点收寄' },
      { value: 'OFFSITE_PICKUP', label: '场外揽收' },
    ],
  });
  deepEqual(setOptionsOf(listed, 'stage'), { kind: 'unavailable' });
  deepEqual(setOptionsOf({ kind: 'loading' }, 'stage'), { kind: 'loading' });
  deepEqual(setOptionsOf({ kind: 'unconfigured' }, 'stage'), { kind: 'unconfigured' });
  deepEqual(setOptionsOf({ kind: 'unavailable' }, 'stage'), { kind: 'unavailable' });
});
