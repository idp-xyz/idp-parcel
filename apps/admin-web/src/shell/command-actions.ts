import type { CommandPaletteAction } from '@idpxyz/ui-workspace';
import type { DensityPreference, ThemePreference } from './preferences';
import { densityToggleLabel, themeToggleLabel } from './top-bar-model';

// 命令面板的动作集（票 admin-web-workspace-form/03 第 1 条）：面板能做的每一件事在这里生成，CommandPaletteHost 只挂。
//
// 面板不是搜索：这里没有一条动作发请求。三组的来源都是本机已知的事实——导航词表（navigation.ts 的 pageTitleById）、
// 第一轮 02 存在本机 localStorage 里的最近对象、壳层自己的两个开关。能搜到的对象只有这台浏览器打开过的，这不是缺陷
// 是边界：对象名称属业务数据，第一轮 02 的隐私边界不让前端为了补名字去发请求，最近对象的标题因此只是「模块名 · 对象标识」。
//
// 为什么单独一个 .ts：三组的条数、标签、关键词与 run 写什么 hash 是与 React 无关的事实，组件层在 node:test 里钉不到
// （理由见 templates/loading-shape.ts 文件头），抬到这里钉。只 import type，不 import 任何 @idpxyz/* 的运行时——
// 测试编成 CommonJS 后能直接 require，与 preferences.ts / top-bar-model.ts 同款。
//
// 参照 idp-ui@6751fb2 apps/myshop-web/src/commandActions.ts 的三组分法（navigation / recent / actions），但它的
// 「新建订单 / 导出 / 刷新」那组假快捷动作（run 只弹一句「功能开发中」）一条不搬——spec「红线」不允许假动作；
// 「切换右栏」落为壳层组的「显示 / 隐藏检查器」（下面的 TOGGLE_INSPECTOR_ACTION_ID）；「切换底栏」不搬，本仓没有底栏。

/**
 * 最近对象在本文件里需要的最小形；与第一轮 02 的 RecentObject（pages/my-work/recent-objects.ts）结构兼容，
 * 那边多出来的字段（打开时刻）本文件不读。不 import 那边的类型：存储归那张票，这里只收已读好的数组。
 */
export interface RecentEntry {
  /** 导航条目 id，与 navigation.ts 同一套。 */
  moduleId: string;
  /** hash 第二段解码后的对象标识；语义归那个模块页。 */
  objectId: string;
  /** 展示用标题；不是对象名称（见文件头的隐私边界）。 */
  title: string;
}

/**
 * 壳层开关的现值与切法；由 CommandPaletteHost 从 useTheme / useDensity 取了注入，本文件不认识 Provider。
 * 检查器栏的可见性归 Layout 的工作区状态（shell/workspace-state.ts），同样由宿主注入现值与切法。
 */
export interface ShellToggles {
  theme: ThemePreference;
  toggleTheme: () => void;
  density: DensityPreference;
  toggleDensity: () => void;
  inspectorVisible: boolean;
  toggleInspector: () => void;
}

export interface BuildCommandActionsInput {
  /** 导航词表：id → 页名。工作台也在里面，所以它也是一条「打开 工作台」。 */
  pageTitleById: Record<string, string>;
  /** 已读好的最近对象，最近的在前；只取前 RECENT_ACTIONS_LIMIT 条。 */
  recentObjects: RecentEntry[];
  /**
   * 对象地址怎么写归第一轮 02 的 recentObjectHash（它与各模块页写 hash 的形一致），这里不复制第二份写法，
   * 由调用方把那一个函数注进来。
   */
  recentObjectHash: (entry: RecentEntry) => string;
  shellToggles: ShellToggles;
  /** 落 hash 的地方；默认写 window.location.hash，测试注入以免碰全局。 */
  navigate?: (hash: string) => void;
}

/** 最近对象在面板里的条数上限，沿 myshop-web 的前 10；再多要翻，翻就不如去「我的工作」那一页。 */
export const RECENT_ACTIONS_LIMIT = 10;

/** 三组动作的 id 前缀；面板按 id 去重与选中，前缀让导航条目与同名对象标识不撞。 */
export const NAVIGATION_ACTION_ID_PREFIX = 'nav:';
export const RECENT_ACTION_ID_PREFIX = 'recent:';
export const SHELL_ACTION_ID_PREFIX = 'shell:';

export const TOGGLE_THEME_ACTION_ID = `${SHELL_ACTION_ID_PREFIX}toggle-theme`;
export const TOGGLE_DENSITY_ACTION_ID = `${SHELL_ACTION_ID_PREFIX}toggle-density`;
export const TOGGLE_INSPECTOR_ACTION_ID = `${SHELL_ACTION_ID_PREFIX}toggle-inspector`;

