import { useEffect, useRef, type KeyboardEvent as ReactKeyboardEvent, type ReactNode } from 'react';
import { ArrowDown, ArrowUp, ArrowUpDown, Inbox, RefreshCw, Search } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { useToast } from '@idpxyz/ui-theme-runtime';
import { useSplitResize, useWorkspaceTabPanel } from '@idpxyz/ui-workspace';
import { useInspector } from './inspector-context';
import { skeletonOf } from './loading-shape';
import { StateSlot, type StateSlotProps, type TemplateViewState } from './state-slot';
import { stepSelection, workbenchKeyAction, type WorkbenchFocus, type WorkbenchSort } from './workbench';

/** 命令头里的一格指标。可点即筛选，active 表示它正是当前筛选。custom 替换数值位（如进度条）。 */
export interface WorkbenchKpi {
  label: string;
  value: ReactNode;
  tone?: string;
  icon?: ReactNode;
  pulse?: boolean;
  active?: boolean;
  onClick?: () => void;
  custom?: ReactNode;
}

/** 工具条上的状态胶囊，带计数。 */
export interface WorkbenchChip {
  id: string;
  label: ReactNode;
  count?: number;
  active: boolean;
  onClick: () => void;
}

export interface WorkbenchColumn<Row> {
  key: string;
  header: ReactNode;
  /** 给了才能点表头排序；值交给 onSort。 */
  sortKey?: string;
  right?: boolean;
  /** 详情栏打开时收起的次要列，让列表在窄栏里仍读得清主身份与状态。 */
  hideWhenDetailOpen?: boolean;
  width?: string;
  render: (row: Row) => ReactNode;
}

export interface WorkbenchPageTemplateProps<Row> {
  title: string;
  subtitle?: string;
  kpis?: WorkbenchKpi[];
  /** 刷新钮；refreshing 由调用方按「有没有一问在途」给，收尾时模板报 toast。 */
  onRefresh?: () => void;
  refreshing?: boolean;
  primaryAction?: ReactNode;
  search: { value: string; onChange: (value: string) => void; placeholder?: string };
  statusChips?: WorkbenchChip[];
  extraFilters?: ReactNode;
  toolbarRight?: ReactNode;
  columns: WorkbenchColumn<Row>[];
  rows: Row[];
  rowKey: (row: Row) => string;
  selectedKey: string | null;
  onSelect: (key: string | null) => void;
  /** 需要人处理的行（橙底）；判据是业务决定，模板只摆。 */
  rowAttention?: (row: Row) => boolean;
  sort?: WorkbenchSort;
  onSort?: (key: string) => void;
  /** ready 态下筛没了时的那句话；与 viewState 的空态（册上本来就没有）是两件事。 */
  noMatchText: string;
  onClearFilters?: () => void;
  renderDetail: (key: string) => ReactNode;
  /** 四态由调用方注入：非 ready 只替换列表区，命令头与工具条保留。 */
  viewState: TemplateViewState;
  stateOverride?: StateSlotProps['override'];
}

// 主题色在 tailwind.config.js 里是裸 `var(--idpxyz-…)`，Tailwind 不给它们生成 `/NN` 透明度变体——不报错，类直接不存在，
// 边框随之落回 preflight 的浅灰（暗色主题下就是白线）。本文件一律用主题实色令牌（hover / activeItem / border）。
// 自己收 Esc 与方向键的部件（菜单上下选、弹层里 Esc 关弹层、下拉选项）；页签条单列，见 workbenchKeyAction。
const OVERLAY_SELECTOR = '[role="menu"],[role="dialog"],[role="listbox"],[role="combobox"]';

function focusOf(target: EventTarget | null): WorkbenchFocus {
  if (!(target instanceof HTMLElement)) return 'free';
  const tag = target.tagName;
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || target.isContentEditable) return 'typing';
  if (target.closest(OVERLAY_SELECTOR)) return 'overlay';
  if (target.closest('[role="tablist"]')) return 'tablist';
  return 'free';
}

/**
 * 单据工作台（票 admin-web-group-legal-entities/15）：形态照 idp-prism 采购订单页的 DocumentWorkbench——命令头（标题 +
 * 可点指标 + 刷新 + 主动作）· 工具条（`/` 聚焦的检索 + 带计数的状态胶囊）· 可拖分隔的主从分栏 · 表头点排序、关注行
 * 橙底、选中行左侧强调条 · ↑/↓ 换行、Esc 收详情。
 *
 * 与参照页的差别都是本仓规矩逼出来的：四态走 StateSlot（未配置 ≠ 暂无数据）；没有批量勾选列与分页位——今天接它的
 * 册既没有批量命令端点，读口也不分页，摆出来就是假动作；键盘只在本标签是工作区活动标签时生效，壳层会把非活动标签
 * 留在 DOM 里。行上不接双击：第一击开栏会收列、行跟着重排，第二击可能落到另一行上；开整页对象归详情栏的动作。
 */
