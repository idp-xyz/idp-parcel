import { Fragment, useEffect, useMemo, useRef, useState, type ElementType } from 'react';
import { EditorGroup, Sidebar, useResize } from '@idpxyz/ui-workspace';
import { useDensity } from '@idpxyz/ui-theme-runtime';
import { navigationSections, sidebarIconMap, pageTitleById } from './navigation';
import { pageById } from './page-registry';
import { Workbench } from './pages/Workbench';
import { UnwiredModule } from './pages/UnwiredModule';
import { TopBar } from './shell/TopBar';
import { CommandPaletteHost } from './shell/CommandPaletteHost';
import { WorkspaceStatusBar } from './shell/WorkspaceStatusBar';
import { isOpenCommandPaletteShortcut } from './shell/command-actions';
import { useSessionPrincipal } from './shell/session';
import { logout } from './auth/oidc';
import {
  listRecentObjects,
  recentObjectFromHash,
  recentObjectHash,
  recentObjectTitle,
  recordRecentObject,
} from './pages/my-work/recent-objects';
import {
  SIDEBAR_WIDTH_MAX,
  SIDEBAR_WIDTH_MIN,
  WORKBENCH_MODULE_ID,
  activateTab,
  closeAll,
  closeOthers,
  closeTab,
  closeToRight,
  hashForTab,
  loadWorkspaceState,
  moduleIdOfTab,
  openTab,
  reopenClosed,
  reorderTabs,
  saveWorkspaceState,
  setSidebarWidth,
  tabForHash,
  togglePinned,
  workspaceLocationLabel,
  workspaceShortcutOf,
  type WorkspaceState,
} from './shell/workspace-state';

// 多标签工作区外壳（票 admin-web-workspace-form/01，参照 idp-ui@6751fb2 apps/myshop-web/src/App.tsx）：顶栏 + 左侧导航 +
// EditorGroup 多标签主区 + 底部状态栏。第一轮裁「不引入标签页、等首个真实页面出现后再定」的前提已变——真实页面早就有了
// （委托查阅的 hash 二段详情、对象工作区母版），参照物也换成了多标签壳；蓝图母版 B / C 都假定人同时开着一张队列和几个对象在比对。
// 右栏位（检查器）随票 02 装；底栏不留位——没有事件 / 日志 / 备注读口，一个永远空的底栏不是「禁用态 + 说明」能交代的（spec「不做」）。
// 不装 ActivityBar：本仓只有一种侧栏内容，myshop-web 的那一格点了也只是折叠侧栏，是 IDE 形不是能力。
//
// 顶栏是 shell/TopBar 自己的件（手册「顶栏规范」的全局位），这里只喂它当前页名、会话主体名与登出动作；
// 登出仍是 auth/oidc.ts 那一个，外壳不另起一套会话处置。
// 页面映射在 page-registry：没登记的 id 落 UnwiredModule 诚实占位——导航条目先于页面出现时，缺的是页面不是路由。
//
// **导航位置的唯一权威仍是地址栏 hash**（#/<模块id>[/<对象id>][/<页内子路径>][?<查询串>]）：刷新回到原页、浏览器前进后退可用、
// 模块页与对象页都可收藏转发。标签集是 hash 的镜像不是第二个权威：标签 id 就是 hash 的前两段（shell/workspace-state.ts 的
// tabIdFromHash），hashchange → openTab（新地址开新标签 / 已开则激活）；点标签、关标签、重开都只写 hash，状态经同一条 hashchange 回流。
// 三条互斥各自为何：不双写——若点标签既 setState 又写 hash，两条路对同一件事各答一次，失配时没人知道哪个对；
// 不在标签里存页内状态——第二段之后与查询串归页面（详情钻取、保存视图的 `?view=`），标签只记地址，页面卸载即丢；
// 关标签写 hash——关掉活动标签后「现在在哪」这一问仍由 hash 答，标签集只是算出该落到哪个邻居，再把答案写回地址。
// 工作台不是标签而是 EditorGroup 的 emptyStateContent：activeTabId 为 null 即显示工作台，理由见 workspace-state.ts 文件头。
// 非活动标签卸载（preserveInactiveTabContent 关）：每张页各自 fetch，留着会攒请求——myshop-web 的演示数据无此顾虑，我们有。
//
// 外壳代管的唯一一件「页内」事是记最近对象：落到带第二段的对象地址就记一条本机历史（pages/my-work/recent-objects.ts）——
// 记的是地址不是状态，且只有外壳站在每次 hash 变化的必经之路上。
//
// 键盘监听只挂一个：Ctrl/⌘+K 开命令面板、Ctrl/⌘+W 关活动标签、Ctrl/⌘+Shift+T 重开，三条在同一个 keydown 里分派——两个监听各认
// 各的会在「谁先 preventDefault」上互相猜。命令面板（shell/CommandPaletteHost）因此只是受控件：open 态在这里、TopBar 搜索位与快捷键
// 两个入口指向同一份态。面板读最近对象用的是 recent-objects.ts 那一份读函数与地址写法，由外壳注入，host 不认识存储。

