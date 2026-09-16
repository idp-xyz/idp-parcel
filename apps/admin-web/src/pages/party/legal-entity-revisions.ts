// 集团与法人页详情抽屉「修订历史」区的判读（票 admin-web-group-legal-entities/03 第 5 条）——法人册对
// revision-timeline.ts 共用判读的薄适配。判读本体（序、标题、色调、停用两件、时刻格式化）在那边；这里只交
// 本册多出的那一格（钉着哪个参与方身份）与注释句的主语。票 12 把判读抽成共用时保住了本模块的导出名，
// GroupLegalEntitiesPage 与既有用例一字不动——它们守的行为没变。

import type { LegalEntityRevisionRecord } from './api';
import {
  identityRevisionHistoryNote,
  identityRevisionTimeline,
  type RevisionTimelineItem,
} from './revision-timeline';

export type { RevisionTimelineItem } from './revision-timeline';

/**
 * 每笔在共用格之外显「参与方身份 <partyId>」：法人钉着哪个业务参与方身份是这一笔修订的内容，两笔之间换了
 * 就在并排的描述里自己比。名称不显——它在参与方册上不随法人修订走（理由在 api.ts 的 LegalEntityRevisionRecord）。
 */
export function legalEntityRevisionTimeline(
  revisions: readonly LegalEntityRevisionRecord[],
  format: (iso: string) => string,
): RevisionTimelineItem[] {
  return identityRevisionTimeline(revisions, format, (row) => `参与方身份 ${row.partyId}`);
}

export function revisionHistoryNote(count: number): string {
  return identityRevisionHistoryNote(count, '法人');
}
