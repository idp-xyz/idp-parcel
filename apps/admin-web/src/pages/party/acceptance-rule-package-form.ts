// 接单规则包发布表单的纯逻辑（票 admin-write-faces/12）：草稿形状、草稿 → 载荷、「某一节留空了没」、表单认领的
// JSON 路径、各封闭集下拉的选项来源判读。全部是纯函数，node:test 钉着；组件 AcceptanceRulePackagePublicationForm.tsx
// 只负责摆。写法照 customer-contract-form.ts 与 authorization-rule-form.ts。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 07 硬句）。一版规则包是一次提交：正文与全部声明在同一份载荷、
// 同一个摘要下（票 12「声明随发布怎么表达」）。**某一节整节留空 = 该通道未声明**：载荷里不带那一节，消费方照旧译
// `未配置`；节里只要有一格有字或有一行，整节照原样上送——没选的码、空的行、同一判断两条时点锚、空组、只有有效期
// 没有终局行、未封闭零格，都由服务端答（逐格问题或预览上的`未受理`带成因），表单不代判也不静默补齐、不给默认不预选。
// 各节封闭集的码从服务端词表读口取（票 20），这里没有任何一份内置的码；下面几张表只是码 → 本页中文，缺词原样示出。

import type { ApiResult } from '../catalogue-api';
import { finalOutcomeLabels, intakeSourceLabels } from './presentation';
import {
  vocabularyOptions,
  type AcceptanceRulePackageBodyPayload,
  type CommercialPublicationPayload,
  type PublicationVocabularyResponseBody,
  type SourceDataAmendmentPayload,
  type VocabularyOption,
} from './publication-draft-api';

// ——草稿形状。每格都是操作者手里的字：封闭集格存原词（`''` = 还没选），行是几格一组，组是选中的原词列表。

export interface AssembledRuleDraft {
  category: string;
  reference: string;
}

export interface RulePackageBodyDraft {
  serviceProduct: string;
  contract: string;
  legalEntity: string;
  scope: string;
  effectiveStartsAt: string;
  effectiveEndsAt: string;
  rules: AssembledRuleDraft[];
}

export interface AsOfPolicyDraft {
  judgment: string;
  semantics: string;
  policyVersion: string;
}

export interface AcceptanceContentDraft {
  applicableGroups: string[];
  manualReview: string;
}

export interface IntakeQualificationDraft {
  sources: string[];
  qualifications: string[];
}

export interface FinalRuleDraft {
  outcome: string;
  finalKind: string;
}

export interface FinalRuleValidityDraft {
  anchor: string;
  duration: string;
}

/** `closed` 的三态：`''` = 还没选。它不是布尔——「没选」必须与 false 分得开，false 是登记方说出的一句话。 */
export type ClosedDraft = '' | 'true' | 'false';

export interface AmendmentRuleDraft {
  dataGroup: string;
  stage: string;
  intent: string;
  allowance: string;
}

export interface SourceDataAmendmentDraft {
  closed: ClosedDraft;
  rules: AmendmentRuleDraft[];
}

export interface AcceptanceRulePackageDraft {
  objectId: string;
  version: string;
  scope: string;
  effectiveStartsAt: string;
  effectiveEndsAt: string;
  body: RulePackageBodyDraft;
  asOfPolicies: AsOfPolicyDraft[];
  acceptanceContent: AcceptanceContentDraft;
  intakeQualification: IntakeQualificationDraft;
  finalRules: FinalRuleDraft[];
  finalRuleValidity: FinalRuleValidityDraft;
  sourceDataAmendment: SourceDataAmendmentDraft;
}

export function emptyAssembledRuleDraft(): AssembledRuleDraft {
  return { category: '', reference: '' };
}

export function emptyAsOfPolicyDraft(): AsOfPolicyDraft {
  return { judgment: '', semantics: '', policyVersion: '' };
}

export function emptyFinalRuleDraft(): FinalRuleDraft {
  return { outcome: '', finalKind: '' };
}

export function emptyAmendmentRuleDraft(): AmendmentRuleDraft {
  return { dataGroup: '', stage: '', intent: '', allowance: '' };
}

/**
 * 空草稿。正文一节永远在场，所以它带一行空规则等人填（零行由领域拒，正文表达不了「无条件接受」）；六节声明
 * 一格不填、一行不带——那正是「留空即未声明」的起点，带一行空行会让一节在操作者没碰它之前就被送上去。
 */
