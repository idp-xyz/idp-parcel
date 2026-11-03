import { useEffect, useState } from 'react';

// 加载占位（骨架、「正在加载…」）晚一点再露出。答案常在几十毫秒内就到：占位一露就换成内容，眼睛看到的是「空 → 骨架 →
// 内容」两次跳，比直接「空 → 内容」更显慢（用户 2026-11-03 问「骨架屏对这个有影响吗」，实测公司主体首开骨架只露约
// 35ms 就换成表）。等过这一段还没答到，才露出占位告诉人在读。
// 占位在等的这段里仍占着位置（只是看不见），露出与否不改版面，答案到了也不塌一下。

/** 占位露出前的等待。两百毫秒内到的答案看着就是「直接出来」，再久才需要告诉人「在读」。 */
export const loadingRevealDelayMs = 200;

/** 挂上之后过了 `delayMs` 才返回 true；占位据此决定看得见与否。 */
export function useDelayedReveal(delayMs: number = loadingRevealDelayMs): boolean {
  const [revealed, setRevealed] = useState(delayMs <= 0);
  useEffect(() => {
    if (revealed) return;
    const timer = window.setTimeout(() => setRevealed(true), delayMs);
    return () => window.clearTimeout(timer);
  }, [delayMs, revealed]);
  return revealed;
}

/** 等待期里给占位外层加的类：看不见、读屏也不念，但位置照占。 */
export function revealClass(revealed: boolean): string {
  return revealed ? '' : 'invisible';
}
