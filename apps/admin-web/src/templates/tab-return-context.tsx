import { createContext, useContext } from 'react';

// 页面「回到某张标签」的口（票 admin-web-workspace-form/06）：详情页的「返回列表」经它回到列表那张标签上次停的地址
// （连检索词的 `?q=`），不再写死模块首址把地址里的页内状态剥掉。落到哪个地址由壳层定，页面只说回哪张。
//
// 默认值直接写那张标签的首址：没有壳层的地方（探针、没装多标签的宿主）照旧回得去，只是不带上次的查询串。

export interface TabReturnController {
  /** 回到这张标签；id 即 hash 的前两段（如 `shipment-request-inquiry`）。 */
  returnTo(tabId: string): void;
}

const firstAddressController: TabReturnController = {
  returnTo(tabId) {
    window.location.hash = `#/${tabId}`;
  },
};

const TabReturnContext = createContext<TabReturnController>(firstAddressController);

export const TabReturnProvider = TabReturnContext.Provider;

export function useTabReturn(): TabReturnController {
  return useContext(TabReturnContext);
}