export function emptyAcceptanceRulePackageDraft(): AcceptanceRulePackageDraft {
  return {
    objectId: '',
    version: '',
    scope: '',
    effectiveStartsAt: '',
    effectiveEndsAt: '',
    body: {
      serviceProduct: '',
      contract: '',
      legalEntity: '',
      scope: '',
      effectiveStartsAt: '',
      effectiveEndsAt: '',
      rules: [emptyAssembledRuleDraft()],
    },
    asOfPolicies: [],
    acceptanceContent: { applicableGroups: [], manualReview: '' },
    intakeQualification: { sources: [], qualifications: [] },
    finalRules: [],
    finalRuleValidity: { anchor: '', duration: '' },
    sourceDataAmendment: { closed: '', rules: [] },
  };
}

// ——六节声明各自「留空了没」。判据只有一条：节里有没有任何一格有字或任何一行——不看填得对不对，那是服务端的事。

export const declarationSections = [
  'asOfPolicies',
  'acceptanceContent',
  'intakeQualification',
  'finalRules',
  'finalRuleValidity',
  'sourceDataAmendment',
] as const;

export type DeclarationSection = (typeof declarationSections)[number];

export function sectionDeclared(draft: AcceptanceRulePackageDraft, section: DeclarationSection): boolean {
  switch (section) {
    case 'asOfPolicies':
      return draft.asOfPolicies.length > 0;
    case 'acceptanceContent':
      return draft.acceptanceContent.applicableGroups.length > 0 || draft.acceptanceContent.manualReview !== '';
    case 'intakeQualification':
      return draft.intakeQualification.sources.length > 0 || draft.intakeQualification.qualifications.length > 0;
    case 'finalRules':
      return draft.finalRules.length > 0;
    case 'finalRuleValidity':
      return draft.finalRuleValidity.anchor !== '' || draft.finalRuleValidity.duration !== '';
    case 'sourceDataAmendment':
      return draft.sourceDataAmendment.closed !== '' || draft.sourceDataAmendment.rules.length > 0;
  }
}

/**
 * 草稿 → 载荷。可缺的壳格与正文区间上界留空即不进载荷（Go 侧 omitempty 同义）；正文一节永远在场、行照原样；六节
 * 声明按 sectionDeclared 决定在不在场，在场的节整节原样送——空数组也送，「一行都没有」由服务端答。`closed` 没选就
 * 不送那一格。载荷里没有身份也没有摘要，壳上也没有指名引用（本册的引用都在正文五维里）。
 */
export function payloadOf(draft: AcceptanceRulePackageDraft): CommercialPublicationPayload {
  const body: AcceptanceRulePackageBodyPayload = {
    rulePackageBody: {
      serviceProduct: draft.body.serviceProduct,
      contract: draft.body.contract,
      legalEntity: draft.body.legalEntity,
      scope: draft.body.scope,
      effectiveStartsAt: draft.body.effectiveStartsAt,
      rules: draft.body.rules.map((rule) => ({ category: rule.category, reference: rule.reference })),
    },
  };
  if (draft.body.effectiveEndsAt !== '') body.rulePackageBody.effectiveEndsAt = draft.body.effectiveEndsAt;
  if (sectionDeclared(draft, 'asOfPolicies')) {
    body.asOfPolicies = draft.asOfPolicies.map((row) => ({
      judgment: row.judgment,
      semantics: row.semantics,
      policyVersion: row.policyVersion,
    }));
  }
  if (sectionDeclared(draft, 'acceptanceContent')) {
    body.acceptanceContent = {
      applicableGroups: [...draft.acceptanceContent.applicableGroups],
      manualReview: draft.acceptanceContent.manualReview,
    };
  }
  if (sectionDeclared(draft, 'intakeQualification')) {
    body.intakeQualification = {
      sources: [...draft.intakeQualification.sources],
      qualifications: [...draft.intakeQualification.qualifications],
    };
  }
  if (sectionDeclared(draft, 'finalRules')) {
    body.finalRules = draft.finalRules.map((row) => ({ outcome: row.outcome, finalKind: row.finalKind }));
  }
  if (sectionDeclared(draft, 'finalRuleValidity')) {
    body.finalRuleValidity = { anchor: draft.finalRuleValidity.anchor, duration: draft.finalRuleValidity.duration };
  }
  if (sectionDeclared(draft, 'sourceDataAmendment')) {
    const amendment: SourceDataAmendmentPayload = {
      rules: draft.sourceDataAmendment.rules.map((row) => ({
        dataGroup: row.dataGroup,
        stage: row.stage,
        intent: row.intent,
        allowance: row.allowance,
      })),
    };
    if (draft.sourceDataAmendment.closed !== '') amendment.closed = draft.sourceDataAmendment.closed === 'true';
    body.sourceDataAmendment = amendment;
  }

  const payload: CommercialPublicationPayload = {
    kind: 'ACCEPTANCE_RULE_PACKAGE',
    objectId: draft.objectId,
    version: draft.version,
    scope: draft.scope,
    effectiveStartsAt: draft.effectiveStartsAt,
    acceptanceRulePackage: body,
  };
  if (draft.effectiveEndsAt !== '') payload.effectiveEndsAt = draft.effectiveEndsAt;
  return payload;
}

