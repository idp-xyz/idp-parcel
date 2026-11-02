import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import {
  WorkbenchPageTemplate,
  toggleWorkbenchSort,
  useAddressKeyword,
  type WorkbenchColumn,
  type WorkbenchSort,
} from '../../templates';
import { moduleInfoById } from '../../navigation';
import { StatusPill } from '../../components/StatusPill';
import type { TechnicalRow } from '../../components/TechnicalDetails';
import { labelFor } from '../../vocabulary/status';
import { objectNouns } from '../../vocabulary/objects';
import { formatInstant } from '../moment';
import { matchesSearch } from '../party/list-order';
import {
  listExceptionTriageRecords,
  type ApiResult,
  type DispositionRequestRecord,
  type ExceptionTriageRegistry,
  type SignalEpisodeRecord,
} from './case-api';
import { handlerLabel } from './case-presentation';
import { ExceptionDetailPanel, RowStatusPill } from './ExceptionDetailPanel';
import { countByStatus, dispositionStatus, episodeStatus, sortRowsBy, type SortAccessor } from './exception-list';
import { exceptionViewState } from './exception-view-state';
import { statusFilterParts } from './status-filter';
import { sourceContextLabels } from './presentation';

// 页面标题与主责出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['exception-triage'];

// 异常处理（visibility-exception「异常识别、分诊与分级」与处置协调），对 GET /exception-triage-records 两本册取数，
// 改版换成两签工作台：「异常信号」是进来的信号连同分诊结论，「处置请求」是案件发给各环节的处置。
//
// 改版前信号签是复核模板，四个分诊决定按钮全部禁用（分诊决定的命令端点未建）。一排永远点不动的按钮对运营只是噪音，
// 改成右栏一句话说清楚「分诊按规则自动做，这里查看」；端点建成后再把决定区接回来。
// 信号类型、可信度、处置动作是租户登记的开放引用，经词典分册取参考译法，查不到的码原样显示；规则、因果条件、事实依据、
// 各种编号进右栏「技术详情」。

const episodeKindScope = 'exception-signal';
const confidenceScope = 'signal-confidence';
const actionScope = 'disposition-action';
// 存量发作期的因果条件（VE 迁移 0029 回填的那一格）：不是一个真条件，不进技术详情。
const recordedBeforeCauses = 'RECORDED_BEFORE_CAUSAL_CONDITION';

/** 一本册的取数：挂载取一次，刷新时保留上一份答案不闪空。两册行形状不同，Body 由调用方按册给。 */
function useTriageRegistry<Body>(registry: ExceptionTriageRegistry) {
  const [answer, setAnswer] = useState<ApiResult<Body> | null>(null);
  const [reloadToken, setReloadToken] = useState(0);
  const [refreshing, setRefreshing] = useState(false);
  useEffect(() => {
    let cancelled = false;
    setAnswer((previous) => (previous?.kind === 'outcome' ? previous : null));
    void listExceptionTriageRecords(registry).then((result) => {
      if (cancelled) return;
      setAnswer(result as ApiResult<Body>);
      setRefreshing(false);
    });
    return () => {
      cancelled = true;
    };
  }, [registry, reloadToken]);
  const retry = () => {
    setRefreshing(true);
    setReloadToken((token) => token + 1);
  };
  return { answer, retry, refreshing };
}

// —— 异常信号签 ——

const episodeSort: Record<string, SortAccessor<SignalEpisodeRecord>> = {
  parcel: { value: (row) => row.parcel },
  started: { value: (row) => row.startedAt, instant: true },
  'last-hit': { value: (row) => row.lastHitAt, instant: true },
};

