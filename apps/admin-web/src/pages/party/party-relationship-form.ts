// 参与方关系登记表单的纯逻辑（票 admin-web-group-legal-entities/10 第 2 条）。全部是纯函数，node:test 钉着；
// 组件 PartyRelationshipRegistrationForm.tsx 只负责摆。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）：双方是否在册、角色与双方是否
// 匹配、区间是否倒置、修订是否连续，一律送上去让服务端答。这里只做编码层：修订号编成整数、各墙钟时刻换成
// RFC 3339、**可缺键缺席而不是零值**。
//
// 缺席为什么不能用零值顶：partyRelationshipDocument 用指针表达 effectiveEndsAt 与 approval 的缺席——缺终点即开
// 区间、缺批准即登为候选关系；零时刻在 NewEffectiveInterval 里恰是「无终点」，一个显式给出的坏时刻也会解成零值，
// 两者就分不开了，所以键在场解不出时刻是形状错。表单这一层把「操作者没填」编成缺键，把「填了但换不出来」拦在
// 本地，两种情形都不让一个零时刻过线。
//
// 各格不裁首尾空白、载荷不带租户格，理由同 business-party-form.ts。

import type { PartyRelationshipRecord } from './api';
import { wallTimeToRfc3339 } from '../moment';
import { partyRoleLabels } from './presentation';
import { revisionOf } from './business-party-form';
import type { PickerOption } from './PublicationFormFields';

export interface PartyRelationshipDraft {
  relationshipId: string;
  /** 文本框原值；编成整数在组载荷时做。 */
  revision: string;
  holder: string;
  counterparty: string;
  /** partyRoleLabels 的键之一；空串是「未选」，照送让服务端点名，表单不预选。 */
  role: string;
  scope: string;
  basis: string;
  /** `datetime-local` 原值（操作者本地墙钟）。 */
  effectiveStartsAt: string;
  /** 留空即开区间——键缺席。 */
  effectiveEndsAt: string;
  /** 「已批准」勾选框：不勾即候选关系，批准引用与批准时刻不进载荷，残字也不进。 */
  approved: boolean;
  approvalReference: string;
  approvedAt: string;
}

/** 键名逐字取 relationshipApprovalDocument 的 json 标签。 */
export interface PartyRelationshipApprovalItem {
  reference: string;
  approvedAt?: string;
}

/** 键名逐字取 partyRelationshipDocument 的 json 标签。 */
export interface PartyRelationshipRegistrationItem {
  relationshipId: string;
  revision?: number;
  holder: string;
  counterparty: string;
  role: string;
  scope: string;
  basis: string;
  effectiveStartsAt?: string;
  effectiveEndsAt?: string;
  approval?: PartyRelationshipApprovalItem;
}

export interface PartyRelationshipRegistrationPayload {
  relationships: PartyRelationshipRegistrationItem[];
}

export function emptyPartyRelationshipDraft(): PartyRelationshipDraft {
  return {
    relationshipId: '',
    revision: '',
    holder: '',
    counterparty: '',
    role: '',
    scope: '',
    basis: '',
    effectiveStartsAt: '',
    effectiveEndsAt: '',
    approved: false,
    approvalReference: '',
    approvedAt: '',
  };
}

/** 表单认领的 JSON 路径；一项一批，下标固定 0；批准那几格带嵌套路径。 */
export const partyRelationshipFieldPaths = [
  'relationships[0].relationshipId',
  'relationships[0].revision',
  'relationships[0].holder',
  'relationships[0].counterparty',
  'relationships[0].role',
  'relationships[0].scope',
  'relationships[0].basis',
  'relationships[0].effectiveStartsAt',
  'relationships[0].effectiveEndsAt',
  'relationships[0].approval.reference',
  'relationships[0].approval.approvedAt',
] as const;

export type PartyRelationshipFieldPath = (typeof partyRelationshipFieldPaths)[number];

