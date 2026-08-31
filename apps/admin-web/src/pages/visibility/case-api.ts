// VE 案件侧三页的读面（GET /exception-triage-records、GET /exception-case-records、
// GET /claims-recovery-records，票 admin-skeleton-closure-batch/06）。与本目录
// catalogue-api.ts 分文件：那边是目录侧六册（已接线，本文件一个符号都不碰），这边
// 是案件侧登记册查阅；传输与五格判读收敛在共享 catalogue-api。
//
// 形状以 internal/visibilityexception/adapters/http/query_exception_triage_records.go、
// query_exception_case_records.go、query_claims_recovery_records.go 为准，此处只做
// 镜像不虚构；registry 是传输形状由调用方给定，响应外壳的 outcome 词已把册子分开。

import { exchangeMasterData, type ApiResult } from '../catalogue-api';

export type { ApiResult } from '../catalogue-api';

// ---- 异常分诊与处置协调（exception-triage 页） ----

export type ExceptionTriageRegistry = 'signal-episode' | 'disposition-request';

/** 分诊结论三件成对在场（0002 与发作期同笔提交）；outcome 词表复用 presentation.ts。 */
export interface TriageConclusionRecord {
  outcome: string;
  rule: string;
  triagedAt: string;
}

/**
 * 一段信号发作期连同它的分诊结论。ended 两键成对缺席即仍活跃；triage 缺席即尚未
 * 分诊——缺席即答案，页面不代判。信号类型与可信度是开放引用，原词直示。
 */
export interface SignalEpisodeRecord {
  episodeId: string;
  parcel: string;
  kind: string;
  rule: string;
  confidence: string;
  hits: number;
  startedAt: string;
  lastHitAt: string;
  releaseBasis?: string;
  endedAt?: string;
  priorEpisode?: string;
  triage?: TriageConclusionRecord;
}

/**
 * 一份处置请求。judgment 两键成对缺席即源上下文尚无答复（封闭四走向，词表在
 * case-presentation.ts）；cancellation 只在已判断后可能在场；supersededBy 在场即
 * 这行已被替代——替代不是删除，原请求照列。实际执行结果没有键：那是目标上下文
 * 按事实返回的东西。
 */
export interface DispositionRequestRecord {
  requestId: string;
  caseId: string;
  targetContext: string;
  action: string;
  scope: string;
  reason: string;
  evidence: string;
  intentVersion: number;
  sentAt: string;
  acceptanceWindow?: string;
  judgment?: string;
  judgedAt?: string;
  cancellation?: string;
  supersededBy?: string;
}

export type ExceptionTriageListResponseBody =
  | { outcome: 'SIGNAL_EPISODES_LISTED'; episodes: SignalEpisodeRecord[] }
  | { outcome: 'DISPOSITION_REQUESTS_LISTED'; requests: DispositionRequestRecord[] };

interface TriageBodyByRegistry {
  'signal-episode': Extract<
    ExceptionTriageListResponseBody,
    { outcome: 'SIGNAL_EPISODES_LISTED' }
  >;
  'disposition-request': Extract<
    ExceptionTriageListResponseBody,
    { outcome: 'DISPOSITION_REQUESTS_LISTED' }
  >;
}

/** 返回类型按请求的 registry 收窄：两册行形状不同，页面按视图择形状。 */
export function listExceptionTriageRecords<Registry extends ExceptionTriageRegistry>(
  registry: Registry,
): Promise<ApiResult<TriageBodyByRegistry[Registry]>> {
  return exchangeMasterData<TriageBodyByRegistry[Registry]>(
    `/exception-triage-records?registry=${encodeURIComponent(registry)}`,
  );
}

// ---- 异常案件（exception-cases 页） ----

/**
 * 一件异常案件。phase 三相封闭词（词表在 case-presentation.ts）；conclusion 只随
 * 已关闭在场；mergedInto 在场即受控归并（原编号不删除）。页面模板的严重度、优先级、
 * 当前工作条件与响应周期在存储上没有登记格——键结构上不存在，不是空值（票 06
 * Comments）。
 */
export interface ExceptionCaseRecord {
  caseId: string;
  rootParcel: string;
  impactScope: string;
  responsibleTeam: string;
  phase: string;
  establishedAt: string;
  firstResponse?: string;
  closedAt?: string;
  conclusion?: string;
  mergedInto?: string;
}

export interface ExceptionCaseListResponseBody {
  outcome: 'EXCEPTION_CASES_LISTED';
  cases: ExceptionCaseRecord[];
}

