// 本目录 fetch 出口:服务产品、商业策略与商业关系载体目录(ADR-0077;票
// master-data-wiring/05 与 admin-web-page-wiring-frontier/01)。
// 传输与五格判读收敛在共享 catalogue-api,本文件只保留本上下文的类型与查询函数。

import { exchangeMasterData, postMasterData, type ApiResult } from '../catalogue-api';
import type { RegistrationResponseBody } from '../../components/registration';

export type { ApiResult } from '../catalogue-api';

export interface ServiceProductRecord {
  objectId: string;
  version: string;
  scope: string;
  status: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  publishedAt: string;
  form?: string;
}

export interface ServiceProductListResponseBody {
  outcome: 'SERVICE_PRODUCTS_LISTED';
  products: ServiceProductRecord[];
}

export type CommercialPolicyKind =
  | 'ACCEPTANCE_RULE_PACKAGE'
  | 'PRE_ACCEPTANCE_CONTROL'
  | 'PRICE_POLICY'
  | 'SETTLEMENT_POLICY'
  | 'AS_OF_POLICY'
  | 'AUTHORIZATION_RULE'
  | 'CREDIT_POLICY'
  | 'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY'
  | 'CUSTOMER_SERVICE_RULE';

export interface AssembledRuleRecord {
  category: string;
  reference: string;
}

export interface FinalRuleRecord {
  outcome: string;
  finalKind: string;
}

// 两个 *Declared 布尔与合同页的 contentRegistered 同款:未声明与「声明了但为空」都
// 表现为空数组,恢复动作却相反。allowedIntakeSources 与 intakeQualificationRefs 不并
// 成一栏——前者不允许空、后者允许显式空,两栏的「空」不是同一件事。
export interface RulePackageRecord {
  objectId: string;
  version: string;
  serviceProduct: string;
  contract: string;
  legalEntity: string;
  scope: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  declaredAt: string;
  rules: AssembledRuleRecord[];
  intakeQualificationDeclared: boolean;
  allowedIntakeSources: string[];
  intakeQualificationRefs: string[];
  finalRulesDeclared: boolean;
  finalRules: FinalRuleRecord[];
}

export interface PreAcceptanceControlRecord {
  contractObjectId: string;
  contractVersion: string;
  requirement: string;
  notApplicableBasis?: string;
  declaredAt: string;
}

export interface PricePolicyRecord {
  objectId: string;
  version: string;
  direction: string;
  planRef: string;
  planDirection: string;
  bindingConversion: string;
  policyScope: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  registeredAt: string;
  /**
   * 口径节（0022，Go `pricePolicyBody.caliberDeclared` / `caliber`，票 admin-write-faces/14）。布尔与节成对：0010 早于 0022，
   * 只有正文没有口径的行是合法状态，布尔让页面分得开「没登记口径」与「口径节缺了」。节里三处可缺的键是口径说出的真话
   * （不适用因而没有分类、采购方向因而没有系数、不涉外币因而没有汇率），缺键即「没有」，不是空串。
   */
  caliberDeclared: boolean;
  caliber?: PricePolicyCaliberRecord;
}

export interface PricePolicyCaliberRecord {
  taxDisposition: string;
  taxClassification?: string;
  volumetricFactor?: string;
  fx?: { quoteType: string; asOfSemantics: string; asOfPolicyVersion: string };
  registeredAt: string;
}

export interface SettlementPolicyRecord {
  objectId: string;
  version: string;
  method: string;
  legalEntity: string;
  counterparty: string;
  contractLabel: string;
  chargeScope: string;
  currency: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  registeredAt: string;
}

export interface AsOfPolicyRecord {
  rulePackageObjectId: string;
  rulePackageVersion: string;
  judgmentType: string;
  semanticsRef: string;
  policyVersion: string;
  declaredAt: string;
}

export interface CancellationAuthorityRecord {
  party: string;
  ruleReference: string;
}

