// 参与方关系详情上「撤销」「到期」两个动作的纯逻辑（票 ps-external-mark-relations/17）。全部是纯函数，node:test 钉着；
// 组件 PartyRelationshipEndActions.tsx 只负责摆。
//
// 只做三件事：判这一行此刻能不能开这两个动作（不能就说为什么），把草稿编成载荷（落点修订、时刻换 RFC 3339），把确认框
// 要复述的话拼出来。关系能不能结束、终点到没到、时刻晚不晚于登记时刻，仍由服务端按册面答——这里禁用按钮只为不让人对着
// 一段显然结束不了的关系去填表，服务端的门一道不少。
//
// 依据不裁首尾空白、载荷不带租户格，理由同 business-party-form.ts。

import type { PartyRelationshipEndingAction, PartyRelationshipRecord } from './api';
import { formatInstant, wallTimeToRfc3339 } from '../moment';
import { labelOf, partyRoleLabels, relationshipStatusLabels } from './presentation';

export const partyRelationshipEndingActionLabels: Record<PartyRelationshipEndingAction, string> = {
  revocation: '撤销',
  expiration: '到期',
};

/** 这一行能不能开这个动作；不能时 reason 是要摆在按钮旁的那句话。 */
export type EndingAvailability = { available: true } | { available: false; reason: string };

/**
 * 只有`已生效`的关系能撤销或登记到期（PC CONTEXT「参与方关系」生命周期：已生效 → 已到期、已撤销或已替代）。到期另要区间
 * 有终点：开区间的关系没有可到期的终点，要结束它用撤销。终点到没到不在这里判——那要拿浏览器的钟去比，而判它的是服务端的钟。
 */
export function endingAvailability(row: PartyRelationshipRecord, action: PartyRelationshipEndingAction): EndingAvailability {
  if (row.status !== 'EFFECTIVE') {
    return {
      available: false,
      reason: `关系状态为「${labelOf(relationshipStatusLabels, row.status)}」：只有已生效的关系能${partyRelationshipEndingActionLabels[action]}。`,
    };
  }
  if (action === 'expiration' && !row.effectiveEndsAt) {
    return { available: false, reason: '有效区间没有终点：开区间的关系无期可到，要结束它用撤销。' };
  }
  return { available: true };
}

export interface RelationshipEndingDraft {
  basis: string;
  /** `datetime-local` 原值（操作者本地墙钟）。 */
  at: string;
}

/** RFC 3339 时刻在给定区的墙钟原值（`YYYY-MM-DDTHH:mm:ss`），供 `datetime-local` 预填；取不出交回空串。 */
export function wallTimeOf(instant: string, timeZone: string): string {
  const shown = formatInstant(instant, { timeZone });
  const match = /^(\d{4}-\d{2}-\d{2}) (\d{2}:\d{2}:\d{2})/.exec(shown);
  return match ? `${match[1]}T${match[2]}` : '';
}

/**
 * 草稿的初值：撤销是即时结束，时刻预填此刻；到期预填区间终点——到期时刻不早于终点，终点本身就是最常填的那一格。两格都
 * 只是预填，操作者可以改；依据一律空着，凭什么结束要人自己写。
 */
export function initialEndingDraft(
  row: PartyRelationshipRecord,
  action: PartyRelationshipEndingAction,
  now: Date,
  timeZone: string,
): RelationshipEndingDraft {
  const instant = action === 'expiration' && row.effectiveEndsAt ? row.effectiveEndsAt : now.toISOString();
  return { basis: '', at: wallTimeOf(instant, timeZone) };
}

/** 终止修订的落点：这一行是操作者看到的最新修订，落点就是它的下一号；册面已被推进时服务端答修订错位。 */
export function endingRevisionOf(row: PartyRelationshipRecord): number {
  return row.revision + 1;
}

const payloadSection: Record<PartyRelationshipEndingAction, 'revocations' | 'expirations'> = {
  revocation: 'revocations',
  expiration: 'expirations',
};

/** 键名逐字取服务端批文一项的 json 标签。 */
export interface RelationshipEndingItem {
  relationshipId: string;
  revision: number;
  basis: string;
  at?: string;
}

/** 草稿 → 载荷：一节恰一项。时刻换不出来时该键缺席，由 endingLocalProblems 在送之前拦。 */
export function endingPayloadOf(
  row: PartyRelationshipRecord,
  action: PartyRelationshipEndingAction,
  draft: RelationshipEndingDraft,
  timeZone: string,
): Record<string, RelationshipEndingItem[]> {
  const item: RelationshipEndingItem = {
    relationshipId: row.relationshipId,
    revision: endingRevisionOf(row),
    basis: draft.basis,
  };
  const at = wallTimeToRfc3339(draft.at, timeZone);
  if (at !== null) item.at = at;
  return { [payloadSection[action]]: [item] };
}

/** 本地能判的编码层问题，按格名归组；空对象即可以进确认框。依据为空是服务端的话，这里不拦。 */
export function endingLocalProblems(draft: RelationshipEndingDraft, timeZone: string): Record<string, string[]> {
  if (wallTimeToRfc3339(draft.at, timeZone) === null) {
    return { at: ['时刻要是可解析的日期时间'] };
  }
  return {};
}

/** 一句话说清是哪段关系：持有方 —角色→ 相对方。名称转不到时如实只写标识。 */
export function relationshipSentence(row: PartyRelationshipRecord): string {
  const holder = row.holderNameKnown && row.holderName ? `${row.holderName}（${row.holderId}）` : row.holderId;
  const counterparty =
    row.counterpartyNameKnown && row.counterpartyName ? `${row.counterpartyName}（${row.counterpartyId}）` : row.counterpartyId;
  return `${holder} —${labelOf(partyRoleLabels, row.role)}→ ${counterparty}`;
}

/** 确认框要复述的各行：哪段关系、生效边界（本地时间 + UTC）、依据、落点修订。时刻换不出来时那一行照实说。 */
export function endingConfirmLines(
  row: PartyRelationshipRecord,
  action: PartyRelationshipEndingAction,
  draft: RelationshipEndingDraft,
  timeZone: string,
): string[] {
  const at = wallTimeToRfc3339(draft.at, timeZone);
  const boundary =
    at === null
      ? '生效边界：时刻不可解析'
      : `生效边界：${formatInstant(at, { timeZone })}（${formatInstant(at, { timeZone: 'UTC' })}）起不再支持新的商业决定`;
  return [
    `关系：${relationshipSentence(row)}（${row.relationshipId}）`,
    boundary,
    `${partyRelationshipEndingActionLabels[action]}依据：${draft.basis === '' ? '（未填，服务端会拒）' : draft.basis}`,
    `落点修订：r${endingRevisionOf(row)}（这一行当前为 r${row.revision}）`,
  ];
}

/** 确认框正文：只影响边界之后的新决定、已作出的决定不变、不可撤回——三句不随动作变，按下之前要读到。 */
export function endingConfirmMessage(action: PartyRelationshipEndingAction): string {
  return (
    `${partyRelationshipEndingActionLabels[action]}只影响边界之后的新决定，已作出的决定不变；` +
    '终止修订落册后操作不可撤回，要重新合作须另登一段新关系。'
  );
}
