// 商业规则与策略页的页内位置（票 party-commercial-uiux/05）：哪一签、哪一本册放进地址的查询串，刷新与换标签回来都还在；
// 政策册签与发布签共用同一本册，读到哪本、发布的就是哪本。另有「这本册怎么发布」的对照。
// 纯函数、只引零依赖的 address-query，node:test 直接跑。

import { hashQueryValue, hashWithQueryValue } from '../../templates/address-query';
import type { CommercialPolicyKind } from './api';
import { commercialPolicyKinds } from './presentation';

export const POLICIES_TAB_PARAM = 'tab';
export const POLICIES_REGISTER_PARAM = 'register';

export type PoliciesTab = 'registers' | 'publish' | 'json';

const tabs: readonly PoliciesTab[] = ['registers', 'publish', 'json'];
const defaultRegister: CommercialPolicyKind = 'ACCEPTANCE_RULE_PACKAGE';

export function policiesTabOf(hash: string): PoliciesTab {
  const tab = hashQueryValue(hash, POLICIES_TAB_PARAM);
  return (tabs as readonly string[]).includes(tab) ? (tab as PoliciesTab) : 'registers';
}

export function policiesRegisterOf(hash: string): CommercialPolicyKind {
  const register = hashQueryValue(hash, POLICIES_REGISTER_PARAM);
  return (commercialPolicyKinds as readonly string[]).includes(register) ? (register as CommercialPolicyKind) : defaultRegister;
}

/** 把签与册写回地址；取缺省值的那一项不写，地址保持最短。路径与别的键（检索词、保存视图）原样。 */
export function policiesHash(hash: string, place: { tab: PoliciesTab; register: CommercialPolicyKind }): string {
  const withTab = hashWithQueryValue(hash, POLICIES_TAB_PARAM, place.tab === 'registers' ? '' : place.tab);
  return hashWithQueryValue(withTab, POLICIES_REGISTER_PARAM, place.register === defaultRegister ? '' : place.register);
}

/**
 * 这本册在「发布新版本」签里怎么发：七本有自己的表单；两本没有自己的版本、随所属版本一起发布——接受前财务控制是
 * 客户合同上的声明（在客户与合同页发），接受判断时点策略挂在接单规则包上（在规则包表单里发）。
 */
export type PublishTarget =
  | { kind: 'form' }
  | { kind: 'with-rule-package' }
  | { kind: 'with-contract' };

export function publishTargetOf(register: CommercialPolicyKind): PublishTarget {
  switch (register) {
    case 'AS_OF_POLICY':
      return { kind: 'with-rule-package' };
    case 'PRE_ACCEPTANCE_CONTROL':
      return { kind: 'with-contract' };
    default:
      return { kind: 'form' };
  }
}
