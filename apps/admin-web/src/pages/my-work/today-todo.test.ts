import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import {
  acceptanceReviewCount,
  disputedStatementCount,
  openExceptionCaseCount,
  todoNeedsAttention,
  todoNoteText,
  todoStateOf,
  todoValueText,
  unadmittedObjectCount,
} from './today-todo';

// 本文件钉首页「今日待办」的判读：读口答了才有数，读不到不报 0，三种读不到分开说。

const count = (body: { n: number }) => body.n;

test('读口答了才有数，0 只在真答了 0 时出现', () => {
  equal(todoValueText(todoStateOf(null, count)), '…');
  equal(todoValueText(todoStateOf({ kind: 'outcome', body: { n: 3 } }, count)), '3');
  equal(todoValueText(todoStateOf({ kind: 'outcome', body: { n: 0 } }, count)), '0');
  equal(todoNoteText(todoStateOf({ kind: 'outcome', body: { n: 0 } }, count), '等人处理'), '暂时没有要处理的');
  equal(todoNoteText(todoStateOf({ kind: 'outcome', body: { n: 2 } }, count), '等人处理'), '等人处理');
});

test('未接通、无权查看、读不到三种分开，不折成 0', () => {
  equal(todoValueText(todoStateOf({ kind: 'unconfigured' }, count)), '未接通');
  equal(todoValueText(todoStateOf({ kind: 'notGranted' }, count)), '无权查看');
  equal(todoValueText(todoStateOf({ kind: 'loginRequired' }, count)), '无权查看');
  equal(todoValueText(todoStateOf({ kind: 'noAnswer' }, count)), '读不到');
  equal(todoValueText(todoStateOf({ kind: 'transport' }, count)), '读不到');
  equal(todoNeedsAttention(todoStateOf({ kind: 'unconfigured' }, count)), false);
  equal(todoNeedsAttention(todoStateOf({ kind: 'outcome', body: { n: 1 } }, count)), true);
});

test('各格数的是在等人处理的那几件', () => {
  equal(acceptanceReviewCount({ entries: [{ reviewCompleted: false }, { reviewCompleted: true }] }), 1);
  equal(openExceptionCaseCount({ cases: [{}, { closedAt: '2026-10-28T00:00:00Z' }] }), 1);
});

test('卡关与待对账用读口答的全部件数，不数截断后的那一页', () => {
  equal(unadmittedObjectCount({ unadmittedCount: 250 }), 250);
  equal(disputedStatementCount({ disputedCount: 0 }), 0);
});
