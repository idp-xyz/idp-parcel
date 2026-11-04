// 供应商协议页「地址 → 视图」的判读（票 commercial-registers-edit-containers/03）。纯函数、只引零依赖的 address-query，node:test 直接跑。
// 两种视图由 hash 的 `?tab=` 定：缺省是目录工作台；`publish` 是「签订协议」登记视图。做法与服务产品页同一套。

import { hashQueryValue, hashWithQueryValue } from '../../templates/address-query';

export const SUPPLIER_AGREEMENTS_VIEW_PARAM = 'tab';

export type SupplierAgreementsView = 'catalogue' | 'publish';

export function supplierAgreementsViewOf(hash: string): SupplierAgreementsView {
  return hashQueryValue(hash, SUPPLIER_AGREEMENTS_VIEW_PARAM) === 'publish' ? 'publish' : 'catalogue';
}

/** 进出登记视图的地址：只改这一个键，路径与检索词等别的键原样；回目录时键不写。 */
export function supplierAgreementsViewHash(hash: string, view: SupplierAgreementsView): string {
  return hashWithQueryValue(hash, SUPPLIER_AGREEMENTS_VIEW_PARAM, view === 'catalogue' ? '' : view);
}