const shellPaths = ['objectId', 'version', 'scope', 'effectiveStartsAt', 'effectiveEndsAt'] as const;
const root = 'acceptanceRulePackage';

/**
 * 表单渲染了哪几条 JSON 路径（流程组件把服务端点名的其余路径单独列出）。行按行数长、组按选中数长：服务端对整节、
 * 整行与行里每一格都可能点名。这里认领的每一条组件都真的渲染了问题——认领而不显示会把服务端的话吞掉。
 */
export function acceptanceRulePackageFieldPaths(draft: AcceptanceRulePackageDraft): string[] {
  const bodyPath = `${root}.rulePackageBody`;
  const paths: string[] = [
    ...shellPaths,
    bodyPath,
    `${bodyPath}.serviceProduct`,
    `${bodyPath}.contract`,
    `${bodyPath}.legalEntity`,
    `${bodyPath}.scope`,
    `${bodyPath}.effectiveStartsAt`,
    `${bodyPath}.effectiveEndsAt`,
  ];
  draft.body.rules.forEach((_, index) => {
    const row = `${bodyPath}.rules[${index}]`;
    paths.push(row, `${row}.category`, `${row}.reference`);
  });
  draft.asOfPolicies.forEach((_, index) => {
    const row = `${root}.asOfPolicies[${index}]`;
    paths.push(row, `${row}.judgment`, `${row}.semantics`, `${row}.policyVersion`);
  });
  paths.push(`${root}.acceptanceContent`, `${root}.acceptanceContent.manualReview`);
  draft.acceptanceContent.applicableGroups.forEach((_, index) => {
    paths.push(`${root}.acceptanceContent.applicableGroups[${index}]`);
  });
  paths.push(`${root}.intakeQualification`);
  draft.intakeQualification.sources.forEach((_, index) => {
    paths.push(`${root}.intakeQualification.sources[${index}]`);
  });
  draft.intakeQualification.qualifications.forEach((_, index) => {
    paths.push(`${root}.intakeQualification.qualifications[${index}]`);
  });
  draft.finalRules.forEach((_, index) => {
    const row = `${root}.finalRules[${index}]`;
    paths.push(row, `${row}.outcome`, `${row}.finalKind`);
  });
  paths.push(`${root}.finalRuleValidity`, `${root}.finalRuleValidity.anchor`, `${root}.finalRuleValidity.duration`);
  paths.push(`${root}.sourceDataAmendment`, `${root}.sourceDataAmendment.closed`);
  draft.sourceDataAmendment.rules.forEach((_, index) => {
    const row = `${root}.sourceDataAmendment.rules[${index}]`;
    paths.push(row, `${row}.dataGroup`, `${row}.stage`, `${row}.intent`, `${row}.allowance`);
  });
  return paths;
}

// ——词表：一口读回本册全部封闭集，各下拉各取自己那一集。码只从这里来；下面的表只给中文。

/** 本册词表里各集的键名（与 Go `PublicationVocabulary` 对 ACCEPTANCE_RULE_PACKAGE 答的集名同词）。 */
export type VocabularySetName =
  | 'category'
  | 'judgment'
  | 'applicableGroups'
  | 'manualReview'
  | 'sources'
  | 'outcome'
  | 'anchor'
  | 'stage'
  | 'intent'
  | 'allowance';

export type VocabularyState =
  | { kind: 'loading' }
  | { kind: 'listed'; sets: Record<string, string[]> }
  | { kind: 'unconfigured' }
  | { kind: 'unavailable' };

/**
 * 词表读口的答复判读：读到了按集名成表；403 是可辨的「未配置」——下拉显占位、不内置任何码顶替；别的失败是读不到，
 * 同样只显占位。
 */
export function vocabularyStateOf(answer: ApiResult<PublicationVocabularyResponseBody> | null): VocabularyState {
  if (answer === null) return { kind: 'loading' };
  if (answer.kind === 'unconfigured') return { kind: 'unconfigured' };
  if (answer.kind !== 'outcome') return { kind: 'unavailable' };
  const sets: Record<string, string[]> = {};
  for (const set of answer.body.sets) sets[set.name] = [...set.codes];
  return { kind: 'listed', sets };
}

