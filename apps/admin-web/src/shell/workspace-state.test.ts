import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  CLOSED_TABS_LIMIT,
  INSPECTOR_WIDTH_DEFAULT,
  INSPECTOR_WIDTH_MAX,
  INSPECTOR_WIDTH_MIN,
  SIDEBAR_WIDTH_DEFAULT,
  SIDEBAR_WIDTH_MAX,
  SIDEBAR_WIDTH_MIN,
  WORKSPACE_STORAGE_KEY,
  activateTab,
  closeAll,
  closeOthers,
  closeTab,
  closeToRight,
  hashForTab,
  initialWorkspaceState,
  loadWorkspaceState,
  moduleIdOfTab,
  objectIdOfTab,
  openTab,
  reopenClosed,
  reorderTabs,
  saveWorkspaceState,
  setInspectorVisible,
  setInspectorWidth,
  setSidebarWidth,
  tabForHash,
  tabIdFromHash,
  themeWord,
  togglePinned,
  workspaceLocationLabel,
  workspaceShortcutOf,
  type WorkspaceState,
  type WorkspaceTab,
} from './workspace-state';

// 本文件钉的是工作区状态的操作事实（票 admin-web-workspace-form/01 第 1 条）：每个操作的正向 + 至少一条边界，
// 与 hash ↔ 标签的换算。EditorGroup 接线那半（点标签写 hash、hashchange 回流）在 node:test 里钉不到，
// 用 scripts/dom-probe.mjs 一次性实测，结论写票面。

const pageTitleById: Record<string, string> = {
  workbench: '工作台',
  'shipment-request-inquiry': '委托查阅',
  'exception-cases': '异常案件',
  'price-cards': '价卡目录',
};
const known = (id: string) => pageTitleById[id] !== undefined;

const tab = (id: string, extra: Partial<WorkspaceTab> = {}): WorkspaceTab => ({
  id,
  name: pageTitleById[moduleIdOfTab(id)] ?? id,
  ...extra,
});

function stateWith(tabs: WorkspaceTab[], activeTabId: string | null = tabs[tabs.length - 1]?.id ?? null): WorkspaceState {
  return { ...initialWorkspaceState(), tabs, activeTabId };
}

const ids = (state: WorkspaceState) => state.tabs.map((t) => t.id);

class MapStorage {
  private readonly map = new Map<string, string>();
  getItem(key: string) {
    return this.map.get(key) ?? null;
  }
  setItem(key: string, value: string) {
    this.map.set(key, value);
  }
}

// —— hash ↔ 标签 ——

test('tabIdFromHash：模块页取一段，对象地址取两段，第三段与查询串都不进 id', () => {
  equal(tabIdFromHash('#/exception-cases', known), 'exception-cases');
  equal(tabIdFromHash('#/shipment-request-inquiry/SR-1', known), 'shipment-request-inquiry/SR-1');
  equal(tabIdFromHash('#/shipment-request-inquiry/SR-1/timeline', known), 'shipment-request-inquiry/SR-1');
  equal(tabIdFromHash('#/exception-cases?view=v1', known), 'exception-cases');
});

test('tabIdFromHash：空 hash、#/workbench 与词表外的模块都落工作台（null）', () => {
  equal(tabIdFromHash('', known), null);
  equal(tabIdFromHash('#/', known), null);
  equal(tabIdFromHash('#/workbench', known), null);
  equal(tabIdFromHash('#/no-such-module/x', known), null);
});

test('tabIdFromHash：段保持 hash 原样不解码，写回 hash 字节相同', () => {
  const id = tabIdFromHash('#/shipment-request-inquiry/SR%2F1', known);
  equal(id, 'shipment-request-inquiry/SR%2F1');
  equal(hashForTab(id), '#/shipment-request-inquiry/SR%2F1');
  equal(objectIdOfTab(id!), 'SR/1');
});

test('tabForHash：名字取模块名、对象标识落 subtitle；模块标签没有 subtitle', () => {
  deepEqual(tabForHash('#/shipment-request-inquiry/SR-1', pageTitleById), {
    id: 'shipment-request-inquiry/SR-1',
    name: '委托查阅',
    subtitle: 'SR-1',
  });
  deepEqual(tabForHash('#/exception-cases', pageTitleById), { id: 'exception-cases', name: '异常案件' });
  equal(tabForHash('#/workbench', pageTitleById), null);
});

