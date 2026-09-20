// 参与方模块册页持有列表状态的钩子（票 admin-web-group-legal-entities/10 第 5 条立于业务参与方页，票 13 第 6 条抬成
// 各册页共用并改重取做法）。页面持有读签的列表答案再传下去：登记签从这份答案取修订号建议，登记册答 REGISTERED /
// DEACTIVATED 时触发对应读签重取，两签共享同一份答案而不各取一次。
//
// 状态转移是纯函数 registerListReducer，node:test 钉着；钩子只负责把 load 接上 effect。

import { useEffect, useReducer } from 'react';
import type { ApiResult } from '../catalogue-api';

export interface RegisterListState<Body> {
  /** 最近一份答案；首取回来之前是 null（加载中）。 */
  answer: ApiResult<Body> | null;
  /** 重取序号：问了几次。effect 以它为依赖，每变一次重取一次。 */
  version: number;
}

export type RegisterListEvent<Body> = { kind: 'reload' } | { kind: 'answered'; answer: ApiResult<Body> };

/**
 * 重取**不清上一份业务答案**（票 10 评审 S1）：清了，登记签那一瞬看到的是「列表没取到」——未手改的修订建议闪回 1、
 * 标识格提示句闪成「列表里没有」，新答案到了再复原；开着的抽屉也会因行找不到而关掉再打开。旧册面在新答案到来前
 * 继续可用，是重取这一动作本来的语义：问的是「有没有更新」，不是「忘掉已知的」。
 *
 * 上一份**不是**业务答案（未配置 / 调用方问题 / 未形成答案 / 没到达）时照旧回到加载中：那几态下的「重试」按钮
 * 若不换成加载中，按的人看不到任何反应，分不出是没按到还是还没回。
 */
export function registerListReducer<Body>(
  state: RegisterListState<Body>,
  event: RegisterListEvent<Body>,
): RegisterListState<Body> {
  switch (event.kind) {
    case 'reload':
      return { answer: state.answer?.kind === 'outcome' ? state.answer : null, version: state.version + 1 };
    case 'answered':
      return { answer: event.answer, version: state.version };
  }
}

/**
 * 一册的列表状态：`answer` 最近一份答案、`retry` 重取、`version` 重取序号。`load` 要是稳定引用（模块级函数），
 * 写成内联箭头会每次渲染重取。未回的旧请求按 cancelled 丢，不让慢答案盖掉快答案。
 */
export function useRegisterList<Body>(load: () => Promise<ApiResult<Body>>) {
  const [state, dispatch] = useReducer(registerListReducer<Body>, { answer: null, version: 0 });

  useEffect(() => {
    let cancelled = false;
    void load().then((next) => {
      if (!cancelled) dispatch({ kind: 'answered', answer: next });
    });
    return () => {
      cancelled = true;
    };
  }, [load, state.version]);

  const retry = () => dispatch({ kind: 'reload' });
  return { answer: state.answer, retry, version: state.version };
}