export type SetOptionsState =
  | { kind: 'loading' }
  | { kind: 'options'; options: VocabularyOption[] }
  | { kind: 'unconfigured' }
  | { kind: 'unavailable' };

/**
 * 某一集的下拉选项：服务端码 × 本页中文（vocabularyOptions，集外原样）。答复里没有这一集就是读不到——不拿别的集
 * 顶、不拿内置码顶。选项里没有任何一项带「选中」：不给默认、不预选归表单硬句。
 */
export function setOptionsOf(state: VocabularyState, name: VocabularySetName): SetOptionsState {
  if (state.kind !== 'listed') return state;
  const codes = state.sets[name];
  if (!codes) return { kind: 'unavailable' };
  return { kind: 'options', options: vocabularyOptions(codes, vocabularyLabels[name]) };
}

// ——码 → 本页中文。收寄来源与责任结果沿用读面既有的两张表（presentation.ts）：同一个词不因换到写签而换名；其余
// 几集今天只有这张表单在用，中文取 PC CONTEXT / 各 ADR 原词。集外的码由 vocabularyOptions 原样示出，不在这里猜格。

export const ruleCategoryLabels: Record<string, string> = {
  MINIMUM_INGRESS_IDENTITY: '最小入口身份',
  SHIPMENT_INVARIANT: '委托不变量',
  PRODUCT_AND_CONTRACT_DOCUMENT: '产品与合同单证',
  REGULATORY_SOURCE_DOCUMENT: '监管原始资料',
  CROSS_FIELD_CONDITION: '跨字段条件',
};

export const judgmentTypeLabels: Record<string, string> = {
  NETWORK_REACHABILITY: '网络可达性',
  PRE_ACCEPTANCE_FINANCIAL_CONTROL: '接受前财务控制',
};

export const checkGroupLabels: Record<string, string> = {
  CUSTOMER_RELATIONSHIP: '客户关系',
  LEGAL_ENTITY_AND_CONTRACT: '法人与合同',
  PRODUCT_AND_SERVICE: '产品与服务',
  MEMBER_BASELINE: '成员基线',
  REQUIRED_DOCUMENT: '必要单证',
  PRE_ACCEPTANCE_FINANCIAL_CONTROL: '接受前财务控制',
  NETWORK_REACHABILITY: '网络可达性',
};

// 答的是「要不要人工复核」（接单规则正文），不是「谁有权复核」——中文把「人工复核」点全，免得与授权那一问混读。
export const manualReviewLabels: Record<string, string> = {
  REQUIRED: '要求人工复核',
  NOT_REQUIRED: '不要求人工复核',
};

export const validityAnchorLabels: Record<string, string> = {
  CHANNEL_RESULT_OBSERVED: '渠道结果业务时间',
};

// 六格原词的权威在 parcel-shipment（PS CONTEXT「资料修订阶段」），这里只给中文。
export const amendmentStageLabels: Record<string, string> = {
  ACCEPTED_NOT_YET_RECEIVED: '已接受、尚未收件',
  RECEIVED_OR_MEASURED: '已收件或已量方',
  LABELLED_OR_BAGGED: '已贴标或已装袋',
  CUSTOMS_DATA_FORMING_NOT_SUBMITTED: '关务资料形成中、尚未申报',
  CUSTOMS_SUBMITTED: '已申报',
  CASE_CLOSED_OR_SERVICE_COMPLETED: '已结案或服务完成',
};

export const amendmentIntentLabels: Record<string, string> = {
  SUPPLEMENT: '补充',
  CORRECTION: '更正',
  EXPLICIT_CLEAR: '显式清空',
};

// 只有两值：「未声明」是缺格的读法（由 closed 决定），不是一行能选的值（ADR-0120 Decision 三），词表不会答它。
export const amendmentAllowanceLabels: Record<string, string> = {
  ALLOWED: '允许',
  DISALLOWED: '不允许',
};

const vocabularyLabels: Record<VocabularySetName, Record<string, string>> = {
  category: ruleCategoryLabels,
  judgment: judgmentTypeLabels,
  applicableGroups: checkGroupLabels,
  manualReview: manualReviewLabels,
  sources: intakeSourceLabels,
  outcome: finalOutcomeLabels,
  anchor: validityAnchorLabels,
  stage: amendmentStageLabels,
  intent: amendmentIntentLabels,
  allowance: amendmentAllowanceLabels,
};