/** 命令面板每次打开时重读一遍最近对象；模块级常量而不是每次渲染新建，host 的 useMemo 依赖它的引用。 */
const readRecentObjectsForPalette = () => listRecentObjects(window.localStorage);

const isKnownModule = (moduleId: string) => pageTitleById[moduleId] !== undefined;

/** hash 落到标签集上：对象 / 模块地址开或激活对应标签；工作台、空 hash 与词表外的 id 都是「没有活动标签」。 */
function applyHash(state: WorkspaceState, hash: string): WorkspaceState {
  const tab = tabForHash(hash, pageTitleById);
  return tab ? openTab(state, tab) : activateTab(state, null);
}

function workspaceFromStorageAndLocation(): WorkspaceState {
  return applyHash(loadWorkspaceState(window.localStorage, isKnownModule), window.location.hash);
}

// 模块 id 必须在导航词表内才记：未知 id 的地址上面已落回工作台，历史里若留下它，就成了一条查无出处的模块。
// 标题只从模块名与对象标识拼，不发请求取名——对象名称属业务数据，列表页打开时自己知道、进详情页再显。
function recordRecentObjectFromHash(): void {
  const hit = recentObjectFromHash(window.location.hash);
  if (!hit) return;
  const moduleTitle = pageTitleById[hit.moduleId];
  if (moduleTitle === undefined) return;
  recordRecentObject(window.localStorage, {
    moduleId: hit.moduleId,
    objectId: hit.objectId,
    title: recentObjectTitle(moduleTitle, hit.moduleId, hit.objectId),
    at: new Date().toISOString(),
  });
}

/** 单一编辑器组；不分栏（spec 判断项 2 推荐第一版不做），id 只为 EditorGroup 的拖拽载荷与面板上下文有个名字。 */
const MAIN_EDITOR_GROUP_ID = 'main';

