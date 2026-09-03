import { useEffect, useState } from 'react';
import { Ban, FolderPlus, Link2, Search } from 'lucide-react';
import {
  ReviewFlowTemplate,
  ListPageTemplate,
  type DetailField,
  type ListColumn,
  type ReviewDecisionOption,
} from '../../templates';
import { moduleInfoById } from '../../navigation';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  listExceptionTriageRecords,
  type ApiResult,
  type DispositionRequestRecord,
  type ExceptionTriageListResponseBody,
  type ExceptionTriageRegistry,
  type SignalEpisodeRecord,
} from './case-api';
import {
  caseLabelOf,
  dispositionCancellationLabels,
  dispositionJudgmentLabels,
} from './case-presentation';
import { labelOf, sourceContextLabels, triageOutcomeLabels } from './presentation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['exception-triage'];

/** 分诊结果的决定 id。四结果名单出自 visibility-exception CONTEXT.md「异常分诊」定义。 */
type TriageDecisionId =
  | 'link-existing-case'
  | 'auto-establish-case'
  | 'route-manual-review'
  | 'no-case';

/**
 * 分诊决定集。标签逐字取 CONTEXT.md「异常分诊」的四结果原词，不自造变体、
 * 不折叠为二元「建立案件 / 不建案」。
 *
 * 四项决定全部如实呈现为不可用：本页已接的是查阅面（GET /exception-triage-records，
 * 票 admin-skeleton-closure-batch/06），分诊决定的命令端点未建——查阅面收下决定
 * 就等于让读口长出第二种「处置」语义。依赖说明只挂第一项，避免同一句话渲染四行。
 */
const triageDecisions: ReviewDecisionOption<TriageDecisionId>[] = [
  {
    id: 'link-existing-case',
    label: '关联既有案件',
    variant: 'secondary',
    icon: <Link2 size={13} />,
    disabled: true,
    disabledReason:
      '四项分诊决定共用的命令端点未建；本页当前为查阅面（票 06 阶段一只开查询），决定端点就绪后随接线开放。',
  },
  {
    id: 'auto-establish-case',
    label: '自动建立案件',
    variant: 'default',
    icon: <FolderPlus size={13} />,
    disabled: true,
  },
  {
    id: 'route-manual-review',
    label: '进入人工复核',
    variant: 'secondary',
    icon: <Search size={13} />,
    disabled: true,
  },
  {
    id: 'no-case',
    label: '不建案',
    variant: 'danger',
    icon: <Ban size={13} />,
    disabled: true,
  },
];

type TriageView = 'episodes' | 'requests';

const registryOf: Record<TriageView, ExceptionTriageRegistry> = {
  episodes: 'signal-episode',
  requests: 'disposition-request',
};

const chipClass = (active: boolean) =>
  `px-2.5 py-1 text-[12px] rounded border ${
    active
      ? 'border-idpxyz-accent text-idpxyz-accent'
      : 'border-idpxyz-border text-idpxyz-textMuted hover:bg-idpxyz-hover'
  }`;

/** 发作期详情：逐字段照实转写，缺席按端点在场规则给词（成对缺席即仍活跃/未分诊）。 */
function episodeDetailFields(episode: SignalEpisodeRecord): DetailField[] {
  const fields: DetailField[] = [
    { label: '发作期标识', value: <span className="font-mono">{episode.episodeId}</span> },
    { label: '目标包裹', value: <span className="font-mono">{episode.parcel}</span> },
    { label: '信号类型', value: <span className="font-mono">{episode.kind}</span> },
    { label: '判定规则', value: <span className="font-mono">{episode.rule}</span> },
    { label: '可信度', value: <span className="font-mono">{episode.confidence}</span> },
    { label: '命中次数', value: String(episode.hits) },
    { label: '开始时间', value: formatInstant(episode.startedAt) },
    { label: '最近命中', value: formatInstant(episode.lastHitAt) },
    {
      label: '发作期结束',
      value: episode.endedAt
        ? `${formatInstant(episode.endedAt)}${episode.releaseBasis ? `（依据 ${episode.releaseBasis}）` : ''}`
        : '仍活跃（未结束）',
    },
  ];
  if (episode.priorEpisode) {
    fields.push({
      label: '前一发作期',
      value: <span className="font-mono">{episode.priorEpisode}</span>,
    });
  }
  fields.push({
    label: '分诊结论',
    value: episode.triage
      ? `${labelOf(triageOutcomeLabels, episode.triage.outcome)} · 规则 ${episode.triage.rule} · ${formatInstant(episode.triage.triagedAt)}`
      : '尚未分诊',
  });
  return fields;
}