test('hashForTab：null 写 #/workbench，与侧栏点工作台同一形', () => {
  equal(hashForTab(null), '#/workbench');
  equal(hashForTab('exception-cases'), '#/exception-cases');
});

// —— openTab / activateTab ——

test('openTab：新 id 追加并激活；同 id 只激活并按这次的地址刷新名字', () => {
  let state = openTab(initialWorkspaceState(), tab('exception-cases'));
  state = openTab(state, tab('shipment-request-inquiry/SR-1', { subtitle: 'SR-1' }));
  deepEqual(ids(state), ['exception-cases', 'shipment-request-inquiry/SR-1']);
  equal(state.activeTabId, 'shipment-request-inquiry/SR-1');

  state = openTab(state, { id: 'exception-cases', name: '异常案件（改名后）' });
  deepEqual(ids(state), ['exception-cases', 'shipment-request-inquiry/SR-1']);
  equal(state.activeTabId, 'exception-cases');
  equal(state.tabs[0].name, '异常案件（改名后）');
});

test('openTab：已关闭栈里的同 id 随重新打开移除', () => {
  let state = closeTab(stateWith([tab('exception-cases')]), 'exception-cases');
  equal(state.closedTabs.length, 1);
  state = openTab(state, tab('exception-cases'));
  deepEqual(state.closedTabs, []);
});

test('activateTab：只认已开的 id；null 回工作台', () => {
  const state = stateWith([tab('exception-cases'), tab('price-cards')]);
  equal(activateTab(state, 'exception-cases').activeTabId, 'exception-cases');
  equal(activateTab(state, 'not-open').activeTabId, 'price-cards');
  equal(activateTab(state, null).activeTabId, null);
});

// —— closeTab 三种邻接 ——

test('closeTab：关活动标签激活右邻', () => {
  const state = closeTab(stateWith([tab('a'), tab('b'), tab('c')], 'b'), 'b');
  deepEqual(ids(state), ['a', 'c']);
  equal(state.activeTabId, 'c');
});

test('closeTab：没有右邻激活左邻', () => {
  const state = closeTab(stateWith([tab('a'), tab('b'), tab('c')], 'c'), 'c');
  equal(state.activeTabId, 'b');
});

test('closeTab：关最后一张落工作台（null），标签集为空', () => {
  const state = closeTab(stateWith([tab('a')], 'a'), 'a');
  deepEqual(ids(state), []);
  equal(state.activeTabId, null);
});

test('closeTab：关非活动标签不改活动标签；不认识的 id 原样返回', () => {
  const state = stateWith([tab('a'), tab('b')], 'b');
  equal(closeTab(state, 'a').activeTabId, 'b');
  equal(closeTab(state, 'zzz'), state);
});

test('closeTab：关掉的进已关闭栈、去掉固定标记、最近的在前、上限 ' + CLOSED_TABS_LIMIT, () => {
  let state = stateWith([tab('a', { pinned: true }), tab('b')], 'b');
  state = closeTab(state, 'a');
  deepEqual(state.closedTabs, [{ id: 'a', name: 'a' }]);
  state = closeTab(state, 'b');
  deepEqual(state.closedTabs.map((t) => t.id), ['b', 'a']);

  let many = stateWith(Array.from({ length: CLOSED_TABS_LIMIT + 5 }, (_, i) => tab(`t${i}`)));
  for (let i = 0; i < CLOSED_TABS_LIMIT + 5; i++) many = closeTab(many, `t${i}`);
  equal(many.closedTabs.length, CLOSED_TABS_LIMIT);
  equal(many.closedTabs[0].id, `t${CLOSED_TABS_LIMIT + 4}`);
});

// —— 固定与批量关闭 ——

test('togglePinned：固定的排到前面，再切一次取消；不认识的 id 原样返回', () => {
  let state = stateWith([tab('a'), tab('b'), tab('c')]);
  state = togglePinned(state, 'c');
  deepEqual(ids(state), ['c', 'a', 'b']);
  equal(state.tabs[0].pinned, true);
  state = togglePinned(state, 'c');
  equal(state.tabs[0].pinned, false);
  equal(togglePinned(state, 'zzz'), state);
});

