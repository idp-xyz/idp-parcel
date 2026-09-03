// 参考序列目录的收窄（票 pricing-reference-series-operations/05 第 3 项「一键跳到该序列的复核动作」）。
//
// 两种收窄是两种东西，所以是两个参数而不是一个搜索词：
//
// - `focus` 是「只看这条序列」，按（标识 + 种类）**精确**匹配。它由覆盖摘要条上点一格触发，目的是
//   让这条序列的各版本与行上的「复核」按钮收到眼前；`null` 是没选，不是选了空串。键带种类是因为
//   覆盖读口的行键就是（标识 + 种类）——版本表没有把一条标识钉死在一个种类上，同标识两种类各出
//   一行是照实转写；只按标识只看会把后端刻意分开的两行又折回一行。
// - `keyword` 是搜索框里的词，**包含**匹配、不分大小写、扫标识 / 来源 / 登记责任方三格。
//
// 若把只看做成往搜索框里填标识，前缀同名的兄弟序列（`SYN-PRC-FUEL` 与 `SYN-PRC-FUEL-WEEKLY`）会
// 一起出现，而看的人以为自己只看着一条——这与本仓反复记的「两种状态长同一张脸」同族，所以在
// 类型上就分开。两者叠加而不是二选一：只看之内还能按责任方再筛。

import type { ReferenceSeriesRecord } from './api';

/** 覆盖读口一行的键：标识 + 种类。 */
export interface SeriesKey {
  seriesId: string;
  kind: string;
}

export interface SeriesNarrowing {
  focus: SeriesKey | null;
  keyword: string;
}

export function sameSeries(a: SeriesKey, b: SeriesKey): boolean {
  return a.seriesId === b.seriesId && a.kind === b.kind;
}

export function visibleSeriesRows(
  rows: ReferenceSeriesRecord[],
  { focus, keyword }: SeriesNarrowing,
): ReferenceSeriesRecord[] {
  const focused = focus === null ? rows : rows.filter((row) => sameSeries(row, focus));
  const needle = keyword.trim().toLowerCase();
  if (!needle) return focused;
  return focused.filter((row) =>
    [row.seriesId, row.sourceIdentifier, row.registrant].some((field) =>
      field.toLowerCase().includes(needle),
    ),
  );
}