/** 与主题 / 密度两条同一句式：说切过去会变成什么。 */
export function inspectorToggleLabel(visible: boolean): string {
  return visible ? '隐藏检查器' : '显示检查器';
}

/** 「打开 」后接页名或对象标题；中间一个空格，让面板按 label 匹配时「打开」不与页名黏成一个词。 */
export function openLabel(target: string): string {
  return `打开 ${target}`;
}

/**
 * 模块页地址 `#/<模块id>`，与 Layout 点击导航写的形一致——hash 是导航位置的唯一权威（Layout.tsx 文件头），
 * 面板不另起跳转，只写同一个 hash，状态经 hashchange 回流。
 */
export function moduleHash(moduleId: string): string {
  return `#/${moduleId}`;
}

/**
 * 面板认的开面板快捷键：Ctrl+K 或 ⌘+K（macOS 上 Ctrl 位被系统占着，myshop-web 同样两个都认）。
 * Alt 一并按下不认——Alt+K 在若干输入法里是别的意思；Shift 不管，大小写 K 都算。
 */
export function isOpenCommandPaletteShortcut(event: {
  key: string;
  ctrlKey: boolean;
  metaKey: boolean;
  altKey: boolean;
}): boolean {
  return (event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLowerCase() === 'k';
}

/**
 * 关键词一律小写、去空、去重：vendor CommandPalette（ui-workspace 0.1.25）过滤时把查询词小写了，却拿 keywords
 * 原样做 includes——大写的对象标识（SR-… 之类）不先小写就永远搜不到。label 那一侧它自己会小写，这里不用管。
 */
function keywordsOf(...raw: string[]): string[] {
  const seen = new Set<string>();
  for (const k of raw) {
    const v = k.trim().toLowerCase();
    if (v !== '') seen.add(v);
  }
  return [...seen];
}

function assignLocationHash(hash: string): void {
  window.location.hash = hash;
}

export function buildCommandActions({
  pageTitleById,
  recentObjects,
  recentObjectHash,
  shellToggles,
  navigate = assignLocationHash,
}: BuildCommandActionsInput): CommandPaletteAction[] {
  const navigation: CommandPaletteAction[] = Object.entries(pageTitleById).map(([id, title]) => ({
    id: `${NAVIGATION_ACTION_ID_PREFIX}${id}`,
    label: openLabel(title),
    group: 'navigation',
    keywords: keywordsOf(id, title),
    run: () => navigate(moduleHash(id)),
  }));

  const recent: CommandPaletteAction[] = recentObjects.slice(0, RECENT_ACTIONS_LIMIT).map((entry) => ({
    id: `${RECENT_ACTION_ID_PREFIX}${entry.moduleId}/${entry.objectId}`,
    label: openLabel(entry.title),
    group: 'recent',
    // 模块名查不到（导航词表已改）就退到 moduleId，不编名字——与 02 的 recentObjectTitle 同一态度。
    keywords: keywordsOf(entry.objectId, entry.title, pageTitleById[entry.moduleId] ?? entry.moduleId),
    run: () => navigate(recentObjectHash(entry)),
  }));

  // vendor 的 group 联合里没有 shell，壳层开关落在 actions 组；标签沿 top-bar-model 的「切换到 <目标态>」——
  // 与顶栏两个图标按钮的 aria-label 同一句，面板与顶栏不各自漂。
  const shell: CommandPaletteAction[] = [
    {
      id: TOGGLE_THEME_ACTION_ID,
      label: themeToggleLabel(shellToggles.theme),
      group: 'actions',
      keywords: keywordsOf('主题', '深色', '浅色', 'theme', 'dark', 'light'),
      run: shellToggles.toggleTheme,
    },
    {
      id: TOGGLE_DENSITY_ACTION_ID,
      label: densityToggleLabel(shellToggles.density),
      group: 'actions',
      keywords: keywordsOf('密度', '紧凑', '舒适', 'density', 'compact', 'comfortable'),
      run: shellToggles.toggleDensity,
    },
    {
      id: TOGGLE_INSPECTOR_ACTION_ID,
      label: inspectorToggleLabel(shellToggles.inspectorVisible),
      group: 'actions',
      keywords: keywordsOf('检查器', '右栏', 'inspector', 'sidebar'),
      run: shellToggles.toggleInspector,
    },
  ];

  return [...navigation, ...recent, ...shell];
}
