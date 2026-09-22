// 工作区状态的纯逻辑（票 admin-web-workspace-form/01 第 1 条）：多标签壳层的标签集、活动标签、已关闭栈与侧栏宽度，
// 以及它们与地址栏 hash 之间的换算。渲染归 Layout 与 @idpxyz/ui-workspace 的 EditorGroup，这里只管「状态怎么变」。
//
// 为什么单独一个 .ts：每个操作的正向与边界（关活动标签落到哪个邻居、固定的不被批量关、已关闭栈上限、坏存储回默认）
// 都是与 React 无关的事实，组件层在 node:test 里钉不到（理由见 templates/loading-shape.ts 文件头），抬到这里钉。
// 零依赖——不 import 任何 @idpxyz/* 的运行时也不 import navigation.ts（那里带着 lucide 图标），测试编成 CommonJS 后能直接
// require；导航词表由调用方以 pageTitleById 注入，与 shell/command-actions.ts 同款。
//
// 参照 idp-ui@6751fb2 apps/myshop-web/src/hooks/useWorkspaceState.ts 与本仓 vendor 的 ui-workspace useEditorGroupTabState：
// 固定的标签排在前、批量关闭放过固定的、已关闭栈上限 20、load 逐字段校验坏值回默认。两处刻意不同：
//   - **工作台不是标签，是「没有活动标签」那一格。** myshop-web 的工作台是一张常驻标签；本仓若照搬，EditorGroup 会在它上面
//     照样画一个 ×，按下去什么都不发生——spec 红线不允许假动作。这里把工作台做成 EditorGroup 的 emptyStateContent：
//     activeTabId 为 null 即显示工作台，关掉最后一张标签自然落回它，没有一个按不动的按钮。
//   - **hash 仍是位置权威**（Layout.tsx 文件头）：标签 id 就是 hash 路径，点标签写 hash、hashchange 再回流成 openTab；
//     本模块不写 window.location，也不在标签里另存页内状态。

export const WORKSPACE_STORAGE_KEY = 'parcel-admin-web:workspace';

/** 工作台的模块 id；`#/workbench`、空 hash 与词表外的 id 都落到它，此时没有活动标签。 */
export const WORKBENCH_MODULE_ID = 'workbench';

/** 已关闭栈上限，沿 myshop-web：再多也不会有人一路 Ctrl+Shift+T 翻回去。 */
export const CLOSED_TABS_LIMIT = 20;

/** 侧栏宽度三值与 Layout 此前写死给 useResize 的同值；抬到这里让 load 的区间校验与拖拽的钳位用同一组数。 */
export const SIDEBAR_WIDTH_DEFAULT = 240;
export const SIDEBAR_WIDTH_MIN = 170;
export const SIDEBAR_WIDTH_MAX = 500;

/** 与 ui-workspace 的 Tab 同形的子集；不 import 它的类型是为了零依赖，Layout 把它原样交给 EditorGroup。 */
export interface WorkspaceTab {
  /** 即 hash 路径 `<moduleId>` 或 `<moduleId>/<objectId>`（查询串已剥），见 tabIdFromHash。 */
  id: string;
  /** 模块名（pageTitleById 里的词）；对象标签也用模块名，对象标识落 subtitle。 */
  name: string;
  /** 对象标签的对象标识（解码后）；模块标签没有。 */
  subtitle?: string;
  pinned?: boolean;
}

export interface WorkspaceState {
  tabs: WorkspaceTab[];
  /** null = 工作台（没有活动标签）。 */
  activeTabId: string | null;
  /** 最近关闭的在前。 */
  closedTabs: WorkspaceTab[];
  sidebarWidth: number;
}

/** localStorage 的最小子集；测试用 Map 顶替，生产传 window.localStorage。 */
export interface WorkspaceStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

export function initialWorkspaceState(): WorkspaceState {
  return { tabs: [], activeTabId: null, closedTabs: [], sidebarWidth: SIDEBAR_WIDTH_DEFAULT };
}

// —— hash ↔ 标签 ——

