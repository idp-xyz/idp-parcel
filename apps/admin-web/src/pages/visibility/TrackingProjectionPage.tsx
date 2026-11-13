import { Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { RegistrationPanel } from '../../components/registration';
import { etaEntryEndpoint, etaEntryHint, etaEntryOutcomeLabels, etaEntryProblemNote, etaEntryTitle, submitETAEntry } from './eta-entry';
import { useState } from 'react';
import {
  WorkbenchPageTemplate,
  toggleWorkbenchSort,
  useAddressKeyword,
  type TemplateViewState,
  type WorkbenchChip,
  type WorkbenchColumn,
  type WorkbenchSort,
} from '../../templates';
import { moduleInfoById } from '../../navigation';
import { StatusPill } from '../../components/StatusPill';
import { objectNouns } from '../../vocabulary/objects';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { CursorPager, pastFirstPage } from '../shipment-request/cursor-pager';
import { problemNote } from './presentation';
import { listTrackingProjections, type ApiResult, type ProjectionListResponseBody } from './api';
import { TrackingDetailPanel } from './TrackingDetailPanel';
import { defaultTrackingSort, trackingListConditions } from './list-conditions';
import { PageOnlyNote } from './page-only';
import {
  countTrackingByStatus,
  currentStatusLabelOf,
  progressPhrasesOf,
  trackingNoMatchNote,
  trackingNoMatchOnPageNote,
  trackingRowOf,
  trackingRowsOfStatus,
  type TrackingRow,
} from './tracking-list';
import { formatInstant } from '../moment';

// 页面标题、一句话与主责出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['tracking-projection'];

// 物流动态（领域：全程追踪投影），对 GET /tracking-projections 真实端点取数；改版第一步换成工作台形态（设计源「五、改版示意」）。
// 列只放人要看的：包裹号（投影按包裹出）、当前状态（最新里程碑的中文胶囊）、进度（托运 / 路由 / 节点 / 运输 / 关务五段折成一格中文短语，缺的段
// 不显）、更新时间；投影版本、前身、替代关系进右栏详情底部的「技术详情」。
//
// 作用域裁决已落：运营查阅走独立读口消费投影库，不复用客户隔离读口（ADR-0076）。作用域是租户级、无客户维，由服务端接入面从
// 认证结果铸造，本页不送任何自报身份参数；「投影未形成」对租户内已授权查阅是如实的业务答案。
//
// 进度一格仍不是统一状态机：各源各一段、按服务链顺序排在一起只是阅读便利，GLOSSARY「追踪摘要」禁统一状态机那条规则照旧
// （折叠的判据在 tracking-list.ts 的 progressPhrasesOf）。追踪摘要、当前 ETA、可见性缺口、异常影响是本上下文拥有的独立对象，
// 投影读模型未携带，等各自读面落地再上列，不虚构空列。
//
// 读口已迁到 ADR-0144，按游标翻页（工具条右端）。检索词（地址上的 `?q=`，停稳后）与包裹号、更新时间两列的先后下推
// （list-conditions 的 trackingListConditions）。当前状态是从条目派生的词，读口没有这一维：状态胶囊留着、只筛本页，胶囊旁写明；
// 胶囊不带计数、命令头不摆指标——一页里数出来的不是各状态的总数，当前条件下共几件由翻页条说。

const columns: WorkbenchColumn<TrackingRow>[] = [
  {
    key: 'parcel',
    header: objectNouns.parcelNumber,
    sortKey: 'parcel',
    render: (row) => (
      <span className="truncate font-mono font-medium text-[color:var(--ds-text-link-hover)]" title={row.parcelId}>
        {row.parcelId}
      </span>
    ),
  },
  {
    key: 'status',
    header: objectNouns.currentStatus,
    render: (row) =>
      row.latestMilestone ? <StatusPill code={row.latestMilestone} /> : <StatusPill label={currentStatusLabelOf(row)} tone="muted" />,
  },
  {
    key: 'progress',
    header: objectNouns.progress,
    render: (row) => {
      const phrases = progressPhrasesOf(row);
      return phrases.length === 0 ? (
        <span className="text-idpxyz-breadcrumb">—</span>
      ) : (
        <span className="text-idpxyz-text">{phrases.join(' · ')}</span>
      );
    },
  },
  {
    key: 'derived-at',
    header: objectNouns.updatedAt,
    sortKey: 'derived-at',
    hideWhenDetailOpen: true,
    render: (row) => (
      <time className="font-mono text-idpxyz-breadcrumb" dateTime={row.derivedAt} title={row.derivedAt}>
        {formatInstant(row.derivedAt)}
      </time>
    ),
  },
];

// 取数答案 → 模板页面态。空列表走空态而不是就绪态的空表格；带检索词答出零行、翻过第一页答出零行都不是空态（narrowed），
// 留在就绪态，表格区另显 noMatchText。
function viewStateOf(
  answer: ApiResult<ProjectionListResponseBody> | null,
  rowCount: number,
  retry: () => void,
  narrowed: boolean,
): TemplateViewState {
  if (answer === null) return { kind: 'loading', shape: 'table' };
  switch (answer.kind) {
    case 'outcome':
      return rowCount === 0 && !narrowed
        ? {
            kind: 'empty',
            title: '还没有物流动态',
            description: '包裹收寄、运输、清关等环节产生动态后，这里就会出现。',
          }
        : { kind: 'ready' };
    case 'unconfigured':
      return {
        kind: 'unconfigured',
        title: '接入渠道未配置',
        description: '物流动态的接入渠道还没配置好，这不是故障：重试不会改变结果，需要先完成渠道配置。',
        facts: { owner: info.owner, source: info.source, endpoint: 'GET /tracking-projections' },
      };
    case 'loginRequired':
    case 'notGranted':
      return { kind: answer.kind, facts: { owner: info.owner, source: info.source } };
    case 'callerProblem':
      return {
        kind: 'error',
        title: '这次查询没有被接受',
        // 读口按 ADR-0144 拒掉的查询（集外的键、坏游标、检索词过长）随答一句理由散文，原样接在后面。
        description: answer.detail === undefined ? problemNote(answer.code) : `${problemNote(answer.code)} ${answer.detail}`,
        technical: [
          { label: 'HTTP 状态', value: String(answer.status) },
          { label: '问题码', value: answer.code },
        ],
      };
    case 'noAnswer':
      return {
        kind: 'error',
        title: '这次没有取到物流动态',
        description: problemNote(answer.code),
        onRetry: retry,
        technical: [
          { label: 'HTTP 状态', value: String(answer.status) },
          { label: '问题码', value: answer.code },
        ],
      };
    case 'transport':
      return {
        kind: 'error',
        title: '连不上服务',
        description: '请求没有到达后端；请确认网络与服务在运行后重试。',
        onRetry: retry,
        technical: [{ label: '传输层', value: answer.message }],
      };
  }
}

/**
 * 物流动态页。「物流动态」签是原来的查阅面；「录入预计到达」签给没有预测来源对接的租户：把承运商告知的或自己估的
 * 预计到达录进来。
 */
export function TrackingProjectionPage() {
  return (
    <Tabs defaultValue="projections" className="flex-1 flex flex-col overflow-hidden gap-0 bg-idpxyz-editor">
      <TabsList className="px-4 shrink-0">
        <TabsTrigger value="projections">物流动态</TabsTrigger>
        <TabsTrigger value="eta-entry">录入预计到达</TabsTrigger>
      </TabsList>
      <TabsContent value="projections" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
        <TrackingProjections />
      </TabsContent>
      <TabsContent value="eta-entry" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
        <RegistrationPanel
          moduleId="tracking-projection"
          title={etaEntryTitle}
          endpoint={`POST ${etaEntryEndpoint}`}
          snapshotHint={etaEntryHint}
          submit={submitETAEntry}
          outcomeLabels={etaEntryOutcomeLabels}
          problemNote={etaEntryProblemNote}
        />
      </TabsContent>
    </Tabs>
  );
}

function TrackingProjections() {
  const [search, setSearch] = useAddressKeyword();
  const [status, setStatus] = useState<'ALL' | string>('ALL');
  const [sort, setSort] = useState<WorkbenchSort>(defaultTrackingSort);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const conditions = trackingListConditions(useSettledKeyword(search), sort);
  // null 表示取数中；答案（含各种未形成）一律进 answer，页面不吞任何一格。换条件回第一页、翻页与重取都在 useCatalogueCursor 里。
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listTrackingProjections);

  const rows = answer?.kind === 'outcome' ? answer.body.projections.map(trackingRowOf) : [];
  const answered = answer?.kind === 'outcome';
  const visible = trackingRowsOfStatus(rows, status);
  // 胶囊只列这一页上出现过的状态词（里程碑是租户登记的开放词表）；选中的那一格翻到没有它的页也留着，好点掉。
  const labels = countTrackingByStatus(rows).map((item) => item.label);
  if (status !== 'ALL' && !labels.includes(status)) labels.push(status);
  const toggleStatus = (label: string) => setStatus(status === label ? 'ALL' : label);
  const statusChips: WorkbenchChip[] | undefined = answered
    ? [
        { id: 'ALL', label: '全部', active: status === 'ALL', onClick: () => setStatus('ALL') },
        ...labels.map((label) => ({ id: label, label, active: status === label, onClick: () => toggleStatus(label) })),
      ]
    : undefined;

  return (
    <WorkbenchPageTemplate<TrackingRow>
      title={info.title}
      subtitle={info.summary}
      onRefresh={retry}
      refreshing={answer === null}
      search={{ value: search, onChange: setSearch, placeholder: '搜包裹号…' }}
      statusChips={statusChips}
      extraFilters={answered ? <PageOnlyNote>状态只筛本页</PageOnlyNote> : undefined}
      toolbarRight={pagination ? <CursorPager pagination={pagination} /> : undefined}
      columns={columns}
      rows={visible}
      rowKey={(row) => row.projectionVersion}
      selectedKey={selectedKey}
      onSelect={setSelectedKey}
      sort={sort}
      onSort={(key) => setSort(toggleWorkbenchSort(sort, key))}
      noMatchText={rows.length > 0 ? trackingNoMatchOnPageNote : trackingNoMatchNote}
      onClearFilters={() => {
        setSearch('');
        setStatus('ALL');
      }}
      renderDetail={(key) => {
        const row = visible.find((item) => item.projectionVersion === key);
        return row ? <TrackingDetailPanel row={row} onClose={() => setSelectedKey(null)} /> : null;
      }}
      viewState={viewStateOf(answer, rows.length, retry, catalogueConditionsNarrow(conditions) || pastFirstPage(pagination))}
    />
  );
}
