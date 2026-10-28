import { operatorRefusalSentence } from '../../auth/access-refusal';
import type { ModuleInfo } from '../../navigation';
import type { TemplateViewState } from '../../templates/state-slot';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  type ApiResult,
  type ManualItemOutcome,
  type PostIntakeDirection,
  type PostIntakeDispositionDecisionDraft,
  type PostIntakeDispositionDecisionResponseBody,
  type PostIntakeDispositionRequestRow,
  type PostIntakeDispositionRequestsResponseBody,
  type PostIntakeManualItemResultDraft,
  type PostIntakeManualItemResultResponseBody,
  type PostIntakeManualItemRow,
  type PostIntakeManualItemsResponseBody,
} from './api';
import { shortPrincipalRef } from '../../components/principal-ref';

// 收寄后处置页的呈现逻辑(UC-PS-006;ADR-0332 收寄后服务处置决定、ADR-0341 待人工处理项),与页面组件分开住:
// 这一层没有 React,测试能直接钉去向集、草案形状、各格结果与空态措辞。词表取服务端响应的原词。

export interface DirectionOption {
  id: PostIntakeDirection;
  label: string;
  variant: 'secondary' | 'default' | 'danger';
}

/** 去向恰是五种(ADR-0332 决定一)。拦截、退运、服务终止会改变包裹走向,按强调档位画;继续按中性档位。 */
export const directionOptions: readonly DirectionOption[] = [
  { id: 'CONTINUE', label: '继续', variant: 'secondary' },
  { id: 'INTERCEPT', label: '拦截', variant: 'default' },
  { id: 'REDIRECT', label: '改送', variant: 'default' },
  { id: 'RETURN', label: '退运', variant: 'default' },
  { id: 'TERMINATE_SERVICE', label: '服务终止', variant: 'danger' },
];

export function directionLabel(direction: string): string {
  return directionOptions.find((option) => option.id === direction)?.label ?? direction;
}

const ownerLabels: Record<string, string> = {
  NETWORK_ROUTING: '网络路由',
  NODE_OPERATIONS: '节点作业',
  TRANSPORT_FULFILLMENT: '运输履约',
};
export function ownerLabel(owner: string): string {
  return ownerLabels[owner] ?? owner;
}

const actionLabels: Record<string, string> = {
  HOLD_AT_CONTROL: '在控制点扣留',
  HOLD_ONWARD_MOVEMENT: '暂停后续运转',
  REPLAN_TO_ALTERNATE_DESTINATION: '改规划到备选目的地',
  START_ALTERNATE_JOURNEY: '另起改送旅程',
  PLAN_RETURN_ROUTE: '规划退运路线',
  LOAD_FOR_RETURN: '装载退运',
  START_RETURN_JOURNEY: '另起退运旅程',
  RELEASE_UNCONSUMED_PLAN: '释放未消耗的规划',
};
export function actionLabel(action: string): string {
  return actionLabels[action] ?? action;
}

// ---- 待决请求 ----

export function requestKeyOf(row: PostIntakeDispositionRequestRow): string {
  return `${row.parcelId}#${row.requestKey}`;
}

export interface RequestQueueRow {
  id: string;
  title: string;
  subtitle: string;
  statusText: string;
  meta: string;
}

export function requestQueueRowOf(row: PostIntakeDispositionRequestRow): RequestQueueRow {
  return {
    id: requestKeyOf(row),
    title: row.parcelId,
    subtitle: `请求 ${row.requestKey} · ${row.origin}`,
    statusText: row.currentVersion === 0 ? '尚无处置决定' : `链尾 v${row.currentVersion}`,
    meta: formatInstant(row.recordedAt),
  };
}

/** 请求上为空的字段说「当时没有保全」,不说没有请求方。 */
const notPreserved = '未保全(先于保全落下的待处置记录)';

