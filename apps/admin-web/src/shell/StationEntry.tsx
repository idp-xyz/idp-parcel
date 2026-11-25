import { Suspense, lazy } from 'react';
import { PageChunkBoundary, PageLoading } from './PageChunkBoundary';
import { appScreen } from './station-address';
import { useWarehouseOffered } from './use-warehouse-switch';

// 作业台地址上 App 摆的东西：不带外壳的全屏页。整块作业台经 import() 引入，入口块不带它，也不带仓配页面——静态引
// `pages/warehouse/index.ts`（barrel）会把整个仓配域钉回入口块（page-registry.tsx 的 `loadWarehouse` 为此走 import()：
// 「入口一静态引 barrel，那个域就被钉回入口块」）；作业台放在 `pages/warehouse/station/` 下，从这里只动态引它的入口模块。
//
// 部署开关明确关着（useWarehouseOffered 为假）时摆「本部署没有启用」的说明，不渲染工位；答出之前与答不出照常渲染工位，
// 与外壳里仓配页同一个口径——服务端照旧在每个口上判开关。判断在 station-address.ts 的 appScreen。

const StationRoot = lazy(() => import('../pages/warehouse/station/StationRoot').then((m) => ({ default: m.StationRoot })));
const WarehouseNotEnabled = lazy(() => import('./WarehouseNotEnabled').then((m) => ({ default: m.WarehouseNotEnabled })));

function StationLoading() {
  return (
    <div className="flex h-screen w-screen items-center justify-center bg-idpxyz-bg text-idpxyz-text">
      <PageLoading />
    </div>
  );
}

export function StationEntry({ hash }: { hash: string }) {
  const offered = useWarehouseOffered();
  const screen = appScreen(hash, offered);
  return (
    <PageChunkBoundary>
      <Suspense fallback={<StationLoading />}>
        {screen.kind === 'station-not-enabled' ? (
          <div className="flex h-screen w-screen flex-col bg-idpxyz-bg text-idpxyz-text">
            <WarehouseNotEnabled />
            <p className="pb-8 text-center type-body">
              <a className="text-[color:var(--ds-text-link-hover)] hover:underline" href="#/workbench">
                回到管理台
              </a>
            </p>
          </div>
        ) : screen.kind === 'station' ? (
          <StationRoot hash={hash} />
        ) : null}
      </Suspense>
    </PageChunkBoundary>
  );
}
