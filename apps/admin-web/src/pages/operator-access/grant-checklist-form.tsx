import { useState } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { selectClass } from '../party/PublicationFormFields';
import type { ApiResult } from '../catalogue-api';
import { currentDisplayTimeZone, wallTimeOf, wallTimeToRfc3339 } from '../moment';
import { registerOperatorGrant, type OperatorGrantRecord, type OperatorListResponseBody, type OperatorRecord, type OperatorWriteResponseBody } from './api';
import { planGrantChecklist } from './grant-checklist';
import { dataVisibilityKindLabels, decisionKindLabel, faceLabel, operatorWriteOutcomeLabel } from './presentation';

function grantWriteLine(answer: ApiResult<OperatorWriteResponseBody>): string {
  switch (answer.kind) {
    case 'outcome':
      return operatorWriteOutcomeLabel(answer.body.outcome);
    case 'callerProblem':
      return `请求的格式不对（${answer.code}）`;
    case 'notGranted':
      return '你的账号没有操作者接入管理授予。';
    case 'loginRequired':
      return '登录已失效，请重新登录。';
    case 'unconfigured':
      return '操作者渠道尚未配置。';
    default:
      return '答案未形成，请稍后重试。';
  }
}

/**
 * 一次把勾中的若干格授给这一个人。每一笔仍走授予登记口；已经生效的同一格不重复登。
 * 作业范围的引用是租户自己的对象，这里不预填。
 */
