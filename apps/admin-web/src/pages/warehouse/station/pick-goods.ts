// 拣货工位认商品码：扫到的码是不是这一格要的商品（node:test 直接测，读口作参数传入）。
//
// 扫到的码等于任务的商品编码，当场算对，不问后端。不等时可能是这个商品登记过的条码或平台商品码，用读口
// `GET /warehouse-fulfillment-goods-by-code?ownerAccount=…&code=…` 认一次：后端按（货主，码）找此刻指向的商品，码是商品编码或
// 此刻有效的别名都认（见 `findGoodsByCode`）。认成同一个商品算对；认成别的商品、认不出都算错，由状态机说错在哪。
//
// 货主取自这一格任务所属的出库单：任务带 requestId，出库单的货主从出库单读口按号取（精确，没有按包含匹配带出别的单的问题）。
// 只在第一次要认条码时才取，同一张出库单只取一次；同一个（货主，码）在这一趟里只问一次，「认不出」也记下——
// 这一趟里商品册不会为了一个扫错的码变出新的别名。没问成（网络、授权）的不记，下一次还问。

import type { ApiResult } from '../../catalogue-api';
import { findGoodsByCode, getOutboundRequestDetail, type GoodsFoundBody, type OutboundRequestDetailBody } from '../api';
import { describeReadFailure } from './work-fact-submit';

export type GoodsIdentification =
  /** 认成这一格的商品。 */
  | { kind: 'same'; via: 'code' | 'alias' }
  /** 认成这位货主的另一个商品。 */
  | { kind: 'other'; goodsCode: string }
  /** 商品编码里没有，也不是有效的条码或平台商品码。 */
  | { kind: 'unknown' }
  /** 没能认：取不到货主，或认码的读口没问成。不记数，也不说成「认不出」。 */
  | { kind: 'failed'; reason: string };

export type OwnerLookup =
  | { kind: 'owner'; ownerAccount: string }
  | { kind: 'missing' }
  | { kind: 'failed'; reason: string };

export type GoodsLookup =
  | { kind: 'found'; goodsCode: string }
  | { kind: 'notFound' }
  | { kind: 'failed'; reason: string };

export interface GoodsIdentifierDeps {
  ownerOf(requestId: string): Promise<OwnerLookup>;
  findGoods(ownerAccount: string, code: string): Promise<GoodsLookup>;
}

export interface GoodsCellFacts {
  requestId: string;
  goodsCode: string;
}

export interface GoodsIdentifier {
  identify(cell: GoodsCellFacts, code: string): Promise<GoodsIdentification>;
}

export function createGoodsIdentifier(deps: GoodsIdentifierDeps): GoodsIdentifier {
  const owners = new Map<string, string>();
  const codes = new Map<string, string | null>();

  async function ownerOf(requestId: string): Promise<{ owner: string } | { failure: string }> {
    const known = owners.get(requestId);
    if (known !== undefined) return { owner: known };
    const looked = await deps.ownerOf(requestId);
    if (looked.kind === 'owner') {
      owners.set(requestId, looked.ownerAccount);
      return { owner: looked.ownerAccount };
    }
    return { failure: looked.kind === 'missing' ? '查不到这一格所属的出库单，认不了条码' : `没能取到这一格所属出库单的货主：${looked.reason}` };
  }

  return {
    async identify(cell, code) {
      if (code === cell.goodsCode) return { kind: 'same', via: 'code' };
      const owner = await ownerOf(cell.requestId);
      if ('failure' in owner) return { kind: 'failed', reason: owner.failure };
      const key = `${owner.owner}\u0000${code}`;
      let goodsCode = codes.get(key);
      if (goodsCode === undefined) {
        const found = await deps.findGoods(owner.owner, code);
        if (found.kind === 'failed') return { kind: 'failed', reason: `读商品资料没成：${found.reason}` };
        goodsCode = found.kind === 'found' ? found.goodsCode : null;
        codes.set(key, goodsCode);
      }
      if (goodsCode === null) return { kind: 'unknown' };
      return goodsCode === cell.goodsCode ? { kind: 'same', via: 'alias' } : { kind: 'other', goodsCode };
    },
  };
}

/** 一张出库单的货主：读出库单明细，答里有这张单就取它的货主。 */
export function ownerLookupOf(answer: ApiResult<OutboundRequestDetailBody>): OwnerLookup {
  if (answer.kind !== 'outcome') return { kind: 'failed', reason: describeReadFailure(answer) };
  const owner = answer.body.request?.ownerAccount;
  return owner ? { kind: 'owner', ownerAccount: owner } : { kind: 'missing' };
}

/** 认码读口的答复：找到了交商品编码，答 found 为假是没有，别的是没问成。 */
export function goodsLookupOf(answer: ApiResult<GoodsFoundBody>): GoodsLookup {
  if (answer.kind !== 'outcome') return { kind: 'failed', reason: describeReadFailure(answer) };
  const goodsCode = answer.body.goods?.goodsCode;
  return answer.body.found && goodsCode ? { kind: 'found', goodsCode } : { kind: 'notFound' };
}

/** 接真读口的那一份。 */
export function liveGoodsIdentifier(): GoodsIdentifier {
  return createGoodsIdentifier({
    ownerOf: async (requestId) => ownerLookupOf(await getOutboundRequestDetail(requestId)),
    findGoods: async (ownerAccount, code) => goodsLookupOf(await findGoodsByCode(ownerAccount, code)),
  });
}
