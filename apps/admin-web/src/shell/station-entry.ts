// 从外壳里的链接点进作业台之前的那一问（纯 TypeScript，node:test 直接测）。
//
// 作业台是不带外壳的全屏页：进去就把 Layout 卸掉，外壳里各标签的页面连同没提交的输入一起卸掉。标签集与活动标签存在本机
// 存储里，回来能恢复；页面里填了一半的草稿恢复不了，丢得一声不响正是 templates/unsaved-input.ts 与 beforeunload 的确认要防的。
// 那个文件登记着哪些标签此刻带着没提交的输入，这里只管判断与措辞；取标签、问人、改地址由 enter-station.ts 接线。

/** 带着没提交输入的标签，按给定的次序。 */
export function tabsWithUnsavedInput(tabIds: readonly string[], hasUnsavedInput: (tabId: string) => boolean): string[] {
  return tabIds.filter((tabId) => hasUnsavedInput(tabId));
}

const previewLimit = 3;

/** 同一个模块可以开几张标签，标题重复的只说一次；写得太长的取前几个，其余写个数。 */
function titlesPreview(titles: readonly string[]): string {
  const unique = [...new Set(titles)];
  if (unique.length <= previewLimit) return unique.join('、');
  return `${unique.slice(0, previewLimit).join('、')} 等 ${unique.length} 个`;
}

export function leaveShellQuestion(titles: readonly string[]): string {
  return (
    `管理台里「${titlesPreview(titles)}」标签上有没提交的输入。作业台是全屏页，进去会把管理台关掉，这些输入会丢。\n\n` +
    '还是要进作业台吗？'
  );
}

export interface StationEntryDeps {
  /** 外壳此刻开着的标签。 */
  openTabIds(): string[];
  hasUnsavedInput(tabId: string): boolean;
  titleOf(tabId: string): string;
  /** 问一句，答「继续」为真。 */
  ask(message: string): boolean;
  go(hash: string): void;
}

/**
 * 进作业台。没有带没提交输入的标签就直接进；有就问一次，答不继续就留在原地、不改地址。
 * 返回有没有进去。
 */
export function enterStationWith(address: string, deps: StationEntryDeps): boolean {
  const unsaved = tabsWithUnsavedInput(deps.openTabIds(), deps.hasUnsavedInput);
  if (unsaved.length > 0 && !deps.ask(leaveShellQuestion(unsaved.map((tabId) => deps.titleOf(tabId))))) return false;
  deps.go(address);
  return true;
}
