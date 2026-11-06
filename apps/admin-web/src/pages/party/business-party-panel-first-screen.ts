// 往来单位右栏首屏的判读（票 business-party-ux/08）：头区下那一句随状态的提示、主动作该是哪一枚。纯函数，node:test 钉着；栏只负责摆。
//
// 提示只翻译生命周期规则，不写「已建立 X 关系」那种结论句（spec「原型的做法怎么取」：关系结论由头部角色标签说）。待生效那句
// 的依据是 CONTEXT「参与方身份」：生效时点未到的登记不支持新的商业决定；「还没有合作关系」照票 02 空状态的写法给下一步。
// 我方公司不提「没有关系」：它多半是关系的相对方，持有的角色本来就少，提了是误报。已停用不在这里——停用横幅已经在说。

import type { PartyRelationshipView } from './party-overview';

/** 还在起作用的关系：候选与已生效；到期、撤销、替代都是终局（CONTEXT「参与方关系」生命周期）。 */
const liveRelationshipStatuses: ReadonlySet<string> = new Set(['CANDIDATE', 'EFFECTIVE']);

/** 这家单位名下还在起作用的关系有几段，持有方与相对方两侧都算——两侧都是「与它有往来」。 */
export function liveRelationshipCountOf(views: readonly PartyRelationshipView[]): number {
  return views.filter((view) => liveRelationshipStatuses.has(view.status)).length;
}

export type PanelHint = { kind: 'pending'; effectiveFrom: string } | { kind: 'no-relationship' } | null;

/**
 * 头区下的提示。`liveRelationships` 为 null 即关系册还没答出：不知道有没有关系，就不说「还没有」。
 */
export function panelHintOf(
  row: { status: string; effectiveFrom: string },
  ours: boolean,
  liveRelationships: number | null,
): PanelHint {
  if (row.status === 'REGISTERED') return { kind: 'pending', effectiveFrom: row.effectiveFrom };
  if (row.status === 'EFFECTIVE' && !ours && liveRelationships === 0) return { kind: 'no-relationship' };
  return null;
}

export type PanelPrimaryAction = 'register-relationship' | 'update-profile' | null;

/**
 * 头区主动作：没有在用关系的单位，下一步就是登关系；其余照旧是「更新资料」。公司主体钉着的（`pinned` 为真，已停用的也算）
 * 不登往来单位资料，没有「更新资料」；法人册没答出（null）照今天的做法先摆它。没有写授予什么都不摆。
 */
export function panelPrimaryActionOf(mayWrite: boolean, pinned: boolean | null, hint: PanelHint): PanelPrimaryAction {
  if (!mayWrite) return null;
  if (hint?.kind === 'no-relationship') return 'register-relationship';
  return pinned === true ? null : 'update-profile';
}
