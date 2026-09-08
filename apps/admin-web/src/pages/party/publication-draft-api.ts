// 运营操作者面商业发布路径的四口客户端（ADR-0126 Decision 三、四；票 admin-write-faces/08 落 Go 侧，
// 票 16 落本文件作前端公共半边，供子票 09–17 各册表单共用）。发布相关一律在这里，不进 api.ts：那一份
// 是读面与受控 JSON 镜像口的出口，两条路径的答案代数不同，混在一处会让「主路径」与「高级口」在
// 调用点长得一样。
//
// 线格式逐格镜像 internal/partycommercial/adapters/http 的 publication_draft_payload.go（载荷）、
// preview_commercial_publication.go（预览答复）、publication_draft.go（载体三口答复）与
// register_commercial_publication.go 的 publicationAnswer（发布口嵌进来的受控发布答案）。键名与
// outcome 原词从 Go 抄，不自造；Go 那边加一格，这里跟着加一格。
//
// **载荷里只有内容，没有身份，也没有摘要。** 租户、录入者、批准者由 Intake 从 ADR-0100 的操作者信封交进，
// 载荷里出现 tenant / submitter / contentDigest / approval 之类的键会被严格解码按未知键拒（伞票 07 硬句：
// 表单不算摘要、不收也不送批准人）。**今天四口都挂字面量 UnconfiguredIntake{}**，请求必然答
// 403 ACCESS_CHANNEL_NOT_CONFIGURED——那是诚实答案不是接线缺陷（ADR-0085 两阶段），装配点换真 Intake 即
// 点亮，本文件一行不用改。

import { exchangeMasterData, postMasterData, type ApiResult } from '../catalogue-api';

/**
 * 发布轴的商业对象类别（domain.CommercialObjectKind 的 String() 原词，封闭集；Go 那边加一格这里跟着加）。
 * 它与 api.ts 的 CommercialPolicyKind（政策册的 `?kind=`）是两条分类轴，不逐字对应（见 presentation.ts
 * 的 policyKindSources）。
 */
export type CommercialObjectKindName =
  | 'SERVICE_PRODUCT'
  | 'CUSTOMER_CONTRACT'
  | 'SUPPLIER_AGREEMENT'
  | 'ACCEPTANCE_RULE_PACKAGE'
  | 'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY'
  | 'PRICE_RULE'
  | 'SETTLEMENT_POLICY'
  | 'CREDIT_POLICY'
  | 'AUTHORIZATION_RULE'
  | 'CUSTOMER_SERVICE_RULE';

/**
 * 信用政策册正文（Go `CreditPolicyBodyPayload`）。额度两键**恰一在场**由领域构造门判——两格都填或都空
 * 照样送上去，答回来的是 `creditPolicy.limit` 那一格的拒绝；零金额是「授予零信用」，不折成缺席，所以
 * 这里是数值而不是可空字符串。
 */
export interface CreditPolicyBodyPayload {
  legalEntity: string;
  authorityLevel: string;
  chargeType: string;
  limitMinor?: number;
  limitRatioBasisPoints?: number;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
}

/**
 * 供应商协议册正文（Go `SupplierAgreementBodyPayload`，票 admin-write-faces/11）。六格无子表：供应商与责任法人是
 * 主数据读面上的引用、采购方案是 parcel-pricing 方案版本的引用串（从价卡目录选，表单不读方案内容）、协议自己的
 * 适用范围与区间。**没有方向键**——领域把供应商协议钉死为采购（BUY），载荷里出现 direction 按未知键拒。
 */
export interface SupplierAgreementBodyPayload {
  supplier: string;
  legalEntity: string;
  scope: string;
  purchasePlan: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
}

/**
 * 载荷线格式（Go `CommercialPublicationPayload`）：版本壳（少了身份里的租户与摘要）加各册正文一格。
 * 各册子票在这里各加自己那一格正文类型，与 Go 侧同笔加；正文格的键名就是 Go 结构体上的 json 标签。
 */
export interface CommercialPublicationPayload {
  kind: CommercialObjectKindName;
  objectId: string;
  version: string;
  scope: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  /** 壳上的指名引用：被引对象类别（原词）→ 对象标识。 */
  references?: Partial<Record<CommercialObjectKindName, string>>;
  creditPolicy?: CreditPolicyBodyPayload;
  supplierAgreement?: SupplierAgreementBodyPayload;
  customerContract?: CustomerContractBodyPayload;
  authorizationRule?: AuthorizationRuleBodyPayload;
  settlementPolicy?: SettlementPolicyBodyPayload;
  pricePolicy?: PricePolicyBodyPayload;
  acceptanceRulePackage?: AcceptanceRulePackageBodyPayload;
  preAcceptanceFinancialControlPolicy?: PreAcceptanceFinancialControlPolicyBodyPayload;
}

