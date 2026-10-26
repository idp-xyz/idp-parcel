import { useEffect, useState, type ReactNode } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { primaryActionClass } from '../../components/primary-action';
import { OperatorRefusalNote } from '../../components/states';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  decideReroute,
  listRerouteSuggestions,
  type ApiResult,
  type RerouteDecisionResponseBody,
  type RerouteSuggestionListResponseBody,
  type RerouteSuggestionRecord,
} from './api';
import { labelOf } from './presentation';
import {
  describeRerouteDecisionAnswer,
  rerouteDecisionAvailabilityOf,
  rerouteDecisionProblemNote,
  rerouteDecisionRequestOf,
  rerouteSuggestionStateLabels,
  suggestionCandidateOutcomeLabels,
  type RerouteDecisionDraft,
} from './reroute-decision';

const info = moduleInfoById['route-plans'];

/** 与路由计划页两册共用的列表页头：标题、说明、检索与切册 chip。 */
export interface RoutePlansHeader {
  title: string;
  description: string;
  search: { value: string; onChange: (value: string) => void; placeholder: string };
  filters: ReactNode;
}

/**
 * 路由计划页「改路建议」一册（ADR-0284 决定六）：复核在原计划失效后形成的改路建议，逐份列阻塞、候选与状态；待决定的那几份可择一
 * 合格候选、填决定依据，经授权改路决定口提交（ADR-0284 决定一）。决定方是登录的操作者，本页不收也不送；成不成立由服务端在判断键
 * 锁里按此刻的证据重判，本页的状态只是读到时的大概。
 */
