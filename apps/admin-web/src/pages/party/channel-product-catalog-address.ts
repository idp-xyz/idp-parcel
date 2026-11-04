// 渠道产品目录页「地址 → 视图」的判读（票 commercial-registers-edit-containers/05）。纯函数、只引零依赖的 address-query，node:test 直接跑。
// 两种视图由 hash 的 `?tab=` 定：缺省是目录工作台；`register` 是「设置产品渠道」登记视图，键名沿用此前那个签的值。

import { hashQueryValue, hashWithQueryValue } from '../../templates/address-query';

export const CHANNEL_PRODUCT_CATALOG_VIEW_PARAM = 'tab';

export type ChannelProductCatalogView = 'catalogue' | 'register';

export function channelProductCatalogViewOf(hash: string): ChannelProductCatalogView {
  return hashQueryValue(hash, CHANNEL_PRODUCT_CATALOG_VIEW_PARAM) === 'register' ? 'register' : 'catalogue';
}

/** 进出登记视图的地址：只改这一个键，路径与检索词等别的键原样；回目录时键不写。 */
export function channelProductCatalogViewHash(hash: string, view: ChannelProductCatalogView): string {
  return hashWithQueryValue(hash, CHANNEL_PRODUCT_CATALOG_VIEW_PARAM, view === 'catalogue' ? '' : view);
}
