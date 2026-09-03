// 覆盖地平线摘要条的判读（票 pricing-reference-series-operations/05 第 3 项）。
//
// 判读放在纯函数里而不是组件里，与 operations 的 handover-scope-summary、party 的
// policy-rows 同一条：这一层要钉的是「哪两种状态不得折成一句」，而那用 node:test 钉得住，
// 用组件钉不住。组件只负责摆。

import type { ReferenceSeriesCoverageRecord } from './api';

/**
 * 覆盖那一格的三种答案。**三种不是两种**，而这正是本模块存在的理由：
 *
 * - `open`——有在用版本且它没有上界。**不是「剩余很多天」，是没有终点。**
 * - `bounded`——有在用版本且有止点，`remainingDays` 才有意义。
 * - `none`——今天没有在用版本。**它与 `bounded` 且剩余为 0 不是一回事**：前者是「没得用」
 *   （去登记或去复核），后者是「用到今天为止」（去登记下一版）。折成同一句话，看的人
 *   会跑错方向。
 */
export type CoverageKind = 'open' | 'bounded' | 'none';

export interface CoverageCell {
  kind: CoverageKind;
  /** 只在 `bounded` 时在场。可以为负——已经过期是一种真实状态，不夹逼到 0。 */
  remainingDays?: number;
  /** 只在 `open` / `bounded` 时在场。 */
  inForceVersion?: string;
}

/**
 * 待办那一格。两个计数分开给，因为续办动作不同：等复核人 vs 等登记方更正。
 *
 * `attention` 只回答「这一行需不需要有人看一眼」，**不回答「紧不紧急」**——紧急要阈值，
 * 而阈值是租户参数（票面第 4 项：机制只把数算出来摆着，阈值未配置）。所以页面据它决定
 * 要不要把那一格显出来，不据它标黄标红。
 */
export interface PendingCell {
  unreviewed: number;
  returned: number;
  attention: boolean;
}

export interface CoverageRow {
  seriesId: string;
  kind: string;
  registeredVersionCount: number;
  coverage: CoverageCell;
  pending: PendingCell;
  /** 最近一次复核；从未复核过的序列没有它，那是如实缺席不是零。 */
  lastReview?: { at: string; decision: string };
}

/**
 * 按 `asOf` 与止点算剩余天数。
 *
 * **向下取整到自然天，并明写它是按 UTC 算的**：后端刻意不算这个数，理由是「算差值要挑
 * 时区与舍入口径」——那两样确实得挑，所以这里挑了并写下来，而不是让它成为一个没人说得清
 * 口径的数字。两个时刻都是服务端给的 RFC3339，同一条时间轴上作差，不引入本地时区。
 */
export function remainingDaysBetween(asOf: string, effectiveTo: string): number {
  const from = Date.parse(asOf);
  const to = Date.parse(effectiveTo);
  if (Number.isNaN(from) || Number.isNaN(to)) return 0;
  return Math.floor((to - from) / 86_400_000);
}

export function coverageRowOf(
  record: ReferenceSeriesCoverageRecord,
  asOf: string,
): CoverageRow {
  // 先看布尔再看子对象：布尔是服务端说的「有没有」，子对象在不在场是序列化的事。
  // 反过来拿子对象去推有没有，会把一次响应不合契约读成「今天没有在用版本」。
  let coverage: CoverageCell;
  if (!record.inForceResolved || record.inForce === undefined) {
    coverage = { kind: 'none' };
  } else if (record.inForce.openEnded) {
    coverage = { kind: 'open', inForceVersion: record.inForce.version };
  } else {
    coverage = {
      kind: 'bounded',
      inForceVersion: record.inForce.version,
      // openEnded 为假时 effectiveTo 必在场（后端两者成对）；真缺了就是响应不合契约，
      // 按 0 天算会把它伪装成「今天到期」，所以退回 none 让页面如实说没得用。
      remainingDays:
        record.inForce.effectiveTo === undefined
          ? undefined
          : remainingDaysBetween(asOf, record.inForce.effectiveTo),
    };
    if (coverage.remainingDays === undefined) coverage = { kind: 'none' };
  }

  return {
    seriesId: record.seriesId,
    kind: record.kind,
    registeredVersionCount: record.registeredVersionCount,
    coverage,
    pending: {
      unreviewed: record.unreviewedVersionCount,
      returned: record.returnedVersionCount,
      attention: record.unreviewedVersionCount > 0 || record.returnedVersionCount > 0,
    },
    lastReview:
      record.lastReviewedAt !== undefined && record.lastReviewDecision !== undefined
        ? { at: record.lastReviewedAt, decision: record.lastReviewDecision }
        : undefined,
  };
}

/** 覆盖那一格的措辞。三种答案三句话，不共用一个破折号。 */
export function coverageNote(cell: CoverageCell): string {
  switch (cell.kind) {
    case 'open':
      return '在用版本无终点';
    case 'bounded':
      return cell.remainingDays !== undefined && cell.remainingDays < 0
        ? `在用版本已于 ${-cell.remainingDays} 天前到期`
        : `在用版本还盖 ${cell.remainingDays} 天`;
    case 'none':
      return '今天没有在用版本';
  }
}