// cancellationAuthorityDeclared 这个布尔在本族比别处更要紧:数组里少一个请求方**不是**
// 少一份声明,而是这份目录说出的真话(该请求方不许取消)。三态因此是「未声明 / 已声明
// 且该方允许 / 已声明但该方不许」,页面必须先看布尔才知道手上这份空缺属于哪一种。
export interface AuthorizationRuleRecord {
  objectId: string;
  version: string;
  scope: string;
  status: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  publishedAt: string;
  cancellationAuthorityDeclared: boolean;
  declaredAt?: string;
  cancellationAuthorities: CancellationAuthorityRecord[];
}

// 信用政策册（0020_credit_policy.sql）。额度两键**恰一在场**：金额行只有 limitMinor（最小货币
// 单位），比例行只有 limitRatioBasisPoints（基点）。后端用指针而不用 omitempty 的整数，是因为
// 零额度是合法声明（「授予零信用」）——前端同样不得拿 0 当缺席；两键都缺才是响应不合契约。
// 授权层级与费用类型是开放引用集，按原词展示。
export interface CreditPolicyRecord {
  objectId: string;
  version: string;
  legalEntity: string;
  authorityLevel: string;
  chargeType: string;
  limitMinor?: number;
  limitRatioBasisPoints?: number;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  registeredAt: string;
}

// 接受前财务控制策略册（0024，ADR-0115）。上列的是策略**版本壳**，正文左连接：contentRegistered
// 与合同页同款显式布尔——「壳在、正文不在」是合法状态（发布得出来、正文还没登），正是票
// admin-write-faces/06 立票时管理台看不见的那一格，页面不拿 content 的有无去推它。正文里的
// controls 一律在场且按判断顺序排列，至少一项由写入把守；控制项键名与受控 CLI 批文同名。
export interface PreAcceptanceControlItemRecord {
  control: string;
  chargeScope: string;
  order: number;
  onFailure: string;
  responsibility: string;
}

export interface PreAcceptanceFinancialControlPolicyRecord {
  objectId: string;
  version: string;
  scope: string;
  status: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  publishedAt: string;
  contentRegistered: boolean;
  content?: {
    jointPassCondition: string;
    registeredAt: string;
    controls: PreAcceptanceControlItemRecord[];
  };
}

// 客户服务规则册（0023，ADR-0104）。上列的是规则**版本壳**，正文左连接：contentRegistered 与接受前财务
// 控制策略册同款显式布尔——「壳在、正文不在」是合法状态，且正是 visibility-exception 点读答未登记、两维
// 停在未决的那个状态，页面不拿 content 的有无去推它。正文里 serviceProduct / customerContract **恰一在场**
// （后端 omitempty 让另一键不长出来）：同一个标识串作产品与作合同是两件事，前端不得把两键折成一格「对象」；
// 两键皆无或皆有是响应不合契约。两张子表一律在场——无客户差异的那一项是空数组，那是正文说出的真话，不是
// 缺键；两项合起来至少一项由写入把守。期限种类是封闭集（FIRST_CLAIM / MATERIAL_SUPPLEMENT / CONCLUSION_REVIEW），
// 起算事件、日历、索赔类型与材料条目都是引用（目录归 visibility-exception），按原词展示。
export interface ClaimDeadlineRecord {
  kind: string;
  startEvent: string;
  durationDays: number;
  calendar: string;
}

export interface MinimumMaterialsRecord {
  claimKind: string;
  materials: string[];
}

export interface CustomerServiceRuleRecord {
  objectId: string;
  version: string;
  scope: string;
  status: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  publishedAt: string;
  contentRegistered: boolean;
  content?: {
    serviceProduct?: string;
    customerContract?: string;
    responsibleParty: string;
    scope: string;
    registeredAt: string;
    claimDeadlines: ClaimDeadlineRecord[];
    minimumMaterials: MinimumMaterialsRecord[];
  };
}

