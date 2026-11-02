import { useEffect, useState, type ReactNode } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { primaryActionClass } from '../../components/primary-action';
import { OperatorRefusalNote } from '../../components/states';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { technicalRows } from '../settlement/ops-ui';
import { useMayMakeDecision } from '../use-operator-grants';
import { ShipmentNo } from '../shipment-request/refs';
import { parcelNoOf } from '../shipment-request/ref-no';
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
  // 两个决定口认同一种运营决定授予（授权改路）；明确没有时只显状态、不摆按钮（ADR-0315）。
  const mayDecide = useMayMakeDecision('AUTHORIZED_REROUTE');
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
      id: 'parcel',
      header: '运单',
      render: (row) => (
        <div className="flex min-w-0 flex-col items-start gap-0.5">
          <ShipmentNo shipmentRequestId={row.shipmentRequestId} />
          <span className="text-[11px] text-idpxyz-textMuted">
            包裹 <span className="font-mono">{parcelNoOf(row.declaredParcelId)}</span>
          </span>
        </div>
      ),
    },
    {
      id: 'blockers',
      header: '为什么没自动改路',
      className: 'font-mono text-xs',
      render: (row) => (row.blockers.length > 0 ? row.blockers.join('、') : '—'),
    },
    {
      id: 'candidates',
      header: '备选路线（建议时）',
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
    { id: 'suggested-at', header: '建议时间', className: 'min-w-44 font-mono text-xs', render: (row) => formatInstant(row.suggestedAt) },
    {
      id: 'state',
      header: '状态',
      className: 'min-w-44 text-xs',
      render: (row) => {
        const availability = rerouteDecisionAvailabilityOf(row);
        return availability.kind === 'available' && mayDecide ? (
          <Button variant="outline" disabled={deciding !== null} onClick={() => setDeciding(row)}>
            {labelOf(rerouteSuggestionStateLabels, row.state)} · 决定
          </Button>
        ) : (
          <span>
            {labelOf(rerouteSuggestionStateLabels, row.state)}
            {availability.kind === 'available' ? null : <span className="block text-idpxyz-textMuted">{availability.reason}</span>}
          </span>
        );
      },
    },
    {
      id: 'technical',
      header: '',
      className: 'align-top',
      render: (row) => (
        <TechnicalDetails
          className="max-w-md open:min-w-[18rem]"
          rows={technicalRows([
            { label: '运单标识', value: row.shipmentRequestId },
            { label: '包裹标识', value: row.declaredParcelId },
            { label: '客户编码', value: row.customerAccountId },
            { label: '所在复核', value: row.correlationId },
            { label: '失效的计划', value: row.reviewedPlan },
            { label: '失效依据', value: row.lapseBasis },
            { label: '状态码', value: row.state },
          ])}
        />
      ),
    },
  ];

  return (
    <>
      <ListPageTemplate<RerouteSuggestionRecord>
        {...header}
        filterSummary={answer?.kind === 'outcome' ? `共 ${suggestions.length} 份` : undefined}
        columns={columns}
        rows={visible}
        rowKey={(row) => row.correlationId}
        viewState={catalogueViewState(answer, suggestions.length, retry, {
          module: info,
          endpoint: 'GET /route-reroute-suggestions',
          emptyTitle: '没有待处理的改路建议',
          emptyDescription: '运输途中路线失效、又不能自动改路时，系统会在这里给出改路建议，等你选一条备选路线。',
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
        <CardTitle>决定改路</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-2 text-xs">
        <p className="text-idpxyz-textMuted">
          包裹 <span className="font-mono">{suggestion.declaredParcelId}</span> 的原计划{' '}
          <span className="font-mono">{suggestion.reviewedPlan ?? '—'}</span> 已失效。从建议里合格的备选路线中选一条：系统按此刻的
          情况再核一遍，通过就改走新路线，从决定的时刻起生效；如果建议之后又复核过、或这条路线此刻被限制，就不会生效。决定人记为当前登录的你。
        </p>
        <fieldset>
          <legend className="text-idpxyz-textMuted">选哪条路线</legend>
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
          操作入口还没有配置好，这次什么都没有保存；重试也不会改变结果，请联系系统管理员完成配置。
        </p>
      );
    case 'loginRequired':
    case 'notGranted':
      return <OperatorRefusalNote kind={answer.kind} what="授权改路" />;
    case 'callerProblem':
      return (
        <p className="text-idpxyz-danger">
          没有保存：{rerouteDecisionProblemNote(answer.code)}
        </p>
      );
    case 'noAnswer':
      return (
        <p className="text-idpxyz-danger">
          结果还不确定：{rerouteDecisionProblemNote(answer.code)}
        </p>
      );
    case 'transport':
      return <p className="text-idpxyz-danger">连不上服务，这次没有提交；请检查网络后重试（{answer.message}）</p>;
  }
}
