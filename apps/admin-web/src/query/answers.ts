import { useEffect, useRef } from 'react';
import { useQuery } from '@tanstack/react-query';
import type { ApiResult } from '../pages/catalogue-api';
import { currentTenantKey, useTenantKey } from '../pages/use-operator-binding';
import { queryClient } from './client';
import { answerToShow } from './freshness';

// 页面经这里读（ADR-0360）。键依次是租户段（use-operator-binding 的 useTenantKey）、域、资源、参数；页面只给后三段。
//
// 读函数照旧答 ApiResult 不抛。查询函数包一层不带参数的箭头：TanStack 会把查询上下文当第一个参数递进去，有的读函数
// 第一个参数是可选的查询条件，直接交出去就会被当成条件发给服务端。

export type AnswerKey = readonly (string | number)[];
type Loader<Body> = () => Promise<ApiResult<Body>>;

export interface AnswerQuery<Body> {
  /** 该画的那一份：还没答、或上一份不是 outcome 而正在重问时是 null（加载）。判读在 freshness.ts 的 answerToShow。 */
  answer: ApiResult<Body> | null;
  /** 再问一遍（刷新键、重试）。 */
  retry: () => void;
  /** 有一问在途，或还没答过——工作台刷新钮据此转圈、收尾报 toast。 */
  pending: boolean;
}

export interface AnswerOptions {
  /** 不满足时不发（对象页还没认出是哪一家）。 */
  enabled?: boolean;
  /**
   * 挂载时必问一遍，问到之前不给缓存里那份：写口要对着此刻判的读（资料表单预填、收尾旧版）用它。缓存里那份是新鲜期内
   * 别人改之前问到的，拿它预填再保存，会把别人刚落的那一版无声改回去。问到之后照常进缓存，别处同键的读跟着新。
   */
  fresh?: boolean;
}

/** 读一个答案：新鲜期内的缓存直接给，过期了先给缓存、后台再问。租户段还没问完时不发。 */
export function useAnswer<Body>(key: AnswerKey, load: Loader<Body>, options?: AnswerOptions): AnswerQuery<Body> {
  const tenant = useTenantKey();
  const enabled = tenant !== null && (options?.enabled ?? true);
  const fresh = options?.fresh === true;
  const query = useQuery({
    queryKey: [tenant, ...key],
    queryFn: () => load(),
    enabled,
    ...(fresh ? { staleTime: 0 } : {}),
  });
  return {
    answer: fresh && !query.isFetchedAfterMount ? null : answerToShow(query.data, query.isFetching),
    retry: () => {
      // 已有一问在途（多半是写后作废刚发起的那一问）就等它答，不掐掉重发一遍。
      if (enabled) void query.refetch({ cancelRefetch: false });
    },
    pending: query.isFetching || query.data === undefined,
  };
}

/**
 * 外部给的刷新信号（页面别处落册后拧的那把钥匙）一变就再问一遍；挂载时那一次不算——挂载本来就会按新鲜期决定问不问。
 * `retry` 每次渲染可以是新函数，按最新那一份调。
 */
export function useRetryWhenChanged(signal: unknown, retry: () => void): void {
  const last = useRef(signal);
  const latestRetry = useRef(retry);
  latestRetry.current = retry;
  useEffect(() => {
    if (Object.is(last.current, signal)) return;
    last.current = signal;
    latestRetry.current();
  }, [signal]);
}

// 不带参数的读口按读函数本身认：同一个模块级读函数在哪页用都是同一份缓存。键里的函数名只为调试时认得出，认的是序号——
// 打包后函数名会压成一两个字母，不同模块的读函数可能同名。
const loaderIds = new WeakMap<object, string>();
let nextLoaderId = 0;

/** 不带参数的读口的键：域 + 读函数。`load` 要是模块级函数，内联箭头每次渲染都是新键。 */
export function loaderKey(domain: string, load: object): AnswerKey {
  let id = loaderIds.get(load);
  if (id === undefined) {
    nextLoaderId += 1;
    id = `${(load as { name?: string }).name || 'load'}#${nextLoaderId}`;
    loaderIds.set(load, id);
  }
  return [domain, id];
}

/** 预读：新鲜期内已有的不再问。租户段还没问完时不读——那时页面自己也还没开始问。 */
export function prefetchAnswer<Body>(key: AnswerKey, load: Loader<Body>): void {
  const tenant = currentTenantKey();
  if (tenant === null) return;
  void queryClient.prefetchQuery({ queryKey: [tenant, ...key], queryFn: () => load() });
}

/**
 * 写口答到之后作废受影响的答案（ADR-0360 决定三）：以 `prefix` 打头的键一律过期，挂着的页（含藏着的标签）后台重问，
 * 没挂的下次用到再问。按前缀不按租户：一页加载里只有一个租户。
 */
export function invalidateAnswers(prefix: AnswerKey): void {
  void queryClient.invalidateQueries({
    predicate: (query) => prefix.every((part, index) => query.queryKey[index + 1] === part),
  });
}

/**
 * 写口答到之后作废一个前缀下的答案。只看 kind：outcome 是写口答到了（答的可能是业务上的「不予登记」，作废一次无害）；
 * 没答到的什么都没变，不作废。各域的写口出口（party-writes.ts、pricing-writes.ts）都经这里。
 */
export function invalidateWhenAnswered<Answer extends { kind: string }>(prefix: AnswerKey, request: Promise<Answer>): Promise<Answer> {
  return request.then((answer) => {
    if (answer.kind === 'outcome') invalidateAnswers(prefix);
    return answer;
  });
}
