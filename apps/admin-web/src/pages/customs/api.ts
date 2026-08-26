// 本目录 fetch 出口:关务两条目录查阅端点——合规规则库(GET /customs-compliance-rules,
// ADR-0077、票 master-data-wiring/04)与案件配置册(GET /customs-case-registers,票
// admin-web-page-wiring-frontier/05)。册子都按 ?registry= 分派,各端点各自封闭集。
// 传输与五格判读收敛在共享 catalogue-api,本文件只保留本上下文的类型与查询函数。

import { exchangeMasterData, type ApiResult } from '../catalogue-api';

export type { ApiResult } from '../catalogue-api';

export type ComplianceRegistry = 'case-requirement' | 'interpretation';

export interface CaseRequirementRuleRecord {
  jurisdiction: string;
  direction: string;
  procedure: string;
  required: boolean;
  basis: string;
}

export interface InterpretationRuleRecord {
  layer: string;
  jurisdiction: string;
  rule: string;
  appliesFrom: string;
  appliesUntil?: string;
}

export interface CaseRequirementListResponseBody {
  outcome: 'CASE_REQUIREMENT_RULES_LISTED';
  rules: CaseRequirementRuleRecord[];
}

export interface InterpretationListResponseBody {
  outcome: 'INTERPRETATION_RULES_LISTED';
  rules: InterpretationRuleRecord[];
}

export type ComplianceRulesListResponseBody =
  | CaseRequirementListResponseBody
  | InterpretationListResponseBody;

export function listComplianceRules(
  registry: ComplianceRegistry,
): Promise<ApiResult<ComplianceRulesListResponseBody>> {
  return exchangeMasterData<ComplianceRulesListResponseBody>(
    `/customs-compliance-rules?registry=${encodeURIComponent(registry)}`,
  );
}

// —— 案件配置册(customs-cases 页三格查阅面) ——
// 形状以 internal/customscompliance/adapters/http/query_case_registers.go 为准,
// 此处只做镜像不虚构。

/** 案件配置册封闭三格,与传输层 ?registry= 分派同词。门禁两表归 customs-restrictions 页(票 06),不在本集。 */
export type CaseRegisterRegistry = 'readiness' | 'submission-authority' | 'closure-obligation';

/**
 * 一条就绪判断。撤销两列同现同缺:缺席即仍有效,在场即`不再就绪`且原判断(依据与
 * 形成时间)原样保留——失效不是删除,页面判读看这对字段,不造第二个布尔。
 */
export interface ReadinessJudgmentRecord {
  unit: string;
  basis: string;
  judgedAt: string;
  revokedBy?: string;
  revokedAt?: string;
}

/** 一条提交授权,与就绪同形的另一条轨(CONTEXT 244:分别形成和失效)。 */
export interface SubmissionAuthorityRecord {
  unit: string;
  authority: string;
  grantedAt: string;
  revokedBy?: string;
  revokedAt?: string;
}

export interface ClosureObligationItemRecord {
  obligation: string;
  scope: string;
  /** 封闭三值 CONCLUDED / HANDED_OVER / UNRESOLVED;词表在 presentation.ts。 */
  state: string;
  basis: string;
  /** 只在承接项在场(承接必须指名接收责任方,CONTEXT 硬句 219)。 */
  handedTo?: string;
  appliesFrom: string;
  /** 缺席即尚无终点,不是已失效。 */
  appliesUntil?: string;
}

/**
 * 一份关闭义务目录连同全部已登记义务项。items 空数组是「目录已登记、当前无义务项」
 * 的显式一格;「目录未登记 → 未决」表现为整份目录不在 catalogues 里——页面文案必须
 * 分开说(0008 自注:这两格含义相反)。
 */
export interface ClosureObligationCatalogueRecord {
  case: string;
  registeredAt: string;
  items: ClosureObligationItemRecord[];
}

export interface ReadinessJudgmentListResponseBody {
  outcome: 'READINESS_JUDGMENTS_LISTED';
  judgments: ReadinessJudgmentRecord[];
}

export interface SubmissionAuthorityListResponseBody {
  outcome: 'SUBMISSION_AUTHORITIES_LISTED';
  authorities: SubmissionAuthorityRecord[];
}

export interface ClosureObligationListResponseBody {
  outcome: 'CLOSURE_OBLIGATIONS_LISTED';
  catalogues: ClosureObligationCatalogueRecord[];
}

export type CaseRegisterListResponseBody =
  | ReadinessJudgmentListResponseBody
  | SubmissionAuthorityListResponseBody
  | ClosureObligationListResponseBody;

// registry → 结果格的对照:响应体不回显 registry,判别靠 outcome,这张表把请求参数
// 与可能到场的那一格钉在一起,调用侧按 registry 收窄后拿到的就是单格类型,不必写
// 空分支。
interface CaseRegisterBodyByRegistry {
  readiness: ReadinessJudgmentListResponseBody;
  'submission-authority': SubmissionAuthorityListResponseBody;
  'closure-obligation': ClosureObligationListResponseBody;
}

export function listCaseRegisters<Registry extends CaseRegisterRegistry>(
  registry: Registry,
): Promise<ApiResult<CaseRegisterBodyByRegistry[Registry]>> {
  return exchangeMasterData<CaseRegisterBodyByRegistry[Registry]>(
    `/customs-case-registers?registry=${encodeURIComponent(registry)}`,
  );
}
