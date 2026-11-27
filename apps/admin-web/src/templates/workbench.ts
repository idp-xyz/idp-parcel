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
 * 按键时焦点在哪：输入框里（typing）、菜单 / 弹层 / 下拉这类自己收 Esc 与方向键的部件里（overlay）、
 * 页签条里（tablist，方向键归它切签，Esc 不归它）、别处（free）。
 */
export type WorkbenchFocus = 'free' | 'typing' | 'overlay' | 'tablist';

/**
 * 按键 → 动作。输入框与弹层里一个键都不占；页签条里只让出方向键与 `/`，Esc 照旧收详情——详情栏的页签条就在栏里，
 * 点过页签后 Esc 收不了栏会让人以为键盘坏了。Esc 只在详情真开着时算关详情，否则不占它（壳层与弹层可能要用）。
 */
export function workbenchKeyAction(key: string, focus: WorkbenchFocus, detailOpen: boolean): WorkbenchKeyAction | null {
  if (focus === 'typing' || focus === 'overlay') return null;
  if (key === 'Escape') return detailOpen ? 'close-detail' : null;
  if (focus === 'tablist') return null;
  if (key === '/') return 'focus-search';
  if (key === 'ArrowDown') return 'select-next';
  if (key === 'ArrowUp') return 'select-previous';
  return null;
}

/**
 * 把一行滚进列表可视区，scrollTop 要加的量。
 * 表头粘在滚动区顶上，横向滚动条占着底边：这两截都不算看得见。
 * 不算顶上的，滚回第一行会停在表头底下；不算底下的，最后一行停在横向滚动条后面，滚动条也到不了头。
 * listBottom 是滚动区外框的底。bottom 是从外框底边让出的高度（下边框加横向滚动条）。
 * 只动这一层的 scrollTop，不带动外层能滚的容器。
 */
export function scrollDeltaToReveal(box: {
  listTop: number;
  listBottom: number;
  rowTop: number;
  rowBottom: number;
  header: number;
  bottom?: number;
}): number {
  const topLimit = box.listTop + box.header;
  const bottomLimit = box.listBottom - (box.bottom ?? 0);
  if (box.rowTop < topLimit) return box.rowTop - topLimit;
  if (box.rowBottom > bottomLimit) return box.rowBottom - bottomLimit;
  return 0;
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
