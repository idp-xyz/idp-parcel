// 波次：状态的词、进度条上的落点与拣货进度（纯 TypeScript，node:test 直接测）。
//
// 波次的状态由读口答，后端只看待执行、拣满、少拣三种任务，与下面的拣货进度数的是同一批：有任务等着拣、没有一条拣满或少拣
// 是已放出；有任务等着拣、已有拣满或少拣是拣货中；没有任务等着拣、至少一条拣满或少拣是已完成；任务全被截单或撤单作废是已取消。
// 放出之后不往里加单，要加就再放一个波次。

import type { DocumentTrack, TrackStep } from './document-track';

/** 还有任务等着拣的两种：拣货工位只做这两种波次，列表上也只数这两种。 */
export const activeWaveStatuses = ['RELEASED', 'PICKING'] as const;
/** 不会再有任务要拣的两种，只增不减，不数。 */
export const settledWaveStatuses = ['COMPLETED', 'CANCELLED'] as const;

const waveStatusLabels: Record<string, string> = {
  RELEASED: '已放出',
  PICKING: '拣货中',
  COMPLETED: '已完成',
  CANCELLED: '已取消',
};

/** 读口没答出的陌生状态词原样示出，不替它猜。 */
export function waveStatusLabel(status: string): string {
  return waveStatusLabels[status] ?? status;
}

interface WavePicking {
  singleUnit?: boolean;
  batchPick?: boolean;
}

/**
 * 这个波次的货怎么拣，列表上的短词。合并拣货是放出时选的；单品单件波次不用选也是合着拿，但它不分拣——
 * 复核打包时扫货配单。两样都不是就是逐单拣。
 */
export function wavePickingLabel(wave: WavePicking): string {
  if (wave.batchPick) return '合并拣货';
  return wave.singleUnit ? '合着拿' : '逐单拣';
}

/** 同一件事在波次单据页上的整句。 */
export function wavePickingNote(wave: WavePicking): string {
  if (wave.batchPick) return '合并拣货：同库位同商品的货合在一起拿，拿回来再按出库单分拣';
  return wave.singleUnit ? '合着拿：单品单件，复核打包时扫货配单，不用分拣' : '逐单拣：货在拣货时就按出库单分开';
}

export const waveSteps: readonly TrackStep[] = [
  { id: 'RELEASED', label: '已放出' },
  { id: 'PICKING', label: '拣货中' },
  { id: 'COMPLETED', label: '已完成' },
];

/** 已取消是流程停下，不是走到了哪一步；它是截单的结果，不是出错，答中性。陌生状态词示在第一步上。 */
export function waveTrack(status: string): DocumentTrack {
  switch (status) {
    case 'RELEASED':
    case 'PICKING':
    case 'COMPLETED':
      return { steps: waveSteps, currentId: status };
    case 'CANCELLED':
      return { steps: waveSteps, currentId: 'RELEASED', halt: { label: '已取消', tone: 'neutral' } };
    default:
      return { steps: waveSteps, currentId: 'RELEASED', currentLabel: status };
  }
}

/**
 * 这个波次里现在还能撤出的出库单：没撤过，在这个波次里有任务等着拣，并且没有一条拣过（拣满或少拣）。
 * 拣过的要走截单，拣出来的货得回库。这里只按读到的任务判，够不够格以后端的答复为准——出库单另被截掉、
 * 或刚被别人拣了一条，后端会答状态不符。
 */
export function withdrawableRequests(wave: {
  requests: readonly string[];
  withdrawnRequests: readonly string[];
  tasks: ReadonlyArray<{ requestId: string; status: string }>;
}): string[] {
  const withdrawn = new Set(wave.withdrawnRequests);
  return wave.requests.filter((requestId) => {
    if (withdrawn.has(requestId)) return false;
    const own = wave.tasks.filter((task) => task.requestId === requestId);
    return own.some((task) => task.status === 'OPEN') && !own.some((task) => task.status === 'DONE' || task.status === 'SHORT');
  });
}

export interface WaveProgress {
  /** 算进度的任务数：待执行、拣满、少拣。作废与迟到的不算，那些货不再是这个波次要拣的。 */
  tasks: number;
  open: number;
  done: number;
  short: number;
  /** 被截单作废的任务（含作废之后才拣出来的迟到任务）。 */
  cancelled: number;
  /** 有了结果的任务占算进度的任务的百分比，取整；没有任务可算时是 0。 */
  percent: number;
}

export function waveProgress(tasks: ReadonlyArray<{ status: string }>): WaveProgress {
  let open = 0;
  let done = 0;
  let short = 0;
  let cancelled = 0;
  for (const task of tasks) {
    if (task.status === 'OPEN') open += 1;
    else if (task.status === 'DONE') done += 1;
    else if (task.status === 'SHORT') short += 1;
    else cancelled += 1;
  }
  const counted = open + done + short;
  return { tasks: counted, open, done, short, cancelled, percent: counted === 0 ? 0 : Math.floor(((done + short) / counted) * 100) };
}
