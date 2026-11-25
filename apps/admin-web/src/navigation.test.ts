import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import {
  moduleInfoById,
  navigationSections,
  pageTitleById,
  parcelNavigationSections,
  warehouseNavigationSections,
} from './navigation';

// 国际小包与仓配系统各自一套独立导航，互不混杂；navigationSections 是合并后的唯一模块分区表，供面包屑反查。

test('国际小包导航保持十组运营顺序，不含仓配页面', () => {
  deepEqual(
    parcelNavigationSections.map((section) => section.title),
    ['工作台', '运单', '客户', '合作伙伴', '产品与价格', '线路与运力', '清关', '异常与索赔', '账务', '设置'],
  );
});

test('仓配系统导航按岗位分七组，现场的人只用作业台一组', () => {
  deepEqual(
    warehouseNavigationSections.map((section) => section.title),
    ['总览', '入库', '出库', '作业台', '库存', '计费', '基础资料'],
  );
  deepEqual(
    warehouseNavigationSections.find((section) => section.title === '作业台')?.items.map((item) => item.id),
    ['warehouse-workstation'],
  );
});

test('两个工作区共用的扫描设备与操作者授予挂在仓配基础资料末尾', () => {
  const baseData = warehouseNavigationSections[warehouseNavigationSections.length - 1];
  deepEqual(baseData.items.slice(-2).map((item) => item.id), ['work-devices', 'operator-access']);
});

test('导航每个条目都有页名，且条目 label 与页名一致', () => {
  for (const section of [...navigationSections, ...warehouseNavigationSections]) {
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

test('试点治理与演示在国际小包设置组末尾；工作台组收下「我的工作」两条', () => {
  const settings = parcelNavigationSections[parcelNavigationSections.length - 1];
  deepEqual(settings.items.slice(-2).map((item) => item.id), ['stage-admission', 'template-preview']);
  deepEqual(
    parcelNavigationSections[0].items.map((item) => item.id),
    ['workbench', 'recent-objects', 'saved-views'],
  );
});

test('导航里每个条目都在 moduleInfoById 或固定页名表里，没有查无出处的 id', () => {
  const navIds = navigationSections.flatMap((section) => section.items.map((item) => item.id));
  const registered = new Set([...Object.keys(moduleInfoById), 'workbench', 'shipment-request', 'template-preview']);
  for (const id of navIds) ok(registered.has(id), `${id} 查无出处`);
  for (const id of Object.keys(moduleInfoById)) ok(navIds.includes(id), `${id} 登记了却不在导航里`);
});