// 响应体按 kind 判别:各册子的行形状互不相同(传输层注释原话),合成一个字段并集
// 会让页面在错误的形状上「读得通」。kind 由服务端随响应回显,这里以它作判别子。
export type CommercialPolicyListResponseBody =
  | { outcome: 'COMMERCIAL_POLICIES_LISTED'; kind: 'ACCEPTANCE_RULE_PACKAGE'; policies: RulePackageRecord[] }
  | { outcome: 'COMMERCIAL_POLICIES_LISTED'; kind: 'PRE_ACCEPTANCE_CONTROL'; policies: PreAcceptanceControlRecord[] }
  | { outcome: 'COMMERCIAL_POLICIES_LISTED'; kind: 'PRICE_POLICY'; policies: PricePolicyRecord[] }
  | { outcome: 'COMMERCIAL_POLICIES_LISTED'; kind: 'SETTLEMENT_POLICY'; policies: SettlementPolicyRecord[] }
  | { outcome: 'COMMERCIAL_POLICIES_LISTED'; kind: 'AS_OF_POLICY'; policies: AsOfPolicyRecord[] }
  | { outcome: 'COMMERCIAL_POLICIES_LISTED'; kind: 'AUTHORIZATION_RULE'; policies: AuthorizationRuleRecord[] }
  | { outcome: 'COMMERCIAL_POLICIES_LISTED'; kind: 'CREDIT_POLICY'; policies: CreditPolicyRecord[] }
  | {
      outcome: 'COMMERCIAL_POLICIES_LISTED';
      kind: 'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY';
      policies: PreAcceptanceFinancialControlPolicyRecord[];
    }
  | { outcome: 'COMMERCIAL_POLICIES_LISTED'; kind: 'CUSTOMER_SERVICE_RULE'; policies: CustomerServiceRuleRecord[] };

export interface ControlBindingRecord {
  chargeScope: string;
  policyId?: string;
  inapplicabilityBasis?: string;
}

// contentRegistered 是服务端给的显式布尔,页面不拿 bindings.length 去推它:**没登记
// 正文**与**登记了正文但零约定**都表现为空数组,而两者的恢复动作相反(前者去登记正文,
// 后者无事可做)。少了这个布尔,两态在页面上只能撞成同一句话。
export interface CustomerContractRecord {
  objectId: string;
  version: string;
  scope: string;
  status: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  publishedAt: string;
  contentRegistered: boolean;
  rulePackageId?: string;
  declaredAt?: string;
  bindings: ControlBindingRecord[];
}

export interface CustomerContractListResponseBody {
  outcome: 'CUSTOMER_CONTRACTS_LISTED';
  contracts: CustomerContractRecord[];
}

// 供应商协议册（0021 正文；读口逐键透出见后端 supplierAgreementBody 注释，票 admin-write-faces/19 在这里镜像）。
// 上列的是协议**版本壳**，正文左连接：contentRegistered 与合同页同款显式布尔——壳在正文缺是合法状态（壳可先
// 入册，正文随发布登记），页面不拿正文键的有无去推它：**没登记正文**与**登记了正文但某键为空**都可能表现为
// 键缺席，恢复动作相反（前者去发布正文，后者是响应不合契约、要查写侧），页面必须先看布尔。正文各键只在布尔
// 为真时在场（与 Go 侧 omitempty 同形）；agreementEffectiveEndsAt 在正文在场时也可缺，协议区间无上界是合法声明。
// 方向不在这里：领域恒为 BUY、库上不成列，后端刻意不透，前端转写一个常量等于为同一件事立第二个口径。
export interface SupplierAgreementRecord {
  objectId: string;
  version: string;
  scope: string;
  status: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  publishedAt: string;
  contentRegistered: boolean;
  supplier?: string;
  legalEntity?: string;
  purchasePlan?: string;
  agreementScope?: string;
  agreementEffectiveStartsAt?: string;
  agreementEffectiveEndsAt?: string;
  registeredAt?: string;
}

