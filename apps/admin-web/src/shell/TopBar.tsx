import { Package } from 'lucide-react';
import { ATTRIBUTION_SEPARATOR, PRODUCT_ATTRIBUTION } from './top-bar-model';

// 顶栏（票 admin-web-ux-alignment/01）：手册「顶栏规范」把它定为全局能力的固定位置，黄金标准「Top Bar 黄金标准」要求
// 高度固定、全产品位置一致——所以它从 Layout 里抽成自己的件，页面层不再各自长头部。高度沿 loms-web console/Layout
// 的品牌头：h-12（48px），本票不动。
//
// 左侧是手册「Top Bar 归属信息规范」的 `{产品} / IDP · {模块名称}`：归属两段弱化色、模块名称主色。模块名称就是当前页名，
// 由 Layout 从 pageTitleById 取了传进来——顶栏不认识路由，只认字。产品图标沿用 Package。

export interface TopBarProps {
  /** 模块名称：当前页在 pageTitleById 里的名字，归属信息的第三段。 */
  moduleTitle: string;
}

export function TopBar({ moduleTitle }: TopBarProps) {
  return (
    <header className="h-12 flex items-center gap-2 px-4 border-b border-idpxyz-border bg-idpxyz-titleBar shrink-0">
      <Package className="h-5 w-5 shrink-0 text-idpxyz-accent" aria-hidden="true" />
      <div className="flex min-w-0 items-baseline gap-1.5">
        <span className="whitespace-nowrap text-[12px] text-idpxyz-textMuted">{PRODUCT_ATTRIBUTION}</span>
        {/* 分隔符只是视觉断句，读屏念「Parcel / IDP 工作台」比念出一个间隔号清楚。 */}
        <span className="text-[12px] text-idpxyz-textMuted" aria-hidden="true">
          {ATTRIBUTION_SEPARATOR}
        </span>
        <span className="truncate text-[14px] font-bold text-idpxyz-textBright">{moduleTitle}</span>
      </div>
    </header>
  );
}
