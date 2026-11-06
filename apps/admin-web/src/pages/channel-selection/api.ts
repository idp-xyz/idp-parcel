// 渠道择优决定查阅页的读面（票 label-channel/23）：GET /channel-selection-decisions。
//
// 形状以 internal/parcelshipment/adapters/http 的 query_channel_selection_decisions.go 为准，此处只做镜像
// 不虚构。列表与单份共用一个端点，按 decisionId 分派（与 /shipment-request-views 同形）；传输与五格判读
// 收敛在共享 catalogue-api。本页只有读——并列冲突的人工裁决不在本票（票 23「先答再开工」第一条）。
//
// 并列清单按 ADR-0144 翻页：view、scope、mapping 是册子选择器，与游标、`q` 同进一个查询串，答复多一格 page；
// 收哪些键见 ports.TiedChannelSelectionPages，集外的键答 400。按决定号取单份是有界查找，不翻页。

import { exchangeMasterData, type ApiResult } from '../catalogue-api';
import { catalogueRequestPath, type CatalogueConditions, type CataloguePage } from '../catalogue-query';

export type { ApiResult } from '../catalogue-api';

/**
 * 逐候选一行：候选引用、所用 BUY 评价引用（没登记价卡的候选没经过评价，缺席）、四格之一、出局因由
 * （只随 EXCLUDED 在场）。**没有金额**：金额留在 parcel-pricing 的评价上，读面只透评价引用（票 23 红线）。
 */
export interface ChannelSelectionCandidateRecord {
  candidate: string;
  evaluation?: string;
  outcome: string;
  exclusion?: string;
}

/**
 * 一条决定记录。selectedCandidate 只在有选中者时在场——并列冲突与无人参选两格都没有选中者，缺席就是
 * 「这一次没选出来」。四格与结论词是领域封闭集原名，页面译回中文原词、认不得的原样示出。
 */
export interface ChannelSelectionDecisionRecord {
  decisionId: string;
  scope: string;
  mapping: string;
  assembledAsOf: string;
  rule: string;
  decidedAt: string;
  conclusion: string;
  selectedCandidate?: string;
  candidates: ChannelSelectionCandidateRecord[];
}

export interface TiedChannelSelectionDecisionsResponseBody {
  outcome: 'TIED_CHANNEL_SELECTION_DECISIONS_LISTED';
  decisions: ChannelSelectionDecisionRecord[];
  page: CataloguePage;
}

export interface ChannelSelectionDecisionResponseBody {
  outcome: 'CHANNEL_SELECTION_DECISION';
  decision: ChannelSelectionDecisionRecord;
}

/** 被择优对象：商业范围引用 + 产品—渠道映射引用，收窄时两个都要给（服务端只给一半答 400）。 */
export interface ChannelSelectionSubjectFilter {
  scope: string;
  mapping: string;
}

/** 并列清单的查询条件：选择器只由 tiedChannelSelectionConditions 造，`view=tied` 不会漏、scope 与 mapping 不会只给一半。 */
export type TiedChannelSelectionConditions = CatalogueConditions & {
  readonly selectors:
    | { readonly view: 'tied' }
    | { readonly view: 'tied'; readonly scope: string; readonly mapping: string };
};

/**
 * `view=tied` 是必备维：「没传就当全部」是隐含默认，服务端拒。可按对象收窄——服务端只收到一半答 400，所以两个一起给或
 * 都不给。
 */
export function tiedChannelSelectionConditions(
  subject?: ChannelSelectionSubjectFilter,
  conditions: Omit<CatalogueConditions, 'selectors'> = {},
): TiedChannelSelectionConditions {
  return {
    ...conditions,
    selectors: subject ? { view: 'tied', scope: subject.scope, mapping: subject.mapping } : { view: 'tied' },
  };
}

/**
 * 列并列冲突。不做任何按时间的隐式截断——「近期」之类窄口要服务端长出显式参数后再接，页面不自己截；一页之外的由游标
 * 往后翻。
 */
export function listTiedChannelSelectionDecisions(
  conditions: TiedChannelSelectionConditions = tiedChannelSelectionConditions(),
  after: string | null = null,
): Promise<ApiResult<TiedChannelSelectionDecisionsResponseBody>> {
  return exchangeMasterData<TiedChannelSelectionDecisionsResponseBody>(
    catalogueRequestPath('/channel-selection-decisions', conditions, after),
  );
}

/** 按标识取一条。找不到是 404 CHANNEL_SELECTION_DECISION_NOT_VISIBLE，终局答案不是故障，由页面专门呈现。 */
export function findChannelSelectionDecision(
  decisionId: string,
): Promise<ApiResult<ChannelSelectionDecisionResponseBody>> {
  return exchangeMasterData<ChannelSelectionDecisionResponseBody>(
    `/channel-selection-decisions?decisionId=${encodeURIComponent(decisionId)}`,
  );
}
