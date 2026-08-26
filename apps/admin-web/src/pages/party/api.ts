// 本目录 fetch 出口:服务产品、商业策略与商业关系载体目录(ADR-0077;票
// master-data-wiring/05 与 admin-web-page-wiring-frontier/01)。
// 传输与五格判读收敛在共享 catalogue-api,本文件只保留本上下文的类型与查询函数。

import { exchangeMasterData, type ApiResult } from '../catalogue-api';

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
  | 'AUTHORIZATION_RULE';

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

// 响应体按 kind 判别:六种册子的行形状互不相同(传输层注释原话),合成一个字段并集
// 会让页面在错误的形状上「读得通」。kind 由服务端随响应回显,这里以它作判别子。
export type CommercialPolicyListResponseBody =
  | { outcome: 'COMMERCIAL_POLICIES_LISTED'; kind: 'ACCEPTANCE_RULE_PACKAGE'; policies: RulePackageRecord[] }
  | { outcome: 'COMMERCIAL_POLICIES_LISTED'; kind: 'PRE_ACCEPTANCE_CONTROL'; policies: PreAcceptanceControlRecord[] }
  | { outcome: 'COMMERCIAL_POLICIES_LISTED'; kind: 'PRICE_POLICY'; policies: PricePolicyRecord[] }
  | { outcome: 'COMMERCIAL_POLICIES_LISTED'; kind: 'SETTLEMENT_POLICY'; policies: SettlementPolicyRecord[] }
  | { outcome: 'COMMERCIAL_POLICIES_LISTED'; kind: 'AS_OF_POLICY'; policies: AsOfPolicyRecord[] }
  | { outcome: 'COMMERCIAL_POLICIES_LISTED'; kind: 'AUTHORIZATION_RULE'; policies: AuthorizationRuleRecord[] };

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

// 供应商协议只有版本壳:供应商、采购价格条件与结算条件在领域对象上,但服务端没有
// 正文表可读(见后端 supplierAgreementBody 注释),因此这里也没有对应字段。
export interface SupplierAgreementRecord {
  objectId: string;
  version: string;
  scope: string;
  status: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  publishedAt: string;
}

export interface SupplierAgreementListResponseBody {
  outcome: 'SUPPLIER_AGREEMENTS_LISTED';
  agreements: SupplierAgreementRecord[];
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
