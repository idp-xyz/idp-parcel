import { useCallback, useEffect, useState } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { downloadQuotationWorkbook, listQuotationSends, readQuotationSend, type QuotationSendBody } from './quotation-api';
import { quotationDeliveryNote } from './quotation-send';

const info = moduleInfoById['quotation-sends'];

/** 发出记录的列表与详情。详情上再下载同一份文件，不新增记录。 */
export function QuotationSendsPage() {
  const [items, setItems] = useState<QuotationSendBody[]>([]);
  const [note, setNote] = useState(quotationDeliveryNote);
  const [selected, setSelected] = useState<QuotationSendBody | null>(null);
  const [message, setMessage] = useState<string | null>(null);

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
      return;
    }
    setSelected(answer.body.send);
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
            <Button size="sm" variant="outline" onClick={() => void download()}>
              再下载同一份文件
            </Button>
          </article>
        ) : null}
      </div>
    </section>
  );
}