export function GrantChecklistForm({
  operator,
  vocabulary,
  onWritten,
  onClose,
}: {
  operator: OperatorRecord;
  vocabulary: Pick<OperatorListResponseBody, 'faces' | 'decisionKinds' | 'workScopeKinds' | 'visibilityKinds'>;
  onWritten: () => void;
  onClose: () => void;
}) {
  const timeZone = currentDisplayTimeZone();
  const plainFaces = vocabulary.faces.filter(
    (face) => face !== 'OPERATION_DECISION' && face !== 'WORK_FACT_REGISTRATION' && face !== 'DATA_VISIBILITY',
  );
  const visibilityKinds = vocabulary.visibilityKinds ?? [];
  const [chosenFaces, setChosenFaces] = useState<string[]>([]);
  const [chosenDecisions, setChosenDecisions] = useState<string[]>([]);
  const [scopes, setScopes] = useState<{ kind: string; reference: string }[]>([]);
  const [visibilities, setVisibilities] = useState<{ kind: string; reference: string }[]>([]);
  const [prefix, setPrefix] = useState('');
  const [basis, setBasis] = useState('');
  const [startsAt, setStartsAt] = useState(() => wallTimeOf(new Date(), timeZone));
  const [endsAt, setEndsAt] = useState('');
  const [busy, setBusy] = useState(false);
  const [recorded, setRecorded] = useState(0);
  const [results, setResults] = useState<{ grantId: string; text: string }[]>([]);

  const startsRfc = wallTimeToRfc3339(startsAt, timeZone);
  const endsRfc = endsAt === '' ? undefined : wallTimeToRfc3339(endsAt, timeZone);
  const timeProblem = startsRfc === null ? '生效起点填得不对。' : endsAt !== '' && endsRfc === null ? '生效终点填得不对。' : null;
  const plan = timeProblem
    ? null
    : planGrantChecklist(
        { faces: chosenFaces, decisionKinds: chosenDecisions, workScopes: scopes, visibilities },
        {
          prefix,
          basis,
          effectiveStartsAt: startsRfc ?? '',
          ...(endsRfc ? { effectiveEndsAt: endsRfc } : {}),
          issuer: operator.issuer,
          subject: operator.subject,
        },
        operator.grants,
        new Date(),
      );
  const problem = timeProblem ?? plan?.problem ?? null;

  const toggle = (current: string[], value: string, set: (next: string[]) => void) => {
    set(current.includes(value) ? current.filter((item) => item !== value) : [...current, value]);
  };

  const submit = () => {
    if (!plan || plan.payloads.length === 0 || problem || busy) return;
    setBusy(true);
    setResults([]);
    void (async () => {
      const rows: { grantId: string; text: string }[] = [];
      let count = 0;
      for (const payload of plan.payloads) {
        const answer = await registerOperatorGrant(payload);
        if (answer.kind === 'outcome' && answer.body.outcome === 'RECORDED') count += 1;
        rows.push({ grantId: payload.grantId, text: grantWriteLine(answer) });
        setResults([...rows]);
      }
      setRecorded((previous) => previous + count);
      setBusy(false);
    })();
  };

  const close = () => {
    if (recorded > 0) onWritten();
    onClose();
  };

  return (
    <section className="flex flex-col gap-3 rounded border border-idpxyz-border bg-idpxyz-inputBg p-3">
      <h3 className="type-value font-medium text-idpxyz-text">按清单授予</h3>
      <p className="type-caption text-idpxyz-textMuted">勾中的每一格各登一笔。标识是前缀加上这一格的原词。已经生效的同一格会跳过。</p>
      <label className="flex flex-col gap-1 type-body">
        <span className="text-idpxyz-textMuted">授予标识前缀</span>
        <Input value={prefix} onChange={(event) => setPrefix(event.target.value)} placeholder="例如 SYN-GRANT-UAT" className="font-mono type-body" />
      </label>
      <label className="flex flex-col gap-1 type-body">
        <span className="text-idpxyz-textMuted">依据</span>
        <Input value={basis} onChange={(event) => setBasis(event.target.value)} placeholder="批准单据或依据引用" className="font-mono type-body" />
      </label>
      <div className="grid grid-cols-2 gap-3">
        <label className="flex flex-col gap-1 type-body">
          <span className="text-idpxyz-textMuted">生效起点</span>
          <Input type="datetime-local" value={startsAt} onChange={(event) => setStartsAt(event.target.value)} className="font-mono type-body" />
          <span className="type-caption text-idpxyz-breadcrumb">按本地时刻（{timeZone}）填。</span>
        </label>
        <label className="flex flex-col gap-1 type-body">
          <span className="text-idpxyz-textMuted">生效终点</span>
          <Input type="datetime-local" value={endsAt} onChange={(event) => setEndsAt(event.target.value)} className="font-mono type-body" />
          <span className="type-caption text-idpxyz-breadcrumb">留空即不设终点。</span>
        </label>
      </div>
      <fieldset className="flex flex-col gap-1">
        <legend className="type-body text-idpxyz-textMuted">能力面</legend>
        {plainFaces.map((face) => (
          <label key={face} className="flex items-center gap-2 type-body">
            <input
              type="checkbox"
              checked={chosenFaces.includes(face)}
              onChange={() => toggle(chosenFaces, face, setChosenFaces)}
            />
            {faceLabel(face)}
          </label>
        ))}
      </fieldset>
      {vocabulary.decisionKinds.length > 0 ? (
        <fieldset className="flex flex-col gap-1">
          <legend className="flex items-center gap-2 type-body text-idpxyz-textMuted">
            运营决定
            <button
              type="button"
              className="type-caption text-idpxyz-breadcrumb"
              onClick={() =>
                setChosenDecisions(chosenDecisions.length === vocabulary.decisionKinds.length ? [] : [...vocabulary.decisionKinds])
              }
            >
              {chosenDecisions.length === vocabulary.decisionKinds.length ? '全不选' : '全选'}
            </button>
          </legend>
          <div className="max-h-48 overflow-y-auto">
            {vocabulary.decisionKinds.map((kind) => (
              <label key={kind} className="flex items-center gap-2 type-body" title={kind}>
                <input
                  type="checkbox"
                  checked={chosenDecisions.includes(kind)}
                  onChange={() => toggle(chosenDecisions, kind, setChosenDecisions)}
                />
                {decisionKindLabel(kind)}
              </label>
            ))}
          </div>
        </fieldset>
      ) : null}
      {vocabulary.workScopeKinds.length > 0 ? (
        <fieldset className="flex flex-col gap-2">
          <legend className="type-body text-idpxyz-textMuted">作业事实</legend>
          {scopes.map((scope, index) => (
            <div key={index} className="grid grid-cols-[1fr_1fr_auto] gap-2">
              <select
                className={selectClass}
                value={scope.kind}
                onChange={(event) =>
                  setScopes(scopes.map((item, at) => (at === index ? { ...item, kind: event.target.value } : item)))
                }
              >
                {vocabulary.workScopeKinds.map((kind) => (
                  <option key={kind} value={kind}>
                    {kind}
                  </option>
                ))}
              </select>
              <Input
                value={scope.reference}
                onChange={(event) =>
                  setScopes(scopes.map((item, at) => (at === index ? { ...item, reference: event.target.value } : item)))
                }
                placeholder="范围引用"
                className="font-mono type-body"
              />
              <Button type="button" variant="ghost" size="sm" onClick={() => setScopes(scopes.filter((_, at) => at !== index))}>
                去掉
              </Button>
            </div>
          ))}
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="type-body self-start"
            onClick={() => setScopes([...scopes, { kind: vocabulary.workScopeKinds[0] ?? '', reference: '' }])}
          >
            加一个作业范围
          </Button>
        </fieldset>
      ) : null}
      {visibilityKinds.length > 0 ? (
        <fieldset className="flex flex-col gap-2">
          <legend className="type-body text-idpxyz-textMuted">看得见的数据</legend>
          <p className="type-caption text-idpxyz-textMuted">引用填 * 表示这一类全部。不填这一类，就是这一类什么都看不见。</p>
          {visibilities.map((scope, index) => (
            <div key={index} className="grid grid-cols-[1fr_1fr_auto] gap-2">
              <select
                className={selectClass}
                value={scope.kind}
                onChange={(event) =>
                  setVisibilities(visibilities.map((item, at) => (at === index ? { ...item, kind: event.target.value } : item)))
                }
              >
                {visibilityKinds.map((kind) => (
                  <option key={kind} value={kind}>
                    {dataVisibilityKindLabels[kind] ?? kind}
                  </option>
                ))}
              </select>
              <Input
                value={scope.reference}
                onChange={(event) =>
                  setVisibilities(visibilities.map((item, at) => (at === index ? { ...item, reference: event.target.value } : item)))
                }
                placeholder="标识，或 *"
                className="font-mono type-body"
              />
              <Button type="button" variant="ghost" size="sm" onClick={() => setVisibilities(visibilities.filter((_, at) => at !== index))}>
                去掉
              </Button>
            </div>
          ))}
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="type-body self-start"
            onClick={() => setVisibilities([...visibilities, { kind: visibilityKinds[0] ?? '', reference: '' }])}
          >
            加一个可见范围
          </Button>
        </fieldset>
      ) : null}
      {problem ? <p className="type-body text-idpxyz-textMuted">{problem}</p> : null}
      {plan && plan.skipped > 0 ? (
        <p className="type-caption text-idpxyz-textMuted">已有 {plan.skipped} 格生效或未到，这次跳过。</p>
      ) : null}
      {plan && plan.payloads.length > 0 && !problem ? (
        <p className="type-body text-idpxyz-text">将登记 {plan.payloads.length} 笔。</p>
      ) : null}
      {results.length > 0 ? (
        <ul className="flex flex-col gap-1">
          {results.map((row) => (
            <li key={row.grantId} className="type-caption">
              <span className="font-mono text-idpxyz-textMuted">{row.grantId}</span> {row.text}
            </li>
          ))}
        </ul>
      ) : null}
      <div className="flex items-center justify-end gap-2">
        <Button variant="outline" size="sm" className="type-body" onClick={close} disabled={busy}>
          取消
        </Button>
        <Button
          variant="default"
          size="sm"
          className={`${primaryActionClass} type-body`}
          disabled={busy || problem !== null || !plan || plan.payloads.length === 0}
          onClick={submit}
        >
          {busy ? '登记中…' : '登记勾中的'}
        </Button>
      </div>
    </section>
  );
}
