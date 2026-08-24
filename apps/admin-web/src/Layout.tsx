import { useState, type ComponentType } from 'react';
import { Package } from 'lucide-react';
import { Sidebar, useResize } from '@idpxyz/ui-workspace';
import { navigationSections, sidebarIconMap, pageTitleById } from './navigation';
import { Workbench } from './pages/Workbench';
import { UnwiredModule } from './pages/UnwiredModule';
import { ShipmentRequestPage } from './pages/shipment-request';
import { TemplatePreviewPage } from './pages/template-preview';
import {
  AcceptanceReviewPage,
  ExceptionTriagePage,
  ReconciliationPage,
  StageAdmissionPage,
} from './pages/governance';
import {
  PriceCardCatalogPage,
  ReferenceSeriesPage,
  PricingEvaluationsPage,
} from './pages/pricing';

// 导航 id → 页面组件。没登记的 id 落到 UnwiredModule 的诚实占位——
// 导航条目先于页面出现时，缺的是页面不是路由。
const pageById: Record<string, ComponentType> = {
  workbench: Workbench,
  'shipment-request': ShipmentRequestPage,
  'template-preview': TemplatePreviewPage,
  'acceptance-review': AcceptanceReviewPage,
  'exception-triage': ExceptionTriagePage,
  reconciliation: ReconciliationPage,
  'stage-admission': StageAdmissionPage,
  'price-card-catalog': PriceCardCatalogPage,
  'reference-series': ReferenceSeriesPage,
  'pricing-evaluation': PricingEvaluationsPage,
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
        <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
          {(() => {
            const ActivePage = pageById[active];
            return ActivePage ? <ActivePage /> : <UnwiredModule moduleId={active} />;
          })()}
        </div>
      </div>
    </div>
  );
}
