import { test } from 'node:test';
import { equal, ok } from 'node:assert/strict';
import { parcelNavigationSections, warehouseNavigationSections } from '../navigation';
import {
  WORKSPACE_MODE_STORAGE_KEY,
  isWarehouseModule,
  isWarehouseWithdrawn,
  loadWorkspaceMode,
  resolveWorkspaceMode,
  workspaceHomeModule,
  type WorkspaceMode,
} from './workspace-mode';

test('仓配模块与小包模块各自落在对应的工作区，共享设置页保持原工作区', () => {
  equal(isWarehouseModule('warehouse-dashboard'), true);
  equal(isWarehouseModule('warehouse-outbound'), true);
  equal(isWarehouseModule('stock-imports'), true);
  equal(isWarehouseModule('workbench'), false);
  equal(isWarehouseModule('shipment-request-inquiry'), false);

  equal(resolveWorkspaceMode('warehouse-stock', 'parcel'), 'warehouse');
  equal(resolveWorkspaceMode('shipment-request-inquiry', 'warehouse'), 'parcel');
  equal(resolveWorkspaceMode('work-devices', 'warehouse'), 'warehouse');
  equal(resolveWorkspaceMode('work-devices', 'parcel'), 'parcel');
  equal(resolveWorkspaceMode('operator-access', 'warehouse'), 'warehouse');
  equal(workspaceHomeModule.warehouse, 'warehouse-dashboard');
  equal(workspaceHomeModule.parcel, 'workbench');
});

test('首次加载按地址栏优先，其次读本地记忆', () => {
  const store = {
    getItem(key: string) {
      return key === WORKSPACE_MODE_STORAGE_KEY ? 'warehouse' : null;
    },
  };
  equal(loadWorkspaceMode(store, 'work-devices'), 'warehouse');
  equal(loadWorkspaceMode(store, 'shipment-request-inquiry'), 'parcel');
  equal(loadWorkspaceMode(undefined, 'warehouse-workstation'), 'warehouse');
});

// 仓配部署开关（shell/warehouse-switch.ts）带来的用例：关着时 warehouseOffered 为假；开着、不知道、还没答出时为真，
// 也是缺省——这些情形要与没有这个开关时逐字一样，所以期望值写成字面表，不拿函数与它自己比。

const sharedSettingsPages = ['work-devices', 'operator-access'];
const warehousePages = ['warehouse-dashboard', 'warehouse-workstation', 'warehouse-outbound', 'warehouse-settings', 'stock-imports'];
const parcelPages = ['workbench', 'shipment-request-inquiry', 'recent-objects', 'pending-publications'];

// Covers: 关着时 `warehouse` 一律改写成 `parcel`——仓配页、小包页都一样，共享设置页上留着的「仓配系统」也不例外。
test('仓配不可用时工作区恒为国际小包，不论当前停在哪一页、先前是哪个工作区', () => {
  const current: readonly WorkspaceMode[] = ['parcel', 'warehouse'];
  for (const moduleId of [...warehousePages, ...parcelPages, ...sharedSettingsPages]) {
    for (const from of current) {
      equal(resolveWorkspaceMode(moduleId, from, false), 'parcel', `${moduleId}（原 ${from}）`);
    }
  }
});

// Covers: 开着或不知道时的推导与现状逐字一致：显式传真与不传同一个答案，且都是原来的那张表。
test('仓配可用或不知道时，工作区推导与开关进来之前逐字一致', () => {
  const table: ReadonlyArray<readonly [string, WorkspaceMode, WorkspaceMode]> = [
    ['warehouse-stock', 'parcel', 'warehouse'],
    ['warehouse-stock', 'warehouse', 'warehouse'],
    ['stock-imports', 'parcel', 'warehouse'],
    ['shipment-request-inquiry', 'warehouse', 'parcel'],
    ['shipment-request-inquiry', 'parcel', 'parcel'],
    ['workbench', 'warehouse', 'parcel'],
    ['work-devices', 'warehouse', 'warehouse'],
    ['work-devices', 'parcel', 'parcel'],
    ['operator-access', 'warehouse', 'warehouse'],
    ['operator-access', 'parcel', 'parcel'],
  ];
  for (const [moduleId, from, want] of table) {
    equal(resolveWorkspaceMode(moduleId, from), want, `${moduleId}（原 ${from}，不传）`);
    equal(resolveWorkspaceMode(moduleId, from, true), want, `${moduleId}（原 ${from}，传真）`);
  }
});

