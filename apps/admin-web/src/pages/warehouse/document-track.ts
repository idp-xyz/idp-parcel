// 单据在进度条上的落点（纯 TypeScript，node:test 直接测）。出库单、建账导入与待认领到货各有自己的步子，停下的方式也不同，
// 但落点的形状是同一个：走到哪一步、卡在这一步时叫什么、流程停了没有。

export interface TrackStep {
  id: string;
  label: string;
}

export interface DocumentTrack {
  steps: readonly TrackStep[];
  currentId: string;
  /** 卡在当前这一步、还没做完时，用状态本身的词代替这一步的名字。 */
  currentLabel?: string;
  /** 流程已经停下：失败类答红，拆成新单、已处置这类正常结束答中性。 */
  halt?: { label: string; tone: 'danger' | 'neutral' };
}
