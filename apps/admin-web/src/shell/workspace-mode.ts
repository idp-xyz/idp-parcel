// 顶栏业务工作区切换（国际小包 | 仓配系统）。
// 纯逻辑、不引 React，node:test 直接钉。两个工作区各自维护独立的左侧菜单，互不混杂。
//
// 仓配履约是可选能力。部署开关明确关着时只剩国际小包一个工作区（判读见 warehouse-switch.ts）：`resolveWorkspaceMode`、
// `loadWorkspaceMode` 与 `isWarehouseWithdrawn` 都收 `warehouseOffered`，为假即「仓配不可用」；`resolveWorkspaceMode` 与
// `loadWorkspaceMode` 缺省为真——不知道开关、还没答出时与没有这个开关时逐字一样。

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

/**
 * 这个模块在本部署里被收起：仓配不可用、而它是仓配页。页面区换成「本部署没有启用仓配履约」的说明，命令面板不列它。
 * 共享设置页不在其中——扫描设备与操作者授予两个工作区共用，开关关着也照旧留在国际小包的设置里。
 */
export function isWarehouseWithdrawn(moduleId: string, warehouseOffered: boolean): boolean {
  return !warehouseOffered && isWarehouseModule(moduleId);
}

/**
 * 按当前模块推导应处的工作区；共享设置页保持原模式，避免点扫描设备或操作者授权时侧栏跳走。
 * 仓配不可用时一律是国际小包：共享设置页上也不再沿用「仓配系统」，免得侧栏停在一个不存在的工作区。
 */
export function resolveWorkspaceMode(moduleId: string, current: WorkspaceMode, warehouseOffered = true): WorkspaceMode {
  if (!warehouseOffered) return 'parcel';
  if (sharedSettingsIds.has(moduleId)) return current;
  return isWarehouseModule(moduleId) ? 'warehouse' : 'parcel';
}

/** 首次加载的工作区。仓配不可用时，本机记着「上次在仓配系统」也不再把人带进去。 */
export function loadWorkspaceMode(
  storage: Pick<Storage, 'getItem'> | undefined,
  activeModuleId: string,
  warehouseOffered = true,
): WorkspaceMode {
  if (!warehouseOffered) return 'parcel';
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
