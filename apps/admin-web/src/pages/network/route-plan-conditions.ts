// 路线规划各册的列表条件（票 catalogue-read-pagination-rollout/14；契约 ADR-0144 决定三、四）：检索框下推成 `q`，状态胶囊下推成
// 各册点名的那一维，表头的先后下推成 `sort`。各册点名了哪些维只在 internal/networkrouting/ports 的 catalogue_pages.go 一处，这里
// 照它取名、不另造——发了没点名的键或词表外的词，整问答 400。
// 单独成模块、不依赖 React：条件拼没拼对拿 node:test 就能钉住；页面只负责摆。

import type { WorkbenchChip, WorkbenchSort } from '../../templates';
import type { CatalogueConditions } from '../catalogue-query';
import { labelFor } from '../../vocabulary/status';
import { labelOf } from './presentation';
import { initialRouteTieStateLabels } from './initial-route-tie';
import { rerouteSuggestionStateLabels } from './reroute-decision';

/** 路线规划页上切的册：前两册同挂 GET /route-plans（register 选择器），后两册各有自己的读口。 */
export type RouteRegister = 'initial-route' | 'reassessment' | 'reroute-suggestion' | 'initial-route-tie';

/** 状态胶囊「全部」的 id：不带状态一维。 */
export const allStatuses = 'ALL';

/** 一册的状态胶囊下推到读口的哪一维；labels 的键即该维词表里的词，胶囊只出这几格。 */
export interface RouteStatusDimension {
  readonly dimension: string;
  readonly labels: Readonly<Record<string, string>>;
}

/** 本册词表的一格去掉破折号之后的说明：胶囊与行上的状态胶囊用同一个词，说明进详情。 */
export function stateWordOf(labels: Readonly<Record<string, string>>, code: string): string {
  return labelOf(labels as Record<string, string>, code).split('——')[0];
}

// 结论码与行上的结论胶囊同一个词（共享词表的 codeStatus 取的也是它）。
function conclusionWords(codes: readonly string[]): Record<string, string> {
  return Object.fromEntries(codes.map((code) => [code, labelFor(code)]));
}

function stateWords(labels: Readonly<Record<string, string>>): Record<string, string> {
  return Object.fromEntries(Object.keys(labels).map((code) => [code, stateWordOf(labels, code)]));
}

/** 初始路由与复核按 conclusion，改路建议与成本并列按 state（ports 的 InitialRoutePages、RouteReassessmentPages 等）。 */
export const routeStatusDimensions: Readonly<Record<RouteRegister, RouteStatusDimension>> = {
  'initial-route': { dimension: 'conclusion', labels: conclusionWords(['ROUTE_FORMED', 'NO_CURRENT_ROUTE']) },
  reassessment: {
    dimension: 'conclusion',
    labels: conclusionWords(['STILL_APPLICABLE', 'PLAN_LAPSED', 'FIRST_PLAN_FORMED', 'REROUTED']),
  },
  'reroute-suggestion': { dimension: 'state', labels: stateWords(rerouteSuggestionStateLabels) },
  'initial-route-tie': { dimension: 'state', labels: stateWords(initialRouteTieStateLabels) },
};

/**
 * 表头那一列与它下推到的可排维。只有列上显的正是读口可排的那一维才给（初始路由的落册时刻、成本并列的停住时刻）；复核与改路建议
 * 列上显的是复核、建议形成的时刻，读口按落册时刻排、不按它们，那两册的列不给排，次序随读口。
 */
export interface RouteSortColumn {
  /** 列的 sortKey。 */
  readonly key: string;
  /** 读口的可排维名。 */
  readonly field: string;
}

/** 缺省序：这一维倒序，最新的在前——各册读口的缺省也是它。 */
export function newestFirst(column: RouteSortColumn): WorkbenchSort {
  return { key: column.key, dir: -1 };
}

/** 表头的先后 → 读口的 `sort`。缺省序不写进查询串（不带条件的第一页因此是裸路径），正序写维名。 */
export function routeSortParam(sort: WorkbenchSort, column: RouteSortColumn): string | undefined {
  return sort.key === column.key && sort.dir === 1 ? column.field : undefined;
}

/** 检索词、下推的状态胶囊与表头先后拼成一问的条件；选择器由读口函数加上。 */
export function routeListConditions(
  keyword: string,
  status: RouteStatusDimension,
  picked: string,
  sort?: string,
): Omit<CatalogueConditions, 'selectors'> {
  return {
    q: keyword,
    sort,
    filters: picked === allStatuses ? {} : { [status.dimension]: [picked] },
  };
}

/** 下推的胶囊：「全部」加词表各词，再点一下回到全部。不带计数——读口分页时到手的只是一页，页上的数不是总数。 */
export function routeStatusChips(status: RouteStatusDimension, picked: string, onPick: (code: string) => void): WorkbenchChip[] {
  return [
    { id: allStatuses, label: '全部', active: picked === allStatuses, onClick: () => onPick(allStatuses) },
    ...Object.entries(status.labels).map(([code, label]) => ({
      id: code,
      label,
      active: picked === code,
      onClick: () => onPick(picked === code ? allStatuses : code),
    })),
  ];
}