export function WorkbenchPageTemplate<Row>({
  title,
  subtitle,
  kpis,
  onRefresh,
  refreshing = false,
  primaryAction,
  search,
  statusChips,
  extraFilters,
  toolbarRight,
  columns,
  rows,
  rowKey,
  selectedKey,
  onSelect,
  rowAttention,
  sort,
  onSort,
  noMatchText,
  onClearFilters,
  renderDetail,
  viewState,
  stateOverride,
}: WorkbenchPageTemplateProps<Row>) {
  const searchRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const split = useSplitResize('horizontal');
  const tabPanel = useWorkspaceTabPanel();
  const activeTab = tabPanel?.isActiveTab ?? true;
  const { addToast } = useToast();
  // 详情就在右栏，壳层的检查器栏在本页让位——否则右边常驻一条空栏。
  const inspector = useInspector();
  useEffect(() => inspector.yieldColumn(), [inspector]);
  const ready = viewState.kind === 'ready';
  const detailOpen = ready && selectedKey !== null && rows.some((row) => rowKey(row) === selectedKey);

  // 刷新收尾报一次 toast：只报用户按钮触发的那一次，且要先看到在途再看到结束，免得首取或别处触发的重取也冒一句。
  const refreshRequested = useRef(false);
  const refreshSeen = useRef(false);
  useEffect(() => {
    if (!refreshRequested.current) return;
    if (refreshing) {
      refreshSeen.current = true;
      return;
    }
    if (!refreshSeen.current) return;
    refreshRequested.current = false;
    refreshSeen.current = false;
    if (viewState.kind === 'ready' || viewState.kind === 'empty') {
      addToast({ type: 'success', title: '已刷新' });
    } else {
      addToast({
        type: 'error',
        title: '刷新未形成答案',
        message: 'description' in viewState ? viewState.description : undefined,
      });
    }
  }, [refreshing, viewState, addToast]);

  const handleRefresh = () => {
    if (!onRefresh || refreshing) return;
    refreshRequested.current = true;
    refreshSeen.current = false;
    onRefresh();
  };

  const keys = rows.map(rowKey);
  const keysRef = useRef(keys);
  keysRef.current = keys;
  useEffect(() => {
    if (!activeTab) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.defaultPrevented) return;
      const action = workbenchKeyAction(event.key, focusOf(event.target), detailOpen);
      if (action === null) return;
      event.preventDefault();
      if (action === 'focus-search') searchRef.current?.focus();
      else if (action === 'close-detail') onSelect(null);
      else if (ready) {
        const next = stepSelection(keysRef.current, selectedKey, action);
        if (next !== null) onSelect(next);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [activeTab, selectedKey, detailOpen, onSelect, ready]);

  // 选中行换了就把它滚进可视区：方向键一路按下去不该把选中行按到屏幕外。
  useEffect(() => {
    if (selectedKey === null) return;
    const row = listRef.current?.querySelector(`[data-row-key="${CSS.escape(selectedKey)}"]`);
    row?.scrollIntoView({ block: 'nearest' });
  }, [selectedKey]);

  const visibleColumns = columns.filter((column) => !(detailOpen && column.hideWhenDetailOpen));

  const SortIcon = ({ sortKey }: { sortKey: string }) =>
    !sort || sort.key !== sortKey ? (
      <ArrowUpDown className="h-3 w-3 opacity-40" aria-hidden="true" />
    ) : sort.dir === -1 ? (
      <ArrowDown className="h-3 w-3" aria-hidden="true" />
    ) : (
      <ArrowUp className="h-3 w-3" aria-hidden="true" />
    );

  const onRowKeyDown = (event: ReactKeyboardEvent<HTMLTableRowElement>, key: string) => {
    if (event.target !== event.currentTarget || event.key !== 'Enter') return;
    event.preventDefault();
    onSelect(selectedKey === key ? null : key);
  };

  const table =
    rows.length === 0 ? (
      <div className="flex flex-col items-center justify-center py-16 text-idpxyz-textMuted">
        <Inbox className="mb-3 h-8 w-8 opacity-50" aria-hidden="true" />
        <p className="text-[13px]">{noMatchText}</p>
        {onClearFilters && (
          <Button variant="ghost" size="sm" className="mt-2 text-[11px]" onClick={onClearFilters}>
            清除筛选
          </Button>
        )}
      </div>
    ) : (
      <table className="w-full text-[12px]">
        <thead className="sticky top-0 z-10 bg-idpxyz-sidebar">
          <tr className="border-b border-idpxyz-border text-left text-idpxyz-textMuted">
            {visibleColumns.map((column, index) => (
              <th
                key={column.key}
                scope="col"
                className={`px-3 py-2 font-medium ${index === 0 ? 'pl-6' : ''} ${column.right ? 'text-right' : ''} ${
                  index === visibleColumns.length - 1 ? 'pr-6' : ''
                }`}
                style={column.width ? { width: column.width } : undefined}
                aria-sort={
                  column.sortKey && sort?.key === column.sortKey
                    ? sort.dir === 1
                      ? 'ascending'
                      : 'descending'
                    : undefined
                }
              >
                {column.sortKey && onSort ? (
                  <button
                    type="button"
                    onClick={() => onSort(column.sortKey!)}
                    className={`inline-flex items-center gap-1 hover:text-idpxyz-textBright ${
                      column.right ? 'w-full flex-row-reverse justify-start' : ''
                    }`}
                  >
                    {column.header}
                    <SortIcon sortKey={column.sortKey} />
                  </button>
                ) : (
                  column.header
                )}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => {
            const key = rowKey(row);
            const selected = selectedKey === key;
            const attention = rowAttention?.(row) ?? false;
            return (
              <tr
                key={key}
                data-row-key={key}
                tabIndex={0}
                aria-selected={selected}
                onClick={() => onSelect(selected ? null : key)}
                onKeyDown={(event) => onRowKeyDown(event, key)}
                className={`cursor-pointer border-b border-idpxyz-border transition-colors focus-visible:outline focus-visible:outline-1 focus-visible:-outline-offset-1 focus-visible:outline-idpxyz-accent ${
                  selected
                    ? 'bg-idpxyz-activeItem shadow-[inset_2px_0_0_0_var(--idpxyz-accent)]'
                    : attention
                      ? 'bg-orange-500/5 hover:bg-orange-500/10'
                      : 'hover:bg-idpxyz-hover'
                }`}
              >
                {visibleColumns.map((column, index) => (
                  <td
                    key={column.key}
                    className={`px-3 py-2.5 ${index === 0 ? 'pl-6' : ''} ${column.right ? 'text-right' : ''} ${
                      index === visibleColumns.length - 1 ? 'pr-6' : ''
                    }`}
                  >
                    {column.render(row)}
                  </td>
                ))}
              </tr>
            );
          })}
        </tbody>
      </table>
    );

  return (
    <div className="relative flex flex-1 flex-col overflow-hidden bg-idpxyz-editor">
      <div className="flex shrink-0 items-center gap-5 border-b border-idpxyz-border px-6 py-3">
        <div className="min-w-0 shrink-0">
          <h1 className="text-[16px] font-semibold leading-tight text-idpxyz-textBright">{title}</h1>
          {subtitle && <p className="truncate text-[11px] text-idpxyz-textMuted">{subtitle}</p>}
        </div>
        {kpis && kpis.length > 0 && (
          <>
            <div className="hidden h-7 w-px shrink-0 bg-idpxyz-border md:block" />
            <div className="hidden min-w-0 items-center gap-4 overflow-x-auto md:flex">
              {kpis.map((kpi) =>
                kpi.custom != null ? (
                  <div key={kpi.label} className="flex flex-col gap-0.5 px-1">
                    <span className="whitespace-nowrap text-[9px] uppercase tracking-wide text-idpxyz-textMuted">
                      {kpi.label}
                    </span>
                    {kpi.custom}
                  </div>
                ) : (
                  <button
                    key={kpi.label}
                    type="button"
                    onClick={kpi.onClick}
                    disabled={!kpi.onClick}
                    aria-pressed={kpi.onClick ? kpi.active === true : undefined}
                    title={kpi.label}
                    className={`flex shrink-0 flex-col items-start gap-0.5 rounded px-1.5 py-0.5 transition-colors disabled:cursor-default ${
                      kpi.active ? 'bg-idpxyz-activeItem ring-1 ring-idpxyz-accent' : 'hover:bg-idpxyz-hover'
                    }`}
                  >
                    <span className="flex items-center gap-1 whitespace-nowrap text-[9px] uppercase leading-none tracking-wide text-idpxyz-textMuted">
                      {kpi.icon}
                      {kpi.label}
                    </span>
                    <span
                      className={`text-[16px] font-semibold leading-none ${kpi.tone ?? 'text-idpxyz-textBright'} ${
                        kpi.pulse ? 'animate-pulse' : ''
                      }`}
                    >
                      {kpi.value}
                    </span>
                  </button>
                ),
              )}
            </div>
          </>
        )}
        <div className="ml-auto flex shrink-0 items-center gap-2">
          {onRefresh && (
            <Button
              variant="ghost"
              size="icon"
              onClick={handleRefresh}
              disabled={refreshing}
              title={refreshing ? '刷新中…' : '刷新'}
              aria-label={refreshing ? '刷新中…' : '刷新'}
            >
              <RefreshCw className={`h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} aria-hidden="true" />
            </Button>
          )}
          {primaryAction}
        </div>
      </div>

      <div className="flex shrink-0 flex-wrap items-center gap-3 border-b border-idpxyz-border px-6 py-2.5">
        <div className="flex w-64 items-center gap-2 rounded border border-idpxyz-border bg-idpxyz-inputBg px-3 py-1.5 text-[12px]">
          <Search className="h-3.5 w-3.5 shrink-0 text-idpxyz-textMuted" aria-hidden="true" />
          <input
            ref={searchRef}
            type="text"
            value={search.value}
            onChange={(event) => search.onChange(event.target.value)}
            placeholder={search.placeholder ?? '搜索…'}
            aria-label={search.placeholder ?? '搜索'}
            className="min-w-0 flex-1 bg-transparent text-idpxyz-text placeholder-idpxyz-textMuted outline-none"
            onKeyDown={(event) => event.key === 'Escape' && event.currentTarget.blur()}
          />
          <kbd className="rounded border border-idpxyz-border px-1 text-[9px] text-idpxyz-textMuted">/</kbd>
        </div>
        {statusChips && statusChips.length > 0 && (
          <div className="flex flex-wrap items-center gap-1">
            {statusChips.map((chip) => (
              <button
                key={chip.id}
                type="button"
                onClick={chip.onClick}
                aria-pressed={chip.active}
                className={`rounded px-2 py-1 text-[10px] transition-colors ${
                  chip.active
                    ? 'bg-idpxyz-accent text-white'
                    : 'text-idpxyz-textMuted hover:bg-idpxyz-hover hover:text-idpxyz-text'
                }`}
              >
                {chip.label}
                {chip.count != null && <span className="opacity-70"> {chip.count}</span>}
              </button>
            ))}
          </div>
        )}
        {extraFilters}
        {toolbarRight && <span className="ml-auto text-[11px] text-idpxyz-textMuted">{toolbarRight}</span>}
      </div>

      <div ref={split.containerRef} className="flex min-h-0 flex-1 overflow-hidden">
        <div
          data-pane="list"
          className="flex min-h-0 min-w-0 flex-col"
          style={detailOpen ? { flex: `0 0 ${split.ratio}%` } : { flex: '1 1 100%' }}
        >
          <div ref={listRef} className="min-h-0 flex-1 overflow-auto">
            {ready ? (
              table
            ) : viewState.kind === 'loading' && skeletonOf(viewState.shape) === 'table' ? (
              // 加载骨架摆成表的样子：顶对齐、列数随真表、首格与真表 pl-6 对齐（骨架格自带 px-3）。居中放会让几行
              // 骨架浮在页面中下部，上面一大片空，读起来像页面坏了。
              <div className="px-3">
                <StateSlot state={{ ...viewState, cols: viewState.cols ?? visibleColumns.length }} override={stateOverride} />
              </div>
            ) : (
              <div className="flex h-full items-center justify-center">
                <StateSlot state={viewState} override={stateOverride} />
              </div>
            )}
          </div>
        </div>
        {detailOpen && selectedKey !== null && (
          <>
            <div
              className="resize-handle-h"
              role="separator"
              aria-orientation="vertical"
              aria-label="拖动调整列表与详情的宽度"
              onMouseDown={split.handleMouseDown}
            />
            <div data-pane="detail" className="h-full min-h-0 overflow-hidden" style={{ flex: '1 1 0%' }}>
              {renderDetail(selectedKey)}
            </div>
          </>
        )}
      </div>
    </div>
  );
}
