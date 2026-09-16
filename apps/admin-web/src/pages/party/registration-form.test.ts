import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { revisionOf } from './registration-form';

// 本文件钉的是参与方模块各登记表单共用的编码层规则（票 admin-web-group-legal-entities/13 第 3 条抬出）。四份表单
// 的修订号格同一判：编成的是 JSON 整数不是身份串，所以这一格允许首尾空白、其余身份串各表单自己裁不裁与它无关。

// Covers: 修订号只收正整数原文——零、负数、小数、空、非数字都编不出，交回 undefined 由各表单的问题表拦住不送。
test('修订号只编正整数', () => {
  equal(revisionOf('1'), 1);
  equal(revisionOf('12'), 12);
  equal(revisionOf('0'), undefined);
  equal(revisionOf('-1'), undefined);
  equal(revisionOf('1.5'), undefined);
  equal(revisionOf(''), undefined);
  equal(revisionOf('x'), undefined);
});

// Covers: 修订号周围的空白不构成第二个值——它编成的是数，" 3 " 与 "3" 是同一个修订号；这是身份串「不裁空白」判据
// 的唯一例外，理由在 business-party-form.ts 文件头。
test('修订号允许首尾空白', () => {
  equal(revisionOf(' 3 '), 3);
  equal(revisionOf('\t7\n'), 7);
  equal(revisionOf('   '), undefined);
});