interface RequestRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

function requestCol(
  id: string,
  header: string,
  options?: { mono?: boolean; align?: 'left' | 'center' | 'right'; className?: string },
): ListColumn<RequestRow> {
  return {
    id,
    header,
    align: options?.align,
    className: options?.className,
    render: (row) =>
      options?.mono ? (
        <span className="font-mono text-[12px]">{row.values[id] ?? '—'}</span>
      ) : (
        (row.values[id] ?? '—')
      ),
  };
}

// 处置请求册的列面：请求六要件、发送与判断、取消与替代照行转写。实际执行结果没有
// 列——那是目标上下文按事实返回的东西，行内没有它的字段（端点注释同句）。
const requestColumns: ListColumn<RequestRow>[] = [
  requestCol('requestId', '请求标识', { mono: true }),
  requestCol('caseId', '关联案件', { mono: true }),
  requestCol('targetContext', '接收上下文', { className: 'w-[88px]' }),
  requestCol('action', '处置动作', { mono: true }),
  requestCol('scope', '处置范围', { mono: true }),
  requestCol('intentVersion', '意图版本', { align: 'right', className: 'w-[72px]', mono: true }),
  requestCol('sentAt', '发送时刻', { mono: true }),
  requestCol('judgment', '源上下文判断'),
  requestCol('cancellation', '取消答复'),
  requestCol('supersededBy', '被替代为', { mono: true }),
];

function requestRowsOf(requests: DispositionRequestRecord[]): RequestRow[] {
  return requests.map((record) => ({
    key: `request:${record.requestId}`,
    values: {
      requestId: record.requestId,
      caseId: record.caseId,
      targetContext: labelOf(sourceContextLabels, record.targetContext),
      action: record.action,
      scope: record.scope,
      intentVersion: String(record.intentVersion),
      sentAt: formatInstant(record.sentAt),
      // 判断两键成对在场；尚无答复整格缺席——「还没答」不演成四走向里的任何一种。
      judgment: record.judgment
        ? `${caseLabelOf(dispositionJudgmentLabels, record.judgment)}${
            record.judgedAt ? ` · ${formatInstant(record.judgedAt)}` : ''
          }`
        : '',
      cancellation: record.cancellation
        ? caseLabelOf(dispositionCancellationLabels, record.cancellation)
        : '',
      supersededBy: record.supersededBy ?? '',
    },
  }));
}

/**
 * 异常分诊与处置协调。队列语义取 visibility-exception CONTEXT.md「异常识别、分诊
 * 与分级」：本页列信号发作期册（连同各自的分诊结论）与处置请求册两本登记册
 * （GET /exception-triage-records，票 admin-skeleton-closure-batch/06）。查阅不
 * 分诊、不建案、不发处置请求——决定区如实禁用，等分诊命令端点。
 */
