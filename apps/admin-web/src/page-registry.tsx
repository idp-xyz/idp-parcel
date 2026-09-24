import { lazy, type ComponentType } from 'react';

// 页面按域懒加载（票 admin-web-bundle-size/01）：每个域的 barrel 是一个懒加载块，入口块只剩外壳、工作台与模板。
// 同一个域的几页共用一个 import()，模块只取一次。别处要从某个域取非页面的东西（如 main.tsx 的 configure*Api）时
// 直接引那个模块、不经 barrel：入口一静态引 barrel，那个域就被钉回入口块。
const loadMyWork = () => import('./pages/my-work');
const loadShipmentRequest = () => import('./pages/shipment-request');
const loadChannelSelection = () => import('./pages/channel-selection');
const loadTemplatePreview = () => import('./pages/template-preview');
const loadGovernance = () => import('./pages/governance');
const loadPricing = () => import('./pages/pricing');
const loadNetwork = () => import('./pages/network');
const loadOperations = () => import('./pages/operations');
const loadSettlement = () => import('./pages/settlement');
const loadCollection = () => import('./pages/collection');
const loadVisibility = () => import('./pages/visibility');
const loadCustoms = () => import('./pages/customs');
const loadParty = () => import('./pages/party');

const domainLoaders = [
  loadMyWork,
  loadShipmentRequest,
  loadChannelSelection,
  loadTemplatePreview,
  loadGovernance,
  loadPricing,
  loadNetwork,
  loadOperations,
  loadSettlement,
  loadCollection,
  loadVisibility,
  loadCustoms,
  loadParty,
];

// 导出名经属性访问取（`(m) => m.XxxPage`），拼错仍是 tsc 错误——不用字符串名查表。
function page<Module>(load: () => Promise<Module>, pick: (module: Module) => ComponentType): ComponentType {
  return lazy(() => load().then((module) => ({ default: pick(module) })));
}

/**
 * 首屏后空闲时把各域块预取回来：导航点击、命令面板、恢复标签与 hash 直达走的是同一条加载路径，预取之后切页不再等块。
 * 取回失败不在这里报——真打开那一页时由页面区的错误边界接住。
 */
export function prefetchPages(): void {
  for (const load of domainLoaders) void load().catch(() => undefined);
}

