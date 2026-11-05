import { useState, type ReactNode } from 'react';
import { X } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import {
  WorkbenchPageTemplate,
  toggleWorkbenchSort,
  type ListCursorPaginationProps,
  type TemplateViewState,
  type WorkbenchColumn,
  type WorkbenchSort,
} from '../../templates';
import type { StateSlotProps } from '../../templates/state-slot';
import { EmptyState } from '../../components/states';
import { TechnicalDetails, type TechnicalRow } from '../../components/TechnicalDetails';
import { matchesSearch } from '../party/list-order';
import { countByStatus, sortRowsBy, type RowStatus, type SortAccessor } from '../visibility/exception-list';
import { statusFilterParts } from '../visibility/status-filter';
import { RowStatusPill } from '../visibility/ExceptionDetailPanel';

// 线路与运力区各页的工作台外壳：网络与网点、服务区域、口岸与申报路径、路线规划、网点作业、运输班次、外部轨迹时间确认共用。
// 形状是全站的单据工作台（templates/WorkbenchPageTemplate）：状态胶囊计数既是页头指标也是筛选、表头排序、右栏详情、键盘；
// 这里只多管一件事——各页都有几本册在同一页里切换，切册的那排签放进工具条（sections），换册时由调用方给新的 key 重挂本件，
// 筛选与选中跟着清掉，上一册的选中行不会落到下一册的同号行上。
//
// 行状态、计数、排序、检索的纯函数借异常区那一份（visibility/exception-list、status-filter），不另写第二份。

export type { RowStatus, SortAccessor };

export interface RouteWorkbenchProps<Row> {
  title: string;
  subtitle: string;
  /** 切册的签（工具条里）；一页只有一本册时不给。 */
  sections?: ReactNode;
  /** 工具条上本册特有的筛选（如订舱号、轨迹来源）。 */
  extraFilters?: ReactNode;
  primaryAction?: ReactNode;
  /** 拿到业务答案了没有：没拿到时不报计数，免得「0 条」与状态区的「这不是没有记录」打架。 */
  answered: boolean;
  rows: readonly Row[];
  rowKey: (row: Row) => string;
  /** 行的状态胶囊；给了才有指标格与状态胶囊计数。 */
  statusOf?: (row: Row) => RowStatus | undefined;
  /** 检索命中哪几格；检索已下推到读口时（search 由调用方持有）不在这里再筛。 */
  searchCells: (row: Row) => readonly (string | undefined)[];
  search?: { value: string; onChange: (value: string) => void; pushedDown?: boolean };
  searchPlaceholder: string;
  columns: WorkbenchColumn<Row>[];
  sortAccessors?: Readonly<Record<string, SortAccessor<Row>>>;
  defaultSort?: WorkbenchSort;
  rowAttention?: (row: Row) => boolean;
  renderDetail: (row: Row, close: () => void) => ReactNode;
  viewState: TemplateViewState;
  stateOverride?: StateSlotProps['override'];
  onRefresh?: () => void;
  /** 读口分页时的游标（useCatalogueCursor 交回的那一份）；给了就在工具条右侧摆上一页 / 下一页。 */
  pagination?: ListCursorPaginationProps;
  noun?: string;
}

