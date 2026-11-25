import { pageTitleById } from '../navigation';
import { tabHasUnsavedInput } from '../templates/unsaved-input';
import { enterStationWith } from './station-entry';
import { loadWorkspaceState, moduleIdOfTab } from './workspace-state';

/**
 * 从外壳里的链接点进作业台：先问一句再走（有没提交输入的标签时），判断在 station-entry.ts。
 *
 * 开着的标签读本机存储里那份工作区状态，不向 Layout 要：外壳每改一次标签就存一次，这里读到的就是此刻的标签条，Layout 不用
 * 为了这一问多露一个口。存储读不了（禁用存储）当没有标签，不拦人。
 */
export function enterStation(address: string): boolean {
  return enterStationWith(address, {
    openTabIds: () => {
      try {
        return loadWorkspaceState(window.localStorage, pageTitleById).tabs.map((tab) => tab.id);
      } catch {
        return [];
      }
    },
    hasUnsavedInput: tabHasUnsavedInput,
    titleOf: (tabId) => pageTitleById[moduleIdOfTab(tabId)] ?? tabId,
    ask: (message) => window.confirm(message),
    go: (hash) => {
      window.location.hash = hash;
    },
  });
}
