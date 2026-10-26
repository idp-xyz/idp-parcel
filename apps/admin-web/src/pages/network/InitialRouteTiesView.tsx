import { useEffect, useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { primaryActionClass } from '../../components/primary-action';
import { OperatorRefusalNote } from '../../components/states';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  decideInitialRouteTie,
  listInitialRouteTies,
  type ApiResult,
  type InitialRouteTieDecisionResponseBody,
  type InitialRouteTieListResponseBody,
  type InitialRouteTieRecord,
} from './api';
import { labelOf } from './presentation';
import {
  describeInitialRouteTieAnswer,
  initialRouteTieAvailabilityOf,
  initialRouteTieProblemNote,
  initialRouteTieRequestOf,
  initialRouteTieStateLabels,
  tiedAmountText,
  type InitialRouteTieDraft,
} from './initial-route-tie';
import { suggestionCandidateOutcomeLabels } from './reroute-decision';
import type { RoutePlansHeader } from './RerouteSuggestionsView';

const info = moduleInfoById['route-plans'];

/**
 * 路由计划页「并列」一册（ADR-0309 决定六）：初始路由最低成本并列的包裹，逐件列全部候选、并列的那几家与打平的金额；待裁决的那几件可在
 * 并列的那几家里择一、填决定依据，经并列裁决口提交（ADR-0309 决定二）。决定方是登录的操作者，本页不收也不送；成不成立由服务端在判断键
 * 锁里按此刻的证据重判，本页的状态只是读到时的大概。
 */
