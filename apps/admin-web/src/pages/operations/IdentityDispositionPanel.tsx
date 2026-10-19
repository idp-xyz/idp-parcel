import { useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { OperatorRefusalNote } from '../../components/states';
import {
  disposeIdentityConflict,
  type ApiResult,
  type CoDisposedUnit,
  type IdentityDispositionResponseBody,
} from './identity-disposition-api';
import {
  coDisposedUnitsFromEcho,
  describeDispositionAnswer,
  dispositionProblemNote,
  dispositionRequestOf,
  identityReviewRowOf,
  type DispositionDraft,
} from './identity-disposition';
import type { IdentityReviewUnit } from './records-api';

/**
 * 待识别实物页按实物列的那一半与「处置」动作（票 no-reception-identity/06；UC-NO-004、ADR-0210）。逐行的待识别册分不出哪一版仍是
 * 当前有效版本、冲突处置过没有；这里把一件实物各条链的当前有效版本列在一起，处置版写出处置了什么。
 *
 * 「处置」不改正式包裹身份，也不改变实物去向：它只在这件实物的处置候选（都出自 parcel-shipment 的身份核对答复）里指定一件，或确认
 * 都不是。指定的那件此刻关联在别的实物上时，服务端答「关联对象另有归属」并回显那几件；页面据此带出须连带的实物，人确认后一并提交。
 */
export function IdentityDispositionPanel({ units, onLanded }: { units: IdentityReviewUnit[]; onLanded: () => void }) {
  const [draft, setDraft] = useState<DispositionDraft | null>(null);
  const rows = units.map(identityReviewRowOf);
  const editing = draft ? rows.find((row) => row.unit === draft.unit) : undefined;

  return (
    <Card className="mx-4 mb-4">
      <CardHeader>
        <CardTitle>按实物看各链当前有效版本 · 身份冲突处置</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        {rows.length === 0 ? (
          <p className="text-xs text-idpxyz-textMuted">当前页没有待识别过的实物。</p>
        ) : (
          <table className="w-full text-xs">
            <thead>
              <tr className="text-left text-idpxyz-textMuted">
                <th className="py-1 pr-2 font-normal">作业实物</th>
                <th className="py-1 pr-2 font-normal">各链当前有效版本</th>
                <th className="py-1 pr-2 font-normal">身份冲突</th>
                <th className="py-1 pr-2 font-normal">处置了什么</th>
                <th className="py-1 font-normal">动作</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={row.key} className="align-top border-t border-idpxyz-border">
                  <td className="py-1 pr-2 font-mono">{row.values.unit}</td>
                  <td className="py-1 pr-2 font-mono">{row.values.heads}</td>
                  <td className="py-1 pr-2">{row.values.status}</td>
                  <td className="py-1 pr-2">{row.values.disposition || '—'}</td>
                  <td className="py-1">
                    {row.disposition.kind === 'available' ? (
                      <Button
                        variant="outline"
                        disabled={draft !== null}
                        onClick={() => {
                          if (row.disposition.kind !== 'available') return;
                          setDraft({
                            unit: row.unit,
                            disposedVersion: row.disposition.disposedVersion,
                            parcel: '',
                            basis: '',
                            coDisposedUnits: [],
                          });
                        }}
                      >
                        处置
                      </Button>
                    ) : (
                      <span className="text-idpxyz-textMuted">{row.disposition.reason}</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        {draft && editing && editing.disposition.kind === 'available' ? (
          <DispositionForm
            draft={draft}
            versions={editing.disposition.versions}
            candidates={editing.disposition.candidates}
            onChange={setDraft}
            onClose={() => setDraft(null)}
            onLanded={() => {
              setDraft(null);
              onLanded();
            }}
          />
        ) : null}
      </CardContent>
    </Card>
  );
}

type SubmissionState =
  | { kind: 'idle' }
  | { kind: 'incomplete'; reason: string }
  | { kind: 'submitting' }
  | { kind: 'answered'; answer: ApiResult<IdentityDispositionResponseBody> };

function DispositionForm({
  draft,
  versions,
  candidates,
  onChange,
  onClose,
  onLanded,
}: {
  draft: DispositionDraft;
  versions: string[];
  candidates: string[];
  onChange: (draft: DispositionDraft) => void;
  onClose: () => void;
  onLanded: () => void;
}) {
  const [state, setState] = useState<SubmissionState>({ kind: 'idle' });
  const submit = (next: DispositionDraft) => {
    const built = dispositionRequestOf(next);
    if (built.kind === 'incomplete') {
      setState({ kind: 'incomplete', reason: built.reason });
      return;
    }
    setState({ kind: 'submitting' });
    void disposeIdentityConflict(built.request).then((answer) => {
      setState({ kind: 'answered', answer });
      if (answer.kind === 'outcome' && describeDispositionAnswer(answer.body).landed) onLanded();
    });
  };
  const choose = (parcel: string) => onChange({ ...draft, parcel, coDisposedUnits: [] });
  const withCoDisposed = (units: CoDisposedUnit[]) => {
    const next = { ...draft, coDisposedUnits: units };
    onChange(next);
    submit(next);
  };

  return (
    <div className="mt-2 flex flex-col gap-2 border-t border-idpxyz-border pt-2">
      <p className="text-xs text-idpxyz-textMuted">
        处置 <span className="font-mono">{draft.unit}</span>：指定处置候选中的一件，或确认都不是。这件实物每条链各追加一版处置版，原判断与
        冲突版照留；操作者身份由接入渠道给，本表单不收也不送。
      </p>
      <label className="block text-xs">
        <span className="text-idpxyz-textMuted">所处置的版本（复核页上看到的当前有效版本）</span>
        <select
          className="ml-2 font-mono"
          value={draft.disposedVersion}
          onChange={(event) => onChange({ ...draft, disposedVersion: event.target.value })}
        >
          {versions.map((version) => (
            <option key={version} value={version}>
              {version}
            </option>
          ))}
        </select>
      </label>
      <fieldset className="text-xs">
        <legend className="text-idpxyz-textMuted">结果</legend>
        {candidates.map((candidate) => (
          <label key={candidate} className="mr-4 inline-flex items-center gap-1 font-mono">
            <input type="radio" name="disposition-result" checked={draft.parcel === candidate} onChange={() => choose(candidate)} />
            指定 {candidate}
          </label>
        ))}
        <label className="inline-flex items-center gap-1">
          <input type="radio" name="disposition-result" checked={draft.parcel === ''} onChange={() => choose('')} />
          都不是
        </label>
      </fieldset>
      <label className="block">
        <span className="text-xs text-idpxyz-textMuted">处置依据引用 *</span>
        <Input
          value={draft.basis}
          className="font-mono text-[13px]"
          placeholder="复核记录、工单或核实凭证的引用，逐字保存、不判真伪"
          onChange={(event) => onChange({ ...draft, basis: event.target.value })}
        />
      </label>
      {draft.coDisposedUnits.length > 0 ? (
        <p className="text-xs text-idpxyz-textMuted">
          一并确认不是 {draft.parcel}：
          {draft.coDisposedUnits.map((co) => (
            <span key={co.unit} className="ml-1 font-mono">
              {co.unit}@{co.version}
            </span>
          ))}
        </p>
      ) : null}
      <div className="flex items-center gap-3">
        <Button
          variant="default"
          className={primaryActionClass}
          onClick={() => submit(draft)}
          disabled={state.kind === 'submitting'}
        >
          {state.kind === 'submitting' ? '提交中…' : '提交处置'}
        </Button>
        <Button variant="outline" onClick={onClose}>
          收起
        </Button>
      </div>
      <DispositionAnswerNote state={state} parcel={draft.parcel} onCoDispose={withCoDisposed} />
    </div>
  );
}

function DispositionAnswerNote({
  state,
  parcel,
  onCoDispose,
}: {
  state: SubmissionState;
  parcel: string;
  onCoDispose: (units: CoDisposedUnit[]) => void;
}) {
  const frame = 'text-xs';
  switch (state.kind) {
    case 'idle':
    case 'submitting':
      return null;
    case 'incomplete':
      return <p className={`${frame} text-idpxyz-danger`}>未提交：{state.reason}</p>;
  }
  const answer = state.answer;
  switch (answer.kind) {
    case 'outcome': {
      const note = describeDispositionAnswer(answer.body);
      const tone = note.tone === 'answered' ? 'text-idpxyz-textMuted' : 'text-idpxyz-danger';
      const echo =
        answer.body.outcome === 'ASSOCIATED_ELSEWHERE' ? coDisposedUnitsFromEcho(parcel, answer.body.associatedUnits ?? []) : null;
      return (
        <div className={`${frame} ${tone}`}>
          <p>{note.headline}</p>
          {note.details.length > 0 ? (
            <ul className="mt-0.5 ml-4 list-disc">
              {note.details.map((line) => (
                <li key={line}>{line}</li>
              ))}
            </ul>
          ) : null}
          {echo?.kind === 'ready' ? (
            <Button variant="outline" className="mt-1" onClick={() => onCoDispose(echo.units)}>
              确认这些实物不是 {parcel}，一并提交
            </Button>
          ) : null}
          {echo?.kind === 'blocked' ? <p className="mt-1 text-idpxyz-danger">{echo.reason}</p> : null}
        </div>
      );
    }
    case 'unconfigured':
      return (
        <p className={`${frame} text-idpxyz-textMuted`}>
          接入渠道未配置（403 ACCESS_CHANNEL_NOT_CONFIGURED）。处置端点已建立并装配，操作者渠道的发行方参数未设，服务端如实拒绝。
        </p>
      );
    case 'loginRequired':
    case 'notGranted':
      return <OperatorRefusalNote kind={answer.kind} what="身份冲突处置" className={`${frame} text-idpxyz-textMuted`} />;
    case 'callerProblem':
      return (
        <p className={`${frame} text-idpxyz-danger`}>
          调用方式问题（HTTP {answer.status}）：{dispositionProblemNote(answer.code)}
        </p>
      );
    case 'noAnswer':
      return (
        <p className={`${frame} text-idpxyz-danger`}>
          服务端未形成答案（HTTP {answer.status}）：{dispositionProblemNote(answer.code)}
        </p>
      );
    case 'transport':
      return <p className={`${frame} text-idpxyz-danger`}>请求未到达 parcel-api：{answer.message}</p>;
  }
}
