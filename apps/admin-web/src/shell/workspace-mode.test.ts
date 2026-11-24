import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import {
  WORKSPACE_MODE_STORAGE_KEY,
  isWarehouseModule,
  loadWorkspaceMode,
  resolveWorkspaceMode,
  workspaceHomeModule,
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
