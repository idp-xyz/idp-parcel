import type { ContactSocialAccountRecord, LegalEntityContactRecord } from './api';
import { contactPurposeLabels } from './legal-entity-profile';

// 联系人卡的判读（票 company-subject-ux/11 第 1 条）：卡里哪几行显、用途胶囊怎么折算、社交账号怎么串，都在这里定，
// 组件 LegalEntityContactCards.tsx 只照着摆。放纯函数是为了在 node:test 里钉——卡的空行规则（没有的项整行不显、不画「—」）
// 与读出表那套相反，这里判错了页面上只是少一行，没人会报。

export interface ContactCardPurpose {
  code: string;
  label: string;
  /** 这位是该用途的主要联系人（资料的 primaryFor 含此用途）。 */
  primary: boolean;
}

export interface ContactCard {
  name: string;
  /** 「部门 · 职务」，只有一个就只显那一个；两个都没有则缺席，整行不显。 */
  affiliation?: string;
  purposes: ContactCardPurpose[];
  phone?: string;
  email?: string;
  /** 账号为空的条目已剔掉；空数组即整行不显。 */
  socialAccounts: ContactSocialAccountRecord[];
}

export function contactCardOf(contact: LegalEntityContactRecord): ContactCard {
  const primaryFor = new Set<string>(contact.primaryFor ?? []);
  const purposes: ContactCardPurpose[] = (contact.purposes ?? []).map((code) => ({
    code,
    label: contactPurposeLabels[code] ?? code,
    primary: primaryFor.has(code),
  }));
  // primaryFor 里有、purposes 里没有的用途：表单校验不放行这种组合，但旧资料或批量口可能带进来。「主要」是比
  // 「有此用途」更强的断言，补成一颗胶囊，而不是因为没列在用途里就把它丢掉。
  for (const code of contact.primaryFor ?? []) {
    if (!purposes.some((purpose) => purpose.code === code)) purposes.push({ code, label: contactPurposeLabels[code] ?? code, primary: true });
  }
  return {
    name: contact.name,
    affiliation: joinPresent([contact.department, contact.position]),
    purposes,
    phone: present(contact.phone),
    email: present(contact.email),
    socialAccounts: (contact.socialAccounts ?? [])
      .map((account) => ({ platform: account.platform.trim(), account: account.account.trim() }))
      .filter((account) => account.account !== ''),
  };
}

/**
 * 社交账号串成一行的文字形（「微信 SYN-1 · 企业微信 SYN-2」），给悬停与读屏用；页面上平台与账号各自着色，
 * 分隔与顺序与这一句一致，改一处另一处就跟着错，所以组件也从这里取分隔符。
 */
export const socialAccountSeparator = ' · ';

export function socialAccountsText(accounts: readonly ContactSocialAccountRecord[]): string {
  return accounts.map((account) => (account.platform ? `${account.platform} ${account.account}` : account.account)).join(socialAccountSeparator);
}

function present(value: string | undefined): string | undefined {
  const trimmed = value?.trim();
  return trimmed ? trimmed : undefined;
}

function joinPresent(values: readonly (string | undefined)[]): string | undefined {
  const kept = values.map(present).filter((value): value is string => value !== undefined);
  return kept.length ? kept.join(' · ') : undefined;
}
