// 服务产品页「地址 → 视图」的判读（票 commercial-registers-edit-containers/01）。纯函数、只引零依赖的 address-query，node:test 直接跑。
//
// 三种视图由 hash 的 `?tab=` 定：缺省是目录工作台；`publish` 是「新增产品版本」登记视图；`register` 是「登记服务形态」登记视图。
// 键名沿用此前页签的值（此前这两张表单是列表旁边的签，不进地址）；做法与往来单位的登记视图同一套——视图住在地址上，刷新与
// 后退都按地址走，进出的历史收口由页面做。

import { hashQueryValue, hashWithQueryValue } from '../../templates/address-query';

export const SERVICE_PRODUCTS_VIEW_PARAM = 'tab';

export type ServiceProductsView = 'versions' | 'publish' | 'register';

export function serviceProductsViewOf(hash: string): ServiceProductsView {
  const tab = hashQueryValue(hash, SERVICE_PRODUCTS_VIEW_PARAM);
  return tab === 'publish' || tab === 'register' ? tab : 'versions';
}

/** 进出登记视图的地址：只改这一个键，路径与检索词等别的键原样；回目录时键不写，地址保持最短。 */
export function serviceProductsViewHash(hash: string, view: ServiceProductsView): string {
  return hashWithQueryValue(hash, SERVICE_PRODUCTS_VIEW_PARAM, view === 'versions' ? '' : view);
}
