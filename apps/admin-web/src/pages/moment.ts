// 时刻呈现（票 admin-web-group-legal-entities/01 裁决 3）。
//
// **呈现时区是装配点配置的一维，未配置即 UTC。** 存储与线格式一律 UTC（RFC 3339），这里只管
// 显；浏览器在 main.tsx 配一次操作者的本地区，Node 里的行构造测试从不配置，于是既有的
// `… UTC` 断言一条不用改，且不随跑测试那台机器的时区变。这与 catalogue-api 的
// `configureMasterDataApi` 同一个形状：环境知识只在组合根出现一次。
//
// 输出一律到秒、不带毫秒、带显式偏移（`UTC` / `UTC+8` / `UTC+5:30` / `UTC-3`）。旧实现只剥
// `.000Z`，毫秒非零的时刻就漏成 `04:27:55.939Z` 半格式化串——同一列里两种写法并存，读的人
// 会以为那是两种时间。

let displayTimeZone = 'UTC';

/** 装配点调用一次；传不合法的 IANA 区名会在首次格式化时抛出，宁可早红。 */
export function configureDisplayTimeZone(timeZone: string): void {
  displayTimeZone = timeZone;
}

export function currentDisplayTimeZone(): string {
  return displayTimeZone;
}

interface WallClock {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
  second: number;
}

// Intl 是本模块唯一的时区知识来源：不内置任何偏移表，夏令时由它答。
function wallClockOf(instant: Date, timeZone: string): WallClock {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone,
    hourCycle: 'h23',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).formatToParts(instant);
  const read = (type: Intl.DateTimeFormatPartTypes) => Number(parts.find((part) => part.type === type)?.value);
  return {
    year: read('year'),
    month: read('month'),
    day: read('day'),
    // hourCycle h23 下 Intl 仍可能对午夜答 "24"（引擎差异），归零。
    hour: read('hour') % 24,
    minute: read('minute'),
    second: read('second'),
  };
}

function asUtcMillis(clock: WallClock): number {
  return Date.UTC(clock.year, clock.month - 1, clock.day, clock.hour, clock.minute, clock.second);
}

/** 该区在该时刻相对 UTC 的偏移分钟数（东正西负）。 */
export function zoneOffsetMinutes(instant: Date, timeZone: string): number {
  return Math.round((asUtcMillis(wallClockOf(instant, timeZone)) - instant.getTime()) / 60_000);
}

/** 偏移的显示名：零偏移是裸 `UTC`；整点偏移不带分钟；半点区带分钟。 */
export function zoneLabel(timeZone: string, instant: Date): string {
  const offset = zoneOffsetMinutes(instant, timeZone);
  if (offset === 0) return 'UTC';
  const sign = offset > 0 ? '+' : '-';
  const magnitude = Math.abs(offset);
  const hours = Math.floor(magnitude / 60);
  const minutes = magnitude % 60;
  return minutes === 0 ? `UTC${sign}${hours}` : `UTC${sign}${hours}:${String(minutes).padStart(2, '0')}`;
}

const pad = (value: number) => String(value).padStart(2, '0');

export interface FormatInstantOptions {
  /** 单次覆盖全局配置——悬停里要并排显 UTC 与本地时用。 */
  timeZone?: string;
}

/**
 * `YYYY-MM-DD HH:mm:ss <偏移>`。解析不了的串原样交回：那是服务端给的值，页面不替它圆，
 * 原样显出来比显成「Invalid Date」或空更容易让人去查源头。
 */
export function formatInstant(value: string, options: FormatInstantOptions = {}): string {
  const instant = new Date(value);
  if (Number.isNaN(instant.getTime())) return value;
  const timeZone = options.timeZone ?? displayTimeZone;
  const clock = wallClockOf(instant, timeZone);
  return (
    `${clock.year}-${pad(clock.month)}-${pad(clock.day)} ` +
    `${pad(clock.hour)}:${pad(clock.minute)}:${pad(clock.second)} ${zoneLabel(timeZone, instant)}`
  );
}

export function formatRange(from: string, to?: string): string {
  return `${formatInstant(from)} → ${to ? formatInstant(to) : '持续有效'}`;
}

const wallTimePattern = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/;

/**
 * `datetime-local` 收到的墙钟时刻（`YYYY-MM-DDTHH:mm[:ss]`，无时区）按给定 IANA 区换成 RFC 3339
 * UTC（到秒，`Z` 结尾）。换不出来交回 null，调用方按编码层问题报，不猜、不补默认。
 *
 * 算法：先把墙钟当 UTC 取一个猜测时刻，问该区在猜测时刻的偏移并回推，再用回推结果的偏移
 * 校正一次——跨夏令时切换点时第一次问到的偏移可能是切换前那一格。两次不一致时取第二次。
 */
export function wallTimeToRfc3339(wallTime: string, timeZone: string): string | null {
  const match = wallTimePattern.exec(wallTime.trim());
  if (!match) return null;
  const clock: WallClock = {
    year: Number(match[1]),
    month: Number(match[2]),
    day: Number(match[3]),
    hour: Number(match[4]),
    minute: Number(match[5]),
    second: Number(match[6] ?? '0'),
  };
  const guess = asUtcMillis(clock);
  if (Number.isNaN(guess)) return null;
  // Date.UTC 会把 13 月、45 日这种值滚进下一格而不报错；滚过就不是操作者填的那个时刻。
  const roundTrip = wallClockOf(new Date(guess), 'UTC');
  if (
    roundTrip.year !== clock.year ||
    roundTrip.month !== clock.month ||
    roundTrip.day !== clock.day ||
    roundTrip.hour !== clock.hour ||
    roundTrip.minute !== clock.minute ||
    roundTrip.second !== clock.second
  ) {
    return null;
  }
  const first = guess - zoneOffsetMinutes(new Date(guess), timeZone) * 60_000;
  const resolved = guess - zoneOffsetMinutes(new Date(first), timeZone) * 60_000;
  return new Date(resolved).toISOString().replace(/\.\d{3}Z$/, 'Z');
}
