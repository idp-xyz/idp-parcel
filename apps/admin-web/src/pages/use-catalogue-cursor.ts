import { useEffect, useReducer, useState } from 'react';
import { cursorPageNumber, type ListCursorPaginationProps } from '../templates';
import type { ApiResult } from './catalogue-api';
import {
  catalogueCursorAnswer,
  catalogueCursorReducer,
  catalogueCursorRequest,
  startCatalogueCursor,
} from './catalogue-cursor';
import { catalogueConditionsQuery, type CatalogueConditions, type CataloguePage } from './catalogue-query';

export interface CatalogueCursor<Body> {
  /** 此刻这一问的答复；还在途是 null（加载中）。 */
  answer: ApiResult<Body> | null;
  /** 接 ListPageTemplate 的 pagination 槽（游标形）；只在拿到业务答案后给。 */
  pagination: ListCursorPaginationProps | undefined;
  retry: () => void;
}

/**
 * 已迁到 ADR-0144 的目录页取数（票 catalogue-read-pagination/05）：按条件取第一页，下一页带上答复的游标，上一页由走过的游标回退，
 * 条件一变回第一页（决定一）。状态转移在 catalogue-cursor.ts。`load` 要是稳定引用（api.ts 的模块级读口函数），写成内联箭头会每次
 * 渲染重取；`conditions` 每次渲染可以是新对象，认的是它的签名。
 */
export function useCatalogueCursor<C extends CatalogueConditions, Body extends { page: CataloguePage }>(
  conditions: C,
  load: (conditions: C, after: string | null) => Promise<ApiResult<Body>>,
): CatalogueCursor<Body> {
  const signature = catalogueConditionsQuery(conditions);
  const [stored, dispatch] = useReducer(catalogueCursorReducer<Body>, signature, startCatalogueCursor<Body>);
  // 条件变了：这一帧就按第一页算，同时把它记进状态——组件可以在渲染里更新自己的状态，React 丢掉这一帧、带着新状态重渲。
  const state = catalogueCursorReducer(stored, { kind: 'conditions', signature });
  if (state !== stored) dispatch({ kind: 'conditions', signature });
  const request = catalogueCursorRequest(state);

  useEffect(() => {
    let cancelled = false;
    void load(conditions, request.after).then((answer) => {
      if (!cancelled) dispatch({ kind: 'answered', key: request.key, answer });
    });
    return () => {
      cancelled = true;
    };
    // conditions 不作依赖：它的签名已在 request.key 里，而它本身每次渲染都是新对象，作依赖会每次渲染重取。
  }, [load, request.key]);

  const answer = catalogueCursorAnswer(state);
  const page = answer?.kind === 'outcome' ? answer.body.page : null;
  return {
    answer,
    retry: () => dispatch({ kind: 'retry' }),
    pagination:
      page === null
        ? undefined
        : {
            mode: 'cursor',
            page: cursorPageNumber(state.trail),
            size: page.size,
            total: page.total,
            next: page.next,
            onPrevious: () => dispatch({ kind: 'previous' }),
            onNext: (next) => dispatch({ kind: 'next', next }),
          },
  };
}

const KEYWORD_SETTLE_MS = 300;

/**
 * 检索词停稳之后才交出去下推；首帧就是当前值（地址上带着 `?q=` 打开的页立刻按它取）。检索框是受控输入，每敲一个字就下推一问的话，
 * 答复互相追赶、表格区跟着逐字闪回加载中。框里的字与地址上的 `?q=` 照旧逐字跟手，只有发出去的那一问等它停稳。
 */
export function useSettledKeyword(keyword: string): string {
  const [settled, setSettled] = useState(keyword);
  useEffect(() => {
    const timer = window.setTimeout(() => setSettled(keyword), KEYWORD_SETTLE_MS);
    return () => window.clearTimeout(timer);
  }, [keyword]);
  return settled;
}
