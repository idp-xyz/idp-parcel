// 业务参与方页详情抽屉「修订历史」区的判读（票 admin-web-group-legal-entities/12 第 5 条）——参与方册对
// revision-timeline.ts 共用判读的薄适配，与 legal-entity-revisions.ts 成对。判读本体在那边；这里只交本册多出的
// 那一格（名称）与注释句的主语。

import type { BusinessPartyRevisionRecord } from './api';
import {
  identityRevisionHistoryNote,
  identityRevisionTimeline,
  type RevisionTimelineItem,
} from './revision-timeline';

export type { RevisionTimelineItem } from './revision-timeline';

/**
 * 每笔在共用格之外显「名称 <partyName>」：名称登在参与方册自己的行上、随修订走，是这一笔的内容——
 * 「从哪份换到哪份」在并排的描述里自己比（理由在 api.ts 的 BusinessPartyRevisionRecord）。
 */
export function businessPartyRevisionTimeline(
  revisions: readonly BusinessPartyRevisionRecord[],
  format: (iso: string) => string,
): RevisionTimelineItem[] {
  return identityRevisionTimeline(revisions, format, (row) => `名称 ${row.partyName}`);
}

export function businessPartyRevisionHistoryNote(count: number): string {
  return identityRevisionHistoryNote(count, '参与方');
}
