// 出库单状态在步进条上的落点（纯 TypeScript，node:test 直接测）。

export interface TrackStep {
  id: string;
  label: string;
}

export interface OutboundTrack {
  steps: readonly TrackStep[];
  currentId: string;
  /** 卡在当前这一步、还没做完时，用状态本身的词代替这一步的名字。 */
  currentLabel?: string;
  /** 流程已经停下：失败类答红，拆成新单这类正常结束答中性。 */
  halt?: { label: string; tone: 'danger' | 'neutral' };
}

export const selfOutboundSteps: readonly TrackStep[] = [
  { id: 'RESERVED', label: '已预占' },
  { id: 'ALLOCATED', label: '已放波次' },
  { id: 'PICKED', label: '已拣货' },
  { id: 'CHECKED', label: '已复核' },
  { id: 'PACKED', label: '已打包' },
  { id: 'SHIPPED', label: '已出库' },
];

// 代理仓的单不走波次、拣货与复核；打包与出库是代理仓回执落下的同名状态。
export const agentOutboundSteps: readonly TrackStep[] = [
  { id: 'HANDED_OFF', label: '待转出' },
  { id: 'FORWARDED', label: '已转出' },
  { id: 'PACKED', label: '已打包' },
  { id: 'SHIPPED', label: '已出库' },
];

const halts: Record<string, NonNullable<OutboundTrack['halt']>> = {
  CANCELLED: { label: '已取消', tone: 'danger' },
  REJECTED: { label: '已拒单', tone: 'danger' },
  PARTIALLY_CANCELLED: { label: '部分取消', tone: 'danger' },
  AWAITING_RETURN: { label: '已中止，待回库', tone: 'danger' },
  SPLIT: { label: '已拆成新单', tone: 'neutral' },
};

const stuckAtReservation: Record<string, string> = {
  PARTIALLY_RESERVED: '部分预占',
  PENDING_POLICY: '待定缺货策略',
};

export function outboundTrack(status: string, agentWarehouse: boolean): OutboundTrack {
  const steps = agentWarehouse || status === 'HANDED_OFF' || status === 'FORWARDED' ? agentOutboundSteps : selfOutboundSteps;
  const first = steps[0].id;
  const halt = halts[status];
  if (halt) return { steps, currentId: first, halt };
  const stuck = stuckAtReservation[status];
  if (stuck) return { steps: selfOutboundSteps, currentId: 'RESERVED', currentLabel: stuck };
  if (steps.some((step) => step.id === status)) return { steps, currentId: status };
  return { steps, currentId: first, currentLabel: status };
}