/**
 * 授权规则册正文（Go `AuthorizationRuleBodyPayload`，票 admin-write-faces/17）：只有取消授权目录一节——请求方 × 规则引用
 * 的几行。`party` 是封闭二值的原词，由服务端词表读口供下拉（`fetchPublicationVocabulary('AUTHORIZATION_RULE')` 的 `party`
 * 集），表单不内置；同一请求方第二行、零行都由服务端在预览上答`未受理`带成因。授权授予册不经这条发布路，载荷里没有它的键。
 */
export interface AuthorizationRuleBodyPayload {
  cancellationAuthority: CancellationAuthorityRulePayload[];
}

export interface CancellationAuthorityRulePayload {
  party: string;
  rule: string;
}

/**
 * 结算政策册正文（Go `SettlementPolicyBodyPayload`，票 admin-write-faces/15）：一种结算方式与它覆盖的六维适用范围
 * （0011），键名镜像受控批文 settlementPolicyBody。`method` 只收 String() 原词（PREPAID / TERMS）——码由词表读口供
 * （fetchPublicationVocabulary('SETTLEMENT_POLICY') 的 `method` 一集），表单不内置枚举；第三个取值「客户级默认」是
 * 本上下文明禁的，载荷层不替它开口。`contract` 是客户合同版本的**二维引用**（对象 + 版本两格，与批文的
 * contractVersionDocument 同形）：两段式「对象/版本」串只许领域 NewQualifiedVersionLabel 一处拼，载荷里写成串或
 * `contractLabel` 键都会被服务端按形状 / 未知键拒。六维一维不少（ADR-0044）；区间上界可缺。
 */
export interface SettlementPolicyBodyPayload {
  method: string;
  legalEntity: string;
  counterparty: string;
  contract: ContractVersionReferencePayload;
  chargeScope: string;
  currency: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
}

/** 客户合同版本的二维引用（Go `ContractVersionReferencePayload`）：哪个合同对象、哪一版。 */
export interface ContractVersionReferencePayload {
  objectId: string;
  version: string;
}

/**
 * 价格规则册正文（Go `PricePolicyBodyPayload`，票 admin-write-faces/14）：0010 正文七格加可缺的口径节，键名镜像受控
 * 批文 `pricePolicyBody`。`planDirection` 与 `conversion` 是发布当时 parcel-pricing 对方案方向的答复与当时声明的转换
 * （ADR-0057）——表单如实收、不从方案反推、不预选；`conversion` 空串不会被服务端折成 NONE，SELL 绑 BUY 却没写转换由
 * 领域答`适用冲突`（AT-PC-033）。口径节**没有方向键**：体积口径的方向就是政策方向，载荷里出现按未知键拒。
 */
export interface PricePolicyBodyPayload {
  direction: string;
  pricingPlan: string;
  planDirection: string;
  conversion: string;
  scope: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  caliber?: PricePolicyCaliberPayload;
}

/**
 * 口径节（Go `PricePolicyCaliberPayload`）。`taxClassification` 只在含税 / 未税时在场、`volumetricFactor` 只在销售方向在场
 * ——在场规则由服务端按库上 CHECK 同形的构造门答在这两格上，表单只按 taxDisposition / direction 显隐（显隐是呈现不是
 * 裁门）。`fx` 可缺：不涉及外币的政策没有汇率口径，缺席是「没声明」；在场则三格缺一即点名那一格。
 */
export interface PricePolicyCaliberPayload {
  taxDisposition: string;
  taxClassification?: string;
  volumetricFactor?: string;
  fx?: FxCaliberPayload;
}

export interface FxCaliberPayload {
  quoteType: string;
  asOfSemantics: string;
  asOfPolicyVersion: string;
}

/**
 * 接单规则包册正文（Go `AcceptanceRulePackageBodyPayload`，票 admin-write-faces/12）：一格分节，键名镜像受控批文
 * `declarations` 下的同名键——0014 的正文 `rulePackageBody` 不可缺，其余每条声明通道各一节，**整节缺席 = 该通道
 * 未声明**（表单把留空的节整节不送，不把「留空」写成「无」）。各节里的封闭集（规则分类、判断类型、校验组、
 * 人工复核、收寄来源、责任结果、起算时刻种类、修订阶段 / 意图 / 允许性）由服务端词表读口供下拉
 * （`fetchPublicationVocabulary('ACCEPTANCE_RULE_PACKAGE')`），表单不内置；跨格的判（同一判断两条时点锚、空组、
 * 只有有效期没有终局行、未封闭零格）由领域在预览上答`未受理`带成因。待路由许可挂在服务产品版本上，不在本册。
 */
