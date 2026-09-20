import { useState, type ReactNode } from 'react';
import {
  PageHeader,
  PageHeaderContent,
  PageHeaderTitle,
  PageHeaderDescription,
  PageHeaderActions,
  FilterBar,
  FilterSearch,
  FilterGroup,
} from '@idpxyz/ui-patterns';
import {
  Breadcrumb,
  Button,
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
  Pagination,
  Tooltip,
} from '@idpxyz/ui-primitives';
import { useDensity } from '@idpxyz/ui-theme-runtime';
import { navigationSections, pageTitleById } from '../navigation';
import { resolveBreadcrumb, type TemplateBreadcrumb } from './breadcrumb';
import { densityRowPadding, filterBarSlots, type FilterBarSlot } from './list-page-structure';
import { StateSlot, type TemplateViewState, type StateSlotProps } from './state-slot';

/** 列定义。render 拿整行而非取值路径，让调用方组合多字段（如单号+徽章）不求模板开洞。 */
export interface ListColumn<Row> {
  /** 列的稳定标识，用作 key；与数据字段名无绑定关系。 */
  id: string;
  header: ReactNode;
  align?: 'left' | 'center' | 'right';
  /** 附加到表头与单元格的 Tailwind 类（如宽度约束）。 */
  className?: string;
  render: (row: Row) => ReactNode;
}

export interface ListSearchProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
}

export interface ListPaginationProps {
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
  onPageSizeChange?: (pageSize: number) => void;
  pageSizeOptions?: number[];
}

export interface ListSortOption {
  value: string;
  /** 选项文案由调用方给全（含方向，如「提交时间 ↓」）：排序键是业务决定，模板不替它命名。 */
  label: string;
}

export interface ListSortProps {
  options: ListSortOption[];
  value: string;
  onChange: (value: string) => void;
}

export interface ListSavedView {
  id: string;
  label: string;
}

/**
 * 保存视图位。保存的是「当前筛选态」这件 UI 自身的事实，存哪、存多久归调用方（票 admin-web-ux-alignment/02 落地后
 * 各页按需接）；模板只给切换与保存两个动作的位置。
 */
export interface ListSavedViewsProps {
  /** 当前选中的视图 id；不在任何已存视图上时为 null。 */
  current: string | null;
  list: ListSavedView[];
  onSave: () => void;
  onSelect: (id: string) => void;
}

export interface ListPageTemplateProps<Row> {
  title: string;
  description?: string;
  /**
   * 面包屑 `区 › 页`。不传则按 moduleId 反查导航分区；两者都没有就不渲染这一条——
   * 面包屑说的是「这页在业务体系里的位置」，模板不为没有位置的页编一个。
   */
  breadcrumb?: TemplateBreadcrumb;
  /** 导航条目 id（与 navigation.ts 同一套），只用于反查面包屑。 */
  moduleId?: string;
  /** 页头右侧动作区（如「导出」按钮）。 */
  headerActions?: ReactNode;
  /** 不传则不渲染搜索框——过滤条整体仍在，便于只有下拉筛选的页面。 */
  search?: ListSearchProps;
  /** 过滤控件（下拉、chip 等）由调用方提供：筛选维度是业务决定，模板不预设。 */
  filters?: ReactNode;
  /**
   * 排序 / 保存视图 / 更多筛选三个位都可选：不传时位置照旧渲染成禁用按钮 + 悬停说明
   * （黄金标准 Rule 2「即使功能未完整，也要按完整模板布局」），传了才有动作。
   */
  sort?: ListSortProps;
  savedViews?: ListSavedViewsProps;
  /** 低频筛选控件，收在「更多筛选」按钮之后的第二行里，不平铺进主过滤条。 */
  moreFilters?: ReactNode;
  /** 过滤条右端的统计摘要（如「共 N 条」）。 */
  filterSummary?: ReactNode;
  columns: ListColumn<Row>[];
  rows: Row[];
  rowKey: (row: Row) => string;
  onRowClick?: (row: Row) => void;
  /**
   * ready 态下 rows 为空时表格区显的一行（如「当前筛选条件下没有匹配」）。它与 viewState 的空态是两个事实：
   * 空态说的是数据源为空，这一行说的是调用方在已取回数据上筛没了——调用方仍报 ready，模板不从 rows.length
   * 推断任何一态。不传则空表体照旧只显表头。
   */
  emptyRowsNote?: ReactNode;
  /** 不传则不渲染分页条（如队列页一次拉全量）。 */
  pagination?: ListPaginationProps;
  /**
   * 四态由调用方注入，模板不从 rows.length 推断空态——
   * 「闸门未放行」与「暂无数据」在本产品是两个必须区分的事实。
   */
  viewState: TemplateViewState;
  stateOverride?: StateSlotProps['override'];
}

