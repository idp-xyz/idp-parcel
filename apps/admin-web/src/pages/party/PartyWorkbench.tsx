import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
  type RefObject,
} from 'react';
import { ArrowDown, ArrowUp, ArrowUpDown, ChevronLeft, ChevronRight, Inbox, Search } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { useSplitResize } from '@idpxyz/ui-workspace';
import { revealClass, useDelayedReveal } from '../../components/use-delayed-reveal';
import { cursorPagerControls } from '../../templates/cursor-pagination';
import type { ListCursorPaginationProps } from '../../templates/ListPageTemplate';
import { StateSlot, type StateSlotProps, type TemplateViewState } from '../../templates/state-slot';
import type { WorkbenchSort } from '../../templates/workbench';
import { useTabViewRef } from '../../templates/workspace-view-state';
import { partyColumnLayoutOf, partyFooterPageText } from './business-parties-workbench';
import { usePartyListKeys } from './use-party-list-keys';

// 往来单位页新版式的列表区（票 business-party-redesign/02）：工具条一行、定宽列的表、表底翻页条，页面态只换表格那一块。
// 本页自己的布局，不改 WorkbenchPageTemplate——那张模板别的页还在用，这套版式先在这一页试，验收后再收进共用模板。
// 键盘（use-party-list-keys.ts）、选中、滚动位置与分栏比例的记忆照搬模板的做法（同一组纯函数与钩子），换的只是长相：
// - 各列给定宽，多出来的宽度全落在行尾；列表窄到放不下时行尾的箭头不摆，别的列先往各自的窄宽收、首列再让，首列留一个
//   下限，再窄就横向滚（business-parties-workbench.ts 的 partyColumnLayoutOf）。
// - 翻页在表底，不在工具条右端：工具条那一行留给筛选。
// - 首次加载的骨架照将要出现的列来排，行高与真表一样，答案到了行不跳。
// 行上没有悬停底色（用户 2026-11-03 裁定：浅色主题里悬停与选中两种底色太像），与模板同。

export interface PartyWorkbenchColumn<Row> {
  key: string;
  header: ReactNode;
  /** 定宽（像素）。 */
  width: number;
  /** 列表窄到首列不够它的定宽时（右栏开着），这一列最多收到这么宽；首列不看它。不给就不收。 */
  narrowWidth?: number;
  /** 给了才能点表头排序；值交给 onSort。 */
  sortKey?: string;
  /** 列表可用宽度达到此像素值才展示。 */
  minListWidth?: number;
  /** 骨架里这一格画多宽的一条（像素）；首列另有图标块与两行，不看它。 */
  skeleton?: number;
  render: (row: Row) => ReactNode;
}

export interface PartyWorkbenchFooter {
  /** 左边那一句（共几家）；还没有答案时不给。 */
  countText: string | null;
  pagination?: ListCursorPaginationProps;
}

export interface PartyWorkbenchProps<Row> {
  /** 分栏比例与滚动位置按它记在工作区标签上。 */
  viewStateKey: string;
  search: { value: string; onChange: (value: string) => void; placeholder: string };
  /** 检索框后面的筛选件。拿到工具条此刻的宽度：放得下摆芯片，放不下自己收成下拉，不折行。 */
  filters?: (toolbarWidth: number | null) => ReactNode;
  columns: PartyWorkbenchColumn<Row>[];
  rows: Row[];
  rowKey: (row: Row) => string;
  selectedKey: string | null;
  onSelect: (key: string | null) => void;
  /** 需要人去查的行铺一层很淡的警示底色；选中时让位给选中的底色。 */
  rowAttention?: (row: Row) => boolean;
  sort?: WorkbenchSort;
  onSort?: (key: string) => void;
  /** ready 态下筛没了时的那句话；与空册是两件事。 */
  noMatchText: string;
  onClearFilters?: () => void;
  footer: PartyWorkbenchFooter;
  renderDetail: (key: string) => ReactNode;
  viewState: TemplateViewState;
  stateOverride?: StateSlotProps['override'];
  /**
   * 挂上来时把焦点落到这一行：名录从主从分栏收回成表时，焦点原在的那一件（名录栏的行、右栏的关闭钮）已经卸掉，落回刚才开着
   * 的那一行，↑/↓ 才接得上。只在焦点确实没了着落时落，不从别处抢。
   */
  initialFocusKey?: string | null;
}