export interface AcceptanceRulePackageBodyPayload {
  rulePackageBody: RulePackageBodyPayload;
  asOfPolicies?: AsOfPolicyPayload[];
  acceptanceContent?: AcceptanceContentPayload;
  intakeQualification?: IntakeQualificationPayload;
  finalRules?: FinalRulePayload[];
  finalRuleValidity?: FinalRuleValidityPayload;
  sourceDataAmendment?: SourceDataAmendmentPayload;
}

/** 0014 正文：五维适用性（区间上界可缺）与按分类归档的规则引用表。 */
export interface RulePackageBodyPayload {
  serviceProduct: string;
  contract: string;
  legalEntity: string;
  scope: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  rules?: AssembledRulePayload[];
}

export interface AssembledRulePayload {
  category: string;
  reference: string;
}

export interface AsOfPolicyPayload {
  judgment: string;
  semantics: string;
  policyVersion: string;
}

/** `manualReview` 答的是「要不要人工复核」（接单规则正文），不是「谁有权」——表单只收前者（票 12 硬句）。 */
export interface AcceptanceContentPayload {
  applicableGroups?: string[];
  manualReview: string;
}

export interface IntakeQualificationPayload {
  sources?: string[];
  qualifications?: string[];
}

export interface FinalRulePayload {
  outcome: string;
  finalKind: string;
}

/** 面单有效期（ADR-0119）：随终局规则同一通道的一格，`duration` 是 ISO-8601 子集 `P[nD][T[nH][nM][nS]]`。 */
export interface FinalRuleValidityPayload {
  anchor: string;
  duration: string;
}

/**
 * 资料修订允许（ADR-0120）。`closed` 可缺是为了让「没选」原样到达服务端（它答「须在场」）——false 是「缺格转复核」、
 * true 是「缺格即不允许」，两句都要登记方自己说，表单不给默认。
 */
export interface SourceDataAmendmentPayload {
  closed?: boolean;
  rules?: SourceDataAmendmentRulePayload[];
}

export interface SourceDataAmendmentRulePayload {
  dataGroup: string;
  stage: string;
  intent: string;
  allowance: string;
}

/**
 * 接受前财务控制策略册正文（Go `PreAcceptanceFinancialControlPolicyBodyPayload`，票 admin-write-faces/13）：父行一格共同
 * 通过条件加子表逐行的控制项（0024，ADR-0115 Decision 二），键名镜像受控批文 preAcceptanceFinancialControlPolicyBody。
 * 三个封闭集（jointPassCondition / control / onFailure）只收 String() 原词——码由词表读口供
 * （fetchPublicationVocabulary('PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY') 的同名三集），表单不内置枚举；控制种类里没有
 * 「无控制」（ADR-0115 Decision 一：那一句由客户合同声明），载荷层不替它开口。至少一项、判断顺序唯一、（种类 × 范围）
 * 唯一是跨行的门，由服务端在预览上答成成因，不在这里判。
 */
export interface PreAcceptanceFinancialControlPolicyBodyPayload {
  jointPassCondition: string;
  controls: PreAcceptanceControlItemPayload[];
}

/**
 * 一行控制项（Go `PreAcceptanceControlItemPayload`）：种类 × 费用范围引用 × 判断顺序 × 失败处置 × 责任引用。范围与
 * 责任是开放引用（0024 头注），只查非空不校验存在性。`order` 在 Go 侧是普通整数（零与缺席同义——都不是「排第几」
 * 的答案），这里写成可缺：表单留空即不送键，服务端点名 `.order`，不由表单替它填 0。
 */
export interface PreAcceptanceControlItemPayload {
  control: string;
  chargeScope: string;
  order?: number;
  onFailure: string;
  responsibility: string;
}

