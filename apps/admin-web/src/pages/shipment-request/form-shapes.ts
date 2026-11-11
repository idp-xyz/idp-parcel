import { currentDisplayTimeZone, wallTimeToRfc3339 } from '../moment';

// 下单页共用的几条形状,小包下单页与平台货件页都用。只拦服务端必拒的形状:接单规则才说哪些格必填。

/** 十进制数的形状:点两侧都要有数字、不收指数与符号。与领域 decimalShape 同一条。 */
export function decimalShape(raw: string): boolean {
  return /^\d+(\.\d+)?$/.test(raw);
}

/** 镜像领域 MeasurementValue 的保真纪律:十进制且大于 0,"0"/"0.00" 不是正数。 */
export function positiveDecimal(raw: string): boolean {
  return decimalShape(raw) && /[1-9]/.test(raw);
}

/** 两端空白去掉后与别的格重复的下标(空格不算);客户包裹号与箱标号在一票里都不能重复。 */
export function duplicatedAt(values: string[]): number[] {
  const seen = new Map<string, number>();
  const repeated = new Set<number>();
  values.forEach((value, index) => {
    const text = value.trim();
    if (text === '') return;
    const first = seen.get(text);
    if (first === undefined) {
      seen.set(text, index);
    } else {
      repeated.add(first);
      repeated.add(index);
    }
  });
  return [...repeated].sort((left, right) => left - right);
}

/**
 * 「客户要求的开始时间」:datetime-local 的墙钟按页面显示时区折成 RFC 3339——服务端按 RFC 3339 读,原样送墙钟必被拒。
 * 留空交回 undefined(键缺席,没说就是没说);折不动交回 null,调用方报形状错。
 */
export function requestEffectiveAtOf(wallTime: string, timeZone: string = currentDisplayTimeZone()): string | undefined | null {
  if (wallTime.trim() === '') return undefined;
  return wallTimeToRfc3339(wallTime, timeZone);
}