export interface SupplierAgreementListResponseBody {
  outcome: 'SUPPLIER_AGREEMENTS_LISTED';
  agreements: SupplierAgreementRecord[];
}

// 参与方身份两册（票 admin-remainder-mechanism-batch/01）。partyNameKnown 与合同页
// contentRegistered 同款显式布尔：法人钉着的参与方在册上查无此人是写入门失败才会
// 出现的悬空，页面按缺席如实显示，不拿空串去推、不补占位文本。
export interface GroupLegalEntityRecord {
  tenantId: string;
  legalEntityId: string;
  /** 封闭词转写：本册今天只有 RESPONSIBLE_LEGAL_ENTITY 一格（经营组织没有登记面）。 */
  kind: string;
  partyId: string;
  partyName?: string;
  partyNameKnown: boolean;
  /** 装载时点对生命周期事实的导出：REGISTERED / EFFECTIVE / DEACTIVATED。 */
  status: string;
  revision: number;
  basis: string;
  effectiveFrom: string;
  deactivatedAt?: string;
  deactivationBasis?: string;
  registeredAt: string;
}

export interface GroupLegalEntityListResponseBody {
  outcome: 'GROUP_LEGAL_ENTITIES_LISTED';
  entities: GroupLegalEntityRecord[];
}

// 方向由持有方→相对方的字段次序表达（CONTEXT：方向由「哪一方对哪一方持有该角色」
// 表达，不另设标志位）。status 是登记进来的关系状态事实（CANDIDATE/EFFECTIVE/
// EXPIRED/REVOKED/SUPERSEDED），不随装载时钟走。
export interface PartyRelationshipRecord {
  tenantId: string;
  relationshipId: string;
  revision: number;
  holderId: string;
  holderName?: string;
  holderNameKnown: boolean;
  counterpartyId: string;
  counterpartyName?: string;
  counterpartyNameKnown: boolean;
  role: string;
  scope: string;
  basis: string;
  status: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  endedAt?: string;
  endBasis?: string;
  successorId?: string;
  registeredAt: string;
}

export interface PartyRelationshipListResponseBody {
  outcome: 'PARTY_RELATIONSHIPS_LISTED';
  relationships: PartyRelationshipRecord[];
}

// 参与方身份本体册（票 admin-remainder-mechanism-batch/01 的补格裁定）。它与法人册、
// 关系册并列：一个参与方既可以不是法人、也可以不在任何关系里，被停用的那种恰恰如此，
// 少了本册，身份生命周期的「已登记」与「已停用」两格在管理台没有实例可显。
//
// status 按装载时点导出（REGISTERED / EFFECTIVE / DEACTIVATED）；停用两件只在已停用时
// 在场。这里没有 partyNameKnown：名称就在本册行上，不像法人与关系那样要左连接过来。
export interface BusinessPartyRecord {
  tenantId: string;
  partyId: string;
  partyName: string;
  status: string;
  revision: number;
  basis: string;
  effectiveFrom: string;
  deactivatedAt?: string;
  deactivationBasis?: string;
  registeredAt: string;
}

export interface BusinessPartyListResponseBody {
  outcome: 'BUSINESS_PARTIES_LISTED';
  parties: BusinessPartyRecord[];
}

// 货主客户账户册（票 admin-write-faces/04）——ADR-0003 三级边界的第三级。账户面向一个货主
// 客户建立、必须显式关联其客户参与方（PC CONTEXT），所以它与法人册同形而不与身份本体册
// 同形：名称不在本册行上，从参与方册左连接转写，customerPartyNameKnown 为假是写入门失败
// 才会出现的悬空引用。status 按装载时点导出（REGISTERED / EFFECTIVE / DEACTIVATED），
// 表上没有状态列。
export interface CustomerAccountRecord {
  tenantId: string;
  accountId: string;
  customerPartyId: string;
  customerPartyName?: string;
  customerPartyNameKnown: boolean;
  status: string;
  revision: number;
  basis: string;
  effectiveFrom: string;
  deactivatedAt?: string;
  deactivationBasis?: string;
  registeredAt: string;
}

