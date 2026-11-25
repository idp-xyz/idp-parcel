// 「从某一刻到现在过了多久」的短写法（纯 TypeScript，node:test 直接测）。队列上只要一眼看出哪一单放得最久，
// 所以只取最大的一个单位，不写「2 小时 13 分」。

const minuteMs = 60_000;
const hourMs = 60 * minuteMs;
const dayMs = 24 * hourMs;

/** 时刻认不出、或在现在之后（两台机器的钟差），都答空串：不知道就不写。 */
export function elapsedLabel(since: string | undefined, now: Date): string {
  if (!since) return '';
  const start = Date.parse(since);
  if (Number.isNaN(start)) return '';
  const elapsed = now.getTime() - start;
  if (elapsed < 0) return '';
  if (elapsed < minuteMs) return '不到 1 分钟';
  if (elapsed < hourMs) return `${Math.floor(elapsed / minuteMs)} 分钟`;
  if (elapsed < dayMs) return `${Math.floor(elapsed / hourMs)} 小时`;
  return `${Math.floor(elapsed / dayMs)} 天`;
}

/** 预计时刻已经过去多久。还没到、或认不出，答空串。 */
export function overdueLabel(expectedAt: string | undefined, now: Date): string {
  return elapsedLabel(expectedAt, now);
}

/**
 * 一段规定时长（认领期限这类租户登记的秒数）的写法：能整除的取最大的单位，不能整除的退到更小的单位，不四舍五入——
 * 规则里写的是多少，页面上就写多少。不是正数答空串。
 */
export function durationLabel(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '';
  for (const [size, name] of [[dayMs / 1000, '天'], [hourMs / 1000, '小时'], [minuteMs / 1000, '分钟']] as const) {
    if (seconds % size === 0) return `${seconds / size} ${name}`;
  }
  return `${seconds} 秒`;
}
