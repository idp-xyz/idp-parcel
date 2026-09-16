import { test, afterEach } from 'node:test';
import { equal } from 'node:assert/strict';
import {
  configureDisplayTimeZone,
  formatInstant,
  formatRange,
  wallTimeToRfc3339,
  zoneLabel,
} from './moment';

// 本文件钉的是「呈现时区是装配点配置的一维，未配置即 UTC」：Node 里跑的测试从不配置，
// 所以全站 38 处调用点上既有的 `… UTC` 断言一条不用改；浏览器在 main.tsx 配一次，操作者
// 看到的是本地时刻带显式偏移。两种输出都到秒、都不带毫秒——截图里 `04:27:55.939Z` 那种
// 半格式化串正是旧实现只剥 `.000Z` 留下的。

afterEach(() => {
  configureDisplayTimeZone('UTC');
});

// Covers: 未配置时区 → UTC；毫秒无论是不是 .000 一律不显；后缀是裸 `UTC` 不是 `UTC+0`。
test('默认按 UTC 显示到秒，毫秒不显', () => {
  equal(formatInstant('2026-09-16T04:27:55.939Z'), '2026-09-16 04:27:55 UTC');
  equal(formatInstant('2026-01-01T00:00:00.000Z'), '2026-01-01 00:00:00 UTC');
  equal(formatInstant('2026-01-01T00:00:00Z'), '2026-01-01 00:00:00 UTC');
});

// Covers: 装配点配了区就按该区显墙钟时刻，偏移写成 UTC+8；半小时区写 UTC+5:30；负偏移写 UTC-3。
test('配置显示时区后按该区显示并带显式偏移', () => {
  configureDisplayTimeZone('Asia/Shanghai');
  equal(formatInstant('2026-09-16T04:27:55.939Z'), '2026-09-16 12:27:55 UTC+8');

  configureDisplayTimeZone('Asia/Kolkata');
  equal(formatInstant('2026-09-16T04:27:55Z'), '2026-09-16 09:57:55 UTC+5:30');

  configureDisplayTimeZone('America/Sao_Paulo');
  equal(formatInstant('2026-09-16T04:27:55Z'), '2026-09-16 01:27:55 UTC-3');
});

// Covers: 单次调用可以显式指定时区，不动全局配置——悬停里要同时显 UTC 与本地。
test('单次调用可指定时区覆盖全局配置', () => {
  configureDisplayTimeZone('Asia/Shanghai');
  equal(formatInstant('2026-09-16T04:27:55Z', { timeZone: 'UTC' }), '2026-09-16 04:27:55 UTC');
  equal(zoneLabel('Asia/Shanghai', new Date('2026-09-16T04:27:55Z')), 'UTC+8');
  equal(zoneLabel('UTC', new Date('2026-09-16T04:27:55Z')), 'UTC');
});

// Covers: 解析不了的串原样交回，不抛、不改写——那是服务端给的值，页面不替它圆。
test('解析不了的时刻原样交回', () => {
  equal(formatInstant('not-a-moment'), 'not-a-moment');
  equal(formatRange('2026-01-01T00:00:00Z'), '2026-01-01 00:00:00 UTC → 持续有效');
  equal(
    formatRange('2026-01-01T00:00:00Z', '2026-01-01T01:00:00Z'),
    '2026-01-01 00:00:00 UTC → 2026-01-01 01:00:00 UTC',
  );
});

// Covers: datetime-local 收到的墙钟时刻按指定时区换成 RFC 3339 UTC，秒可缺补零，毫秒不带；
// 换不出来（空、格式不对）交回 null 让调用方按编码层问题报，不猜。
test('墙钟时刻按时区换成 RFC 3339 UTC', () => {
  equal(wallTimeToRfc3339('2026-01-02T08:00', 'Asia/Shanghai'), '2026-01-02T00:00:00Z');
  equal(wallTimeToRfc3339('2026-01-02T00:00:30', 'UTC'), '2026-01-02T00:00:30Z');
  equal(wallTimeToRfc3339('2026-07-01T09:30', 'Europe/Berlin'), '2026-07-01T07:30:00Z');
  equal(wallTimeToRfc3339('', 'UTC'), null);
  equal(wallTimeToRfc3339('2026-13-45T99:99', 'UTC'), null);
  equal(wallTimeToRfc3339('yesterday', 'UTC'), null);
});
