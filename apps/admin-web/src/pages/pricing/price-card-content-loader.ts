// 价卡内容的取数：同一版价卡的内容不变（版本不可覆盖，摘要随行），所以读到了就记住；同一版同时被问几次（列表里每行的
// 「币种与单位」格、详情栏）只发一个请求；并发有上限，一屏几十行价卡不会一口气打出几十个请求。
//
// 只记业务答案里的「查到了」。未配置、没有授予、查无、传输失败都不记：它们会因为配置、授予、登记而变，记住就成了
// 「重试也不会好」的假象。纯逻辑、不碰 React 与 fetch（fetch 由调用方注入），node:test 直接钉。

import type { ApiResult, PriceCardContentBody } from './api';

export type ContentAnswer = ApiResult<PriceCardContentBody>;

export type ContentFetcher = (planId: string, planVersion: string) => Promise<ContentAnswer>;

export interface ContentLoader {
  /** 取一版价卡的内容。key 带上内容摘要：同一引用换了内容（不应发生）就不会拿到旧的。 */
  load(key: string, planId: string, planVersion: string): Promise<ContentAnswer>;
  /** 已记住的答案；没有即 null。 */
  peek(key: string): ContentAnswer | null;
}

export function contentKeyOf(row: { planId: string; planVersion: string; contentDigest: string }): string {
  return `${row.planId}@${row.planVersion}#${row.contentDigest}`;
}

function isFound(answer: ContentAnswer): boolean {
  return answer.kind === 'outcome' && answer.body.outcome === 'PRICE_CARD_CONTENT_FOUND';
}

export function createContentLoader(fetcher: ContentFetcher, maxConcurrent: number): ContentLoader {
  const remembered = new Map<string, ContentAnswer>();
  const inFlight = new Map<string, Promise<ContentAnswer>>();
  const waiting: Array<() => void> = [];
  let running = 0;

  const acquire = (): Promise<void> =>
    new Promise((resolve) => {
      if (running < maxConcurrent) {
        running += 1;
        resolve();
      } else {
        waiting.push(() => {
          running += 1;
          resolve();
        });
      }
    });

  const release = () => {
    running -= 1;
    waiting.shift()?.();
  };

  return {
    peek: (key) => remembered.get(key) ?? null,
    load(key, planId, planVersion) {
      const known = remembered.get(key);
      if (known !== undefined) return Promise.resolve(known);
      const pending = inFlight.get(key);
      if (pending !== undefined) return pending;
      const started = acquire()
        .then(() => fetcher(planId, planVersion))
        .finally(release)
        .then((answer) => {
          if (isFound(answer)) remembered.set(key, answer);
          return answer;
        })
        .finally(() => {
          inFlight.delete(key);
        });
      inFlight.set(key, started);
      return started;
    },
  };
}