/** 检索框：放大镜、输入、`/` 键位提示。表的工具条与名录栏共用；宽度由调用方经 className 给。 */
export function PartySearchBox({
  inputRef,
  value,
  onChange,
  placeholder,
  className,
}: {
  inputRef: RefObject<HTMLInputElement | null>;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
  className: string;
}) {
  return (
    <div
      className={`flex items-center gap-2 rounded border border-idpxyz-border bg-idpxyz-inputBg px-3 py-1.5 type-body focus-within:border-idpxyz-accent focus-within:ring-1 focus-within:ring-idpxyz-accent ${className}`}
    >
      <Search className="h-3.5 w-3.5 shrink-0 text-idpxyz-breadcrumb" aria-hidden="true" />
      <input
        ref={inputRef}
        type="text"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        aria-label={placeholder}
        className="min-w-0 flex-1 bg-transparent text-idpxyz-text placeholder-idpxyz-breadcrumb outline-none"
        onKeyDown={(event) => event.key === 'Escape' && event.currentTarget.blur()}
      />
      <kbd className="rounded border border-idpxyz-border px-1 type-caption text-idpxyz-breadcrumb">/</kbd>
    </div>
  );
}

/** 行尾留给箭头的最小宽度；各定宽列加上它还放得下，才算「放得下」。 */
const TRAILING_MIN = 48;
/** 各列定宽放不下时，首列至少留这么宽（图标块加一个看得出是谁的名称）。 */
const FIRST_COLUMN_MIN = 180;
const SKELETON_ROWS = 14;

