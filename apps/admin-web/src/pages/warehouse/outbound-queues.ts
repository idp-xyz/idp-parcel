// 出库单列表按哪一队看（纯 TypeScript，node:test 直接测）。标题行上的队列数字与工具条里的状态下拉读同一份。
//
// 队不等于状态：卡在预占上的两种状态（待定缺货策略、部分预占）对操作者是同一件事——等人拿主意——合成「待处理」一队；
// 具体是哪一种，行上的徽章写着。其余各队一队一个状态。

/** 不按队筛。 */
export const allQueues = 'ALL';

/** 卡在预占上、等人拿主意的那一队。 */
export const attentionQueue = 'ATTENTION';

const attentionStatuses = ['PENDING_POLICY', 'PARTIALLY_RESERVED'] as const;

/**
 * 还在往下走的各队，照一张出库单往下走的先后；待处理排最前，它最需要人看。这几队的数量摆在标题行上。
 */
export const workingQueues = [attentionQueue, 'RESERVED', 'ALLOCATED', 'PICKED', 'CHECKED', 'PACKED'] as const;

/** 不会再往下走的两队：只增不减，数出来的不是待办，所以只在状态下拉里、不摆数量。 */
export const settledQueues = ['SHIPPED', 'CANCELLED'] as const;

/** 要向读口各问一次数量的状态：各队背后的全部状态。 */
export const countedStatuses: readonly string[] = workingQueues.flatMap((queue) => queueStatusesOf(queue));

/** 一队背后的状态；不按队筛时是空的。 */
export function queueStatusesOf(queue: string): string[] {
  if (queue === allQueues) return [];
  if (queue === attentionQueue) return [...attentionStatuses];
  return [queue];
}

/** 是不是还在往下走的一队：这样的队里等得最久的排最前。 */
export function isWorkingQueue(queue: string): boolean {
  return (workingQueues as readonly string[]).includes(queue);
}

/**
 * 一队有几张：它背后各状态的数量相加。有一个状态没答出来（读口没给总数、那一问没成）就答 null——少算一截的数不如不写。
 */
export function queueCount(counts: Readonly<Record<string, number | null | undefined>>, queue: string): number | null {
  let total = 0;
  for (const status of queueStatusesOf(queue)) {
    const count = counts[status];
    if (typeof count !== 'number') return null;
    total += count;
  }
  return total;
}

/**
 * 把记下来的那一队认成现在的队：标签的视图状态里可能还记着合并之前单独的一种卡住状态，照「待处理」看；认不出的照不筛看。
 */
export function knownQueue(stored: string): string {
  if ((attentionStatuses as readonly string[]).includes(stored)) return attentionQueue;
  if (stored === allQueues || isWorkingQueue(stored) || (settledQueues as readonly string[]).includes(stored)) return stored;
  return allQueues;
}
