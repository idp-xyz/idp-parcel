import type { ReactNode } from 'react';
import { ChevronDown, CircleUser, Landmark, Moon, Package, Rows3, Rows4, Search, Sun } from 'lucide-react';
import { Button, DropdownMenu, MenuItem, MenuLabel, MenuSeparator, Tag, Tooltip } from '@idpxyz/ui-primitives';
import { useDensity, useTheme } from '@idpxyz/ui-theme-runtime';
import {
  ATTRIBUTION_SEPARATOR,
  GLOBAL_SEARCH_LABEL,
  GLOBAL_SEARCH_UNAVAILABLE_REASON,
  PRODUCT_ATTRIBUTION,
  SCOPE_CHIP_HINT,
  SIGN_OUT_LABEL,
  densityToggleLabel,
  scopeChipLabel,
  themeToggleLabel,
  userMenuLabel,
} from './top-bar-model';

// 顶栏（票 admin-web-ux-alignment/01）：手册「顶栏规范」把它定为全局能力的固定位置，黄金标准「Top Bar 黄金标准」要求
// 高度固定、全产品位置一致——所以它从 Layout 里抽成自己的件，页面层不再各自长头部。高度沿 loms-web console/Layout
// 的品牌头：h-12（48px），本票不动。
//
// 左侧是手册「Top Bar 归属信息规范」的 `{产品} / IDP · {模块名称}`：归属两段弱化色、模块名称主色。模块名称就是当前页名，
// 由 Layout 从 pageTitleById 取了传进来——顶栏不认识路由，只认字。产品图标沿用 Package。
//
// 右侧四个位从左到右：全局搜索、作用域、主题 + 密度切换、用户菜单。手册「顶栏规范」里的「Alerts / Notifications」
// 故意没有：没有面向 UI 的通知读口，一只永远为 0 的铃铛是假位，spec「红线」只允许禁用态 + 说明、不允许假动作假计数。
// 全局搜索与作用域今天都是「留位」——黄金标准要功能未完整时也保留视觉占位与结构位置，位留下了，动作没有编。
//
// 主题与密度直接读上游两个 Provider 的 hook，不经 props：它们是壳层自己的全局开关，Layout 不该替它转手。默认值与持久化
// 归 shell/preference-sync，这里只切。

export interface TopBarProps {
  /** 模块名称：当前页在 pageTitleById 里的名字，归属信息的第三段。 */
  moduleTitle: string;
  /** 会话主体的显示名。auth/oidc.ts 的读口是异步的，读到之前为 undefined，触发按钮那一段就先不显。 */
  principal?: string;
  /**
   * 可显示的租户名。今天没有任何读口告诉前端本会话绑的是哪个租户——ADR-0100 把绑定放在服务端的操作者册，id_token 不带
   * 租户声明——Layout 因此不传，chip 显「由服务端按会话判定」。位与文案已就位，等读口。
   */
  tenantName?: string;
  /** 「退出」走 auth/oidc.ts 既有的 logout，这里不自己清会话。 */
  onSignOut: () => void;
}

export function TopBar({ moduleTitle, principal, tenantName, onSignOut }: TopBarProps) {
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

      <div className="ml-auto flex shrink-0 items-center gap-2">
        <GlobalSearchSlot />
        <ScopeChip tenantName={tenantName} />
        <ThemeToggle />
        <DensityToggle />
        <UserMenu principal={principal} onSignOut={onSignOut} />
      </div>
    </header>
  );
}

/**
 * 全局搜索位。上游 ui-workspace 的 TitleBar 里这一位也是一个按钮形的外壳（点了才弹 CommandPalette），这里照那个形：
 * 一个不做事的按钮 + 说明。用 aria-disabled 而不用原生 disabled——原生 disabled 的按钮不发指针与焦点事件，Tooltip 永远
 * 弹不出来，「为什么不能用」就没人看得到；aria-disabled 让读屏念出「不可用」，鼠标与键盘都还能停上去看说明。
 */
