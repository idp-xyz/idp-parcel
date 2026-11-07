// 物流动态工作台的行模型、进度折叠与本页状态筛选的纯逻辑（改版第一步，设计源「五、改版示意」的「物流动态」列）。
// node:test 钉着；页面只负责摆。检索词与表头先后已下推到读口（list-conditions 的 trackingListConditions），这里不再筛、不再排。
//
// 行 = 一份当前投影版本，各格全部从端点体派生，不引入端点体之外的字段。五个源上下文不再各占一列、各显英文事实类型，
// 而是折成一格「进度」：每源最后一条当前有效条目的事实类型经用语词典译成一句短语（「已收寄 · 已放行」），缺的源不显。
// 这仍不是统一状态机——各源各一段、按服务链顺序排在一起只是阅读便利，GLOSSARY「追踪摘要」禁统一状态机那条规则照旧。

import { byInstant, byString } from '../party/list-order';
import { labelFor } from '../../vocabulary/status';
import type { ProjectionEntryRecord, TrackingProjectionRecord } from './api';

/**
 * 当前有效条目 = 未被同投影内任何条目指名为前身的条目。替代关系只在同一源上下文、同一事实引用的版本之间成立
 * （CONTEXT「来源事实替代关系」），判据据此取 (source, fact, 版本) 三段；被替代条目继续在场是为可追溯，各维度与里程碑
 * 只由当前有效条目派生（CONTEXT「追踪投影与里程碑」）。
 */
export function currentEffectiveEntries(entries: readonly ProjectionEntryRecord[]): ProjectionEntryRecord[] {
  const superseded = new Set(
    entries.filter((entry) => entry.supersedes).map((entry) => `${entry.source}\u0000${entry.fact}\u0000${entry.supersedes}`),
  );
  return entries.filter((entry) => !superseded.has(`${entry.source}\u0000${entry.fact}\u0000${entry.factVersion}`));
}

/** 服务链方向（托运 → 路由 → 节点 → 运输 → 关务）只是阅读便利，不含语义主张。 */
export const sourceOrder = ['PARCEL_SHIPMENT', 'NETWORK_ROUTING', 'NODE_OPERATIONS', 'TRANSPORT_FULFILLMENT', 'CUSTOMS_COMPLIANCE'];

export interface TrackingRow {
  parcelId: string;
  projectionVersion: string;
  priorVersion?: string;
  derivedAt: string;
  /** 当前有效已归类条目的最后一条的里程碑码；一条都归不上时为空串，页面如实显「未归类」。 */
  latestMilestone: string;
  /** 被替代条目数。 */
  supersededCount: number;
  /** 当前有效条目数。 */
  currentCount: number;
  /**
   * 每源最后一条当前有效条目，按业务发生时间先后排、同时刻按 sourceOrder——读起来是「这票货先后经过了什么」；托运段的
   * 终局往往最后发生，按服务链排会把「已完结」排到最前。每源内部条目按业务发生时间自然排序由派生侧保证，本页不另立判据。
   */
  latestBySource: ProjectionEntryRecord[];
}

export function trackingRowOf(record: TrackingProjectionRecord): TrackingRow {
  const current = currentEffectiveEntries(record.entries);
  const latest = new Map<string, ProjectionEntryRecord>();
  for (const entry of current) latest.set(entry.source, entry);
  const classified = current.filter((entry) => entry.milestone);
  const ordered = [...latest.values()].sort(
    (left, right) => byInstant(left.occurredAt, right.occurredAt) || rankOf(left.source) - rankOf(right.source),
  );
  return {
    parcelId: record.parcel,
    projectionVersion: record.version,
    priorVersion: record.priorVersion,
    derivedAt: record.derivedAt,
    latestMilestone: classified.length ? (classified[classified.length - 1].milestone as string) : '',
    supersededCount: record.entries.length - current.length,
    currentCount: current.length,
    latestBySource: ordered,
  };
}

function rankOf(source: string): number {
  const index = sourceOrder.indexOf(source);
  return index === -1 ? sourceOrder.length : index;
}

/** 「进度」一格：各源最后一步的中文短语，按发生先后；没有条目的源不出现。连续同词只留一个（节点「已收寄」与运输「已收寄」）。 */
export function progressPhrasesOf(row: TrackingRow): string[] {
  const phrases: string[] = [];
  for (const entry of row.latestBySource) {
    const word = labelFor(entry.kind);
    if (phrases[phrases.length - 1] !== word) phrases.push(word);
  }
  return phrases;
}

/** 当前状态一格的词：最新里程碑经词典；没有归类上的里程碑如实说「未归类」——那是 CONTEXT 硬句，不是待修的缺陷。 */
export const unclassifiedNote = '未归类';

export function currentStatusLabelOf(row: TrackingRow): string {
  return row.latestMilestone ? labelFor(row.latestMilestone) : unclassifiedNote;
}

/**
 * 当前状态胶囊只筛本页：状态是从条目派生的词，投影读口没有这一维（ports.TrackingProjectionPages 不放筛选维），下推筛不出来。
 * status 取「全部」或某个当前状态的词（词经词典，同词的里程碑码算一格）。
 */
export function trackingRowsOfStatus(rows: readonly TrackingRow[], status: 'ALL' | string): TrackingRow[] {
  return status === 'ALL' ? [...rows] : rows.filter((row) => currentStatusLabelOf(row) === status);
}

export interface TrackingStatusCount {
  label: string;
  count: number;
}

/** 按当前状态的词计数，只列出现过的；按出现次数多→少排，同数按词。未归类也算一格。 */
export function countTrackingByStatus(rows: readonly TrackingRow[]): TrackingStatusCount[] {
  const counts = new Map<string, number>();
  for (const row of rows) {
    const label = currentStatusLabelOf(row);
    counts.set(label, (counts.get(label) ?? 0) + 1);
  }
  return [...counts.entries()]
    .map(([label, count]) => ({ label, count }))
    .sort((left, right) => right.count - left.count || byString(left.label, right.label));
}

export const trackingNoMatchNote = '当前筛选条件下没有匹配的包裹';

/** 检索与表头已下推，读口这一页都对；本页一格不剩只会是状态胶囊筛空了，说清那只是这一页。 */
export const trackingNoMatchOnPageNote = '这一页没有该状态的包裹；状态只筛本页，翻页看其余几页';