export interface CustomerAccountListResponseBody {
  outcome: 'CUSTOMER_ACCOUNTS_LISTED';
  accounts: CustomerAccountRecord[];
}

// 产品—渠道映射册（票 admin-remainder-mechanism-batch/02）。channels 为空数组即显式
// 登记的“未配置”绑定——那是登记者说出的商业声明（该产品尚无可用渠道候选），不是
// 数据缺件，页面据此如实显示。行上没有状态字段：映射没有独立状态代数，是否参与新的
// 渠道决策由消费方对有效区间判断。
export interface ProductChannelMappingRecord {
  tenantId: string;
  mappingId: string;
  revision: number;
  productObjectId: string;
  productVersionLabel: string;
  channels: string[];
  basis: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  registeredAt: string;
}

export interface ProductChannelMappingListResponseBody {
  outcome: 'PRODUCT_CHANNEL_MAPPINGS_LISTED';
  mappings: ProductChannelMappingRecord[];
}

export function listServiceProducts(): Promise<ApiResult<ServiceProductListResponseBody>> {
  return exchangeMasterData<ServiceProductListResponseBody>('/commercial-service-products');
}

export function listCommercialPolicies(
  kind: CommercialPolicyKind,
): Promise<ApiResult<CommercialPolicyListResponseBody>> {
  return exchangeMasterData<CommercialPolicyListResponseBody>(
    `/commercial-policies?kind=${encodeURIComponent(kind)}`,
  );
}

// 合同与协议各走自己的路径,不并进 /commercial-policies 的 kind 分派:那个参数分的是
// 「商业规则与策略」一页里的五个页签,而这两样是管理台上两张独立的页(裁决与理由在
// 后端 query_commercial_relations.go 的文件注释)。
export function listCustomerContracts(): Promise<ApiResult<CustomerContractListResponseBody>> {
  return exchangeMasterData<CustomerContractListResponseBody>('/commercial-customer-contracts');
}

export function listSupplierAgreements(): Promise<ApiResult<SupplierAgreementListResponseBody>> {
  return exchangeMasterData<SupplierAgreementListResponseBody>('/commercial-supplier-agreements');
}

// 集团与法人、业务参与方各走自己的路径，判据与合同/协议同一条：管理台上两张独立的
// 页一页一入口，且两册的状态代数不同（身份状态按时点导出、关系状态是登记事实），
// 折进一个带 kind 的入口会让两种状态在同一响应形状里相互冒充。
export function listGroupLegalEntities(): Promise<ApiResult<GroupLegalEntityListResponseBody>> {
  return exchangeMasterData<GroupLegalEntityListResponseBody>('/commercial-group-legal-entities');
}

export function listPartyRelationships(): Promise<ApiResult<PartyRelationshipListResponseBody>> {
  return exchangeMasterData<PartyRelationshipListResponseBody>('/commercial-party-relationships');
}

export function listBusinessParties(): Promise<ApiResult<BusinessPartyListResponseBody>> {
  return exchangeMasterData<BusinessPartyListResponseBody>('/commercial-business-parties');
}

// 客户账户册与客户合同同页分签却各走自己的入口：合同上列的是商业版本壳（草稿→发布→
// 退役），账户上列的是参与方身份的登记修订（登记→生效→停用），两套状态代数不同——判据
// 与上面身份/关系两口分立那条同一句。
export function listCustomerAccounts(): Promise<ApiResult<CustomerAccountListResponseBody>> {
  return exchangeMasterData<CustomerAccountListResponseBody>('/commercial-customer-accounts');
}

