import { useEffect, useRef, useState, type ReactNode, type SyntheticEvent } from 'react';
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
import {
  densityRowPadding,
  filterBarSlots,
  rowInteraction,
  rowKeyOpens,
  type FilterBarSlot,
} from './list-page-structure';
import {
  clearSelection,
  retainSelectedRows,
  rowsToCsv,
  selectedOnPage,
  selectedRows,
  toggleAllOnPage,
  toggleSelected,
  type CsvCellText,
  type PageSelectionState,
} from './list-selection';
import { StateSlot, type TemplateViewState, type StateSlotProps } from './state-slot';
import { useInspector } from './inspector-context';
import type { InspectorContent } from './inspector';

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

/**
 * 选择模型（蓝图 10.5「多选行 → 露出 Bulk Action Bar」）。选中集按 rowKey 记、由调用方持有——它是页面的 UI 瞬态，模板不替页面存；
 * 翻页保留、筛选变更**不**自动清（用户可能在跨筛选攒一批再一起导出）；换模块（组件卸载）即丢，不进 saved-views：保存视图存的是
 * 「筛选态」，选中集是对某一批具体对象的临时圈定，两者保质期差几个量级。传了才出复选列（表头 + 每行）；不传时 DOM 里没有任何
 * 复选框，行为与今天同。
 */
export interface ListSelectionProps {
  selected: ReadonlySet<string>;
  onChange: (next: Set<string>) => void;
}

export interface ListCsvExportProps<Row> {
  /** 下载文件名，含扩展名。 */
  fileName: string;
  /**
   * 取字函数由调用方给：列的 render 出的是 ReactNode，CSV 不能从节点里抠字。对某列回 undefined 表示这一格没有文本；
   * 一列在全部待导出行上都没有文本（如动作列）就整列跳过。
   */
  cellText: CsvCellText<Row, ListColumn<Row>>;
}

/**
 * 批量动作栏放什么。栏本身随 selection 长出——`selected.size > 0` 时在 Filter Bar 之下、表之上；为 0 时不渲染，不留空条——
 * 「已选 N 项」与「取消选择」是栏的固定件，这里只定中间的动作。默认动作只有本机能诚实完成的「导出所选（CSV）」，有 csv 才出；
 * 要命令端点的批量动作（暂挂 / 分配 / 打标签之类）由调用方按端点有无经 extra 传入，模板不预设一个禁用的假动作
 * （spec 红线：批量动作栏默认只有本机能完成的动作）。
 */
export interface ListBulkActionsProps<Row> {
  csv?: ListCsvExportProps<Row>;
  extra?: ReactNode;
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
  /** 多选。不传则无复选列、无动作栏，与今天同。 */
  selection?: ListSelectionProps;
  /** 批量动作栏的动作；没传 selection 时它无处可长，不渲染。 */
  bulkActions?: ListBulkActionsProps<Row>;
  /** 单击一行：预览 / 选中。有 onRowOpen 时它仍然是单击的语义，不被双击顶掉。 */
  onRowClick?: (row: Row) => void;
  /**
   * 单击一行时交给右侧检查器的内容（票 admin-web-workspace-form/02 第 3 条，蓝图母版 B「List → Preview → Inspector」）。
   * 有它时单击 = `onRowClick?.(row)` **且**把 `inspector(row)` 交给壳层的检查器（经 useInspector），被单击的行标记为选中；
   * 行进 Tab 序、聚焦即交给检查器；rows 换了按键重推或清空。没有它时行为零变化。字段只取行里已有的，不发第二个请求。
   * 双击仍走 onRowOpen。
   */
  inspector?: (row: Row) => InspectorContent;
  /**
   * 双击一行（或行聚焦后按 Enter）：开对象。由调用方写 hash 二段路由；壳层已是多标签工作区（票 admin-web-workspace-form/01），
   * 对象地址会开成自己的标签。接了它（或 inspector）行才进 Tab 序；不接则行为与今天同。
   */
  onRowOpen?: (row: Row) => void;
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

// 可聚焦行的焦点态：outline 而不是 ring——ring 是 box-shadow，浏览器对 <tr> 不一定画；outline 内缩一格免得被容器
// overflow 裁掉。只在 focus-visible 时出，鼠标点行不闪框。
const rowFocusClass =
  'focus-visible:outline focus-visible:outline-1 focus-visible:-outline-offset-1 focus-visible:outline-idpxyz-accent ' +
  'focus-visible:bg-idpxyz-hover';

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

// 复选用原生 input 而不是 ui-primitives 的 Checkbox（Radix，渲成 button）：表格里一列几十个复选，原生控件自带 indeterminate 三态、
// 键盘与 AT 语义，且与仓内既有表单（party/* 的 checkbox）同一个控件；accent 色让它吃主题令牌。
const checkboxClass = 'accent-idpxyz-accent';

// 表头复选。indeterminate 只是 DOM 属性、不是 HTML 特性，React 声明不了它，只能在提交后对着节点设；
// checked 只在「全」时为真——「部分」既不算勾也不算空，由 indeterminate 表达。
function PageSelectAllCheckbox({
  state,
  disabled,
  onToggle,
}: {
  state: PageSelectionState;
  disabled: boolean;
  onToggle: () => void;
}) {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = state === 'some';
  }, [state]);
  return (
    <input
      ref={ref}
      type="checkbox"
      className={checkboxClass}
      aria-label="选择本页全部"
      checked={state === 'all'}
      disabled={disabled}
      onChange={onToggle}
    />
  );
}

