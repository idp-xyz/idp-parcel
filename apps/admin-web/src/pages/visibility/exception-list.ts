// 异常与索赔区各列表的行状态与计数纯逻辑：一行该亮哪个状态胶囊、指标格怎么数、检索命中什么。node:test 钉着；页面只负责摆。
//
// 状态胶囊一律给「码 + 词典分册」，词与色调由用语词典定；只有册上没有码可指的格（还没分诊、还没审核、还没有任何通知节点）
// 才由这里给一个派生词，并且派生词与真码分开——「还没答」不演成任何一个已答的值。

import type { WorkbenchSort } from '../../templates/workbench';
import { byInstant, byString } from '../party/list-order';
import type { StatusTone } from '../../vocabulary/status';
import { statusEntryFor } from '../../vocabulary/status';
import type {
  ClaimItemRecord,
  CustomerNotificationRecord,
  NotificationMilestoneRecord,
  SignalEpisodeRecord,
} from './case-api';
import { notificationScope } from './case-presentation';

/** 一枚状态胶囊要的东西：有码给码（与分册），没有码时给派生词与色调。 */
export interface RowStatus {
  code?: string;
  scope?: string;
  label: string;
  tone: StatusTone;
}

export function codeStatus(code: string, scope?: string): RowStatus {
  const entry = statusEntryFor(code, scope);
  return { code, scope, label: entry.label, tone: entry.tone };
}

/** 尚未分诊的发作期：信号已到、还没有分诊结论，要有人看，琥珀。 */
export const untriagedLabel = '待分诊';

/** 信号发作期的状态：有分诊结论示结论（自动立案 / 关联既有案件 / 进入人工复核 / 不立案），没有则「待分诊」。 */
export function episodeStatus(episode: SignalEpisodeRecord): RowStatus {
  return episode.triage ? codeStatus(episode.triage.outcome) : { label: untriagedLabel, tone: 'warn' };
}

/** 处置请求还没有接收方答复：等对方判断，蓝（推进中），不是任何一种判断。 */
export const awaitingJudgmentLabel = '等对方答复';

export function dispositionStatus(request: { judgment?: string; supersededBy?: string }): RowStatus {
  // 被替代的请求照列（替代不是删除），但它已不是在办的那一版，状态说「已替代」。
  if (request.supersededBy) return codeStatus('SUPERSEDED');
  return request.judgment ? codeStatus(request.judgment, 'disposition-judgment') : { label: awaitingJudgmentLabel, tone: 'info' };
}

/** 尚未审核的索赔项：收到了、资格还没判，琥珀。 */
export const unscreenedLabel = '待审核';

/**
 * 索赔项的状态取最靠后的那一判：撤回 > 责任结论 > 资格审核 > 待审核。三判是分步的不同判断（收到、通过资格审核、确认
 * 赔偿责任），胶囊只示走到哪一步；各判的原值在详情与技术详情里分别照列，不压并。
 */
export function claimStatus(item: ClaimItemRecord): RowStatus {
  if (item.withdrawn) return codeStatus('WITHDRAWN');
  if (item.conclusion) return codeStatus(item.conclusion);
  if (item.screen) return codeStatus(item.screen);
  return { label: unscreenedLabel, tone: 'warn' };
}

/** 一个通知节点都还没有：决定已作出、过程还没开始，琥珀。 */
export const noMilestoneLabel = '待发送';

/**
 * 通知进度取登记时刻最晚的那个节点。渠道接受、送达与客户确认在册上分别记录、不覆盖；这里只读最后一步走到哪，
 * 不把它们折成一个「已通知」——哪个结果满足通知义务由客户合同与通知策略判断。
 */
export function latestNotificationMilestone(milestones: readonly NotificationMilestoneRecord[]): NotificationMilestoneRecord | undefined {
  let latest: NotificationMilestoneRecord | undefined;
  for (const node of milestones) {
    if (latest === undefined || Date.parse(node.recordedAt) >= Date.parse(latest.recordedAt)) latest = node;
  }
  return latest;
}

export function notificationStatus(notification: CustomerNotificationRecord): RowStatus {
  const latest = latestNotificationMilestone(notification.milestones);
  return latest ? codeStatus(latest.milestone, notificationScope) : { label: noMilestoneLabel, tone: 'warn' };
}

/** 按状态词计数，供指标格与胶囊计数用；顺序按首次出现，计数为零的词不出现。 */
export function countByStatus<Row>(rows: readonly Row[], statusOf: (row: Row) => RowStatus): { label: string; tone: StatusTone; count: number }[] {
  const counts = new Map<string, { label: string; tone: StatusTone; count: number }>();
  for (const row of rows) {
    const status = statusOf(row);
    const seen = counts.get(status.label);
    if (seen) seen.count += 1;
    else counts.set(status.label, { label: status.label, tone: status.tone, count: 1 });
  }
  return [...counts.values()];
}

/** 一列怎么排：取行上的字，instant 为真时按时刻比（缺席排最前）。 */
export interface SortAccessor<Row> {
  value: (row: Row) => string | undefined;
  instant?: boolean;
}

/** 按表头排序态排行；不认识的键原序返回。 */
export function sortRowsBy<Row>(rows: readonly Row[], sort: WorkbenchSort, accessors: Readonly<Record<string, SortAccessor<Row>>>): Row[] {
  const accessor = accessors[sort.key];
  if (accessor === undefined) return [...rows];
  const compare = accessor.instant ? byInstant : byString;
  return [...rows].sort((left, right) => sort.dir * compare(accessor.value(left) ?? '', accessor.value(right) ?? ''));
}