// 过滤条上原生 select 的样子，与 FilterSearch 同高同字号；不从 pages/ 借类名——模板层不反向依赖页面层。
const filterControlClass =
  'h-6 shrink-0 rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 text-[11px] text-idpxyz-text ' +
  'outline-none focus-visible:ring-1 focus-visible:ring-idpxyz-accent/35';

// 禁用位：按钮 + 悬停说明，不是灰色占位方块（票 admin-web-ux-alignment/03 裁决 1）。
// 用 aria-disabled 而不是 disabled：Button 原语的 disabled 带 pointer-events-none，悬停说明就出不来，
// 键盘也聚焦不到它——说明本身是给人看的，位不能自己把说明藏起来。
function DisabledSlot({ label, reason }: { label: string; reason: string }) {
  return (
    <Tooltip content={reason}>
      <Button
        variant="outline"
        size="sm"
        aria-disabled="true"
        className="shrink-0 cursor-not-allowed opacity-50 hover:bg-transparent hover:text-idpxyz-textMuted"
        onClick={(event) => event.preventDefault()}
      >
        {label}
      </Button>
    </Tooltip>
  );
}

// 列表页模板：Breadcrumb + PageHeader + FilterBar + Table + Pagination，形态对齐 Monitor 页黄金标准
// （idp-ui@53df1666「IDP Monitor Page Golden Standard」）。36 张列表页共用这一个模板：一切新位都走可选 prop，
// 不传时的默认行为就是各页今天的行为。
// 非 ready 态只替换表格区，页头与过滤条保留——加载中用户仍能改筛选条件。
export function ListPageTemplate<Row>({
  title,
  description,
  breadcrumb,
  moduleId,
  headerActions,
  search,
  filters,
  sort,
  savedViews,
  moreFilters,
  filterSummary,
  columns,
  rows,
  rowKey,
  onRowClick,
  emptyRowsNote,
  pagination,
  viewState,
  stateOverride,
}: ListPageTemplateProps<Row>) {
  const alignClass = (align?: 'left' | 'center' | 'right') =>
    align === 'right' ? 'text-right' : align === 'center' ? 'text-center' : 'text-left';

  const crumb =
    breadcrumb ?? (moduleId ? resolveBreadcrumb(moduleId, navigationSections, pageTitleById) : null);

  // 密度（手册「栅格与密度」两档；黄金标准 11.5 Monitor 页默认推荐 Compact）。等票 admin-web-ux-alignment/01 在壳层挂
  // DensityProvider 之前 Layout 里没有 Provider——实测 @idpxyz/ui-theme-runtime 0.1.23 的 useDensity 在无 Provider 时
  // 不抛错，直接回 createContext 的默认值 compact，所以这里不需要 try / catch 兜底；01 挂上 Provider 后初值同为 compact，
  // 全站默认档前后一致，切换才是用户的选择。
  const { density } = useDensity();
  const cellPadding = densityRowPadding(density);

  // 「更多筛选」展开与否是模板自己的呈现状态，不回流给调用方：调用方只关心筛选值。
  const [moreFiltersOpen, setMoreFiltersOpen] = useState(false);
  const slots = filterBarSlots({
    sort: sort !== undefined,
    savedViews: savedViews !== undefined,
    moreFilters: moreFilters !== undefined,
  });

  // 位的启用与否由 filterBarSlots 决定；这里只管「启用的位长什么样」。视图模式位永远禁用，走不到下面。
  const renderSlot = (slot: FilterBarSlot) => {
    if (!slot.enabled) {
      return <DisabledSlot key={slot.id} label={slot.label} reason={slot.disabledReason ?? ''} />;
    }
    if (slot.id === 'sort' && sort) {
      return (
        <select
          key={slot.id}
          aria-label={slot.label}
          className={filterControlClass}
          value={sort.value}
          onChange={(event) => sort.onChange(event.target.value)}
        >
          {sort.options.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      );
    }
    if (slot.id === 'saved-view' && savedViews) {
      return (
        <span key={slot.id} className="flex shrink-0 items-center gap-1">
          <select
            aria-label={slot.label}
            className={filterControlClass}
            value={savedViews.current ?? ''}
            onChange={(event) => {
              if (event.target.value !== '') savedViews.onSelect(event.target.value);
            }}
          >
            <option value="">{slot.label}</option>
            {savedViews.list.map((view) => (
              <option key={view.id} value={view.id}>
                {view.label}
              </option>
            ))}
          </select>
          <Button variant="outline" size="sm" className="shrink-0" onClick={savedViews.onSave}>
            保存当前视图
          </Button>
        </span>
      );
    }
    if (slot.id === 'more-filters' && moreFilters !== undefined) {
      return (
        <Button
          key={slot.id}
          variant={moreFiltersOpen ? 'default' : 'outline'}
          size="sm"
          className="shrink-0"
          aria-expanded={moreFiltersOpen}
          onClick={() => setMoreFiltersOpen((open) => !open)}
        >
          {slot.label}
        </Button>
      );
    }
    return null;
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      {/* 22px 面包屑条，照 loms-web OrderList 的尺寸；分区不是页，所以只是文字、不给链接。 */}
      {crumb && (
        <div className="flex h-[22px] shrink-0 select-none items-center border-b border-idpxyz-border px-4">
          <Breadcrumb items={[{ label: crumb.section }, { label: crumb.page }]} />
        </div>
      )}
      <PageHeader>
        <PageHeaderContent>
          <PageHeaderTitle>{title}</PageHeaderTitle>
          {description && <PageHeaderDescription>{description}</PageHeaderDescription>}
        </PageHeaderContent>
        {headerActions && <PageHeaderActions>{headerActions}</PageHeaderActions>}
      </PageHeader>

      {/* 黄金标准 9.2 顺序：搜索 → 主筛选 → 排序 → 视图 → 保存视图 → 更多筛选 → 右端计数。位不够宽时横向滚，不换行。 */}
      <FilterBar className="overflow-x-auto">
        {search && (
          <div className="max-w-[280px] min-w-[160px] flex-1">
            <FilterSearch
              value={search.value}
              placeholder={search.placeholder}
              onChange={(e) => search.onChange(e.target.value)}
              onClear={search.value ? () => search.onChange('') : undefined}
            />
          </div>
        )}
        {filters && <FilterGroup>{filters}</FilterGroup>}
        <FilterGroup>{slots.map(renderSlot)}</FilterGroup>
        {filterSummary && (
          <span className="ml-auto shrink-0 text-[11px] text-idpxyz-textMuted">{filterSummary}</span>
        )}
      </FilterBar>
      {moreFilters !== undefined && moreFiltersOpen && (
        <div className="flex shrink-0 items-center gap-2 border-b border-idpxyz-border px-4 py-1.5">
          {moreFilters}
        </div>
      )}

      {viewState.kind === 'ready' ? (
        <>
          {/* 主表放进 surface 层容器（黄金标准「Main Content 黄金标准」；照 loms-web ShipmentMonitor：外圈留白 + 边框 + 圆角 +
              --idpxyz-sidebar 一档底色，Light 下页底 editor 是 Layer 1、这个容器是 Layer 2）。容器吃满剩余高度，滚动发生在容器内，
              所以表头吸顶仍对着容器的滚动口；分页条留在容器外的页底，翻页时它不随表体滚走。 */}
          <div className="flex min-h-0 flex-1 flex-col p-2">
            <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-md border border-idpxyz-border bg-idpxyz-sidebar">
              <div className="min-h-0 flex-1 overflow-auto">
                <Table stickyHeader>
                  <TableHeader>
                    <TableRow>
                      {columns.map((col) => (
                        // 吸顶表头原语自带 editor 底色，进了 sidebar 容器就成了一条异色带，这里盖成容器同色；靠 cn 同族让位，不改原语。
                        <TableHead
                          key={col.id}
                          className={`bg-idpxyz-sidebar ${alignClass(col.align)} ${col.className ?? ''}`}
                        >
                          {col.header}
                        </TableHead>
                      ))}
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {rows.length === 0 && emptyRowsNote !== undefined ? (
                      // 这一行不是数据行：不给悬停底色，纵向收紧到一行说明的高度——容器已经吃满剩余高度，空表体不该再用一整格
                      // 数据行的留白把「筛没了」这句话顶开。
                      <TableRow className="hover:bg-transparent">
                        <TableCell
                          colSpan={columns.length}
                          className="py-1.5 text-center text-idpxyz-textMuted"
                        >
                          {emptyRowsNote}
                        </TableCell>
                      </TableRow>
                    ) : null}
                    {rows.map((row) => (
                      <TableRow
                        key={rowKey(row)}
                        className={onRowClick ? 'cursor-pointer' : undefined}
                        onClick={onRowClick ? () => onRowClick(row) : undefined}
                      >
                        {columns.map((col) => (
                          <TableCell
                            key={col.id}
                            className={`${cellPadding} ${alignClass(col.align)} ${col.className ?? ''}`}
                          >
                            {col.render(row)}
                          </TableCell>
                        ))}
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </div>
          </div>
          {pagination && (
            <div className="border-t border-idpxyz-border px-4 py-1.5 shrink-0">
              <Pagination
                variant="table"
                page={pagination.page}
                pageSize={pagination.pageSize}
                total={pagination.total}
                onPageChange={pagination.onPageChange}
                onPageSizeChange={pagination.onPageSizeChange}
                pageSizeOptions={pagination.pageSizeOptions}
              />
            </div>
          )}
        </>
      ) : (
        <div className="flex-1 flex items-center justify-center overflow-auto">
          <StateSlot state={viewState} override={stateOverride} />
        </div>
      )}
    </div>
  );
}