test('closeOthers：留下自己与固定的，活动标签落到自己', () => {
  const state = closeOthers(stateWith([tab('a', { pinned: true }), tab('b'), tab('c'), tab('d')], 'd'), 'c');
  deepEqual(ids(state), ['a', 'c']);
  equal(state.activeTabId, 'c');
  deepEqual(state.closedTabs.map((t) => t.id), ['b', 'd']);
});

test('closeToRight：只关右侧的非固定标签；活动标签在其中时落到锚点', () => {
  const state = closeToRight(stateWith([tab('a'), tab('b'), tab('c', { pinned: true }), tab('d')], 'd'), 'a');
  deepEqual(ids(state), ['a', 'c']);
  equal(state.activeTabId, 'a');
});

test('closeToRight：活动标签在左侧不动', () => {
  const state = closeToRight(stateWith([tab('a'), tab('b'), tab('c')], 'a'), 'b');
  deepEqual(ids(state), ['a', 'b']);
  equal(state.activeTabId, 'a');
});

test('closeOthers / closeToRight：不认识的 id 原样返回', () => {
  const state = stateWith([tab('a'), tab('b')], 'b');
  equal(closeOthers(state, 'zzz'), state);
  equal(closeToRight(state, 'zzz'), state);
});

test('closeAll：固定的留下；活动标签被关时落工作台，活动标签是固定的时不动', () => {
  const closed = closeAll(stateWith([tab('a', { pinned: true }), tab('b')], 'b'));
  deepEqual(ids(closed), ['a']);
  equal(closed.activeTabId, null);

  const kept = closeAll(stateWith([tab('a', { pinned: true }), tab('b')], 'a'));
  equal(kept.activeTabId, 'a');
});

// —— 重开 ——

test('reopenClosed：栈顶出栈追加并激活；已被别处重开的只激活不重复；栈空原样返回', () => {
  let state = stateWith([tab('a'), tab('b')], 'b');
  state = closeTab(state, 'a');
  state = closeTab(state, 'b');
  const empty = initialWorkspaceState();
  equal(reopenClosed(empty), empty, '空栈不该改状态');

  state = reopenClosed(state);
  deepEqual(ids(state), ['b']);
  equal(state.activeTabId, 'b');
  deepEqual(state.closedTabs.map((t) => t.id), ['a']);

  state = openTab(state, tab('a'));
  deepEqual(state.closedTabs, []);
  const again = reopenClosed(state);
  equal(again, state);
});

// —— 拖排 ——

test('reorderTabs：与 vendor 同一算法，越界原样返回', () => {
  const state = stateWith([tab('a'), tab('b'), tab('c')]);
  deepEqual(ids(reorderTabs(state, 0, 2)), ['b', 'c', 'a']);
  deepEqual(ids(reorderTabs(state, 2, 0)), ['c', 'a', 'b']);
  deepEqual(ids(reorderTabs(state, 0, 3)), ['b', 'c', 'a']);
  equal(reorderTabs(state, 0, 4), state);
  equal(reorderTabs(state, -1, 0), state);
  equal(reorderTabs(state, 1, 1), state);
});

// —— 侧栏宽度 ——

test('setSidebarWidth：钳进区间；非数回默认', () => {
  equal(setSidebarWidth(initialWorkspaceState(), 9999).sidebarWidth, SIDEBAR_WIDTH_MAX);
  equal(setSidebarWidth(initialWorkspaceState(), 1).sidebarWidth, SIDEBAR_WIDTH_MIN);
  equal(setSidebarWidth(initialWorkspaceState(), Number.NaN).sidebarWidth, SIDEBAR_WIDTH_DEFAULT);
});

// —— 检查器栏 ——

test('检查器栏：默认可见且宽度取默认；setInspectorWidth 钳进区间；setInspectorVisible 同值不换对象', () => {
  const state = initialWorkspaceState();
  equal(state.inspectorVisible, true);
  equal(state.inspectorWidth, INSPECTOR_WIDTH_DEFAULT);
  equal(setInspectorWidth(state, 9999).inspectorWidth, INSPECTOR_WIDTH_MAX);
  equal(setInspectorWidth(state, 1).inspectorWidth, INSPECTOR_WIDTH_MIN);
  equal(setInspectorWidth(state, Number.NaN).inspectorWidth, INSPECTOR_WIDTH_DEFAULT);
  equal(setInspectorVisible(state, true), state);
  equal(setInspectorVisible(state, false).inspectorVisible, false);
});

