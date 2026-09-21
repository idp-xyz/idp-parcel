import { useEffect, useState } from 'react';
import { Sidebar, useResize } from '@idpxyz/ui-workspace';
import { useDensity } from '@idpxyz/ui-theme-runtime';
import { navigationSections, sidebarIconMap, pageTitleById } from './navigation';
import { pageById } from './page-registry';
import { Workbench } from './pages/Workbench';
import { UnwiredModule } from './pages/UnwiredModule';
import { TopBar } from './shell/TopBar';
import { CommandPaletteHost } from './shell/CommandPaletteHost';
import { useSessionPrincipal } from './shell/session';
import { logout } from './auth/oidc';
import {
  listRecentObjects,
  recentObjectFromHash,
  recentObjectHash,
  recentObjectTitle,
  recordRecentObject,
} from './pages/my-work/recent-objects';

// 传统控制台外壳：顶栏 + 左侧导航 + 单页区，参考 idp-ui
// apps/loms-web 的 console/Layout；不引入标签页与底部/右侧面板，
// 等首个真实页面出现后再按实际交互决定是否升级形态。
// 顶栏是 shell/TopBar 自己的件（手册「顶栏规范」的全局位），这里只喂它当前页名、会话主体名与登出动作；
// 登出仍是 auth/oidc.ts 那一个，外壳不另起一套会话处置。
// 页面映射在 page-registry：没登记的 id 落 UnwiredModule 诚实占位——
// 导航条目先于页面出现时，缺的是页面不是路由。工作台是外壳首页，
// 不入登记，由这里直接渲染并注入跳转能力。
//
// 导航位置的唯一权威是地址栏 hash（#/<模块id>[/<页内子路径>][?<查询串>]）：刷新回到原页、
// 浏览器前进后退可用、模块页可收藏转发。外壳只认第一段并校验其在导航词表内；
// 后段归各页面自取（如委托查阅用第二段承载详情钻取），查询串也归页面（保存视图跳转的 `?view=`
// 由模块页的保存视图位读，外壳剥掉不认），外壳不代管页内状态。
// 点击导航写 hash，状态经 hashchange 事件回流——单一来源，不双写。
// 外壳代管的唯一一件「页内」事是记最近对象：落到带第二段的对象地址就记一条本机历史
// （pages/my-work/recent-objects.ts）——记的是地址不是状态，且只有外壳站在每次 hash 变化的必经之路上。
//
// 命令面板（shell/CommandPaletteHost）也挂在这里、与 TopBar 并列：它有两个入口——host 自己听的 Ctrl/⌘+K 与
// TopBar 搜索位的按钮——两个入口要指向同一份 open 态，态只能在它们共同的父级。面板读最近对象用的是
// recent-objects.ts 那一份读函数与地址写法，由外壳注入，host 不认识存储；面板里的跳转也只写 hash，走上面同一条路。

/** 命令面板每次打开时重读一遍最近对象；模块级常量而不是每次渲染新建，host 的 useMemo 依赖它的引用。 */
const readRecentObjectsForPalette = () => listRecentObjects(window.localStorage);

function moduleIdFromHash(): string {
  // 先剥查询串再取段：`?view=` 挂在第一段上，不剥的话 `#/<moduleId>?view=<id>` 会被当成一个未知 id 落回工作台。
  const first = window.location.hash.replace(/^#\/?/, '').split('?')[0].split('/')[0];
  if (!first) return 'workbench';
  const id = decodeURIComponent(first);
  // 未知 id（手改地址、旧链接）落回工作台，不给 UnwiredModule 一个查无出处的 id。
  return pageTitleById[id] !== undefined ? id : 'workbench';
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

export function Layout() {
  const [active, setActiveState] = useState<string>(moduleIdFromHash);

  useEffect(() => {
    // 首帧也记一次：刷新回到详情页、从收藏直接打开，都是「打开过」；同一对象去重置顶，重复记不会多出一条。
    recordRecentObjectFromHash();
    const onHashChange = () => {
      setActiveState(moduleIdFromHash());
      recordRecentObjectFromHash();
    };
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  const setActive = (id: string) => {
    window.location.hash = `#/${id}`;
  };
  const sidebarResize = useResize({
    direction: 'horizontal',
    initialSize: 240,
    minSize: 170,
    maxSize: 500,
  });
  const { density } = useDensity();
  const principal = useSessionPrincipal();
  const [commandPaletteOpen, setCommandPaletteOpen] = useState(false);
  const openCommandPalette = () => setCommandPaletteOpen(true);

  const renderActive = () => {
    if (active === 'workbench') return <Workbench onNavigate={setActive} />;
    const ActivePage = pageById[active];
    return ActivePage ? <ActivePage /> : <UnwiredModule moduleId={active} />;
  };

  return (
    <div className="h-screen w-screen flex flex-col bg-idpxyz-bg text-idpxyz-text overflow-hidden">
      <TopBar
        moduleTitle={pageTitleById[active] || active}
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
          activeFile={active}
          navigationSections={navigationSections}
          iconMap={sidebarIconMap}
        />
        <div className="resize-handle-h" onMouseDown={sidebarResize.handleMouseDown} />
        {/* main 地标：读屏用户跳过导航直达页面内容的锚点。data-density 是密度两档在 DOM 上的落点（票面第 4 条），
            给只能从 DOM 读档的消费者（样式选择器、探针）用；ListPageTemplate 走的是 useDensity，不读它。 */}
        <main className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor" data-density={density}>
          {renderActive()}
        </main>
      </div>
    </div>
  );
}
