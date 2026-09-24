// 最近对象（票 admin-web-ux-alignment/02 第 2 条）：操作者在本机浏览器里打开过的对象地址的历史。
//
// 它是 UI 自身的事实——「你在这台机器上点开过哪些地址」——不是业务数据：对象地址就是 hash 二段路由
// `#/<moduleId>/<objectId>`，外壳每次落到带第二段的地址就记一条；对象名称属业务数据，这里**不发请求取名**，
// 标题只从模块名与对象标识拼（列表页打开时自己知道名字、进详情页再显）。
//
// 隐私边界：存的是本机浏览器的 localStorage（键带产品名，与 shell/preferences.ts 同一前缀约定），换机、换浏览器不带，
// 不上服务端、不进任何读口；`clearRecentObjects` 是唯一的删除入口，页面上「清空历史」就是它，没有逐条删——
// 一条历史不是一个对象，删一条并不让那个对象消失，只会让人以为它消失了。
//
// 为什么零依赖：与 shell/preferences.ts 同款，不 import 任何 @idpxyz/* 的东西（只引同样零依赖的 templates/address-query.ts），
// 测试编成 CommonJS 后能直接 require；存储经 RecentObjectsStorage 注入，测试用 Map 顶替、不碰真 localStorage。

import { decodeHashSegment } from '../../templates/address-query';

export const RECENT_OBJECTS_STORAGE_KEY = 'parcel-admin-web:recent-objects';

/** 上限：再多也翻不到，而 localStorage 是别的产品也在用的公共地方。 */
export const RECENT_OBJECTS_LIMIT = 50;

export interface RecentObject {
  /** 导航条目 id（与 navigation.ts 同一套）。 */
  moduleId: string;
  /** hash 第二段解码后的对象标识；语义归那个模块页。 */
  objectId: string;
  /** 展示用标题，见 recentObjectTitle；不是对象名称。 */
  title: string;
  /** 上次打开时刻，RFC 3339（UTC）。 */
  at: string;
}

/** localStorage 的最小子集；测试用 Map 顶替，生产传 window.localStorage。 */
export interface RecentObjectsStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

function isRecentObject(value: unknown): value is RecentObject {
  if (typeof value !== 'object' || value === null) return false;
  const v = value as Record<string, unknown>;
  return (
    typeof v.moduleId === 'string' &&
    v.moduleId !== '' &&
    typeof v.objectId === 'string' &&
    v.objectId !== '' &&
    typeof v.title === 'string' &&
    typeof v.at === 'string'
  );
}

/** 最近的在前。存坏了（手改、别的版本写的）当空，不抛：一条坏历史不该把工作台卡住。 */
export function listRecentObjects(storage: RecentObjectsStorage): RecentObject[] {
  const raw = storage.getItem(RECENT_OBJECTS_STORAGE_KEY);
  if (raw === null) return [];
  try {
    const parsed: unknown = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter(isRecentObject) : [];
  } catch {
    return [];
  }
}

function sameObject(a: Pick<RecentObject, 'moduleId' | 'objectId'>, b: Pick<RecentObject, 'moduleId' | 'objectId'>): boolean {
  return a.moduleId === b.moduleId && a.objectId === b.objectId;
}

/**
 * 记一次打开：同一对象（模块 + 标识）去重置顶，标题与时刻取这一次的；超过上限从尾部截。返回写入后的列表。
 */
export function recordRecentObject(storage: RecentObjectsStorage, entry: RecentObject): RecentObject[] {
  const rest = listRecentObjects(storage).filter((item) => !sameObject(item, entry));
  const next = [entry, ...rest].slice(0, RECENT_OBJECTS_LIMIT);
  storage.setItem(RECENT_OBJECTS_STORAGE_KEY, JSON.stringify(next));
  return next;
}

/** 唯一删除入口：整键移除，不留空数组。 */
export function clearRecentObjects(storage: RecentObjectsStorage): void {
  storage.removeItem(RECENT_OBJECTS_STORAGE_KEY);
}

/** 标题 = 模块名 · 对象标识；模块名查不到（导航词表已改）就用 id 本身，不编名字。 */
export function recentObjectTitle(moduleTitle: string | undefined, moduleId: string, objectId: string): string {
  return `${moduleTitle ?? moduleId} · ${objectId}`;
}

/**
 * 从地址栏 hash 认出「对象地址」：`#/<moduleId>/<objectId>[?…]` 才算，只有一段的是模块页、不记；
 * 第一段上可能挂着 `?view=` 之类的查询串（保存视图跳转用），归模块页读，这里剥掉不认。解不开的段（畸形百分号）也不认。
 */
export function recentObjectFromHash(hash: string): { moduleId: string; objectId: string } | null {
  const path = hash.replace(/^#\/?/, '').split('?')[0];
  const [first, second] = path.split('/');
  if (!first || !second) return null;
  const moduleId = decodeHashSegment(first);
  const objectId = decodeHashSegment(second);
  return moduleId === null || objectId === null ? null : { moduleId, objectId };
}

/** 对象地址：与各模块页写 hash 的形一致，标识按 URI 组件编码。 */
export function recentObjectHash(entry: Pick<RecentObject, 'moduleId' | 'objectId'>): string {
  return `#/${entry.moduleId}/${encodeURIComponent(entry.objectId)}`;
}

export interface RecentObjectsFilter {
  /** 搜索词：对标题与对象标识做大小写不敏感的包含匹配；空串不筛。 */
  keyword: string;
  /** 只看某模块；null 不筛。 */
  moduleId: string | null;
}

/** 页面侧对已读出列表的便利过滤，不改存储。 */
export function filterRecentObjects(list: RecentObject[], filter: RecentObjectsFilter): RecentObject[] {
  const needle = filter.keyword.trim().toLowerCase();
  return list.filter((item) => {
    if (filter.moduleId !== null && item.moduleId !== filter.moduleId) return false;
    if (needle === '') return true;
    return item.title.toLowerCase().includes(needle) || item.objectId.toLowerCase().includes(needle);
  });
}

/** 列表里出现过的模块 id，按首次出现（即最近）排序——给筛选 chip 组用，不列没历史的模块。 */
export function recentModuleIds(list: RecentObject[]): string[] {
  const seen: string[] = [];
  for (const item of list) if (!seen.includes(item.moduleId)) seen.push(item.moduleId);
  return seen;
}