// 映射目录不并进 /commercial-service-products：那边上列版本壳，这边上列登记册信封
// （产品×渠道×区间的修订），行形状与修订轴不同（后端读口注释同一条裁决）。
export function listProductChannelMappings(): Promise<
  ApiResult<ProductChannelMappingListResponseBody>
> {
  return exchangeMasterData<ProductChannelMappingListResponseBody>(
    '/commercial-product-channel-mappings',
  );
}

// ——以下为在线登记口（ADR-0085，票 admin-write-faces/02 商业片）。
//
// **今天这些请求必然答 403 ACCESS_CHANNEL_NOT_CONFIGURED**，那是诚实答案不是接线缺陷：
// 写准入不另立形（决定一），登记端点挂的是字面量 UnconfiguredIntake{}，与其余命令面同等
// `PAR-INT-01` 证据；墙降当天由装配点换真 Intake 即点亮，本文件一行不用改。
//
// **请求体形状此刻没有契约。** 决定三把「渠道原始载荷 → 登记快照」的翻译划给渠道接入
// 契约、随 `PAR-INT-01` 提供，所以这里不发明字段：页面收的是登记快照 JSON 本体，与受控
// 登记口 `parcel-commercial <子命令> -input` 吃的同一份形状，原样作请求体送出。真渠道
// 接线时以渠道契约为准重谈，不得反过来把这里当成已发布的 Schema。
//
// 与网络那一族还差一格：网络的在线口与 CLI 共用 registrationjson 那份译装，形状被编译期
// 钉住；商业的译装在 cmd/parcel-commercial 的 package main 里，两口只锁得到同一个登记
// 用例。快照形状对不对，今天只有 CLI 文档与人工核对在守（缺口记在票 02 的商业片
// Comment）。

/**
 * 登记种类封闭集。取值与传输层的端点构造函数一一对应；`publication` 之外的种类词与受控
 * CLI 的子命令同源。
 */
export type CommercialRegistrationKind =
  | 'publication'
  | 'business-party'
  | 'legal-entity'
  | 'customer-account'
  | 'party-relationship'
  | 'identity-deactivation'
  | 'service-product-form'
  | 'product-channel-mapping';

/**
 * 逐类登记端点。路径取「读口册名 + 该类种类词 + -registrations」，与网络、关务、VE
 * 三族同一条命名约定。
 *
 * 发布那一个是例外，取 `/commercial-publications` 不带 `-registrations`：本上下文的动词
 * 是发布，答案代数说的也是发布（`已发布已生效`/`已计划生效`），叫成登记会让它与身份、
 * 映射两族的登记答案混为一谈。它也只有一个端点而不是按对象类别铺一排——服务产品、
 * 规则包、合同、协议与各类策略是同一个发布用例的输入，类别在快照的版本规格里。
 */
export const commercialRegistrationEndpoints: Record<CommercialRegistrationKind, string> = {
  publication: '/commercial-publications',
  'business-party': '/commercial-business-party-registrations',
  'legal-entity': '/commercial-legal-entity-registrations',
  'customer-account': '/commercial-customer-account-registrations',
  'party-relationship': '/commercial-party-relationship-registrations',
  'identity-deactivation': '/commercial-party-identity-deactivations',
  'service-product-form': '/commercial-service-product-form-registrations',
  'product-channel-mapping': '/commercial-product-channel-mapping-registrations',
};

/**
 * 一类一个端点，本函数按种类取路径而不是裂成八个同形包装。
 *
 * 传输层那边逐类各立一个端点构造函数与一个 Intake 接口，为的是让「把一类的译装接到另一
 * 类的端点上」在编译期就红；那条保护在这里没有落点——快照本体在前端是未翻译的 JSON，
 * 分不分函数都一样送得出去。判据与网络页的 registerNetworkCatalogVersion 同一条。
 */
export function registerCommercial(
  kind: CommercialRegistrationKind,
  snapshot: unknown,
): Promise<ApiResult<RegistrationResponseBody>> {
  return postMasterData<RegistrationResponseBody>(commercialRegistrationEndpoints[kind], snapshot);
}

