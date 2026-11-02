import { useEffect, useState } from 'react';
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
  // 两个决定口认同一种运营决定授予（授权改路）；明确没有时只显状态、不摆按钮（ADR-0315）。
  const mayDecide = useMayMakeDecision('AUTHORIZED_REROUTE');
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
      id: 'tied',
      header: '并列的路线与相同的成本',
      className: 'min-w-56 font-mono text-xs',
      render: (row) => tiedAmountText(row),
    },
    {
      id: 'candidates',
      header: '全部备选路线',
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
    { id: 'settled-at', header: '发现并列的时间', className: 'min-w-44 font-mono text-xs', render: (row) => formatInstant(row.settledAt) },
    {
      id: 'state',
      header: '状态',
      className: 'min-w-44 text-xs',
      render: (row) => {
        const availability = initialRouteTieAvailabilityOf(row);
        return availability.kind === 'available' && mayDecide ? (
          <Button variant="outline" disabled={deciding !== null} onClick={() => setDeciding(row)}>
            {labelOf(initialRouteTieStateLabels, row.state)} · 裁决
          </Button>
        ) : (
          <span>
            {labelOf(initialRouteTieStateLabels, row.state)}
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
            { label: '受理基线', value: row.acceptanceBaseline },
            { label: '服务目的', value: row.servicePurpose },
            { label: '状态码', value: row.state },
          ])}
        />
      ),
    },
  ];

  return (
    <>
      <ListPageTemplate<InitialRouteTieRecord>
        {...header}
        filterSummary={answer?.kind === 'outcome' ? `共 ${ties.length} 件` : undefined}
        columns={columns}
        rows={visible}
        rowKey={(row) => `${row.shipmentRequestId}/${row.acceptanceBaseline}/${row.declaredParcelId}/${row.servicePurpose}`}
        viewState={catalogueViewState(answer, ties.length, retry, {
          module: info,
          endpoint: 'GET /route-initial-route-ties',
          emptyTitle: '没有等裁决的成本并列',
          emptyDescription: '定路线时几条备选路线成本一样、选不出唯一一条时，会停在这里等你选一条。',
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
        <CardTitle>选定路线</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-2 text-xs">
        <p className="text-idpxyz-textMuted">
          包裹 <span className="font-mono">{tie.declaredParcelId}</span> 有几条备选路线成本一样。从并列的那几条里选一条：系统按此刻的情况再核一遍，
          通过就定下这件包裹的路线，从决定的时刻起生效；这条路线此刻被限制、已不合格或依据不明时不会生效。决定人记为当前登录的你。
        </p>
        <fieldset>
          <legend className="text-idpxyz-textMuted">选哪条路线</legend>
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
          操作入口还没有配置好，这次什么都没有保存；重试也不会改变结果，请联系系统管理员完成配置。
        </p>
      );
    case 'loginRequired':
    case 'notGranted':
      return <OperatorRefusalNote kind={answer.kind} what="授权改路" />;
    case 'callerProblem':
      return (
        <p className="text-idpxyz-danger">
          没有保存：{initialRouteTieProblemNote(answer.code)}
        </p>
      );
    case 'noAnswer':
      return (
        <p className="text-idpxyz-danger">
          结果还不确定：{initialRouteTieProblemNote(answer.code)}
        </p>
      );
    case 'transport':
      return <p className="text-idpxyz-danger">连不上服务，这次没有提交；请检查网络后重试（{answer.message}）</p>;
  }
}
