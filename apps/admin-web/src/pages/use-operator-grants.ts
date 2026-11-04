import { useEffect, useState } from 'react';
import { exchangeMasterData } from './catalogue-api';
import { heldGrantsOf, mayMakeDecision, mayWriteRegistry, type HeldGrants, type OperatorGrantsResponseBody } from './operator-grants';

// 一页加载只问一次：各页的写入口都要这同一个答案，而操作者换授予要整页重载才重新问（授予在铸造那一刻取，
// 令牌之外没有会话态）。在途与答出的都记在模块级，各页同时挂载也只发一次请求；答出的值留着，后挂载的页第一帧就拿到，
// 不先摆出入口再收回。答不出（依赖故障、拒绝、读不到）不记：那多半是一时的，记住就得整页重载才会再问。
let pending: Promise<HeldGrants | undefined> | null = null;
let settled: HeldGrants | undefined;

function loadHeldGrants(): Promise<HeldGrants | undefined> {
  if (pending === null) {
    pending = exchangeMasterData<OperatorGrantsResponseBody>('/operator-grants')
      .then(heldGrantsOf)
      .then((held) => {
        if (held === undefined) pending = null;
        else settled = held;
        return held;
      });
  }
  return pending;
}

/**
 * 登记与发布入口摆不摆。答出之前、或答不出时是 true：入口照摆，让服务端去答——摆不摆只是呈现（ADR-0315），
 * 只有明确知道没有登记册配置写授予才收起。
 */
export function useMayWriteRegistry(): boolean {
  return useRegistryWriteGrant().mayWrite;
}

/**
 * 登记册配置写授予的两格（票 company-subject-section-edit/05）：`settled` 是授予答出了没有，`mayWrite` 与 useMayWriteRegistry 同一
 * 口径（答出之前、答不出时都是 true）。要在「明确没有授予」时做点什么（把人从编辑态地址上请出去）的页面看这两格——
 * 只看 mayWrite 分不清「还没答」和「没有」，加载期就动手会把有授予的人也踢出去。
 */
export function useRegistryWriteGrant(): { settled: boolean; mayWrite: boolean } {
  const [held, setHeld] = useState<HeldGrants | undefined>(settled);
  useEffect(() => {
    let cancelled = false;
    void loadHeldGrants().then((next) => {
      if (!cancelled) setHeld(next);
    });
    return () => {
      cancelled = true;
    };
  }, []);
  return { settled: held !== undefined, mayWrite: mayWriteRegistry(held) };
}

/** 某一种运营决定的写入口摆不摆。答出之前与答不出时是 true（ADR-0315）：只有明确知道没有那一种授予才收起。 */
export function useMayMakeDecision(decision: string): boolean {
  const [held, setHeld] = useState<HeldGrants | undefined>(settled);
  useEffect(() => {
    let cancelled = false;
    void loadHeldGrants().then((next) => {
      if (!cancelled) setHeld(next);
    });
    return () => {
      cancelled = true;
    };
  }, []);
  return mayMakeDecision(held, decision);
}
