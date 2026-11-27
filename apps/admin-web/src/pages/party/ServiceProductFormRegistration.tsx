import { useState } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import { RegistrationAnswerNote, type RegistrationPanelState } from '../../components/registration/RegistrationPanel';
import { moduleInfoById } from '../../navigation';
import { primaryActionClass } from '../../components/primary-action';
import { selectClass } from './PublicationFormFields';
import { commercialRegistrationEndpoints, productChannelOutcomeLabels, registerCommercial } from './api';
import { problemNote, serviceFormLabels } from './presentation';
import type { ServiceProductFormTarget } from './service-products-address';
import {
  emptyServiceProductFormDraft,
  readServiceProductFormSnapshot,
  serviceProductFormCodes,
  serviceProductFormGapSummary,
  serviceProductFormGaps,
  serviceProductFormPayload,
  serviceProductFormSnapshotNote,
  type ServiceProductFormCode,
  type ServiceProductFormDraft,
} from './service-product-form-registration';

const blank = '必填，不能留空';

function isFormCode(value: string): value is ServiceProductFormCode {
  return (serviceProductFormCodes as readonly string[]).includes(value);
}

/**
 * 给已经发布的一版补服务形态。目录「未登记」带入产品、版本、适用范围，这一页只让人选形态。
 * JSON 收在技术详情里：在线口与受控命令行同一份形状，但租户不自报。
 */
export function ServiceProductFormRegistration({
  target,
  onRegistered,
}: {
  target: ServiceProductFormTarget | null;
  onRegistered?: () => void;
}) {
  const [draft, setDraft] = useState<ServiceProductFormDraft>(() =>
    target
      ? { productId: target.productId, version: target.version, scope: target.scope, form: '' }
      : emptyServiceProductFormDraft(),
  );
  const [attempted, setAttempted] = useState(false);
  const [state, setState] = useState<RegistrationPanelState>({ kind: 'idle' });
  const [snapshotText, setSnapshotText] = useState('');
  const [snapshotProblem, setSnapshotProblem] = useState<string | null>(null);
  const locked = target !== null;
  const gaps = attempted ? serviceProductFormGaps(draft) : [];
  const gapPaths = new Set(gaps.map((gap) => gap.path));

  const send = (snapshot: unknown) => {
    setState({ kind: 'submitting' });
    void registerCommercial('service-product-form', snapshot).then((answer) => {
      setState({ kind: 'answered', answer });
      if (
        answer.kind === 'outcome' &&
        (answer.body.outcome === 'REGISTERED' || answer.body.outcome === 'ALREADY_REGISTERED')
      ) {
        onRegistered?.();
      }
    });
  };

  const submitFields = () => {
    setAttempted(true);
    setSnapshotProblem(null);
    const missing = serviceProductFormGaps(draft);
    if (missing.length > 0) {
      setState({ kind: 'idle' });
      const host = document.querySelector<HTMLElement>(`[data-field-path="${missing[0].path}"]`);
      const focusable = host?.matches('input,select,textarea')
        ? host
        : host?.querySelector<HTMLElement>('input,select,textarea');
      focusable?.focus();
      return;
    }
    const payload = serviceProductFormPayload(draft);
    if (payload) send(payload);
  };

  const submitSnapshot = () => {
    const read = readServiceProductFormSnapshot(snapshotText);
    if (!read.ok) {
      setSnapshotProblem(read.problem);
      setState({ kind: 'idle' });
      return;
    }
    setSnapshotProblem(null);
    send(read.snapshot);
  };

  const field = (path: 'productId' | 'version' | 'scope', label: string) => (
    <label className="flex flex-col gap-1" data-field-path={path}>
      <span className="type-body text-idpxyz-breadcrumb">
        {label} *
      </span>
      <Input
        value={draft[path]}
        readOnly={locked}
        className="font-mono type-value"
        onChange={(event) => setDraft((current) => ({ ...current, [path]: event.target.value }))}
      />
      {gapPaths.has(path) ? <span className="type-caption text-idpxyz-danger">{blank}</span> : null}
    </label>
  );

  return (
    <div className="flex-1 overflow-auto p-4">
      <div className="flex max-w-3xl flex-col gap-4">
        <div className="grid grid-cols-2 gap-3">
          {field('productId', '产品')}
          {field('version', '版本')}
          {field('scope', '适用范围')}
          <label className="flex flex-col gap-1">
            <span className="type-body text-idpxyz-breadcrumb">形态 *</span>
            <select
              className={selectClass}
              data-field-path="form"
              value={draft.form}
              onChange={(event) => {
                const value = event.target.value;
                setDraft((current) => ({ ...current, form: isFormCode(value) ? value : '' }));
              }}
            >
              <option value="">请选择</option>
              {serviceProductFormCodes.map((code) => (
                <option key={code} value={code}>
                  {serviceFormLabels[code]}
                </option>
              ))}
            </select>
            {gapPaths.has('form') ? <span className="type-caption text-idpxyz-danger">{blank}</span> : null}
          </label>
        </div>
        {gaps.length > 0 ? <p className="type-body text-idpxyz-danger">{serviceProductFormGapSummary(gaps)}</p> : null}
        <div className="flex flex-wrap items-center gap-3">
          <Button variant="default" className={primaryActionClass} onClick={submitFields} disabled={state.kind === 'submitting'}>
            {state.kind === 'submitting' ? '提交中…' : '提交'}
          </Button>
          <RegistrationAnswerNote
            state={state}
            owner={moduleInfoById['service-products'].owner}
            outcomeLabels={productChannelOutcomeLabels}
            problemNote={problemNote}
          />
        </div>
        <details className="type-body text-idpxyz-textMuted">
          <summary className="cursor-pointer select-none">技术详情</summary>
          <p className="mt-2">{serviceProductFormSnapshotNote}</p>
          <p className="mt-1 font-mono type-caption">POST {commercialRegistrationEndpoints['service-product-form']}</p>
          <textarea
            className={`${selectClass} mt-2 min-h-[8rem] font-mono`}
            value={snapshotText}
            placeholder="粘贴这一项的 JSON"
            onChange={(event) => setSnapshotText(event.target.value)}
          />
          {snapshotProblem ? <p className="mt-1 text-idpxyz-danger">{snapshotProblem}</p> : null}
          <Button variant="outline" className="mt-2" onClick={submitSnapshot} disabled={state.kind === 'submitting' || snapshotText.trim() === ''}>
            提交这份 JSON
          </Button>
        </details>
      </div>
    </div>
  );
}