test('load：检查器宽度越界钳进区间，可见性非布尔当没存（可见）', () => {
  const storage = new MapStorage();
  storage.setItem(
    WORKSPACE_STORAGE_KEY,
    JSON.stringify({ tabs: [], activeTabId: null, closedTabs: [], sidebarWidth: 240, inspectorWidth: 5, inspectorVisible: 'no' }),
  );
  const state = loadWorkspaceState(storage, pageTitleById);
  equal(state.inspectorWidth, INSPECTOR_WIDTH_MIN);
  equal(state.inspectorVisible, true);
  storage.setItem(WORKSPACE_STORAGE_KEY, JSON.stringify({ tabs: [], inspectorVisible: false }));
  equal(loadWorkspaceState(storage, pageTitleById).inspectorVisible, false);
});

// —— 持久化 ——

test('save / load 往返相等', () => {
  const storage = new MapStorage();
  let state = openTab(initialWorkspaceState(), tab('exception-cases'));
  state = openTab(state, tab('shipment-request-inquiry/SR-1', { subtitle: 'SR-1' }));
  state = togglePinned(state, 'exception-cases');
  state = closeTab(state, 'shipment-request-inquiry/SR-1');
  state = setSidebarWidth(state, 300);
  saveWorkspaceState(storage, state);
  deepEqual(loadWorkspaceState(storage, pageTitleById), state);
});

test('load：没存、JSON 坏、不是对象 → 初始态', () => {
  equal(loadWorkspaceState(new MapStorage(), pageTitleById).tabs.length, 0);
  const bad = new MapStorage();
  bad.setItem(WORKSPACE_STORAGE_KEY, '{not json');
  deepEqual(loadWorkspaceState(bad, pageTitleById), initialWorkspaceState());
  bad.setItem(WORKSPACE_STORAGE_KEY, '42');
  deepEqual(loadWorkspaceState(bad, pageTitleById), initialWorkspaceState());
});

test('load：坏的那一格回默认，好的留下——形不对的标签、重复 id、词表外模块、工作台伪标签一律剔除', () => {
  const storage = new MapStorage();
  storage.setItem(
    WORKSPACE_STORAGE_KEY,
    JSON.stringify({
      tabs: [
        { id: 'exception-cases', name: '异常案件' },
        { id: 'exception-cases', name: '重复' },
        { id: '', name: '空 id' },
        { id: 'gone-module/x', name: '词表已改' },
        { id: 'workbench', name: '工作台' },
        { id: 'price-cards', name: '价卡目录', pinned: 'yes' },
        { id: 'shipment-request-inquiry/SR-1', name: '委托查阅', subtitle: 'SR-1', pinned: true },
      ],
      activeTabId: 'gone-module/x',
      closedTabs: [{ id: 'exception-cases', name: '已在开着' }, { id: 'price-cards', name: '价卡目录', pinned: true }],
      sidebarWidth: 'wide',
    }),
  );
  const state = loadWorkspaceState(storage, pageTitleById);
  deepEqual(ids(state), ['exception-cases', 'shipment-request-inquiry/SR-1']);
  equal(state.tabs[1].pinned, true);
  equal(state.activeTabId, null, '活动标签不在集里落工作台');
  deepEqual(state.closedTabs, [{ id: 'price-cards', name: '价卡目录' }], '已开着的不进已关闭栈，固定标记去掉');
  equal(state.sidebarWidth, SIDEBAR_WIDTH_DEFAULT);
});

test('load：id 不规范或解码会抛的标签剔除，不抛出 load——畸形百分号、尾斜杠、三段、带查询串、次段畸形', () => {
  const storage = new MapStorage();
  storage.setItem(
    WORKSPACE_STORAGE_KEY,
    JSON.stringify({
      tabs: [
        { id: '%', name: '孤百分号' },
        { id: '%E0', name: '首段畸形' },
        { id: 'exception-cases/', name: '尾斜杠' },
        { id: 'shipment-request-inquiry/SR-1/timeline', name: '三段' },
        { id: 'exception-cases?view=v1', name: '带查询串' },
        { id: 'shipment-request-inquiry/%E0%A4%A', name: '次段畸形' },
        { id: 'price-cards', name: '价卡目录' },
      ],
      activeTabId: 'exception-cases/',
      closedTabs: [{ id: '%E0%A4%A', name: '坏' }, { id: 'exception-cases', name: '异常案件' }],
    }),
  );
  const state = loadWorkspaceState(storage, pageTitleById);
  deepEqual(ids(state), ['price-cards']);
  equal(state.activeTabId, null);
  deepEqual(state.closedTabs.map((t) => t.id), ['exception-cases']);
});

