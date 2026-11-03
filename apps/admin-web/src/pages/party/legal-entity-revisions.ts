// 集团与法人对象页「登记记录」签的判读（票 admin-web-group-legal-entities/03 第 5 条）——法人册对
// revision-timeline.ts 共用判读的薄适配。判读本体（序、标题、色调、停用两件、时刻格式化）在那边；这里只交
// 本册多出的内容（钉着哪个往来单位档案、身份两格与更正来源单据）与注释句的主语。票 12 把判读抽成共用时保住了本模块的
// 导出名，GroupLegalEntitiesPage 与既有用例一字不动——它们守的行为没变。

import type { LegalEntityRevisionRecord } from './api';
import { countryLabel } from '../refdata-names';
import { identityLayerCellsOf, type RegistrationNumberTypeNameOf } from './legal-entity-identity';
import { identityLayerAbsentNote, identityRecordNoun, identityRevisionLabel } from './presentation';
import {
  identityRevisionHistoryNote,
  identityRevisionTimeline,
  type InstantFormatter,
  type RevisionTimelineItem,
} from './revision-timeline';

export type { RevisionTimelineItem } from './revision-timeline';

/**
 * 每笔在共用格之外显「往来单位档案 <partyId>」与身份两格：法人钉着哪个业务参与方身份、登的是哪国哪个号，都是这一笔
 * 登记的内容，两笔之间换了就在并排的描述里自己比。名称不显——它在参与方册上不随法人修订走（理由在 api.ts 的
 * LegalEntityRevisionRecord）。标题短写「第 N 版」：轴名由「登记记录」签承担（presentation.ts 的 identityRevisionLabel）。
 * `typeNameOf` 是注册号类型目录折成的查法（legal-entity-identity.ts），不交或译不出按码显。
 */
export function legalEntityRevisionTimeline(
  revisions: readonly LegalEntityRevisionRecord[],
  format: InstantFormatter,
  typeNameOf?: RegistrationNumberTypeNameOf,
): RevisionTimelineItem[] {
  return identityRevisionTimeline(
    revisions,
    format,
    (row) => legalEntityRevisionContent(row, typeNameOf),
    identityRevisionLabel,
  );
}

/**
 * 身份层没登的那笔如实写 identityLayerAbsentNote：历史修订登记于身份层落地之前，不是漏填（票 legal-entity-profile/04
 * 「历史修订两格为空时如实写」）。更正来源单据只在身份更正那一笔在场，键不在就不说。注册地经基础数据译名
 * （ADR-0335 决定七）；译名表没读回时原样显码，与列表格同一口径。
 */
function legalEntityRevisionContent(row: LegalEntityRevisionRecord, typeNameOf?: RegistrationNumberTypeNameOf): string {
  const cells = identityLayerCellsOf(row, typeNameOf);
  const identity =
    cells === null
      ? `注册地与注册号：${identityLayerAbsentNote}`
      : `注册地 ${countryLabel(cells.country)} · 注册号 ${cells.numbers}`;
  const correction = row.identityCorrectionBasis !== undefined ? ` · 更正来源单据 ${row.identityCorrectionBasis}` : '';
  return `往来单位档案 ${row.partyId} · ${identity}${correction}`;
}

export function revisionHistoryNote(count: number): string {
  return identityRevisionHistoryNote(count, '公司', identityRecordNoun);
}
