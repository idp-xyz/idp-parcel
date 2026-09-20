import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  RECENT_OBJECTS_LIMIT,
  RECENT_OBJECTS_STORAGE_KEY,
  clearRecentObjects,
  filterRecentObjects,
  listRecentObjects,
  recentModuleIds,
  recentObjectFromHash,
  recentObjectHash,
  recentObjectTitle,
  recordRecentObject,
  type RecentObject,
  type RecentObjectsStorage,
} from './recent-objects';

// 本文件钉的是最近对象的纯逻辑（票 admin-web-ux-alignment/02 第 2 条）：键名带产品名、去重置顶、上限、清空是唯一删除入口、
// 从 hash 认对象地址、标题不发请求只拼字。存储用 Map 顶替，不碰真 localStorage。

function storageOf(initial: Record<string, string> = {}): RecentObjectsStorage & { dump(): Record<string, string> } {
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

function entry(moduleId: string, objectId: string, at = '2026-09-20T10:00:00Z'): RecentObject {
  return { moduleId, objectId, title: recentObjectTitle(undefined, moduleId, objectId), at };
}

// Covers: 键名带产品名——同一浏览器上多个 IDP 产品各存各的。
test('存储键带产品名', () => {
  equal(RECENT_OBJECTS_STORAGE_KEY, 'parcel-admin-web:recent-objects');
});

// Covers: 无存储为空；存坏了（非 JSON / 非数组 / 缺字段的条目）当空或被剔掉，不抛。
test('无存储或坏存储读出空列表', () => {
  deepEqual(listRecentObjects(storageOf()), []);
  deepEqual(listRecentObjects(storageOf({ [RECENT_OBJECTS_STORAGE_KEY]: '{not json' })), []);
  deepEqual(listRecentObjects(storageOf({ [RECENT_OBJECTS_STORAGE_KEY]: '{"a":1}' })), []);
  const mixed = JSON.stringify([entry('m', '1'), { moduleId: 'm' }, { moduleId: '', objectId: 'x', title: '', at: '' }, 42]);
  deepEqual(listRecentObjects(storageOf({ [RECENT_OBJECTS_STORAGE_KEY]: mixed })), [entry('m', '1')]);
});

// Covers: 记一条后能读回；同一对象再记一次去重置顶，且标题与时刻取新的一次。
test('记录去重置顶', () => {
  const storage = storageOf();
  recordRecentObject(storage, entry('a', '1', '2026-09-20T10:00:00Z'));
  recordRecentObject(storage, entry('a', '2', '2026-09-20T10:01:00Z'));
  recordRecentObject(storage, entry('b', '1', '2026-09-20T10:02:00Z'));
  deepEqual(
    listRecentObjects(storage).map((item) => `${item.moduleId}/${item.objectId}`),
    ['b/1', 'a/2', 'a/1'],
  );

  const again = recordRecentObject(storage, entry('a', '1', '2026-09-20T10:03:00Z'));
  deepEqual(
    again.map((item) => `${item.moduleId}/${item.objectId}`),
    ['a/1', 'b/1', 'a/2'],
  );
  equal(again[0].at, '2026-09-20T10:03:00Z');
  equal(listRecentObjects(storage).length, 3);
});

// Covers: 不同模块同一标识是两个对象，不互相顶掉。
test('同标识不同模块不算同一对象', () => {
  const storage = storageOf();
  recordRecentObject(storage, entry('a', 'X'));
  recordRecentObject(storage, entry('b', 'X'));
  equal(listRecentObjects(storage).length, 2);
});

// Covers: 上限 50——第 51 条进来时最旧的一条出局；正好 50 条不截。
test('上限 50 从尾部截', () => {
  equal(RECENT_OBJECTS_LIMIT, 50);
  const storage = storageOf();
  for (let i = 1; i <= RECENT_OBJECTS_LIMIT; i += 1) recordRecentObject(storage, entry('m', String(i)));
  const full = listRecentObjects(storage);
  equal(full.length, RECENT_OBJECTS_LIMIT);
  equal(full[full.length - 1].objectId, '1');

  recordRecentObject(storage, entry('m', 'overflow'));
  const list = listRecentObjects(storage);
  equal(list.length, RECENT_OBJECTS_LIMIT);
  equal(list[0].objectId, 'overflow');
  equal(list[list.length - 1].objectId, '2');
});

// Covers: 清空整键移除、读回为空；空存储上清空不抛。
test('清空是唯一删除入口', () => {
  const storage = storageOf();
  recordRecentObject(storage, entry('m', '1'));
  clearRecentObjects(storage);
  deepEqual(storage.dump(), {});
  deepEqual(listRecentObjects(storage), []);
  clearRecentObjects(storage);
});

// Covers: 标题只拼字——模块名 · 标识；模块名查不到时用 id 本身，不编。
test('标题从模块名与标识拼，不发请求', () => {
  equal(recentObjectTitle('委托查阅', 'shipment-request-inquiry', 'SR-1'), '委托查阅 · SR-1');
  equal(recentObjectTitle(undefined, 'gone-module', 'SR-1'), 'gone-module · SR-1');
});

// Covers: 只有带第二段的 hash 算对象地址；模块页、工作台、空 hash 都不算；第一段上的 ?view= 查询串剥掉；第二段解码。
test('从 hash 认对象地址', () => {
  deepEqual(recentObjectFromHash('#/shipment-request-inquiry/SR-1'), {
    moduleId: 'shipment-request-inquiry',
    objectId: 'SR-1',
  });
  deepEqual(recentObjectFromHash('#/shipment-request-inquiry/SR%2F1'), {
    moduleId: 'shipment-request-inquiry',
    objectId: 'SR/1',
  });
  equal(recentObjectFromHash('#/shipment-request-inquiry'), null);
  equal(recentObjectFromHash('#/shipment-request-inquiry?view=v1'), null);
  equal(recentObjectFromHash('#/'), null);
  equal(recentObjectFromHash(''), null);
  equal(recentObjectFromHash('#/m/'), null);
});

// Covers: 对象地址与解析互逆，标识按 URI 组件编码。
test('对象地址与解析互逆', () => {
  const hash = recentObjectHash({ moduleId: 'm', objectId: 'a/b c' });
  equal(hash, '#/m/a%2Fb%20c');
  deepEqual(recentObjectFromHash(hash), { moduleId: 'm', objectId: 'a/b c' });
});

// Covers: 页面过滤——搜索词对标题与标识不分大小写包含；按模块；两者可叠；空词不筛；出现过的模块按最近序列出。
test('过滤与模块列表', () => {
  const list = [
    { ...entry('b', 'SR-9'), title: '委托查阅 · SR-9' },
    { ...entry('a', 'pc-1'), title: '价卡目录 · pc-1' },
    { ...entry('b', 'SR-1'), title: '委托查阅 · SR-1' },
  ];
  equal(filterRecentObjects(list, { keyword: '', moduleId: null }).length, 3);
  deepEqual(
    filterRecentObjects(list, { keyword: 'sr-', moduleId: null }).map((item) => item.objectId),
    ['SR-9', 'SR-1'],
  );
  deepEqual(
    filterRecentObjects(list, { keyword: '价卡', moduleId: null }).map((item) => item.objectId),
    ['pc-1'],
  );
  deepEqual(
    filterRecentObjects(list, { keyword: '', moduleId: 'b' }).map((item) => item.objectId),
    ['SR-9', 'SR-1'],
  );
  deepEqual(
    filterRecentObjects(list, { keyword: '1', moduleId: 'b' }).map((item) => item.objectId),
    ['SR-1'],
  );
  deepEqual(recentModuleIds(list), ['b', 'a']);
});
