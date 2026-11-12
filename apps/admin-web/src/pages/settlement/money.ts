// 金额的屏上写法。放在纯 .ts 里是为了 node:test 能直接钉：测试链按 CommonJS 发射，引 tsx 就得把 React 带进来。

/**
 * 金额串只加千分位，不换单位：传输用的是币种最小单位的十进制计数串，小数位属币种语义，页面不代判精度。
 * 非纯数字的串原样回显。
 */
export function formatMoney(amount: string | undefined, currency: string): string {
  if (amount === undefined) return '';
  const grouped = /^-?\d+$/.test(amount) ? amount.replace(/\B(?=(\d{3})+(?!\d))/g, ',') : amount;
  return currency ? `${grouped} ${currency}` : grouped;
}

/** 最小币单位的整数串是不是正数。0、全零串与非整数串都不算——「已扣多少」这种格靠它分辨「没扣」与「扣了一部分」。 */
export function isPositiveMinorAmount(amount: string | undefined): boolean {
  return amount !== undefined && /^\d+$/.test(amount) && /[1-9]/.test(amount);
}