const episodeColumns: WorkbenchColumn<SignalEpisodeRecord>[] = [
  {
    key: 'parcel',
    header: objectNouns.shipmentNumber,
    sortKey: 'parcel',
    render: (row) => (
      <span className="truncate font-mono font-medium text-[color:var(--ds-text-link-hover)]" title={row.parcel}>
        {row.parcel}
      </span>
    ),
  },
  { key: 'kind', header: objectNouns.exception, render: (row) => <StatusPill code={row.kind} scope={episodeKindScope} /> },
  { key: 'status', header: '分诊', render: (row) => <RowStatusPill status={episodeStatus(row)} /> },
  {
    key: 'started',
    header: '发生时间',
    sortKey: 'started',
    render: (row) => (
      <time className="font-mono text-idpxyz-breadcrumb" dateTime={row.startedAt}>
        {formatInstant(row.startedAt)}
      </time>
    ),
  },
  {
    key: 'last-hit',
    header: '最近一次',
    sortKey: 'last-hit',
    hideWhenDetailOpen: true,
    render: (row) => (
      <span className="text-idpxyz-breadcrumb">
        <time className="font-mono" dateTime={row.lastHitAt}>
          {formatInstant(row.lastHitAt)}
        </time>
        {row.hits > 1 ? ` · 共 ${row.hits} 次` : ''}
      </span>
    ),
  },
];

function episodeTechnical(episode: SignalEpisodeRecord): TechnicalRow[] {
  return [
    { label: '信号编号', value: episode.episodeId },
    { label: '包裹标识', value: episode.parcel },
    { label: '异常类型码', value: episode.kind },
    { label: '可信度码', value: episode.confidence },
    { label: '识别规则', value: episode.rule },
    { label: '因果条件', value: episode.causalCondition === recordedBeforeCauses ? '' : episode.causalCondition },
    ...episode.factBasis.map((basis) => ({
      label: `${sourceContextLabels[basis.source] ?? basis.source}事实`,
      value: `${basis.fact} @ ${basis.version}${basis.judgement ? ` · 判断 ${basis.judgement}` : ''}`,
    })),
    { label: '上一次信号', value: episode.priorEpisode ?? '' },
    { label: '结束依据', value: episode.releaseBasis ?? '' },
    { label: '分诊规则', value: episode.triage?.rule ?? '' },
    { label: '分诊结果码', value: episode.triage?.outcome ?? '' },
  ];
}

function EpisodeDetail({ episode, onClose }: { episode: SignalEpisodeRecord; onClose: () => void }) {
  const status = episodeStatus(episode);
  return (
    <ExceptionDetailPanel
      ariaLabel={`${objectNouns.exception} ${episode.parcel}`}
      title={episode.parcel}
      status={status}
      subtitle={`${labelFor(episode.kind, episodeKindScope)} · 开始于 ${formatInstant(episode.startedAt)}`}
      facts={[
        { label: '可信度', value: labelFor(episode.confidence, confidenceScope) },
        { label: '发生次数', value: `${episode.hits} 次` },
        { label: '最近一次', value: formatInstant(episode.lastHitAt) },
        { label: '是否还在发生', value: episode.endedAt ? `已于 ${formatInstant(episode.endedAt)} 结束` : '仍在发生' },
        { label: '分诊时间', value: episode.triage ? formatInstant(episode.triage.triagedAt) : '还没有分诊' },
      ]}
      technical={episodeTechnical(episode)}
      onClose={onClose}
    >
      <p>
        {episode.triage ? `分诊结论：${status.label}。` : '这条信号还没有分诊结论。'}
        信号按「设置 · 追踪规则」里的分诊规则自动分诊；管理台里还不能手工分诊，这里只能查看。
      </p>
      {episode.factBasis.length === 0 ? <p>这条信号记录得较早，当时没有保存它依据的业务事实。</p> : null}
    </ExceptionDetailPanel>
  );
}