/**
 * 客户合同册正文（Go `CustomerContractBodyPayload`，票 admin-write-faces/10）：一格两层，键名镜像受控批文
 * declarations 下的 `contractContent`（0012 正文：规则包 + 按费用范围的约定表）与 `preAcceptanceControl`
 * （0007 合同级「要不要」声明）。约定行两格**恰一在场**、`不适用`必带依据都由服务端裁——两格都空或都填照样
 * 送上去，答回来的是那一行 / 那一节的拒绝，表单不代判也不静默补齐。策略侧没有「无控制」取值（ADR-0115
 * Decision 一）：「明确无控制」只能写成 `inapplicabilityBasis` 或合同级 `NOT_APPLICABLE` + 依据。
 */
export interface CustomerContractBodyPayload {
  contractContent: ContractContentPayload;
  preAcceptanceControl?: PreAcceptanceControlPayload;
}

export interface ContractContentPayload {
  rulePackage: string;
  bindings?: ControlBindingPayload[];
}

export interface ControlBindingPayload {
  chargeScope: string;
  policy?: string;
  inapplicabilityBasis?: string;
}

export interface PreAcceptanceControlPayload {
  requirement: string;
  notApplicableBasis?: string;
}

/** 批准口与发布口的载荷（Go `PublicationDraftReferencePayload`）：只指名哪一版载体，身份从信封来。 */
export interface PublicationDraftReferencePayload {
  kind: CommercialObjectKindName;
  objectId: string;
  version: string;
}

/** 一格立不住的记录（Go `payloadProblemAnswer`）：`field` 是载荷里的 JSON 路径，`problem` 是构造门原话。 */
export interface PayloadProblemRecord {
  field: string;
  problem: string;
}

/**
 * 预览答复（Go `publicationPreviewAnswer`）。`PREVIEWED` 带规范化版本与摘要；`NOT_ACCEPTED` 带 `cause`
 * （领域构造门一句话：没接的册 / 正文缺席 / 壳立不住）**或** `problems`（解码收齐的逐格问题）。不受理时
 * 没有摘要可透——服务端刻意不给，免得一个空摘要看起来像算出来的。
 */
export interface PublicationPreviewResponseBody {
  outcome: string;
  canonicalization?: string;
  contentDigest?: string;
  cause?: string;
  problems?: PayloadProblemRecord[];
}

/**
 * 录入口与批准口共用的答复（Go `draftAnswer`）。载体在场时带它的规范化版本、摘要与状态；`cause` 只随
 * 录入的 `NOT_ACCEPTED` 在场。`status` 取 domain.PublicationDraftStatus 原词，见 draftStatusLabels。
 */
export interface PublicationDraftResponseBody {
  outcome: string;
  canonicalization?: string;
  contentDigest?: string;
  status?: string;
  cause?: string;
}

/**
 * 受控发布用例的整份答案（Go `publicationAnswer`，与 `/commercial-publications` 同一形状）。发布口把它
 * 嵌进自己的答复里——同一个用例的答案在两口不换形。`declaredDigest` / `computedDigest` 两串只随对账门
 * 的 `NOT_ACCEPTED` 在场（受控批文那一半，主路径走不到）。
 */
export interface CommercialPublicationAnswerBody {
  outcome: string;
  pendingCause?: string;
  cause?: string;
  declaredDigest?: string;
  computedDigest?: string;
  declarations?: { channel: string; outcome: string }[];
}

/**
 * 发布口答复（Go `draftPublicationAnswer`）：载体侧的 `outcome`，加受控发布用例的整份答案——它只在载体真
 * 交给了发布用例时在场（`DRAFT_PUBLISHED` 与 `PUBLICATION_NOT_LANDED` 两格）。
 */
export interface PublicationDraftPublicationResponseBody {
  outcome: string;
  publication?: CommercialPublicationAnswerBody;
}

/**
 * 四口路径。预览与录入送**同一份**载荷、走服务端同一段解码，预览页上的摘要与载体上记下的逐字节相等
 * （ADR-0101 决定四）；批准与发布只送载体引用。
 */
export const publicationDraftEndpoints = {
  preview: '/commercial-publication-previews',
  submit: '/commercial-publication-drafts',
  approve: '/commercial-publication-draft-approvals',
  publish: '/commercial-publication-draft-publications',
} as const;

export function previewCommercialPublication(
  payload: CommercialPublicationPayload,
): Promise<ApiResult<PublicationPreviewResponseBody>> {
  return postMasterData<PublicationPreviewResponseBody>(publicationDraftEndpoints.preview, payload);
}

export function submitPublicationDraft(
  payload: CommercialPublicationPayload,
): Promise<ApiResult<PublicationDraftResponseBody>> {
  return postMasterData<PublicationDraftResponseBody>(publicationDraftEndpoints.submit, payload);
}