test('load：名字与副标题按词表与 id 现算，存储里的旧名不读', () => {
  const storage = new MapStorage();
  storage.setItem(
    WORKSPACE_STORAGE_KEY,
    JSON.stringify({
      tabs: [
        { id: 'exception-cases', name: '改名前的旧名' },
        { id: 'shipment-request-inquiry/SR%2F1', name: '旧', subtitle: '旧副标题' },
      ],
      closedTabs: [{ id: 'price-cards', name: '旧', subtitle: '不该有' }],
    }),
  );
  const state = loadWorkspaceState(storage, pageTitleById);
  deepEqual(state.tabs, [
    { id: 'exception-cases', name: '异常案件' },
    { id: 'shipment-request-inquiry/SR%2F1', name: '委托查阅', subtitle: 'SR/1' },
  ]);
  deepEqual(state.closedTabs, [{ id: 'price-cards', name: '价卡目录' }]);
});

test('load：已关闭栈截到上限，截的是最旧的那头', () => {
  const storage = new MapStorage();
  const closedTabs = Array.from({ length: CLOSED_TABS_LIMIT + 5 }, (_, i) => ({ id: `shipment-request-inquiry/SR-${i}` }));
  storage.setItem(WORKSPACE_STORAGE_KEY, JSON.stringify({ tabs: [], closedTabs }));
  const state = loadWorkspaceState(storage, pageTitleById);
  equal(state.closedTabs.length, CLOSED_TABS_LIMIT);
  equal(state.closedTabs[0].id, 'shipment-request-inquiry/SR-0');
});

test('load：sidebarWidth 越界钳进区间', () => {
  const storage = new MapStorage();
  storage.setItem(WORKSPACE_STORAGE_KEY, JSON.stringify({ tabs: [], activeTabId: null, closedTabs: [], sidebarWidth: 1 }));
  equal(loadWorkspaceState(storage, pageTitleById).sidebarWidth, SIDEBAR_WIDTH_MIN);
});

// —— 状态栏 ——

test('workspaceLocationLabel：对象标签「模块名 · 对象标识」、模块标签只有模块名、工作台显工作台', () => {
  equal(workspaceLocationLabel({ id: 'shipment-request-inquiry/SR-1', name: '委托查阅', subtitle: 'SR-1' }, '工作台'), '委托查阅 · SR-1');
  equal(workspaceLocationLabel({ id: 'exception-cases', name: '异常案件' }, '工作台'), '异常案件');
  equal(workspaceLocationLabel(null, '工作台'), '工作台');
});

test('themeWord 说现在是什么', () => {
  equal(themeWord('light'), '浅色主题');
  equal(themeWord('dark'), '深色主题');
});

// —— 快捷键 ——

const key = (k: string, mods: Partial<{ ctrlKey: boolean; metaKey: boolean; altKey: boolean; shiftKey: boolean }> = {}) => ({
  key: k,
  ctrlKey: false,
  metaKey: false,
  altKey: false,
  shiftKey: false,
  ...mods,
});

test('workspaceShortcutOf：Ctrl/⌘+W 关、Ctrl/⌘+Shift+T 重开；Alt 不认、无修饰不认、Shift+W 不认', () => {
  equal(workspaceShortcutOf(key('w', { ctrlKey: true })), 'close-active-tab');
  equal(workspaceShortcutOf(key('W', { metaKey: true })), 'close-active-tab');
  equal(workspaceShortcutOf(key('T', { ctrlKey: true, shiftKey: true })), 'reopen-closed-tab');
  equal(workspaceShortcutOf(key('t', { metaKey: true, shiftKey: true })), 'reopen-closed-tab');
  equal(workspaceShortcutOf(key('w', { ctrlKey: true, altKey: true })), null);
  equal(workspaceShortcutOf(key('w', { ctrlKey: true, shiftKey: true })), null);
  equal(workspaceShortcutOf(key('t', { ctrlKey: true })), null);
  equal(workspaceShortcutOf(key('w')), null);
});
