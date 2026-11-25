// 在读口的分页里找「码完全相同」的那一行（纯 TypeScript，node:test 直接测，读口作参数传入）。工位凡要核一个扫进来的码登没登记
// （放货位、出库单、包材……）都走这一个：检索词在读面上是字面包含、不分大小写，答回来的行不一定就是要找的那一个，要在行里认出码
// 完全相同的那一行；这一页没有就沿 page.next 往下翻，翻到认出，或翻完（读口说没有下一页）；翻页设上限，到了上限还没认出就说没问完。
//
// 三种结果分得开：认出了；真的问完了没有；没问成（读口没通、没答、接入被拒、答复不成列表的样子、翻到上限）。只有第二种才许说
// 「没有」——第三种说成没有，就是把「没核成」当成了「核过了没有」。

import type { ApiResult } from '../../catalogue-api';
import type { CataloguePage } from '../../catalogue-query';
import { describeReadFailure } from './work-fact-submit';

export type Seen<Row> = { kind: 'found'; row: Row } | { kind: 'absent' } | { kind: 'unread'; reason: string };

/** 没问成时各种情形怎么说，原因里不带句号。 */
export interface SeekWords {
  /** 读口没问成；cause 是 describeReadFailure 的话。 */
  failed(cause: string): string;
  /** 答复不成列表的样子（没有行、没有页）。 */
  shape: string;
  /** 翻到页数上限还没认出。 */
  tooMany(maxPages: number): string;
}

export interface Seek<Body extends { page: CataloguePage }, Row> {
  /** 读一页：after 是上一页给的游标，第一页是 null。 */
  read: (after: string | null) => Promise<ApiResult<Body>>;
  rowsOf: (body: Body) => readonly Row[] | undefined;
  /** 这一行就是要找的那一个。 */
  isIt: (row: Row) => boolean;
  words: SeekWords;
  maxPages: number;
}

/** 下一页的游标；答复里没有 page 或没有 next 是形状不对，与「没有下一页」（null）是两回事。 */
function nextCursor(body: { page: CataloguePage }): string | null | undefined {
  const page: Partial<CataloguePage> | null | undefined = body.page;
  return page === undefined || page === null ? undefined : page.next;
}

export async function seek<Body extends { page: CataloguePage }, Row>(search: Seek<Body, Row>): Promise<Seen<Row>> {
  let after: string | null = null;
  for (let pages = 1; ; pages += 1) {
    const answer: ApiResult<Body> = await search.read(after);
    if (answer.kind !== 'outcome') return { kind: 'unread', reason: search.words.failed(describeReadFailure(answer)) };
    const rows = search.rowsOf(answer.body);
    const next = nextCursor(answer.body);
    if (!Array.isArray(rows) || next === undefined) return { kind: 'unread', reason: search.words.shape };
    const hit = rows.find(search.isIt);
    if (hit !== undefined) return { kind: 'found', row: hit };
    if (next === null) return { kind: 'absent' };
    if (pages >= search.maxPages) return { kind: 'unread', reason: search.words.tooMany(search.maxPages) };
    after = next;
  }
}
