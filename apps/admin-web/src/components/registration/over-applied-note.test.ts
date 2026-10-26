import { test } from 'node:test';
import { equal, match } from 'node:assert/strict';
import { overAppliedNote, unreconciledScopesNote } from './over-applied-note';

// 本文件钉登记答案里外部资金事实更正的两类后话（ADR-0285）：服务端把超核金额放在响应体的 overAppliedMinor 上、把没推进的
// 作用域放在 unreconciledScopes 上，状态码仍是已采用；页面若只读 outcome，操作员手工把一笔收款往下改时就看不到有核销要撤、
// 有作用域账不平——和受控 CLI 说「累计超核」「账已不平」是同一件事。组件本身带着 ui-primitives，Node 测试装不进来，所以说明
// 抽成纯函数在这里钉。

// Covers: 超核的更正：说累计超核的金额与续办（要人撤哪一笔核销），不断言入账余额已重算——那一句对没映射到作用域的事实与有作用域
// 被跳过的更正都不属实。
test('超核的更正：说累计超核的金额与续办，不断言入账余额已重算', () => {
  const note = overAppliedNote(2000) ?? '';
  match(note, /累计超核：2000/);
  match(note, /撤销哪一笔核销要人判断/);
  equal(note.includes('已按新金额重算'), false);
});

// Covers: 没有超核（字段缺席、为 0）：不出现说明。
test('没有超核：不出现说明', () => {
  equal(overAppliedNote(undefined), undefined);
  equal(overAppliedNote(0), undefined);
});

// Covers: 有作用域被跳过：逐个点名法人、账户、币种，说入账余额没有跟着推进与续办。
test('作用域账已不平：逐个点名没推进的作用域', () => {
  const note =
    unreconciledScopesNote([
      { legalEntity: 'le-1', account: 'acct-1', currency: 'CNY' },
      { legalEntity: 'le-1', account: 'acct-2', currency: 'CNY' },
    ]) ?? '';
  match(note, /法人 le-1、账户 acct-1、币种 CNY/);
  match(note, /法人 le-1、账户 acct-2、币种 CNY/);
  match(note, /入账余额没有跟着新金额推进/);
  match(note, /核对指向它们的核销与调整/);
});

// Covers: 没有作用域被跳过（字段缺席、空数组）：不出现说明。
test('没有作用域被跳过：不出现说明', () => {
  equal(unreconciledScopesNote(undefined), undefined);
  equal(unreconciledScopesNote([]), undefined);
});