export function RerouteSuggestionsView({ header, keyword }: { header: RoutePlansHeader; keyword: string }) {
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<RerouteSuggestionListResponseBody> | null>(null);
  const [deciding, setDeciding] = useState<RerouteSuggestionRecord | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listRerouteSuggestions().then((loaded) => {
      if (!cancelled) setAnswer(loaded);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const retry = () => setReloadKey((value) => value + 1);
  const suggestions = answer?.kind === 'outcome' ? answer.body.suggestions : [];
  const needle = keyword.trim().toLowerCase();
  const visible = needle
    ? suggestions.filter((row) =>
        [row.correlationId, row.shipmentRequestId, row.declaredParcelId, row.customerAccountId, row.reviewedPlan ?? ''].some(
          (value) => value.toLowerCase().includes(needle),
        ),
      )
    : suggestions;

  const columns: ListColumn<RerouteSuggestionRecord>[] = [
    {
      id: 'declared-parcel',
      header: '申报包裹',
      className: 'font-mono text-xs',
      render: (row) => <span className="text-idpxyz-accent">{row.declaredParcelId}</span>,
    },
    { id: 'correlation', header: '建议所在复核', className: 'font-mono text-xs', render: (row) => row.correlationId },
    { id: 'reviewed-plan', header: '失效的计划', className: 'font-mono text-xs', render: (row) => row.reviewedPlan ?? '—' },
    {
      id: 'blockers',
      header: '为什么没自动',
      className: 'font-mono text-xs',
      render: (row) => (row.blockers.length > 0 ? row.blockers.join('、') : '—'),
    },
    {
      id: 'candidates',
      header: '候选（建议形成时）',
      className: 'min-w-56 text-xs',
      render: (row) => (
        <ul>
          {row.candidates.map((candidate) => (
            <li key={candidate.id}>
              <span className="font-mono">{candidate.id}</span> {labelOf(suggestionCandidateOutcomeLabels, candidate.outcome)}
              {candidate.reason ? <span className="text-idpxyz-textMuted">（{candidate.reason}）</span> : null}
            </li>
          ))}
        </ul>
      ),
    },
    { id: 'suggested-at', header: '建议时点', className: 'min-w-44 font-mono text-xs', render: (row) => formatInstant(row.suggestedAt) },
    {
      id: 'state',
      header: '状态',
      className: 'min-w-44 text-xs',
      render: (row) => {
        const availability = rerouteDecisionAvailabilityOf(row);
        return availability.kind === 'available' ? (
          <Button variant="outline" disabled={deciding !== null} onClick={() => setDeciding(row)}>
            {labelOf(rerouteSuggestionStateLabels, row.state)} · 决定
          </Button>
        ) : (
          <span>
            {labelOf(rerouteSuggestionStateLabels, row.state)}
            <span className="block text-idpxyz-textMuted">{availability.reason}</span>
          </span>
        );
      },
    },
  ];

  return (
    <>
      <ListPageTemplate<RerouteSuggestionRecord>
        {...header}
        filterSummary={answer?.kind === 'outcome' ? `改路建议 ${suggestions.length} 份` : undefined}
        columns={columns}
        rows={visible}
        rowKey={(row) => row.correlationId}
        viewState={catalogueViewState(answer, suggestions.length, retry, {
          module: info,
          endpoint: 'GET /route-reroute-suggestions',
          emptyTitle: '当前租户尚无改路建议',
          emptyDescription: '改路建议由复核在原计划失效、又不满足自动改路条件时形成；没有这样的复核，册就是空的，本页不预置数据。',
        })}
      />
      {deciding ? (
        <RerouteDecisionCard
          suggestion={deciding}
          onClose={() => setDeciding(null)}
          onLanded={() => {
            setDeciding(null);
            retry();
          }}
        />
      ) : null}
    </>
  );
}

type SubmissionState =
  | { kind: 'idle' }
  | { kind: 'incomplete'; reason: string }
  | { kind: 'submitting' }
  | { kind: 'answered'; answer: ApiResult<RerouteDecisionResponseBody> };

function RerouteDecisionCard({
  suggestion,
  onClose,
  onLanded,
}: {
  suggestion: RerouteSuggestionRecord;
  onClose: () => void;
  onLanded: () => void;
}) {
  const availability = rerouteDecisionAvailabilityOf(suggestion);
  const candidates = availability.kind === 'available' ? availability.candidates : [];
  const [draft, setDraft] = useState<RerouteDecisionDraft>({ candidate: candidates[0] ?? '', basis: '' });
  const [state, setState] = useState<SubmissionState>({ kind: 'idle' });

  const submit = () => {
    const built = rerouteDecisionRequestOf(suggestion, draft);
    if (built.kind === 'incomplete') {
      setState({ kind: 'incomplete', reason: built.reason });
      return;
    }
    setState({ kind: 'submitting' });
    void decideReroute(built.request).then((answer) => {
      setState({ kind: 'answered', answer });
      if (answer.kind === 'outcome' && describeRerouteDecisionAnswer(answer.body).landed) onLanded();
    });
  };

  return (
    <Card className="mx-4 mb-4">
      <CardHeader>
        <CardTitle>授权改路决定</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-2 text-xs">
        <p className="text-idpxyz-textMuted">
          包裹 <span className="font-mono">{suggestion.declaredParcelId}</span> 的原计划{' '}
          <span className="font-mono">{suggestion.reviewedPlan ?? '—'}</span> 已失效。择一条建议里合格的候选：服务端按此刻的证据重判它，
          成立时形成改路决定与新计划，从决定的时刻起生效；建议之后又有复核、或此刻被限制淘汰，都不成立。决定方是登录的操作者，本表单
          不收也不送。
        </p>
        <fieldset>
          <legend className="text-idpxyz-textMuted">所择候选</legend>
          {candidates.map((candidate) => (
            <label key={candidate} className="mr-4 inline-flex items-center gap-1 font-mono">
              <input
                type="radio"
                name="reroute-candidate"
                checked={draft.candidate === candidate}
                onChange={() => setDraft({ ...draft, candidate })}
              />
              {candidate}
            </label>
          ))}
        </fieldset>
        <label className="block">
          <span className="text-idpxyz-textMuted">决定依据引用 *</span>
          <Input
            value={draft.basis}
            className="font-mono text-[13px]"
            placeholder="审批单、工单或来电记录的引用，逐字保存、不判真伪"
            onChange={(event) => setDraft({ ...draft, basis: event.target.value })}
          />
        </label>
        <div className="flex items-center gap-3">
          <Button variant="default" className={primaryActionClass} onClick={submit} disabled={state.kind === 'submitting'}>
            {state.kind === 'submitting' ? '提交中…' : '提交决定'}
          </Button>
          <Button variant="outline" onClick={onClose}>
            收起
          </Button>
        </div>
        <DecisionAnswerNote state={state} />
      </CardContent>
    </Card>
  );
}

function DecisionAnswerNote({ state }: { state: SubmissionState }) {
  switch (state.kind) {
    case 'idle':
    case 'submitting':
      return null;
    case 'incomplete':
      return <p className="text-idpxyz-danger">未提交：{state.reason}</p>;
  }
  const answer = state.answer;
  switch (answer.kind) {
    case 'outcome': {
      const note = describeRerouteDecisionAnswer(answer.body);
      return (
        <div className={note.tone === 'answered' ? 'text-idpxyz-textMuted' : 'text-idpxyz-danger'}>
          <p>{note.headline}</p>
          {note.details.length > 0 ? (
            <ul className="mt-0.5 ml-4 list-disc">
              {note.details.map((line) => (
                <li key={line}>{line}</li>
              ))}
            </ul>
          ) : null}
        </div>
      );
    }
    case 'unconfigured':
      return (
        <p className="text-idpxyz-textMuted">
          接入渠道未配置（403 ACCESS_CHANNEL_NOT_CONFIGURED）。决定口已建立并装配，操作者渠道的发行方参数未设，服务端如实拒绝。
        </p>
      );
    case 'loginRequired':
    case 'notGranted':
      return <OperatorRefusalNote kind={answer.kind} what="授权改路" />;
    case 'callerProblem':
      return (
        <p className="text-idpxyz-danger">
          调用方式问题（HTTP {answer.status}）：{rerouteDecisionProblemNote(answer.code)}
        </p>
      );
    case 'noAnswer':
      return (
        <p className="text-idpxyz-danger">
          服务端未形成答案（HTTP {answer.status}）：{rerouteDecisionProblemNote(answer.code)}
        </p>
      );
    case 'transport':
      return <p className="text-idpxyz-danger">请求未到达 parcel-api：{answer.message}</p>;
  }
}
