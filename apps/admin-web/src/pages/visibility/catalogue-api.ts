// VE 六类规则与策略目录的读面（GET /visibility-catalogues?kind=，票
// admin-web-page-wiring-frontier/02）。与本目录 api.ts 分文件：那边是运营追踪投影
// 查阅（ADR-0076，派生态，自带前缀注入），这边是主数据目录查阅（登记态），传输与
// 五格判读收敛在共享 catalogue-api，本文件只保留本上下文的类型与查询函数——与
// party/api.ts、customs/api.ts 同款分工。
//
// 形状以 internal/visibilityexception/adapters/http/query_visibility_catalogues.go
// 为准，此处只做镜像不虚构：六种册子的行形状互不相同，kind 由服务端随响应回显，
// 调用方按 kind 择形状。

import { exchangeMasterData, type ApiResult } from '../catalogue-api';

export type { ApiResult } from '../catalogue-api';

/** 目录种类封闭集，与传输层 ?kind= 分派同词（种类命名册子，与登记写口同词根）。 */
export type VisibilityCatalogueKind =
  | 'MILESTONE_MAPPING'
  | 'TRIAGE_RULE'
  | 'NOTIFICATION_POLICY'
  | 'CLAIM_ELIGIBILITY'
  | 'CLAIM_AUTHORIZATION'
  | 'DISCLOSURE_POLICY';

export interface MilestoneMappingEntryRecord {
  /** 源上下文，传输层封闭五元；词表在 presentation.ts 的 sourceContextLabels。 */
  source: string;
  /** 源上下文拥有的事实类型，开放词表，原词转写。 */
  factKind: string;
  /** 标准里程碑，版本化登记的实例参数，原词直示不译。 */
  milestone: string;
}

/** 一版里程碑映射连同整版条目。effectiveTo 缺席即当前版（未闭区间）。 */
export interface MilestoneMappingCatalogueRecord {
  version: string;
  effectiveFrom: string;
  effectiveTo?: string;
  approvedBy: string;
  entries: MilestoneMappingEntryRecord[];
}

export interface TriageRuleEntryRecord {
  /** 异常信号类型，开放引用集，原词转写。 */
  signalKind: string;
  /** 可信度判断依据，开放引用（domain ConfidenceReference），原词转写。 */
  confidence: string;
  /** 分诊结果，domain TriageOutcome 封闭四格；词表在 presentation.ts。 */
  outcome: string;
}

export interface TriageRuleCatalogueRecord {
  version: string;
  effectiveFrom: string;
  effectiveTo?: string;
  approvedBy: string;
  entries: TriageRuleEntryRecord[];
}

/**
 * 一条通知策略。没有版本与区间字段：这份目录的版本化由披露策略引用本身承担（0010），
 * 换版即换引用、新旧两行并存。deadlineAfter 是库存 interval 的文本转写——它是相对量，
 * 绝对截止点在目录上根本不存在（由披露决定时间加出来）。
 */
export interface NotificationPolicyRecord {
  policy: string;
  channel: string;
  deadlineAfter: string;
  obligation: string;
  approvedBy: string;
}

export interface ClaimEligibilityRecord {
  contract: string;
  version: string;
  approvedBy: string;
  /** 覆盖的索赔种类引用，登记入口要求至少一项（application checkEntries）。 */
  coveredKinds: string[];
}

/**
 * applicants 允许为空数组：目录行在场而名单为空是「此账户当前不授权任何人代提」的
 * 显式决定（0018），与「还没登记」（整行不在 catalogues 里）不是一回事——页面文案
 * 必须分开说，别把已作出的授权决定读丢。
 */
export interface ClaimAuthorizationRecord {
  customer: string;
  version: string;
  approvedBy: string;
  applicants: string[];
}

/** 一维一格：content 只在 SHOWN 时在场（0012 的 shape 约束）。 */
export interface DisclosureCellRecord {
  state: string;
  content?: string;
}

export interface DisclosurePolicyEntryRecord {
  customer: string;
  milestones: DisclosureCellRecord;
  eta: DisclosureCellRecord;
  final: DisclosureCellRecord;
  note: DisclosureCellRecord;
}

export interface DisclosurePolicyCatalogueRecord {
  version: string;
  effectiveFrom: string;
  effectiveTo?: string;
  approvedBy: string;
  entries: DisclosurePolicyEntryRecord[];
}

// 响应体按 kind 判别：唯一业务成格 VISIBILITY_CATALOGUES_LISTED，空册也是这一格
// （ADR-0077 Decision 四，空册本身就是内容，不折成未配置）。
export type VisibilityCatalogueListResponseBody =
  | { outcome: 'VISIBILITY_CATALOGUES_LISTED'; kind: 'MILESTONE_MAPPING'; catalogues: MilestoneMappingCatalogueRecord[] }
  | { outcome: 'VISIBILITY_CATALOGUES_LISTED'; kind: 'TRIAGE_RULE'; catalogues: TriageRuleCatalogueRecord[] }
  | { outcome: 'VISIBILITY_CATALOGUES_LISTED'; kind: 'NOTIFICATION_POLICY'; catalogues: NotificationPolicyRecord[] }
  | { outcome: 'VISIBILITY_CATALOGUES_LISTED'; kind: 'CLAIM_ELIGIBILITY'; catalogues: ClaimEligibilityRecord[] }
  | { outcome: 'VISIBILITY_CATALOGUES_LISTED'; kind: 'CLAIM_AUTHORIZATION'; catalogues: ClaimAuthorizationRecord[] }
  | { outcome: 'VISIBILITY_CATALOGUES_LISTED'; kind: 'DISCLOSURE_POLICY'; catalogues: DisclosurePolicyCatalogueRecord[] };

/**
 * 返回类型按请求的 kind 收窄：服务端回显的 kind 与请求同值（传输层封闭集校验后
 * 分派），三张页面各管两册，收窄后页面不必为四种不会到场的册形写空分支。
 */
export function listVisibilityCatalogues<Kind extends VisibilityCatalogueKind>(
  kind: Kind,
): Promise<ApiResult<Extract<VisibilityCatalogueListResponseBody, { kind: Kind }>>> {
  return exchangeMasterData<Extract<VisibilityCatalogueListResponseBody, { kind: Kind }>>(
    `/visibility-catalogues?kind=${encodeURIComponent(kind)}`,
  );
}
