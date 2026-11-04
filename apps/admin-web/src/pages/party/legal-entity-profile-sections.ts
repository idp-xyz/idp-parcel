// 公司资料的分区编辑（票 company-subject-section-edit/01）：读出的每张卡各有自己的「编辑」，打开只装这一组格的抽屉。
// 服务端只收整版资料（CONTEXT「法人资料」：资料链不可覆盖），分区只是界面上的分区：提交仍是「此刻的当前版 + 这一组
// 的改动」拼成的整版新资料。本文件是纯判读——分区表、路径归属、新行、来源单据的自动写法——node:test 钉着；组件只摆。

import type { LegalEntityProfileDraft, ProfileMissingField } from './legal-entity-profile';
import type { ProfileChangeKind, ProfileComparison } from './legal-entity-profile-history';

/** 四个分区，与读出的四张卡（LegalEntityProfileContent）一一对应。 */
export type ProfileSectionId = 'representativeAndAddresses' | 'invoicing' | 'bankAccounts' | 'contacts';

/** 表单里的五组格（LegalEntityProfileFields 的 FormGroup）；读出的「代表人与地址」一张卡对应前两组。 */
export type ProfileFieldGroupId = 'representative' | 'addresses' | 'invoicing' | 'bankAccounts' | 'contacts';

export interface ProfileSection {
  id: ProfileSectionId;
  title: string;
  groups: readonly ProfileFieldGroupId[];
  /** 这一区认领的 JSON 路径前缀：缺项按它分「本区的」与「别区的」。 */
  pathPrefixes: readonly string[];
  /** 差异预览里属本区的 group 名（compareLegalEntityProfiles 给每一项标的那个词）。 */
  changeGroups: readonly string[];
  /** 逐行的区：「添加」打开抽屉时先加一空行，焦点滚到那一行的第一个必填格。 */
  list?: {
    key: 'bankAccounts' | 'contacts';
    addLabel: string;
    focusPath: (index: number) => string;
  };
}

export const profileSections: Record<ProfileSectionId, ProfileSection> = {
  representativeAndAddresses: {
    id: 'representativeAndAddresses',
    title: '代表人与地址',
    groups: ['representative', 'addresses'],
    pathPrefixes: ['profiles[0].representative', 'profiles[0].registeredAddress', 'profiles[0].officeAddress', 'profiles[0].correspondenceAddress'],
    changeGroups: ['代表人信息', '地址信息'],
  },
  invoicing: {
    id: 'invoicing',
    title: '税务与开票',
    groups: ['invoicing'],
    pathPrefixes: ['profiles[0].invoiceTitle', 'profiles[0].invoiceTitleLanguage', 'profiles[0].invoiceTitles', 'profiles[0].taxRegistrationNumbers'],
    changeGroups: ['税务与开票'],
  },
  bankAccounts: {
    id: 'bankAccounts',
    title: '收款账户',
    groups: ['bankAccounts'],
    pathPrefixes: ['profiles[0].bankAccounts'],
    changeGroups: ['收款账户'],
    list: { key: 'bankAccounts', addLabel: '添加收款账户', focusPath: (index) => `profiles[0].bankAccounts[${index}].holderName` },
  },
  contacts: {
    id: 'contacts',
    title: '联系人',
    groups: ['contacts'],
    pathPrefixes: ['profiles[0].contacts'],
    changeGroups: ['联系人'],
    list: { key: 'contacts', addLabel: '添加联系人', focusPath: (index) => `profiles[0].contacts[${index}]` },
  },
};

/** 抽屉装什么：整张表单，或某一区（`addRow` 即从「添加」打开，先加一空行）。 */
export type ProfileEditScope = { kind: 'full' } | { kind: 'section'; section: ProfileSectionId; addRow: boolean };

/** 读出卡上「编辑」「添加」交给页面的请求；页面据它开抽屉。 */
export interface ProfileEditRequest {
  section: ProfileSectionId;
  addRow?: boolean;
}

export function profileEditScopeOf(request: ProfileEditRequest): ProfileEditScope {
  return { kind: 'section', section: request.section, addRow: request.addRow === true };
}

