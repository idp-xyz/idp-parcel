import type { ListDensity } from '@idpxyz/ui-theme-runtime';

// 列表页模板的结构位（idp-ui@53df1666「IDP Monitor Page Golden Standard」的 Filter Bar 黄金标准 9.2 顺序与 Rule 2）。
// 抽成纯函数只为一件事：模板本体是 .tsx、依赖 ESM-only 的 @idpxyz 原语，现有 run-tests（CommonJS 发射）加载不了它，
// 「哪几个位、什么顺序、不传时怎么说」这几条要钉的规则于是抬到这里，用 node:test 钉。

/**
 * 密度两档对应的行内边距（黄金标准「Table 黄金标准」11.5；两个值照 loms-web `cellPy`）。
 * 只给 Tailwind 类名，不给像素：与 TableCell 原语自带的 py-2 经 cn 合并时后者让位，两档都不等于原语的 py-2，
 * 所以切换密度是全站可见的变化，不是「comfortable = 原语默认」。
 */
export function densityRowPadding(density: ListDensity): string {
  return density === 'compact' ? 'py-1' : 'py-2.5';
}

/** 行交互的输入：调用方接了哪些行回调。 */
export interface RowInteractionInputs {
  /** 接了 onRowClick 或 inspector（单击：预览 / 选中 / 交给检查器）。 */
  click: boolean;
  /** 接了 onRowOpen（双击 / Enter：开对象）。 */
  open: boolean;
  /** 接了 inspector（聚焦即交给检查器）。 */
  inspect?: boolean;
}

export interface RowInteraction {
  /**
   * 行进不进 Tab 序。接了 onRowOpen（键盘要能「开」）或 inspector（键盘要能把行送进检查器）才给 0；都没接就不给，
   * 只接 onRowClick 的页行属性不变。不用正数：正数会抢整页的 Tab 顺序。
   */
  tabIndex: 0 | undefined;
  /** 任一行回调存在即可点（指针样式）：双击本身也是指针动作。 */
  clickable: boolean;
}

/**
 * 按调用方接了哪些行回调算出行的交互属性（手册「Table 规范」「Drill-down 模式」：单击预览、双击开对象；
 * 手册「可访问性」表格键盘导航：行可聚焦、Enter 等价双击）。
 */
export function rowInteraction(inputs: RowInteractionInputs): RowInteraction {
  return {
    tabIndex: inputs.open || inputs.inspect ? 0 : undefined,
    clickable: inputs.click || inputs.open,
  };
}

/**
 * 键盘上等价双击的键。只认 Enter（KeyboardEvent.key；小键盘回车的 key 也是 Enter）：Space 在滚动容器里是翻页键、
 * 方向键留给浏览器滚动，行不能吞它们。
 */
export function rowKeyOpens(key: string): boolean {
  return key === 'Enter';
}

/** Filter Bar 在搜索区与主筛选之后的四个结构位，顺序即渲染顺序。 */
export type FilterBarSlotId = 'sort' | 'view-mode' | 'saved-view' | 'more-filters';

export interface FilterBarSlot {
  id: FilterBarSlotId;
  /** 位上按钮的文案；禁用态也用同一个词，位置与词都不因功能未接而变。 */
  label: string;
  /** 调用方接了这个位就 true；false 时渲染禁用按钮 + 悬停说明（spec 红线「留位不留假动作」）。 */
  enabled: boolean;
  /** 禁用时的悬停说明，说清「为什么没有」而不是「即将推出」；enabled 时无。 */
  disabledReason?: string;
}

export interface FilterBarSlotInputs {
  sort: boolean;
  savedViews: boolean;
  moreFilters: boolean;
}

/**
 * 过滤条上真正接了动作的位。没接的不占位：一个不能点的按钮，悬停才说明为什么，读起来像坏了。
 * 顺序仍是排序、保存视图、更多筛选。视图只有表格一种，不另占一个不能点的位。
 */
export function filterBarSlots(inputs: FilterBarSlotInputs): FilterBarSlot[] {
  const slots: FilterBarSlot[] = [];
  if (inputs.sort) slots.push({ id: 'sort', label: '排序', enabled: true });
  if (inputs.savedViews) slots.push({ id: 'saved-view', label: '保存视图', enabled: true });
  if (inputs.moreFilters) slots.push({ id: 'more-filters', label: '更多筛选', enabled: true });
  return slots;
}
