// 单据工作台模板的纯逻辑（票 admin-web-group-legal-entities/15）。形态照 idp-prism 采购订单工作台
// （prism-web `shared/workbench/DocumentWorkbench.tsx`）：键盘优先、表头点排序。模板本体依赖 ESM-only 的
// @idpxyz 原语，run-tests 的 CommonJS 发射加载不了它，要钉的规则抬到这里用 node:test 钉。

/** 表头排序态。dir 1 升序、-1 降序。 */
export interface WorkbenchSort {
  key: string;
  dir: 1 | -1;
}

/** 点同一列翻方向；点另一列从降序起——与参照页同一口径，新点的列先看「最新 / 最大」那头。 */
export function toggleWorkbenchSort(current: WorkbenchSort, key: string): WorkbenchSort {
  if (current.key === key) return { key, dir: current.dir === 1 ? -1 : 1 };
  return { key, dir: -1 };
}

/** 工作台认的键盘动作。 */
export type WorkbenchKeyAction = 'focus-search' | 'close-detail' | 'select-next' | 'select-previous';

/**
 * 按键 → 动作。在输入框里只认不了任何键：`/` 与方向键在那里是在打字、移光标。Esc 只在详情开着时算关详情，
 * 否则不占它（壳层与弹层可能要用）。
 */
export function workbenchKeyAction(key: string, editing: boolean, detailOpen: boolean): WorkbenchKeyAction | null {
  if (editing) return null;
  if (key === '/') return 'focus-search';
  if (key === 'Escape') return detailOpen ? 'close-detail' : null;
  if (key === 'ArrowDown') return 'select-next';
  if (key === 'ArrowUp') return 'select-previous';
  return null;
}

/**
 * 方向键换行：没有选中时向下落在第一行、向上落在最后一行；到头不绕回。选中的行已被筛掉时同「没有选中」。
 * 空表答 null。
 */
export function stepSelection(
  keys: readonly string[],
  current: string | null,
  action: 'select-next' | 'select-previous',
): string | null {
  if (keys.length === 0) return null;
  const index = current === null ? -1 : keys.indexOf(current);
  if (index === -1) return action === 'select-next' ? keys[0] : keys[keys.length - 1];
  const next = action === 'select-next' ? Math.min(index + 1, keys.length - 1) : Math.max(index - 1, 0);
  return keys[next];
}
