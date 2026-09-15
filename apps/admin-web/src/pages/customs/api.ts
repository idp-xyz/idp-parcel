// 本目录 fetch 出口:关务各条目录查阅端点——合规规则库(GET /customs-compliance-rules,
// ADR-0077、票 master-data-wiring/04)、案件配置册(GET /customs-case-registers,票
// admin-web-page-wiring-frontier/05)、门禁条件册(GET /customs-gate-conditions,票 06)
// 与口岸/申报路径册(GET /customs-ports-paths,票 admin-remainder-mechanism-batch/03)。
// 除门禁册只有一本不设分派参数外,其余按 ?registry= 分派、各端点各自封闭集。
// 传输与五格判读收敛在共享 catalogue-api,本文件只保留本上下文的类型与查询函数。

import { exchangeMasterData, postMasterData, type ApiResult } from '../catalogue-api';
import type { RegistrationResponseBody } from '../../components/registration';

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

/** 一条提交授权,与就绪同形的另一条轨(CONTEXT「提交授权与就绪判断分别形成和失效」)。 */
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
  /** 只在承接项在场(承接必须指名接收责任方,CONTEXT「来源责任方、接收责任方、接受决定及权限」)。 */
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

// —— 门禁条件册(customs-restrictions 页放行门禁核对签) ——
// 形状以 internal/customscompliance/adapters/http/query_gate_conditions.go 为准,
// 此处只做镜像不虚构。

/**
 * 一项前置条件认定。state 封闭三值 MET / UNMET / CONFLICTING,刻意没有「未知」格
 * ——判断不出来的前置条件不该进折叠,读回集外取值在服务端读口就上抛,传输层不折第
 * 四格(词表在 presentation.ts)。
 */
export interface GateFindingRecord {
  precondition: string;
  state: string;
}

/**
 * 一份门禁条件目录连同全部已登记认定。findings 空数组是「此动作在此边界本就不受
 * 门禁」的如实一格(领域 FoldGateConclusion 折为不适用);「目录未登记 → 未决」表现
 * 为整份目录不在 gates 里。
 *
 * 这两格的含义与关闭义务那对**相反**(0008 自注、票 06 形状约束一):那边空清单是
 * 「无义务项」的中性事实,这边空清单是放行侧的绿灯,未登记才无从复核。页面文案因此
 * 不得照抄 ClosureObligationCatalogueRecord 那一份。
 */
export interface GateConditionCatalogueRecord {
  scope: string;
  /** 封闭四值 OUTBOUND_RELEASE / LOADING_DEPARTURE / CROSS_CUSTOMS_MOVEMENT / FINAL_DELIVERY。 */
  action: string;
  boundary: string;
  registeredAt: string;
  findings: GateFindingRecord[];
}

export interface GateConditionListResponseBody {
  outcome: 'GATE_CONDITIONS_LISTED';
  gates: GateConditionCatalogueRecord[];
}

/**
 * 门禁册只有一本,故本函数不收分派参数——封闭集为一时参数只会造出一个恒定值。
 * 它与上面两个查询函数形状不同,正是「各立入口 vs 一页里的页签」那道裁决的表形。
 */
export function listGateConditions(): Promise<ApiResult<GateConditionListResponseBody>> {
  return exchangeMasterData<GateConditionListResponseBody>('/customs-gate-conditions');
}

// —— 口岸目录与申报路径目录(customs-ports-paths 页两签查阅面) ——
// 形状以 internal/customscompliance/adapters/http/query_ports_paths.go 为准,
// 此处只做镜像不虚构。

/**
 * 口岸/申报路径册封闭两格,与传输层 ?registry= 分派同词(也与受控 CLI 的两命令同词)。
 * 合规候选区域不在本集:区域维未建模(等自己的票),封闭集不为它预留假格。
 */
export type PortsPathsRegistry = 'candidate-port' | 'declaration-path';

/**
 * 一版口岸合规候选:口岸标识与生效区间。目录事实只有这两件——所属区域与关务适用性
 * 判断都不在册(区域维未建模;适用性是判断链的产物,不是目录事实)。
 */
