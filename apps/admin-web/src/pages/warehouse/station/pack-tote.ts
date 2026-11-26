import type { ToteListBody, ToteRecord } from '../api';
import { seek, type SeekWords } from './catalogue-seek';
import { MAX_ORDER_PAGES, type OrderFinder, type OrderLookup } from './pack-order';
import type { ToteReader } from './pick-totes';

const toteWords: SeekWords = {
  failed: (cause) => `读周转箱名册没成：${cause}`,
  shape: '读周转箱名册得到的答复不是周转箱列表的样子',
  tooMany: (maxPages) => `周转箱里含这个码的行太多，翻了 ${maxPages} 页还没认出它`,
};

/**
 * 复核打包台扫到的码也可以是周转箱：货是装在箱里送过来的，扫箱比找单号快。先照原样当出库单号找；
 * 这个仓和别的仓都没有这个号，才到本仓的周转箱名册里找箱号与它完全相同的那个箱——出库单号优先，免得一个箱号
 * 恰好与某张单的号相同时找错。箱上绑着出库单就按那张单的请求号再找一次单；箱空着或已停用，如实说这是个没绑单的箱。
 */
export function withToteLookup(orders: OrderFinder, totes: ToteReader, maxPages: number = MAX_ORDER_PAGES): OrderFinder {
  return {
    async find(warehouse, code): Promise<OrderLookup> {
      const direct = await orders.find(warehouse, code);
      if (direct.kind !== 'absent') return direct;
      const tote = await seek<ToteListBody, ToteRecord>({
        read: (after) => totes({ filters: { warehouseCode: [warehouse] }, q: code }, after),
        rowsOf: (body) => body.totes,
        isIt: (row) => row.warehouseCode === warehouse && row.toteCode === code,
        words: toteWords,
        maxPages,
      });
      if (tote.kind === 'unread') return { kind: 'unverified', reason: tote.reason };
      if (tote.kind !== 'found') return direct;
      if (tote.row.state !== 'IN_USE' || !tote.row.requestId) return { kind: 'emptyTote', state: tote.row.state };
      return orders.find(warehouse, tote.row.requestId);
    },
  };
}
