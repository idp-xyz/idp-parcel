import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import { moduleInfoById, navigationSections, pageTitleById } from './navigation';

// 本文件钉改版后的导航与页名（设计源「三、新的信息架构」）：十组按运营工作排；每页有运营叫法的标题与一句 summary；
// 导航条目与 pageTitleById 一一对上。页 id 不在这里钉——page-registry 的 liveIds 与 pageById 守着它们。

test('导航分十组、按运营工作排序', () => {
  deepEqual(
    navigationSections.map((section) => section.title),
    ['工作台', '运单', '客户', '合作伙伴', '产品与价格', '线路与运力', '清关', '异常与索赔', '账务', '设置'],
  );
});

test('导航每个条目都有页名，且条目 label 与页名一致', () => {
  for (const section of navigationSections) {
    for (const item of section.items) {
      ok(pageTitleById[item.id] !== undefined, `${item.id} 没有页名`);
      equal(item.label, pageTitleById[item.id], `${item.id} 的导航词与页名不一致`);
    }
  }
});

test('每个已登记模块都有运营叫法的标题与一句 summary，都不含英文', () => {
  for (const [id, info] of Object.entries(moduleInfoById)) {
    ok(info.title.trim() !== '', `${id} 标题为空`);
    equal(/[A-Za-z]/.test(info.title), false, `${id} 的标题「${info.title}」含英文`);
    ok(info.summary !== undefined && info.summary.trim() !== '', `${id} 没有 summary`);
    equal(/[A-Za-z]/.test(info.summary ?? ''), false, `${id} 的 summary 含英文`);
  }
});

test('试点治理与演示在设置组末尾；工作台组收下「我的工作」两条', () => {
  const settings = navigationSections[navigationSections.length - 1];
  deepEqual(settings.items.slice(-2).map((item) => item.id), ['stage-admission', 'template-preview']);
  deepEqual(
    navigationSections[0].items.map((item) => item.id),
    ['workbench', 'recent-objects', 'saved-views'],
  );
});

test('导航里每个条目都在 moduleInfoById 或固定页名表里，没有查无出处的 id', () => {
  const navIds = navigationSections.flatMap((section) => section.items.map((item) => item.id));
  const registered = new Set([...Object.keys(moduleInfoById), 'workbench', 'shipment-request', 'template-preview']);
  for (const id of navIds) ok(registered.has(id), `${id} 查无出处`);
  for (const id of Object.keys(moduleInfoById)) ok(navIds.includes(id), `${id} 登记了却不在导航里`);
});
