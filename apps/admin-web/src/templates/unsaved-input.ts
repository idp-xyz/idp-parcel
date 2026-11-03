import { useEffect } from 'react';
import { useCurrentTabId } from './current-tab';

// 各标签上有没有没提交的输入。外壳据此决定离开满新鲜期回来时能不能按新开页重挂：重挂会把藏着的那张登记表单连同草稿一起
// 丢掉，而人离开时那张表单还挂着 beforeunload 的确认，说明它在乎——丢得一声不响正是那条确认要防的。所以有没提交的输入
// 的标签回来时不重挂，照原样显，草稿还在（ADR-0360 决定四）。
// 按标签记计数：一张标签上可能有几个表单各自登记，最后一个撤下才算没有。只在本页内存里。

const unsavedCountByTab = new Map<string, number>();

/** 这张标签上此刻有没有没提交的输入。 */
export function tabHasUnsavedInput(tabId: string): boolean {
  return (unsavedCountByTab.get(tabId) ?? 0) > 0;
}

/**
 * 表单有没提交的输入时给本页所在的标签登记一笔，输入提交或表单卸载时撤下。与 beforeunload 的确认挂在同一处、用同一个
 * 判断：刷新、关页要确认的输入，换标签回来也不该被重挂丢掉。不在工作区标签里时什么都不做。
 */
export function useMarkUnsavedInput(unsaved: boolean): void {
  const tabId = useCurrentTabId();
  useEffect(() => {
    if (!unsaved || tabId === undefined) return;
    unsavedCountByTab.set(tabId, (unsavedCountByTab.get(tabId) ?? 0) + 1);
    return () => {
      const left = (unsavedCountByTab.get(tabId) ?? 1) - 1;
      if (left > 0) unsavedCountByTab.set(tabId, left);
      else unsavedCountByTab.delete(tabId);
    };
  }, [unsaved, tabId]);
}
