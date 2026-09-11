// VE 六类规则与策略目录的读面（GET /visibility-catalogues?kind=，票
// admin-web-page-wiring-frontier/02）。与本目录 api.ts 分文件：那边是运营追踪投影
// 查阅（ADR-0076，派生态，自带前缀注入），这边是主数据目录查阅（登记态），传输与
// 五格判读收敛在共享 catalogue-api，本文件只保留本上下文的类型与查询函数——与
// party/api.ts、customs/api.ts 同款分工。
//
// 形状以 internal/visibilityexception/adapters/http/query_visibility_catalogues.go
// 为准，此处只做镜像不虚构：六种册子的行形状互不相同，kind 由服务端随响应回显，
// 调用方按 kind 择形状。

import { exchangeMasterData, postMasterData, type ApiResult } from '../catalogue-api';
import type { RegistrationResponseBody } from '../../components/registration';

export type { ApiResult } from '../catalogue-api';

/**
 * 目录种类封闭集，与传输层 ?kind= 分派同词（种类命名册子，与登记写口同词根）。
 * 异常披露规则（0023）与冲突信号规则（0025）两册随票 ve-disclosure-policy-view/03 加入：
 * 前者与披露策略（0012）是相邻的两本册，词里的「规则」与「策略」就是分册的记号。
 */
export type VisibilityCatalogueKind =
  | 'MILESTONE_MAPPING'
  | 'TRIAGE_RULE'
  | 'NOTIFICATION_POLICY'
  | 'CLAIM_ELIGIBILITY'
  | 'CLAIM_AUTHORIZATION'
  | 'DISCLOSURE_POLICY'
  | 'EXCEPTION_DISCLOSURE_RULE'
  | 'CONFLICT_SIGNAL_RULE';

/**
 * 今天有在线登记写面的那六册（ADR-0085，票 admin-write-faces/02 切片 02d）。异常披露规则与
 * 冲突信号规则两册只有读签：写签跟着读签走（伞票纪律），归票 ve-disclosure-policy-view/02
 * 步二在 03 进 main 之后铺——本文件不替它发明登记端点。
 */
export type VisibilityRegistrableCatalogueKind = Exclude<
  VisibilityCatalogueKind,
  'EXCEPTION_DISCLOSURE_RULE' | 'CONFLICT_SIGNAL_RULE'
>;

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

/**
 * 一条异常披露规则条目（0023）：对某客户账户的某类信号在某可信度依据下，披露条件成不成立、
 * 批准范围允不允许自动发布、内容从哪来。content 只在 disclosable 时在场（0023 成对约束）；
 * autoRelease 不会在 disclosable 为假时为真。三格照登转写，页面不做第二道 shape 校验。
 */
export interface ExceptionDisclosureRuleEntryRecord {
  customer: string;
  /** 异常信号类型，开放引用集，原词转写。 */
  signalKind: string;
  /** 可信度判断依据，开放引用，原词转写。 */
  confidence: string;
  disclosable: boolean;
  autoRelease: boolean;
  content?: string;
}

/**
 * 一版异常披露规则连同整版条目。与 DisclosurePolicyCatalogueRecord 抬头同形、条目不同形：
 * 那册按客户答四维内容,这册按客户 × 信号 × 可信度答异常要不要对外说——相邻两册各自成形。
 */
export interface ExceptionDisclosureRuleCatalogueRecord {
  version: string;
  effectiveFrom: string;
  effectiveTo?: string;
  approvedBy: string;
  entries: ExceptionDisclosureRuleEntryRecord[];
}

/**
 * 冲突信号规则（0025）：一租户至多一行，没有生效区间——换版是一次治理动作而不是接续闭合，
 * 行上只有库落下的登记时刻。仍以数组到场：空册是 []，与其余各册同形。
 */
export interface ConflictSignalRuleRecord {
  signalKind: string;
  version: string;
  confidence: string;
  approvedBy: string;
  registeredAt: string;
}

