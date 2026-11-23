import { useCallback, useEffect, useState } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { downloadQuotationWorkbook, listQuotationSends, readQuotationSend, recordQuotationReply, type QuotationSendBody, type QuotationSendDetailBody } from './quotation-api';
import { publishPriceCardDraft } from './api';
import { quotationDeliveryNote } from './quotation-send';
import { quotationReadingLabels, quotationReplyOutcomeLabels, quotationReplyPayload } from './quotation-reply';

const info = moduleInfoById['quotation-sends'];

function effectivenessLabel(step: string): string {
  switch (step) {
    case 'PUBLISH_CARD':
      return '合同已经指名这个计价范围。下一步是发布这张价卡。';
    case 'ALREADY_EFFECTIVE':
      return '价卡已在册，合同已经指名这个范围。';
    case 'CANNOT_PUBLISH':
      return '所引草稿已撤下，不能发布。';
    default:
      return '合同还没指名这个计价范围。给已登记客户换来源列表还不能做，这里不发布新合同。';
  }
}

/** 发出记录的列表与详情。详情上再下载同一份文件，不新增记录。 */
export function QuotationSendsPage() {
  const [items, setItems] = useState<QuotationSendBody[]>([]);
  const [note, setNote] = useState(quotationDeliveryNote);
  const [selected, setSelected] = useState<QuotationSendBody | null>(null);
  const [detail, setDetail] = useState<QuotationSendDetailBody | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [decision, setDecision] = useState<'ACCEPTED' | 'REJECTED'>('ACCEPTED');
  const [repliedAt, setRepliedAt] = useState('');
  const [evidenceIndex, setEvidenceIndex] = useState('');
  const [rejectionReason, setRejectionReason] = useState('');

  const load = useCallback(async () => {
    const answer = await listQuotationSends();
    if (answer.kind !== 'outcome') {
      setMessage(answer.kind === 'transport' ? answer.message : '发出记录没有读回来');
      return;
    }
    setItems(answer.body.items);
    setNote(answer.body.deliveryNote || quotationDeliveryNote);
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const open = async (quotationId: string) => {
    const answer = await readQuotationSend(quotationId);
    if (answer.kind !== 'outcome' || !answer.body.send) {
      setMessage(answer.kind === 'outcome' ? answer.body.deliveryNote : '这一条没有读回来');
      setSelected(null);
      setDetail(null);
      return;
    }
    setSelected(answer.body.send);
    setDetail(answer.body);
    setNote(answer.body.deliveryNote);
  };

  const download = async () => {
    if (!selected) return;
    const answer = await downloadQuotationWorkbook(selected.quotationId);
    if (answer.kind !== 'file') {
      setMessage(answer.kind === 'outcome' ? '这份文件没有再导出' : answer.kind === 'transport' ? answer.message : '这份文件没有再导出');
      return;
    }
    const url = URL.createObjectURL(answer.blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = answer.fileName;
    link.click();
    URL.revokeObjectURL(url);
  };

  return (
    <section className="space-y-4 p-5" aria-label={info.title}>
      <header>
        <h1 className="type-title font-semibold text-idpxyz-textBright">{info.title}</h1>
        <p className="type-body text-idpxyz-textMuted">{info.summary}</p>
        <p className="type-caption text-idpxyz-textMuted">{note}</p>
      </header>
      {message ? (
        <p role="status" className="type-body text-idpxyz-text">
          {message}
        </p>
      ) : null}
      <div className="grid gap-4 lg:grid-cols-2">
        <ul className="divide-y divide-idpxyz-border rounded border border-idpxyz-border">
          {items.length === 0 ? <li className="px-3 py-2 type-body text-idpxyz-textMuted">还没有发出记录</li> : null}
          {items.map((item) => (
            <li key={item.quotationId}>
              <button type="button" className="w-full px-3 py-2 text-left hover:bg-idpxyz-hover" onClick={() => void open(item.quotationId)}>
                <div className="font-mono text-idpxyz-textBright">{item.quotationId}</div>
                <div className="type-caption text-idpxyz-textMuted">
                  {item.counterpartyName} · {item.planId} {item.planVersion}
                </div>
              </button>
            </li>
          ))}
        </ul>
        {selected ? (
          <article className="space-y-2 rounded border border-idpxyz-border p-3" aria-label={`报价 ${selected.quotationId}`}>
            <h2 className="font-mono text-idpxyz-textBright">{selected.quotationId}</h2>
            <p>相对方 {selected.counterpartyName}</p>
            <p>责任法人 {selected.legalEntityName}</p>
            <p>
              有效期 {selected.validFrom} 至 {selected.validUntil}（终点不含）
            </p>
            <p>条款 {selected.terms === '' ? '没有另加条款' : selected.terms}</p>
            <p className="type-caption text-idpxyz-textMuted">{quotationDeliveryNote}</p>
            <p>读法 {quotationReadingLabels[detail?.reading ?? ''] ?? '待读'}</p>
            {detail?.effectivenessNextStep ? <p>生效 {effectivenessLabel(detail.effectivenessNextStep)}</p> : null}
            {detail?.effectivenessNextStep === 'PUBLISH_CARD' && selected.lane === 'APPROVED_DRAFT' ? (
              <Button
                size="sm"
                onClick={() => {
                  void publishPriceCardDraft({
                    planId: selected.planId,
                    planVersion: selected.planVersion,
                    publicationBasis: selected.quotationId,
                  }).then((answer) => {
                    setMessage(answer.kind === 'outcome' ? answer.body.outcome : answer.kind === 'transport' ? answer.message : '没有发布');
                    void open(selected.quotationId);
                  });
                }}
              >
                发布这张价卡
              </Button>
            ) : null}
            {detail?.conclusion ? <p>{detail.conclusion}</p> : null}
            {detail?.reply ? (
              <p>
                答复 {detail.reply.decision}，客户时刻 {detail.reply.repliedAt}，证据 {detail.reply.evidenceIndex}
              </p>
            ) : (
              <form
                className="space-y-2"
                onSubmit={(event) => {
                  event.preventDefault();
                  const payload = quotationReplyPayload({
                    quotationId: selected.quotationId,
                    decision,
                    repliedAt,
                    evidenceIndex,
                    rejectionReason,
                    contentDigest: selected.contentDigest,
                  });
                  if (!payload.repliedAt || payload.evidenceIndex.trim() === '') {
                    setMessage(quotationReplyOutcomeLabels.NOT_ACCEPTED);
                    return;
                  }
                  void recordQuotationReply(payload).then((answer) => {
                    if (answer.kind !== 'outcome') {
                      setMessage(answer.kind === 'transport' ? answer.message : '没有记下');
                      return;
                    }
                    setMessage(quotationReplyOutcomeLabels[answer.body.outcome] ?? answer.body.conclusion ?? answer.body.outcome);
                    void open(selected.quotationId);
                  });
                }}
              >
                <label className="block type-caption text-idpxyz-textMuted">
                  客户答复时刻（协调世界时）
                  <input className="mt-0.5 w-full rounded border border-idpxyz-border bg-transparent px-2 py-1" value={repliedAt} onChange={(event) => setRepliedAt(event.target.value)} required />
                </label>
                <label className="block type-caption text-idpxyz-textMuted">
                  证据索引
                  <input className="mt-0.5 w-full rounded border border-idpxyz-border bg-transparent px-2 py-1" value={evidenceIndex} onChange={(event) => setEvidenceIndex(event.target.value)} required />
                </label>
                <div className="flex gap-2">
                  <Button type="button" size="sm" variant={decision === 'ACCEPTED' ? 'default' : 'outline'} onClick={() => setDecision('ACCEPTED')}>
                    接受
                  </Button>
                  <Button type="button" size="sm" variant={decision === 'REJECTED' ? 'default' : 'outline'} onClick={() => setDecision('REJECTED')}>
                    拒绝
                  </Button>
                </div>
                {decision === 'REJECTED' ? (
                  <label className="block type-caption text-idpxyz-textMuted">
                    拒绝原因，可以空着
                    <input className="mt-0.5 w-full rounded border border-idpxyz-border bg-transparent px-2 py-1" value={rejectionReason} onChange={(event) => setRejectionReason(event.target.value)} />
                  </label>
                ) : null}
                <Button size="sm" type="submit">
                  代录答复
                </Button>
              </form>
            )}
            <Button size="sm" variant="outline" onClick={() => void download()}>
              再下载同一份文件
            </Button>
          </article>
        ) : null}
      </div>
    </section>
  );
}
