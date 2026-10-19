// 送站交接单区与交接单详情上的「确认」（票 ps-external-mark-relations/13「做什么」第 1、3 项；UC-NO-006 步骤 5B、ADR-0212 决定三）。
//
// 单独成模块而不写在页面里，理由同 identity-disposition：最容易错的几格是纯函数——没下结论的账户与否认的账户长成一样、确认按钮出现在已有
// 结论的账户上；载荷漏了交付方参与方或依据引用；「未配置」与「不在授权交付关系内」被当成失败，或写成同一个恢复动作。页面组件要 React 与
// DOM，判读拿 node:test 就能钉住。

import { formatInstant } from '../catalogue-view';
import type { BusinessPartyRecord } from '../party/api';
import { identityStatusLabels } from '../party/presentation';
import type {
  DeliveryHandoverConfirmationRequest,
  DeliveryHandoverConfirmationResponseBody,
  DeliveryHandoverNoteRecord,
  HandoverBindingRecord,
  HandoverMemberRecord,
} from './delivery-scope-api';
import { labelOf } from './presentation';

/** 绑定结论两值（domain HandoverBindingConclusion）；没有结论的写「待确认」，那是 UC-NO-006 未决结束的原词，不是领域里的第三个结论。 */
export const bindingConclusionLabels: Record<string, string> = {
  CONFIRMED: '已确认',
  DENIED: '已否认',
};

/** 结论途径两值（domain HandoverBindingRoute）。 */
export const bindingRouteLabels: Record<string, string> = {
  CUSTOMER_CHANNEL: '客户渠道',
  OPERATION_DECISION: '运营决定',
};

/** 成员形成方式两值（domain HandoverMembershipKind）。 */
export const membershipLabels: Record<string, string> = {
  RECEPTION: '收寄带来',
  ASSIGNMENT: '归入',
};

/** 确认口结果代数（application DeliveryHandoverOutcome 里运营决定那一路会答的几格）。 */
export const confirmationOutcomeLabels: Record<string, string> = {
  BINDING_CONFIRMED: '绑定已确认',
  EXISTING_RESULT: '同一结论已在册——未再落',
  EXISTING_CONCLUSION: '这个账户已有不同的结论——未覆盖',
  NOT_IN_AUTHORIZED_DELIVERY_RELATIONSHIP: '不在授权交付关系内——未落结论',
  AUTHORIZED_DELIVERY_NOT_CONFIGURED: '授权交付关系未配置——未落结论',
  HANDOVER_NOTE_STATE_MISMATCH: '交接单状态不符——未落结论',
};

export interface HandoverNoteReviewRow {
  key: string;
  noteId: string;
  values: Readonly<Record<string, string>>;
}

function conclusionCount(record: DeliveryHandoverNoteRecord, conclusion: string | undefined): number {
  return record.claims.filter((claim) => claim.conclusion === conclusion).length;
}

/**
 * 一张交接单转写成交接单区的一行。范围成立与否取答复里那一格（领域按结论派生），不由页面数确认数再比——那是第二个口径；页面只在没成立
 * 时按结论分「有否认」与「还有待确认」，两者的出路不同：否认之后成员要另行归入，待确认的是去确认。
 */
export function handoverNoteRowOf(record: DeliveryHandoverNoteRecord): HandoverNoteReviewRow {
  const denied = conclusionCount(record, 'DENIED');
  let scope: string;
  if (record.scopeEstablished) {
    scope = `已成立${record.scopeEstablishedAt ? ` · ${formatInstant(record.scopeEstablishedAt)}` : ''}`;
  } else if (denied > 0) {
    scope = '不成立：有账户否认，名下成员可另行归入';
  } else {
    scope = '未成立：还有账户待确认';
  }
  return {
    key: `handover-note:${record.noteId}`,
    noteId: record.noteId,
    values: {
      noteId: record.noteId,
      node: record.node,
      deliveringParty: record.deliveringParty,
      openedAt: formatInstant(record.openedAt),
      state: record.closed ? `已关闭${record.closedAt ? ` · ${formatInstant(record.closedAt)}` : ''}` : '开放',
      members: String(record.memberCount),
      claims: record.claims.map((claim) => claim.customerAccount).join('、'),
      bindings: `已确认 ${conclusionCount(record, 'CONFIRMED')} · 已否认 ${denied} · 待确认 ${conclusionCount(record, undefined)}`,
      scope,
    },
  };
}

/** 「确认」此刻能不能对这个账户发起；不能时带一句为什么，页面照列，不给一个必然答「已有结论」的按钮。 */
export type ConfirmAvailability =
  | { kind: 'available'; noteId: string; customerAccount: string }
  | { kind: 'unavailable'; reason: string };

