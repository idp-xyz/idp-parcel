import { useSyncExternalStore } from 'react';
import { listCountries, listCurrencies } from './reference-data/api';
import {
  emptyRefdataNames,
  refdataNamesFromAnswers,
  setCurrentRefdataNames,
  type RefdataNames,
} from './refdata-names';

// 国家 / 币种译名的取数与订阅（ADR-0335 决定七）。目录是产品内置的、整表几百行的常量，所以一个标签页里整表读一次、
// 全站共用（与 use-customer-names 同一做法：模块级缓存）；读不全（未配置、拒绝、依赖故障）不缓存，下次挂载再问，
// 读不到时页面按码显示，不把一次读不到当成“目录里没有”。
//
// 用 useSyncExternalStore 订阅，而不是每个组件各存一份状态：列表的每个单元格都可以直接调这个钩子，
// 订阅便宜，名表读回时所有订阅者一起重绘。

let snapshot: RefdataNames = emptyRefdataNames;
let loading = false;
let loaded = false;
const listeners = new Set<() => void>();

function publish(next: RefdataNames): void {
  snapshot = next;
  setCurrentRefdataNames(next);
  for (const listener of listeners) listener();
}

function ensureLoaded(): void {
  if (loaded || loading) return;
  loading = true;
  void Promise.all([listCountries(), listCurrencies()])
    .then(([countries, currencies]) => {
      const result = refdataNamesFromAnswers(countries, currencies);
      loaded = result.complete;
      // 这次没读全时不拿空表盖掉之前已经读回的那一本：读不回只是这一次的事，不是目录变空了。
      publish({
        countries: result.names.countries.size > 0 ? result.names.countries : snapshot.countries,
        currencies: result.names.currencies.size > 0 ? result.names.currencies : snapshot.currencies,
      });
    })
    .catch(() => undefined)
    .finally(() => {
      loading = false;
    });
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  ensureLoaded();
  return () => {
    listeners.delete(listener);
  };
}

/** 国家 / 币种译名表。读回之前与读不到时是空表；配合 countryLabel / currencyLabel 使用，调用它是为了名表读回时重绘。 */
export function useRefdataNames(): RefdataNames {
  return useSyncExternalStore(
    subscribe,
    () => snapshot,
    () => emptyRefdataNames,
  );
}
