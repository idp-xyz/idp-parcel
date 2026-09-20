import type { NavItem, NavigationSection } from '@idpxyz/ui-workspace';

// 面包屑 `区 › 页` 的反查（黄金标准「Page Header / Breadcrumb 黄金标准」典型形式 `Operations > Orders`，
// idp-ui@53df1666）。放在模板层而不是导航层：导航只知道条目属于哪个分区，「把分区和页名拼成一条面包屑」
// 是页面模板的呈现决定；对象工作区模板将来要在同一条后面再接对象段，所以这里不绑定列表模板。

export interface TemplateBreadcrumb {
  /** 分区名，取导航分区标题（如「主数据」）。 */
  section: string;
  /** 页名，取 pageTitleById；查不到时退回导航条目 label——两者按 navigation.ts 的约定本就是同一个词。 */
  page: string;
}

/**
 * 按模块 id 反查其所在分区与页名。不在任何分区里的 id（手改地址、未登记的演示 id）返回 null，
 * 由调用方决定不渲染——面包屑不能为一个查无出处的 id 编一个分区。
 */
export function resolveBreadcrumb(
  moduleId: string,
  sections: readonly NavigationSection[],
  pageTitles: Readonly<Record<string, string>>,
): TemplateBreadcrumb | null {
  for (const section of sections) {
    const item = findItem(section.items, moduleId);
    if (item) {
      return { section: section.title, page: pageTitles[moduleId] ?? item.label };
    }
  }
  return null;
}

// NavItem 允许嵌套 children；本仓导航今天是平的，但一旦有人折出二级，面包屑不该无声地消失。
function findItem(items: readonly NavItem[], id: string): NavItem | undefined {
  for (const item of items) {
    if (item.id === id) return item;
    const nested = item.children ? findItem(item.children, id) : undefined;
    if (nested) return nested;
  }
  return undefined;
}