export interface HandoverClaimReviewRow {
  key: string;
  values: Readonly<Record<string, string>>;
  confirm: ConfirmAvailability;
}

/**
 * 声称的每个账户一行。结论是终态（ADR-0212 决定三）：已确认的不再确认；已否认的运营决定也改不了——否认是客户账户自己的话，运营决定只能
 * 确认。待确认的才给「确认」，交接单开没开放都给（运营决定不限于已关闭：柜台上先确认、再逐件收寄是常态）。
 */
export function claimRowsOf(record: DeliveryHandoverNoteRecord): HandoverClaimReviewRow[] {
  return record.claims.map((claim) => {
    const concluded = claim.conclusion !== undefined && claim.conclusion !== '';
    const evidence = claim.deliveringParticipant
      ? `交付方参与方 ${claim.deliveringParticipant} · 依据 ${claim.basis ?? '—'}`
      : '';
    let confirm: ConfirmAvailability;
    if (!concluded) {
      confirm = { kind: 'available', noteId: record.noteId, customerAccount: claim.customerAccount };
    } else if (claim.conclusion === 'DENIED') {
      confirm = { kind: 'unavailable', reason: '客户已否认：结论是终态，否认是客户账户自己的话，运营决定改不了' };
    } else if (claim.conclusion === 'CONFIRMED') {
      confirm = { kind: 'unavailable', reason: '已确认，不再确认' };
    } else {
      confirm = { kind: 'unavailable', reason: `结论 ${claim.conclusion}（页面未收录），不给确认` };
    }
    return {
      key: `claim:${record.noteId}:${claim.customerAccount}`,
      values: {
        customerAccount: claim.customerAccount,
        conclusion: concluded ? labelOf(bindingConclusionLabels, claim.conclusion ?? '') : '待确认',
        route: claim.route ? labelOf(bindingRouteLabels, claim.route) : '',
        concluded: concluded ? `${claim.concluder ?? '—'} · ${claim.concludedAt ? formatInstant(claim.concludedAt) : '—'}` : '',
        evidence,
      },
      confirm,
    };
  });
}

/** 一个成员转写成详情上成员表的一行。 */
export function handoverMemberRowOf(member: HandoverMemberRecord): { key: string; values: Readonly<Record<string, string>> } {
  return {
    key: `member:${member.membership}:${member.factId}`,
    values: {
      unit: member.unit,
      membership: labelOf(membershipLabels, member.membership),
      factId: member.factId,
      formedAt: formatInstant(member.formedAt),
    },
  };
}

/**
 * 交付方参与方的候选：取参与方与商业的参与方目录，按标识排。停用与未生效的照列并标出状态——一位交付方能不能替这个账户送站由授权交付判断
 * 答（参与方与商业），页面不替它先筛一遍；交付方就是账户所属参与方本身时也从这里选。
 */
export function deliveringParticipantOptionsOf(parties: readonly BusinessPartyRecord[]): { value: string; label: string }[] {
  return [...parties]
    .sort((left, right) => (left.partyId < right.partyId ? -1 : left.partyId > right.partyId ? 1 : 0))
    .map((party) => {
      const status = party.status === 'EFFECTIVE' ? '' : `· ${labelOf(identityStatusLabels, party.status)}`;
      return { value: party.partyId, label: `${party.partyName}（${party.partyId}）${status}` };
    });
}

/** 确认表单里人填的那几格；交接单与账户随所点的那一行带进来。 */
export type ConfirmationDraft = DeliveryHandoverConfirmationRequest;

/** 拼确认载荷：交付方参与方没选、依据引用空白时不发（服务端照样答坏载荷，页面先说）。 */
export function confirmationRequestOf(
  draft: ConfirmationDraft,
): { kind: 'ready'; request: DeliveryHandoverConfirmationRequest } | { kind: 'incomplete'; reason: string } {
  if (draft.deliveringParticipant.trim() === '') return { kind: 'incomplete', reason: '先从参与方目录选交付方参与方' };
  if (draft.basis.trim() === '') return { kind: 'incomplete', reason: '依据引用必填' };
  return {
    kind: 'ready',
    request: {
      noteId: draft.noteId,
      customerAccount: draft.customerAccount,
      deliveringParticipant: draft.deliveringParticipant,
      basis: draft.basis,
    },
  };
}

/**
 * 确认答复的呈现。`tone` 两色：`answered`（形成了的业务答案，含没落结论的各格——服务端知道为什么没落，不是失败）与 `refused`（未收录的
 * 码）；`landed` 只在落了结论时为真，页面据此重读详情与列表。
 */
