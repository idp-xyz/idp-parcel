// 分拣工位读的两样东西（纯 TypeScript，node:test 直接测，读口作参数传入）：一个波次此刻的分拣情况，和本仓要分拣的波次。
//
// 分拣情况按波次号读 `GET /warehouse-fulfillment-wave-sort`：后端只答还没打包、没取消的出库单。不属于这台设备绑的仓的波次
// 不做——投放口会拒（作业范围对不上），不如在这里就说清。
// 要分拣的波次从波次列表按 `batchPick` 筛，只读第一页（放出时刻从新到旧）；更早的输波次号进。

import type { ApiResult } from '../../catalogue-api';
import type { CatalogueConditions } from '../../catalogue-query';
import type { SortPlanBody, WaveListBody, WaveRecord } from '../api';
import { commonHeaderFields, type WorkFactSpec } from '../work-fact';
import type { SortOrder } from './sort-flow';
import { describeReadFailure } from './work-fact-submit';

/**
 * 分拣投放这一种作业事实的定义。它不在旧作业台的那张表里：旧作业台按那张表摆手填的表单，而分拣只在这个工位做——
 * 归哪张单是工位照分拣情况判的，手填不出来。
 */
export const sortFactSpec: WorkFactSpec = {
  id: 'sort-put',
  category: 'outbound',
  label: '分拣投放',
  title: '把合并拣货拿回来的货分进出库单的周转箱',
  endpoint: '/warehouse-fulfillment-sort-puts',
  usesDeviceSequence: false,
  usesOccurredAt: false,
  snapshotHint: '波次、出库请求、商品、件数与周转箱。',
  form: {
    fields: [
      ...commonHeaderFields(false, false),
      { path: 'waveId', label: '波次号', kind: 'text', required: true },
      { path: 'requestId', label: '出库请求号', kind: 'text', required: true },
      { path: 'goodsCode', label: '商品编码', kind: 'text', required: true },
      { path: 'quantity', label: '件数', kind: 'integer', required: true },
      { path: 'toteCode', label: '周转箱号', kind: 'text', required: true },
    ],
  },
};

export type SortPlanAnswer =
  | { kind: 'plan'; orders: SortOrder[] }
  /** 这个波次不用分拣、没有这个波次、不属于本仓：说明原因，不进分拣。 */
  | { kind: 'refused'; reason: string }
  | { kind: 'failed'; reason: string };

export function sortPlanOf(answer: ApiResult<SortPlanBody>, waveId: string, warehouse: string): SortPlanAnswer {
  if (answer.kind !== 'outcome') return { kind: 'failed', reason: describeReadFailure(answer) };
  const body = answer.body;
  if (body.outcome === 'WAVE_NOT_FOUND') return { kind: 'refused', reason: `没有波次 ${waveId}` };
  if (body.warehouseCode !== undefined && body.warehouseCode !== warehouse) {
    return { kind: 'refused', reason: `波次 ${waveId} 是仓 ${body.warehouseCode} 的，这台设备绑的是仓 ${warehouse}` };
  }
  if (body.outcome === 'SORT_NOT_REQUIRED') {
    return { kind: 'refused', reason: `波次 ${waveId} 不用分拣：它是逐单拣的，或者是单品单件波次（复核打包时扫货配单）` };
  }
  if (body.outcome !== 'SORT_PLAN_FOUND') return { kind: 'failed', reason: `读口答了一个这里不认识的结果（${body.outcome}）` };
  return {
    kind: 'plan',
    orders: (body.requests ?? []).map((request) => ({
      requestId: request.requestId,
      documentNumber: request.documentNumber ?? '',
      ownerAccount: request.ownerAccount,
      totes: request.totes ?? [],
      lines: request.lines ?? [],
    })),
  };
}

export interface SortWaveChoice {
  waveId: string;
  status: string;
  releasedAt: string;
  requests: number;
}

export type SortWavesAnswer = { kind: 'waves'; waves: SortWaveChoice[]; more: boolean } | { kind: 'failed'; reason: string };

/** 本仓合并拣货的波次，新的在前。已取消的不列：它的任务都作废了，没有货要分。 */
export async function loadSortWaves(
  warehouse: string,
  list: (conditions: CatalogueConditions, after: string | null) => Promise<ApiResult<WaveListBody>>,
): Promise<SortWavesAnswer> {
  const answer = await list({ filters: { warehouseCode: [warehouse], batchPick: ['true'], status: ['RELEASED', 'PICKING', 'COMPLETED'] } }, null);
  if (answer.kind !== 'outcome') return { kind: 'failed', reason: describeReadFailure(answer) };
  return {
    kind: 'waves',
    waves: answer.body.waves.map((wave: WaveRecord) => ({
      waveId: wave.waveId,
      status: wave.status,
      releasedAt: wave.releasedAt,
      requests: wave.requests.length - wave.withdrawnRequests.length,
    })),
    more: answer.body.page.next !== null,
  };
}
