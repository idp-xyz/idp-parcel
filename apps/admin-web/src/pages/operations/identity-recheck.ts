// 待识别实物区的行转写与人工重核答复的判读（票 no-reception-source/03；ADR-0199 决定三）。
//
// 单独成模块而不写在页面里：这一区最容易错的是两格——「外部标识观察」把来源未保全、设备没报号与号尾带不可见字符三件事
// 说成同一片空白；「重核」答复把不重核的三格（记身份冲突、来源未保全、指名的不是待识别）说成失败。页面组件要跑起来得有
// React 与 DOM，判读是纯函数就能拿 node:test 钉住（同 reception-rows 的先例）。冲突来由那几句在 identity-conflict-cause，
// 这里只按它判重核能不能发起、答复里那一句冲突怎么解（票 no-reception-identity/07；ADR-0211）。

import { formatInstant } from '../catalogue-view';
import type { IdentityRecheckResponseBody } from './identity-recheck-api';
import { identityCheckCellOf } from './identity-check';
import { deliveryScopeBasisCellOf, deliveryScopeCheckCellOf, deliveryScopeRecoveryOf, pendingCauseOf } from './delivery-scope-check';
import {
  ambiguityOnlyCause,
  conflictReleasableByRecheck,
  identityConflictCauseCellOf,
  identityConflictCauseShortOf,
} from './identity-conflict-cause';
import { labelOf, receptionKindLabels } from './presentation';
import type { UnidentifiedItemRecord } from './records-api';

/** 重核口结果代数（application RecheckOutcome）。 */
export const recheckOutcomeLabels: Record<string, string> = {
  INTAKE_FORMED: '重核成立——追加一版节点收寄已形成',
  IDENTITY_CONFLICT: '重核结果是身份冲突——追加冲突版',
  IDENTITY_NOW_CHECKED: '这回问到了，仍查无此号——追加一版待识别 · 已核对',
  STILL_UNIDENTIFIED: '仍查无此号——判断不变，未追加',
  EXISTING_RESULT: '这一件已有后续判断——不再追加',
  IDENTITY_CONFLICT_RECORDED: '不重核：这一件记着身份冲突',
  SOURCE_NOT_PRESERVED: '不重核：来源未保全',
  NOT_PENDING_IDENTIFICATION: '不重核：指名的那一版不是待识别实物',
  STILL_AMBIGUOUS: '仍有未决的同号声明——暂不能关联，判断不变',
  AMBIGUITY_GONE_SCOPE_UNVERIFIED: '同号声明已不在，但没有可验证交付范围——追加一版待识别 · 范围未证',
};

/** 「重核」动作此刻能不能发起；不能时带一句为什么，页面照列，不让人去点一个必然答「不重核」的按钮。 */
export type RecheckAvailability =
  | { kind: 'available'; judgmentVersion: string }
  | { kind: 'unavailable'; reason: string };

export interface UnidentifiedReviewRow {
  key: string;
  values: Readonly<Record<string, string>>;
  recheck: RecheckAvailability;
}

/**
 * 「外部标识观察」一格。三种不同的「看不见号」各写各的字：来源保全之前的行号码从未留存；设备没报号是空串；号两端带空白
 * 或号里带控制字符（扫描附带的 Tab、回车）时加引号转义示出——身份核对逐字比对，一个看不见的 Tab 就是 PS 答零候选的原因。
 */
export function externalMarkCellOf(record: Pick<UnidentifiedItemRecord, 'sourcePreserved' | 'externalMark'>): string {
  if (!record.sourcePreserved) return '来源未保全';
  const mark = record.externalMark ?? '';
  if (mark === '') return '（设备未报号）';
  if (mark !== mark.trim() || /\p{Cc}/u.test(mark)) return JSON.stringify(mark);
  return mark;
}

/**
 * 重核对象的条件照 ADR-0199 决定三逐条判：记身份冲突的只经人工处置解除（ADR-0210 决定一），不经重核——来由是只因歧义信号的
 * 例外（ADR-0211 决定二）；来源未保全的无号可问；已被接续的不是当前有效版本。
 * 服务端照样会逐条判、照样答一格不重核——页面先说清楚，只是省一次必然不追加的写。
 */
