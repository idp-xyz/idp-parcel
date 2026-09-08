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
