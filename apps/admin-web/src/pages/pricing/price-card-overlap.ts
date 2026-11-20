// 价卡适用期重叠的话（票 02）。只提示，不拦保存、批准、发布：新版要先入册，旧版才经指名后继收尾。
// 同方案盖不住尾部，和另一个方案，都是 GOVERNANCE_DECISION，但下一步不一样，分开说。

import { formatInstant } from '../moment';
import type { PriceCardOverlap } from './api';

export interface PriceCardOverlapLine {
  key: string;
  text: string;
}

const doesNotBlock = '这只是提示，不阻止保存、批准或发布。';

function momentText(value: string): string {
  return value.includes('T') ? formatInstant(value) : value;
}

function rangeText(start: string, end?: string): string {
  if (!end) return `${momentText(start)} 起，没有截止`;
  return `${momentText(start)} 至 ${momentText(end)}（不含截止）`;
}

export function priceCardOverlapLines(overlaps: readonly PriceCardOverlap[] | undefined): PriceCardOverlapLine[] {
  if (!overlaps || overlaps.length === 0) return [];
  return overlaps.map((item, index) => {
    const ref = `${item.plan.id}@${item.plan.version}`;
    const span = rangeText(item.overlapStart, item.overlapEnd);
    const key = `${index}-${item.plan.id}@${item.plan.version}-${item.resolution}`;
    switch (item.resolution) {
      case 'SUCCESSOR_CUT':
        return {
          key,
          text: `同一方案的 ${ref} 仍与这张卡同时适用，交集是 ${span}。发布之后可以对旧版做后继截止。${doesNotBlock}`,
        };
      case 'RETIRE_PREDECESSOR':
        return {
          key,
          text: `同一方案的 ${ref} 与这张卡同时适用，交集是 ${span}。这张卡的起点不晚于旧版，截止会把旧版截没，发布之后应走退役。${doesNotBlock}`,
        };
      case 'GOVERNANCE_DECISION':
        if (item.samePlan) {
          return {
            key,
            text: `同一方案的 ${ref} 与这张卡同时适用，交集是 ${span}。这张卡盖不住旧版的尾部，自动收尾会留空档，需要人来裁。${doesNotBlock}`,
          };
        }
        return {
          key,
          text: `另一个方案 ${ref} 与这张卡同时适用，交集是 ${span}。收尾只处理同一方案的旧版，这一张要人来裁。${doesNotBlock}`,
        };
      default:
        return { key, text: `${ref} 与这张卡的适用期有交集（${span}）。${doesNotBlock}` };
    }
  });
}
