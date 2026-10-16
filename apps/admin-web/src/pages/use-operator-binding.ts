import { useEffect, useState } from 'react';
import { exchangeMasterData } from './catalogue-api';
import { boundTenantOf, type OperatorBindingResponseBody } from './operator-binding';

// 一页加载只问一次：顶栏作用域与集团与法人页头要的是同一个答案，而一个会话的操作者只绑一个租户（ADR-0100），换操作者要
// 重新登录、整页重载。在途与答出的都记在这一个模块级 promise 上，两处同时挂载也只发一次请求。答不出（依赖故障、读不到）
// 不记：那多半是一时的，记住就得整页重载才会再问。
let pending: Promise<string | undefined> | null = null;

function loadBoundTenant(): Promise<string | undefined> {
  if (pending === null) {
    pending = exchangeMasterData<OperatorBindingResponseBody>('/operator-binding')
      .then(boundTenantOf)
      .then((tenant) => {
        if (tenant === undefined) pending = null;
        return tenant;
      });
  }
  return pending;
}

/** 当前会话绑定的集团（租户）标识；答出之前、或答不出（未配置、拒绝、依赖故障）时是 undefined。 */
export function useBoundTenant(): string | undefined {
  const [tenant, setTenant] = useState<string | undefined>(undefined);
  useEffect(() => {
    let cancelled = false;
    void loadBoundTenant().then((next) => {
      if (!cancelled) setTenant(next);
    });
    return () => {
      cancelled = true;
    };
  }, []);
  return tenant;
}