export interface CandidatePortRecord {
  port: string;
  appliesFrom: string;
  /** 缺席即尚无终点(开放版),不是已失效;终点在后继版本登记时落定(换版)。 */
  appliesUntil?: string;
}

/**
 * 一版申报路径:路径标识、三维路径事实(经哪个口岸、按哪个方向、以哪种申报模式)与
 * 生效区间。port 是标识引用——「引用的口岸此刻是否在册」是读者拿两册对照的判断,
 * 本行不代答。
 */
export interface DeclarationPathRecord {
  path: string;
  port: string;
  /** 封闭二向 IMPORT / EXPORT;词表在 presentation.ts。 */
  direction: string;
  /** 申报模式是引用不是封闭词表:真实模式集属监管规则实例半边。 */
  declarationMode: string;
  appliesFrom: string;
  /** 缺席即尚无终点,约定同 CandidatePortRecord。 */
  appliesUntil?: string;
}

export interface CandidatePortListResponseBody {
  outcome: 'CANDIDATE_PORTS_LISTED';
  ports: CandidatePortRecord[];
}

export interface DeclarationPathListResponseBody {
  outcome: 'DECLARATION_PATHS_LISTED';
  paths: DeclarationPathRecord[];
}

// registry → 结果格的对照,判据同 CaseRegisterBodyByRegistry:调用侧按 registry 收窄
// 后拿到单格类型,不必写空分支。
interface PortsPathsBodyByRegistry {
  'candidate-port': CandidatePortListResponseBody;
  'declaration-path': DeclarationPathListResponseBody;
}

export function listPortsPaths<Registry extends PortsPathsRegistry>(
  registry: Registry,
): Promise<ApiResult<PortsPathsBodyByRegistry[Registry]>> {
  return exchangeMasterData<PortsPathsBodyByRegistry[Registry]>(
    `/customs-ports-paths?registry=${encodeURIComponent(registry)}`,
  );
}

// —— 监管凭证册(customs-cases 页凭证签,票 sa-cc/10) ——
// 形状以 internal/customscompliance/adapters/http/query_credentials.go 为准,
// 此处只做镜像不虚构。

/**
 * 一版监管凭证:一身份一版、不可变,换期限或额度是另一张凭证(0014 自注)。uses 只在来源
 * 提供了次数额度时在场——缺席是「来源未提供」,**不是 0 也不是已用尽**:余额(占用 / 释放 /
 * 核销)不是本册登记内容,页面不得把缺席译成一个数字。registeredAt 是登记动作的时钟,与
 * validFrom / validTo 两端是三件事。
 */
export interface CredentialRecord {
  credential: string;
  issuer: string;
  holder: string;
  procedure: string;
  validFrom: string;
  validTo: string;
  uses?: number;
  registeredAt: string;
}

export interface CredentialListResponseBody {
  outcome: 'CREDENTIALS_LISTED';
  credentials: CredentialRecord[];
}

/** 凭证册只有一本,不收分派参数(判据同 listGateConditions)。 */
export function listCredentials(): Promise<ApiResult<CredentialListResponseBody>> {
  return exchangeMasterData<CredentialListResponseBody>('/customs-credentials');
}

// —— 税费付款协作事项册与税费付款核对册(customs-restrictions 页两签,票 sa-cc/10) ——
// 形状以 internal/customscompliance/adapters/http/query_duty_collaborations.go 与
// query_duty_verifications.go 为准,此处只做镜像不虚构。两册各立入口、不设 ?registry=:
// 它们是 UC-CC-009「七层对象必须分离」里的两层,各自一口。

/**
 * 一份税费付款协作事项。kind 封闭二值:ASSESSED_DUTY 带 duty 不带 noPayBasis,
 * EXPLICITLY_NOT_REQUIRED 反之——第三种「没有结果所以不用付」在类型上没有格(CONTEXT
 * 「税费付款协作事项」:缺少税费结果不能被解释为无需付款)。obligor 只是法定义务人,
 * 实际付款方与最终承担费用的客户可以不同、不能互相推导(CONTEXT「不能互相推导」),本行没有那两列。
 */
