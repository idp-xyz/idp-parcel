import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  SAVED_VIEWS_STORAGE_KEY,
  defaultSavedViewFor,
  filterSavedViews,
  listSavedViews,
  listSavedViewsFor,
  removeSavedView,
  saveView,
  savedViewHash,
  savedViewIdFromHash,
  savedViewModuleIds,
  setDefaultSavedView,
  updateSavedView,
  type SavedViewsClock,
  type SavedViewsStorage,
} from './saved-views';

// 本文件钉的是保存视图的纯逻辑（票 admin-web-ux-alignment/02 第 3 条）：键名带产品名、state 不透明原样存取、默认同模块唯一、
// 按模块列出、删除是唯一删除入口、跳转地址形。存储用 Map 顶替、时钟与 id 可注入，不碰真 localStorage、不看真时钟。

function storageOf(initial: Record<string, string> = {}): SavedViewsStorage & { dump(): Record<string, string> } {
  const map = new Map(Object.entries(initial));
  return {
    getItem: (key) => map.get(key) ?? null,
    setItem: (key, value) => {
      map.set(key, value);
    },
    removeItem: (key) => {
      map.delete(key);
    },
    dump: () => Object.fromEntries(map),
  };
}

function clockOf(): SavedViewsClock & { tick(): void } {
  let n = 0;
  return {
    now: () => `2026-09-20T10:0${n}:00Z`,
    newId: () => `v${n}`,
    tick: () => {
      n += 1;
    },
  };
}

// Covers: 键名带产品名。
test('存储键带产品名', () => {
  equal(SAVED_VIEWS_STORAGE_KEY, 'parcel-admin-web:saved-views');
});

// Covers: 无存储与坏存储读空；缺字段的条目被剔掉。
test('无存储或坏存储读出空列表', () => {
  deepEqual(listSavedViews(storageOf()), []);
  deepEqual(listSavedViews(storageOf({ [SAVED_VIEWS_STORAGE_KEY]: 'nope' })), []);
  const mixed = JSON.stringify([
    { id: 'v1', moduleId: 'm', name: 'n', state: null, savedAt: 't', isDefault: false },
    { id: 'v2', moduleId: 'm', name: 'n' },
  ]);
  equal(listSavedViews(storageOf({ [SAVED_VIEWS_STORAGE_KEY]: mixed })).length, 1);
});

// Covers: 保存后能读回，state 原样（对象 / 数组 / null 都不解释）；新的排最前；名字留空不存。
test('保存：state 不透明原样存取，新的在前，空名不存', () => {
  const storage = storageOf();
  const clock = clockOf();
  const state = { status: ['已提交', '已接受'], sort: 'submittedAt desc', nested: { page: 2 } };
  const first = saveView(storage, { moduleId: 'shipment-request-inquiry', name: ' 待处理 ', state }, clock);
  equal(first?.id, 'v0');
  equal(first?.name, '待处理');
  equal(first?.isDefault, false);
  clock.tick();
  saveView(storage, { moduleId: 'price-card-catalog', name: '在用价卡', state: [1, 2, 3] }, clock);
  clock.tick();
  equal(saveView(storage, { moduleId: 'x', name: '   ', state: null }, clock), null);

  const list = listSavedViews(storage);
  deepEqual(list.map((view) => view.id), ['v1', 'v0']);
  deepEqual(list[1].state, state);
  deepEqual(list[0].state, [1, 2, 3]);
  equal(list[1].savedAt, '2026-09-20T10:00:00Z');
});

// Covers: 按模块列出只给该模块的；别的模块一条不带。
test('按模块列出', () => {
  const storage = storageOf();
  const clock = clockOf();
  saveView(storage, { moduleId: 'a', name: 'a1', state: 1 }, clock);
  clock.tick();
  saveView(storage, { moduleId: 'b', name: 'b1', state: 2 }, clock);
  clock.tick();
  saveView(storage, { moduleId: 'a', name: 'a2', state: 3 }, clock);
  deepEqual(listSavedViewsFor(storage, 'a').map((view) => view.name), ['a2', 'a1']);
  deepEqual(listSavedViewsFor(storage, 'b').map((view) => view.name), ['b1']);
  deepEqual(listSavedViewsFor(storage, 'c'), []);
});

