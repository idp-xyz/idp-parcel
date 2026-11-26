// 波次：状态的词、进度条上的落点与拣货进度（纯 TypeScript，node:test 直接测）。
//
// 波次的状态由读口答，是后端从它的拣货任务算出来的：还有任务等着拣、一条都没拣过是已放出；还有任务等着拣、已经拣过是拣货中；
// 没有任务等着拣、至少一条有了结果（拣满或少拣）是已完成；任务全被截单作废是已取消。放出之后不往里加单，要加就再放一个波次。

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