export function RouteWorkbench<Row>({
  title,
  subtitle,
  sections,
  extraFilters,
  primaryAction,
  answered,
  rows,
  rowKey,
  statusOf,
  searchCells,
  search: externalSearch,
  searchPlaceholder,
  columns,
  sortAccessors,
  defaultSort,
  rowAttention,
  renderDetail,
  viewState,
  stateOverride,
  onRefresh,
  pagination,
  noun = '条',
}: RouteWorkbenchProps<Row>) {
  const [localSearch, setLocalSearch] = useState('');
  const [status, setStatus] = useState('ALL');
  const [sort, setSort] = useState<WorkbenchSort>(defaultSort ?? { key: '', dir: 1 });
  const [selectedKey, setSelectedKey] = useState<string | null>(null);

  const search = externalSearch ?? { value: localSearch, onChange: setLocalSearch };
  const pushedDown = externalSearch?.pushedDown === true;
  const statusLabel = (row: Row) => statusOf?.(row)?.label;
  const counts = statusOf ? countByStatus(rows.filter((row) => statusOf(row) !== undefined), (row) => statusOf(row)!) : [];
  const filtered = rows.filter(
    (row) =>
      (status === 'ALL' || statusLabel(row) === status) &&
      (pushedDown || matchesSearch(search.value, [...searchCells(row), statusLabel(row)])),
  );
  const visible = sortAccessors ? sortRowsBy(filtered, sort, sortAccessors) : filtered;
  const filter = answered && statusOf ? statusFilterParts(counts, rows.length, status, setStatus) : undefined;
  const close = () => setSelectedKey(null);

  const summary = answered ? `共 ${rows.length} ${noun}${visible.length === rows.length ? '' : `，当前显示 ${visible.length} ${noun}`}` : undefined;

  return (
    <WorkbenchPageTemplate<Row>
      title={title}
      subtitle={subtitle}
      kpis={filter?.kpis}
      onRefresh={onRefresh}
      primaryAction={primaryAction}
      search={{ value: search.value, onChange: search.onChange, placeholder: searchPlaceholder }}
      statusChips={filter?.chips}
      extraFilters={
        sections || extraFilters ? (
          <>
            {sections ? <div className="flex flex-wrap items-center gap-1 border-l border-idpxyz-border pl-3">{sections}</div> : null}
            {extraFilters}
          </>
        ) : undefined
      }
      toolbarRight={
        pagination ? (
          <span className="inline-flex items-center gap-2">
            {summary ? <span>{summary}</span> : null}
            <CursorPager pagination={pagination} />
          </span>
        ) : (
          summary
        )
      }
      columns={columns}
      rows={visible}
      rowKey={rowKey}
      selectedKey={selectedKey}
      onSelect={setSelectedKey}
      rowAttention={rowAttention}
      sort={sortAccessors ? sort : undefined}
      onSort={sortAccessors ? (key) => setSort(toggleWorkbenchSort(sort, key)) : undefined}
      noMatchText="当前筛选条件下没有匹配的记录"
      onClearFilters={() => {
        search.onChange('');
        setStatus('ALL');
      }}
      renderDetail={(key) => {
        const row = visible.find((candidate) => rowKey(candidate) === key);
        return row ? renderDetail(row, close) : null;
      }}
      viewState={viewState}
      stateOverride={stateOverride}
    />
  );
}

/** 读口分页的上一页 / 下一页：页码与总数照答复回显，末页（next 为 null）下一页钮不能按。 */
function CursorPager({ pagination }: { pagination: ListCursorPaginationProps }) {
  if (pagination.page <= 1 && pagination.next === null) return null;
  return (
    <span className="inline-flex items-center gap-1">
      <Button variant="ghost" size="sm" className="h-6 px-2 type-body" disabled={pagination.page <= 1} onClick={pagination.onPrevious}>
        上一页
      </Button>
      <span>第 {pagination.page} 页</span>
      <Button
        variant="ghost"
        size="sm"
        className="h-6 px-2 type-body"
        disabled={pagination.next === null}
        onClick={() => pagination.next !== null && pagination.onNext(pagination.next)}
      >
        下一页
      </Button>
    </span>
  );
}

