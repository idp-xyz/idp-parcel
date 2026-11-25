// 拣货工位的待拣队列：从波次读口读出本仓还没拣的拣货任务，排成作业的次序（纯 TypeScript，node:test 直接测）。
//
// 读口只有波次列表，一个波次带着它的全部任务。波次读口声明的筛选维只有仓与是否预取，没有「任务还没拣」这一维，所以只能把波次
// 翻出来、在前端挑任务状态为待执行的。放出去的波次只增不减，翻到底会越来越慢，所以从新到旧翻（读口缺省序是放出时刻倒序）、
// 设页数上限，翻不完时在结果里如实带出「只看了多少」，由界面照实说，不假装看全了。
//
// 指名了波次（地址里的 `?wave=`）时用检索词只问那一个：检索按包含匹配，答回来的里面再认出号完全相同的；认出就不再翻页。

import type { ApiResult } from '../../catalogue-api';
import type { CatalogueConditions } from '../../catalogue-query';
import type { WaveListBody, WaveRecord } from '../api';

/** 一格 = 一条拣货任务：去哪个库位、拣哪个商品、拣几件。 */
export interface PickCell {
  taskId: string;
  waveId: string;
  requestId: string;
  locationCode: string;
  goodsCode: string;
  unit: string;
  stockState: string;
  /** 应拣数量。 */
  quantity: number;
}

/** 一次最多翻几页（每页读口最多 200 个波次）。 */
export const MAX_WAVE_PAGES = 5;

// 库位码里的数字按数值比：A-1-2 排在 A-1-10 前面，拣货的人顺着库位走。比不出先后的再按任务号定，次序才稳定。
const collator = new Intl.Collator('en', { numeric: true });

export function compareLocationCodes(left: string, right: string): number {
  return collator.compare(left, right);
}

function compareCells(left: PickCell, right: PickCell): number {
  const byLocation = compareLocationCodes(left.locationCode, right.locationCode);
  if (byLocation !== 0) return byLocation;
  if (left.taskId === right.taskId) return 0;
  return left.taskId < right.taskId ? -1 : 1;
}

export interface PickScope {
  warehouse: string;
  /** 只做这个波次；null 是本仓全部波次。 */
  wave: string | null;
}

/**
 * 从读到的波次挑出要拣的格：本仓的、（指名了波次时）那个波次的、任务状态是待执行的；按库位码排。
 * 数量不是正整数的任务不收：后端不收小于 1 的实拣数量，放进来只会卡住这一格。
 */
export function cellsOfWaves(waves: readonly WaveRecord[], scope: PickScope): PickCell[] {
  const cells = new Map<string, PickCell>();
  for (const wave of waves) {
    if (wave.warehouseCode !== scope.warehouse) continue;
    if (scope.wave !== null && wave.waveId !== scope.wave) continue;
    for (const task of wave.tasks) {
      if (task.status !== 'OPEN') continue;
      if (!Number.isInteger(task.quantity) || task.quantity < 1) continue;
      cells.set(task.taskId, {
        taskId: task.taskId,
        waveId: wave.waveId,
        requestId: task.requestId,
        locationCode: task.locationCode,
        goodsCode: task.goodsCode,
        unit: task.unit,
        stockState: task.stockState,
        quantity: task.quantity,
      });
    }
  }
  return [...cells.values()].sort(compareCells);
}

/** 「全部波次」在下拉里的值：下拉不收空串；波次号是放波次时系统给的号，不会是它。 */
export const allWavesChoice = '__all__';

export interface WaveChoice {
  value: string;
  label: string;
}

/**
 * 「换波次」下拉的选项：全部波次在最前，后面是读到的还有格没拣的波次，次序照队列里给的（新的在前）。地址里指名的波次不在其中时
 * 也列出来，下拉上写的才是此刻正在做的这一个。
 */
export function waveChoices(current: string | null, waves: ReadonlyArray<{ waveId: string; open: number }>): WaveChoice[] {
  const named = current !== null && !waves.some((entry) => entry.waveId === current) ? [{ waveId: current, open: 0 }, ...waves] : waves;
  return [
    { value: allWavesChoice, label: '全部波次' },
    ...named.map((entry) => ({ value: entry.waveId, label: entry.open > 0 ? `${entry.waveId}（还有 ${entry.open} 格）` : entry.waveId })),
  ];
}

/** 下拉里选中的值换回波次号；「全部波次」是 null。 */
export function waveOfChoice(value: string): string | null {
  return value === allWavesChoice ? null : value;
}

export interface PickQueue {
  cells: PickCell[];
  /** 这一趟读到了几个波次。 */
  wavesRead: number;
  pagesRead: number;
  /** 翻到页数上限，读口还有下一页：更早的波次没有看，里面若还有没拣的格，这里看不到。 */
  truncated: boolean;
  /** 指名了波次，而读下来没有找到它（或它不属于本仓）。 */
  waveMissing: boolean;
  /** 读到的波次里各还有几格没拣（只列还有的），新的在前；供「换波次」选。 */
  waves: Array<{ waveId: string; open: number }>;
}

export type PickQueueAnswer =
  | { kind: 'queue'; queue: PickQueue }
  | { kind: 'failed'; failure: Exclude<ApiResult<WaveListBody>, { kind: 'outcome' }> };

export interface PickQueueInput extends PickScope {
  list: (conditions: CatalogueConditions, after: string | null) => Promise<ApiResult<WaveListBody>>;
  maxPages?: number;
}

export async function loadPickQueue(input: PickQueueInput): Promise<PickQueueAnswer> {
  const { warehouse, wave, list } = input;
  const maxPages = input.maxPages ?? MAX_WAVE_PAGES;
  const conditions: CatalogueConditions = { filters: { warehouseCode: [warehouse] }, ...(wave === null ? {} : { q: wave }) };
  const waves: WaveRecord[] = [];
  let after: string | null = null;
  let pagesRead = 0;
  do {
    const answer: ApiResult<WaveListBody> = await list(conditions, after);
    if (answer.kind !== 'outcome') return { kind: 'failed', failure: answer };
    pagesRead += 1;
    waves.push(...answer.body.waves);
    after = answer.body.page.next;
    if (wave !== null && waves.some((candidate) => candidate.waveId === wave)) after = null;
  } while (after !== null && pagesRead < maxPages);

  const scoped = waves.filter((candidate) => candidate.warehouseCode === warehouse && (wave === null || candidate.waveId === wave));
  return {
    kind: 'queue',
    queue: {
      cells: cellsOfWaves(waves, { warehouse, wave }),
      wavesRead: waves.length,
      pagesRead,
      truncated: after !== null,
      waveMissing: wave !== null && scoped.length === 0,
      waves: scoped
        .map((candidate) => ({ waveId: candidate.waveId, open: candidate.tasks.filter((task) => task.status === 'OPEN').length }))
        .filter((entry) => entry.open > 0),
    },
  };
}
