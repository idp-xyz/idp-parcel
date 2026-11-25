import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import {
  RECENT_ACTIONS_LIMIT,
  TOGGLE_DENSITY_ACTION_ID,
  TOGGLE_INSPECTOR_ACTION_ID,
  TOGGLE_THEME_ACTION_ID,
  buildCommandActions,
  inspectorToggleLabel,
  isOpenCommandPaletteShortcut,
  moduleHash,
  openLabel,
  type RecentEntry,
  type ShellToggles,
} from './command-actions';

// 本文件钉的是命令面板动作集的字面事实（票 admin-web-workspace-form/03 第 1 条）：三组各几条、标签与关键词长什么样、
// run 往哪写 hash。面板组件那半（Ctrl+K 监听挂没挂、列表渲不渲）在 node:test 里钉不到，理由见 templates/loading-shape.ts
// 文件头；本票用一次性 esbuild 束实测，结论写票面。

const pageTitleById: Record<string, string> = {
  workbench: '工作台',
  'shipment-request-inquiry': '委托查阅',
  'exception-cases': '异常案件',
};

function stubHash(entry: RecentEntry): string {
  return `#/${entry.moduleId}/${entry.objectId}`;
}

function build(recentObjects: RecentEntry[], overrides: Partial<ShellToggles> = {}) {
  const written: string[] = [];
  const calls: string[] = [];
  const shellToggles: ShellToggles = {
    theme: 'light',
    toggleTheme: () => calls.push('theme'),
    density: 'comfortable',
    toggleDensity: () => calls.push('density'),
    inspectorVisible: true,
    toggleInspector: () => calls.push('inspector'),
    ...overrides,
  };
  const actions = buildCommandActions({
    pageTitleById,
    recentObjects,
    recentObjectHash: stubHash,
    shellToggles,
    navigate: (hash) => written.push(hash),
  });
  return { actions, written, calls };
}

function recentEntries(count: number): RecentEntry[] {
  return Array.from({ length: count }, (_, i) => ({
    moduleId: 'shipment-request-inquiry',
    objectId: `SR-${String(i + 1).padStart(3, '0')}`,
    title: `委托查阅 · SR-${String(i + 1).padStart(3, '0')}`,
  }));
}

// Covers: 导航词表 N 条 → navigation N 条，工作台也算一条；标签「打开 <页名>」，run 写 `#/<id>`——与 Layout 点击导航同一个 hash。
test('导航组一词一条，工作台在内，run 写模块页 hash', () => {
  const { actions, written } = build([]);
  const navigation = actions.filter((a) => a.group === 'navigation');
  equal(navigation.length, Object.keys(pageTitleById).length);
  const workbench = navigation.find((a) => a.label === openLabel('工作台'));
  ok(workbench, '「打开 工作台」在场');
  workbench.run();
  deepEqual(written, [moduleHash('workbench')]);
  equal(moduleHash('exception-cases'), '#/exception-cases');
});

// Covers: 关键词含 id 与页名——按 id 打字（英文）与按页名打字（中文）都搜得到同一条。
test('导航条目的关键词含 id 与页名', () => {
  const { actions } = build([]);
  const inquiry = actions.find((a) => a.id === 'nav:shipment-request-inquiry');
  ok(inquiry);
  ok(inquiry.keywords.includes('shipment-request-inquiry'));
  ok(inquiry.keywords.includes('委托查阅'));
});

// Covers: 最近对象 12 条 → recent 10 条（上限沿 myshop-web 前 10），取的是最前面的十条；run 写注入的 recentObjectHash 给的地址。
test('最近对象超过上限只取前十，run 写对象地址', () => {
  const { actions, written } = build(recentEntries(12));
  const recent = actions.filter((a) => a.group === 'recent');
  equal(recent.length, RECENT_ACTIONS_LIMIT);
  equal(recent[0].label, openLabel('委托查阅 · SR-001'));
  equal(recent[recent.length - 1].label, openLabel('委托查阅 · SR-010'));
  recent[0].run();
  deepEqual(written, ['#/shipment-request-inquiry/SR-001']);
});