// Covers: 关着时首次加载不进仓配——本机记着「上次在仓配系统」、地址落在仓配页、共享设置页上带着旧记忆，都不行。
test('仓配不可用时首次加载是国际小包，本机记着上次在仓配系统也不把人带进去', () => {
  const rememberedWarehouse = {
    getItem(key: string) {
      return key === WORKSPACE_MODE_STORAGE_KEY ? 'warehouse' : null;
    },
  };
  for (const moduleId of [...sharedSettingsPages, ...warehousePages, ...parcelPages]) {
    equal(loadWorkspaceMode(rememberedWarehouse, moduleId, false), 'parcel', moduleId);
    equal(loadWorkspaceMode(undefined, moduleId, false), 'parcel', `${moduleId}（没有本机存储）`);
  }
});

test('仓配可用或不知道时，首次加载与开关进来之前逐字一致', () => {
  const rememberedWarehouse = {
    getItem(key: string) {
      return key === WORKSPACE_MODE_STORAGE_KEY ? 'warehouse' : null;
    },
  };
  const rememberedParcel = {
    getItem(key: string) {
      return key === WORKSPACE_MODE_STORAGE_KEY ? 'parcel' : null;
    },
  };
  type ModeStorage = Parameters<typeof loadWorkspaceMode>[0];
  const loads: ReadonlyArray<(storage: ModeStorage, moduleId: string) => WorkspaceMode> = [
    (storage, moduleId) => loadWorkspaceMode(storage, moduleId),
    (storage, moduleId) => loadWorkspaceMode(storage, moduleId, true),
  ];
  for (const load of loads) {
    equal(load(rememberedWarehouse, 'work-devices'), 'warehouse');
    equal(load(rememberedParcel, 'work-devices'), 'parcel');
    equal(load(rememberedWarehouse, 'shipment-request-inquiry'), 'parcel');
    equal(load(rememberedParcel, 'warehouse-outbound'), 'warehouse');
    equal(load(undefined, 'warehouse-workstation'), 'warehouse');
    equal(load(undefined, 'workbench'), 'parcel');
  }
});

// Covers: 关着时被收起的只有仓配页；共享设置页（扫描设备、操作者与授予）与小包页照旧；开着时一个都不收起。
test('仓配不可用时只有仓配页被收起，共享设置页与小包页不受影响', () => {
  for (const moduleId of warehousePages) {
    equal(isWarehouseWithdrawn(moduleId, false), true, `${moduleId} 关着时收起`);
    equal(isWarehouseWithdrawn(moduleId, true), false, `${moduleId} 开着时不收起`);
  }
  for (const moduleId of [...sharedSettingsPages, ...parcelPages]) {
    equal(isWarehouseWithdrawn(moduleId, false), false, `${moduleId} 关着时也不收起`);
    equal(isWarehouseWithdrawn(moduleId, true), false, `${moduleId} 开着时不收起`);
  }
});

// Covers: 判据（模块 id 的写法）与导航表对得上——仓配导航里除共享设置页外的条目一个不漏地被判成仓配页，国际小包导航里一个也不被收起，
// 共享设置页也确实挂在国际小包的设置里。新增仓配页若用了不合写法的 id，关着时它会漏出来，这条就红。
test('关着时被收起的恰是仓配导航里的仓配页，国际小包导航整份留下，共享设置页留在小包设置里', () => {
  const warehouseIds = warehouseNavigationSections.flatMap((section) => section.items.map((item) => item.id));
  const ownIds = warehouseIds.filter((id) => !sharedSettingsPages.includes(id));
  ok(ownIds.length > 0);
  for (const id of ownIds) equal(isWarehouseWithdrawn(id, false), true, `${id} 是仓配页，关着时该收起`);

  const parcelIds = parcelNavigationSections.flatMap((section) => section.items.map((item) => item.id));
  for (const id of parcelIds) equal(isWarehouseWithdrawn(id, false), false, `${id} 在小包导航里，不该被收起`);
  for (const id of sharedSettingsPages) {
    ok(parcelIds.includes(id), `${id} 留在国际小包导航里`);
    ok(warehouseIds.includes(id), `${id} 在仓配导航里也有，两个工作区共用`);
  }
});
