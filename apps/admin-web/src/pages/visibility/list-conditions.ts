// 物流动态与异常、索赔区各列表的查询条件（票 catalogue-read-pagination-rollout/14；契约 ADR-0144 决定三、四）：屏上的检索词、
// 状态胶囊与表头先后换成读口的条件，读口点名的维下推，不在答回来的那一页上再筛再排。各册点名了哪些维只在
// internal/visibilityexception/ports 的 catalogue_pages.go 一处，这里照它取名、不另造——发了没点名的键，整个请求答 400。
//
// 读口不认的派生状态（还没分诊、还没答复、已被替代）筛不出来：保留成只筛本页的胶囊，胶囊 id 另取，不进查询串，页面字面写明「本页」。
// 纯函数，node:test 钉着；页面只负责摆。

import type { WorkbenchSort } from '../../templates/workbench';
import type { CatalogueConditions } from '../catalogue-query';
import {
  claimsRecoveryConditions,
  exceptionTriageConditions,
  type ClaimsRecoveryConditions,
  type ClaimsRecoveryRegistry,
  type DispositionRequestRecord,
  type ExceptionTriageConditions,
  type SignalEpisodeRecord,
} from './case-api';
import { casePhaseCodes, dispositionJudgmentCodes, triageOutcomeCodes } from './case-presentation';

/** 状态胶囊「全部」的 id：不带状态一维，也不在本页上筛。 */
export const allStatuses = 'ALL';

/**
 * 表头排序态 → 读口的 `sort`。缺省序（读口不带 sort 时的次序）不写进查询串，不带检索的第一页因此是裸路径；其余按列换成读口的
 * 维名，倒序加 `-`。没有对应维的列不该给 sortKey——查不到就当缺省序。
 */
export function workbenchSortParam(
  sort: WorkbenchSort,
  fields: Readonly<Record<string, string>>,
  defaultSort: WorkbenchSort,
): string | undefined {
  const field = fields[sort.key];
  if (field === undefined || (sort.key === defaultSort.key && sort.dir === defaultSort.dir)) return undefined;
  return sort.dir === 1 ? field : `-${field}`;
}

function isOneOf<Code extends string>(codes: readonly Code[], value: string): value is Code {
  return (codes as readonly string[]).includes(value);
}

// —— 物流动态（ports.TrackingProjectionPages）——

/** 默认更新时间新→旧：客服先看刚动过的那一票；也是读口的缺省序。 */
export const defaultTrackingSort: WorkbenchSort = { key: 'derived-at', dir: -1 };

/** 投影没有状态一格，读口不放筛选维；检索词与表头两列的先后下推。 */
export function trackingListConditions(keyword: string, sort: WorkbenchSort): CatalogueConditions {
  return { q: keyword, sort: workbenchSortParam(sort, { parcel: 'parcel', 'derived-at': 'derivedAt' }, defaultTrackingSort) };
}

// —— 异常案件（ports.ExceptionCasePages）——

export type CasePhaseFilter = typeof allStatuses | (typeof casePhaseCodes)[number];

export const defaultCaseSort: WorkbenchSort = { key: 'established', dir: -1 };

/** 状态胶囊下推成 phase 一维。案件号列不给排：单据号可空，读口不放它作可排维。 */
export function exceptionCaseConditions(keyword: string, phase: CasePhaseFilter, sort: WorkbenchSort): CatalogueConditions {
  return {
    q: keyword,
    filters: phase === allStatuses ? {} : { phase: [phase] },
    sort: workbenchSortParam(sort, { parcel: 'rootParcel', established: 'establishedAt' }, defaultCaseSort),
  };
}

// —— 异常信号（ports.SignalEpisodePages）——

/** 只筛本页的「待分诊」胶囊：尚未分诊的发作期不落在读口 outcome 一维的任何词上，下推筛不出它。 */
export const untriagedOnPage = 'PAGE:UNTRIAGED';

export const defaultEpisodeSort: WorkbenchSort = { key: 'started', dir: -1 };

/** 胶囊是分诊结论之一就下推成 outcome；「待分诊」只筛本页，不进查询串。 */
export function signalEpisodeConditions(
  keyword: string,
  status: string,
  sort: WorkbenchSort,
): ExceptionTriageConditions<'signal-episode'> {
  return exceptionTriageConditions('signal-episode', {
    q: keyword,
    filters: isOneOf(triageOutcomeCodes, status) ? { outcome: [status] } : {},
    sort: workbenchSortParam(sort, { parcel: 'parcel', started: 'startedAt', 'last-hit': 'lastHitAt' }, defaultEpisodeSort),
  });
}

/** 只筛本页的那一格在到手的行上筛；下推了的胶囊读口已筛过，这里原样放行。 */
export function episodeOnPage(episode: SignalEpisodeRecord, status: string): boolean {
  return status !== untriagedOnPage || episode.triage === undefined;
}

// —— 处置请求（ports.DispositionRequestPages）——

/** 只筛本页的两格：还没有答复、已被替代——读口的 judgment 一维筛不出它们（被替代的请求照列，也按它自己的答复参与筛选）。 */
export const awaitingJudgmentOnPage = 'PAGE:AWAITING_JUDGMENT';
export const supersededOnPage = 'PAGE:SUPERSEDED';

export const defaultRequestSort: WorkbenchSort = { key: 'sent', dir: -1 };

export function dispositionRequestConditions(
  keyword: string,
  status: string,
  sort: WorkbenchSort,
): ExceptionTriageConditions<'disposition-request'> {
  return exceptionTriageConditions('disposition-request', {
    q: keyword,
    filters: isOneOf(dispositionJudgmentCodes, status) ? { judgment: [status] } : {},
    sort: workbenchSortParam(sort, { sent: 'sentAt' }, defaultRequestSort),
  });
}

export function requestOnPage(request: Pick<DispositionRequestRecord, 'judgment' | 'supersededBy'>, status: string): boolean {
  if (status === awaitingJudgmentOnPage) return request.judgment === undefined && request.supersededBy === undefined;
  if (status === supersededOnPage) return request.supersededBy !== undefined;
  return true;
}

// —— 客户索赔、客户通知、追偿（ports.ClaimItemPages 等三册）——

/** 三册的胶囊与可排维页面上都没有，只下推检索词；registry 是册子选择器。 */
export function claimsRegistryConditions<Registry extends ClaimsRecoveryRegistry>(
  registry: Registry,
  keyword: string,
): ClaimsRecoveryConditions<Registry> {
  return claimsRecoveryConditions(registry, { q: keyword });
}