/** 单册端点无 registry 参数：页面只有一张案件列表。 */
export function listExceptionCaseRecords(): Promise<ApiResult<ExceptionCaseListResponseBody>> {
  return exchangeMasterData<ExceptionCaseListResponseBody>('/exception-case-records');
}

// ---- 索赔与追偿（claims-recovery 页三页签） ----

export type ClaimsRecoveryRegistry =
  | 'customer-notification'
  | 'claim-item'
  | 'recovery-matter';

/** 通知过程的一个节点：封闭六值，分别记录不覆盖。 */
export interface NotificationMilestoneRecord {
  milestone: string;
  recordedAt: string;
}

/**
 * 一份客户异常通知义务。content 是披露内容快照的引用不是正文；milestones 整列照
 * 登记转写，页面不把它们折成一个「已通知」——哪个结果满足通知义务由客户合同与
 * 通知策略判断。
 */
export interface CustomerNotificationRecord {
  notificationId: string;
  customer: string;
  episode: string;
  decidedAt: string;
  policy: string;
  content: string;
  deadline: string;
  channel: string;
  obligation: string;
  milestones: NotificationMilestoneRecord[];
}

/**
 * 一件客户索赔项。三判分步：screen 两键成对在场即资格初筛已判（封闭三值）；
 * conclusion 四值封闭随行带结论时刻与复核截止；priorConclusion 在场即复核换过版。
 * supplement 四键只随等待补充在场；deadlineVersions 计期限版本数。首次索赔期限
 * 与金额在行上没有登记格（票 06 Comments；金额归 settlement-accounting）。
 */
export interface ClaimItemRecord {
  batch: string;
  itemId: string;
  customer: string;
  applicant?: string;
  contract: string;
  target: string;
  kind: string;
  submittedAt: string;
  revision: number;
  screen?: string;
  screenBasis?: string;
  missingMaterials?: string;
  supplementScope?: string;
  supplementNotice?: string;
  supplementDeadline?: string;
  deadlineVersions: number;
  conclusion?: string;
  concludedAt?: string;
  reviewBy?: string;
  priorConclusion?: string;
  withdrawn: boolean;
  withdrawnAt?: string;
}

/** 某一动作种类的最近过程节点：milestone 封闭七值，attempt 是当前尝试序。 */
export interface RecoveryActionRecord {
  milestone: string;
  attempt: number;
  occurredAt: string;
}

/**
 * 一件追偿事项。预先通知与正式主张各取最近节点、成对缺席即该种类尚无动作，两类
 * 不折并成「已追偿」。对方响应与外部责任结论在存储上没有登记格——页面不把
 * 「无响应」代判成拒绝（票 06 Comments）。
 */
export interface RecoveryMatterRecord {
  matterId: string;
  caseId: string;
  counterparty: string;
  scope: string;
  basis: string;
  legalEntity: string;
  evidence: string;
  deadline: string;
  openedAt: string;
  preliminaryNotice?: RecoveryActionRecord;
  formalAssertion?: RecoveryActionRecord;
}

export type ClaimsRecoveryListResponseBody =
  | { outcome: 'CUSTOMER_NOTIFICATIONS_LISTED'; notifications: CustomerNotificationRecord[] }
  | { outcome: 'CLAIM_ITEMS_LISTED'; items: ClaimItemRecord[] }
  | { outcome: 'RECOVERY_MATTERS_LISTED'; matters: RecoveryMatterRecord[] };

interface ClaimsBodyByRegistry {
  'customer-notification': Extract<
    ClaimsRecoveryListResponseBody,
    { outcome: 'CUSTOMER_NOTIFICATIONS_LISTED' }
  >;
  'claim-item': Extract<ClaimsRecoveryListResponseBody, { outcome: 'CLAIM_ITEMS_LISTED' }>;
  'recovery-matter': Extract<
    ClaimsRecoveryListResponseBody,
    { outcome: 'RECOVERY_MATTERS_LISTED' }
  >;
}

/** 返回类型按请求的 registry 收窄，同 listExceptionTriageRecords。 */
export function listClaimsRecoveryRecords<Registry extends ClaimsRecoveryRegistry>(
  registry: Registry,
): Promise<ApiResult<ClaimsBodyByRegistry[Registry]>> {
  return exchangeMasterData<ClaimsBodyByRegistry[Registry]>(
    `/claims-recovery-records?registry=${encodeURIComponent(registry)}`,
  );
}