/** 这条 JSON 路径归不归这一区。 */
export function profileSectionOwnsPath(section: ProfileSection, path: string): boolean {
  return section.pathPrefixes.some((prefix) => path === prefix || path.startsWith(`${prefix}.`) || path.startsWith(`${prefix}[`));
}

/**
 * 必填缺项按区分两摞：本区的照旧点名、滚到格；别区的（例如改联系人时注册地址还空着）这张抽屉里没有那一格，只能列出来
 * 指路整张表单。来源单据与生效时间两格属「这次更新」，分区抽屉里也有，算本区的。
 */
export function splitMissingBySection(
  section: ProfileSection,
  missing: readonly ProfileMissingField[],
): { inside: ProfileMissingField[]; outside: ProfileMissingField[] } {
  const inside: ProfileMissingField[] = [];
  const outside: ProfileMissingField[] = [];
  for (const field of missing) {
    const own = profileSectionOwnsPath(section, field.path) || field.path === 'profiles[0].basis' || field.path === 'profiles[0].effectiveFrom';
    (own ? inside : outside).push(field);
  }
  return { inside, outside };
}

/** 从「添加」打开：逐行的区末尾加一空行并交回该行的焦点路径；不逐行的区原样交回。 */
export function draftWithNewRow(
  section: ProfileSection,
  draft: LegalEntityProfileDraft,
): { draft: LegalEntityProfileDraft; focusPath: string | null } {
  if (section.list === undefined) return { draft, focusPath: null };
  if (section.list.key === 'bankAccounts') {
    const index = draft.bankAccounts.length;
    return {
      draft: {
        ...draft,
        bankAccounts: [
          ...draft.bankAccounts,
          { label: '', holderName: '', bankName: '', accountNumber: '', swiftCode: '', bankAddress: '', currency: '', note: '' },
        ],
      },
      focusPath: section.list.focusPath(index),
    };
  }
  const index = draft.contacts.length;
  return {
    draft: {
      ...draft,
      contacts: [...draft.contacts, { name: '', email: '', phone: '', department: '', position: '', purposes: [], primaryFor: [] }],
    },
    focusPath: section.list.focusPath(index),
  };
}

const changeVerbs: Record<Exclude<ProfileChangeKind, 'unchanged'>, string> = { added: '新增', modified: '修改', removed: '移除' };

/**
 * 分区保存时来源单据的自动写法（产品策略，spec「三类归法」）：「更新联系人：新增 张三、修改 李四」。按主体归并——
 * 一个联系人改了三格是一处「修改」，不是三处；主体名里带着本区名的（「收款账户 · 人民币（CNY） …」）去掉那个前缀。
 * 最多点三处，其余并成「等 N 处」：来源单据是一行字，不是变更日志，整份差异在版本历史里。什么都没改也要写一句——
 * 确认后仍登记为新的一版（整张表单的预览也这么说）。
 */
export function profileSectionBasisOf(section: ProfileSection, comparison: ProfileComparison): string {
  const entries = new Map<string, { label: string; kinds: Set<ProfileChangeKind> }>();
  for (const field of comparison.fields) {
    if (field.kind === 'unchanged' || !section.changeGroups.includes(field.group)) continue;
    const raw = field.subject || field.field;
    const label = raw.startsWith(`${section.title} · `) ? raw.slice(section.title.length + 3) : raw;
    const key = field.contactKey ?? label;
    const entry = entries.get(key) ?? { label, kinds: new Set<ProfileChangeKind>() };
    entry.kinds.add(field.kind);
    entries.set(key, entry);
  }
  const parts = [...entries.values()].map((entry) => {
    const verb = entry.kinds.size === 1 ? changeVerbs[[...entry.kinds][0] as Exclude<ProfileChangeKind, 'unchanged'>] : '修改';
    return `${verb} ${entry.label}`;
  });
  if (parts.length === 0) return `更新${section.title}：内容未变，重新登记一版`;
  const shown = parts.slice(0, 3).join('、');
  return `更新${section.title}：${parts.length > 3 ? `${shown} 等 ${parts.length} 处` : shown}`;
}
