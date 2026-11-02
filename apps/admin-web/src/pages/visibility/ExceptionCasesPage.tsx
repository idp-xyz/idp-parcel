import { useEffect, useState } from 'react';
import {
  WorkbenchPageTemplate,
  toggleWorkbenchSort,
  useAddressKeyword,
  type WorkbenchColumn,
  type WorkbenchSort,
} from '../../templates';
import { moduleInfoById } from '../../navigation';
import { labelFor } from '../../vocabulary/status';
import { objectNouns } from '../../vocabulary/objects';
import { formatInstant } from '../moment';
import { matchesSearch } from '../party/list-order';
import { listExceptionCaseRecords, type ApiResult, type ExceptionCaseListResponseBody, type ExceptionCaseRecord } from './case-api';
import { ExceptionDetailPanel, RowStatusPill } from './ExceptionDetailPanel';
import { numberlessNote, primaryReference } from './document-number';
import { codeStatus, countByStatus, sortRowsBy, type SortAccessor } from './exception-list';
import { exceptionViewState } from './exception-view-state';
import { statusFilterParts } from './status-filter';

// 页面标题与主责出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['exception-cases'];

// 异常案件（visibility-exception），对 GET /exception-case-records 取数；改版换成工作台形态（设计源「五、改版示意」）。
// 列只放人要看的：案件号（单据号，ADR-0355 决定十一）、运单号（案件的根对象是包裹）、状态胶囊、处理团队、立案时间、首次响应；
// 案件内部标识、影响范围、归并指向与结论原码进右栏「技术详情」。没有单据号的案件主列照旧显内部标识，技术详情说为什么没有。
// 首页「今日待办」异常那一格点进来就落在这一页。
//
// 严重度、处置优先级、当前工作条件与响应周期不上列：案件行在存储上刻意只落精简主生命周期（0008），那几维结构上不存在，
// 列出来只能代填，代填会把工作条件升格成第二套主状态。归并与关闭是案件命令面的判断，本页只查阅、不推进。
// 改版前的「导出所选（CSV）」随列表模板一起退场：工作台没有多选。

const caseStatus = (record: ExceptionCaseRecord) => codeStatus(record.phase);

const sortAccessors: Record<string, SortAccessor<ExceptionCaseRecord>> = {
  number: { value: (row) => row.documentNumber ?? row.caseId },
  parcel: { value: (row) => row.rootParcel },
  established: { value: (row) => row.establishedAt, instant: true },
};

const defaultSort: WorkbenchSort = { key: 'established', dir: -1 };

/** 关闭结论：并入其他案件经词典；其余结论是开放引用，词典查不到就原样显示。 */
function conclusionText(record: ExceptionCaseRecord): string | undefined {
  return record.conclusion ? labelFor(record.conclusion, 'case-conclusion') : undefined;
}

const columns: WorkbenchColumn<ExceptionCaseRecord>[] = [
  {
    key: 'number',
    header: objectNouns.exceptionCaseNumber,
    sortKey: 'number',
    render: (row) => {
      const reference = primaryReference(row.documentNumber, row.caseId, row.numberlessReason);
      return reference.numbered ? (
        <span className="truncate font-mono font-medium text-idpxyz-textBright" title={reference.text}>
          {reference.text}
        </span>
      ) : (
        <span className="truncate font-mono text-[12px] text-idpxyz-breadcrumb" title={`${reference.text}（${reference.hint}）`}>
          {reference.text}
        </span>
      );
    },
  },
  {
    key: 'parcel',
    header: objectNouns.shipmentNumber,
    sortKey: 'parcel',
    render: (row) => (
      <span className="truncate font-mono font-medium text-[color:var(--ds-text-link-hover)]" title={row.rootParcel}>
        {row.rootParcel}
      </span>
    ),
  },
  { key: 'status', header: objectNouns.status, render: (row) => <RowStatusPill status={caseStatus(row)} /> },
  {
    key: 'team',
    header: '处理团队',
    render: (row) => <span className="truncate font-mono text-[12px] text-idpxyz-text">{row.responsibleTeam}</span>,
  },
  {
    key: 'established',
    header: '立案时间',
    sortKey: 'established',
    render: (row) => (
      <time className="font-mono text-idpxyz-breadcrumb" dateTime={row.establishedAt}>
        {formatInstant(row.establishedAt)}
      </time>
    ),
  },
  {
    key: 'first-response',
    header: '首次响应',
    hideWhenDetailOpen: true,
    render: (row) =>
      row.firstResponse ? (
        <time className="font-mono text-idpxyz-breadcrumb" dateTime={row.firstResponse}>
          {formatInstant(row.firstResponse)}
        </time>
      ) : (
        <span className="text-idpxyz-breadcrumb">还没有响应</span>
      ),
  },
];