function SignalEpisodesWorkbench() {
  const [search, setSearch] = useAddressKeyword();
  const [status, setStatus] = useState<string>('ALL');
  const [sort, setSort] = useState<WorkbenchSort>({ key: 'started', dir: -1 });
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const { answer, retry, refreshing } = useTriageRegistry<{ episodes: SignalEpisodeRecord[] }>('signal-episode');
  const answered = answer?.kind === 'outcome';
  const rows = answer?.kind === 'outcome' ? answer.body.episodes : [];
  const counts = countByStatus(rows, episodeStatus);
  const visible = sortRowsBy(
    rows.filter(
      (row) =>
        (status === 'ALL' || episodeStatus(row).label === status) &&
        matchesSearch(search, [row.parcel, row.episodeId, row.kind, labelFor(row.kind, episodeKindScope)]),
    ),
    sort,
    episodeSort,
  );
  const filter = answered ? statusFilterParts(counts, rows.length, status, setStatus) : undefined;

  return (
    <WorkbenchPageTemplate<SignalEpisodeRecord>
      title={info.title}
      subtitle="运输、清关、派送中识别出的异常信号，以及每条信号的分诊结论。待分诊的信号行首亮灯。"
      kpis={filter?.kpis}
      onRefresh={retry}
      refreshing={refreshing}
      search={{ value: search, onChange: setSearch, placeholder: '搜运单号、异常类型…' }}
      statusChips={filter?.chips}
      toolbarRight={answered ? `共 ${rows.length} 条，当前显示 ${visible.length} 条` : undefined}
      columns={episodeColumns}
      rows={visible}
      rowKey={(row) => row.episodeId}
      selectedKey={selectedKey}
      onSelect={setSelectedKey}
      rowAttention={(row) => !row.triage}
      sort={sort}
      onSort={(key) => setSort(toggleWorkbenchSort(sort, key))}
      noMatchText="当前筛选条件下没有匹配的异常信号"
      onClearFilters={() => {
        setSearch('');
        setStatus('ALL');
      }}
      renderDetail={(key) => {
        const episode = visible.find((row) => row.episodeId === key);
        return episode ? <EpisodeDetail episode={episode} onClose={() => setSelectedKey(null)} /> : null;
      }}
      viewState={exceptionViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /exception-triage-records?registry=signal-episode',
        subject: '异常信号',
        emptyTitle: '还没有异常信号',
        emptyDescription: '派送失败、海关查验、轨迹超时这类情况被识别出来后，信号会出现在这里并按规则分诊。',
      })}
    />
  );
}

// —— 处置请求签 ——

const requestSort: Record<string, SortAccessor<DispositionRequestRecord>> = {
  sent: { value: (row) => row.sentAt, instant: true },
};

const requestColumns: WorkbenchColumn<DispositionRequestRecord>[] = [
  {
    key: 'action',
    header: '处置',
    render: (row) => <span className="truncate font-medium text-idpxyz-textBright">{labelFor(row.action, actionScope)}</span>,
  },
  { key: 'handler', header: '交给', render: (row) => handlerLabel(row.targetContext) },
  { key: 'status', header: objectNouns.status, render: (row) => <RowStatusPill status={dispositionStatus(row)} /> },
  {
    key: 'sent',
    header: '发出时间',
    sortKey: 'sent',
    render: (row) => (
      <time className="font-mono text-idpxyz-breadcrumb" dateTime={row.sentAt}>
        {formatInstant(row.sentAt)}
      </time>
    ),
  },
  {
    key: 'cancellation',
    header: '取消',
    hideWhenDetailOpen: true,
    render: (row) => (row.cancellation ? <StatusPill code={row.cancellation} /> : <span className="text-idpxyz-breadcrumb">—</span>),
  },
];

