import { useEffect, useState } from 'react';
import {
  WorkbenchPageTemplate,
  toggleWorkbenchSort,
  useAddressKeyword,
  type TemplateViewState,
  type WorkbenchChip,
  type WorkbenchColumn,
  type WorkbenchKpi,
  type WorkbenchSort,
} from '../../templates';
import { moduleInfoById } from '../../navigation';
import { StatusPill } from '../../components/StatusPill';
import { statusToneText } from '../../components/status-tone';
import { toneForLabel, type StatusTone } from '../../vocabulary/status';
import { objectNouns } from '../../vocabulary/objects';
import { problemNote } from './presentation';
import { listTrackingProjections, type ApiResult, type ProjectionListResponseBody } from './api';
import { TrackingDetailPanel } from './TrackingDetailPanel';
import {
  countTrackingByStatus,
  currentStatusLabelOf,
  defaultTrackingSort,
  filterTrackingRows,
  progressPhrasesOf,
  sortTrackingRows,
  trackingCountSummary,
  trackingNoMatchNote,
  trackingRowOf,
  type TrackingRow,
} from './tracking-list';
import { formatInstant } from '../moment';

// 页面标题、一句话与主责出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['tracking-projection'];

// 物流动态（领域：全程追踪投影），对 GET /tracking-projections 真实端点取数；改版第一步换成工作台形态（设计源「五、改版示意」）。
// 列只放人要看的：运单号、当前状态（最新里程碑的中文胶囊）、进度（托运 / 路由 / 节点 / 运输 / 关务五段折成一格中文短语，缺的段
// 不显）、更新时间；投影版本、前身、替代关系进右栏详情底部的「技术详情」。
//
// 作用域裁决已落：运营查阅走独立读口消费投影库，不复用客户隔离读口（ADR-0076）。作用域是租户级、无客户维，由服务端接入面从
// 认证结果铸造，本页不送任何自报身份参数；「投影未形成」对租户内已授权查阅是如实的业务答案。
//
// 进度一格仍不是统一状态机：各源各一段、按服务链顺序排在一起只是阅读便利，GLOSSARY「追踪摘要」禁统一状态机那条规则照旧
// （折叠的判据在 tracking-list.ts 的 progressPhrasesOf）。追踪摘要、当前 ETA、可见性缺口、异常影响是本上下文拥有的独立对象，
// 投影读模型未携带，等各自读面落地再上列，不虚构空列。

const kpiToneClass: Partial<Record<StatusTone, string>> = {
  good: statusToneText.success,
  info: statusToneText.info,
  warn: statusToneText.warning,
  bad: statusToneText.danger,
};

const columns: WorkbenchColumn<TrackingRow>[] = [
  {
    key: 'parcel',
    header: objectNouns.shipmentNumber,
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

// 取数答案 → 模板页面态。空列表走空态而不是就绪态的空表格；筛出为空不是空态——viewState 按总数判，表格区另显 noMatchText。
function viewStateOf(answer: ApiResult<ProjectionListResponseBody> | null, rowCount: number, retry: () => void): TemplateViewState {
  if (answer === null) return { kind: 'loading', shape: 'table' };
  switch (answer.kind) {
    case 'outcome':
      return rowCount === 0
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
        description: problemNote(answer.code),
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

export function TrackingProjectionPage() {
  const [search, setSearch] = useAddressKeyword();
  const [status, setStatus] = useState<'ALL' | string>('ALL');
  const [sort, setSort] = useState<WorkbenchSort>(defaultTrackingSort);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  // null 表示取数中；答案（含各种未形成）一律进 answer，页面不吞任何一格。
  const [answer, setAnswer] = useState<ApiResult<ProjectionListResponseBody> | null>(null);
  const [reloadToken, setReloadToken] = useState(0);
  const [refreshing, setRefreshing] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setAnswer((previous) => (previous?.kind === 'outcome' ? previous : null));
    void listTrackingProjections().then((result) => {
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
  const rows = answer?.kind === 'outcome' ? answer.body.projections.map(trackingRowOf) : [];
  const answered = answer?.kind === 'outcome';
  const visible = sortTrackingRows(filterTrackingRows(rows, { search, status }), sort);
  const counts = countTrackingByStatus(rows);
  const toggleStatus = (label: string) => setStatus(status === label ? 'ALL' : label);

  // 指标即筛：按当前状态的词计数，只列真出现过的；里程碑是租户登记的开放词表，格数随数据走。
  const kpis: WorkbenchKpi[] | undefined = answered
    ? counts.map((item) => ({
        label: item.label,
        value: item.count,
        tone: kpiToneClass[toneForLabel(item.label)],
        active: status === item.label,
        onClick: () => toggleStatus(item.label),
      }))
    : undefined;

  const statusChips: WorkbenchChip[] | undefined = answered
    ? [
        { id: 'ALL', label: '全部', count: rows.length, active: status === 'ALL', onClick: () => setStatus('ALL') },
        ...counts.map((item) => ({
          id: item.label,
          label: item.label,
          count: item.count,
          active: status === item.label,
          onClick: () => toggleStatus(item.label),
        })),
      ]
    : undefined;

  return (
    <WorkbenchPageTemplate<TrackingRow>
      title={info.title}
      subtitle={info.summary}
      kpis={kpis}
      onRefresh={retry}
      refreshing={refreshing}
      search={{ value: search, onChange: setSearch, placeholder: '搜运单号…' }}
      statusChips={statusChips}
      toolbarRight={answered ? trackingCountSummary(rows.length, visible.length) : undefined}
      columns={columns}
      rows={visible}
      rowKey={(row) => row.projectionVersion}
      selectedKey={selectedKey}
      onSelect={setSelectedKey}
      sort={sort}
      onSort={(key) => setSort(toggleWorkbenchSort(sort, key))}
      noMatchText={trackingNoMatchNote}
      onClearFilters={() => {
        setSearch('');
        setStatus('ALL');
      }}
      renderDetail={(key) => {
        const row = visible.find((item) => item.projectionVersion === key);
        return row ? <TrackingDetailPanel row={row} onClose={() => setSelectedKey(null)} /> : null;
      }}
      viewState={viewStateOf(answer, rows.length, retry)}
    />
  );
}