/**
 * 从 hash 认出标签 id：`#/<moduleId>[/<objectId>][/…][?…]` → `<moduleId>` 或 `<moduleId>/<objectId>`。
 * 只取前两段——第三段起归页面自己（今天没有页用到，用到时它是页内位置不是另一张标签）；查询串（保存视图的 `?view=`）
 * 归模块页读，剥掉不进 id。段保持 hash 里的原样（不解码）：id 要能原样写回 hash，解码再编码不保证字节相同。
 * 词表外的模块 id 与空 hash 都答 null——落工作台，与 Layout 此前对未知 id 的处置一致。
 */
export function tabIdFromHash(hash: string, isKnownModule: (moduleId: string) => boolean): string | null {
  const path = hash.replace(/^#\/?/, '').split('?')[0];
  const [first, second] = path.split('/');
  if (!first) return null;
  const moduleId = decodeURIComponent(first);
  if (moduleId === WORKBENCH_MODULE_ID || !isKnownModule(moduleId)) return null;
  return second ? `${first}/${second}` : first;
}

export function moduleIdOfTab(tabId: string): string {
  return decodeURIComponent(tabId.split('/')[0]);
}

export function objectIdOfTab(tabId: string): string | null {
  const second = tabId.split('/')[1];
  return second ? decodeURIComponent(second) : null;
}

/**
 * 由 hash 造出要打开的标签；name 取模块名、subtitle 取对象标识——只拼字不取名，与 pages/my-work/recent-objects.ts 的
 * recentObjectTitle 同一态度（对象名称属业务数据，前端不为补名字发请求）。落工作台时答 null。
 */
export function tabForHash(hash: string, pageTitleById: Record<string, string>): WorkspaceTab | null {
  const id = tabIdFromHash(hash, (moduleId) => pageTitleById[moduleId] !== undefined);
  if (id === null) return null;
  const objectId = objectIdOfTab(id);
  const tab: WorkspaceTab = { id, name: pageTitleById[moduleIdOfTab(id)] };
  if (objectId !== null) tab.subtitle = objectId;
  return tab;
}

/** 标签的地址：`#/<id>`；null（工作台）写 `#/workbench`，与侧栏点「工作台」写的同一形。 */
export function hashForTab(tabId: string | null): string {
  return `#/${tabId ?? WORKBENCH_MODULE_ID}`;
}

// —— 标签操作 ——

function withoutClosed(closedTabs: WorkspaceTab[], id: string): WorkspaceTab[] {
  return closedTabs.filter((tab) => tab.id !== id);
}

/** 关掉的进栈时去掉固定标记：重开是一张新标签，不该带着旧的固定态回来（vendor useEditorGroupTabState 同此）。 */
function pushClosed(closedTabs: WorkspaceTab[], closed: WorkspaceTab[]): WorkspaceTab[] {
  const incoming = closed.map(({ pinned: _pinned, ...tab }) => tab);
  const rest = closedTabs.filter((tab) => !incoming.some((c) => c.id === tab.id));
  return [...incoming, ...rest].slice(0, CLOSED_TABS_LIMIT);
}

/** 同 id 已开 → 只激活（名字与副标题按这一次的地址刷新）；否则追加并激活。已关闭栈里的同 id 顺手移除。 */
export function openTab(state: WorkspaceState, tab: WorkspaceTab): WorkspaceState {
  const existing = state.tabs.find((t) => t.id === tab.id);
  const tabs = existing
    ? state.tabs.map((t) => (t.id === tab.id ? { ...t, name: tab.name, subtitle: tab.subtitle } : t))
    : [...state.tabs, tab];
  return { ...state, tabs, activeTabId: tab.id, closedTabs: withoutClosed(state.closedTabs, tab.id) };
}

/** 点标签：只认已开的 id；null 即回工作台。 */
export function activateTab(state: WorkspaceState, id: string | null): WorkspaceState {
  if (id !== null && !state.tabs.some((t) => t.id === id)) return state;
  return { ...state, activeTabId: id };
}

/**
 * 关一张：活动标签被关时激活右邻，没有右邻就左邻，都没有落工作台（null）。不认识的 id 原样返回。
 * 固定的也能被直接关——EditorGroup 在固定标签上照样画 ×，这里若拒绝，那个 × 就成了假动作。
 */
export function closeTab(state: WorkspaceState, id: string): WorkspaceState {
  const index = state.tabs.findIndex((t) => t.id === id);
  if (index < 0) return state;
  const closed = state.tabs[index];
  const tabs = state.tabs.filter((t) => t.id !== id);
  let activeTabId = state.activeTabId;
  if (activeTabId === id) {
    const neighbour = tabs[index] ?? tabs[index - 1];
    activeTabId = neighbour ? neighbour.id : null;
  }
  return { ...state, tabs, activeTabId, closedTabs: pushClosed(state.closedTabs, [closed]) };
}

/** 固定 / 取消固定；固定的排到前面（稳定排序，固定标签之间保持原序）。 */
export function togglePinned(state: WorkspaceState, id: string): WorkspaceState {
  if (!state.tabs.some((t) => t.id === id)) return state;
  const toggled = state.tabs.map((t) => (t.id === id ? { ...t, pinned: !t.pinned } : t));
  const tabs = [...toggled.filter((t) => t.pinned), ...toggled.filter((t) => !t.pinned)];
  return { ...state, tabs };
}

function closeMany(state: WorkspaceState, shouldClose: (tab: WorkspaceTab, index: number) => boolean): WorkspaceState {
  const closed = state.tabs.filter((tab, index) => shouldClose(tab, index) && !tab.pinned);
  if (closed.length === 0) return state;
  const tabs = state.tabs.filter((tab) => !closed.includes(tab));
  return { ...state, tabs, closedTabs: pushClosed(state.closedTabs, closed) };
}

/** 关其它：留下 keepId 与固定的，活动标签落到 keepId。 */
export function closeOthers(state: WorkspaceState, keepId: string): WorkspaceState {
  if (!state.tabs.some((t) => t.id === keepId)) return state;
  const next = closeMany(state, (tab) => tab.id !== keepId);
  return { ...next, activeTabId: keepId };
}

/** 关右侧：anchorId 之右的非固定标签全关；活动标签若在其中，落到 anchorId。 */
export function closeToRight(state: WorkspaceState, anchorId: string): WorkspaceState {
  const anchor = state.tabs.findIndex((t) => t.id === anchorId);
  if (anchor < 0) return state;
  const next = closeMany(state, (_tab, index) => index > anchor);
  const activeSurvives = next.tabs.some((t) => t.id === next.activeTabId);
  return { ...next, activeTabId: activeSurvives ? next.activeTabId : anchorId };
}

/** 全关：固定的留下；活动标签若被关，落工作台（null）——固定的留着不等于要看它。 */
export function closeAll(state: WorkspaceState): WorkspaceState {
  const next = closeMany(state, () => true);
  const activeSurvives = next.tabs.some((t) => t.id === next.activeTabId);
  return { ...next, activeTabId: activeSurvives ? next.activeTabId : null };
}

/** 重开最近关闭的一张：栈顶出栈；它若已被别的路径重新打开，只激活不重复追加。栈空原样返回。 */
export function reopenClosed(state: WorkspaceState): WorkspaceState {
  const [top, ...rest] = state.closedTabs;
  if (!top) return state;
  const reopened = openTab({ ...state, closedTabs: rest }, top);
  return reopened;
}

/**
 * 拖排：`to` 是 EditorGroup 报的落点（被悬停那张的下标，或尾部落点 tabs.length），取出一张后直接插在 `to`——
 * 与 vendor useEditorGroupTabState 同一算法，往右拖落在悬停那张之后、往左拖落在其之前；别的 idp 工作台也这么动，
 * 这里不另起一套手感。越界的下标原样返回，不抛——拖放坐标来自 DOM，这里得自己稳。
 */
export function reorderTabs(state: WorkspaceState, from: number, to: number): WorkspaceState {
  const n = state.tabs.length;
  if (from < 0 || from >= n || to < 0 || to > n || from === to) return state;
  const tabs = [...state.tabs];
  const [moved] = tabs.splice(from, 1);
  tabs.splice(to, 0, moved);
  return { ...state, tabs };
}

export function setSidebarWidth(state: WorkspaceState, width: number): WorkspaceState {
  return { ...state, sidebarWidth: clampWidth(width, SIDEBAR_WIDTH_MIN, SIDEBAR_WIDTH_MAX, SIDEBAR_WIDTH_DEFAULT) };
}

// —— 持久化 ——

function clampWidth(value: unknown, min: number, max: number, fallback: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback;
  return Math.min(max, Math.max(min, value));
}

function isWorkspaceTab(value: unknown): value is WorkspaceTab {
  if (typeof value !== 'object' || value === null) return false;
  const v = value as Record<string, unknown>;
  return (
    typeof v.id === 'string' &&
    v.id !== '' &&
    typeof v.name === 'string' &&
    (v.subtitle === undefined || typeof v.subtitle === 'string') &&
    (v.pinned === undefined || typeof v.pinned === 'boolean')
  );
}

/** 只保留形对的、id 不重复的、模块在词表内的标签——词表已改的模块留在标签栏上会点进 UnwiredModule。 */
function sanitizeTabs(value: unknown, isKnownModule: (moduleId: string) => boolean): WorkspaceTab[] {
  if (!Array.isArray(value)) return [];
  const seen = new Set<string>();
  const tabs: WorkspaceTab[] = [];
  for (const item of value) {
    if (!isWorkspaceTab(item) || seen.has(item.id)) continue;
    if (moduleIdOfTab(item.id) === WORKBENCH_MODULE_ID || !isKnownModule(moduleIdOfTab(item.id))) continue;
    seen.add(item.id);
    const tab: WorkspaceTab = { id: item.id, name: item.name };
    if (item.subtitle !== undefined) tab.subtitle = item.subtitle;
    if (item.pinned) tab.pinned = true;
    tabs.push(tab);
  }
  return tabs;
}

/**
 * 读回上次的工作区：逐字段校验，坏的那一格回默认而不是整份丢——存坏了半格不该把所有标签都吞掉。
 * 存不下来、JSON 不合法当没存。活动标签不在标签集里时落工作台。
 */
export function loadWorkspaceState(
  storage: WorkspaceStorage,
  isKnownModule: (moduleId: string) => boolean,
): WorkspaceState {
  const raw = storage.getItem(WORKSPACE_STORAGE_KEY);
  if (raw === null) return initialWorkspaceState();
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return initialWorkspaceState();
  }
  if (typeof parsed !== 'object' || parsed === null) return initialWorkspaceState();
  const v = parsed as Record<string, unknown>;
  const tabs = sanitizeTabs(v.tabs, isKnownModule);
  const activeTabId =
    typeof v.activeTabId === 'string' && tabs.some((t) => t.id === v.activeTabId) ? v.activeTabId : null;
  const closedTabs = sanitizeTabs(v.closedTabs, isKnownModule)
    .filter((tab) => !tabs.some((t) => t.id === tab.id))
    .map(({ pinned: _pinned, ...tab }) => tab)
    .slice(0, CLOSED_TABS_LIMIT);
  return {
    tabs,
    activeTabId,
    closedTabs,
    sidebarWidth: clampWidth(v.sidebarWidth, SIDEBAR_WIDTH_MIN, SIDEBAR_WIDTH_MAX, SIDEBAR_WIDTH_DEFAULT),
  };
}

export function saveWorkspaceState(storage: WorkspaceStorage, state: WorkspaceState): void {
  storage.setItem(WORKSPACE_STORAGE_KEY, JSON.stringify(state));
}

// —— 快捷键 ——

export type WorkspaceShortcut = 'close-active-tab' | 'reopen-closed-tab';

/**
 * 壳层认的两个标签快捷键，沿 myshop-web：Ctrl/⌘+W 关活动标签、Ctrl/⌘+Shift+T 重开。Alt 一并按下不认（与
 * shell/command-actions.ts 的 Ctrl+K 同一理由）。判定归这里、要不要 preventDefault 归 Layout——工作台上没有可关的
 * 标签时不拦浏览器自己的 Ctrl+W，别把关浏览器标签页的快捷键吞了却什么都不做。
 */
export function workspaceShortcutOf(event: {
  key: string;
  ctrlKey: boolean;
  metaKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
}): WorkspaceShortcut | null {
  if (!(event.ctrlKey || event.metaKey) || event.altKey) return null;
  const key = event.key.toLowerCase();
  if (key === 'w' && !event.shiftKey) return 'close-active-tab';
  if (key === 't' && event.shiftKey) return 'reopen-closed-tab';
  return null;
}
