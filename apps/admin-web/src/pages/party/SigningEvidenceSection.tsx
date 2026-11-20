import { useState } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { useAnswer } from '../../query/answers';
import { partyAnswersDomain } from './party-writes';
import { fieldLabel, selectClass } from './PublicationFormFields';
import { labelOf } from './presentation';
import {
  readSigningEvidences,
  registerSigningEvidence,
  registerSigningEvidenceRequirement,
  signingEvidenceLocalProblem,
  signingEvidenceOutcomeLabels,
  signingEvidenceRequirementText,
  type SigningEvidenceDraft,
} from './signing-evidence';

const emptyDraft = (): SigningEvidenceDraft => ({
  evidenceId: '',
  signedOn: '',
  counterpartySignatory: '',
  ourSignatory: '',
  fileName: '',
  sha256: '',
  libraryIndex: '',
  corrects: '',
  recordedAt: '',
});

/**
 * 对象页上的签署证据：要求状态、历史，以及有登记册写授予的人能登记和更正。
 * 要求没登记时两个布尔都要人明确勾选后才送，不把没勾读成一份默认规则。
 */
export function SigningEvidenceSection({
  kind,
  objectId,
  version,
  mayWrite,
}: {
  kind: 'CUSTOMER_CONTRACT' | 'SUPPLIER_AGREEMENT';
  objectId: string;
  version: string;
  mayWrite: boolean;
}) {
  const read = useAnswer([partyAnswersDomain, 'signing-evidence', kind, objectId, version], () => readSigningEvidences(kind, objectId, version), {
    fresh: true,
  });
  const body = read.answer?.kind === 'outcome' ? read.answer.body : null;
  const [draft, setDraft] = useState(emptyDraft);
  const [requiresContract, setRequiresContract] = useState(false);
  const [requiresAgreement, setRequiresAgreement] = useState(false);
  const [localProblem, setLocalProblem] = useState<string | null>(null);
  const [answerText, setAnswerText] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submitEvidence() {
    const problem = signingEvidenceLocalProblem(draft);
    setLocalProblem(problem);
    if (problem) return;
    setBusy(true);
    const result = await registerSigningEvidence(kind, objectId, version, { ...draft, recordedAt: new Date().toISOString() });
    setBusy(false);
    if (result.kind === 'outcome') {
      setAnswerText(labelOf(signingEvidenceOutcomeLabels, result.body.outcome));
      if (result.body.outcome === 'SAVED' || result.body.outcome === 'ALREADY_REGISTERED') setDraft(emptyDraft());
      read.retry();
      return;
    }
    setAnswerText(result.kind === 'callerProblem' ? (result.detail ?? result.code) : '没有登记。');
  }

  async function submitRequirement() {
    setBusy(true);
    const result = await registerSigningEvidenceRequirement(requiresContract, requiresAgreement);
    setBusy(false);
    if (result.kind === 'outcome') {
      setAnswerText(labelOf(signingEvidenceOutcomeLabels, result.body.outcome));
      read.retry();
      return;
    }
    setAnswerText(result.kind === 'callerProblem' ? (result.detail ?? result.code) : '没有登记。');
  }

  return (
    <div className="flex flex-col gap-3">
      <p className="type-body text-idpxyz-text">{body ? signingEvidenceRequirementText(body) : '签署证据还没读到。'}</p>
      {body && body.evidences.length === 0 ? <p className="type-body text-idpxyz-textMuted">这一版还没有签署证据。</p> : null}
      {body && body.evidences.length > 0 ? (
        <ul className="flex flex-col gap-2">
          {body.evidences.map((evidence) => (
            <li key={evidence.evidenceId} className="type-body text-idpxyz-text">
              <span className="font-mono">{evidence.evidenceId}</span>
              {' · '}
              {evidence.signedOn}
              {' · '}
              {evidence.fileName}
              {evidence.corrects ? ` · 更正 ${evidence.corrects}` : ''}
            </li>
          ))}
        </ul>
      ) : null}
      {mayWrite && body?.requirement === 'NOT_CONFIGURED' ? (
        <div className="flex flex-col gap-2">
          <label className="inline-flex items-center gap-2 type-body text-idpxyz-text">
            <input type="checkbox" checked={requiresContract} onChange={(event) => setRequiresContract(event.target.checked)} />
            客户合同要有签署证据才能发布
          </label>
          <label className="inline-flex items-center gap-2 type-body text-idpxyz-text">
            <input type="checkbox" checked={requiresAgreement} onChange={(event) => setRequiresAgreement(event.target.checked)} />
            供应商协议要有签署证据才能发布
          </label>
          <Button type="button" size="sm" disabled={busy} onClick={() => void submitRequirement()}>
            登记要求
          </Button>
        </div>
      ) : null}
      {mayWrite ? (
        <div className="grid gap-2 md:grid-cols-2">
          <label>
            <span className={fieldLabel}>证据标识</span>
            <input className={selectClass} value={draft.evidenceId} onChange={(event) => setDraft({ ...draft, evidenceId: event.target.value })} />
          </label>
          <label>
            <span className={fieldLabel}>签署日期</span>
            <input className={selectClass} value={draft.signedOn} placeholder="YYYY-MM-DD" onChange={(event) => setDraft({ ...draft, signedOn: event.target.value })} />
          </label>
          <label>
            <span className={fieldLabel}>对方签署人</span>
            <input className={selectClass} value={draft.counterpartySignatory} onChange={(event) => setDraft({ ...draft, counterpartySignatory: event.target.value })} />
          </label>
          <label>
            <span className={fieldLabel}>我方签署人</span>
            <input className={selectClass} value={draft.ourSignatory} onChange={(event) => setDraft({ ...draft, ourSignatory: event.target.value })} />
          </label>
          <label>
            <span className={fieldLabel}>文件名</span>
            <input className={selectClass} value={draft.fileName} onChange={(event) => setDraft({ ...draft, fileName: event.target.value })} />
          </label>
          <label>
            <span className={fieldLabel}>SHA-256</span>
            <input className={selectClass} value={draft.sha256} onChange={(event) => setDraft({ ...draft, sha256: event.target.value })} />
          </label>
          <label>
            <span className={fieldLabel}>受限证据库索引</span>
            <input className={selectClass} value={draft.libraryIndex} onChange={(event) => setDraft({ ...draft, libraryIndex: event.target.value })} />
          </label>
          <label>
            <span className={fieldLabel}>更正哪一条</span>
            <input className={selectClass} value={draft.corrects} onChange={(event) => setDraft({ ...draft, corrects: event.target.value })} />
          </label>
          <div className="md:col-span-2">
            <Button type="button" size="sm" disabled={busy} onClick={() => void submitEvidence()}>
              {draft.corrects.trim() === '' ? '登记签署证据' : '登记更正'}
            </Button>
          </div>
        </div>
      ) : null}
      {localProblem ? <p className="type-body text-idpxyz-text">{localProblem}</p> : null}
      {answerText ? <p className="type-body text-idpxyz-text">{answerText}</p> : null}
    </div>
  );
}
