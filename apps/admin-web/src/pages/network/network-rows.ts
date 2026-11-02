// 线路与运力区几页共用的纯函数：版本此刻在不在期、时区的中文叫法、版本号的屏上写法、路线的一句话。
// 只引词典与读口类型，不引 React，node:test 里直接钉。
//
// 「在不在期」是管理台按此刻对一行版本的生效区间做的呈现判断，不是选版：同一对象的几版照旧逐行在列，页面不替读者挑
// 「当前那一版」——路由该用哪一版是路由判断的事（network-routing CONTEXT 的选版规则归它）。

import type { PlannedLegRecord } from './api';

/** 一行版本按此刻落在区间的哪一边；码进 vocabulary/status.ts 译中文。 */
export type ValidityCode = 'IN_FORCE' | 'NOT_YET_EFFECTIVE' | 'EXPIRED';

/** 半开区间 [from, to)：起点缺席或解析不了答 null（页面显「—」，不猜）。 */
export function validityOf(from: string | undefined, to: string | undefined, now: Date): ValidityCode | null {
  if (!from) return null;
  const start = Date.parse(from);
  if (Number.isNaN(start)) return null;
  if (now.getTime() < start) return 'NOT_YET_EFFECTIVE';
  if (to) {
    const end = Date.parse(to);
    if (!Number.isNaN(end) && now.getTime() >= end) return 'EXPIRED';
  }
  return 'IN_FORCE';
}

/** 停运与恢复一类调整：未开始 / 生效中 / 已解除。解除时刻缺席即仍在生效。 */
export type AdjustmentWindowCode = 'NOT_YET_EFFECTIVE' | 'IN_FORCE' | 'LIFTED';

export function adjustmentWindowOf(effectiveAt: string | undefined, liftedAt: string | undefined, now: Date): AdjustmentWindowCode | null {
  const code = validityOf(effectiveAt, liftedAt, now);
  if (code === null) return null;
  return code === 'EXPIRED' ? 'LIFTED' : code;
}

/**
 * IANA 时区名的中文叫法（「Asia/Shanghai」→「中国标准时间」），取浏览器 Intl 的本地化名；运行环境给不出就原样交回，
 * 不自建城市表——时区是租户登记的取值，产品不替它翻第二份。
 */
export function timeZoneLabel(zone: string | undefined): string {
  if (!zone) return '—';
  try {
    const parts = new Intl.DateTimeFormat('zh-CN', { timeZone: zone, timeZoneName: 'long' }).formatToParts(new Date(0));
    const name = parts.find((part) => part.type === 'timeZoneName')?.value;
    return name && name !== zone ? name : zone;
  } catch {
    return zone;
  }
}

/** 版本号的屏上写法：「第 2 版」。版本号由登记方给，进技术详情；主列只在同一对象有多版时才需要它。 */
export function versionWord(version: number | string | undefined): string {
  return version === undefined || version === '' ? '' : `第 ${version} 版`;
}

/** 路由计划里的一段：形状归读口类型（api.ts 的 PlannedLegRecord），这里只取个短名给路线一句话用。 */
export type RouteLeg = PlannedLegRecord;

/**
 * 一条路线的一句话：「SYN-NODE-SHA-HUB → SYN-NODE-SZX-GATE → … → 末端派送」。段按计划给的顺序首尾相接；派送段的终点
 * 是服务区域引用，不是网点，所以说成「末端派送」而不露区域引用（引用在技术详情里）。
 */
export function routeSummaryOf(legs: readonly RouteLeg[] | undefined, label: (node: string) => string = (node) => node): string {
  if (!legs || legs.length === 0) return '';
  // 先按编码首尾相接，再逐站换成屏上写法（如网点名称）：拿换过的字去比相接，两个同名网点会被误当成同一站。
  const stops: { code: string; delivery: boolean }[] = [];
  for (const leg of legs) {
    if (stops.length === 0 || stops[stops.length - 1].code !== leg.from) stops.push({ code: leg.from, delivery: false });
    stops.push({ code: leg.to, delivery: leg.kind === 'DELIVERY' });
  }
  return stops.map((stop) => (stop.delivery ? '末端派送' : label(stop.code))).join(' → ');
}