/**
 * 发布答案代数（`application.PublishCommercialAuthorityOutcome` 原名），逐格中文。
 *
 * 本表与下面的声明落点表由「商业规则与策略」页的受控发布签消费（票 admin-write-faces/03 的
 * 裁决：对象类别词表与该页的册词是两条分类轴，处置是把对应关系写在签上与册名旁，而不是对齐
 * 词表或另开纯写页）。两表跟着端点走而不是跟着页面走，所以留在这里。
 *
 * `已计划生效`单列而不并进`已发布已生效`：它入了册，但生效边界未开，**不得用于生产
 * 解析**——两格折成一句「已发布」，操作者会以为这一版此刻就在算数。
 *
 * `发布未决`是答案不是失败：草稿与来源原样保留、一个字节没写，续办是去确认批准角色或
 * 先发布被引对象；折成「提交失败」会让人以为重试有用。
 */
export const publicationOutcomeLabels: Record<string, string> = {
  PUBLISHED_EFFECTIVE: '已发布并已生效（生效边界已开，本版此刻算数）',
  PLANNED_EFFECTIVE: '已发布、已计划生效（边界未开，此刻还不参与生产解析）',
  REPLAYED: '同一份重放（原版本不被顶替，本次没有造第二个版本）',
  CONTENT_CONFLICT: '内容冲突（同键异内容，原版本不被顶替；改内容要发新版本号）',
  PENDING: '发布未决（批准角色未确认，或正文指名的对象尚未发布；一个字节没写）',
};

/**
 * 声明通道落点的逐格中文（`ports.DeclarationSaveOutcome` 原名）。
 *
 * 与 publicationOutcomeLabels 分表而不并进去：两套代数有重名格。`CONTENT_CONFLICT` 在
 * 版本那一栏说的是同键异内容、改内容要发新版本号；在声明这一栏说的是同一个拥有版本
 * 携带了不同正文——声明随发布固定，改声明必须发新版本，事后补不进去。并表会让其中
 * 一种顶着另一种的中文显示出来。
 */
export const declarationLandingLabels: Record<string, string> = {
  SAVED: '已登记',
  ALREADY_REGISTERED: '同拥有版本同正文重放（原正文不被顶替）',
  CONTENT_CONFLICT: '内容冲突（同拥有版本携带了不同正文；声明随发布固定，改声明要发新版本）',
};

/**
 * 参与方身份与关系的登记答案代数（`application.PartyRegistryOutcome` 原名）。
 *
 * `未找到`只出现在停用：要停用的身份从未登记。它不是「路由不存在」——能力在、册也在，
 * 登记方要去查的是册面。
 */
export const partyIdentityOutcomeLabels: Record<string, string> = {
  REGISTERED: '已登记（本次落库）',
  DEACTIVATED: '已停用（停用是修订链上新的一笔，原修订不被改写）',
  ALREADY_REGISTERED: '同键同内容重放（原修订不被顶替）',
  CONTENT_CONFLICT: '内容冲突（同修订号异内容；更正要占下一个修订号，不覆盖）',
  NOT_ACCEPTED: '受理门拒绝（修订错位或引用悬空，一个字节没写；原因随答复交回）',
  NOT_FOUND: '册上没有这一个身份（停用的对象从未登记）',
};

/** 服务形态与产品—渠道映射的登记答案代数（`application.ProductChannelOutcome` 原名）。 */
export const productChannelOutcomeLabels: Record<string, string> = {
  REGISTERED: '已登记（本次落库）',
  ALREADY_REGISTERED: '同键同内容重放（原修订不被顶替）',
  CONTENT_CONFLICT: '内容冲突（同修订号异内容；更正要占下一个修订号，不覆盖）',
  NOT_ACCEPTED: '受理门拒绝（产品版本悬空、已收尾或修订错位，一个字节没写；原因随答复交回）',
};
