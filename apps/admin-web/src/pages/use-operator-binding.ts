import { useEffect, useState, useSyncExternalStore } from 'react';
import { exchangeMasterData } from './catalogue-api';
import { boundTenantOf, type OperatorBindingResponseBody } from './operator-binding';

// 一页加载只问一次：顶栏作用域与集团与法人页头要的是同一个答案，而一个会话的操作者只绑一个租户（ADR-0100），换操作者要
// 重新登录、整页重载。在途与答出的都记在这一个模块级 promise 上，两处同时挂载也只发一次请求。答不出（依赖故障、读不到）
// 不记：那多半是一时的，记住就得整页重载才会再问。
let pending: Promise<string | undefined> | null = null;

// 取数缓存键的租户段（ADR-0360 决定三）：答出了是绑定的租户标识；问过但答不出是 unboundTenantKey——页面照样去问自己的
// 读口，各自答出拒绝或未配置，不因为顶栏那一问没答上就一直转圈；还没问完是 null，取数等它。键里带租户，换了绑定就换了一整片
// 键，上一个租户的答案不会显给下一个。
const unboundTenantKey = '(unbound)';
let tenantKey: string | null = null;
const tenantKeyListeners = new Set<() => void>();

function publishTenantKey(next: string): void {
  if (tenantKey === next) return;
  tenantKey = next;
  for (const listener of tenantKeyListeners) listener();
}

function loadBoundTenant(): Promise<string | undefined> {
  if (pending === null) {
    pending = exchangeMasterData<OperatorBindingResponseBody>('/operator-binding')
      .then(boundTenantOf)
      .then((tenant) => {
        if (tenant === undefined) pending = null;
        publishTenantKey(tenant ?? unboundTenantKey);
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

function subscribeTenantKey(listener: () => void): () => void {
  tenantKeyListeners.add(listener);
  return () => {
    tenantKeyListeners.delete(listener);
  };
}

/** 取数缓存键的租户段；还没问完是 null。读过之后再挂载的页同步拿到，不为它多等一帧。 */
export function useTenantKey(): string | null {
  const key = useSyncExternalStore(subscribeTenantKey, () => tenantKey, () => null);
  useEffect(() => {
    void loadBoundTenant();
  }, []);
  return key;
}

/** 渲染之外（预读、写后作废）用的租户段；还没问完是 null。 */
export function currentTenantKey(): string | null {
  return tenantKey;
}