export function approvePublicationDraft(
  reference: PublicationDraftReferencePayload,
): Promise<ApiResult<PublicationDraftResponseBody>> {
  return postMasterData<PublicationDraftResponseBody>(publicationDraftEndpoints.approve, reference);
}

export function publishPublicationDraft(
  reference: PublicationDraftReferencePayload,
): Promise<ApiResult<PublicationDraftPublicationResponseBody>> {
  return postMasterData<PublicationDraftPublicationResponseBody>(
    publicationDraftEndpoints.publish,
    reference,
  );
}

// ——以下为答案代数的逐格中文。四口各一张表而不并成一张：`NOT_ACCEPTED` 在预览与录入两栏说的都是
// 「输入立不住」，但 `DRAFT_NOT_FOUND` / `DRAFT_ALREADY_PUBLISHED` 在批准栏与发布栏的续办动作不同，
// 并表会让一栏顶着另一栏的中文显示出来（判据同 api.ts 的 publicationOutcomeLabels 与
// declarationLandingLabels 分表）。未收录的 outcome 由页面原样示出英文原名。

/** 预览答案代数（`application.PreviewCommercialPublicationOutcome` 原名）。 */
export const previewOutcomeLabels: Record<string, string> = {
  PREVIEWED: '已预览（未落库；规范化版本与内容摘要如下，与录入后载体上记下的逐字节相等）',
  NOT_ACCEPTED: '不受理（壳或正文立不住，什么也没算；哪几格不对见下）',
};

/**
 * 录入答案代数（`application.SubmitPublicationDraftOutcome` 原名）。
 *
 * `CONTENT_FIXED` 不是失败也不是放行：这一版载体已过批准，正文固定，眼前这份与它不同——改内容要换
 * 版本号，不是改这一版。`DRAFT_REVISED` 是待批准期间的修订，载体就地更新、仍待批准。
 *
 * `DRAFT_SUBMITTED` 那句不写「等一位不是录入者的批准者」：能否自批由租户的审批职责规则（`PAR-COM-18`）说，
 * ADR-0126 Decision 三不写死双人也不写死单人；规则真要求换人时，答的是批准口的 `NEEDS_ANOTHER_APPROVER`。
 */
export const submitOutcomeLabels: Record<string, string> = {
  DRAFT_SUBMITTED: '已存为待批准（载体本次落册，等批准；能否自批由租户审批职责规则说）',
  DRAFT_REPLAYED: '同一份重放（载体已在册且内容相同，本次没有造第二份）',
  DRAFT_REVISED: '待批准期间已修订（载体就地更新为眼前这份，仍待批准）',
  CONTENT_FIXED: '内容已固定（这一版载体已批准，正文不再收改动；改内容要发新版本号）',
  NOT_ACCEPTED: '不受理（壳或正文立不住，未到达载体册；原因随答复交回）',
};

/**
 * 批准答案代数（`application.ApprovePublicationDraftOutcome` 原名）。拒绝按恢复动作分格（ADR-0029）：
 * `NOT_CONFIGURED` 要租户先登审批职责规则（`PAR-COM-18`，实例半边「待提供」）——不是重试能好的事；
 * `NEEDS_ANOTHER_APPROVER` 与 `APPROVER_NOT_QUALIFIED` 要换人；`DRAFT_CHANGED` 是两个操作者先后动手，
 * 重读再来。
 */
export const approveOutcomeLabels: Record<string, string> = {
  DRAFT_APPROVED: '已批准（载体推进一格，等发布）',
  DRAFT_NOT_FOUND: '载体不在册（先存为待批准）',
  NOT_CONFIGURED: '审批职责规则未登记（PAR-COM-18 实例半边待提供；未登记即不放行，不以任何默认代替）',
  NEEDS_ANOTHER_APPROVER: '需换人批准（批准者不得是录入者）',
  APPROVER_NOT_QUALIFIED: '批准者不合格（授予集不满足审批职责规则要求的层级）',
  DRAFT_ALREADY_APPROVED: '载体已批准（这一格已过，直接发布）',
  DRAFT_ALREADY_PUBLISHED: '载体已发布（这一版已在册）',
  DRAFT_CHANGED: '载体在读与写之间被别的操作者动过（重读后再批）',
};