/** 角色下拉的选项：只从 partyRoleLabels 派生，码是它的键、顺序同它——不在这里再抄一份封闭集。 */
export function partyRoleOptions(): PickerOption[] {
  return Object.entries(partyRoleLabels).map(([code, label]) => ({ value: code, label: `${label} · ${code}` }));
}

// 墙钟留空 → undefined（键缺席）；换出来 → RFC 3339；换不出来也 → undefined，由问题表拦住不送。
function momentOf(wallTime: string, timeZone: string): string | undefined {
  if (wallTime.trim() === '') return undefined;
  return wallTimeToRfc3339(wallTime, timeZone) ?? undefined;
}

/**
 * 草稿 → 载荷。身份串与引用串原样带；修订号与各时刻编不出时**该键缺席**；终点留空与不勾已批准也缺席——
 * 前者是编码失败、后者是操作者的声明，落到载荷上同为缺键，由 partyRelationshipLocalProblems 把前者拦在送之前。
 */
export function partyRelationshipPayloadOf(
  draft: PartyRelationshipDraft,
  timeZone: string,
): PartyRelationshipRegistrationPayload {
  const item: PartyRelationshipRegistrationItem = {
    relationshipId: draft.relationshipId,
    holder: draft.holder,
    counterparty: draft.counterparty,
    role: draft.role,
    scope: draft.scope,
    basis: draft.basis,
  };
  const revision = revisionOf(draft.revision);
  if (revision !== undefined) item.revision = revision;
  const startsAt = momentOf(draft.effectiveStartsAt, timeZone);
  if (startsAt !== undefined) item.effectiveStartsAt = startsAt;
  const endsAt = momentOf(draft.effectiveEndsAt, timeZone);
  if (endsAt !== undefined) item.effectiveEndsAt = endsAt;
  if (draft.approved) {
    const approval: PartyRelationshipApprovalItem = { reference: draft.approvalReference };
    const approvedAt = momentOf(draft.approvedAt, timeZone);
    if (approvedAt !== undefined) approval.approvedAt = approvedAt;
    item.approval = approval;
  }
  return { relationships: [item] };
}

/**
 * 本地能判的**编码层**问题，按 JSON 路径归组；空对象即可送。修订号编不进正整数、各时刻填了却换不成 RFC 3339；
 * 批准时刻只在勾了已批准时才看——不勾时那一格根本不进载荷。角色未选、双方为空、区间倒置都是服务端的话。
 */
export function partyRelationshipLocalProblems(
  draft: PartyRelationshipDraft,
  timeZone: string,
): Record<string, string[]> {
  const problems: Record<string, string[]> = {};
  if (revisionOf(draft.revision) === undefined) {
    problems['relationships[0].revision'] = ['修订号要填正整数'];
  }
  const unparseable = (wallTime: string) => wallTime.trim() !== '' && wallTimeToRfc3339(wallTime, timeZone) === null;
  if (unparseable(draft.effectiveStartsAt)) {
    problems['relationships[0].effectiveStartsAt'] = ['生效起点要是可解析的日期时间'];
  }
  if (unparseable(draft.effectiveEndsAt)) {
    problems['relationships[0].effectiveEndsAt'] = ['生效终点要是可解析的日期时间'];
  }
  if (draft.approved && unparseable(draft.approvedAt)) {
    problems['relationships[0].approval.approvedAt'] = ['批准时刻要是可解析的日期时间'];
  }
  return problems;
}

/**
 * 修订号建议值：关系标识在已取回关系册里 → 最新修订 + 1；不在（或列表没取到）→ 1。只是建议，连续性由服务端判；
 * 按原串比不裁空白，理由同 suggestedBusinessPartyRevision。
 */
export function suggestedPartyRelationshipRevision(
  rows: readonly PartyRelationshipRecord[] | null,
  relationshipId: string,
): number {
  if (relationshipId === '' || rows === null) return 1;
  const latest = rows.find((row) => row.relationshipId === relationshipId);
  return latest ? latest.revision + 1 : 1;
}
