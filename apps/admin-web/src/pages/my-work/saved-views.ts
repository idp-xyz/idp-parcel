// 保存视图（票 admin-web-ux-alignment/02 第 3 条）：某张列表页的筛选 / 排序态，存本机、起个名字、下次一键回到。
//
// `state` 是**不透明的 JSON**：各列表页自己决定存什么、怎么读回（票 03 的 Saved View 控件那头消费），本模块不解释它、
// 不校验它的形状——解释权在页面，这里只保管。跳转地址是 `#/<moduleId>?view=<id>`，模块页按 `?view=` 取 id 再来这里查 state。
//
// 隐私边界：存的是本机浏览器的 localStorage（键带产品名，与 shell/preferences.ts 同一前缀约定），换机、换浏览器不带，
// 不上服务端；删除只有 removeSavedView 一个入口，页面上的「删除」就是它。保存视图存的是筛选条件，不是数据——
// 条件里若有人写进了对象标识，那也只是本机的一条书签。
//
// 为什么零依赖：与 recent-objects.ts 同款，不 import 任何 @idpxyz/* 的东西；存储与「现在几点」「新 id」都可注入，
// 测试不碰真 localStorage、不依赖时钟与随机数。

export const SAVED_VIEWS_STORAGE_KEY = 'parcel-admin-web:saved-views';

export interface SavedView {
  id: string;
  /** 导航条目 id（与 navigation.ts 同一套）。 */
  moduleId: string;
  name: string;
  /** 不透明 JSON，归模块页解释。 */
  state: unknown;
  /** 最近一次保存或更新的时刻，RFC 3339（UTC）。 */
  savedAt: string;
  /** 同一模块最多一个默认视图；模块页打开时可拿它当初始筛选态（是否这么做归模块页）。 */
  isDefault: boolean;
}

export interface SavedViewsStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

/** 时钟与 id 生成可注入：生产用 Date 与 crypto.randomUUID，测试给定值。 */
export interface SavedViewsClock {
  now(): string;
  newId(): string;
}

export const systemClock: SavedViewsClock = {
  now: () => new Date().toISOString(),
  newId: () => crypto.randomUUID(),
};

function isSavedView(value: unknown): value is SavedView {
  if (typeof value !== 'object' || value === null) return false;
  const v = value as Record<string, unknown>;
  return (
    typeof v.id === 'string' &&
    v.id !== '' &&
    typeof v.moduleId === 'string' &&
    v.moduleId !== '' &&
    typeof v.name === 'string' &&
    typeof v.savedAt === 'string' &&
    typeof v.isDefault === 'boolean' &&
    'state' in v
  );
}

/** 全部视图，保存时刻新的在前。存坏了当空，不抛。 */
export function listSavedViews(storage: SavedViewsStorage): SavedView[] {
  const raw = storage.getItem(SAVED_VIEWS_STORAGE_KEY);
  if (raw === null) return [];
  try {
    const parsed: unknown = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter(isSavedView) : [];
  } catch {
    return [];
  }
}

export function listSavedViewsFor(storage: SavedViewsStorage, moduleId: string): SavedView[] {
  return listSavedViews(storage).filter((view) => view.moduleId === moduleId);
}

function write(storage: SavedViewsStorage, views: SavedView[]): void {
  storage.setItem(SAVED_VIEWS_STORAGE_KEY, JSON.stringify(views));
}

export interface SaveViewInput {
  moduleId: string;
  name: string;
  state: unknown;
}

/** 新存一条，排到最前；名字留空则不存（返回 null）——没名字的视图在列表里认不出来。 */
export function saveView(
  storage: SavedViewsStorage,
  input: SaveViewInput,
  clock: SavedViewsClock = systemClock,
): SavedView | null {
  const name = input.name.trim();
  if (name === '') return null;
  const view: SavedView = {
    id: clock.newId(),
    moduleId: input.moduleId,
    name,
    state: input.state,
    savedAt: clock.now(),
    isDefault: false,
  };
  write(storage, [view, ...listSavedViews(storage)]);
  return view;
}

/** 改名或换 state（各自可选），时刻刷新并排到最前；id 不存在返回 null。 */
export function updateSavedView(
  storage: SavedViewsStorage,
  id: string,
  patch: Partial<Pick<SavedView, 'name' | 'state'>>,
  clock: SavedViewsClock = systemClock,
): SavedView | null {
  const views = listSavedViews(storage);
  const current = views.find((view) => view.id === id);
  if (!current) return null;
  const name = patch.name !== undefined ? patch.name.trim() : current.name;
  if (name === '') return null;
  const next: SavedView = {
    ...current,
    name,
    state: patch.state !== undefined ? patch.state : current.state,
    savedAt: clock.now(),
  };
  write(storage, [next, ...views.filter((view) => view.id !== id)]);
  return next;
}

/** 唯一删除入口；返回是否真删了一条。 */
export function removeSavedView(storage: SavedViewsStorage, id: string): boolean {
  const views = listSavedViews(storage);
  const rest = views.filter((view) => view.id !== id);
  if (rest.length === views.length) return false;
  if (rest.length === 0) storage.removeItem(SAVED_VIEWS_STORAGE_KEY);
  else write(storage, rest);
  return true;
}

/**
 * 设为该模块的默认视图：同一模块的其他视图默认标记全部撤下，别的模块不受影响；顺序不动。
 * id 不存在返回 false、不写。
 */
export function setDefaultSavedView(storage: SavedViewsStorage, id: string): boolean {
  const views = listSavedViews(storage);
  const target = views.find((view) => view.id === id);
  if (!target) return false;
  write(
    storage,
    views.map((view) =>
      view.moduleId === target.moduleId ? { ...view, isDefault: view.id === id } : view,
    ),
  );
  return true;
}

/** 该模块的默认视图；没有返回 null。 */
export function defaultSavedViewFor(storage: SavedViewsStorage, moduleId: string): SavedView | null {
  return listSavedViewsFor(storage, moduleId).find((view) => view.isDefault) ?? null;
}

/** 跳转地址：模块页按 `?view=` 取 id。查询串挂在第一段上，外壳认模块时要先把它剥掉。 */
export function savedViewHash(view: Pick<SavedView, 'moduleId' | 'id'>): string {
  return `#/${view.moduleId}?view=${encodeURIComponent(view.id)}`;
}

/** 从 hash 里取 `?view=` 的 id；没有返回 null。给模块页用，本票只提供、不消费。 */
export function savedViewIdFromHash(hash: string): string | null {
  const query = hash.split('?')[1];
  if (!query) return null;
  const id = new URLSearchParams(query).get('view');
  return id ? id : null;
}

export interface SavedViewsFilter {
  keyword: string;
  moduleId: string | null;
}

/** 页面侧对已读出列表的便利过滤（名字包含匹配 + 按模块），不改存储。 */
export function filterSavedViews(list: SavedView[], filter: SavedViewsFilter): SavedView[] {
  const needle = filter.keyword.trim().toLowerCase();
  return list.filter((view) => {
    if (filter.moduleId !== null && view.moduleId !== filter.moduleId) return false;
    return needle === '' || view.name.toLowerCase().includes(needle);
  });
}

/** 列表里出现过的模块 id，按首次出现排序——给筛选 chip 组用。 */
export function savedViewModuleIds(list: SavedView[]): string[] {
  const seen: string[] = [];
  for (const view of list) if (!seen.includes(view.moduleId)) seen.push(view.moduleId);
  return seen;
}