// 响应体按 kind 判别：唯一业务成格 VISIBILITY_CATALOGUES_LISTED，空册也是这一格
// （ADR-0077 Decision 四，空册本身就是内容，不折成未配置）。
export type VisibilityCatalogueListResponseBody =
  | { outcome: 'VISIBILITY_CATALOGUES_LISTED'; kind: 'MILESTONE_MAPPING'; catalogues: MilestoneMappingCatalogueRecord[] }
  | { outcome: 'VISIBILITY_CATALOGUES_LISTED'; kind: 'TRIAGE_RULE'; catalogues: TriageRuleCatalogueRecord[] }
  | { outcome: 'VISIBILITY_CATALOGUES_LISTED'; kind: 'NOTIFICATION_POLICY'; catalogues: NotificationPolicyRecord[] }
  | { outcome: 'VISIBILITY_CATALOGUES_LISTED'; kind: 'CLAIM_ELIGIBILITY'; catalogues: ClaimEligibilityRecord[] }
  | { outcome: 'VISIBILITY_CATALOGUES_LISTED'; kind: 'CLAIM_AUTHORIZATION'; catalogues: ClaimAuthorizationRecord[] }
  | { outcome: 'VISIBILITY_CATALOGUES_LISTED'; kind: 'DISCLOSURE_POLICY'; catalogues: DisclosurePolicyCatalogueRecord[] }
  | { outcome: 'VISIBILITY_CATALOGUES_LISTED'; kind: 'EXCEPTION_DISCLOSURE_RULE'; catalogues: ExceptionDisclosureRuleCatalogueRecord[] }
  | { outcome: 'VISIBILITY_CATALOGUES_LISTED'; kind: 'CONFLICT_SIGNAL_RULE'; catalogues: ConflictSignalRuleRecord[] };

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

// ---- 六类目录登记写面（ADR-0085，票 admin-write-faces/02 切片 02d）----
//
// 登记端点与其余命令面同挂字面量 `UnconfiguredIntake{}`：写准入不另立形，隔离读准入
// （ADR-0078）换得了读行换不了写行。因此**墙降之前提交必然答 403
// ACCESS_CHANNEL_NOT_CONFIGURED**，那是诚实答案不是接线缺陷；登记参数（PAR-INT-01，
// 实例半边）到位后由装配点换真 Intake 即点亮，本文件一行不用改。
//
// **请求体形状此刻没有契约。** ADR-0085 决定三把「渠道原始载荷 → 登记快照」的翻译划给
// 渠道接入契约，随 `PAR-INT-01` 提供。所以这里不发明字段：页面收的是登记快照 JSON 本体，
// 与受控登记口 `parcel-ve-register <种类> -input <file>` 吃的同一份形状——两口共用
// internal/visibilityexception/adapters/registrationjson 那一份翻译，不是两份碰巧同形。
// 真渠道接线时以渠道契约为准重谈，不得反过来把这里当成已发布的 Schema。

/**
 * 逐种类登记端点。同一本册在读口 `?kind=`、写口路径与 CLI 命令名下是同一个词，只随
 * 各入口的拼写惯例变形（查阅用大写下划线，路径与命令用小写连字符）；分诊那册的册名
 * 在 CLI 是复数 `triage-rules`，路径与读口都用单数，取各自入口已发布的原词，不统一。
 */
export const visibilityRegistrationEndpoints: Record<VisibilityRegistrableCatalogueKind, string> = {
  MILESTONE_MAPPING: '/visibility-catalogue-milestone-mapping-registrations',
  TRIAGE_RULE: '/visibility-catalogue-triage-rule-registrations',
  NOTIFICATION_POLICY: '/visibility-catalogue-notification-policy-registrations',
  CLAIM_ELIGIBILITY: '/visibility-catalogue-claim-eligibility-registrations',
  CLAIM_AUTHORIZATION: '/visibility-catalogue-claim-authorization-registrations',
  DISCLOSURE_POLICY: '/visibility-catalogue-disclosure-policy-registrations',
};

/**
 * 一种类一个端点，本函数按种类取路径而不是裂成六个同形包装。
 *
 * 传输层那边逐类各立一个 Intake 接口与一个端点构造函数，为的是让「把一类的译装接到
 * 另一类的端点上」在编译期就红；那条保护在这里没有落点——快照本体在前端是未翻译的
 * JSON，分不分函数都一样送得出去。所以这里与读口的 listVisibilityCatalogues 同形：
 * 种类是封闭集里的一个参数。
 */
export function registerVisibilityCatalogue(
  kind: VisibilityRegistrableCatalogueKind,
  snapshot: unknown,
): Promise<ApiResult<RegistrationResponseBody>> {
  return postMasterData<RegistrationResponseBody>(visibilityRegistrationEndpoints[kind], snapshot);
}
