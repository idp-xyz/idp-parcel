// 往来单位右栏首屏的判读（票 business-party-ux/08）：头区下那一句随状态的提示、主动作该是哪一枚。纯函数，node:test 钉着；栏只负责摆。
//
// 提示只翻译生命周期规则，不写「已建立 X 关系」那种结论句（spec「原型的做法怎么取」：关系结论由头部角色标签说）。待生效那句
// 的依据是 CONTEXT「参与方身份」：生效时点未到的登记不支持新的商业决定；但关系现在就能登——服务端登记关系时在关系生效起点
// 判双方已生效，所以只能说「开始时间不能早于它的生效时间」，说成「等生效后再登」就把一条能走的路说没了。生效日期不在这句里
// 重复：事实行已有，一个事实首屏只出现一处。
// 「还没有合作关系」照票 02 空状态的写法给下一步，不再接「角色由合作关系说」：持有未停用货主客户账户也算客户（CONTEXT 规则
// 「一个业务参与方持有客户角色，当且仅当……」），有账户没关系的单位头部正挂着「客户」。
// 我方公司不提「没有关系」：它多半是关系的相对方，持有的角色本来就少，提了是误报。已停用不在这里——停用横幅已经在说。

import { liveRelationshipStatuses, type PartyRelationshipView } from './party-overview';

/** 这家单位名下还在起作用的关系有几段，持有方与相对方两侧都算——两侧都是「与它有往来」。 */
export function liveRelationshipCountOf(views: readonly PartyRelationshipView[]): number {
  return views.filter((view) => liveRelationshipStatuses.has(view.status)).length;
}

export type PanelHint = { kind: 'pending' } | { kind: 'no-relationship' } | null;

/**
 * 头区下的提示。`liveRelationships` 为 null 即关系册还没答出：不知道有没有关系，就不说「还没有」。`rolesHeld` 是头部标签
 * 认出的角色数（business-party-list.ts 的 indexPartyRoles，含未停用客户账户带来的「客户」，ADR-0367 决定四）：只凭客户账户
 * 就是客户的单位不提「还没有合作关系」——账户不要求先登客户关系，提了就与头部的「客户」标签互相打架。
 */
export function panelHintOf(
  row: { status: string },
  ours: boolean,
  liveRelationships: number | null,
  rolesHeld: number,
): PanelHint {
  if (row.status === 'REGISTERED') return { kind: 'pending' };
  if (row.status === 'EFFECTIVE' && !ours && liveRelationships === 0 && rolesHeld === 0) return { kind: 'no-relationship' };
  return null;
}

export interface PanelHeaderActions {
  /** 摆「登记合作关系」作主动作。 */
  registerRelationship: boolean;
  /** 「更新资料」怎么摆：主动作、让位给登记关系后的描边、不摆。 */
  updateProfile: 'primary' | 'outline' | null;
}

/**
 * 头区的动作：没有在用关系的单位，下一步就是登关系，「更新资料」退成描边；其余「更新资料」是主动作。公司主体钉着的
 * （`pinned` 为真，已停用的也算）不登往来单位资料，没有「更新资料」；法人册没答出（null）照今天的做法先摆它。
 * 没有写授予什么都不摆。栏里的 JSX 只照这里的答案摆，不再自己判一遍 pinned。
 */
export function panelHeaderActionsOf(mayWrite: boolean, pinned: boolean | null, hint: PanelHint): PanelHeaderActions {
  if (!mayWrite) return { registerRelationship: false, updateProfile: null };
  const registerRelationship = hint?.kind === 'no-relationship';
  const updateProfile = pinned === true ? null : registerRelationship ? 'outline' : 'primary';
  return { registerRelationship, updateProfile };
}
