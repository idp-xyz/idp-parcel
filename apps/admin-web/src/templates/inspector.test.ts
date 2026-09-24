import { test } from 'node:test';
import { deepEqual, equal, match, throws } from 'node:assert/strict';
import {
  INSPECTOR_SUMMARY_LIMIT,
  InspectorContractError,
  inspectorActionDisabled,
  inspectorSectionOrder,
  presentFields,
  resolveInspectorForPanel,
  resolveInspectorSections,
  type InspectorContent,
} from './inspector';

// 本文件钉的是检查器契约的硬规则（票 admin-web-workspace-form/02 第 1 条）：节序固定、空节不渲染、概要上限、假动作抛。
// 面板怎么渲染（折叠节开不开、禁用动作带不带说明）在 node:test 里钉不到，用 scripts/dom-probe.mjs 一次性实测，结论写票面。

const full: InspectorContent = {
  title: '委托 SR-1',
  sections: [
    { kind: 'audit', fields: [{ label: '提交时间', value: '2026-09-22T10:00:00Z', mono: true }] },
    { kind: 'related', links: [{ label: '客户合同', hash: '#/party-contracts' }] },
    { kind: 'actions', actions: [{ label: '打开详情', onRun: () => {} }] },
    { kind: 'status', items: [{ label: '委托状态', word: '已接受' }] },
    { kind: 'summary', fields: [{ label: '客户账户', value: 'CA-1' }] },
  ],
};

test('节序固定为 概要 → 状态 → 快速动作 → 关联对象 → 审计，调用方的顺序不算数', () => {
  const kinds = resolveInspectorSections(full).map((s) => s.kind);
  deepEqual(kinds, [...inspectorSectionOrder]);
  deepEqual(
    resolveInspectorSections(full).map((s) => s.label),
    ['概要', '状态', '快速动作', '关联对象', '审计'],
  );
});

test('默认展开：概要 / 状态 / 快速动作开着，关联对象 / 审计折着', () => {
  deepEqual(
    resolveInspectorSections(full).map((s) => [s.kind, s.defaultOpen]),
    [
      ['summary', true],
      ['status', true],
      ['actions', true],
      ['related', false],
      ['audit', false],
    ],
  );
});

test('缺的节与空的节都不出现在结果里', () => {
  const sparse: InspectorContent = {
    title: 'x',
    sections: [
      { kind: 'summary', fields: [{ label: 'a', value: '1' }] },
      { kind: 'related', links: [] },
      { kind: 'actions', actions: [] },
    ],
  };
  deepEqual(resolveInspectorSections(sparse).map((s) => s.kind), ['summary']);
  deepEqual(resolveInspectorSections({ title: 'x', sections: [] }), []);
});

test('同一种节给多次按序接起来', () => {
  const twice: InspectorContent = {
    title: 'x',
    sections: [
      { kind: 'summary', fields: [{ label: 'a', value: '1' }] },
      { kind: 'status', items: [{ label: 's', word: '未决' }] },
      { kind: 'summary', fields: [{ label: 'b', value: '2' }] },
    ],
  };
  const [summary] = resolveInspectorSections(twice);
  equal(summary.section.kind, 'summary');
  if (summary.section.kind === 'summary') deepEqual(summary.section.fields.map((f) => f.label), ['a', 'b']);
});

test(`概要超过 ${INSPECTOR_SUMMARY_LIMIT} 格抛契约错误，恰好 ${INSPECTOR_SUMMARY_LIMIT} 格放行`, () => {
  const fields = (n: number) => Array.from({ length: n }, (_, i) => ({ label: `f${i}`, value: String(i) }));
  equal(resolveInspectorSections({ title: 'x', sections: [{ kind: 'summary', fields: fields(INSPECTOR_SUMMARY_LIMIT) }] }).length, 1);
  throws(
    () => resolveInspectorSections({ title: 'x', sections: [{ kind: 'summary', fields: fields(INSPECTOR_SUMMARY_LIMIT + 1) }] }),
    InspectorContractError,
  );
});

test('动作既无 onRun 也无 disabledReason 抛；有其一放行', () => {
  throws(
    () => resolveInspectorSections({ title: 'x', sections: [{ kind: 'actions', actions: [{ label: '空转' }] }] }),
    (error: unknown) => error instanceof InspectorContractError && /空转/.test(error.message),
  );
  equal(
    resolveInspectorSections({
      title: 'x',
      sections: [{ kind: 'actions', actions: [{ label: '等端点', disabledReason: '命令端点未建' }, { label: '能按', onRun: () => {} }] }],
    }).length,
    1,
  );
});

test('resolveInspectorForPanel：合契约的照常归并，契约错误接住成说明，别的错误照抛', () => {
  const ok = resolveInspectorForPanel(full);
  equal(ok.kind, 'sections');
  if (ok.kind === 'sections') deepEqual(ok.sections.map((s) => s.kind), [...inspectorSectionOrder]);

  const violated = resolveInspectorForPanel({ title: 'x', sections: [{ kind: 'actions', actions: [{ label: '空转' }] }] });
  equal(violated.kind, 'contractError');
  if (violated.kind === 'contractError') match(violated.message, /空转/);

  throws(() => resolveInspectorForPanel({ title: 'x', sections: null as unknown as InspectorContent['sections'] }), TypeError);
});

test('inspectorActionDisabled：给了说明即禁用，即便同时给了 onRun', () => {
  equal(inspectorActionDisabled({ label: 'a', onRun: () => {} }), false);
  equal(inspectorActionDisabled({ label: 'a', disabledReason: '等端点' }), true);
  equal(inspectorActionDisabled({ label: 'a', onRun: () => {}, disabledReason: '等端点' }), true);
});

test('presentFields 丢掉空值、null 与 undefined 的格', () => {
  deepEqual(
    presentFields([{ label: 'a', value: '1' }, { label: 'b', value: '' }, { label: 'c', value: '  ' }, null, undefined]),
    [{ label: 'a', value: '1' }],
  );
});
