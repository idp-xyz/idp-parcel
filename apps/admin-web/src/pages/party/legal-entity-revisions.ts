// 集团与法人页详情抽屉「修订历史」区的判读（票 admin-web-group-legal-entities/03 第 5 条）——法人册对
// revision-timeline.ts 共用判读的薄适配。判读本体（序、标题、色调、停用两件、时刻格式化）在那边；这里只交
// 本册多出的内容（钉着哪个参与方身份、身份两格与身份更正依据）与注释句的主语。票 12 把判读抽成共用时保住了本模块的
// 导出名，GroupLegalEntitiesPage 与既有用例一字不动——它们守的行为没变。

import type { LegalEntityRevisionRecord } from './api';
import { identityLayerCellsOf } from './legal-entity-identity';
import { identityLayerAbsentNote } from './presentation';
import {
  identityRevisionHistoryNote,
  identityRevisionTimeline,
  type RevisionTimelineItem,
} from './revision-timeline';

export type { RevisionTimelineItem } from './revision-timeline';

/**
 * 每笔在共用格之外显「参与方身份 <partyId>」与身份两格：法人钉着哪个业务参与方身份、登的是哪国哪个号，都是这一笔
 * 修订的内容，两笔之间换了就在并排的描述里自己比。名称不显——它在参与方册上不随法人修订走（理由在 api.ts 的
 * LegalEntityRevisionRecord）。
 */
export function legalEntityRevisionTimeline(
  revisions: readonly LegalEntityRevisionRecord[],
  format: (iso: string) => string,
): RevisionTimelineItem[] {
  return identityRevisionTimeline(revisions, format, legalEntityRevisionContent);
}

/**
 * 身份层没登的那笔如实写 identityLayerAbsentNote：历史修订登记于身份层落地之前，不是漏填（票 legal-entity-profile/04
 * 「历史修订两格为空时如实写」）。更正依据只在身份更正那一笔在场，键不在就不说。
 */
function legalEntityRevisionContent(row: LegalEntityRevisionRecord): string {
  const cells = identityLayerCellsOf(row);
  const identity =
    cells === null
      ? `注册国家 / 地区与终身注册号：${identityLayerAbsentNote}`
      : `注册国家 / 地区 ${cells.country} · 终身注册号 ${cells.numbers}`;
  const correction = row.identityCorrectionBasis !== undefined ? ` · 身份更正依据 ${row.identityCorrectionBasis}` : '';
  return `参与方身份 ${row.partyId} · ${identity}${correction}`;
}

export function revisionHistoryNote(count: number): string {
  return identityRevisionHistoryNote(count, '法人');
}