export function PartyWorkbench<Row>({
  viewStateKey,
  search,
  filters,
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
  footer,
  renderDetail,
  viewState,
  stateOverride,
  initialFocusKey = null,
}: PartyWorkbenchProps<Row>) {
  const searchRef = useRef<HTMLInputElement>(null);
  const toolbarRef = useRef<HTMLDivElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const [toolbarWidth, setToolbarWidth] = useState<number | null>(null);
  const [listWidth, setListWidth] = useState<number | null>(null);
  const split = useSplitResize('horizontal');
  const ready = viewState.kind === 'ready';
  const detailOpen = ready && selectedKey !== null && rows.some((row) => rowKey(row) === selectedKey);

  useLayoutEffect(() => {
    const toolbar = toolbarRef.current;
    const list = listRef.current;
    if (!toolbar || !list) return;
    const measure = () => {
      // 隐藏容器的零宽不能覆盖最后一次有效测量（壳层把非活动标签留在 DOM 里）。
      if (toolbar.clientWidth > 0) setToolbarWidth(toolbar.clientWidth);
      if (list.clientWidth > 0) setListWidth(list.clientWidth);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(toolbar);
    observer.observe(list);
    return () => observer.disconnect();
  }, []);
  // 详情开合与分栏比例一变，列表宽就变：在绘制前重量一次，免得先按旧宽画出全部列、下一帧再收（理由同模板）。
  useLayoutEffect(() => {
    const list = listRef.current;
    if (list && list.clientWidth > 0) setListWidth(list.clientWidth);
  }, [detailOpen, split.ratio]);

  const layoutSnapshot = useTabViewRef(`party-workbench.${viewStateKey}.layout`, () => ({ ratio: 50, scrollTop: 0, scrollLeft: 0 }));
  const splitRestored = useRef(false);
  useLayoutEffect(() => {
    split.setRatio(layoutSnapshot.current.ratio);
  }, [split.setRatio, layoutSnapshot]);
  useLayoutEffect(() => {
    if (!splitRestored.current && split.ratio !== layoutSnapshot.current.ratio) return;
    splitRestored.current = true;
    layoutSnapshot.current.ratio = split.ratio;
  }, [split.ratio, layoutSnapshot]);
  useLayoutEffect(() => {
    if (!ready || !listRef.current) return;
    listRef.current.scrollTop = layoutSnapshot.current.scrollTop;
    listRef.current.scrollLeft = layoutSnapshot.current.scrollLeft;
  }, [ready, layoutSnapshot]);

  usePartyListKeys({ listRef, searchRef, keys: rows.map(rowKey), selectedKey, detailOpen, ready, onSelect });

  // 只在挂上来的那一刻看一次（initialFocusKey 的说明），所以不把它作依赖：之后焦点去哪由人定。
  useEffect(() => {
    if (initialFocusKey === null || document.activeElement !== document.body) return;
    listRef.current?.querySelector<HTMLElement>(`[data-row-key="${CSS.escape(initialFocusKey)}"]`)?.focus();
  }, []);

  const visibleColumns = columns.filter(
    (column) => column.minListWidth === undefined || listWidth === null || listWidth >= column.minListWidth,
  );
  // 各列怎么排由 partyColumnLayoutOf 定：放得下余量落在行尾；放不下箭头不摆、首列拿剩下的，别的列按需往窄宽收。
  const layout = partyColumnLayoutOf(listWidth, visibleColumns, TRAILING_MIN, FIRST_COLUMN_MIN);
  const fits = layout.fits;
  const tableStyle = layout.minWidth === null ? undefined : { minWidth: layout.minWidth };
  const colgroup = (
    <colgroup>
      {visibleColumns.map((column, index) => {
        const width = layout.widths[index];
        return <col key={column.key} style={width === null ? undefined : { width }} />;
      })}
      {fits ? <col /> : null}
    </colgroup>
  );
  const cellPadding = (index: number) => `px-3 ${index === 0 ? 'pl-6' : ''} ${!fits && index === visibleColumns.length - 1 ? 'pr-6' : ''}`;

  const SortIcon = ({ sortKey }: { sortKey: string }) =>
    !sort || sort.key !== sortKey ? (
      <ArrowUpDown className="h-3 w-3 opacity-40" aria-hidden="true" />
    ) : sort.dir === -1 ? (
      <ArrowDown className="h-3 w-3" aria-hidden="true" />
    ) : (
      <ArrowUp className="h-3 w-3" aria-hidden="true" />
    );

  const head = (
    <thead className="sticky top-0 z-10 bg-idpxyz-panelBg">
      <tr className="h-9 border-b border-idpxyz-border text-left type-caption text-idpxyz-breadcrumb">
        {visibleColumns.map((column, index) => (
          <th
            key={column.key}
            scope="col"
            className={`whitespace-nowrap font-normal ${cellPadding(index)}`}
            aria-sort={column.sortKey && sort?.key === column.sortKey ? (sort.dir === 1 ? 'ascending' : 'descending') : undefined}
          >
            {column.sortKey && onSort ? (
              <button type="button" onClick={() => onSort(column.sortKey!)} className="inline-flex items-center gap-1 hover:text-idpxyz-textBright">
                {column.header}
                <SortIcon sortKey={column.sortKey} />
              </button>
            ) : (
              column.header
            )}
          </th>
        ))}
        {fits ? <th aria-hidden="true" /> : null}
      </tr>
    </thead>
  );

  const onRowKeyDown = (event: ReactKeyboardEvent<HTMLTableRowElement>, key: string) => {
    if (event.target !== event.currentTarget || event.key !== 'Enter') return;
    event.preventDefault();
    onSelect(selectedKey === key ? null : key);
  };

  const table =
    rows.length === 0 ? (
      <div className="flex flex-col items-center justify-center py-16 text-idpxyz-breadcrumb">
        <Inbox className="mb-3 h-8 w-8 opacity-50" aria-hidden="true" />
        <p className="type-value">{noMatchText}</p>
        {onClearFilters && (
          <Button variant="ghost" size="sm" className="mt-2 type-body text-idpxyz-breadcrumb" onClick={onClearFilters}>
            清除筛选
          </Button>
        )}
      </div>
    ) : (
      <table className="w-full table-fixed type-body" style={tableStyle}>
        {colgroup}
        {head}
        <tbody>
          {rows.map((row) => {
            const key = rowKey(row);
            const selected = selectedKey === key;
            return (
              <tr
                key={key}
                data-row-key={key}
                tabIndex={0}
                aria-selected={selected}
                onClick={() => onSelect(selected ? null : key)}
                onKeyDown={(event) => onRowKeyDown(event, key)}
                // 焦点框只画在未选中的行上：焦点与选中落在同一行时，底色加左竖线已经说明了「在这」。
                className={`h-12 cursor-pointer border-b border-idpxyz-border ${
                  selected
                    ? 'bg-idpxyz-activeItem shadow-[inset_2px_0_0_0_var(--idpxyz-accent)] focus-visible:outline-none'
                    : `focus-visible:outline focus-visible:outline-1 focus-visible:-outline-offset-1 focus-visible:outline-idpxyz-accent ${
                        rowAttention?.(row) ? 'bg-orange-500/5' : ''
                      }`
                }`}
              >
                {visibleColumns.map((column, index) => (
                  <td key={column.key} className={`overflow-hidden ${cellPadding(index)}`}>
                    {column.render(row)}
                  </td>
                ))}
                {fits ? (
                  <td className="pr-6 text-right text-idpxyz-breadcrumb" aria-hidden="true">
                    <ChevronRight className="inline h-4 w-4 opacity-60" />
                  </td>
                ) : null}
              </tr>
            );
          })}
        </tbody>
      </table>
    );

  const skeleton = (
    <WorkbenchSkeleton colgroup={colgroup} head={head} columns={visibleColumns} cellPadding={cellPadding} trailing={fits} tableStyle={tableStyle} />
  );

  const pagination = footer.pagination;
  const pager = pagination ? cursorPagerControls(pagination.page, pagination.next) : null;

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden bg-idpxyz-panelBg">
      <div ref={toolbarRef} className="flex shrink-0 items-center gap-2.5 overflow-hidden px-6 py-2.5">
        <PartySearchBox inputRef={searchRef} {...search} className="w-64 shrink-0" />
        {filters?.(toolbarWidth)}
      </div>

      <div ref={split.containerRef} className="flex min-h-0 flex-1 overflow-hidden border-t border-idpxyz-border">
        <div
          data-pane="list"
          className="flex min-h-0 min-w-0 flex-col"
          style={detailOpen ? { flex: `0 0 ${split.ratio}%` } : { flex: '1 1 100%' }}
        >
          <div
            ref={listRef}
            className={`min-h-0 flex-1 ${viewState.kind === 'loading' ? 'overflow-hidden' : 'overflow-auto'}`}
            onScroll={(event) => {
              if (!ready) return;
              layoutSnapshot.current.scrollTop = event.currentTarget.scrollTop;
              layoutSnapshot.current.scrollLeft = event.currentTarget.scrollLeft;
            }}
          >
            {ready ? (
              table
            ) : viewState.kind === 'loading' ? (
              skeleton
            ) : (
              // 说明块落在上三分之一，贴近工具条和视线，不垂直居中：屏幕一高，居中的块会掉到下半屏。
              <div className="flex justify-center px-6 pb-8 pt-[14vh]">
                <StateSlot state={viewState} override={stateOverride} />
              </div>
            )}
          </div>
          <div className="flex h-9 shrink-0 items-center justify-between border-t border-idpxyz-border pl-6 pr-3 type-caption text-idpxyz-breadcrumb">
            <span className="tabular-nums">{viewState.kind === 'loading' ? '正在读取…' : ready ? footer.countText : null}</span>
            {ready && pagination && pager ? (
              <nav aria-label="翻页" className="flex items-center gap-1" data-cursor-pagination>
                <Button variant="ghost" size="sm" disabled={!pager.canPrevious} onClick={pagination.onPrevious}>
                  <ChevronLeft className="h-3 w-3" aria-hidden="true" />
                  上一页
                </Button>
                <span className="px-1 tabular-nums">{partyFooterPageText(pagination.page, pagination.size, pagination.total)}</span>
                <Button variant="ghost" size="sm" disabled={!pager.canNext} onClick={() => pagination.next !== null && pagination.onNext(pagination.next)}>
                  下一页
                  <ChevronRight className="h-3 w-3" aria-hidden="true" />
                </Button>
              </nav>
            ) : null}
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

/** 首次加载的骨架：与真表同一组列宽、同一个表头、同一个行高。答得快时不露（use-delayed-reveal.ts），免得闪一下。 */
function WorkbenchSkeleton<Row>({
  colgroup,
  head,
  columns,
  cellPadding,
  trailing,
  tableStyle,
}: {
  colgroup: ReactNode;
  head: ReactNode;
  columns: PartyWorkbenchColumn<Row>[];
  cellPadding: (index: number) => string;
  trailing: boolean;
  tableStyle: CSSProperties | undefined;
}) {
  const revealCls = revealClass(useDelayedReveal());
  const bar = 'block h-2.5 rounded bg-idpxyz-hover';
  return (
    <table className={`w-full table-fixed type-body ${revealCls}`} style={tableStyle} role="status" aria-label="加载中">
      {colgroup}
      {head}
      <tbody className="animate-pulse">
        {Array.from({ length: SKELETON_ROWS }, (_, row) => (
          <tr key={row} className="h-12 border-b border-idpxyz-border">
            {columns.map((column, index) => (
              <td key={column.key} className={cellPadding(index)}>
                {index === 0 ? (
                  <span className="flex items-center gap-2.5">
                    <span className="h-8 w-8 shrink-0 rounded-lg bg-idpxyz-hover" />
                    <span className="flex min-w-0 flex-1 flex-col gap-2">
                      <span className={bar} style={{ width: `${[62, 48, 70, 55][row % 4]}%` }} />
                      <span className="block h-2 rounded bg-idpxyz-hover" style={{ width: `${[38, 30, 44][row % 3]}%` }} />
                    </span>
                  </span>
                ) : column.skeleton ? (
                  <span className={bar} style={{ width: column.skeleton, maxWidth: '100%' }} />
                ) : null}
              </td>
            ))}
            {trailing ? <td /> : null}
          </tr>
        ))}
      </tbody>
    </table>
  );
}
