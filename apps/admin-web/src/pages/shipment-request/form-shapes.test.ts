import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { duplicatedAt, positiveDecimal, requestEffectiveAtOf } from './form-shapes';

// 本文件钉下单页共用的几条形状(票 declared-goods-lines/05):重号按去空白后比、空格不算;开始时间按显示时区折成 RFC 3339,
// 留空是没说、填半截是形状错。

test('重号:两端空白去掉后相同即重,首次出现的那一格也标上;空格不算', () => {
  deepEqual(duplicatedAt(['A', ' B', '', 'B ', 'C', '', 'A']), [0, 1, 3, 6]);
  deepEqual(duplicatedAt(['', '']), []);
});

test('正十进制:0 不是正数,不收单位与千分位', () => {
  equal(positiveDecimal('1.50'), true);
  equal(positiveDecimal('0.00'), false);
  equal(positiveDecimal('1,200'), false);
});

test('开始时间按显示时区折成 RFC 3339;留空键缺席,填不全报形状错', () => {
  equal(requestEffectiveAtOf('2026-11-10T10:00', 'Asia/Shanghai'), '2026-11-10T02:00:00Z');
  equal(requestEffectiveAtOf('', 'Asia/Shanghai'), undefined);
  equal(requestEffectiveAtOf('2026-11-10', 'Asia/Shanghai'), null);
});
