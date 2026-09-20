import type { ReactNode } from 'react';
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
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
  Pagination,
} from '@idpxyz/ui-primitives';
import { navigationSections, pageTitleById } from '../navigation';
import { resolveBreadcrumb, type TemplateBreadcrumb } from './breadcrumb';
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

      <FilterBar>
        {search && (
          <div className="max-w-[280px] flex-1">
            <FilterSearch
              value={search.value}
              placeholder={search.placeholder}
              onChange={(e) => search.onChange(e.target.value)}
              onClear={search.value ? () => search.onChange('') : undefined}
            />
          </div>
        )}
        {filters && <FilterGroup>{filters}</FilterGroup>}
        {filterSummary && (
          <span className="ml-auto text-[11px] text-idpxyz-textMuted">{filterSummary}</span>
        )}
      </FilterBar>

      {viewState.kind === 'ready' ? (
        <>
          <div className="flex-1 overflow-auto">
            <Table stickyHeader>
              <TableHeader>
                <TableRow>
                  {columns.map((col) => (
                    <TableHead key={col.id} className={`${alignClass(col.align)} ${col.className ?? ''}`}>
                      {col.header}
                    </TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.length === 0 && emptyRowsNote !== undefined ? (
                  <TableRow>
                    <TableCell colSpan={columns.length} className="text-center text-idpxyz-textMuted">
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
                      <TableCell key={col.id} className={`${alignClass(col.align)} ${col.className ?? ''}`}>
                        {col.render(row)}
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
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