function CaseDetail({ record, onClose }: { record: ExceptionCaseRecord; onClose: () => void }) {
  const conclusion = conclusionText(record);
  const reference = primaryReference(record.documentNumber, record.caseId, record.numberlessReason);
  return (
    <ExceptionDetailPanel
      ariaLabel={`${objectNouns.exceptionCase} ${reference.text}`}
      title={reference.text}
      status={caseStatus(record)}
      subtitle={`立案于 ${formatInstant(record.establishedAt)}`}
      facts={[
        { label: objectNouns.shipmentNumber, value: <span className="font-mono">{record.rootParcel}</span> },
        { label: '处理团队', value: <span className="font-mono">{record.responsibleTeam}</span> },
        { label: '首次响应', value: record.firstResponse ? formatInstant(record.firstResponse) : '还没有响应' },
        { label: '关闭时间', value: record.closedAt ? formatInstant(record.closedAt) : '未关闭' },
        { label: '处理结果', value: conclusion ?? '未关闭' },
      ]}
      technical={[
        { label: objectNouns.exceptionCaseNumber, value: record.documentNumber ?? numberlessNote(record.numberlessReason) },
        { label: '案件内部标识', value: record.caseId },
        { label: '包裹标识', value: record.rootParcel },
        { label: '影响范围', value: record.impactScope },
        { label: '团队编码', value: record.responsibleTeam },
        { label: '状态码', value: record.phase },
        { label: '结论码', value: record.conclusion ?? '' },
        { label: '并入案件', value: record.mergedInto ?? '' },
      ]}
      onClose={onClose}
    >
      {record.mergedInto ? (
        <p>这件案件已并入另一件案件继续处理；原案件、它的案件号和处理记录都保留，并入的案件在下方技术详情里。</p>
      ) : null}
      <p>关闭案件不会改动运单上已经发生的事；要重新处理时，案件重新打开，之前的记录都保留。</p>
    </ExceptionDetailPanel>
  );
}

export function ExceptionCasesPage() {
  const [search, setSearch] = useAddressKeyword();
  const [status, setStatus] = useState<string>('ALL');
  const [sort, setSort] = useState<WorkbenchSort>(defaultSort);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [answer, setAnswer] = useState<ApiResult<ExceptionCaseListResponseBody> | null>(null);
  const [reloadToken, setReloadToken] = useState(0);
  const [refreshing, setRefreshing] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setAnswer((previous) => (previous?.kind === 'outcome' ? previous : null));
    void listExceptionCaseRecords().then((result) => {
      if (cancelled) return;
      setAnswer(result);
      setRefreshing(false);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadToken]);

  const retry = () => {
    setRefreshing(true);
    setReloadToken((token) => token + 1);
  };
  const answered = answer?.kind === 'outcome';
  const rows = answer?.kind === 'outcome' ? answer.body.cases : [];
  const counts = countByStatus(rows, caseStatus);
  const visible = sortRowsBy(
    rows.filter(
      (row) =>
        (status === 'ALL' || caseStatus(row).label === status) &&
        matchesSearch(search, [row.documentNumber, row.rootParcel, row.caseId, row.responsibleTeam, caseStatus(row).label]),
    ),
    sort,
    sortAccessors,
  );
  const filter = answered ? statusFilterParts(counts, rows.length, status, setStatus) : undefined;

  return (
    <WorkbenchPageTemplate<ExceptionCaseRecord>
      title={info.title}
      subtitle="已立案的异常：哪票货出了问题、谁在处理、处理到哪一步。待响应的案件行首亮灯。"
      kpis={filter?.kpis}
      onRefresh={retry}
      refreshing={refreshing}
      search={{ value: search, onChange: setSearch, placeholder: '搜案件号、运单号、处理团队…' }}
      statusChips={filter?.chips}
      toolbarRight={answered ? `共 ${rows.length} 件，当前显示 ${visible.length} 件` : undefined}
      columns={columns}
      rows={visible}
      rowKey={(row) => row.caseId}
      selectedKey={selectedKey}
      onSelect={setSelectedKey}
      rowAttention={(row) => row.phase === 'AWAITING_RESPONSE'}
      sort={sort}
      onSort={(key) => setSort(toggleWorkbenchSort(sort, key))}
      noMatchText="当前筛选条件下没有匹配的案件"
      onClearFilters={() => {
        setSearch('');
        setStatus('ALL');
      }}
      renderDetail={(key) => {
        const record = visible.find((row) => row.caseId === key);
        return record ? <CaseDetail record={record} onClose={() => setSelectedKey(null)} /> : null;
      }}
      viewState={exceptionViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /exception-case-records',
        subject: '异常案件',
        emptyTitle: '还没有异常案件',
        emptyDescription: '异常信号经分诊决定立案后会出现在这里；要看刚进来的异常信号，去「异常处理」。',
      })}
    />
  );
}