export function recheckAvailabilityOf(record: UnidentifiedItemRecord): RecheckAvailability {
  if (record.identityConflict && !conflictReleasableByRecheck(record)) {
    const cause = identityConflictCauseShortOf(record.identityConflictCause);
    return { kind: 'unavailable', reason: `记身份冲突（${cause}），只经下方「处置」解除，不经重核` };
  }
  if (!record.sourcePreserved) return { kind: 'unavailable', reason: '来源未保全，无号可再问' };
  if (record.succeededBy) return { kind: 'unavailable', reason: `已被 ${record.succeededBy} 接续` };
  if (!record.judgmentVersion) return { kind: 'unavailable', reason: '没有判断版本，无从指名' };
  return { kind: 'available', judgmentVersion: record.judgmentVersion };
}

/** 一行待识别实物登记转写成待识别实物区的一行。 */
export function unidentifiedRowOf(record: UnidentifiedItemRecord): UnidentifiedReviewRow {
  return {
    key: `unidentified:${record.sourceId}`,
    values: {
      unit: record.unit,
      node: record.node,
      externalMark: externalMarkCellOf(record),
      candidates: record.candidates.join('、'),
      identityConflict: record.identityConflict ? '有冲突' : '无冲突',
      identityConflictCause: identityConflictCauseCellOf(record),
      receivedAt: formatInstant(record.receivedAt),
      association: record.association ?? '',
      judgmentVersion: record.judgmentVersion ?? '',
      identityCheck: identityCheckCellOf(record),
      pendingCause: pendingCauseOf(record).label,
      deliveryScope: withRecovery(deliveryScopeCheckCellOf(record), deliveryScopeRecoveryOf(record)),
      deliveryScopeBasis: deliveryScopeBasisCellOf(record),
    },
    recheck: recheckAvailabilityOf(record),
  };
}

/**
 * 重核答复的呈现。`tone` 两色：`answered`（形成了的业务答案，含不重核三格——它们是服务端知道为什么不重核的答案，不是
 * 失败）与 `refused`（未收录的码）；`landed` 只在追加了一版时为真，页面据此重读列表。
 */
export interface RecheckNote {
  tone: 'answered' | 'refused';
  landed: boolean;
  headline: string;
  details: string[];
}

export function describeRecheckAnswer(body: IdentityRecheckResponseBody): RecheckNote {
  const headline = `重核口答复：${body.outcome} —— ${labelOf(recheckOutcomeLabels, body.outcome)}`;
  const rechecked = `被重核的那一版 ${body.recheckedVersion}`;
  switch (body.outcome) {
    case 'INTAKE_FORMED':
      return {
        tone: 'answered',
        landed: true,
        headline,
        details: [
          `新版本 ${body.judgmentVersion ?? '—'} 接续${rechecked}，关联包裹 ${body.association ?? '—'}`,
          '业务时间沿用原收寄时刻，控制起点不另起；原待识别记录照留',
          serviceResultLine(body),
          scopeLine(body),
          '收寄意图已随新版本交 parcel-shipment',
        ],
      };
    case 'IDENTITY_CONFLICT':
      return {
        tone: 'answered',
        landed: true,
        headline,
        details: [
          `冲突版 ${body.judgmentVersion ?? '—'} 接续${rechecked}，候选 ${(body.candidates ?? []).join('、') || '—'}`,
          '身份冲突暂停方向性作业；号码唯一指向的包裹若已关联到另一件实物，那一件也追加了冲突版。',
          conflictReleaseLine(body.identityConflictCause),
        ],
      };
    case 'IDENTITY_NOW_CHECKED':
      return {
        tone: 'answered',
        landed: true,
        headline,
        details: [
          `新版本 ${body.judgmentVersion ?? '—'} 接续${rechecked}：parcel-shipment 这回答复了，仍没有标识这个号的包裹`,
          '那一件原先身份未核对（当时没问到），从此记为已核对、查无此号；收寄事实与控制照旧，原记录照留',
        ],
      };
    case 'AMBIGUITY_GONE_SCOPE_UNVERIFIED':
      // 冲突解开了，剩下的只是缺证据（ADR-0212 决定六「只因歧义信号的冲突重核后只缺范围」）：不记冲突的待识别，候选照记，等补证据再重核。
      return {
        tone: 'answered',
        landed: true,
        headline,
        details: [
          `新版本 ${body.judgmentVersion ?? '—'} 接续${rechecked}：不记冲突，候选 ${(body.candidates ?? []).join('、') || '—'} 照记`,
          scopeLine(body),
          '缺的是可验证交付范围：补证据（交接单确认绑定，或归入一张开放的交接单）后再重核；没交收寄意图',
        ],
      };
    case 'STILL_UNIDENTIFIED':
      return {
        tone: 'answered',
        landed: false,
        headline,
        details: [`${rechecked}照旧是当前有效版本；parcel-shipment 此刻仍没有标识这个号的包裹`],
      };
    case 'EXISTING_RESULT':
      return {
        tone: 'answered',
        landed: false,
        headline,
        details: [
          `当前有效版本 ${body.currentVersion ?? '—'}（${labelOf(receptionKindLabels, body.recordedOutcome ?? '')}）`,
          '同一件再重核不会再追加；要看的是当前有效那一版',
        ],
      };
    case 'IDENTITY_CONFLICT_RECORDED':
      return {
        tone: 'answered',
        landed: false,
        headline,
        details: [
          `这一件记着身份冲突（${identityConflictCauseShortOf(body.identityConflictCause)}）；这一来由只经人工处置解除，不经重核`,
        ],
      };
    case 'STILL_AMBIGUOUS':
      // 不是失败，也不是查无此号：parcel-shipment 仍答恰一个候选，只是号码还被另一份已提交、未决的委托报着，说不出到站的是哪一件。
      return {
        tone: 'answered',
        landed: false,
        headline,
        details: [
          `${rechecked}照旧是当前有效版本，身份冲突照留；parcel-shipment 仍只答候选 ${(body.candidates ?? []).join('、') || '—'}，同时号码还被另一份已提交、未决的委托报着`,
          '那份委托被拒绝或撤回之后，parcel-shipment 会交出声明撤除，服务端自动再核一次；也可届时再点重核。现在就要定，用下方「处置」。',
        ],
      };
    case 'SOURCE_NOT_PRESERVED':
      return {
        tone: 'answered',
        landed: false,
        headline,
        details: ['这一行落在来源保全之前，现场看到的号从未在册上留存，无号可按'],
      };
    case 'NOT_PENDING_IDENTIFICATION':
      return {
        tone: 'answered',
        landed: false,
        headline,
        details: ['指名的那一版已是节点收寄或别的判断，不是待识别实物'],
      };
    default:
      // 未收录的 outcome 原样示出：服务端新增一格时，页面宁可显示英文原名，也不把它归进某个既有说法。
      return { tone: 'refused', landed: false, headline, details: [] };
  }
}

