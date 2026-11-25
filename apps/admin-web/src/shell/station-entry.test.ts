import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import { enterStationWith, leaveShellQuestion, tabsWithUnsavedInput, type StationEntryDeps } from './station-entry';

interface Calls {
  asked: string[];
  went: string[];
}

function deps(options: { tabs: string[]; unsaved?: string[]; answer?: boolean }): { deps: StationEntryDeps; calls: Calls } {
  const calls: Calls = { asked: [], went: [] };
  return {
    calls,
    deps: {
      openTabIds: () => options.tabs,
      hasUnsavedInput: (tabId) => (options.unsaved ?? []).includes(tabId),
      titleOf: (tabId) => `标题-${tabId}`,
      ask: (message) => {
        calls.asked.push(message);
        return options.answer ?? true;
      },
      go: (hash) => {
        calls.went.push(hash);
      },
    },
  };
}

test('只挑出带没提交输入的标签，次序照标签条', () => {
  deepEqual(tabsWithUnsavedInput(['a', 'b', 'c'], (id) => id !== 'b'), ['a', 'c']);
  deepEqual(tabsWithUnsavedInput([], () => true), []);
  deepEqual(tabsWithUnsavedInput(['a'], () => false), []);
});

test('没有带没提交输入的标签，直接进作业台，不问', () => {
  const { deps: d, calls } = deps({ tabs: ['warehouse-outbound', 'warehouse-waves/W-1'] });
  equal(enterStationWith('#/station/pick', d), true);
  deepEqual(calls.went, ['#/station/pick']);
  deepEqual(calls.asked, []);
});

test('没有开着的标签也直接进', () => {
  const { deps: d, calls } = deps({ tabs: [] });
  equal(enterStationWith('#/station/pick?wave=W-1', d), true);
  deepEqual(calls.went, ['#/station/pick?wave=W-1']);
  deepEqual(calls.asked, []);
});

test('有带没提交输入的标签就问一次，答继续才进', () => {
  const { deps: d, calls } = deps({ tabs: ['a', 'b'], unsaved: ['b'], answer: true });
  equal(enterStationWith('#/station/pick', d), true);
  equal(calls.asked.length, 1);
  deepEqual(calls.went, ['#/station/pick']);
});

test('答不继续就留在原地，地址不动', () => {
  const { deps: d, calls } = deps({ tabs: ['a', 'b'], unsaved: ['a'], answer: false });
  equal(enterStationWith('#/station/pick', d), false);
  equal(calls.asked.length, 1);
  deepEqual(calls.went, []);
});

test('问句说清是哪些标签、会丢什么、进去是什么意思', () => {
  const question = leaveShellQuestion(['出库单', '入库预报']);
  ok(question.includes('「出库单、入库预报」'));
  ok(question.includes('没提交的输入'));
  ok(question.includes('会丢'));
  ok(question.includes('全屏'));
});

test('同一个模块开了几张标签，标题只说一次；太多的写个数', () => {
  ok(leaveShellQuestion(['出库单', '出库单', '出库单']).includes('「出库单」'));
  const many = leaveShellQuestion(['甲', '乙', '丙', '丁', '戊']);
  ok(many.includes('「甲、乙、丙 等 5 个」'));
});

test('问句里的标题取标签的人话，不是标签 id', () => {
  const { deps: d, calls } = deps({ tabs: ['warehouse-outbound/OB-1'], unsaved: ['warehouse-outbound/OB-1'] });
  enterStationWith('#/station/pick', d);
  ok(calls.asked[0].includes('标题-warehouse-outbound/OB-1'));
});
