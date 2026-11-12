// 往来单位右栏首屏的判读（票 business-party-ux/08）：「概要」签顶上那一条随状态的提示、主动作该是哪一枚。纯函数，node:test 钉着；栏只负责摆。
//
// 提示只翻译生命周期规则，不写「已建立 X 关系」那种结论句（spec「原型的做法怎么取」：关系结论由头部角色标签说）。待生效那句
// 的依据是 CONTEXT「参与方身份」：生效时点未到的登记不支持新的商业决定；但关系现在就能登——服务端登记关系时在关系生效起点
// 判双方已生效，所以只能说「开始时间不能早于它的生效时间」，说成「等生效后再登」就把一条能走的路说没了。生效日期不在这句里
// 重复：事实行已有，一个事实首屏只出现一处。
// 「还没有合作关系」照票 business-party-ux/02 空状态的写法给下一步，不说「角色由合作关系说」：持有未停用货主客户账户也算客户
// （CONTEXT 规则「一个业务参与方持有客户角色，当且仅当……」），有账户没关系的单位头部正挂着「客户」。
// 我方公司不提「没有关系」：它多半是关系的相对方，持有的角色本来就少，提了是误报。已停用不在这里——停用横幅已经在说。

import { formatCount } from './business-parties-workbench';

/**
 * 停用对话框说明后面跟的那一句（票 business-party-redesign/11）：这家单位名下还有几段在用的合作关系，与「还没有合作关系」
 * 数的是同一个数。停用不因此被拦——已有的记录保留——但按下之前该知道还有几段挂在它名下。没有就不说；数是 null（关系册还没
 * 答出，或没读全）也不说：到手的那一页上数出来的不是全数，报出来会被当成「就这些」。
 */
export function deactivationLiveRelationshipsNote(liveRelationships: number | null): string | null {
  if (liveRelationships === null || liveRelationships <= 0) return null;
  return `它名下还有 ${formatCount(liveRelationships)} 段在用的合作关系。`;
}

export type PanelHint = { kind: 'pending' } | { kind: 'no-relationship' } | null;

/**
 * 「概要」签顶上的提示。`liveRelationships` 为 null 即关系册还没答出：不知道有没有关系，就不说「还没有」。`rolesHeld` 是头部标签
 * 认出的角色数（读口在这一行上答的类别，含未停用客户账户带来的「客户」，ADR-0367 决定四）：只凭客户账户
 * 就是客户的单位不提「还没有合作关系」——账户不要求先登客户关系，提了就与头部的「客户」标签互相打架。客户账户册还没答出时
 * 它是 null，同样不说「还没有」：先说了再收回，会闪一下提示和「登记合作关系」主动作。
 */
export function panelHintOf(
  row: { status: string },
  ours: boolean,
  liveRelationships: number | null,
  rolesHeld: number | null,
): PanelHint {
  if (row.status === 'REGISTERED') return { kind: 'pending' };
  if (row.status === 'EFFECTIVE' && !ours && liveRelationships === 0 && rolesHeld === 0) return { kind: 'no-relationship' };
  return null;
}

export interface PanelHeaderActions {
  /** 「登记合作关系」怎么摆：主动作、描边、不摆。 */
  registerRelationship: 'primary' | 'outline' | null;
  /** 「更新资料」怎么摆：主动作、让位给登记关系后的描边、不摆。 */
  updateProfile: 'primary' | 'outline' | null;
}

/**
 * 头区的两枚写动作。位置固定、主次随提示走：提示是「还没有合作关系」时下一步就是登关系，它是主动作、「更新资料」退成描边；
 * 其余「更新资料」是主动作——只凭客户账户就是客户的单位没有那句提示，主动作也就是「更新资料」。
 * 「登记合作关系」不是主动作时也留在原位（描边），但只给已生效、又不是公司主体钉着的单位：待生效的那句提示已经说了关系怎么登，
 * 已停用的不再登新关系，我方公司多半是关系的相对方——这几种要登仍可去「关联」签。
 * 公司主体钉着的（`pinned` 为真，已停用的也算）不登往来单位资料，没有「更新资料」；法人册没答出（null）两枚都先摆，与对象页
 * 同一做法。没有写授予什么都不摆。栏里的 JSX 只照这里的答案摆，不另判 pinned 与状态。
 */
export function panelHeaderActionsOf(
  mayWrite: boolean,
  pinned: boolean | null,
  hint: PanelHint,
  status: string,
): PanelHeaderActions {
  if (!mayWrite) return { registerRelationship: null, updateProfile: null };
  const registerFirst = hint?.kind === 'no-relationship';
  const registerRelationship = registerFirst ? 'primary' : status === 'EFFECTIVE' && pinned !== true ? 'outline' : null;
  const updateProfile = pinned === true ? null : registerFirst ? 'outline' : 'primary';
  return { registerRelationship, updateProfile };
}