function RequestDetail({ request, onClose }: { request: DispositionRequestRecord; onClose: () => void }) {
  return (
    <ExceptionDetailPanel
      ariaLabel={`处置请求 ${request.requestId}`}
      title={labelFor(request.action, actionScope)}
      status={dispositionStatus(request)}
      subtitle={`交给${handlerLabel(request.targetContext)} · 发出于 ${formatInstant(request.sentAt)}`}
      facts={[
        { label: '交给', value: handlerLabel(request.targetContext) },
        { label: '第几版', value: `第 ${request.intentVersion} 版` },
        { label: '对方答复时间', value: request.judgedAt ? formatInstant(request.judgedAt) : '还没有答复' },
        { label: '答复期限', value: request.acceptanceWindow ? formatInstant(request.acceptanceWindow) : '未设定' },
        { label: '取消', value: request.cancellation ? labelFor(request.cancellation) : '没有取消' },
      ]}
      technical={[
        { label: '请求编号', value: request.requestId },
        { label: '案件编号', value: request.caseId },
        { label: '处置动作码', value: request.action },
        { label: '处置范围', value: request.scope },
        { label: '原因', value: request.reason },
        { label: '证据', value: request.evidence },
        { label: '答复码', value: request.judgment ?? '' },
        { label: '取消答复码', value: request.cancellation ?? '' },
        { label: '被替代为', value: request.supersededBy ?? '' },
      ]}
      onClose={onClose}
    >
      {request.supersededBy ? <p>这项处置已经被新的一版替代，原请求保留在这里备查。</p> : null}
      <p>对方答复的是接不接这项处置；实际办成没有，由接手的环节按实际情况回报，在运单的物流动态里看。</p>
    </ExceptionDetailPanel>
  );
}

function DispositionRequestsWorkbench() {
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<string>('ALL');
  const [sort, setSort] = useState<WorkbenchSort>({ key: 'sent', dir: -1 });
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const { answer, retry, refreshing } = useTriageRegistry<{ requests: DispositionRequestRecord[] }>('disposition-request');
  const answered = answer?.kind === 'outcome';
  const rows = answer?.kind === 'outcome' ? answer.body.requests : [];
  const counts = countByStatus(rows, dispositionStatus);
  const visible = sortRowsBy(
    rows.filter(
      (row) =>
        (status === 'ALL' || dispositionStatus(row).label === status) &&
        matchesSearch(search, [row.requestId, row.caseId, row.action, labelFor(row.action, actionScope), handlerLabel(row.targetContext)]),
    ),
    sort,
    requestSort,
  );
  const filter = answered ? statusFilterParts(counts, rows.length, status, setStatus) : undefined;

  return (
    <WorkbenchPageTemplate<DispositionRequestRecord>
      title="处置请求"
      subtitle="异常案件发给运单、网点、运输、清关等环节的处置，以及对方接不接。被新版替代的请求保留备查。"
      kpis={filter?.kpis}
      onRefresh={retry}
      refreshing={refreshing}
      search={{ value: search, onChange: setSearch, placeholder: '搜处置、案件编号…' }}
      statusChips={filter?.chips}
      toolbarRight={answered ? `共 ${rows.length} 项，当前显示 ${visible.length} 项` : undefined}
      columns={requestColumns}
      rows={visible}
      rowKey={(row) => row.requestId}
      selectedKey={selectedKey}
      onSelect={setSelectedKey}
      rowAttention={(row) => row.judgment === 'SUPPLEMENT_REQUIRED'}
      sort={sort}
      onSort={(key) => setSort(toggleWorkbenchSort(sort, key))}
      noMatchText="当前筛选条件下没有匹配的处置请求"
      onClearFilters={() => {
        setSearch('');
        setStatus('ALL');
      }}
      renderDetail={(key) => {
        const request = visible.find((row) => row.requestId === key);
        return request ? <RequestDetail request={request} onClose={() => setSelectedKey(null)} /> : null;
      }}
      viewState={exceptionViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /exception-triage-records?registry=disposition-request',
        subject: '处置请求',
        emptyTitle: '还没有处置请求',
        emptyDescription: '异常案件需要别的环节动手（改路、退运、补资料）时，发出的请求会出现在这里。',
      })}
    />
  );
}

/** 异常处理：两签各接一本册。只查阅——不分诊、不建案、不发处置请求（案件命令面的判断，命令端点未建）。 */
export function ExceptionTriagePage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="signals" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="signals">异常信号</TabsTrigger>
          <TabsTrigger value="requests">处置请求</TabsTrigger>
        </TabsList>
        <TabsContent value="signals" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <SignalEpisodesWorkbench />
        </TabsContent>
        <TabsContent value="requests" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <DispositionRequestsWorkbench />
        </TabsContent>
      </Tabs>
    </div>
  );
}