// 页面登记：导航 id → 页面组件的唯一映射。工作台不在此登记——它是外壳的
// 总览首页而非业务模块，由 Layout 直接渲染；这样工作台可以反向读取本登记
// 派生就绪度总览而不形成模块环。没登记的 id 落 UnwiredModule 诚实占位。
export const pageById: Record<string, ComponentType> = {
  'recent-objects': page(loadMyWork, (m) => m.RecentObjectsPage),
  'saved-views': page(loadMyWork, (m) => m.SavedViewsPage),
  'shipment-request': page(loadShipmentRequest, (m) => m.ShipmentRequestPage),
  'shipment-request-inquiry': page(loadShipmentRequest, (m) => m.ShipmentRequestListPage),
  'label-transactions': page(loadShipmentRequest, (m) => m.LabelTransactionsPage),
  'channel-selection-decisions': page(loadChannelSelection, (m) => m.ChannelSelectionDecisionsPage),
  'cancel-parcel': page(loadShipmentRequest, (m) => m.CancelParcelPage),
  'template-preview': page(loadTemplatePreview, (m) => m.TemplatePreviewPage),
  'acceptance-review': page(loadShipmentRequest, (m) => m.AcceptanceReviewPage),
  'authorized-disposition': page(loadShipmentRequest, (m) => m.AuthorizedDispositionPage),
  'exception-triage': page(loadVisibility, (m) => m.ExceptionTriagePage),
  reconciliation: page(loadSettlement, (m) => m.ReconciliationPage),
  'settlement-application': page(loadSettlement, (m) => m.SettlementApplicationPage),
  'stage-admission': page(loadGovernance, (m) => m.StageAdmissionPage),
  'price-card-catalog': page(loadPricing, (m) => m.PriceCardCatalogPage),
  'reference-series': page(loadPricing, (m) => m.ReferenceSeriesPage),
  'pricing-evaluation': page(loadPricing, (m) => m.PricingEvaluationsPage),
  'network-catalog': page(loadNetwork, (m) => m.NetworkCatalogPage),
  'service-areas': page(loadNetwork, (m) => m.ServiceAreasPage),
  'route-plans': page(loadNetwork, (m) => m.RoutePlansPage),
  'node-operations-review': page(loadOperations, (m) => m.NodeOperationsReviewPage),
  'transport-fulfillment-review': page(loadOperations, (m) => m.TransportFulfillmentReviewPage),
  'effective-time-judgment': page(loadOperations, (m) => m.EffectiveTimeJudgmentPage),
  'charges-billing': page(loadSettlement, (m) => m.ChargesBillingPage),
  'operating-metrics': page(loadSettlement, (m) => m.OperatingMetricsPage),
  'cod-ledger': page(loadCollection, (m) => m.CodLedgerPage),
  'tracking-projection': page(loadVisibility, (m) => m.TrackingProjectionPage),
  'exception-cases': page(loadVisibility, (m) => m.ExceptionCasesPage),
  'claims-recovery': page(loadVisibility, (m) => m.ClaimsRecoveryPage),
  'tracking-judgment-rules': page(loadVisibility, (m) => m.TrackingJudgmentRulesPage),
  'disclosure-policies': page(loadVisibility, (m) => m.DisclosurePoliciesPage),
  'claim-prerequisites': page(loadVisibility, (m) => m.ClaimPrerequisitesPage),
  'customs-cases': page(loadCustoms, (m) => m.CustomsCasesPage),
  'customs-restrictions': page(loadCustoms, (m) => m.CustomsRestrictionsPage),
  'customs-ports-paths': page(loadCustoms, (m) => m.CustomsPortsPathsPage),
  'compliance-rules': page(loadCustoms, (m) => m.ComplianceRulesPage),
  'group-legal-entities': page(loadParty, (m) => m.GroupLegalEntitiesPage),
  'business-parties': page(loadParty, (m) => m.BusinessPartiesPage),
  'party-contracts': page(loadParty, (m) => m.PartyContractsPage),
  'supplier-agreements': page(loadParty, (m) => m.SupplierAgreementsPage),
  'service-products': page(loadParty, (m) => m.ServiceProductsPage),
  'channel-product-catalog': page(loadParty, (m) => m.ChannelProductCatalogPage),
  'commercial-policies': page(loadParty, (m) => m.CommercialPoliciesPage),
};

/**
 * 已接线模块：页面已接到真实数据来源，数据区不再是未配置态或合成 S（不含演示）。
 * 来源有两种：业务模块页对 parcel-api 真实端点发请求；「我的工作」两页（recent-objects / saved-views）
 * 读的是本机浏览器的 localStorage——打开过的对象地址、存下的筛选态都是本机事实，页面已接到它唯一的真实来源，
 * 不计入已接线就会被判成骨架，而它们没有「未配置」可呈现。
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
  'customs-ports-paths',
  'service-products',
  'commercial-policies',
  'party-contracts',
  'supplier-agreements',
  'group-legal-entities',
  'business-parties',
  'channel-product-catalog',
  'tracking-judgment-rules',
  'disclosure-policies',
  'claim-prerequisites',
  'cod-ledger',
  'charges-billing',
  'reconciliation',
  'settlement-application',
  'operating-metrics',
  'pricing-evaluation',
  'route-plans',
  'stage-admission',
  'node-operations-review',
  'transport-fulfillment-review',
  'effective-time-judgment',
  'exception-triage',
  'exception-cases',
  'claims-recovery',
  'acceptance-review',
  'authorized-disposition',
  'label-transactions',
  'channel-selection-decisions',
  // 「我的工作」：来源是本机 localStorage 而非 parcel-api，理由见上方头注。
  'recent-objects',
  'saved-views',
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
