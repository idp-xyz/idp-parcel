// 换工位、退出作业台要不要先问。只拦两种：这一格已经数了还没送出，或这一笔还没送到
// （事实号只在这一屏上，离开就对不上再送一次）。别的时候离开不拦。

const held = (phase: string): boolean =>
  phase === 'submitting' ||
  phase === 'unsent' ||
  phase === 'check-submitting' ||
  phase === 'check-unsent' ||
  phase === 'pack-submitting' ||
  phase === 'pack-unsent' ||
  phase === 'pack-pending';

export function pendingLeave(phase: string, counted: number): string | null {
  if (held(phase)) return '这一笔还没送到。离开会丢掉这一笔的作业事实号，回来对不上。';
  if (counted > 0) return `已经数了 ${counted} 件，还没送出。离开这些数就没了。`;
  return null;
}
