// 运输班次、网点作业与外部轨迹时间确认几页的列表条件（票 catalogue-read-pagination-rollout/14）：检索框下推成 `q`，状态胶囊能下推的
// 下推成读口点名的筛选维；下推不了的——读口派生、不是筛选维的状态词——留成只筛本页的胶囊，页面在胶囊旁字面写明。
// 单独成模块、不依赖 React：条件拼没拼对、页内筛没筛对，拿 node:test 就能钉住。

import type { WorkbenchChip } from '../../templates';
import type { CatalogueConditions } from '../catalogue-query';
import { matchesSearch } from '../party/list-order';
import { effectiveBasisLabels } from './effective-time-judgment';
import {
  consolidationPhaseLabels,
  handoverVerdictLabels,
  masterDocumentStandingLabels,
  receptionKindLabels,
} from './presentation';
import type { NodeOperationsRegistry, TransportFulfillmentRegistry } from './records-api';

export const allStatuses = 'ALL';

/** 一册的状态胶囊下推到读口的哪一维；labels 的键即该维词表里的词——读口不认的词整问答 400，所以胶囊只出这几格。 */
export interface PushedStatus {
  dimension: string;
  labels: Readonly<Record<string, string>>;
}

/** 运输履约各册里状态胶囊下推得了的两册：交接结果按 verdict、总单按 standing（ports.TransportHandoverPages、CarrierMasterDocumentPages）。 */
export const transportPushedStatus: Partial<Record<TransportFulfillmentRegistry, PushedStatus>> = {
  'transport-handover': { dimension: 'verdict', labels: handoverVerdictLabels },
  'carrier-master-document': { dimension: 'standing', labels: masterDocumentStandingLabels },
};

/** 节点作业各册：收货记录按 kind、集包按 phase（ports.ReceptionPages、ConsolidationUnitPages）。 */
export const nodePushedStatus: Partial<Record<NodeOperationsRegistry, PushedStatus>> = {
  reception: { dimension: 'kind', labels: receptionKindLabels },
  'consolidation-unit': { dimension: 'phase', labels: consolidationPhaseLabels },
};

/** 外部轨迹的确认方式按 effectiveBasis（ports.ExternalTrackingFactPages）。 */
export const effectiveBasisStatus: PushedStatus = { dimension: 'effectiveBasis', labels: effectiveBasisLabels };

/** 检索词、下推的状态胶囊与本册别的筛选维拼成一问的条件；选择器由各读口的 *Conditions 加上。 */
export function reviewListConditions(
  keyword: string,
  pushed: PushedStatus | undefined,
  status: string,
  filters: Readonly<Record<string, readonly string[]>> = {},
): Omit<CatalogueConditions, 'selectors'> {
  const dimensions: Record<string, readonly string[]> = { ...filters };
  if (pushed && status !== allStatuses) dimensions[pushed.dimension] = [status];
  return { q: keyword, filters: dimensions };
}

/** 下推的胶囊：「全部」加词表各词。不带计数——读口分页时到手的只是一页，页上的数不是总数。 */
export function pushedStatusChips(pushed: PushedStatus, status: string, onPick: (status: string) => void): WorkbenchChip[] {
  return [
    { id: allStatuses, label: '全部', active: status === allStatuses, onClick: () => onPick(allStatuses) },
    ...Object.entries(pushed.labels).map(([code, label]) => ({
      id: code,
      label,
      active: status === code,
      onClick: () => onPick(status === code ? allStatuses : code),
    })),
  ];
}

/**
 * 只筛本页的胶囊：词取这一页上出现过的，选中的那一格翻到没有它的页也留着、好点掉。同样不带计数。字面的「只筛本页」由页面摆在胶囊旁
 * （visibility/page-only 的 PageOnlyNote），这里不往每格里塞。
 */
export function pageStatusChips(labelsOnPage: readonly string[], status: string, onPick: (status: string) => void): WorkbenchChip[] {
  const labels = [...new Set(labelsOnPage.filter((label) => label !== ''))].sort((a, b) => a.localeCompare(b, 'zh-Hans-CN'));
  if (status !== allStatuses && !labels.includes(status)) labels.push(status);
  return [
    { id: allStatuses, label: '全部', active: status === allStatuses, onClick: () => onPick(allStatuses) },
    ...labels.map((label) => ({
      id: label,
      label,
      active: status === label,
      onClick: () => onPick(status === label ? allStatuses : label),
    })),
  ];
}

/**
 * 到手的这一页上再筛：页内状态胶囊（statusOf 不给即这一册没有）与页内筛选框。筛选框搜行上已转写好的全部格——名称、中文词、时刻都在
 * 里面，正是读口检索不覆盖的那些。
 */
export function rowsOnPage<Row extends { values: Readonly<Record<string, string>> }>(
  rows: readonly Row[],
  refine: string,
  status: string = allStatuses,
  statusOf?: (row: Row) => string | undefined,
): Row[] {
  return rows.filter(
    (row) =>
      (status === allStatuses || statusOf === undefined || statusOf(row) === status) &&
      matchesSearch(refine, Object.values(row.values)),
  );
}
