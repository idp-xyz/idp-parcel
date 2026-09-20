import type { ListDensity } from '@idpxyz/ui-theme-runtime';

// 列表页模板的结构位（idp-ui@53df1666「IDP Monitor Page Golden Standard」的 Filter Bar 黄金标准 9.2 顺序与 Rule 2）。
// 抽成纯函数只为一件事：模板本体是 .tsx、依赖 ESM-only 的 @idpxyz 原语，现有 run-tests（CommonJS 发射）加载不了它，
// 「哪几个位、什么顺序、不传时怎么说」这几条要钉的规则于是抬到这里，用 node:test 钉。

/**
 * 密度两档对应的行内边距（黄金标准「Table 黄金标准」11.5；两个值照 loms-web `cellPy`）。
 * 只给 Tailwind 类名，不给像素：与 TableCell 原语自带的 py-2 经 cn 合并时后者让位，两档都不等于今天的 py-2，
 * 所以切换密度是全站可见的变化，不是「comfortable = 老样子」。
 */
export function densityRowPadding(density: ListDensity): string {
  return density === 'compact' ? 'py-1' : 'py-2.5';
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
 * 按调用方接了哪些位算出四个结构位的状态。视图模式位永远禁用：本仓列表只有表格视图
 * （票 admin-web-ux-alignment/03「不做」——Board / Timeline 不在本轮），留位是为了结构位置一致。
 */
export function filterBarSlots(inputs: FilterBarSlotInputs): FilterBarSlot[] {
  return [
    inputs.sort
      ? { id: 'sort', label: '排序', enabled: true }
      : { id: 'sort', label: '排序', enabled: false, disabledReason: '本页尚未提供排序' },
    { id: 'view-mode', label: '视图：表格', enabled: false, disabledReason: '本页只有表格视图' },
    inputs.savedViews
      ? { id: 'saved-view', label: '保存视图', enabled: true }
      : { id: 'saved-view', label: '保存视图', enabled: false, disabledReason: '本页尚未接入保存视图' },
    inputs.moreFilters
      ? { id: 'more-filters', label: '更多筛选', enabled: true }
      : { id: 'more-filters', label: '更多筛选', enabled: false, disabledReason: '本页没有更多筛选项' },
  ];
}