export interface ConfirmationNote {
  tone: 'answered' | 'refused';
  landed: boolean;
  headline: string;
  details: string[];
}

function bindingSummaryOf(binding: HandoverBindingRecord): string {
  const evidence = binding.deliveringParticipant
    ? ` · 交付方参与方 ${binding.deliveringParticipant} · 依据 ${binding.basis ?? '—'}`
    : '';
  return `${binding.customerAccount} ${labelOf(bindingConclusionLabels, binding.conclusion)}（${labelOf(bindingRouteLabels, binding.route)}）${evidence} · ${formatInstant(binding.concludedAt)}`;
}

export function describeConfirmationAnswer(body: DeliveryHandoverConfirmationResponseBody): ConfirmationNote {
  const headline = `确认口答复：${body.outcome} —— ${labelOf(confirmationOutcomeLabels, body.outcome)}`;
  const recorded = body.binding ? [`在册的结论：${bindingSummaryOf(body.binding)}`] : [];
  const scope = body.scopeEstablishedAt
    ? `声称的账户已全部确认，可验证交付范围已成立（${formatInstant(body.scopeEstablishedAt)}）；名下仍待识别的实物会自动重核`
    : '其余声称的账户还待确认，可验证交付范围尚未成立';
  switch (body.outcome) {
    case 'BINDING_CONFIRMED':
      return {
        tone: 'answered',
        landed: true,
        headline,
        details: [...(body.binding ? [`这次确认：${bindingSummaryOf(body.binding)}`] : []), scope],
      };
    case 'EXISTING_RESULT':
      return { tone: 'answered', landed: false, headline, details: [...recorded, '同一账户同一结论已在册，确认不再落第二份', scope] };
    case 'EXISTING_CONCLUSION':
      return {
        tone: 'answered',
        landed: false,
        headline,
        details: [...recorded, '结论是终态：这个账户已有的结论不被覆盖'],
      };
    case 'NOT_IN_AUTHORIZED_DELIVERY_RELATIONSHIP':
      return {
        tone: 'answered',
        landed: false,
        headline,
        details: [
          '参与方与商业答：这位交付方参与方在开立时刻、开立节点没有替这个客户账户送站的授权交付关系；未落结论',
          '换一位有授权交付关系的交付方参与方再确认，或先在参与方与商业登记这段授权交付关系；也可由客户账户经客户渠道自己确认',
        ],
      };
    case 'AUTHORIZED_DELIVERY_NOT_CONFIGURED':
      return {
        tone: 'answered',
        landed: false,
        headline,
        details: [
          '本租户一条授权交付关系都没登记，交付方又不是这个客户账户所属的参与方本身；未落结论，服务端不默认放行',
          '租户先在参与方与商业登记授权交付关系（租户取值，PAR-NET-17）；若交付的就是账户所属参与方本身，选它即可确认',
        ],
      };
    case 'HANDOVER_NOTE_STATE_MISMATCH':
      return {
        tone: 'answered',
        landed: false,
        headline,
        details: [`交接单状态不符（${body.stateMismatch ?? '未带细分'}）；刷新详情后再看`],
      };
    default:
      // 未收录的 outcome 原样示出：服务端新增一格时，页面宁可显示英文原名，也不把它归进某个既有说法。
      return { tone: 'refused', landed: false, headline, details: [] };
  }
}

/** 确认口 problem+json 码的说明。 */
export function confirmationProblemNote(code: string): string {
  switch (code) {
    case 'DELIVERY_HANDOVER_NOTE_NOT_VISIBLE':
      return '本租户查无这张交接单，或这个客户账户不在它的声称之列（两种同答）；刷新详情再确认。';
    case 'OUTSIDE_ADMISSION_SCOPE':
      return '这笔生产写不在治理登记的生产权威区间内，要阶段治理去登记区间；重试不会好。';
    case 'MALFORMED_REQUEST':
      return '载荷不是服务端接受的形状（交接单、客户账户、交付方参与方与依据引用四格必填，且不得带身份键）；请检查前端与服务端版本是否一致。';
    case 'METHOD_NOT_ALLOWED':
      return '端点不接受当前 HTTP 方法；请检查前端与服务端版本是否一致。';
    case 'INTAKE_FAILED':
      return '接入面未能形成操作者身份；服务端未读取确认内容。';
    case 'IDENTITY_DEPENDENCY_UNAVAILABLE':
      return '接入身份依赖读不动，可稍后重试。';
    case 'NO_ANSWER_FORMED':
      return '确认未形成答案（授权交付判断读不动，或落册与门铃入队失败），未落结论，可稍后重试。';
    default:
      return `服务端问题码：${code}`;
  }
}
