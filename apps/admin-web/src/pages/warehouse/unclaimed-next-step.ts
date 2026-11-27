// 一件待认领到货此刻该做的那一步，与它在进度条上的落点（纯 TypeScript，node:test 直接测）。列表每行最右与单据页头区的
// 主动作读同一份判断，两处不各写一套。

import type { DocumentTrack, TrackStep } from './document-track';

export type UnclaimedNextStep =
  /** 还没人认：由运营认领。 */
  | { kind: 'claim'; label: string }
  /** 认领期限走完仍没人认：按处置规则处置。 */
  | { kind: 'dispose'; label: string }
  /** 已认领，货在等入账：要去作业台做。 */
  | { kind: 'go'; label: string; moduleId: string };

interface ArrivalFacts {
  status: string;
  /** 到货发生的时刻：认领期限从这一刻起算。 */
  occurredAt: string;
  /** 已入账的收货事实：有它就说明这一认领已经被用掉了。 */
  postingReferences?: readonly string[];
}

/** 一个仓的处置规则里与「该不该处置」有关的那一格。 */
interface DisposalWindow {
  claimWindowSeconds: number;
}

// 与后端处置口判认领期限用的是同一把尺（`DisposalRuleContent.ClaimWindowElapsed`）：整秒上限是再大乘进时长就会回绕的那个数。
// 超出的、非正的期限一律当作没走完，不替规则猜一个意思。
const maxClaimWindowSeconds = 9_223_372_036;

const usable = (seconds: number): boolean => seconds > 0 && seconds <= maxClaimWindowSeconds;

/**
 * 认领期限是否走完：从到货发生的时刻起，按整秒比较，不看仓的业务时区与自然日。时刻认不出、发生在现在之后（两台机器的钟差）、
 * 期限不是正数，都答没走完——处置口也是这样判，页面不比它松。
 */
export function claimWindowElapsed(occurredAt: string, claimWindowSeconds: number, now: Date): boolean {
  if (!usable(claimWindowSeconds)) return false;
  const start = Date.parse(occurredAt);
  if (Number.isNaN(start)) return false;
  const elapsedMs = now.getTime() - start;
  if (elapsedMs < 0) return false;
  return Math.floor(elapsedMs / 1000) >= claimWindowSeconds;
}

/** 认领期限到哪一刻为止；时刻认不出或期限用不得，答 null。 */
export function claimDeadline(occurredAt: string, claimWindowSeconds: number): Date | null {
  const start = Date.parse(occurredAt);
  if (Number.isNaN(start) || !usable(claimWindowSeconds)) return null;
  return new Date(start + claimWindowSeconds * 1000);
}

const posted = (arrival: { postingReferences?: readonly string[] }): boolean => (arrival.postingReferences?.length ?? 0) > 0;

/**
 * 没有下一步的：已处置（不会再变），已认领且货已入账（认领被用掉了）。
 *
 * 待认领的下一步是认领；只有认领期限走完仍没人认、且这个仓登记了处置规则时，下一步才改成处置——没登记规则的仓处置会答未配置，
 * 不能把它当作下一步。已认领的货还在暂存，下一步是去作业台收货入账。
 */
export function unclaimedNextStep(arrival: ArrivalFacts, window: DisposalWindow | undefined, now: Date): UnclaimedNextStep | null {
  switch (arrival.status) {
    case 'UNCLAIMED':
      return window !== undefined && claimWindowElapsed(arrival.occurredAt, window.claimWindowSeconds, now)
        ? { kind: 'dispose', label: '处置' }
        : { kind: 'claim', label: '认领' };
    case 'CLAIMED':
      return posted(arrival) ? null : { kind: 'go', label: '去作业台入账', moduleId: 'warehouse-workstation' };
    default:
      return null;
  }
}

export const unclaimedSteps: readonly TrackStep[] = [
  { id: 'UNCLAIMED', label: '待认领' },
  { id: 'CLAIMED', label: '已认领' },
  { id: 'POSTED', label: '已入账' },
];

/**
 * 待认领与已认领而货未入账的，都卡在自己这一步、等人动手，用待办色标出。已处置是正常结束，不是失败，用中性的停止标记。
 * 认领被撤销后到货退回待认领，进度条也就退回第一步。
 */
export function unclaimedTrack(arrival: Pick<ArrivalFacts, 'status' | 'postingReferences'>): DocumentTrack {
  switch (arrival.status) {
    case 'UNCLAIMED':
      return { steps: unclaimedSteps, currentId: 'UNCLAIMED', currentLabel: '待认领' };
    case 'CLAIMED':
      return posted(arrival)
        ? { steps: unclaimedSteps, currentId: 'POSTED' }
        : { steps: unclaimedSteps, currentId: 'CLAIMED', currentLabel: '已认领，待入账' };
    case 'DISPOSED':
      return { steps: unclaimedSteps, currentId: 'UNCLAIMED', halt: { label: '已处置', tone: 'neutral' } };
    default:
      return { steps: unclaimedSteps, currentId: 'UNCLAIMED', currentLabel: arrival.status };
  }
}

/**
 * 认领表单从这件到货带出的格：到货号，以及现场登记时留的货主线索与预报线索。线索不是认定，带出来只是省得再敲，
 * 认领人可以改。预报线索填进入库单号那一格：是单据号就在提交前换成预报标识，是预报标识就照原样送出。
 */
export function claimPrefill(arrival: { arrivalId: string; ownerClue?: string; preAdviceClue?: string }): Record<string, string> {
  return { arrivalId: arrival.arrivalId, ownerAccount: arrival.ownerClue ?? '', preAdviceId: arrival.preAdviceClue ?? '' };
}