// Covers: 空存储 → recent 0 条不报错；导航组与壳层组不受影响。
test('没有最近对象时 recent 组为空，其余两组照常', () => {
  const { actions } = build([]);
  equal(actions.filter((a) => a.group === 'recent').length, 0);
  equal(actions.filter((a) => a.group === 'navigation').length, Object.keys(pageTitleById).length);
  equal(actions.filter((a) => a.group === 'actions').length, 3);
});

// Covers: 最近对象的关键词含对象标识、标题与模块名；且一律小写——vendor 面板拿 keywords 原样 includes 小写后的查询词，
// 大写的对象标识不先小写就搜不到。模块名查不到时退到 moduleId，不编名字。
test('最近对象的关键词含标识、标题、模块名，且全部小写', () => {
  const known = recentEntries(1)[0];
  const orphan: RecentEntry = { moduleId: 'gone-module', objectId: 'X-1', title: 'gone-module · X-1' };
  const { actions } = build([known, orphan]);
  const recent = actions.filter((a) => a.group === 'recent');
  ok(recent[0].keywords.includes('sr-001'), '对象标识小写后在场');
  ok(recent[0].keywords.includes('委托查阅'), '模块名在场');
  ok(recent[0].keywords.includes('委托查阅 · sr-001'), '标题小写后在场');
  ok(recent[1].keywords.includes('gone-module'), '词表里查不到的模块退到 moduleId');
  for (const a of actions) for (const k of a.keywords) equal(k, k.toLowerCase());
});

// Covers: 壳层组三条——切主题、切密度、切检查器栏——落在 vendor 的 actions 组；标签一律「切过去会变成什么」（主题 / 密度沿顶栏
// 同一句，检查器按可见性答「隐藏 / 显示」）；run 直接调注入的切法。没有「新建 / 导出 / 刷新」那类假快捷动作，也没有底栏切换——本仓没有底栏。
test('壳层组三条：切主题、切密度、切检查器，标签指向目标态', () => {
  const { actions, calls } = build([], { theme: 'dark', density: 'compact', inspectorVisible: false });
  const shell = actions.filter((a) => a.group === 'actions');
  deepEqual(
    shell.map((a) => [a.id, a.label]),
    [
      [TOGGLE_THEME_ACTION_ID, '切换到浅色主题'],
      [TOGGLE_DENSITY_ACTION_ID, '切换到舒适密度'],
      [TOGGLE_INSPECTOR_ACTION_ID, '显示检查器'],
    ],
  );
  shell[0].run();
  shell[1].run();
  shell[2].run();
  deepEqual(calls, ['theme', 'density', 'inspector']);
  equal(inspectorToggleLabel(true), '隐藏检查器');
  ok(!actions.some((a) => /新建|导出|刷新|底栏/.test(a.label)), '没有假快捷动作与不存在的位');
});

// Covers: 三组 id 互不相撞——同一个字串既是模块 id 又是对象标识时，前缀把它们分开。
test('动作 id 全集无重复', () => {
  const { actions } = build([{ moduleId: 'workbench', objectId: 'workbench', title: '工作台 · workbench' }]);
  equal(new Set(actions.map((a) => a.id)).size, actions.length);
});

// 仓配部署开关（shell/warehouse-switch.ts）带来的用例：关着时 warehouseOffered 为假；开着、不知道、没传时与没有这个开关时逐字一样。
const pageTitlesWithWarehouse: Record<string, string> = {
  ...pageTitleById,
  'warehouse-dashboard': '仓配工作台',
  'warehouse-outbound': '出库单',
  'stock-imports': '建账导入',
  'work-devices': '扫描设备',
  'operator-access': '操作者与授予',
};

function buildWithWarehouse(recentObjects: RecentEntry[], warehouseOffered?: boolean) {
  return buildCommandActions({
    pageTitleById: pageTitlesWithWarehouse,
    recentObjects,
    recentObjectHash: stubHash,
    shellToggles: {
      theme: 'light',
      toggleTheme: () => {},
      density: 'comfortable',
      toggleDensity: () => {},
      inspectorVisible: true,
      toggleInspector: () => {},
    },
    navigate: () => {},
    ...(warehouseOffered === undefined ? {} : { warehouseOffered }),
  });
}

