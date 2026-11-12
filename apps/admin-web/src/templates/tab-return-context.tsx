import { createContext, useContext } from 'react';

// 页面「回到某张标签」的口（票 admin-web-workspace-form/06）：详情页的「返回列表」经它回到列表那张标签上次停的地址
// （连检索词的 `?q=`），不再写死模块首址把地址里的页内状态剥掉。落到哪个地址由壳层定，页面只说回哪张。
//
// 默认值直接写那张标签的首址：没有壳层的地方（探针、没装多标签的宿主）照旧回得去，只是不带上次的查询串。
//
// 另两样是给「为一次操作临时开的标签」用的（票 business-party-redesign/14）：从列表的右栏进对象页的编辑态，壳层按地址为它开
// 一张标签；退出回列表之后，那张人没有自己开过的标签不该留在标签条上。页面在进去之前问一句那张标签开没开着（isOpen），退出时
// 据此决定是只回去（returnTo）还是连那张一并关掉（closeAndReturnTo）。

export interface TabReturnController {
  /** 回到这张标签；id 即 hash 的前两段（如 `shipment-request-inquiry`）。 */
  returnTo(tabId: string): void;
  /** 这张标签此刻在不在标签条上。 */
  isOpen(tabId: string): boolean;
  /**
   * 回到 `returnTabId` 那张标签，并把 `closeTabId` 那一张关掉。回程与 returnTo 同一条（落到那张标签上次停的地址；它没开着
   * 就为它开一张）。关掉的进「最近关闭」，重开的快捷键照旧找得回。
   */
  closeAndReturnTo(closeTabId: string, returnTabId: string): void;
}

const firstAddressController: TabReturnController = {
  returnTo(tabId) {
    window.location.hash = `#/${tabId}`;
  },
  // 没有壳层就没有标签条：哪一张都算没开着，「关掉」也只剩回去这一半。
  isOpen() {
    return false;
  },
  closeAndReturnTo(_closeTabId, returnTabId) {
    window.location.hash = `#/${returnTabId}`;
  },
};

const TabReturnContext = createContext<TabReturnController>(firstAddressController);

export const TabReturnProvider = TabReturnContext.Provider;

export function useTabReturn(): TabReturnController {
  return useContext(TabReturnContext);
}