export function requestDetailRows(row: PostIntakeDispositionRequestRow): { label: string; value: string; mono?: boolean; title?: string }[] {
  return [
    { label: '包裹', value: row.parcelId, mono: true },
    { label: '请求键', value: row.requestKey, mono: true },
    { label: '来源', value: row.origin },
    { label: '请求方', value: row.requester ? shortPrincipalRef(row.requester) : notPreserved, title: row.requester || undefined },
    { label: '授权', value: row.authority ? shortPrincipalRef(row.authority) : notPreserved, title: row.authority || undefined },
    { label: '原因', value: row.reason || notPreserved },
    { label: '请求时间', value: row.requestedAt ? formatInstant(row.requestedAt) : notPreserved },
    { label: '取消边界前', value: row.intakeBefore || '—' },
    { label: '登记时刻', value: formatInstant(row.recordedAt) },
    { label: '决定链尾版本', value: row.currentVersion === 0 ? '还没决定过(0)' : String(row.currentVersion) },
  ];
}

export function requestSummaryText(body: PostIntakeDispositionRequestsResponseBody): string {
  const count = `等回应 ${body.pendingCount} 件`;
  return body.truncated ? `${count}(只列出前 ${body.requests.length} 件,回应之后其余的会补进来)` : count;
}

export const requestQueueEmptyState = {
  title: '当前租户没有等回应的收寄后处置请求',
  description:
    '读取入口已配置,清单为空——客户在取消边界之后提出的停止请求被保全为待处置后才会列在这里;没有积压是答案,不是缺陷。',
} as const;

export function requestQueueViewState(
  answer: ApiResult<PostIntakeDispositionRequestsResponseBody> | null,
  retry: () => void,
  module: ModuleInfo,
): TemplateViewState {
  const count = answer?.kind === 'outcome' ? answer.body.requests.length : 0;
  return catalogueViewState(answer, count, retry, {
    module,
    endpoint: 'GET /post-intake-disposition-requests',
    emptyTitle: requestQueueEmptyState.title,
    emptyDescription: requestQueueEmptyState.description,
  });
}

// ---- 收寄后服务处置决定 ----

/** 改送必须带目的地,别的去向不带(与服务端 PostIntakeDispositionShapeError 同口径,前端只提前说清)。 */
export function directionBlockedReason(direction: PostIntakeDirection, destination: string): string | undefined {
  if (direction === 'REDIRECT' && destination.trim() === '') return '改送必须先填写目的地。';
  return undefined;
}

/** 草案:回应一份请求,前一版本取该请求带出的链尾版本(没决定过为 0)。决定人由服务端从认证翻译,不在这里。 */
export function decisionDraftOf(
  row: PostIntakeDispositionRequestRow,
  direction: PostIntakeDirection,
  basis: string,
  destination: string,
): PostIntakeDispositionDecisionDraft {
  const draft: PostIntakeDispositionDecisionDraft = {
    parcel: row.parcelId,
    supersedes: row.currentVersion,
    direction,
    requestKey: row.requestKey,
    basis: basis.trim(),
  };
  if (direction === 'REDIRECT') draft.destination = destination.trim();
  return draft;
}

export type OperatorNote =
  | { tone: 'unconfigured'; text: string }
  | { tone: 'refused'; kind: 'loginRequired' | 'notGranted'; text: string }
  | { tone: 'outcome'; text: string }
  | { tone: 'problem'; text: string };

/** 命令口的共同答复格:非业务答案的几格两类命令同形,业务答案由 outcomeText 按各自词表出。 */
function commandNoteOf<Body>(label: string, answer: ApiResult<Body>, outcomeText: (body: Body, status: number) => string, notVisibleCode: string): OperatorNote {
  switch (answer.kind) {
    case 'outcome':
      return { tone: 'outcome', text: outcomeText(answer.body, answer.status) };
    case 'unconfigured':
      return {
        tone: 'unconfigured',
        text: `「${label}」未执行:接入渠道尚未配置(HTTP 403 ACCESS_CHANNEL_NOT_CONFIGURED)。操作者要由已认证的身份翻译得出,页面不代签。`,
      };
    case 'loginRequired':
    case 'notGranted':
      return { tone: 'refused', kind: answer.kind, text: operatorRefusalSentence(answer.kind, `「${label}」未执行`) };
    case 'callerProblem':
      if (answer.code === notVisibleCode) {
        return { tone: 'problem', text: `「${label}」未执行:册上已没有这一笔(统一不可见结果,HTTP 404 ${answer.code})。请重读清单。` };
      }
      if (answer.status === 409) {
        return { tone: 'problem', text: `「${label}」未写入:这一项先被登记成了另一个结果(HTTP 409 ${answer.code})。已重读,以当前结果为准。` };
      }
      return { tone: 'problem', text: `「${label}」被拒(HTTP ${answer.status}):${answer.code}` };
    case 'noAnswer':
      return { tone: 'problem', text: `「${label}」服务端未形成答案(HTTP ${answer.status}):${answer.code}` };
    default:
      return { tone: 'problem', text: `「${label}」未送达:${answer.message}` };
  }
}

