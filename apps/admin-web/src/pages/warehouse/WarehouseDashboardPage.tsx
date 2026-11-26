import { useCallback, useEffect, useState } from 'react';
import { RefreshCw } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { listCustomerAccounts } from '../party/api';
import { moduleInfoById } from '../../navigation';
import {
  getReturnsOverview,
  listAdjustments,
  listCounts,
  listFreezes,
  listOutboundRequests,
  listOwnerPreAdvices,
  listUnclaimedArrivals,
  type PreAdviceRecord,
} from './api';
import {
  countCard,
  exceptionRows,
  outboundCards,
  receiveCard,
  type AdviceWait,
  type ExceptionRow,
  type WorkCard,
  countStationHash,
  freezeList,
} from './dashboard-board';
import { documentHash } from './document-address';
import { returnAdviceKind, returnReceiptKind, returnsModule } from './return-address';
import { awaitsDisposition, returnAdviceStatus, receiptsOfAdvice } from './return-flow';
import { enterStation } from '../../shell/enter-station';

const info = moduleInfoById['warehouse-dashboard'];

interface Board {
  cards: WorkCard[];
  exceptions: ExceptionRow[];
  note: string;
}

function totalOf(answer: { kind: string; body?: { page?: { total: number | null }; requests?: readonly unknown[]; counts?: readonly unknown[]; adjustments?: readonly unknown[]; freezes?: readonly unknown[]; arrivals?: readonly unknown[] } }, rows: readonly unknown[] | undefined): number | null {
  if (answer.kind !== 'outcome') return null;
  const total = answer.body?.page?.total;
  return typeof total === 'number' ? total : (rows?.length ?? 0);
}