export interface DutyCollaborationRecord {
  scope: string;
  kind: string;
  duty?: string;
  noPayBasis?: string;
  obligor: string;
  requirement: string;
  target: string;
  formedAt: string;
}

export interface DutyCollaborationListResponseBody {
  outcome: 'DUTY_COLLABORATIONS_LISTED';
  collaborations: DutyCollaborationRecord[];
}

export function listDutyCollaborations(): Promise<ApiResult<DutyCollaborationListResponseBody>> {
  return exchangeMasterData<DutyCollaborationListResponseBody>('/customs-duty-collaborations');
}

/**
 * 一版税费付款核对。coverage / delta / validity 三轴各自封闭、各占一列——互斥总状态是
 * CONTEXT 明禁形状(ADR-0137 决定三),本行没有、页面也不得折出一个「付款状态」。version
 * 是幂等键上的内容指纹:同键多版本各自成行,迟到事实按新版本追加不覆盖,哪版是当前由
 * 读者按 verifiedAt 判读。basis 是「凭什么把这笔资金关联到这版税费」的证据引用。procedure 是
 * 付款人维按其规则判的真实程序——核对记录的依据维,不是身份维(票 sa-cc/22)。
 */
export interface DutyVerificationRecord {
  duty: string;
  funds: string;
  scope: string;
  procedure: string;
  version: string;
  /** 封闭三值 NONE / PARTIAL / COVERED;词表在 presentation.ts。 */
  coverage: string;
  /** 封闭四值 NO_DELTA / SHORT / EXCESS / PENDING。 */
  delta: string;
  /** 封闭四值 VALID / INVALIDATED / CONFLICTING / PENDING。 */
  validity: string;
  basis: string;
  verifiedAt: string;
}

export interface DutyVerificationListResponseBody {
  outcome: 'DUTY_VERIFICATIONS_LISTED';
  verifications: DutyVerificationRecord[];
}

export function listDutyVerifications(): Promise<ApiResult<DutyVerificationListResponseBody>> {
  return exchangeMasterData<DutyVerificationListResponseBody>('/customs-duty-verifications');
}

// —— 五类配置登记的在线登记口(ADR-0085,票 admin-write-faces/02 切片 02b) ——
// 形状以 internal/customscompliance/adapters/http/register_configuration.go 为准。
//
// 登记端点与其余命令面同挂字面量 UnconfiguredIntake{}:写准入不另立形,隔离读准入
// (ADR-0078)换得了读行换不了写行。因此墙降之前提交必然答 403
// ACCESS_CHANNEL_NOT_CONFIGURED,那是诚实答案不是接线缺陷;登记参数(PAR-INT-01,实例
// 半边)到位后由装配点换真 Intake 即点亮,本文件一行不用改。
//
// 请求体形状此刻没有契约。ADR-0085 决定三把「渠道原始载荷 → 登记快照」的翻译划给渠道
// 接入契约、随 PAR-INT-01 提供,所以这里不发明字段:页面收的是登记快照 JSON 本体,与
// 受控登记口 parcel-customs-register <命令> -input 吃的同一份形状,原样作请求体送出。
// 真渠道接线时以渠道契约为准重谈,不得反过来把这里当成已发布的 Schema。

/**
 * 交回案件配置族答案（application.CaseConfigurationOutcome）的可在线登记关务册。词与端点
 * 路径、受控 CLI 的子命令逐字同一个——同一本册在写口与 CLI 不换词。解释规则的读口参数是
 * interpretation 而写口词是 interpretation-rule,两处不同源自各自端点,本类型跟写口,不改读口那半。
 *
 * 建案要求规则比配置四类晚一步进来:票 02 的关务片把十二个用例分成「配置四类」与「案件
 * 事实七类」,四加七只有十一个,漏掉的第十二个正是它,端点随之补建。监管凭证（票 sa-cc/07
 * 步二）不是配置册而是一版不可变凭证,进本类型是因为它的登记用例交回同一套配置族答案,
 * `registrationOutcomeLabels` 那张表对它逐格成立;换期限 / 持有人 / 额度在册上全是内容冲突
 * ——那是另一张凭证,走另一个身份登记。
 *
 * 案件事实那七个命令(就绪、提交授权及其撤销、关闭义务目录与明细、门禁发现)按票 02 的
 * 范围裁定本就不进写面——它们改的是案上此刻的事实,不是这个租户怎么配置。税费付款协作
 * 与核对两口的答案代数是另一族,见下面 DutyRegistrationKind。
 */
