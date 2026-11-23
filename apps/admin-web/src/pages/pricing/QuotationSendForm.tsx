import { useEffect, useState } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { listBusinessParties, listGroupLegalEntities } from '../party/api';
import { downloadQuotationWorkbook, sendQuotation } from './quotation-api';
import {
  quotationDeliveryNote,
  quotationSendOutcomeLabels,
  quotationSendPayload,
  type QuotationPriceLane,
} from './quotation-send';

const fieldClass = 'mt-0.5 w-full rounded border border-idpxyz-border bg-transparent px-2 py-1 type-body text-idpxyz-text';

/**
 * 从已有往来单位和责任法人里选，填有效期与条款后发出。不在这页建档。
 * 发出成功后的下载走再导出，那一次不新增记录。
 */
export function QuotationSendForm({
  lane,
  planId,
  planVersion,
  contentDigest,
}: {
  lane: QuotationPriceLane;
  planId: string;
  planVersion: string;
  contentDigest: string;
}) {
  const [parties, setParties] = useState<{ id: string; name: string }[]>([]);
  const [entities, setEntities] = useState<{ id: string; name: string }[]>([]);
  const [counterparty, setCounterparty] = useState('');
  const [legalEntity, setLegalEntity] = useState('');
  const [validFrom, setValidFrom] = useState('');
  const [validUntil, setValidUntil] = useState('');
  const [terms, setTerms] = useState('');
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState<string | null>(null);
  const [quotationId, setQuotationId] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const [partyAnswer, entityAnswer] = await Promise.all([listBusinessParties(), listGroupLegalEntities()]);
      if (cancelled) return;
      if (partyAnswer.kind === 'outcome') {
        setParties(partyAnswer.body.parties.map((party) => ({ id: party.partyId, name: party.partyName })));
      }
      if (entityAnswer.kind === 'outcome') {
        setEntities(
          entityAnswer.body.entities.map((entity) => ({
            id: entity.legalEntityId,
            name: entity.partyNameKnown ? (entity.partyName ?? entity.legalEntityId) : entity.legalEntityId,
          })),
        );
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const submit = async () => {
    setBusy(true);
    setNote(null);
    const answer = await sendQuotation(
      quotationSendPayload({
        lane,
        planId,
        planVersion,
        contentDigest,
        counterpartyPartyId: counterparty,
        legalEntityId: legalEntity,
        validFrom,
        validUntil,
        terms,
      }),
    );
    setBusy(false);
    if (answer.kind !== 'outcome') {
      setNote(answer.kind === 'transport' ? answer.message : '没有发出');
      return;
    }
    setNote(answer.body.conclusion ?? quotationSendOutcomeLabels[answer.body.outcome] ?? answer.body.outcome);
    if (answer.body.outcome === 'SENT' && answer.body.quotationId) {
      setQuotationId(answer.body.quotationId);
    }
  };

  const download = async () => {
    if (!quotationId) return;
    setBusy(true);
    const answer = await downloadQuotationWorkbook(quotationId);
    setBusy(false);
    if (answer.kind === 'file') {
      const url = URL.createObjectURL(answer.blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = answer.fileName;
      link.click();
      URL.revokeObjectURL(url);
      return;
    }
    if (answer.kind === 'outcome') {
      const body = answer.body as { conclusion?: string };
      setNote(body.conclusion ?? '这份文件没有再导出');
      return;
    }
    setNote(answer.kind === 'transport' ? answer.message : '这份文件没有再导出');
  };

  return (
    <form
      className="mt-2 flex max-w-md flex-col gap-2"
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <p className="type-caption text-idpxyz-textMuted">
        报给已有往来单位。不在这页建档。{quotationDeliveryNote}
      </p>
      <label className="type-caption text-idpxyz-textMuted">
        相对方
        <select className={fieldClass} value={counterparty} required onChange={(event) => setCounterparty(event.target.value)}>
          <option value="">选择往来单位</option>
          {parties.map((party) => (
            <option key={party.id} value={party.id}>
              {party.name}（{party.id}）
            </option>
          ))}
        </select>
      </label>
      <label className="type-caption text-idpxyz-textMuted">
        责任法人
        <select className={fieldClass} value={legalEntity} required onChange={(event) => setLegalEntity(event.target.value)}>
          <option value="">选择责任法人</option>
          {entities.map((entity) => (
            <option key={entity.id} value={entity.id}>
              {entity.name}（{entity.id}）
            </option>
          ))}
        </select>
      </label>
      <label className="type-caption text-idpxyz-textMuted">
        有效期起点（含）
        <input className={fieldClass} type="datetime-local" value={validFrom} onChange={(event) => setValidFrom(event.target.value)} />
      </label>
      <label className="type-caption text-idpxyz-textMuted">
        有效期终点（不含）
        <input className={fieldClass} type="datetime-local" value={validUntil} onChange={(event) => setValidUntil(event.target.value)} />
      </label>
      <label className="type-caption text-idpxyz-textMuted">
        条款
        <textarea className={fieldClass} rows={2} value={terms} onChange={(event) => setTerms(event.target.value)} />
      </label>
      <div className="flex gap-2">
        <Button size="sm" type="submit" disabled={busy || contentDigest === ''}>
          {busy ? '发出中…' : '发出'}
        </Button>
        {quotationId ? (
          <Button size="sm" type="button" variant="outline" disabled={busy} onClick={() => void download()}>
            下载对客价格表
          </Button>
        ) : null}
      </div>
      {note ? (
        <p role="status" className="type-caption text-idpxyz-text">
          {note}
        </p>
      ) : null}
    </form>
  );
}
