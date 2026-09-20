import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  resolveWorkspaceTabs,
  workspaceTabLabels,
  workspaceTabOrder,
  type WorkspaceTabId,
} from './workspace-tabs';

// 本文件钉的是对象工作区签的两条纯规则（票 admin-web-ux-alignment/04）：固定表与归并。渲染件 DetailPageTemplate 引
// ESM-only 的 @idpxyz 原语，run-tests 的 CommonJS 发射加载不了它（05 / 06 票面有实测），组件层在这里钉不住。
// Content 用字串代替 ReactNode——归并对内容是什么不感兴趣，只管谁在谁前。

// Covers: 六个签的 id、顺序与中文词由固定表钉住（手册「Main Content Tabs 稳定命名」）——换产品不换签名，
// 调用方给不了新 id，也改不了词与顺序。
test('固定表：六个签按手册顺序、词一一对应', () => {
  deepEqual([...workspaceTabOrder], ['summary', 'timeline', 'related', 'exceptions', 'documents', 'audit']);
  deepEqual(
    workspaceTabOrder.map((id) => workspaceTabLabels[id]),
    ['概要', '时间线', '关联', '异常', '文档', '审计'],
  );
});

// Covers: 调用方传进来的顺序不算数——签按固定序排，与传入次序无关。
test('归并按固定序排，调用方传入顺序不算数', () => {
  const tabs = resolveWorkspaceTabs<string>(
    {},
    [
      { id: 'audit', content: 'a' },
      { id: 'related', content: 'r' },
      { id: 'summary', content: 's' },
    ],
  );
  deepEqual(
    tabs.map((tab) => tab.id),
    ['summary', 'related', 'audit'],
  );
});

// Covers: 模板自身内容（基本信息 / 区块 → 概要，审计留痕 → 审计）与调用方同 id 内容按 id 合，
// 模板内容在前、调用方内容接在其后（票面第 3 条）。
test('同 id 归并：模板内容在前，调用方内容接在其后', () => {
  const tabs = resolveWorkspaceTabs<string>(
    { summary: ['basic', 'section-1'], audit: ['trail'] },
    [
      { id: 'summary', content: 'caller-summary' },
      { id: 'audit', content: 'caller-audit' },
    ],
  );
  deepEqual(
    tabs.map((tab) => [tab.id, tab.contents]),
    [
      ['summary', ['basic', 'section-1', 'caller-summary']],
      ['audit', ['trail', 'caller-audit']],
    ],
  );
});

// Covers: 签只显有内容的，不显禁用签（票面裁决 1）——没有任何内容的 id 不出签；只有模板内容或只有调用方内容都算有。
test('没有内容的 id 不出签；只有一方给了内容也出', () => {
  const tabs = resolveWorkspaceTabs<string>({ summary: ['basic'] }, [{ id: 'documents', content: 'doc' }]);
  deepEqual(
    tabs.map((tab) => tab.id),
    ['summary', 'documents'],
  );
  deepEqual(resolveWorkspaceTabs<string>({}, []), []);
  deepEqual(resolveWorkspaceTabs<string>({}, undefined), []);
});

// Covers: 调用方同一 id 传多次，内容按传入次序依次接上；count 取第一个给了的——两份计数不相加，
// 计数是调用方对那一签的陈述，模板不替它做算术。
test('同 id 多次传入依次接上，count 取第一个给了的', () => {
  const tabs = resolveWorkspaceTabs<string>({}, [
    { id: 'related', content: 'r1' },
    { id: 'related', content: 'r2', count: 2 },
    { id: 'related', content: 'r3', count: 9 },
  ]);
  equal(tabs.length, 1);
  deepEqual(tabs[0].contents, ['r1', 'r2', 'r3']);
  equal(tabs[0].count, 2);
});

// Covers: 每个出签的 id 都带固定表里的词，词不从调用方来。
test('出签的词取自固定表', () => {
  const ids: WorkspaceTabId[] = ['timeline', 'exceptions'];
  const tabs = resolveWorkspaceTabs<string>({}, ids.map((id) => ({ id, content: id })));
  deepEqual(
    tabs.map((tab) => tab.label),
    ['时间线', '异常'],
  );
  for (const tab of tabs) equal(tab.count, undefined);
});