// 复选格上截住指针事件：选择不等于预览（蓝图 10.5），格内的点击不冒泡成行的 onRowClick，双击不冒泡成 onRowOpen。
// Enter 不必截——行的 onKeyDown 只认 target 是行本身的按键，复选框上的 Enter 到不了那条分支。
const stopRowEvent = (event: SyntheticEvent) => event.stopPropagation();

// 本机下载：Blob → 对象 URL → 隐藏 <a download> 点一下。URL 延后释放：部分浏览器在 click 返回时尚未开始读 Blob，同步 revoke 会下到空文件。
function downloadTextFile(fileName: string, text: string, mimeType: string) {
  const url = URL.createObjectURL(new Blob([text], { type: mimeType }));
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = fileName;
  anchor.rel = 'noopener';
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}

// 列表页模板：Breadcrumb + PageHeader + FilterBar + Table + Pagination，形态对齐 Monitor 页黄金标准
// （idp-ui@53df1666「IDP Monitor Page Golden Standard」）。所有列表页共用这一个模板，形态与行为分两条规矩：
// 形态（四个结构位、surface 容器、按密度的行距）按黄金标准 Rule 2 无条件长出，不传任何新 prop 的页也长；
// 行为（排序 / 保存视图 / 更多筛选的动作、双击与 Enter 开对象、行进 Tab 序）只在调用方接了对应 prop 时才有——
// 不接的位是禁用按钮 + 悬停说明，onRowOpen 与 inspector 都不接的行不进 Tab 序、只响应单击。
// 多选（复选列 + 批量动作栏）是行为不是形态：接了 selection 才长，不接的页 DOM 里没有一个复选框——它改变行的点击语义
// （多了一格不算行交互的地方），不能像结构位那样无条件长出。
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
  selection,
  bulkActions,
  onRowClick,
  inspector,
  onRowOpen,
  emptyRowsNote,
  pagination,
  viewState,
  stateOverride,
}: ListPageTemplateProps<Row>) {
  const alignClass = (align?: 'left' | 'center' | 'right') =>
    align === 'right' ? 'text-right' : align === 'center' ? 'text-center' : 'text-left';

  // 检查器里正显示的那一行（键）。只在接了 inspector 时有意义；它是模板自己的呈现状态，不回流给调用方——调用方要的
  // 是「单击了哪一行」（onRowClick），选中高亮只是告诉人右栏说的是哪一行。换模块（组件卸载）即丢。
  const inspectorController = useInspector();
  const [inspectedKey, setInspectedKey] = useState<string | null>(null);
  const offersInspector = inspector !== undefined;
  useEffect(() => (offersInspector ? inspectorController.offer() : undefined), [offersInspector, inspectorController]);
  // 最近交给检查器的那份内容的 JSON（函数不进 JSON）。rows 变了按键重推时拿它比：页面常在每次渲染里重建行对象（把读模型
  // 映成展示行），比对象身份会每次都交——交了壳层重渲本页、本页又重建行，转不出来；比内容只在字真变了时才交。
  const shownContentJson = useRef<string | null>(null);
  const inspectRow = (row: Row, key: string) => {
    if (!inspector) return;
    const content = inspector(row);
    shownContentJson.current = JSON.stringify(content);
    setInspectedKey(key);
    inspectorController.show(content);
  };
  const handleRowClick = (row: Row, key: string) => {
    onRowClick?.(row);
    inspectRow(row, key);
  };
  // 右栏不留单击那一刻的旧快照：rows 换了（重取、检索改了）就按键找回那一行重推，找不到就清空。
  // inspector 与 rowKey 不作依赖：它们常是每次渲染新建的函数，用本次渲染的那一份就对。
  useEffect(() => {
    if (!inspector || inspectedKey === null) return;
    const row = rows.find((candidate) => rowKey(candidate) === inspectedKey);
    if (row === undefined) {
      shownContentJson.current = null;
      setInspectedKey(null);
      inspectorController.clear();
      return;
    }
    const content = inspector(row);
    const json = JSON.stringify(content);
    if (json === shownContentJson.current) return;
    shownContentJson.current = json;
    inspectorController.show(content);
  }, [rows, inspectedKey]);

  const crumb =
    breadcrumb ?? (moduleId ? resolveBreadcrumb(moduleId, navigationSections, pageTitleById) : null);

  // 密度（手册「栅格与密度」两档；黄金标准 11.5 Monitor 页默认推荐 Compact）。等票 admin-web-ux-alignment/01 在壳层挂
  // DensityProvider 之前 Layout 里没有 Provider——实测 @idpxyz/ui-theme-runtime 0.1.23 的 useDensity 在无 Provider 时
  // 不抛错，直接回 createContext 的默认值 compact，所以这里不需要 try / catch 兜底；01 挂上 Provider 后初值同为 compact，
  // 全站默认档前后一致，切换才是用户的选择。
  const { density } = useDensity();
  const cellPadding = densityRowPadding(density);

  // 行的交互属性由 rowInteraction 决定；这里只管把它们贴到 <tr> 上。没接任何回调时 className 为 undefined。
  // 接了 inspector 的行也是可单击的——单击把它交给检查器。
  const interaction = rowInteraction({
    click: onRowClick !== undefined || inspector !== undefined,
    open: onRowOpen !== undefined,
    inspect: inspector !== undefined,
  });
  const rowClass =
    [interaction.clickable ? 'cursor-pointer' : '', interaction.tabIndex !== undefined ? rowFocusClass : '']
      .filter((part) => part !== '')
      .join(' ') || undefined;

  // 选中行的记忆（键 → 行），供「导出所选」拿到已翻走的行。放 ref 不放 state：它只在导出那一刻被读，不驱动渲染；
  // 在 effect 里修剪而不在渲染中写 ref，导出是渲染之后的用户事件，读到的一定是修剪过的。
  const selectedRowMemory = useRef(new Map<string, Row>());
  const selectedKeys = selection?.selected;
  useEffect(() => {
    if (selectedKeys === undefined) return;
    selectedRowMemory.current = retainSelectedRows(selectedRowMemory.current, selectedKeys, rows, rowKey);
  }, [selectedKeys, rows, rowKey]);

  const pageKeys = selection ? rows.map(rowKey) : [];
  const pageSelection = selection ? selectedOnPage(selection.selected, pageKeys) : 'none';
  const selectedCount = selection?.selected.size ?? 0;

  const exportSelectedCsv = () => {
    if (!selection || !bulkActions?.csv) return;
    const { fileName, cellText } = bulkActions.csv;
    const text = rowsToCsv(selectedRows(selectedRowMemory.current, selection.selected), columns, cellText);
    downloadTextFile(fileName, text, 'text/csv;charset=utf-8');
  };

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

      {/* 批量动作栏（蓝图 10.7）：Filter Bar 之下、表之上，有选中才出，不留空条；栏高随密度两档走同一个 py- 类。
          不随 viewState 收起——选中集在取数中仍成立，导出所选读的是记忆里的行、不依赖当前表格区。 */}
      {selection && selectedCount > 0 && (
        <div
          role="toolbar"
          aria-label="批量动作"
          className={`flex shrink-0 items-center gap-2 border-b border-idpxyz-accent/30 bg-idpxyz-accent/10 px-4 ${cellPadding}`}
        >
          <span className="text-[11px] font-medium text-idpxyz-accent">已选 {selectedCount} 项</span>
          <span aria-hidden="true" className="mx-1 h-3 w-px bg-idpxyz-border" />
          {bulkActions?.csv && (
            <Button variant="ghost" size="sm" className="shrink-0" onClick={exportSelectedCsv}>
              导出所选（CSV）
            </Button>
          )}
          {bulkActions?.extra}
          <Button
            variant="ghost"
            size="sm"
            className="ml-auto shrink-0"
            onClick={() => selection.onChange(clearSelection())}
          >
            取消选择
          </Button>
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
                      {selection && (
                        <TableHead className="w-8 bg-idpxyz-sidebar text-center">
                          <PageSelectAllCheckbox
                            state={pageSelection}
                            disabled={rows.length === 0}
                            onToggle={() => selection.onChange(toggleAllOnPage(selection.selected, pageKeys))}
                          />
                        </TableHead>
                      )}
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
                          colSpan={columns.length + (selection ? 1 : 0)}
                          className="py-1.5 text-center text-idpxyz-textMuted"
                        >
                          {emptyRowsNote}
                        </TableCell>
                      </TableRow>
                    ) : null}
                    {rows.map((row) => {
                      const key = rowKey(row);
                      // 检查器正显示的行带一层底色与 data-inspected：多选的勾说的是「圈进批量动作」，这一层说的是
                      // 「右栏在说它」，两件事各有各的记号，不共用复选框。
                      const inspected = inspector !== undefined && inspectedKey === key;
                      return (
                        <TableRow
                          key={key}
                          className={
                            [rowClass ?? '', inspected ? 'bg-idpxyz-accent/10 hover:bg-idpxyz-accent/15' : '']
                              .filter((part) => part !== '')
                              .join(' ') || undefined
                          }
                          data-inspected={inspected ? 'true' : undefined}
                          tabIndex={interaction.tabIndex}
                          onClick={onRowClick || inspector ? () => handleRowClick(row, key) : undefined}
                          // 聚焦即交给检查器：键盘用户没有单击，Tab 到哪一行右栏就说哪一行；不占 Enter（那是开对象）。
                          // 只认落在行本身的聚焦——行里的复选框、按钮聚焦时冒泡上来的不算。
                          onFocus={
                            inspector
                              ? (event) => {
                                  if (event.target === event.currentTarget) inspectRow(row, key);
                                }
                              : undefined
                          }
                          onDoubleClick={onRowOpen ? () => onRowOpen(row) : undefined}
                          onKeyDown={
                            onRowOpen
                              ? (event) => {
                                  // 只认落在行本身的 Enter：单元格里的按钮 / 链接自己吃 Enter，冒泡上来的不算开行，否则按一次
                                  // 触发两件事。
                                  if (event.target !== event.currentTarget || !rowKeyOpens(event.key)) return;
                                  onRowOpen(row);
                                }
                              : undefined
                          }
                        >
                          {selection && (
                            <TableCell
                              className={`${cellPadding} w-8 text-center`}
                              onClick={stopRowEvent}
                              onDoubleClick={stopRowEvent}
                            >
                              <input
                                type="checkbox"
                                className={checkboxClass}
                                aria-label={`选择 ${key}`}
                                checked={selection.selected.has(key)}
                                onChange={() => selection.onChange(toggleSelected(selection.selected, key))}
                              />
                            </TableCell>
                          )}
                          {columns.map((col) => (
                            <TableCell
                              key={col.id}
                              className={`${cellPadding} ${alignClass(col.align)} ${col.className ?? ''}`}
                            >
                              {col.render(row)}
                            </TableCell>
                          ))}
                        </TableRow>
                      );
                    })}
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
