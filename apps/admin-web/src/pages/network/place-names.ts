// 网点（物流节点）与口岸的显示名称索引（ADR-0353）：编码 → 此刻生效那一版登的名称。纯函数，node:test 钉着；取数与缓存在
// use-place-names.ts。
//
// 名称是可选的版本内容：登了名称的版本读口带 displayName，没登的不带这个键。所以按编码取名只看此刻生效的那一版——旧版登过的
// 名称不顶到新版上，新版没登就是没有名称；没有名称或查不到时调用方显编码，不猜、不写「未登记」（名称是可选格，缺席不是漏填）。

import { validityOf } from './network-rows';

/** 一行带生效区间与可选名称的版本，网点版本行与口岸行都能折成这个形状。 */
export interface NamedVersion {
  code: string;
  displayName?: string;
  from?: string;
  to?: string;
}

export type PlaceNameIndex = ReadonlyMap<string, string>;

/** 只收此刻生效且登了名称的版本；同一编码有多版同时生效（库面不许，读口兜错）时取先到的那一版。 */
export function placeNameIndexOf(versions: readonly NamedVersion[], now: Date): PlaceNameIndex {
  const index = new Map<string, string>();
  for (const version of versions) {
    if (!version.displayName || index.has(version.code)) continue;
    if (validityOf(version.from, version.to, now) === 'IN_FORCE') index.set(version.code, version.displayName);
  }
  return index;
}

/** 屏上写法：有名称显名称，没有显编码本身。 */
export function placeLabel(index: PlaceNameIndex, code: string | undefined): string {
  if (!code) return '—';
  return index.get(code) ?? code;
}

/** 「A → B」一类两端格的屏上写法，两端各自查名。 */
export function placePairLabel(index: PlaceNameIndex, from: string | undefined, to: string | undefined): string {
  return `${placeLabel(index, from)} → ${placeLabel(index, to)}`;
}
