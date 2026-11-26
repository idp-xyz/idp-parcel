import type { ApiResult } from '../../catalogue-api';
import type { SingleUnitMatchBody, ToteListBody, ToteRecord } from '../api';
import { seek, type SeekWords } from './catalogue-seek';
import { MAX_ORDER_PAGES, type OrderFinder, type OrderLookup } from './pack-order';
import type { ToteReader } from './pick-totes';
import { describeReadFailure } from './work-fact-submit';

export type SingleUnitMatcher = (warehouse: string, code: string) => Promise<ApiResult<SingleUnitMatchBody>>;

/**
 * 扫到的码既不是出库单号、也不是周转箱时，再当一件货去配单：单品单件波次的货是合在一起拿回来的，哪一件发给哪一张单
 * 到复核打包台才定。后端从本仓已拣完、等复核、要这种货的单品单件出库单里挑等得最久的那一张；这里按它的请求号再找一次单，
 * 走的是与扫单号同一条路，并标上「这张单是按货配的」（`byGoods`）——那一件货已经扫过了，开出单来要把它算进复核的数里。
 * 没有单要这种货，仍是原来的「没有这张单」。配单没问成说没核成，不说没有。
 */
export function withSingleUnitMatch(orders: OrderFinder, match: SingleUnitMatcher): OrderFinder {
  return {
    async find(warehouse, code): Promise<OrderLookup> {
      const direct = await orders.find(warehouse, code);
      if (direct.kind !== 'absent') return direct;
      const answer = await match(warehouse, code);
      if (answer.kind !== 'outcome') {
        return { kind: 'unverified', reason: `按这件货给单品单件的出库单配单没成：${describeReadFailure(answer)}` };
      }
      if (answer.body.outcome !== 'SINGLE_UNIT_MATCHED' || !answer.body.requestId) return direct;
      const found = await orders.find(warehouse, answer.body.requestId);
      return found.kind === 'found' ? { ...found, byGoods: true } : found;
    },
  };
}

/**
 * 按货配出来的单读开之后，要不要把刚才扫的那一件算进去：要，只在读开的正是配出来的那张单、并且它停在等逐件扫货复核时。
 * 交回要补扫的码；不用补为 null。
 */
export function scanAfterMatch(matched: { requestId: string; code: string } | null, openedRequest: string, phase: string): string | null {
  if (matched === null || matched.requestId !== openedRequest || phase !== 'checking') return null;
  return matched.code;
}

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
