import { useEffect, useState } from 'react';
import { readPriceCardContent, type PriceCardRecord } from './api';
import { contentKeyOf, createContentLoader, type ContentAnswer } from './price-card-content-loader';

// 取一版价卡内容的钩子。同一版的内容不变，读到了就在模块级记住（列表里每行的「币种与单位」格与详情栏共用）；并发上限 4。
const loader = createContentLoader(readPriceCardContent, 4);

export function usePriceCardContent(row: Pick<PriceCardRecord, 'planId' | 'planVersion' | 'contentDigest'>): {
  answer: ContentAnswer | null;
  retry: () => void;
} {
  const key = contentKeyOf(row);
  const [arrived, setArrived] = useState<{ key: string; answer: ContentAnswer } | null>(null);
  const [reloadToken, setReloadToken] = useState(0);

  useEffect(() => {
    let cancelled = false;
    void loader.load(key, row.planId, row.planVersion).then((answer) => {
      if (!cancelled) setArrived({ key, answer });
    });
    return () => {
      cancelled = true;
    };
  }, [key, row.planId, row.planVersion, reloadToken]);

  // 方向键换行时：已记住的立刻有；没有就是加载中，不拿上一行的答案顶。
  const answer = loader.peek(key) ?? (arrived?.key === key ? arrived.answer : null);
  return {
    answer,
    retry: () => {
      setArrived(null);
      setReloadToken((token) => token + 1);
    },
  };
}