const warehouseNavIds = ['nav:warehouse-dashboard', 'nav:warehouse-outbound', 'nav:stock-imports'];

// Covers: 关着时仓配页面不出现在可跳转的页面里；共享设置页（扫描设备、操作者与授予）与小包页照旧。
test('仓配不可用时导航组不列仓配页，共享设置页与小包页照旧', () => {
  const ids = buildWithWarehouse([], false)
    .filter((a) => a.group === 'navigation')
    .map((a) => a.id);
  for (const id of warehouseNavIds) ok(!ids.includes(id), `${id} 不该在面板里`);
  for (const id of ['nav:workbench', 'nav:shipment-request-inquiry', 'nav:exception-cases', 'nav:work-devices', 'nav:operator-access']) {
    ok(ids.includes(id), `${id} 该照旧在面板里`);
  }
});

// Covers: 指向仓配页的最近对象同样不进面板，而且不占名额：先剔除再按上限取，能点开的照样取满。
test('仓配不可用时最近对象剔掉仓配页的，上限按剔除之后取', () => {
  const warehouseRecent: RecentEntry[] = [
    { moduleId: 'warehouse-outbound', objectId: 'OB-001', title: '出库单 · OB-001' },
    { moduleId: 'stock-imports', objectId: 'IMP-001', title: '建账导入 · IMP-001' },
    { moduleId: 'warehouse-outbound', objectId: 'OB-002', title: '出库单 · OB-002' },
  ];
  const entries = [...warehouseRecent, ...recentEntries(12)];

  const withheld = buildWithWarehouse(entries, false).filter((a) => a.group === 'recent');
  equal(withheld.length, RECENT_ACTIONS_LIMIT);
  equal(withheld[0].label, openLabel('委托查阅 · SR-001'));
  ok(withheld.every((a) => !a.id.includes('warehouse-outbound') && !a.id.includes('stock-imports')));

  const shown = buildWithWarehouse(entries, true).filter((a) => a.group === 'recent');
  equal(shown.length, RECENT_ACTIONS_LIMIT);
  equal(shown[0].label, openLabel('出库单 · OB-001'));
});

// Covers: 开着、不知道（没传）时动作集与过去逐字一样：仓配页与指向它们的最近对象都在，传真与不传同一份。
test('仓配可用或没传时，仓配页与它们的最近对象照旧在面板里，动作集逐字一致', () => {
  const entries: RecentEntry[] = [{ moduleId: 'warehouse-outbound', objectId: 'OB-001', title: '出库单 · OB-001' }, ...recentEntries(2)];
  const omitted = buildWithWarehouse(entries);
  const offered = buildWithWarehouse(entries, true);
  deepEqual(
    offered.map((a) => [a.id, a.label, a.group, a.keywords]),
    omitted.map((a) => [a.id, a.label, a.group, a.keywords]),
  );
  const ids = omitted.map((a) => a.id);
  for (const id of warehouseNavIds) ok(ids.includes(id), `${id} 在面板里`);
  ok(ids.includes('recent:warehouse-outbound/OB-001'));
});

// Covers: 壳层组三条（切主题、切密度、切检查器）与开关无关，关着时也在。
test('仓配不可用时壳层组照旧三条', () => {
  equal(buildWithWarehouse([], false).filter((a) => a.group === 'actions').length, 3);
});

// Covers: 开面板快捷键认 Ctrl+K 与 ⌘+K、大小写 K 都算；Alt 一并按下不认；没按修饰键的 k、按了修饰键的别的键都不认。
test('开面板快捷键只认 Ctrl/⌘ + K', () => {
  equal(isOpenCommandPaletteShortcut({ key: 'k', ctrlKey: true, metaKey: false, altKey: false }), true);
  equal(isOpenCommandPaletteShortcut({ key: 'K', ctrlKey: false, metaKey: true, altKey: false }), true);
  equal(isOpenCommandPaletteShortcut({ key: 'k', ctrlKey: true, metaKey: false, altKey: true }), false);
  equal(isOpenCommandPaletteShortcut({ key: 'k', ctrlKey: false, metaKey: false, altKey: false }), false);
  equal(isOpenCommandPaletteShortcut({ key: 'j', ctrlKey: true, metaKey: false, altKey: false }), false);
});
