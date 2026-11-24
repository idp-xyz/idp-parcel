// 顶栏业务工作区切换（国际小包 | 仓配系统）。
// 纯逻辑、不引 React，node:test 直接钉。两个工作区各自维护独立的左侧菜单，互不混杂。

export type WorkspaceMode = 'parcel' | 'warehouse';

export const WORKSPACE_MODE_STORAGE_KEY = 'idp-parcel.workspace-mode';

export const workspaceModeLabels: Record<WorkspaceMode, string> = {
  parcel: '国际小包',
  warehouse: '仓配系统',
};

export const workspaceHomeModule: Record<WorkspaceMode, string> = {
  parcel: 'workbench',
  warehouse: 'warehouse-dashboard',
};

/** 两个工作区共用的基础设置页（切过去不强制改变当前工作区模式）。 */
const sharedSettingsIds = new Set(['work-devices', 'operator-access']);

export function isWarehouseModule(moduleId: string): boolean {
  return moduleId === 'stock-imports' || moduleId.startsWith('warehouse-');
}

/** 按当前模块推导应处的工作区；共享设置页保持原模式，避免点扫描设备或操作者授权时侧栏跳走。 */
export function resolveWorkspaceMode(moduleId: string, current: WorkspaceMode): WorkspaceMode {
  if (sharedSettingsIds.has(moduleId)) return current;
  return isWarehouseModule(moduleId) ? 'warehouse' : 'parcel';
}

export function loadWorkspaceMode(storage: Pick<Storage, 'getItem'> | undefined, activeModuleId: string): WorkspaceMode {
  if (isWarehouseModule(activeModuleId)) return 'warehouse';
  try {
    const saved = storage?.getItem(WORKSPACE_MODE_STORAGE_KEY);
    if (saved === 'warehouse' || saved === 'parcel') {
      return resolveWorkspaceMode(activeModuleId, saved);
    }
  } catch {
    // 禁用本地存储时按当前页决定。
  }
  return 'parcel';
}

export function saveWorkspaceMode(storage: Pick<Storage, 'setItem'> | undefined, mode: WorkspaceMode): void {
  try {
    storage?.setItem(WORKSPACE_MODE_STORAGE_KEY, mode);
  } catch {
    // 禁用本地存储时忽略持久化失败。
  }
}
