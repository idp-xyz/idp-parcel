// 目录页的游标翻页状态（票 catalogue-read-pagination/05）：在第几页、该带哪个游标、手上的答复答的是不是此刻这一问。状态转移是纯函数，
// node:test 钉着；钩子 useCatalogueCursor 只把它接上 effect。走过的游标与「条件一变回第一页」沿用模板的 cursor-pagination（票 04），
// 这里不另立一套。只引 templates/cursor-pagination 本身而不经 templates/index：那里带着 ESM-only 的 @idpxyz 原语，run-tests 加载不了。

import type { ApiResult } from './catalogue-api';
import {
  currentCursorAfter,
  cursorPageNumber,
  cursorTrailFor,
  nextCursorPage,
  previousCursorPage,
  startCursorTrail,
  type CursorTrail,
} from '../templates/cursor-pagination';

export interface CatalogueCursorState<Body> {
  /** 走过的游标，连同造它的那组条件的签名（catalogueConditionsQuery）。 */
  readonly trail: CursorTrail;
  /**
   * 同一问又问了几次。重试也换一个请求键：手上那份旧答复随之不算数、页面回到加载中——不然按了「重试」，人分不出是没按到还是还没回。
   */
  readonly attempt: number;
  /** 最近收下的一份答复，连同它答的是哪一问、那一问在第几页。 */
  readonly loaded: { readonly key: string; readonly answer: ApiResult<Body>; readonly page: number } | null;
}

export type CatalogueCursorEvent<Body> =
  | { kind: 'conditions'; signature: string }
  | { kind: 'next'; next: string }
  | { kind: 'previous' }
  | { kind: 'retry' }
  | { kind: 'answered'; key: string; answer: ApiResult<Body> };

export function startCatalogueCursor<Body>(signature: string): CatalogueCursorState<Body> {
  return { trail: startCursorTrail(signature), attempt: 0, loaded: null };
}

/** 此刻该发的那一问：带哪个游标，以及认它的请求键（条件签名、游标、重取序号三样）。 */
export function catalogueCursorRequest<Body>(state: CatalogueCursorState<Body>): { key: string; after: string | null } {
  const after = currentCursorAfter(state.trail);
  return { key: JSON.stringify([state.trail.query, after, state.attempt]), after };
}

/** 手上的答复答的若正是此刻这一问就交出；否则 null——这一问还在途，页面显加载中。 */
export function catalogueCursorAnswer<Body>(state: CatalogueCursorState<Body>): ApiResult<Body> | null {
  return state.loaded !== null && state.loaded.key === catalogueCursorRequest(state).key ? state.loaded.answer : null;
}

/**
 * 这一问还在途时可以垫着看的上一页（票 business-party-redesign/06）：手上那份答复答的是别的问——上一组条件、上一页，或刷新之前
 * 的同一问——并且是业务答案时，交出它的正文与它当时在第几页；其余情形交 null。
 *
 * 只是「可以垫着看」：它答的不是此刻这一问，计数、空态与「筛空」都不能照它说。读不读它由页面定——不读的页面在途时照旧只有
 * catalogueCursorAnswer 的 null，行为与原来一样。没答成的那一份（读取失败、未配置）不交：那是一块状态说明，不是一页可垫的行。
 * 新的一问一旦答出（成也好败也好），这里就回到 null，垫着的旧页随之撤下。
 */
export function catalogueCursorPrevious<Body>(state: CatalogueCursorState<Body>): { body: Body; page: number } | null {
  const loaded = state.loaded;
  if (loaded === null || loaded.key === catalogueCursorRequest(state).key) return null;
  return loaded.answer.kind === 'outcome' ? { body: loaded.answer.body, page: loaded.page } : null;
}

function withTrail<Body>(state: CatalogueCursorState<Body>, trail: CursorTrail): CatalogueCursorState<Body> {
  return trail === state.trail ? state : { ...state, trail };
}

export function catalogueCursorReducer<Body>(
  state: CatalogueCursorState<Body>,
  event: CatalogueCursorEvent<Body>,
): CatalogueCursorState<Body> {
  switch (event.kind) {
    case 'conditions':
      // 回第一页这件事要记进状态，不能只在渲染里派生：只派生不记，条件改回原样时旧轨迹会复活，人落回换条件之前翻到的那一页。
      return withTrail(state, cursorTrailFor(state.trail, event.signature));
    case 'next':
      return withTrail(state, nextCursorPage(state.trail, event.next));
    case 'previous':
      return withTrail(state, previousCursorPage(state.trail));
    case 'retry':
      return { ...state, attempt: state.attempt + 1 };
    case 'answered':
      // 只收此刻这一问的答复：换了条件或翻了页之后才回来的慢答复不盖掉新的一问。
      // 页号随答复一起记下：之后翻了页、这份答复成了垫着看的上一页时，翻页条照它当时的页号写。
      return event.key === catalogueCursorRequest(state).key
        ? { ...state, loaded: { key: event.key, answer: event.answer, page: cursorPageNumber(state.trail) } }
        : state;
  }
}
