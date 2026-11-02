import { useEffect, useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import type { WorkbenchColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { primaryActionClass } from '../../components/primary-action';
import { OperatorRefusalNote } from '../../components/states';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useMayMakeDecision } from '../use-operator-grants';
import { shipmentNoOf } from '../shipment-request/ref-no';
import { useCustomerNames } from '../shipment-request/use-customer-names';
import type { StatusTone } from '../../vocabulary/status';
import { RowStatusPill } from '../visibility/ExceptionDetailPanel';
import { RecordDetailPanel, RouteWorkbench, techRows, type RowStatus } from './route-workbench';
import { parcelCell, parcelSearchCells, parcelSubtitle, type RoutePlansHeader } from './route-plan-cells';
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

/**
 * 路由计划页「改路建议」一册（ADR-0284 决定六）：复核在原计划失效后形成的改路建议，逐份列阻塞、候选与状态；待决定的那几份可择一
 * 合格候选、填决定依据，经授权改路决定口提交（ADR-0284 决定一）。决定方是登录的操作者，本页不收也不送；成不成立由服务端在判断键
 * 锁里按此刻的证据重判，本页的状态只是读到时的大概。
 */
const stateTone: Record<string, StatusTone> = { PENDING: 'warn', DECIDED: 'good', SUPERSEDED: 'muted' };

/** 建议状态：词取本册的词表（待决定 / 已决定 / 已过时），色调待决定琥珀、已决定绿、已过时灰。 */
function suggestionStatus(row: RerouteSuggestionRecord): RowStatus {
  return { label: labelOf(rerouteSuggestionStateLabels, row.state).split('——')[0], tone: stateTone[row.state] ?? 'muted' };
}

export function RerouteSuggestionsView({ header }: { header: RoutePlansHeader }) {
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<RerouteSuggestionListResponseBody> | null>(null);
  const names = useCustomerNames();

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

  const columns: WorkbenchColumn<RerouteSuggestionRecord>[] = [
    { key: 'parcel', header: '运单', render: parcelCell },
    { key: 'state', header: '状态', render: (row) => <RowStatusPill status={suggestionStatus(row)} /> },
    {
      key: 'candidates',
      header: '合格的备选路线',
      hideWhenDetailOpen: true,
      render: (row) => String(row.candidates.filter((candidate) => candidate.outcome === 'QUALIFIED').length) + ' 条',
    },
    {
      key: 'suggested-at',
      header: '建议时间',
      sortKey: 'suggested-at',
      render: (row) => <span className="font-mono text-idpxyz-breadcrumb">{formatInstant(row.suggestedAt)}</span>,
    },
  ];

  return (
    <RouteWorkbench<RerouteSuggestionRecord>
      {...header}
      answered={answer?.kind === 'outcome'}
      noun="份"
      rows={suggestions}
      rowKey={(row) => row.correlationId}
      statusOf={suggestionStatus}
      rowAttention={(row) => row.state === 'PENDING'}
      searchCells={(row) => [...parcelSearchCells(row, names), row.correlationId, row.reviewedPlan]}
      searchPlaceholder="搜运单号 / 包裹号 / 客户"
      columns={columns}
      sortAccessors={{ 'suggested-at': { value: (row) => row.suggestedAt, instant: true } }}
      defaultSort={{ key: 'suggested-at', dir: -1 }}
      renderDetail={(row, close) => <SuggestionDetail key={row.correlationId} row={row} names={names} onClose={close} onLanded={retry} />}
      onRefresh={retry}
      viewState={catalogueViewState(answer, suggestions.length, retry, {
        module: info,
        endpoint: 'GET /route-reroute-suggestions',
        emptyTitle: '没有待处理的改路建议',
        emptyDescription: '运输途中路线失效、又不能自动改路时，系统会在这里给出改路建议，等你选一条备选路线。',
      })}
    />
  );
}

/**
 * 一份改路建议的右栏：阻塞、各备选在建议时的结局；待决定且有合格备选时摆「决定改路」，点开在栏内填决定。换一份建议时由调用方
 * 给新 key 重挂，上一份的草稿不带过来。
 */
function SuggestionDetail({
  row,
  names,
  onClose,
  onLanded,
}: {
  row: RerouteSuggestionRecord;
  names: Parameters<typeof parcelSubtitle>[1];
  onClose: () => void;
  onLanded: () => void;
}) {
  // 两个决定口认同一种运营决定授予（授权改路）；明确没有时只显状态、不摆按钮（ADR-0315）。
  const mayDecide = useMayMakeDecision('AUTHORIZED_REROUTE');
  const [deciding, setDeciding] = useState(false);
  const availability = rerouteDecisionAvailabilityOf(row);
  return (
    <RecordDetailPanel
      title={shipmentNoOf(row.shipmentRequestId)}
      status={suggestionStatus(row)}
      subtitle={parcelSubtitle(row, names)}
      facts={[
        { label: '建议时间', value: formatInstant(row.suggestedAt) },
        { label: '触发时间', value: row.triggerOccurredAt ? formatInstant(row.triggerOccurredAt) : '—' },
        { label: '为什么没自动改路', value: row.blockers.length > 0 ? <span className="font-mono">{row.blockers.join('、')}</span> : '—' },
        { label: '决定', value: row.decision ? `${row.decision.candidate}（${formatInstant(row.decision.decidedAt)}）` : '还没有' },
      ]}
      technical={techRows([
        { label: '运单标识', value: row.shipmentRequestId },
        { label: '包裹标识', value: row.declaredParcelId },
        { label: '客户编码', value: row.customerAccountId },
        { label: '所在复核', value: row.correlationId },
        { label: '失效的计划', value: row.reviewedPlan },
        { label: '失效依据', value: row.lapseBasis },
        { label: '新计划版本', value: row.decision?.newPlanVersion },
        { label: '决定人', value: row.decision?.decidedBy },
        { label: '决定依据', value: row.decision?.basis },
        { label: '状态码', value: row.state },
      ])}
      actions={
        availability.kind === 'available' && mayDecide ? (
          deciding ? (
            <RerouteDecisionCard
              suggestion={row}
              onClose={() => setDeciding(false)}
              onLanded={() => {
                setDeciding(false);
                onLanded();
              }}
            />
          ) : (
            <Button variant="outline" onClick={() => setDeciding(true)}>
              决定改路
            </Button>
          )
        ) : availability.kind === 'unavailable' ? (
          <p className="text-[12px] text-idpxyz-breadcrumb">{availability.reason}</p>
        ) : null
      }
      onClose={onClose}
    >
      <p>备选路线（建议时的评估，不是此刻的）：</p>
      <ul className="space-y-0.5">
        {row.candidates.map((candidate) => (
          <li key={candidate.id}>
            <span className="font-mono text-idpxyz-text">{candidate.id}</span> {labelOf(suggestionCandidateOutcomeLabels, candidate.outcome)}
            {candidate.reason ? <span>（{candidate.reason}）</span> : null}
          </li>
        ))}
      </ul>
    </RecordDetailPanel>
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
    <Card>
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
