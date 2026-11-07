import { useState } from 'react';
import {
  WorkbenchPageTemplate,
  toggleWorkbenchSort,
  useAddressKeyword,
  type WorkbenchChip,
  type WorkbenchColumn,
  type WorkbenchSort,
} from '../../templates';
import { moduleInfoById } from '../../navigation';
import { labelFor } from '../../vocabulary/status';
import { objectNouns } from '../../vocabulary/objects';
import { formatInstant } from '../moment';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { CursorPager, pastFirstPage } from '../shipment-request/cursor-pager';
import { listExceptionCaseRecords, type ExceptionCaseRecord } from './case-api';
import { casePhaseCodes } from './case-presentation';
import { ExceptionDetailPanel, RowStatusPill } from './ExceptionDetailPanel';
import { absenceText, caseNumberName, primaryReference } from './document-number';
import { DocumentNumberCell } from './DocumentNumberCell';
import { codeStatus } from './exception-list';
import { exceptionViewState } from './exception-view-state';
import { allStatuses, defaultCaseSort, exceptionCaseConditions, type CasePhaseFilter } from './list-conditions';

// 页面标题与主责出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['exception-cases'];

// 异常案件（visibility-exception），对 GET /exception-case-records 取数；改版换成工作台形态（设计源「五、改版示意」）。
// 列只放人要看的：案件号（单据号，ADR-0355 决定十一）、包裹号（案件的根对象是包裹）、状态胶囊、处理团队、立案时间、首次响应；
// 案件内部标识、影响范围、归并指向与结论原码进右栏「技术详情」。没有单据号的案件主列照旧显内部标识，技术详情说为什么没有。
// 首页「今日待办」异常那一格点进来就落在这一页。
//
// 严重度、处置优先级、当前工作条件与响应周期不上列：案件行在存储上刻意只落精简主生命周期（0008），那几维结构上不存在，
// 列出来只能代填，代填会把工作条件升格成第二套主状态。归并与关闭是案件命令面的判断，本页只查阅、不推进。
// 改版前的「导出所选（CSV）」随列表模板一起退场：工作台没有多选。
//
// 读口已迁到 ADR-0144，按游标翻页（工具条右端）。检索词（地址上的 `?q=`，停稳后）、状态胶囊（phase）与包裹号、立案时间两列的
// 先后下推（list-conditions 的 exceptionCaseConditions）。案件号列不给排：单据号可空，读口不放它作可排维。胶囊不带计数、命令头
// 不摆指标——一页里数出来的不是各状态的总数，当前条件下共几件由翻页条说。

const caseStatus = (record: ExceptionCaseRecord) => codeStatus(record.phase);

/** 关闭结论：并入其他案件经词典；其余结论是开放引用，词典查不到就原样显示。 */
function conclusionText(record: ExceptionCaseRecord): string | undefined {
  return record.conclusion ? labelFor(record.conclusion, 'case-conclusion') : undefined;
}

const columns: WorkbenchColumn<ExceptionCaseRecord>[] = [
  {
    key: 'number',
    header: caseNumberName,
    render: (row) => <DocumentNumberCell documentNumber={row.documentNumber} internal={row.caseId} absence={row.documentNumberAbsence} />,
  },
  {
    key: 'parcel',
    header: objectNouns.parcelNumber,
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
    render: (row) => <span className="truncate font-mono type-body text-idpxyz-text">{row.responsibleTeam}</span>,
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
  const reference = primaryReference(record.documentNumber, record.caseId, record.documentNumberAbsence);
  return (
    <ExceptionDetailPanel
      ariaLabel={`${objectNouns.exceptionCase} ${reference.text}`}
      title={reference.text}
      status={caseStatus(record)}
      subtitle={`立案于 ${formatInstant(record.establishedAt)}`}
      facts={[
        { label: objectNouns.parcelNumber, value: <span className="font-mono">{record.rootParcel}</span> },
        { label: '处理团队', value: <span className="font-mono">{record.responsibleTeam}</span> },
        { label: '首次响应', value: record.firstResponse ? formatInstant(record.firstResponse) : '还没有响应' },
        { label: '关闭时间', value: record.closedAt ? formatInstant(record.closedAt) : '未关闭' },
        { label: '处理结果', value: conclusion ?? '未关闭' },
      ]}
      technical={[
        { label: caseNumberName, value: record.documentNumber ?? absenceText(record.documentNumberAbsence) },
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
  const [phase, setPhase] = useState<CasePhaseFilter>(allStatuses);
  const [sort, setSort] = useState<WorkbenchSort>(defaultCaseSort);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const conditions = exceptionCaseConditions(useSettledKeyword(search), phase, sort);
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listExceptionCaseRecords);
  const rows = answer?.kind === 'outcome' ? answer.body.cases : [];
  const togglePhase = (code: CasePhaseFilter) => setPhase(phase === code ? allStatuses : code);
  const statusChips: WorkbenchChip[] = [
    { id: allStatuses, label: '全部', active: phase === allStatuses, onClick: () => setPhase(allStatuses) },
    ...casePhaseCodes.map((code) => ({ id: code, label: labelFor(code), active: phase === code, onClick: () => togglePhase(code) })),
  ];

  return (
    <WorkbenchPageTemplate<ExceptionCaseRecord>
      title={info.title}
      subtitle="已立案的异常：哪票货出了问题、谁在处理、处理到哪一步。待响应的案件行首亮灯。"
      onRefresh={retry}
      refreshing={answer === null}
      search={{ value: search, onChange: setSearch, placeholder: '搜案件号、包裹号、处理团队…' }}
      statusChips={statusChips}
      toolbarRight={pagination ? <CursorPager pagination={pagination} /> : undefined}
      columns={columns}
      rows={rows}
      rowKey={(row) => row.caseId}
      selectedKey={selectedKey}
      onSelect={setSelectedKey}
      rowAttention={(row) => row.phase === 'AWAITING_RESPONSE'}
      sort={sort}
      onSort={(key) => setSort(toggleWorkbenchSort(sort, key))}
      noMatchText="当前筛选条件下没有匹配的案件"
      onClearFilters={() => {
        setSearch('');
        setPhase(allStatuses);
      }}
      renderDetail={(key) => {
        const record = rows.find((row) => row.caseId === key);
        return record ? <CaseDetail record={record} onClose={() => setSelectedKey(null)} /> : null;
      }}
      viewState={exceptionViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /exception-case-records',
        subject: '异常案件',
        emptyTitle: '还没有异常案件',
        emptyDescription: '异常信号经分诊决定立案后会出现在这里；要看刚进来的异常信号，去「异常处理」。',
        narrowed: catalogueConditionsNarrow(conditions) || pastFirstPage(pagination),
      })}
    />
  );
}