export function WarehouseDashboardPage() {
  const [loading, setLoading] = useState(true);
  const [board, setBoard] = useState<Board>({ cards: [], exceptions: [], note: '' });

  const load = useCallback(() => {
    setLoading(true);
    const now = new Date();
    void (async () => {
      const [allocated, picked, checked, packed, counting, adjustments, freezes, unclaimed, returns, accounts] = await Promise.all([
        listOutboundRequests({ filters: { status: ['ALLOCATED'] } }),
        listOutboundRequests({ filters: { status: ['PICKED'] } }),
        listOutboundRequests({ filters: { status: ['CHECKED'] } }),
        listOutboundRequests({ filters: { status: ['PACKED'] } }),
        listCounts({ filters: { status: ['COUNTING'] } }, null),
        listAdjustments({ filters: { status: ['PENDING'] }, sort: 'submittedAt' }),
        listFreezes({ filters: { status: ['FROZEN'] }, sort: 'frozenAt' }),
        listUnclaimedArrivals({ filters: { status: ['UNCLAIMED'] }, sort: 'receivedAt' }),
        getReturnsOverview(),
        listCustomerAccounts(),
      ]);
      const reviewing = picked.kind === 'outcome' && checked.kind === 'outcome'
        ? (picked.body.page.total ?? picked.body.requests.length) + (checked.body.page.total ?? checked.body.requests.length)
        : null;
      let advices: AdviceWait[] | null = [];
      let ownersTruncated = false;
      if (accounts.kind !== 'outcome') advices = null;
      else {
        ownersTruncated = accounts.body.page?.next != null;
        const lists = await Promise.all((accounts.body.accounts ?? []).map((account) => listOwnerPreAdvices(account.accountId, '')));
        if (lists.some((item) => item.kind !== 'outcome')) advices = null;
        else {
          advices = lists.flatMap((item) => {
            if (item.kind !== 'outcome') return [];
            return (item.body.preAdvices ?? []).filter((advice) => advice.status === 'ADVISED' || advice.status === 'RECEIVING').map(waitOf);
          });
        }
      }
      const returnRows = returns.kind === 'outcome' ? returnExceptions(returns.body.preAdvices, returns.body.receipts) : [];
      const cards = [
        receiveCard(advices, ownersTruncated, now),
        ...outboundCards({
          allocated: totalOf(allocated, allocated.kind === 'outcome' ? allocated.body.requests : undefined),
          reviewing,
          packed: totalOf(packed, packed.kind === 'outcome' ? packed.body.requests : undefined),
        }),
        countCard('退货', returns.kind === 'outcome' ? returnRows.length : null, '等收或等处置。', '#/warehouse-returns'),
        countCard('盘点', totalOf(counting, counting.kind === 'outcome' ? counting.body.counts : undefined), '正在清点。', countStationHash()),
        countCard('冻结', totalOf(freezes, freezes.kind === 'outcome' ? freezes.body.freezes : undefined), '还冻着。', freezeList),
      ];
      const exceptions = exceptionRows({
        now,
        advices: advices ?? [],
        unclaimed: unclaimed.kind === 'outcome' ? unclaimed.body.arrivals ?? [] : [],
        adjustments: adjustments.kind === 'outcome' ? adjustments.body.adjustments ?? [] : [],
        freezes: freezes.kind === 'outcome' ? freezes.body.freezes ?? [] : [],
        returns: returnRows,
      });
      const failed = [allocated, picked, checked, packed, counting, adjustments, freezes, unclaimed, returns].filter((item) => item.kind !== 'outcome').length;
      setBoard({ cards, exceptions, note: failed > 0 ? `${failed} 处没读成，对应的卡写着没读成。` : ownersTruncated ? '货主名册只看了第一页，收货张数不是全租户。' : '' });
      setLoading(false);
    })();
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div className="flex-1 overflow-y-auto bg-idpxyz-editor">
      <div className="mx-auto flex max-w-[1160px] flex-col gap-6 px-6 py-6">
        <header className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h1 className="type-title font-bold text-idpxyz-textBright">{info.title}</h1>
            <p className="mt-1 type-body text-idpxyz-textMuted">各类作业还压着多少，以及等得最久的那几张。点一张卡进那一队，点一行开那张单据。</p>
          </div>
          <Button type="button" variant="outline" disabled={loading} onClick={load}>
            <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
            {loading ? '刷新中…' : '刷新'}
          </Button>
        </header>
        {board.note ? <p className="type-caption text-idpxyz-textMuted">{board.note}</p> : null}
        <section aria-label="作业待处理" className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
          {board.cards.map((item) => (
            <button key={item.name} type="button" className="rounded-lg border border-idpxyz-border bg-idpxyz-sidebar p-4 text-left hover:border-idpxyz-accent" onClick={() => (item.hash.startsWith('#/station') ? enterStation(item.hash) : (window.location.hash = item.hash))}>
              <div className="type-body text-idpxyz-textMuted">{item.name}</div>
              <div className="mt-2 font-mono type-display font-bold text-idpxyz-textBright">{item.count === null ? '—' : item.count}</div>
              <p className="mt-1 type-caption text-idpxyz-textMuted">{item.note}</p>
            </button>
          ))}
        </section>
        <section aria-label="异常清单" className="rounded-lg border border-idpxyz-border bg-idpxyz-sidebar">
          <h2 className="border-b border-idpxyz-border px-4 py-3 type-section text-idpxyz-textBright">等得最久的</h2>
          {board.exceptions.length === 0 ? <p className="px-4 py-6 type-body text-idpxyz-textMuted">这几处读到的队列里没有还在等的。</p> : (
            <ul>
              {board.exceptions.map((item) => (
                <li key={`${item.kind}|${item.title}`} className="border-b border-idpxyz-border last:border-b-0">
                  <button type="button" className="flex w-full items-center justify-between gap-3 px-4 py-3 text-left hover:bg-idpxyz-hover" onClick={() => { window.location.hash = item.hash; }}>
                    <span className="type-body text-idpxyz-textBright">{item.kind}</span>
                    <span className="min-w-0 flex-1 truncate font-mono type-body">{item.title}</span>
                    <span className="type-caption text-idpxyz-textMuted">{item.waited}</span>
                    <span className="type-body text-idpxyz-accent">{item.next}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </div>
  );
}

function waitOf(advice: PreAdviceRecord): AdviceWait {
  return { ownerAccount: advice.ownerAccount, preAdviceId: advice.preAdviceId, documentNumber: advice.documentNumber, status: advice.status, expectedAt: advice.currentNotice?.expectedAt };
}

function returnExceptions(advices: Parameters<typeof receiptsOfAdvice>[0][], receipts: Parameters<typeof receiptsOfAdvice>[1]): ExceptionRow['kind'] extends string ? Array<{ kind: string; title: string; at: string; next: string; hash: string }> : never {
  const rows: Array<{ kind: string; title: string; at: string; next: string; hash: string }> = [];
  for (const advice of advices) {
    if (returnAdviceStatus(receiptsOfAdvice(advice, receipts)) !== 'AWAITING_ARRIVAL') continue;
    rows.push({ kind: '退货待到仓', title: advice.documentNumber ?? advice.adviceId, at: advice.receivedAt, next: '去收退件', hash: documentHash(returnsModule, [returnAdviceKind, advice.adviceId]) });
  }
  for (const receipt of receipts) {
    if (!awaitsDisposition(receipt)) continue;
    rows.push({ kind: '退货待处置', title: receipt.factId, at: receipt.occurredAt, next: '处置', hash: documentHash(returnsModule, [returnReceiptKind, receipt.factId]) });
  }
  return rows;
}