export const REQUEST_NOT_VISIBLE = 'POST_INTAKE_DISPOSITION_REQUEST_NOT_VISIBLE';
export const MANUAL_ITEM_NOT_VISIBLE = 'POST_INTAKE_MANUAL_ITEM_NOT_VISIBLE';

/** 每一格 outcome 都是形成了的业务答案(ADR-0022),按词表呈现,不画成故障。 */
function decisionOutcomeText(label: string, body: PostIntakeDispositionDecisionResponseBody): string {
  const decision = body.decision;
  const owners = decision
    ? decision.ownerRequests.map((request) => `${ownerLabel(request.owner)}:${actionLabel(request.action)}`).join('、')
    : '';
  switch (body.outcome) {
    case 'DECIDED':
      return `「${label}」结果:DECIDED——已形成收寄后服务处置决定${decision ? ` v${decision.version}(${directionLabel(decision.direction)})` : ''},已交各对象所有者的请求:${owners || '无'}。需要人工处理的项见下方「待人工处理项」。`;
    case 'ALREADY_DECIDED':
      return `「${label}」结果:ALREADY_DECIDED——同样的决定此前已形成${decision ? `(v${decision.version},${directionLabel(decision.direction)})` : ''},本次没有再写。`;
    case 'PREDECESSOR_MOVED':
      return `「${label}」结果:PREDECESSOR_MOVED——决定链在你看到之后已经前进${decision ? `(链尾 v${decision.version},${directionLabel(decision.direction)})` : ''},什么也没写。请重读清单后基于链尾再决定。`;
    case 'BOUNDARY_NOT_CROSSED':
      return `「${label}」结果:BOUNDARY_NOT_CROSSED——这件包裹还没越过取消边界,不属收寄后处置,什么也没写。`;
    case 'FINAL_OUTCOME_STANDS':
      return `「${label}」结果:FINAL_OUTCOME_STANDS——包裹已有终局或有效取消${body.finalBasis ? `(依据 ${body.finalBasis})` : ''},收寄后处置不能改写它,什么也没写。`;
    case 'REQUEST_ALREADY_ANSWERED':
      return `「${label}」结果:REQUEST_ALREADY_ANSWERED——这份请求已被别的决定回应过${decision ? `(v${decision.version},${directionLabel(decision.direction)})` : ''},什么也没写。`;
    default:
      return `「${label}」结果:${String(body.outcome)}`;
  }
}

export function decisionNoteOf(label: string, answer: ApiResult<PostIntakeDispositionDecisionResponseBody>): OperatorNote {
  return commandNoteOf(label, answer, (body) => decisionOutcomeText(label, body), REQUEST_NOT_VISIBLE);
}

/** 决定落下(DECIDED)或已有同样的决定后,这份请求不再等回应,清单要重读;其余各格清单不变。 */
export function decisionClearsRequest(answer: ApiResult<PostIntakeDispositionDecisionResponseBody>): boolean {
  if (answer.kind !== 'outcome') return false;
  return answer.body.outcome === 'DECIDED' || answer.body.outcome === 'ALREADY_DECIDED' || answer.body.outcome === 'REQUEST_ALREADY_ANSWERED';
}

// ---- 待人工处理项 ----