/**
 * 发布答案代数（`application.PublishPublicationDraftOutcome` 原名）。
 *
 * `DRAFT_AWAITS_EFFECTIVE_START` 是一格答案不是错误：载体区间未开，发布用例对挂在`已计划生效`版本上的
 * 声明整项拒、且只增仓储没有日后补声明的口，所以到界前不交给它，届期再来发布（票 08 完成记录点名要
 * 显给操作者的那一格）。`PUBLICATION_NOT_LANDED` 说的是受控发布用例答了未决 / 冲突 / 不受理之一，
 * 载体留在`已批准`，用例那一份答案随答复嵌回，续办照那一格办。
 */
export const publishOutcomeLabels: Record<string, string> = {
  DRAFT_PUBLISHED: '已发布（受控发布落定，载体推进为已发布；结果在对应册立刻可见）',
  DRAFT_NOT_FOUND: '载体不在册（先存为待批准）',
  DRAFT_NOT_APPROVED: '载体尚未批准（先批准）',
  DRAFT_ALREADY_PUBLISHED: '载体已发布（这一版已在册）',
  DRAFT_AWAITS_EFFECTIVE_START: '等待生效边界（载体区间未开，到界前不交给发布用例；届期再来发布，不是失败）',
  PUBLICATION_NOT_LANDED: '发布未落定（受控发布用例答了未决 / 冲突 / 不受理之一，载体留在已批准；用例答案见下）',
};

/** 载体状态（`domain.PublicationDraftStatus` 原名），只向前：待批准 → 已批准 → 已发布。 */
export const draftStatusLabels: Record<string, string> = {
  PENDING_APPROVAL: '待批准',
  APPROVED: '已批准',
  PUBLISHED: '已发布',
};

// ——商业发布词表读口（票 admin-write-faces/20，通道 1 代裁）：一口按 kind 答该册正文里各封闭集的码，表单据此供
// 下拉、不内置枚举——ADR-0126 之后正文由服务端按册规范化，表单自带一份枚举就是同一封闭集的第二份写法，漂了无人报。
// 服务端只给码不给中文：中文在各页自己的词表里，下拉选项 = 服务端码 × 本页词表（vocabularyOptions）。这里没有任何
// 一份内置的码作回退：读不到就是读不到（未配置 / 调用方问题 / 未形成答案各归五格结果代数的一格），表单据格写占位。
// 各表单票只消费 fetchPublicationVocabulary，不各写 fetch。

/** 一个封闭集（Go `vocabularySetAnswer`）：`name` 是载荷里那格的键名（如接单规则包的 `stage`），`codes` 按领域枚举顺序。 */
export interface VocabularySetRecord {
  name: string;
  codes: string[];
}

/**
 * 词表答复（Go `publicationVocabularyAnswer`）。kind 合法而正文没有枚举格（服务产品、客户合同……）时 `sets` 是
 * 空数组——不是缺键、不是 404；kind 集合外服务端答 400 + MALFORMED_REQUEST（问题落在 `kind` 那一格），走 `callerProblem`。
 */
export interface PublicationVocabularyResponseBody {
  outcome: string;
  kind: CommercialObjectKindName;
  sets: VocabularySetRecord[];
}

export const publicationVocabularyEndpoint = '/commercial-publication-vocabularies';

/**
 * 读一册的词表。走查阅那半的 exchangeMasterData：403 ACCESS_CHANNEL_NOT_CONFIGURED 交回 `unconfigured` 一格而不是抛错
 * ——它与四口同挂未配置 Intake（理由在 Go 端点注释），表单拿这一格写「词表未就绪」的占位，不拿任何内置码顶替。
 */
export function fetchPublicationVocabulary(
  kind: CommercialObjectKindName,
): Promise<ApiResult<PublicationVocabularyResponseBody>> {
  return exchangeMasterData<PublicationVocabularyResponseBody>(
    `${publicationVocabularyEndpoint}?kind=${encodeURIComponent(kind)}`,
  );
}

/** 一个下拉选项：`value` 是送回服务端的码，`label` 是给操作者看的字。 */
export interface VocabularyOption {
  value: string;
  label: string;
}

/**
 * 码 × 本页中文词表 → 下拉选项，顺序照服务端（领域枚举顺序）。词表没收录的码原样示出、不猜格——判据沿
 * channel-selection-decisions.ts 的 wordOf：某天服务端多一格，页面宁可显示英文原名，也不让它冒充既有一格。
 * 不给默认选中，也不补「请选择」占位项：「表单不给默认、不预选」是伞票 07 的硬句，归表单票落。
 */
export function vocabularyOptions(
  codes: readonly string[],
  labels: Record<string, string>,
): VocabularyOption[] {
  return codes.map((code) => ({ value: code, label: labels[code] ?? code }));
}