/** 切册签：与工具条的状态胶囊同一种按钮样子，当前册高亮。 */
export function SectionTabs<Id extends string>({
  sections,
  active,
  onChange,
}: {
  sections: readonly { id: Id; label: string }[];
  active: Id;
  onChange: (id: Id) => void;
}) {
  return (
    <>
      {sections.map((section) => (
        <button
          key={section.id}
          type="button"
          aria-pressed={section.id === active}
          onClick={() => onChange(section.id)}
          className={`rounded px-2 py-1 type-body transition-colors ${
            section.id === active
              ? 'bg-idpxyz-activeItem font-medium text-idpxyz-textBright ring-1 ring-idpxyz-accent'
              : 'text-idpxyz-breadcrumb hover:bg-idpxyz-hover hover:text-idpxyz-text'
          }`}
        >
          {section.label}
        </button>
      ))}
    </>
  );
}

export interface DetailFact {
  label: string;
  value: ReactNode;
}

/**
 * 右栏详情：头部是对象名与状态胶囊，下面几格业务事实，正文人话，动作区，底部「技术详情」收编号、版本、引用与原码。
 * 形状照异常区的 ExceptionDetailPanel 与物流动态的 TrackingDetailPanel。
 */
export function RecordDetailPanel({
  title,
  mono = true,
  status,
  subtitle,
  facts,
  children,
  actions,
  technical,
  onClose,
}: {
  title: string;
  mono?: boolean;
  status?: RowStatus;
  subtitle?: ReactNode;
  facts: DetailFact[];
  children?: ReactNode;
  actions?: ReactNode;
  technical: TechnicalRow[];
  onClose: () => void;
}) {
  return (
    <div className="flex h-full w-full flex-col bg-idpxyz-bg" aria-label={title}>
      <div className="border-b border-idpxyz-border px-5 pb-3 pt-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2
                className={`truncate type-title font-semibold text-idpxyz-textBright ${mono ? 'font-mono' : ''}`.trim()}
                title={title}
              >
                {title}
              </h2>
              {status ? <RowStatusPill status={status} /> : null}
            </div>
            {subtitle ? <p className="mt-0.5 type-body text-idpxyz-breadcrumb">{subtitle}</p> : null}
          </div>
          <Button
            className="shrink-0 text-idpxyz-breadcrumb"
            variant="ghost"
            size="icon"
            onClick={onClose}
            aria-label="关闭详情"
            title="关闭（Esc）"
          >
            <X className="h-4 w-4" aria-hidden="true" />
          </Button>
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
        {facts.length > 0 ? (
          <dl className="grid grid-cols-2 gap-2.5">
            {facts.map((fact) => (
              <div key={fact.label} className="min-w-0 rounded-md border border-idpxyz-border bg-idpxyz-sidebar px-3 py-2">
                <dt className="type-body text-idpxyz-breadcrumb">{fact.label}</dt>
                <dd className="mt-0.5 break-words type-body font-semibold leading-snug text-idpxyz-textBright">{fact.value}</dd>
              </div>
            ))}
          </dl>
        ) : null}
        {children ? <div className="mt-4 space-y-2 type-body leading-5 text-idpxyz-breadcrumb">{children}</div> : null}
        {actions ? <div className="mt-4">{actions}</div> : null}
        <TechnicalDetails className="mt-4" rows={technical} />
      </div>
    </div>
  );
}

/**
 * 还没接进管理台的册（存储上没有登记表，查询口刻意不收这一册）：说「还不能在这里查看」，与「没有记录」和「接入渠道未配置」
 * 都分开。共享的未配置状态件会把 ACCESS_CHANNEL_NOT_CONFIGURED 折进技术详情——那个码说的是这一口没接渠道，不是这一册没建，
 * 所以这里走状态位的覆盖，不改共享件。
 */
export function notConnectedOverride(title: string, description: string): StateSlotProps['override'] {
  return { unconfigured: () => <EmptyState title={title} description={description} /> };
}

/** 版本、引用这类可缺席的技术行：空的不进折叠。 */
export function techRows(rows: readonly { label: string; value: string | undefined }[]): TechnicalRow[] {
  return rows.filter((row): row is TechnicalRow => (row.value ?? '').trim() !== '');
}