function GlobalSearchSlot() {
  return (
    <Tooltip content={GLOBAL_SEARCH_UNAVAILABLE_REASON} side="bottom">
      <Button
        type="button"
        variant="outline"
        aria-label={GLOBAL_SEARCH_LABEL}
        aria-disabled="true"
        className="w-[240px] cursor-not-allowed justify-start gap-1.5 font-normal opacity-70 hover:bg-transparent hover:text-idpxyz-textMuted"
      >
        <Search className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
        <span className="truncate">{GLOBAL_SEARCH_LABEL}</span>
      </Button>
    </Tooltip>
  );
}

/** 作用域只读 chip：不是切换器，所以是 Tag 不是按钮；悬停说明为什么没有切换。 */
function ScopeChip({ tenantName }: { tenantName?: string }) {
  return (
    <Tooltip content={SCOPE_CHIP_HINT} side="bottom">
      <Tag variant="outline" size="md" className="whitespace-nowrap hover:shadow-none">
        <Landmark className="h-3 w-3 shrink-0" aria-hidden="true" />
        {scopeChipLabel(tenantName)}
      </Tag>
    </Tooltip>
  );
}

/**
 * 图标按钮：aria-label 与 Tooltip 用同一句（手册「可访问性规范」——图标按钮有 accessible name），两处不会各自漂。
 * Tooltip 的触发器 asChild 落到 Button 上，Button 是 forwardRef 且把 props 全透传给 <button>，Radix 要挂的事件与 ref 都到得了。
 */
function IconButton({ label, onClick, children }: { label: string; onClick: () => void; children: ReactNode }) {
  return (
    <Tooltip content={label} side="bottom">
      <Button type="button" variant="ghost" size="icon" aria-label={label} onClick={onClick}>
        {children}
      </Button>
    </Tooltip>
  );
}

function ThemeToggle() {
  const { theme, toggleTheme } = useTheme();
  return (
    <IconButton label={themeToggleLabel(theme)} onClick={toggleTheme}>
      {theme === 'dark' ? <Sun className="h-4 w-4" aria-hidden="true" /> : <Moon className="h-4 w-4" aria-hidden="true" />}
    </IconButton>
  );
}

function DensityToggle() {
  const { density, toggleDensity } = useDensity();
  return (
    <IconButton label={densityToggleLabel(density)} onClick={toggleDensity}>
      {density === 'compact' ? <Rows4 className="h-4 w-4" aria-hidden="true" /> : <Rows3 className="h-4 w-4" aria-hidden="true" />}
    </IconButton>
  );
}

/**
 * 用户菜单。触发按钮带可见的主体名而不是纯图标：ui-primitives 的 Tooltip 与 DropdownMenu 各自把 Radix 的 Root + Trigger
 * 包成一个件、两个 Trigger 的 asChild 没法套在同一个 button 上（外层 Trigger 的 props 会被内层的包装件吞掉），所以这一位不走
 * 「图标 + Tooltip」，走上游 TitleBar 用户按钮的形：图标 + 名字 + 下拉箭头，名字本身就是说明。
 * 主体名未到（读口异步）的那一拍只有图标，aria-label 仍是「用户菜单」。
 */
function UserMenu({ principal, onSignOut }: { principal?: string; onSignOut: () => void }) {
  return (
    <DropdownMenu
      align="right"
      trigger={
        <Button type="button" variant="ghost" aria-label={userMenuLabel(principal)} className="gap-1.5 px-2">
          <CircleUser className="h-4 w-4 shrink-0" aria-hidden="true" />
          {principal ? <span className="max-w-[160px] truncate text-[11px]">{principal}</span> : null}
          <ChevronDown className="h-3 w-3 shrink-0" aria-hidden="true" />
        </Button>
      }
    >
      {principal ? (
        <>
          <MenuLabel className="max-w-[280px] truncate">{principal}</MenuLabel>
          <MenuSeparator />
        </>
      ) : null}
      <MenuItem destructive onSelect={() => onSignOut()}>
        {SIGN_OUT_LABEL}
      </MenuItem>
    </DropdownMenu>
  );
}
