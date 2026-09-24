import { Fragment, useCallback, useEffect, useMemo, useRef, useState, type ElementType } from 'react';
import { PanelRightClose, PanelRightOpen } from 'lucide-react';
import { EditorGroup, Sidebar, useResize } from '@idpxyz/ui-workspace';
import { useDensity } from '@idpxyz/ui-theme-runtime';
import { Button, Tooltip } from '@idpxyz/ui-primitives';
import { navigationSections, sidebarIconMap, pageTitleById } from './navigation';
import {
  InspectorPanel,
  InspectorProvider,
  TabReturnProvider,
  type InspectorContent,
  type InspectorController,
  type TabReturnController,
} from './templates';
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
  INSPECTOR_WIDTH_MAX,
  INSPECTOR_WIDTH_MIN,
  SIDEBAR_WIDTH_MAX,
  SIDEBAR_WIDTH_MIN,
  WORKBENCH_MODULE_ID,
  activateTab,
  addressForTab,
  closeAll,
  closeOthers,
  closeTab,
  closeToRight,
  hashOfUrl,
  loadWorkspaceState,
  moduleIdOfTab,
  openTab,
  rememberTabAddress,
  reopenClosed,
  reorderTabs,
  saveWorkspaceState,
  setInspectorVisible,
  setInspectorWidth,
  setSidebarWidth,
  tabForHash,
  togglePinned,
  workspaceLocationLabel,
  workspaceShortcutOf,
  type TabAddressBook,
  type WorkspaceState,
} from './shell/workspace-state';

// 多标签工作区外壳（票 admin-web-workspace-form/01 与 02 壳层段，参照 idp-ui@6751fb2 apps/myshop-web/src/App.tsx）：顶栏 + 左侧导航 +
// EditorGroup 多标签主区 + 右侧检查器栏 + 底部状态栏。多标签是因为蓝图母版 B / C 都假定人同时开着一张队列和几个对象在比对，
// 单页区每回一次列表就丢一次对象。
// 底栏不留位——没有事件 / 日志 / 备注读口，一个永远空的底栏不是「禁用态 + 说明」能交代的（spec「不做」）。
// 不装 ActivityBar：本仓只有一种侧栏内容，myshop-web 的那一格点了也只是折叠侧栏，是 IDE 形不是能力。
//
// 检查器栏（蓝图母版 B「List → Preview → Inspector」）：列表页单击一行经 templates/inspector-context 的 useInspector 把内容交到这里，
// 右栏常驻显示，翻行时跟着换；切换活动标签即清空——检查器说的是当前列表选中的那一行，换页就不成立了。栏可拖宽（区间是
// workspace-state 的 INSPECTOR_WIDTH_MIN / INSPECTOR_WIDTH_MAX）、可折叠，两者都进工作区状态持久化；折起来时只剩一个展开按钮，
// 不占宽。栏顶是收起按钮而不是 ×：常驻位折起来内容还在，× 的通行语义是丢弃。myshop-web 的 RightSidebar 按对象种类在壳层分派渲染器，
// 这里反过来让**页面**给内容、壳层只渲染契约（templates/inspector.ts）——对象长什么样归拥有它的页，壳层不认识任何业务对象。
//
// 顶栏是 shell/TopBar 自己的件（手册「顶栏规范」的全局位），这里只喂它当前页名、会话主体名与登出动作；
// 登出仍是 auth/oidc.ts 那一个，外壳不另起一套会话处置。
// 页面映射在 page-registry：没登记的 id 落 UnwiredModule 诚实占位——导航条目先于页面出现时，缺的是页面不是路由。
//
// **导航位置的唯一权威是地址栏 hash**（#/<模块id>[/<对象id>][/<页内子路径>][?<查询串>]）：刷新回到原页、浏览器前进后退可用、
// 模块页与对象页都可收藏转发。标签集是 hash 的镜像不是第二个权威：标签 id 就是 hash 的前两段（shell/workspace-state.ts 的
// tabIdFromHash），hashchange → openTab（新地址开新标签 / 已开则激活）。点标签只写 hash，状态经 hashchange 回流；关、重开、批量关
// 得先在标签集上算（关掉活动标签该落哪个邻居只有标签集知道），于是先落状态、活动标签变了再写 hash（applyWorkspace），回流时
// openTab 已开则激活、幂等。后一路对活动标签答了两次，两次答成同一张靠的是 id 规范——hashForTab(id) 认回来仍是 id；
// 从存储读回的标签由 loadWorkspaceState 按这一条筛过。
// 回程（点标签、关标签落邻居、页面经 TabReturnProvider 回到某标签）写的是那张标签上次停的完整地址（addressForTab），不是只有
// 前两段的首址：检索词这类只活在地址里的页内状态才留得住。它由 hashchange 的 oldURL 记下、按 tabForHash 归档，认回来仍是那张
// 标签，上面「答成同一张」的前提不变。
// 三条互斥各自为何：不双写——若点标签既 setState 又写 hash，两条路对同一件事各答一次，失配时没人知道哪个对；
// 不在标签里存页内状态——第二段之后与查询串归页面（详情钻取、保存视图的 `?view=`、检索词的 `?q=`），标签至多记「上次停在
// 哪个地址」，页面状态仍只从地址读、页面卸载即丢；
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

/** hash 落到标签集上：对象 / 模块地址开或激活对应标签；工作台、空 hash 与词表外的 id 都是「没有活动标签」。 */
function applyHash(state: WorkspaceState, hash: string): WorkspaceState {
  const tab = tabForHash(hash, pageTitleById);
  return tab ? openTab(state, tab) : activateTab(state, null);
}

