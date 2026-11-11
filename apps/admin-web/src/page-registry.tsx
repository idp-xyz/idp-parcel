import { lazy, type ComponentType } from 'react';

// 页面按域懒加载（票 admin-web-bundle-size/01）：每个域的 barrel 是一个懒加载块，入口块只剩外壳、工作台与模板。
// 同一个域的几页共用一个 import()，模块只取一次。别处要从某个域取非页面的东西（如 main.tsx 的 configure*Api）时
// 直接引那个模块、不经 barrel：入口一静态引 barrel，那个域就被钉回入口块。

interface DomainLoader<Module> {
  (): Promise<Module>;
  /** 块已取回时同步给出模块，没取回是 undefined。 */
  loaded(): Module | undefined;
}

/** 记下取回的模块，页面据此判断能否不挂起、直接渲染（见 `page`）。 */
function domain<Module>(importer: () => Promise<Module>): DomainLoader<Module> {
  let module: Module | undefined;
  const load = () =>
    importer().then((value) => {
      module = value;
      return value;
    });
  return Object.assign(load, { loaded: () => module });
}

const loadMyWork = domain(() => import('./pages/my-work'));
const loadShipmentRequest = domain(() => import('./pages/shipment-request'));
const loadChannelSelection = domain(() => import('./pages/channel-selection'));
const loadTemplatePreview = domain(() => import('./pages/template-preview'));
const loadGovernance = domain(() => import('./pages/governance'));
const loadPricing = domain(() => import('./pages/pricing'));
const loadNetwork = domain(() => import('./pages/network'));
const loadOperations = domain(() => import('./pages/operations'));
const loadSettlement = domain(() => import('./pages/settlement'));
const loadCollection = domain(() => import('./pages/collection'));
const loadVisibility = domain(() => import('./pages/visibility'));
const loadCustoms = domain(() => import('./pages/customs'));
const loadParty = domain(() => import('./pages/party'));
const loadWorkDevices = domain(() => import('./pages/work-devices'));
const loadOperatorAccess = domain(() => import('./pages/operator-access'));
const loadReferenceData = domain(() => import('./pages/reference-data'));
const loadDocumentNumbering = domain(() => import('./pages/document-numbering'));

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
  loadWorkDevices,
  loadReferenceData,
  loadDocumentNumbering,
];

// 导出名经属性访问取（`(m) => m.XxxPage`），拼错仍是 tsc 错误——不用字符串名查表。
//
// 块已经预取回来时不经 React.lazy：lazy 包装第一次渲染总要挂起一下（哪怕模块早已在手），而 React 19 让挂起后显出的
// 后备至少停留 300ms 才换成内容——预取过的页第一次打开照样先「正在加载」三百毫秒，页面的读请求也要等它挂上才发出
// （用户 2026-11-03 指出「首次打开公司主体感觉加载也很慢」，实测列表请求在点击后约 320ms 才发）。块还没取回时照旧走
// lazy，加载态与取不回来仍由 Layout 的 Suspense 与 PageChunkBoundary 接住。
// 同一页不会在两条路之间换类型而重挂：lazy 那条路的模块经 domain 的 then 取回，loaded 先于 lazy 落定，挂起后重渲时
// Page 已改走直接那条，lazy 那一层从未挂上；loaded 一旦有值也不再回到 undefined。
function page<Module>(load: DomainLoader<Module>, pick: (module: Module) => ComponentType): ComponentType {
  const Lazy = lazy(() => load().then((module) => ({ default: pick(module) })));
  return function Page() {
    const module = load.loaded();
    const Component = module === undefined ? Lazy : pick(module);
    return <Component />;
  };
}

/**
 * 首屏后空闲时把各域块预取回来：导航点击、命令面板、恢复标签与 hash 直达走的是同一条加载路径，预取之后切页不再等块。
 * 取回失败不在这里报——真打开那一页时由页面区的错误边界接住。
 */
export function prefetchPages(): void {
  for (const load of domainLoaders) void load().catch(() => undefined);
}

// 页面登记：导航 id → 页面组件的唯一映射。工作台不在此登记——它是外壳的
// 首页而非业务模块，由 Layout 直接渲染，不经本登记懒加载。没登记的 id 落
// UnwiredModule 诚实占位。
export const pageById: Record<string, ComponentType> = {
  'recent-objects': page(loadMyWork, (m) => m.RecentObjectsPage),
  'saved-views': page(loadMyWork, (m) => m.SavedViewsPage),
  'shipment-request': page(loadShipmentRequest, (m) => m.ShipmentRequestPage),
  'fba-shipment': page(loadShipmentRequest, (m) => m.FbaShipmentPage),
  'shipment-request-inquiry': page(loadShipmentRequest, (m) => m.ShipmentRequestListPage),
  'label-transactions': page(loadShipmentRequest, (m) => m.LabelTransactionsPage),
  'channel-selection-decisions': page(loadChannelSelection, (m) => m.ChannelSelectionDecisionsPage),
  'cancel-parcel': page(loadShipmentRequest, (m) => m.CancelParcelPage),
  'post-intake-disposition': page(loadShipmentRequest, (m) => m.PostIntakeDispositionPage),
  'template-preview': page(loadTemplatePreview, (m) => m.TemplatePreviewPage),
  'acceptance-review': page(loadShipmentRequest, (m) => m.AcceptanceReviewPage),
  'authorized-disposition': page(loadShipmentRequest, (m) => m.AuthorizedDispositionPage),
  'control-release-refusals': page(loadShipmentRequest, (m) => m.ControlReleaseRefusalsPage),
  'exception-triage': page(loadVisibility, (m) => m.ExceptionTriagePage),
  reconciliation: page(loadSettlement, (m) => m.ReconciliationPage),
  'supplier-bills': page(loadSettlement, (m) => m.SupplierBillsPage),
  'settlement-application': page(loadSettlement, (m) => m.SettlementApplicationPage),
  'stage-admission': page(loadGovernance, (m) => m.StageAdmissionPage),
  'price-card-catalog': page(loadPricing, (m) => m.PriceCardCatalogPage),
  'reference-series': page(loadPricing, (m) => m.ReferenceSeriesPage),
  'pricing-evaluation': page(loadPricing, (m) => m.PricingEvaluationsPage),
  'pricing-estimate': page(loadPricing, (m) => m.PricingEstimatePage),
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
  'pending-publications': page(loadParty, (m) => m.PendingPublicationsPage),
  'work-devices': page(loadWorkDevices, (m) => m.WorkDeviceCataloguePage),
  'operator-access': page(loadOperatorAccess, (m) => m.OperatorAccessPage),
  'reference-data': page(loadReferenceData, (m) => m.ReferenceDataPage),
  'document-numbering': page(loadDocumentNumbering, (m) => m.DocumentNumberingPage),
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
  'fba-shipment',
  'shipment-request-inquiry',
  'cancel-parcel',
  'post-intake-disposition',
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
  'pending-publications',
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
  'supplier-bills',
  'settlement-application',
  'operating-metrics',
  'pricing-evaluation',
  'pricing-estimate',
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
  'control-release-refusals',
  'label-transactions',
  'channel-selection-decisions',
  'work-devices',
  'operator-access',
  'reference-data',
  'document-numbering',
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
