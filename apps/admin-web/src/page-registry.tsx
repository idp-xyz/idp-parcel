import type { ComponentType } from 'react';
import {
  ShipmentRequestPage,
  ShipmentRequestListPage,
  CancelParcelPage,
  LabelTransactionsPage,
} from './pages/shipment-request';
import { TemplatePreviewPage } from './pages/template-preview';
import {
  AcceptanceReviewPage,
  ExceptionTriagePage,
  ReconciliationPage,
  SettlementApplicationPage,
  StageAdmissionPage,
} from './pages/governance';
import {
  PriceCardCatalogPage,
  ReferenceSeriesPage,
  PricingEvaluationsPage,
} from './pages/pricing';
import {
  NetworkCatalogPage,
  ServiceAreasPage,
  RoutePlansPage,
} from './pages/network';
import {
  NodeOperationsReviewPage,
  TransportFulfillmentReviewPage,
} from './pages/operations';
import { ChargesBillingPage, OperatingMetricsPage } from './pages/settlement';
import { CodLedgerPage } from './pages/collection';
import {
  TrackingProjectionPage,
  ExceptionCasesPage,
  ClaimsRecoveryPage,
  TrackingJudgmentRulesPage,
  DisclosurePoliciesPage,
  ClaimPrerequisitesPage,
} from './pages/visibility';
import {
  CustomsCasesPage,
  CustomsRestrictionsPage,
  CustomsPortsPathsPage,
  ComplianceRulesPage,
} from './pages/customs';
import {
  GroupLegalEntitiesPage,
  BusinessPartiesPage,
  PartyContractsPage,
  SupplierAgreementsPage,
  ServiceProductsPage,
  ChannelProductCatalogPage,
  CommercialPoliciesPage,
} from './pages/party';

// 页面登记：导航 id → 页面组件的唯一映射。工作台不在此登记——它是外壳的
// 总览首页而非业务模块，由 Layout 直接渲染；这样工作台可以反向读取本登记
// 派生就绪度总览而不形成模块环。没登记的 id 落 UnwiredModule 诚实占位。
export const pageById: Record<string, ComponentType> = {
  'shipment-request': ShipmentRequestPage,
  'shipment-request-inquiry': ShipmentRequestListPage,
  'label-transactions': LabelTransactionsPage,
  'cancel-parcel': CancelParcelPage,
  'template-preview': TemplatePreviewPage,
  'acceptance-review': AcceptanceReviewPage,
  'exception-triage': ExceptionTriagePage,
  reconciliation: ReconciliationPage,
  'settlement-application': SettlementApplicationPage,
  'stage-admission': StageAdmissionPage,
  'price-card-catalog': PriceCardCatalogPage,
  'reference-series': ReferenceSeriesPage,
  'pricing-evaluation': PricingEvaluationsPage,
  'network-catalog': NetworkCatalogPage,
  'service-areas': ServiceAreasPage,
  'route-plans': RoutePlansPage,
  'node-operations-review': NodeOperationsReviewPage,
  'transport-fulfillment-review': TransportFulfillmentReviewPage,
  'charges-billing': ChargesBillingPage,
  'operating-metrics': OperatingMetricsPage,
  'cod-ledger': CodLedgerPage,
  'tracking-projection': TrackingProjectionPage,
  'exception-cases': ExceptionCasesPage,
  'claims-recovery': ClaimsRecoveryPage,
  'tracking-judgment-rules': TrackingJudgmentRulesPage,
  'disclosure-policies': DisclosurePoliciesPage,
  'claim-prerequisites': ClaimPrerequisitesPage,
  'customs-cases': CustomsCasesPage,
  'customs-restrictions': CustomsRestrictionsPage,
  'customs-ports-paths': CustomsPortsPathsPage,
  'compliance-rules': ComplianceRulesPage,
  'group-legal-entities': GroupLegalEntitiesPage,
  'business-parties': BusinessPartiesPage,
  'party-contracts': PartyContractsPage,
  'supplier-agreements': SupplierAgreementsPage,
  'service-products': ServiceProductsPage,
  'channel-product-catalog': ChannelProductCatalogPage,
  'commercial-policies': CommercialPoliciesPage,
};

/**
 * 已接线模块：页面对 parcel-api 真实端点发请求（不含演示）。
 * 这是接线事实的登记处：某页从骨架转接线时在此登记，工作台总览随之变档，
 * 不在页面里另写第二份状态。
 */
export const liveIds: ReadonlySet<string> = new Set([
  'shipment-request',
  'shipment-request-inquiry',
  'cancel-parcel',
  'tracking-projection',
  'price-card-catalog',
  'reference-series',
  'network-catalog',
  'service-areas',
  'compliance-rules',
  'customs-cases',
  'customs-restrictions',
  'service-products',
  'commercial-policies',
  'party-contracts',
  'supplier-agreements',
  'tracking-judgment-rules',
  'disclosure-policies',
  'claim-prerequisites',
]);

/**
 * 演示模块：功能完整但数据为隔离合成 S，不承载业务语义。
 */
export const demoIds: ReadonlySet<string> = new Set(['template-preview']);

export type ModuleReadiness = 'live' | 'skeleton' | 'demo' | 'planned';

/** 按登记机制派生模块就绪度：占位=无页面，接线/演示=显式登记，其余为骨架。 */
export function readinessOf(id: string): ModuleReadiness {
  if (!pageById[id]) return 'planned';
  if (liveIds.has(id)) return 'live';
  if (demoIds.has(id)) return 'demo';
  return 'skeleton';
}
