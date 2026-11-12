// 往来单位页「换条件、翻页、刷新时留着旧的一页」的判读（票 business-party-redesign/06）。纯函数，node:test 钉着；页面与
// PartyWorkbench.tsx、PartyRoster.tsx 只照着摆。
//
// 取数件在一问还在途时把上一份业务答案交出来（use-catalogue-cursor.ts 的 previous），用不用它由页面定——这里就是那个判断。
// 垫着的只是行：计数、空态与「筛空」仍只认此刻这一问的答复，不在这里。

import type { ListCursorPaginationProps } from '../../templates/ListPageTemplate';
import type { ApiResult } from '../catalogue-api';
import type { CataloguePage } from '../catalogue-query';

export interface HeldCataloguePage<Body> {
  /** 上一页的正文：行从它取。 */
  body: Body;
  /**
   * 照上一页写的翻页条：页号、页大小、总数都是它当时的。两个钮在重读时由列表件按住，所以这里的回调什么都不做——拿上一页的
   * 游标去翻，翻的是一组已经换掉的条件。
   */
  pagination: ListCursorPaginationProps;
}

const nothing = () => {};

/**
 * 这一问还在途时垫着看的上一页。此刻这一问已经答出（`answer` 不为 null，成也好败也好）就不垫：成了摆新的一页，败了摆读取失败的
 * 状态块。没有上一页可垫（第一次加载、上一份没答成）也不垫，照旧是骨架。上一页在此刻的页内筛选下一行不剩（`hasRows` 为假）
 * 同样不垫：那时摆出来的只会是「筛空」那句话，而它说的是上一组条件。
 */
export function heldCataloguePage<Body extends { page: CataloguePage }>(
  answer: ApiResult<Body> | null,
  previous: { body: Body; page: number } | null,
  hasRows: (body: Body) => boolean,
): HeldCataloguePage<Body> | null {
  if (answer !== null || previous === null || !hasRows(previous.body)) return null;
  const page = previous.body.page;
  return {
    body: previous.body,
    pagination: { mode: 'cursor', page: previous.page, size: page.size, total: page.total, next: page.next, onPrevious: nothing, onNext: nothing },
  };
}