function workspaceFromStorageAndLocation(): WorkspaceState {
  return applyHash(loadWorkspaceState(window.localStorage, pageTitleById), window.location.hash);
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
  // 各标签上次停在的完整地址：hashchange 时按 oldURL 记下离开的那一格，回程时读。只在回程那一刻被读、不驱动渲染，放 ref。
  const tabAddresses = useRef<TabAddressBook>(new Map());

  useEffect(() => {
    saveWorkspaceState(window.localStorage, workspace);
  }, [workspace]);

  useEffect(() => {
    // 首帧也记一次：刷新回到详情页、从收藏直接打开，都是「打开过」；同一对象去重置顶，重复记不会多出一条。
    recordRecentObjectFromHash();
    const onHashChange = (event: HashChangeEvent) => {
      tabAddresses.current = rememberTabAddress(tabAddresses.current, hashOfUrl(event.oldURL), pageTitleById);
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
    if (next.activeTabId !== prev.activeTabId) window.location.hash = addressForTab(tabAddresses.current, next.activeTabId);
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

  // 检查器：内容由列表页经 useInspector 交进来；控制口 useMemo 住，Provider 值不随每次渲染换引用。
  const [inspectorContent, setInspectorContent] = useState<InspectorContent | null>(null);
  // 主区里挂着几个供内容的列表——非活动标签卸载，实际只有活动页的零个或一个；只拿来选空态句。
  // 计数而不是布尔：换标签时旧页撤回与新页声明的先后不由这里定。
  const [inspectorOffers, setInspectorOffers] = useState(0);
  const inspectorController = useMemo<InspectorController>(
    () => ({
      show: (content) => setInspectorContent(content),
      clear: () => setInspectorContent(null),
      offer: () => {
        setInspectorOffers((n) => n + 1);
        return () => setInspectorOffers((n) => n - 1);
      },
    }),
    [],
  );
  useEffect(() => {
    setInspectorContent(null);
  }, [workspace.activeTabId]);
  // 页面「回到某标签」的口：与点标签同一条回程，落回那张标签上次停的地址。
  const tabReturn = useMemo<TabReturnController>(
    () => ({
      returnTo: (tabId) => {
        window.location.hash = addressForTab(tabAddresses.current, tabId);
      },
    }),
    [],
  );
  const inspectorResize = useResize({
    direction: 'horizontal',
    initialSize: workspace.inspectorWidth,
    minSize: INSPECTOR_WIDTH_MIN,
    maxSize: INSPECTOR_WIDTH_MAX,
    // 把手在栏的左边、栏在右边：往左拖是变宽，与侧栏相反。
    reverse: true,
  });
  useEffect(() => {
    setWorkspace((state) =>
      state.inspectorWidth === inspectorResize.size ? state : setInspectorWidth(state, inspectorResize.size),
    );
  }, [inspectorResize.size]);
  // applyWorkspace 每次渲染新建，但它只读 ref 与稳定的 setState，第一份与最新一份等价，所以这里可以不依赖它。
  const toggleInspector = useCallback(() => applyWorkspace((state) => setInspectorVisible(state, !state.inspectorVisible)), []);
  const inspectorToggle = useMemo(
    () => ({ visible: workspace.inspectorVisible, toggle: toggleInspector }),
    [workspace.inspectorVisible, toggleInspector],
  );

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
        inspector={inspectorToggle}
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
            给只能从 DOM 读档的消费者（样式选择器、探针）用；ListPageTemplate 走的是 useDensity，不读它。
            Provider 只包主区：检查器的内容只能来自主区里的页面。 */}
        <main className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor min-w-0" data-density={density}>
          <InspectorProvider value={inspectorController}>
          <TabReturnProvider value={tabReturn}>
          <EditorGroup
            group={{ id: MAIN_EDITOR_GROUP_ID, tabs: workspace.tabs, activeTab: workspace.activeTabId ?? '' }}
            isActive
            showSplitButtons={false}
            onActivate={() => {}}
            // 点已活动的那张不写 hash：hash 上可能带着页内的 ?view= 或第三段，重写会剥掉它们而页面实例不换，地址与屏上所示就失配了。
            onTabClick={(id) => {
              if (id !== workspaceRef.current.activeTabId) navigate(addressForTab(tabAddresses.current, id));
            }}
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
          </TabReturnProvider>
          </InspectorProvider>
        </main>
        {workspace.inspectorVisible ? (
          <>
            <div className="resize-handle-h" onMouseDown={inspectorResize.handleMouseDown} />
            <aside
              aria-label="检查器"
              className="flex shrink-0 flex-col overflow-hidden border-l border-idpxyz-border"
              style={{ width: inspectorResize.size }}
            >
              <InspectorPanel
                content={inspectorContent}
                contentOffered={inspectorOffers > 0}
                headerActions={
                  <Tooltip content="隐藏检查器" side="left">
                    <Button variant="ghost" size="icon" className="h-5 w-5" aria-label="隐藏检查器" onClick={toggleInspector}>
                      <PanelRightClose className="h-3.5 w-3.5" aria-hidden="true" />
                    </Button>
                  </Tooltip>
                }
              />
            </aside>
          </>
        ) : (
          // 折叠态只剩一个展开按钮：位还在（蓝图「即使功能未完整也要留位」），但不占宽。
          <div className="flex shrink-0 flex-col items-center border-l border-idpxyz-border bg-idpxyz-sidebar px-0.5 pt-1">
            <Tooltip content="显示检查器" side="left">
              <Button variant="ghost" size="icon" aria-label="显示检查器" onClick={toggleInspector}>
                <PanelRightOpen className="h-4 w-4" aria-hidden="true" />
              </Button>
            </Tooltip>
          </div>
        )}
      </div>
      <WorkspaceStatusBar location={workspaceLocationLabel(activeTab, pageTitleById[WORKBENCH_MODULE_ID])} />
    </div>
  );
}