export function InitialRouteTiesView({ header, keyword }: { header: RoutePlansHeader; keyword: string }) {
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<InitialRouteTieListResponseBody> | null>(null);
  const [deciding, setDeciding] = useState<InitialRouteTieRecord | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listInitialRouteTies().then((loaded) => {
      if (!cancelled) setAnswer(loaded);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const retry = () => setReloadKey((value) => value + 1);
  const ties = answer?.kind === 'outcome' ? answer.body.ties : [];
  const needle = keyword.trim().toLowerCase();
  const visible = needle
    ? ties.filter((row) =>
        [row.shipmentRequestId, row.declaredParcelId, row.customerAccountId].some((value) => value.toLowerCase().includes(needle)),
      )
    : ties;

  const columns: ListColumn<InitialRouteTieRecord>[] = [
    {
      id: 'declared-parcel',
      header: '申报包裹',
      className: 'font-mono text-xs',
      render: (row) => <span className="text-idpxyz-accent">{row.declaredParcelId}</span>,
    },
    { id: 'baseline', header: '接受基线', className: 'font-mono text-xs', render: (row) => row.acceptanceBaseline },
    {
      id: 'tied',
      header: '并列的候选与打平的金额',
      className: 'min-w-56 font-mono text-xs',
      render: (row) => tiedAmountText(row),
    },
    {
      id: 'candidates',
      header: '全部候选（入账时）',
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
    { id: 'settled-at', header: '入账时点', className: 'min-w-44 font-mono text-xs', render: (row) => formatInstant(row.settledAt) },
    {
      id: 'state',
      header: '状态',
      className: 'min-w-44 text-xs',
      render: (row) => {
        const availability = initialRouteTieAvailabilityOf(row);
        return availability.kind === 'available' ? (
          <Button variant="outline" disabled={deciding !== null} onClick={() => setDeciding(row)}>
            {labelOf(initialRouteTieStateLabels, row.state)} · 裁决
          </Button>
        ) : (
          <span>
            {labelOf(initialRouteTieStateLabels, row.state)}
            <span className="block text-idpxyz-textMuted">{availability.reason}</span>
          </span>
        );
      },
    },
  ];

  return (
    <>
      <ListPageTemplate<InitialRouteTieRecord>
        {...header}
        filterSummary={answer?.kind === 'outcome' ? `最低成本并列 ${ties.length} 件` : undefined}
        columns={columns}
        rows={visible}
        rowKey={(row) => `${row.shipmentRequestId}/${row.acceptanceBaseline}/${row.declaredParcelId}/${row.servicePurpose}`}
        viewState={catalogueViewState(answer, ties.length, retry, {
          module: info,
          endpoint: 'GET /route-initial-route-ties',
          emptyTitle: '当前租户没有停在最低成本并列的包裹',
          emptyDescription: '并列由初始路由在合格候选成本打平时入账；没有这样的包裹，册就是空的，本页不预置数据。',
        })}
      />
      {deciding ? (
        <InitialRouteTieCard
          tie={deciding}
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
  | { kind: 'answered'; answer: ApiResult<InitialRouteTieDecisionResponseBody> };

function InitialRouteTieCard({
  tie,
  onClose,
  onLanded,
}: {
  tie: InitialRouteTieRecord;
  onClose: () => void;
  onLanded: () => void;
}) {
  const availability = initialRouteTieAvailabilityOf(tie);
  const candidates = availability.kind === 'available' ? availability.candidates : [];
  const [draft, setDraft] = useState<InitialRouteTieDraft>({ candidate: candidates[0] ?? '', basis: '' });
  const [state, setState] = useState<SubmissionState>({ kind: 'idle' });

  const submit = () => {
    const built = initialRouteTieRequestOf(tie, draft);
    if (built.kind === 'incomplete') {
      setState({ kind: 'incomplete', reason: built.reason });
      return;
    }
    setState({ kind: 'submitting' });
    void decideInitialRouteTie(built.request).then((answer) => {
      setState({ kind: 'answered', answer });
      if (answer.kind === 'outcome' && describeInitialRouteTieAnswer(answer.body).landed) onLanded();
    });
  };

  return (
    <Card className="mx-4 mb-4">
      <CardHeader>
        <CardTitle>并列裁决</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-2 text-xs">
        <p className="text-idpxyz-textMuted">
          包裹 <span className="font-mono">{tie.declaredParcelId}</span> 的合格候选在成本上打平。在并列的那几家里择一家：服务端按此刻的证据重判它，
          成立时形成这件包裹的首个计划，从决定的时刻起生效；此刻被限制淘汰、已不合格或证据未知，都不成立。决定方是登录的操作者，本表单不收也不送。
        </p>
        <fieldset>
          <legend className="text-idpxyz-textMuted">所择候选</legend>
          {candidates.map((candidate) => (
            <label key={candidate} className="mr-4 inline-flex items-center gap-1 font-mono">
              <input
                type="radio"
                name="tie-candidate"
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
            {state.kind === 'submitting' ? '提交中…' : '提交裁决'}
          </Button>
          <Button variant="outline" onClick={onClose}>
            收起
          </Button>
        </div>
        <TieAnswerNote state={state} />
      </CardContent>
    </Card>
  );
}

function TieAnswerNote({ state }: { state: SubmissionState }) {
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
      const note = describeInitialRouteTieAnswer(answer.body);
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
          接入渠道未配置（403 ACCESS_CHANNEL_NOT_CONFIGURED）。裁决口已建立并装配，操作者渠道的发行方参数未设，服务端如实拒绝。
        </p>
      );
    case 'loginRequired':
    case 'notGranted':
      return <OperatorRefusalNote kind={answer.kind} what="授权改路" />;
    case 'callerProblem':
      return (
        <p className="text-idpxyz-danger">
          调用方式问题（HTTP {answer.status}）：{initialRouteTieProblemNote(answer.code)}
        </p>
      );
    case 'noAnswer':
      return (
        <p className="text-idpxyz-danger">
          服务端未形成答案（HTTP {answer.status}）：{initialRouteTieProblemNote(answer.code)}
        </p>
      );
    case 'transport':
      return <p className="text-idpxyz-danger">请求未到达 parcel-api：{answer.message}</p>;
  }
}
