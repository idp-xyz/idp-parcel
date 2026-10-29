import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { chargeCodeLabel } from './status';

// Covers: AT-PP-030、AT-PP-032 的界面词：方案级总额上下限产生的补差行与冲减行，费用代码有随产品发布的参考译法
// （最低收费、最高收费），不会在试算页费用行表里以原码露出。
test('总额上下限的两个费用代码有中文译法', () => {
  equal(chargeCodeLabel('MINIMUM_CHARGE'), '最低收费');
  equal(chargeCodeLabel('MAXIMUM_CHARGE'), '最高收费');
});