export type CustomsRegistrationKind =
  | 'interpretation-rule'
  | 'case-requirement'
  | 'gate-catalog'
  | 'candidate-port'
  | 'declaration-path'
  | 'regulatory-credential';

export const customsRegistrationEndpoints: Record<CustomsRegistrationKind, string> = {
  'interpretation-rule': '/customs-interpretation-rule-registrations',
  'case-requirement': '/customs-case-requirement-registrations',
  'gate-catalog': '/customs-gate-catalog-registrations',
  'candidate-port': '/customs-candidate-port-registrations',
  'declaration-path': '/customs-declaration-path-registrations',
  'regulatory-credential': '/customs-regulatory-credential-registrations',
};

/**
 * 一类一个端点,本函数按类取路径而不是裂成五个同形包装。传输层那边逐类各立一个端点
 * 构造函数,为的是让「把一类的译装接到另一类的端点上」在编译期就红;那条保护在这里
 * 没有落点——快照本体在前端是未翻译的 JSON,分不分函数都一样送得出去。
 */
export function registerCustomsConfiguration(
  kind: CustomsRegistrationKind,
  snapshot: unknown,
): Promise<ApiResult<RegistrationResponseBody>> {
  return postMasterData<RegistrationResponseBody>(customsRegistrationEndpoints[kind], snapshot);
}

/**
 * 登记答案代数(application.CaseConfigurationOutcome 原名),逐格中文。本上下文各类共用
 * 一份——服务端那几个登记端点交回的就是同一个枚举。
 *
 * 没有 UNDECIDED 一格。用例把依赖故障折成那个枚举值,而传输层按 ADR-0022 把它写成
 * 「没形成答案」的 5xx,它到不了这张表;真落进来会被 RegistrationPanel 当成登记册的治理
 * 答案示出,而两者的续办动作相反——未决重跑同一份即可,治理答案重试没有用。
 *
 * 负向三格逐格分开说而不折成一句「提交失败」:原行都不被顶替,但续办动作各不相同——
 * 冲突要人工核对既有登记,受理门拒绝要补齐缺件,重放则什么都不用做。
 */
export const registrationOutcomeLabels: Record<string, string> = {
  REGISTERED: '已登记',
  EXISTING: '已在册——同键同内容的重放,原行不动',
  // 改法随各册而异,所以这一格只说不变式加「去核对」,不给一个只对一半册子成立的处方:
  // 三本版本册可以换一个更晚的生效起点登新版,建案要求规则没有版本维,同键异内容一律
  // 落在本格。写死「换生效起点」会让登记方在后者上试一个根本不存在的动作。
  CONTENT_CONFLICT:
    '内容冲突——同键异内容绝不顶替,原行原样留着;续办先核对既有登记,改法随该册有无版本维而异',
  NOT_ACCEPTED: '受理门拒绝——缺件或形状不合,补齐后重登;原行不被顶替',
};

// —— 税费付款协作事项与税费付款核对两口的在线登记(ADR-0085,票 sa-cc/07 步二) ——
// 形状以 internal/customscompliance/adapters/http/register_credential_and_duty.go 为准。
//
// 两口与上面配置族分开成一族,不并进 CustomsRegistrationKind:它们的登记用例交回的是另一套
// 答案代数(application.DutyReconciliationResult),「未决」在那一族分业务未决与依赖故障两半
// ——前者是形成了的答案(200 带 undecidedReason),后者才是没形成答案(5xx)。并进配置族就得让
// registrationOutcomeLabels 那张表替两族说话,而两族有重名格(NOT_ACCEPTED)、续办说法不同。
//
// 写准入、请求体形状与墙降前必答 403 的口径同上面配置族,此处不复述。

