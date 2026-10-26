import { test } from 'node:test';
import { equal, match, notEqual, ok } from 'node:assert/strict';
import { cancellationOutcomeViews } from './presentation';
import type { CancellationOutcome } from './api';

// 本文件钉取消结果词表里委托生命周期闸门的两个词(UC-PS-006 结果语义「委托已终结」「委托待决定」)。
// 期望值取用例文档的原词与客户可行动,不从实现推。

test('取消词表收录委托已终结与委托待决定两个词,标签取用例原词且不画成成立', () => {
  const ended = cancellationOutcomeViews.REQUEST_ENDED;
  const awaiting = cancellationOutcomeViews.REQUEST_AWAITING_DECISION;

  equal(ended.label, '委托已终结');
  equal(awaiting.label, '委托待决定');
  // 两者都是确定的业务答复,不是取消成立,也不是故障。
  ok(!ended.affirmative);
  ok(!awaiting.affirmative);
});

test('委托已终结说无事可做,委托待决定指向撤回', () => {
  const ended = cancellationOutcomeViews.REQUEST_ENDED;
  const awaiting = cancellationOutcomeViews.REQUEST_AWAITING_DECISION;

  match(ended.note, /已被撤回或已被拒绝/);
  match(ended.note, /无事可做/);
  match(awaiting.note, /只是已提交/);
  match(awaiting.note, /改用撤回/);
});

test('取消词表每个词都有标签与说明,且标签互不相同(两个新词不并进未决或请求未受理)', () => {
  const outcomes = Object.keys(cancellationOutcomeViews) as CancellationOutcome[];
  const labels = new Set<string>();
  for (const outcome of outcomes) {
    const view = cancellationOutcomeViews[outcome];
    ok(view.label.length > 0, `${outcome} 缺标签`);
    ok(view.note.length > 0, `${outcome} 缺说明`);
    labels.add(view.label);
  }
  equal(labels.size, outcomes.length);
  notEqual(cancellationOutcomeViews.REQUEST_ENDED.label, cancellationOutcomeViews.CANCELLATION_UNDECIDED.label);
  notEqual(cancellationOutcomeViews.REQUEST_AWAITING_DECISION.label, cancellationOutcomeViews.REQUEST_NOT_ACCEPTED.label);
});
