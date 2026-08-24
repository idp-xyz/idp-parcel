import { useState, type ComponentType } from 'react';
import { Package } from 'lucide-react';
import { Sidebar, useResize } from '@idpxyz/ui-workspace';
import { navigationSections, sidebarIconMap, pageTitleById } from './navigation';
import { Workbench } from './pages/Workbench';
import { UnwiredModule } from './pages/UnwiredModule';
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
} from './pages/visibility';
import { CustomsCasesPage, CustomsRestrictionsPage } from './pages/customs';
import {
  GroupLegalEntitiesPage,
  BusinessPartiesPage,
  PartyContractsPage,
  SupplierAgreementsPage,
  ServiceProductsPage,
  ChannelProductCatalogPage,
  CommercialPoliciesPage,
} from './pages/party';

// 导航 id → 页面组件。没登记的 id 落到 UnwiredModule 的诚实占位——
// 导航条目先于页面出现时，缺的是页面不是路由。
const pageById: Record<string, ComponentType> = {
  workbench: Workbench,
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
  'customs-cases': CustomsCasesPage,
  'customs-restrictions': CustomsRestrictionsPage,
  'group-legal-entities': GroupLegalEntitiesPage,
  'business-parties': BusinessPartiesPage,
  'party-contracts': PartyContractsPage,
  'supplier-agreements': SupplierAgreementsPage,
  'service-products': ServiceProductsPage,
  'channel-product-catalog': ChannelProductCatalogPage,
  'commercial-policies': CommercialPoliciesPage,
};

// 传统控制台外壳：品牌头 + 左侧导航 + 单页区，参考 idp-ui
// apps/loms-web 的 console/Layout；不引入标签页与底部/右侧面板，
// 等首个真实页面出现后再按实际交互决定是否升级形态。
export function Layout() {
  const [active, setActive] = useState<string>('workbench');
  const sidebarResize = useResize({
    direction: 'horizontal',
    initialSize: 240,
    minSize: 170,
    maxSize: 500,
  });

  return (
    <div className="h-screen w-screen flex flex-col bg-idpxyz-bg text-idpxyz-text overflow-hidden">
      <div className="h-12 flex items-center gap-2 px-4 border-b border-idpxyz-border bg-idpxyz-titleBar shrink-0">
        <Package className="h-5 w-5 text-idpxyz-accent" />
        <span className="text-[14px] font-bold text-idpxyz-textBright">IDP Parcel</span>
        <span className="text-[12px] text-idpxyz-textMuted">/ 租户管理台</span>
        <span className="ml-auto text-[12px] text-idpxyz-textMuted">
          {pageTitleById[active] || active}
        </span>
      </div>

      <div className="flex flex-1 overflow-hidden">
        <Sidebar
          width={sidebarResize.size}
          onFileClick={setActive}
          activeFile={active}
          navigationSections={navigationSections}
          iconMap={sidebarIconMap}
        />
        <div className="resize-handle-h" onMouseDown={sidebarResize.handleMouseDown} />
        {/* main 地标：读屏用户跳过导航直达页面内容的锚点。 */}
        <main className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
          {(() => {
            const ActivePage = pageById[active];
            return ActivePage ? <ActivePage /> : <UnwiredModule moduleId={active} />;
          })()}
        </main>
      </div>
    </div>
  );
}