/** problem+json 错误码的中文说明。 */
export function recheckProblemNote(code: string): string {
  switch (code) {
    case 'JUDGMENT_NOT_VISIBLE':
      return '本租户的收寄册上没有这一版判断；刷新列表再指名。';
    case 'OUTSIDE_ADMISSION_SCOPE':
      return '这笔生产写不在治理登记的生产权威区间内，要阶段治理去登记区间；重试不会好。';
    case 'MALFORMED_REQUEST':
      return '载荷不是服务端接受的形状（只有判断版本一键，且不得带身份键）；请检查前端与服务端版本是否一致。';
    case 'METHOD_NOT_ALLOWED':
      return '端点不接受当前 HTTP 方法；请检查前端与服务端版本是否一致。';
    case 'INTAKE_FAILED':
      return '接入面未能形成操作者身份；服务端未读取重核内容。';
    case 'IDENTITY_DEPENDENCY_UNAVAILABLE':
      return '接入身份依赖读不动，可稍后重试。';
    case 'NO_ANSWER_FORMED':
      return '重核未形成答案（身份核对读不动，或落册与意图入队失败），册上未追加，可稍后重试。';
    default:
      return `服务端问题码：${code}`;
  }
}

function withRecovery(cell: string, recovery: string): string {
  return recovery ? `${cell}——${recovery}` : cell;
}

/** 追加了冲突版之后那一句「怎么解」：随这一版记下的来由走（ADR-0211 决定二）。 */
function conflictReleaseLine(cause: string | undefined): string {
  if (cause === ambiguityOnlyCause) {
    return '来由只因歧义信号：声明同号的那份委托被拒绝或撤回之后，服务端会自动再核；也可用下方「处置」直接定。';
  }
  return `这一来由（${identityConflictCauseShortOf(cause)}）只经下方「处置」解除，再重核不会解开。`;
}

function scopeLine(body: IdentityRecheckResponseBody): string {
  return `交付范围核对：${deliveryScopeCheckCellOf(body)}`;
}

function serviceResultLine(body: IdentityRecheckResponseBody): string {
  if (body.serviceResultCheck === 'UNREADABLE') return '服务结果读不动：没有标记不代表没有取消或终局';
  const markers = body.serviceMarkers ?? [];
  if (markers.length > 0) return `服务标记 ${markers.join('、')}`;
  return body.serviceResultCheck === 'CHECKED' ? '服务结果已查：无取消与终局' : '服务结果核对态未带';
}
