import { useState } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { selectClass } from '../party/PublicationFormFields';
import type { ApiResult } from '../catalogue-api';
import { currentDisplayTimeZone, wallTimeOf, wallTimeToRfc3339 } from '../moment';
import { registerOperatorGrant, type OperatorListResponseBody, type OperatorRecord, type OperatorWriteResponseBody } from './api';
import { planGrantChecklist } from './grant-checklist';
import { decisionBundles, unbundledDecisionKinds } from './operator-brief';
import { dataVisibilityKindLabels, decisionKindLabel, faceLabel, operatorWriteOutcomeLabel, workScopeKindLabels } from './presentation';

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

const plainFaces = ['MASTER_DATA_AND_OPERATIONS_READ', 'REGISTRY_CONFIGURATION_WRITE', 'OPERATOR_ACCESS_ADMINISTRATION'] as const;

/**
 * 按组调整这一个人的授予。勾中的每一项仍各登一笔；已经生效或未到的同一格跳过。这里不撤销。
 * 组只是页面上的说法，册上不因此多出一个角色。
 */
export function AdjustGrantsForm({
  operator,
  vocabulary,
  onWritten,
  onClose,
}: {
  operator: OperatorRecord;
  vocabulary: Pick<OperatorListResponseBody, 'decisionKinds' | 'workScopeKinds' | 'visibilityKinds'>;
  onWritten: () => void;
  onClose: () => void;
}) {
  const timeZone = currentDisplayTimeZone();
  const offered = new Set(vocabulary.decisionKinds);
  const bundles = decisionBundles
    .map((bundle) => ({ ...bundle, kinds: bundle.kinds.filter((kind) => offered.has(kind)) }))
    .filter((bundle) => bundle.kinds.length > 0);
  const others = unbundledDecisionKinds(vocabulary.decisionKinds);
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

  const toggleKind = (kind: string) => {
    setChosenDecisions((current) => (current.includes(kind) ? current.filter((item) => item !== kind) : [...current, kind]));
  };
  const toggleBundle = (kinds: readonly string[]) => {
    setChosenDecisions((current) => {
      const all = kinds.every((kind) => current.includes(kind));
      return all ? current.filter((kind) => !kinds.includes(kind)) : [...new Set([...current, ...kinds])];
    });
  };
  const addVisibility = (kind: string, reference: string) => {
    setVisibilities((current) => (current.some((item) => item.kind === kind && item.reference === reference) ? current : [...current, { kind, reference }]));
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

  const groups = [...bundles.map((bundle) => ({ label: bundle.label, kinds: bundle.kinds })), ...(others.length > 0 ? [{ label: '其他', kinds: others }] : [])];

  return (
    <section className="flex flex-col gap-3 rounded border border-idpxyz-border bg-idpxyz-inputBg p-3">
      <h3 className="type-value font-medium text-idpxyz-text">调整授予</h3>
      <p className="type-caption text-idpxyz-textMuted">勾中的每一项各登一笔。已经生效的同一项会跳过。这里不撤销，撤销仍在下面每一笔上。</p>
      <div className="grid grid-cols-2 gap-3">
        <label className="flex flex-col gap-1 type-body">
          <span className="text-idpxyz-textMuted">授予标识前缀</span>
          <Input value={prefix} onChange={(event) => setPrefix(event.target.value)} placeholder="例如 IDP-GRANT" className="font-mono type-body" />
        </label>
        <label className="flex flex-col gap-1 type-body">
          <span className="text-idpxyz-textMuted">依据</span>
          <Input value={basis} onChange={(event) => setBasis(event.target.value)} placeholder="批准单据或依据引用" className="font-mono type-body" />
        </label>
        <label className="flex flex-col gap-1 type-body">
          <span className="text-idpxyz-textMuted">生效起点</span>
          <Input type="datetime-local" value={startsAt} onChange={(event) => setStartsAt(event.target.value)} className="font-mono type-body" />
        </label>
        <label className="flex flex-col gap-1 type-body">
          <span className="text-idpxyz-textMuted">生效终点</span>
          <Input type="datetime-local" value={endsAt} onChange={(event) => setEndsAt(event.target.value)} className="font-mono type-body" />
          <span className="type-caption text-idpxyz-breadcrumb">留空即不设终点。按本地时刻（{timeZone}）。</span>
        </label>
      </div>
      <fieldset className="flex flex-col gap-2">
        <legend className="type-body text-idpxyz-textMuted">能做的事</legend>
        <div className="flex flex-wrap gap-x-4 gap-y-1">
          {plainFaces.map((face) => (
            <label key={face} className="flex items-center gap-2 type-body" title={faceLabel(face)}>
              <input
                type="checkbox"
                checked={chosenFaces.includes(face)}
                onChange={() => setChosenFaces((current) => (current.includes(face) ? current.filter((item) => item !== face) : [...current, face]))}
              />
              {face === 'MASTER_DATA_AND_OPERATIONS_READ' ? '只读' : face === 'OPERATOR_ACCESS_ADMINISTRATION' ? '管人' : '登记册'}
            </label>
          ))}
        </div>
      </fieldset>
      {groups.map((group) => {
        const all = group.kinds.every((kind) => chosenDecisions.includes(kind));
        return (
          <details key={group.label} className="rounded border border-idpxyz-border p-2">
            <summary className="flex cursor-pointer list-none items-center gap-2 type-body">
              <input
                type="checkbox"
                checked={all}
                onClick={(event) => event.stopPropagation()}
                onChange={() => toggleBundle(group.kinds)}
              />
              {group.label}
              <span className="type-caption text-idpxyz-textMuted">{group.kinds.length}</span>
            </summary>
            <div className="mt-2 grid grid-cols-[repeat(auto-fill,minmax(9.5rem,1fr))] gap-x-3 gap-y-1">
              {group.kinds.map((kind) => (
                <label key={kind} className="flex min-w-0 items-start gap-2 type-body" title={kind}>
                  <input type="checkbox" className="mt-1" checked={chosenDecisions.includes(kind)} onChange={() => toggleKind(kind)} />
                  <span>{decisionKindLabel(kind)}</span>
                </label>
              ))}
            </div>
          </details>
        );
      })}
      <fieldset className="flex flex-col gap-2">
        <legend className="type-body text-idpxyz-textMuted">看得见</legend>
        <p className="type-caption text-idpxyz-textMuted">不勾就是这一类仍按他现在的范围。全部要单独勾上。</p>
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" size="sm" className="type-body" onClick={() => addVisibility('CUSTOMER_ACCOUNT', '*')}>
            全部客户
          </Button>
          <Button type="button" variant="outline" size="sm" className="type-body" onClick={() => addVisibility('WAREHOUSE', '*')}>
            全部仓
          </Button>
        </div>
        {visibilities.map((scope, index) => (
          <div key={`${scope.kind}-${index}`} className="grid grid-cols-[1fr_1fr_auto] gap-2">
            <select
              className={selectClass}
              value={scope.kind}
              onChange={(event) => setVisibilities(visibilities.map((item, at) => (at === index ? { ...item, kind: event.target.value } : item)))}
            >
              {(vocabulary.visibilityKinds ?? ['CUSTOMER_ACCOUNT', 'WAREHOUSE']).map((kind) => (
                <option key={kind} value={kind}>
                  {dataVisibilityKindLabels[kind] ?? kind}
                </option>
              ))}
            </select>
            <Input
              value={scope.reference}
              onChange={(event) => setVisibilities(visibilities.map((item, at) => (at === index ? { ...item, reference: event.target.value } : item)))}
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
          onClick={() => addVisibility(vocabulary.visibilityKinds?.[0] ?? 'CUSTOMER_ACCOUNT', '')}
        >
          指定一个
        </Button>
      </fieldset>
      {vocabulary.workScopeKinds.length > 0 ? (
        <fieldset className="flex flex-col gap-2">
          <legend className="type-body text-idpxyz-textMuted">现场作业</legend>
          {scopes.map((scope, index) => (
            <div key={index} className="grid grid-cols-[1fr_1fr_auto] gap-2">
              <select
                className={selectClass}
                value={scope.kind}
                onChange={(event) => setScopes(scopes.map((item, at) => (at === index ? { ...item, kind: event.target.value } : item)))}
              >
                {vocabulary.workScopeKinds.map((kind) => (
                  <option key={kind} value={kind}>
                    {workScopeKindLabels[kind] ?? kind}
                  </option>
                ))}
              </select>
              <Input
                value={scope.reference}
                onChange={(event) => setScopes(scopes.map((item, at) => (at === index ? { ...item, reference: event.target.value } : item)))}
                placeholder="仓或网点标识"
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
      {problem ? <p className="type-body text-idpxyz-textMuted">{problem}</p> : null}
      {plan && plan.skipped > 0 ? <p className="type-caption text-idpxyz-textMuted">已有 {plan.skipped} 项生效或未到，这次跳过。</p> : null}
      {plan && plan.payloads.length > 0 && !problem ? <p className="type-body text-idpxyz-text">将登记 {plan.payloads.length} 笔。</p> : null}
      {results.length > 0 ? (
        <ul className="flex flex-col gap-1">
          {results.map((row) => (
            <li key={row.grantId} className="type-caption text-idpxyz-textMuted">
              <span className="font-mono">{row.grantId}</span> {row.text}
            </li>
          ))}
        </ul>
      ) : null}
      <div className="flex items-center justify-end gap-2">
        <Button variant="outline" size="sm" className="type-body" onClick={close}>
          {recorded > 0 ? '完成' : '取消'}
        </Button>
        <Button variant="default" size="sm" className={`${primaryActionClass} type-body`} disabled={busy || problem !== null || !plan || plan.payloads.length === 0} onClick={submit}>
          {busy ? '登记中…' : '登记'}
        </Button>
      </div>
    </section>
  );
}