export function manualItemKeyOf(item: PostIntakeManualItemRow): string {
  return `${item.parcelId}#${item.decisionVersion}#${item.owner}#${item.action}`;
}

export interface ManualItemLine {
  id: string;
  title: string;
  detail: string;
  superseded: boolean;
  supersededText: string;
  resultText: string;
  /** 结果行里处理人是短名，悬停给带全文的那一份。 */
  resultTitle: string;
}

const outcomeLabels: Record<string, string> = { HANDLED: '已处理', UNABLE: '无法处理' };
export function manualOutcomeLabel(outcome: string): string {
  return outcomeLabels[outcome] ?? outcome;
}

export function manualItemLineOf(item: PostIntakeManualItemRow): ManualItemLine {
  const destination = item.destination ? ` → ${item.destination}` : '';
  return {
    id: manualItemKeyOf(item),
    title: `${item.parcelId} · 决定 v${item.decisionVersion}(${directionLabel(item.direction)}${destination})`,
    detail: `${ownerLabel(item.owner)}:${actionLabel(item.action)} · 决定于 ${formatInstant(item.decidedAt)}`,
    superseded: item.supersededByLaterDecision,
    // 项不因被取代而消失:办之前先看链尾说了什么(ADR-0341)。
    supersededText: item.supersededByLaterDecision
      ? `已被后来的决定取代(链尾 v${item.currentVersion}),办之前先核对链尾决定说了什么`
      : '',
    resultText: item.result
      ? `${manualOutcomeLabel(item.result.outcome)} · ${item.result.note} · ${shortPrincipalRef(item.result.handledBy)} · ${formatInstant(item.result.handledAt)}`
      : '',
    resultTitle: item.result ? `处理人 ${item.result.handledBy}` : '',
  };
}

export const manualOutcomeOptions: readonly { id: ManualItemOutcome; label: string }[] = [
  { id: 'HANDLED', label: '已处理' },
  { id: 'UNABLE', label: '无法处理' },
];

/** 说明必填:做了什么,或为什么办不了。无法处理也是承接结果,当前控制责任不因此消失。 */
export function manualResultBlockedReason(note: string): string | undefined {
  return note.trim() === '' ? '必须填写说明:做了什么,或为什么办不了。' : undefined;
}

export function manualResultDraftOf(item: PostIntakeManualItemRow, outcome: ManualItemOutcome, note: string): PostIntakeManualItemResultDraft {
  return {
    parcel: item.parcelId,
    decisionVersion: item.decisionVersion,
    owner: item.owner,
    action: item.action,
    outcome,
    note: note.trim(),
  };
}

export function manualResultNoteOf(label: string, answer: ApiResult<PostIntakeManualItemResultResponseBody>): OperatorNote {
  return commandNoteOf(
    label,
    answer,
    (body, status) => {
      const result = body.item?.result;
      const now = result ? `当前结果:${manualOutcomeLabel(result.outcome)}(${result.note})` : '';
      if (status === 201) return `「${label}」结果:${body.outcome}——已登记。${now}`;
      return `「${label}」结果:${body.outcome}——这一项此前已登记过同样的结果,本次没有再写。${now}`;
    },
    MANUAL_ITEM_NOT_VISIBLE,
  );
}

export function manualSummaryText(body: PostIntakeManualItemsResponseBody, parcel?: string): string {
  if (parcel) return `包裹 ${parcel} 名下共 ${body.items.length} 项,其中 ${body.pendingCount} 项尚未登记结果`;
  const count = `待人工处理 ${body.pendingCount} 项`;
  return body.truncated ? `${count}(只列出前 ${body.items.length} 项,登记之后其余的会补进来)` : count;
}

export const manualItemsEmptyState = {
  pending: '当前租户没有待人工处理项。收寄后服务处置决定里系统不自动执行的请求(如在控制点扣留、规划退运路线)才会成为待人工处理项;没有积压是答案。',
  readback: '这件包裹名下没有待人工处理项(它没有形成过需要人工处理的收寄后服务处置决定)。',
} as const;
