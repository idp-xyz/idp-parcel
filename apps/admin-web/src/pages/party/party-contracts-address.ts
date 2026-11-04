// 客户与合同页「地址 → 视图」的判读（票 commercial-registers-edit-containers/02）。纯函数、只引零依赖的 address-query，node:test 直接跑。
//
// 四种视图由 hash 的 `?tab=` 定：`contracts`（缺省，合同目录工作台）、`accounts`（客户账户册）两本是列表；`publish-contract`
// （签订合同）、`register`（新增客户账户）是整页登记视图。键值沿用此前页签的值。列表之间换签只替换地址，进登记视图压一条历史、
// 出来后退或替换——做法与往来单位、服务产品页同一套。

import { hashQueryValue, hashWithQueryValue } from '../../templates/address-query';

export const PARTY_CONTRACTS_VIEW_PARAM = 'tab';

export type PartyContractsView = 'contracts' | 'accounts' | 'publish-contract' | 'register';
const views: readonly PartyContractsView[] = ['contracts', 'accounts', 'publish-contract', 'register'];

export function partyContractsViewOf(hash: string): PartyContractsView {
  const tab = hashQueryValue(hash, PARTY_CONTRACTS_VIEW_PARAM);
  return (views as readonly string[]).includes(tab) ? (tab as PartyContractsView) : 'contracts';
}

/** 两本登记视图：离开它们回的是哪本列表——签订合同回合同，新增账户回账户。 */
export function partyContractsListOf(view: PartyContractsView): 'contracts' | 'accounts' {
  return view === 'register' || view === 'accounts' ? 'accounts' : 'contracts';
}

export function isPartyContractsRegisterView(view: PartyContractsView): view is 'publish-contract' | 'register' {
  return view === 'publish-contract' || view === 'register';
}

/** 写回地址：只改这一个键，路径与检索词等别的键原样；缺省的合同目录不写，地址保持最短。 */
export function partyContractsViewHash(hash: string, view: PartyContractsView): string {
  return hashWithQueryValue(hash, PARTY_CONTRACTS_VIEW_PARAM, view === 'contracts' ? '' : view);
}