export function Layout() {
  const [workspace, setWorkspace] = useState<WorkspaceState>(workspaceFromStorageAndLocation);
  // 事件回调（快捷键、EditorGroup 的各 on*）里读最新状态用 ref，不靠闭包——它们在同一帧里可能连着来两次。
  const workspaceRef = useRef(workspace);
  workspaceRef.current = workspace;

  useEffect(() => {
    saveWorkspaceState(window.localStorage, workspace);
  }, [workspace]);

  useEffect(() => {
    // 首帧也记一次：刷新回到详情页、从收藏直接打开，都是「打开过」；同一对象去重置顶，重复记不会多出一条。
    recordRecentObjectFromHash();
    const onHashChange = () => {
      setWorkspace((state) => applyHash(state, window.location.hash));
      recordRecentObjectFromHash();
    };
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  /**
   * 把一个标签操作落到状态上，活动标签若因此变了就把新位置写回 hash（hashchange 回流时 openTab 已开则激活，幂等）。
   * 先改 ref 再 setState：同一帧里紧接着的第二个操作要在第一个的结果上算。
   */
  const applyWorkspace = (operation: (state: WorkspaceState) => WorkspaceState) => {
    const prev = workspaceRef.current;
    const next = operation(prev);
    if (next === prev) return;
    workspaceRef.current = next;
    setWorkspace(next);
    if (next.activeTabId !== prev.activeTabId) window.location.hash = hashForTab(next.activeTabId);
  };

  const navigate = (hash: string) => {
    window.location.hash = hash;
  };
  const setActive = (id: string) => navigate(`#/${id}`);

  const [commandPaletteOpen, setCommandPaletteOpen] = useState(false);
  const openCommandPalette = () => setCommandPaletteOpen(true);

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (isOpenCommandPaletteShortcut(e)) {
        // Ctrl+K 在 Chromium 默认聚焦地址栏，不拦下来面板开了焦点却跑了。
        e.preventDefault();
        setCommandPaletteOpen(true);
        return;
      }
      const shortcut = workspaceShortcutOf(e);
      if (shortcut === null) return;
      const current = workspaceRef.current;
      if (shortcut === 'close-active-tab') {
        // 工作台上没有可关的标签：不拦，让浏览器自己的 Ctrl+W 照常——吞了却什么都不做比不接更糟。
        if (current.activeTabId === null) return;
        e.preventDefault();
        applyWorkspace((state) => closeTab(state, current.activeTabId!));
      } else {
        if (current.closedTabs.length === 0) return;
        e.preventDefault();
        applyWorkspace(reopenClosed);
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, []);

  const sidebarResize = useResize({
    direction: 'horizontal',
    initialSize: workspace.sidebarWidth,
    minSize: SIDEBAR_WIDTH_MIN,
    maxSize: SIDEBAR_WIDTH_MAX,
  });
  useEffect(() => {
    setWorkspace((state) => (state.sidebarWidth === sidebarResize.size ? state : setSidebarWidth(state, sidebarResize.size)));
  }, [sidebarResize.size]);

  const { density } = useDensity();
  const principal = useSessionPrincipal();

  const activeTab = workspace.tabs.find((tab) => tab.id === workspace.activeTabId) ?? null;
  const activeModuleId = activeTab ? moduleIdOfTab(activeTab.id) : WORKBENCH_MODULE_ID;

  // 标签图标沿侧栏同一张表按模块取；对象标签与它的模块页同图标——标签 id 带对象段，表要按 id 键入，所以逐标签映一遍。
  const tabIconMap = useMemo(() => {
    const map: Record<string, ElementType> = {};
    for (const tab of workspace.tabs) {
      const icon = sidebarIconMap[moduleIdOfTab(tab.id)];
      if (icon) map[tab.id] = icon;
    }
    return map;
  }, [workspace.tabs]);

  // 按标签 id 键住内容：两张标签同一模块（列表与它的一份详情）时不复用同一个页面实例，切标签就是换页，页内状态不串。
  const renderTab = (tabId: string) => {
    const moduleId = moduleIdOfTab(tabId);
    const Page = pageById[moduleId];
    return <Fragment key={tabId}>{Page ? <Page /> : <UnwiredModule moduleId={moduleId} />}</Fragment>;
  };

  return (
    <div className="h-screen w-screen flex flex-col bg-idpxyz-bg text-idpxyz-text overflow-hidden">
      <TopBar
        moduleTitle={pageTitleById[activeModuleId] || activeModuleId}
        principal={principal}
        onSignOut={logout}
        onOpenCommandPalette={openCommandPalette}
      />
      <CommandPaletteHost
        open={commandPaletteOpen}
        onOpenChange={setCommandPaletteOpen}
        readRecent={readRecentObjectsForPalette}
        recentObjectHash={recentObjectHash}
      />

      <div className="flex flex-1 overflow-hidden">
        <Sidebar
          width={sidebarResize.size}
          onFileClick={setActive}
          activeFile={activeModuleId}
          navigationSections={navigationSections}
          iconMap={sidebarIconMap}
        />
        <div className="resize-handle-h" onMouseDown={sidebarResize.handleMouseDown} />
        {/* main 地标：读屏用户跳过导航直达页面内容的锚点。data-density 是密度两档在 DOM 上的落点（第一轮 01 第 4 条），
            给只能从 DOM 读档的消费者（样式选择器、探针）用；ListPageTemplate 走的是 useDensity，不读它。 */}
        <main className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor min-w-0" data-density={density}>
          <EditorGroup
            group={{ id: MAIN_EDITOR_GROUP_ID, tabs: workspace.tabs, activeTab: workspace.activeTabId ?? '' }}
            isActive
            showSplitButtons={false}
            onActivate={() => {}}
            onTabClick={(id) => navigate(hashForTab(id))}
            onTabClose={(id) => applyWorkspace((state) => closeTab(state, id))}
            onTabPin={(id) => applyWorkspace((state) => togglePinned(state, id))}
            onCloseOthers={(id) => applyWorkspace((state) => closeOthers(state, id))}
            onCloseToRight={(id) => applyWorkspace((state) => closeToRight(state, id))}
            onCloseAll={() => applyWorkspace(closeAll)}
            onReopenClosed={() => applyWorkspace(reopenClosed)}
            onTabReorder={(from, to) => applyWorkspace((state) => reorderTabs(state, from, to))}
            // 分栏按钮关着（判断项 2），这两个回调到不了；EditorGroup 把它们声明成必填，给空函数不给假实现。
            onSplitRight={() => {}}
            onSplitDown={() => {}}
            tabIconMap={tabIconMap}
            // 标签不编颜色：点的颜色要从对象状态层来，等检查器把「选中对象的状态簇」立起来再说（票面第 3 条）。
            getTabRiskDot={() => null}
            renderContent={renderTab}
            emptyStateContent={<Workbench onNavigate={setActive} />}
            preserveInactiveTabContent={false}
          />
        </main>
      </div>
      <WorkspaceStatusBar location={workspaceLocationLabel(activeTab, pageTitleById[WORKBENCH_MODULE_ID])} />
    </div>
  );
}