/** 协作 / 核对两口封闭二格,词与端点路径、受控 CLI 子命令逐字同一个。 */
export type DutyRegistrationKind = 'duty-collaboration' | 'duty-payment-verification';

export const dutyRegistrationEndpoints: Record<DutyRegistrationKind, string> = {
  'duty-collaboration': '/customs-duty-collaboration-registrations',
  'duty-payment-verification': '/customs-duty-payment-verification-registrations',
};

export function registerDutyReconciliation(
  kind: DutyRegistrationKind,
  snapshot: unknown,
): Promise<ApiResult<RegistrationResponseBody>> {
  return postMasterData<RegistrationResponseBody>(dutyRegistrationEndpoints[kind], snapshot);
}

/**
 * 协作 / 核对两口的登记答案代数(application.DutyReconciliationOutcome 原名),逐格中文。两口
 * 共一张表:同一个枚举、同一格不因来自哪一口而换说法(判据同受控 CLI 的一族一张表)。
 *
 * 资金事实那族格(FUNDS_FACT_RECEIVED / EXISTING_FUNDS_FACT / FUNDS_FACT_CONTENT_CONFLICT)不在表上:
 * 没有任何在线口能交回它们——资金事实只经 settlement-accounting 的采用信封进 CC(ADR-0137 决定
 * 四),列出来就是给一格走不到的答案配中文。核对没有「内容冲突」格:同三维换内容是新版本追加
 * (迟到事实按新版本进、不按到达顺序覆盖),答的仍是形成——表上没有这一格不是漏,是该族的形状。
 *
 * UNDECIDED 这一格**在表上**,与配置族那张表相反:那边用例把依赖故障折成 UNDECIDED、传输层写成
 * 5xx,它到不了表;这边 200 带 outcome=UNDECIDED 只有一种来路——义务依据缺席的业务未决(等核定
 * 税费或明确无需付款依据到了重发同一份),原因在 dutyUndecidedReasonLabels 逐格说。
 *
 * 负向各格续办动作不同,逐格分开说:待关联要补权威关联依据(金额相等、同范围、同付款人都不算)
 * 再登;两道前置未齐要等前置落册后重发同一份;受理门拒绝要改内容;冲突要人工核对。
 */
export const dutyRegistrationOutcomeLabels: Record<string, string> = {
  COLLABORATION_FORMED: '协作事项已形成',
  EXISTING_COLLABORATION: '协作事项已存在——同键同内容的重放,原行不动',
  COLLABORATION_CONTENT_CONFLICT:
    '协作事项内容冲突——同(范围,税费引用)已在册且内容不同,绝不顶替;先核对既有协作事项',
  DUTY_VERIFICATION_FORMED: '核对已形成',
  EXISTING_DUTY_VERIFICATION: '核对已存在——同三维同内容的重放,原行不动;换内容是新版本追加,不是冲突',
  FUNDS_FACT_PENDING_ASSOCIATION:
    '待关联——无权威关联依据不关联;金额相等、同范围、同付款人都不单独构成依据,补上依据再登',
  FUNDS_FACT_NOT_RECEIVED: '资金事实未接收——前置未齐;等 settlement-accounting 采用的资金事实到了 CC 再重发同一份',
  COLLABORATION_NOT_FORMED: '协作事项未形成——前置未齐;先登协作事项再重发同一份',
  NOT_ACCEPTED: '受理门拒绝——缺件或形状不合,补齐后重登;原行不被顶替',
  UNDECIDED: '业务未决——编排形成了答案但在等前置,见「在等」;等到了重发同一份即可',
};

/** 业务未决原因的逐格中文(application.DutyReconciliationReason 里唯一能随 200 到场的那一格)。 */
export const dutyUndecidedReasonLabels: Record<string, string> = {
  DUTY_OBLIGATION_BASIS_ABSENT:
    '义务依据缺席——既无已接受的监管核定税费,也无明确无需付款依据;缺少税费结果不能被解释为无需付款',
};