export function ExceptionTriagePage() {
  const [view, setView] = useState<TriageView>('episodes');
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{
    registry: ExceptionTriageRegistry;
    answer: ApiResult<ExceptionTriageListResponseBody>;
  } | null>(null);
  const registry = registryOf[view];

  useEffect(() => {
    let cancelled = false;
    void listExceptionTriageRecords(registry).then((answer) => {
      if (!cancelled) setLoaded({ registry, answer });
    });
    return () => {
      cancelled = true;
    };
  }, [registry, reloadKey]);

  const answer = loaded?.registry === registry ? loaded.answer : null;
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const retry = () => setReloadKey((value) => value + 1);

  const viewChips = (
    <>
      <button
        type="button"
        className={chipClass(view === 'episodes')}
        onClick={() => setView('episodes')}
      >
        信号发作期
      </button>
      <button
        type="button"
        className={chipClass(view === 'requests')}
        onClick={() => setView('requests')}
      >
        处置请求
      </button>
    </>
  );

  if (view === 'requests') {
    const rows =
      body && body.outcome === 'DISPOSITION_REQUESTS_LISTED' ? requestRowsOf(body.requests) : [];
    const needle = search.trim().toLowerCase();
    const visibleRows = needle
      ? rows.filter((row) =>
          Object.values(row.values).some((value) => value.toLowerCase().includes(needle)),
        )
      : rows;
    return (
      <ListPageTemplate<RequestRow>
        title={info.title}
        description={`${info.owner}——处置请求册：请求一行一版本，被替代的行照列（替代不是删除）；实际执行结果由目标上下文按事实返回，本册无该列`}
        headerActions={viewChips}
        search={{
          value: search,
          onChange: setSearch,
          placeholder: '搜索请求标识 / 案件 / 处置动作',
        }}
        filterSummary={body ? `处置请求 ${visibleRows.length} 行` : undefined}
        columns={requestColumns}
        rows={visibleRows}
        rowKey={(row) => row.key}
        viewState={catalogueViewState(answer, rows.length, retry, {
          module: info,
          endpoint: 'GET /exception-triage-records?registry=disposition-request',
          emptyTitle: '当前租户尚无处置请求',
          emptyDescription:
            '读取入口已配置，登记册为空——处置请求由案件编排发出，事实在接入渠道墙后面，空册是预期不是缺陷。',
        })}
      />
    );
  }

  const episodes = body && body.outcome === 'SIGNAL_EPISODES_LISTED' ? body.episodes : [];
  const selected = episodes.find((episode) => episode.episodeId === selectedId) ?? null;
  return (
    <ReviewFlowTemplate<TriageDecisionId>
      title={info.title}
      description={`${info.owner}——信号发作期册连同各自的分诊结论；分诊决定命令端点未建，决定区如实禁用`}
      headerActions={viewChips}
      queueTitle="进入分诊的信号"
      queue={episodes.map((episode) => ({
        id: episode.episodeId,
        title: episode.parcel,
        subtitle: `${episode.kind} · ${episode.confidence}`,
        // 已分诊示结论原词，未分诊如实说「尚未分诊」——不折成布尔，也不代判。
        status: (
          <span className="shrink-0 text-[11px] text-idpxyz-textMuted">
            {episode.triage ? labelOf(triageOutcomeLabels, episode.triage.outcome) : '尚未分诊'}
          </span>
        ),
        meta: formatInstant(episode.startedAt),
      }))}
      selectedId={selectedId}
      onSelect={setSelectedId}
      detailTitle="发作期详情"
      detailFields={selected ? episodeDetailFields(selected) : undefined}
      decisions={triageDecisions}
      // 四项决定全部 disabled，本回调不可达；分诊命令端点就绪后替换为应用端口调用。
      onDecide={() => {}}
      viewState={catalogueViewState(answer, episodes.length, retry, {
        module: info,
        endpoint: 'GET /exception-triage-records?registry=signal-episode',
        emptyTitle: '当前租户尚无信号发作期',
        emptyDescription:
          '读取入口已配置，登记册为空——信号来自运行时业务事实，事实在接入渠道墙后面，空册是预期不是缺陷。',
      })}
    />
  );
}