// Covers: 默认同模块唯一——设第二个默认时第一个撤下；别的模块的默认不受影响；不存在的 id 返回 false 不写。
test('默认视图同模块唯一', () => {
  const storage = storageOf();
  const clock = clockOf();
  const a1 = saveView(storage, { moduleId: 'a', name: 'a1', state: 1 }, clock)!;
  clock.tick();
  const a2 = saveView(storage, { moduleId: 'a', name: 'a2', state: 2 }, clock)!;
  clock.tick();
  const b1 = saveView(storage, { moduleId: 'b', name: 'b1', state: 3 }, clock)!;

  equal(defaultSavedViewFor(storage, 'a'), null);
  equal(setDefaultSavedView(storage, a1.id), true);
  equal(setDefaultSavedView(storage, b1.id), true);
  equal(defaultSavedViewFor(storage, 'a')?.id, a1.id);
  equal(defaultSavedViewFor(storage, 'b')?.id, b1.id);

  equal(setDefaultSavedView(storage, a2.id), true);
  equal(defaultSavedViewFor(storage, 'a')?.id, a2.id);
  equal(listSavedViewsFor(storage, 'a').filter((view) => view.isDefault).length, 1);
  equal(defaultSavedViewFor(storage, 'b')?.id, b1.id, '别的模块的默认不动');

  const before = storage.dump();
  equal(setDefaultSavedView(storage, 'ghost'), false);
  deepEqual(storage.dump(), before);
});

// Covers: 更新改名 / 换 state，时刻刷新、排到最前；空名不更新；不存在返回 null。
test('更新', () => {
  const storage = storageOf();
  const clock = clockOf();
  const v0 = saveView(storage, { moduleId: 'a', name: 'old', state: { p: 1 } }, clock)!;
  clock.tick();
  saveView(storage, { moduleId: 'a', name: 'other', state: 0 }, clock);
  clock.tick();

  const updated = updateSavedView(storage, v0.id, { name: 'new' }, clock);
  equal(updated?.name, 'new');
  deepEqual(updated?.state, { p: 1 }, '没给 state 就不动');
  equal(updated?.savedAt, '2026-09-20T10:02:00Z');
  equal(listSavedViews(storage)[0].id, v0.id, '更新后排最前');

  equal(updateSavedView(storage, v0.id, { name: '  ' }, clock), null);
  equal(updateSavedView(storage, 'ghost', { name: 'x' }, clock), null);
  deepEqual(updateSavedView(storage, v0.id, { state: [9] }, clock)?.state, [9]);
});

// Covers: 删除是唯一删除入口——删中返回 true；删到空整键移除；不存在返回 false 不写。
test('删除', () => {
  const storage = storageOf();
  const clock = clockOf();
  const v0 = saveView(storage, { moduleId: 'a', name: 'a1', state: 1 }, clock)!;
  clock.tick();
  const v1 = saveView(storage, { moduleId: 'a', name: 'a2', state: 2 }, clock)!;

  equal(removeSavedView(storage, 'ghost'), false);
  equal(removeSavedView(storage, v0.id), true);
  deepEqual(listSavedViews(storage).map((view) => view.id), [v1.id]);
  equal(removeSavedView(storage, v1.id), true);
  deepEqual(storage.dump(), {});
});

// Covers: 跳转地址 `#/<moduleId>?view=<id>`，id 按 URI 组件编码；从 hash 取回同一个 id；没有 ?view= 返回 null。
test('跳转地址与解析', () => {
  const hash = savedViewHash({ moduleId: 'shipment-request-inquiry', id: 'a b/c' });
  equal(hash, '#/shipment-request-inquiry?view=a%20b%2Fc');
  equal(savedViewIdFromHash(hash), 'a b/c');
  equal(savedViewIdFromHash('#/shipment-request-inquiry'), null);
  equal(savedViewIdFromHash('#/shipment-request-inquiry?other=1'), null);
  equal(savedViewIdFromHash('#/shipment-request-inquiry?view='), null);
});

// Covers: 页面过滤——名字包含不分大小写、按模块、可叠；模块列表按首次出现。
test('过滤与模块列表', () => {
  const storage = storageOf();
  const clock = clockOf();
  saveView(storage, { moduleId: 'a', name: 'Pending', state: 1 }, clock);
  clock.tick();
  saveView(storage, { moduleId: 'b', name: '在用', state: 2 }, clock);
  clock.tick();
  saveView(storage, { moduleId: 'a', name: '待处理', state: 3 }, clock);
  const list = listSavedViews(storage);
  deepEqual(filterSavedViews(list, { keyword: 'pend', moduleId: null }).map((view) => view.name), ['Pending']);
  deepEqual(filterSavedViews(list, { keyword: '', moduleId: 'a' }).map((view) => view.name), ['待处理', 'Pending']);
  deepEqual(filterSavedViews(list, { keyword: '待', moduleId: 'a' }).map((view) => view.name), ['待处理']);
  deepEqual(savedViewModuleIds(list), ['a', 'b']);
});
