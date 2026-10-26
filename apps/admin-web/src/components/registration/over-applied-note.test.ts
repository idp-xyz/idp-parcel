import { test } from 'node:test';
import { equal, match } from 'node:assert/strict';
import { overAppliedNote } from './over-applied-note';

// 本文件钉登记答案里外部资金事实更正的超核说明（ADR-0285）：服务端把超核金额放在响应体的 overAppliedMinor 上、状态码仍是
// 已采用；页面若只读 outcome，操作员手工把一笔收款往下改时就看不到有核销要撤——和受控 CLI 说「累计超核」是同一件事。
// 组件本身带着 ui-primitives，Node 测试装不进来，所以说明抽成纯函数在这里钉。

// Covers: 超核的更正：说累计超核的金额与续办（要人撤哪一笔核销）。
test('超核的更正：说累计超核的金额与续办', () => {
  const note = overAppliedNote(2000) ?? '';
  match(note, /累计超核：2000/);
  match(note, /撤销哪一笔核销要人判断/);
});

// Covers: 没有超核（字段缺席、为 0）：不出现说明。
test('没有超核：不出现说明', () => {
  equal(overAppliedNote(undefined), undefined);
  equal(overAppliedNote(0), undefined);
});
