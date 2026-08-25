// 本目录 fetch 出口:服务产品与商业策略目录(ADR-0077、票 master-data-wiring/05)。
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
  | 'AS_OF_POLICY';

export interface AssembledRuleRecord {
  category: string;
  reference: string;
}

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

// 响应体按 kind 判别:五种册子的行形状互不相同(传输层注释原话),合成一个字段并集
// 会让页面在错误的形状上「读得通」。kind 由服务端随响应回显,这里以它作判别子。
export type CommercialPolicyListResponseBody =
  | { outcome: 'COMMERCIAL_POLICIES_LISTED'; kind: 'ACCEPTANCE_RULE_PACKAGE'; policies: RulePackageRecord[] }
  | { outcome: 'COMMERCIAL_POLICIES_LISTED'; kind: 'PRE_ACCEPTANCE_CONTROL'; policies: PreAcceptanceControlRecord[] }
  | { outcome: 'COMMERCIAL_POLICIES_LISTED'; kind: 'PRICE_POLICY'; policies: PricePolicyRecord[] }
  | { outcome: 'COMMERCIAL_POLICIES_LISTED'; kind: 'SETTLEMENT_POLICY'; policies: SettlementPolicyRecord[] }
  | { outcome: 'COMMERCIAL_POLICIES_LISTED'; kind: 'AS_OF_POLICY'; policies: AsOfPolicyRecord[] };

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
