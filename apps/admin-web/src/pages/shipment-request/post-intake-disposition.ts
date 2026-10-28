import { operatorRefusalMessage } from '../../auth/access-refusal';
import type { ModuleInfo } from '../../navigation';
import type { TemplateViewState } from '../../templates/state-slot';
import type { TechnicalRow } from '../../components/TechnicalDetails';
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
import { parcelNoOf } from './ref-no';

// 货物处置页(UC-PS-006 的收寄后服务处置决定;ADR-0332、ADR-0341)的呈现逻辑,与页面组件分开住:
// 这一层没有 React,测试能直接钉去向集、草案形状、各格结果与空态措辞。
// 屏上说运营的话:「待办事项」是领域里的「待人工处理项」,「第 N 次决定」是决定链的版本号;
// 响应里的码与版本号一律收进技术详情,不嵌进正文。

export interface DirectionOption {
  id: PostIntakeDirection;
  label: string;
  variant: 'secondary' | 'default' | 'danger';
}

/** 去向恰是五种(ADR-0332 决定一)。拦截、退运、服务终止会改变包裹走向,按强调档位画;继续按中性档位。 */
export const directionOptions: readonly DirectionOption[] = [
  { id: 'CONTINUE', label: '继续运送', variant: 'secondary' },
  { id: 'INTERCEPT', label: '拦截', variant: 'default' },
  { id: 'REDIRECT', label: '改送', variant: 'default' },
  { id: 'RETURN', label: '退运', variant: 'default' },
  { id: 'TERMINATE_SERVICE', label: '终止服务', variant: 'danger' },
];

export function directionLabel(direction: string): string {
  return directionOptions.find((option) => option.id === direction)?.label ?? direction;
}

// 对象所有者按它在运营里的岗位称呼:线路、网点、运输。
const ownerLabels: Record<string, string> = {
  NETWORK_ROUTING: '线路',
  NODE_OPERATIONS: '网点',
  TRANSPORT_FULFILLMENT: '运输',
};
export function ownerLabel(owner: string): string {
  return ownerLabels[owner] ?? owner;
}

const actionLabels: Record<string, string> = {
  HOLD_AT_CONTROL: '在网点扣件',
  HOLD_ONWARD_MOVEMENT: '暂停继续运送',
  REPLAN_TO_ALTERNATE_DESTINATION: '改线到新目的地',
  START_ALTERNATE_JOURNEY: '按新目的地发运',
  PLAN_RETURN_ROUTE: '规划退运路线',
  LOAD_FOR_RETURN: '退运装车',
  START_RETURN_JOURNEY: '退运发车',
  RELEASE_UNCONSUMED_PLAN: '取消未用的线路计划',
};
export function actionLabel(action: string): string {
  return actionLabels[action] ?? action;
}

// 请求为什么在等处置(ports.PostIntakeDispositionRequestOrigin 的字符串)。
const originLabels: Record<string, string> = {
  BOUNDARY_CROSSED_AT_REQUEST: '申请取消时货已收寄',
  CANCELLATION_NOT_UPHELD: '先前的取消被认定无效',
};
export function originLabel(origin: string): string {
  return originLabels[origin] ?? origin;
}

// ---- 待决请求 ----

export function requestKeyOf(row: PostIntakeDispositionRequestRow): string {
  return `${row.parcelId}#${row.requestKey}`;
}

export interface RequestQueueRow {
  id: string;
  /** 屏上的包裹号(去前缀);整串在 parcelId 上。 */
  title: string;
  parcelId: string;
  subtitle: string;
  statusText: string;
  meta: string;
}

/** 决定链版本的屏上说法:0 是还没决定过,N 是已决定 N 次。 */
export function decisionCountText(version: number): string {
  return version === 0 ? '尚未处置' : `已处置 ${version} 次`;
}

export function requestQueueRowOf(row: PostIntakeDispositionRequestRow): RequestQueueRow {
  return {
    id: requestKeyOf(row),
    title: parcelNoOf(row.parcelId),
    parcelId: row.parcelId,
    subtitle: row.reason ? `${originLabel(row.origin)} · ${row.reason}` : originLabel(row.origin),
    statusText: decisionCountText(row.currentVersion),
    meta: formatInstant(row.recordedAt),
  };
}

/** 请求上为空的字段说「当时没有记下」,不说没有请求方。 */
const notPreserved = '当时没有记下';

// 请求方是「主体种类:标识」的引用(如 CUSTOMER_ACCOUNT:SYN-ACCOUNT-01);屏上按种类称呼,标识照显。
const requesterKindLabels: Record<string, string> = { CUSTOMER_ACCOUNT: '客户', OPERATOR: '操作员' };
export function requesterLabel(ref: string): string {
  const at = ref.indexOf(':');
  if (at < 0) return shortPrincipalRef(ref);
  const kind = requesterKindLabels[ref.slice(0, at)];
  return kind ? `${kind} ${ref.slice(at + 1)}` : shortPrincipalRef(ref);
}

export interface RequestDetailRow {
  label: string;
  value: string;
  mono?: boolean;
  title?: string;
}

/** 右栏的业务行:号、为什么等处置、谁提的、原因、时间。 */
export function requestDetailRows(row: PostIntakeDispositionRequestRow): RequestDetailRow[] {
  return [
    { label: '包裹号', value: parcelNoOf(row.parcelId), mono: true, title: row.parcelId },
    { label: '为什么等处置', value: originLabel(row.origin) },
    { label: '请求方', value: row.requester ? requesterLabel(row.requester) : notPreserved, title: row.requester || undefined },
    { label: '原因', value: row.reason || notPreserved },
    { label: '请求时间', value: row.requestedAt ? formatInstant(row.requestedAt) : notPreserved },
    { label: '登记时间', value: formatInstant(row.recordedAt) },
    { label: '处置记录', value: decisionCountText(row.currentVersion) },
  ];
}

/** 右栏折叠区:整串标识、请求键、授权与边界引用,排查与对协议时才用。 */
export function requestTechnicalRows(row: PostIntakeDispositionRequestRow): TechnicalRow[] {
  return [
    { label: '包裹标识', value: row.parcelId },
    { label: '请求键', value: row.requestKey },
    { label: '来源', value: row.origin },
    { label: '授权', value: row.authority ?? '' },
    { label: '取消边界前', value: row.intakeBefore ?? '' },
    { label: '取消记录', value: row.cancellation ?? '' },
    { label: '决定链尾版本', value: String(row.currentVersion) },
  ];
}

export function requestSummaryText(body: PostIntakeDispositionRequestsResponseBody): string {
  const count = `待处置 ${body.pendingCount} 票`;
  return body.truncated ? `${count}(先列出 ${body.requests.length} 票,处置之后其余的会补进来)` : count;
}

export const requestQueueEmptyState = {
  title: '没有等待处置的运单',
  description: '客户在货物收寄之后申请停止运送,这里才会出现待处置的运单;现在没有积压。',
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
  if (direction === 'REDIRECT' && destination.trim() === '') return '选「改送」要先填新目的地。';
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

/**
 * 写动作之后给操作员看的一句话。正文只说业务上怎么回事、下一步做什么;状态码与结果码收进 technical,
 * 由页面放进「技术详情」折叠区——报障或对日志时展开就有原名。
 */
export type OperatorNote =
  | { tone: 'unconfigured'; text: string; technical: TechnicalRow[] }
  | { tone: 'refused'; kind: 'loginRequired' | 'notGranted'; text: string; technical: TechnicalRow[] }
  | { tone: 'outcome'; text: string; technical: TechnicalRow[] }
  | { tone: 'problem'; text: string; technical: TechnicalRow[] };

const noteRows = (status: number | undefined, code: string): TechnicalRow[] => [
  { label: 'HTTP 状态', value: status === undefined ? '' : String(status) },
  { label: '结果码', value: code },
];

/** 命令口的共同答复格:非业务答案的几格两类命令同形,业务答案由 outcomeText 按各自词表出。 */
function commandNoteOf<Body>(
  label: string,
  answer: ApiResult<Body>,
  outcomeText: (body: Body, status: number) => string,
  outcomeCode: (body: Body) => string,
  notVisibleCode: string,
): OperatorNote {
  switch (answer.kind) {
    case 'outcome':
      return { tone: 'outcome', text: outcomeText(answer.body, answer.status), technical: noteRows(answer.status, outcomeCode(answer.body)) };
    case 'unconfigured':
      return {
        tone: 'unconfigured',
        text: `「${label}」没有执行:这台管理台还没接上操作者登录渠道,请部署的人配置后再试。`,
        technical: noteRows(403, 'ACCESS_CHANNEL_NOT_CONFIGURED'),
      };
    case 'loginRequired':
    case 'notGranted':
      return {
        tone: 'refused',
        kind: answer.kind,
        text: operatorRefusalMessage(answer.kind, `「${label}」没有执行`),
        technical: noteRows(answer.kind === 'loginRequired' ? 401 : 403, answer.kind === 'loginRequired' ? 'OPERATOR_CREDENTIAL_REJECTED' : 'OPERATOR_NOT_GRANTED'),
      };
    case 'callerProblem':
      if (answer.code === notVisibleCode) {
        return { tone: 'problem', text: `「${label}」没有执行:清单上已经没有这一笔,可能刚被别人处理了。请刷新清单再看。`, technical: noteRows(answer.status, answer.code) };
      }
      if (answer.status === 409) {
        return { tone: 'problem', text: `「${label}」没有写入:这一项刚被别人登记成了另一个结果。已刷新,以现在显示的结果为准。`, technical: noteRows(answer.status, answer.code) };
      }
      return { tone: 'problem', text: `「${label}」没有被受理,请检查填写内容后再试。`, technical: noteRows(answer.status, answer.code) };
    case 'noAnswer':
      return { tone: 'problem', text: `「${label}」这次没有处理成功,可以稍后再试;重试不会重复登记。`, technical: noteRows(answer.status, answer.code) };
    default:
      return { tone: 'problem', text: `「${label}」没有送到服务器,请检查网络后再试。`, technical: [{ label: '错误', value: answer.message }] };
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
  const which = decision ? `(第 ${decision.version} 次,${directionLabel(decision.direction)})` : '';
  switch (body.outcome) {
    case 'DECIDED':
      return `已决定${which}。已通知各岗位去办:${owners || '无'}。需要人工办的事项见下方「待办事项」。`;
    case 'ALREADY_DECIDED':
      return `同样的决定此前已经做过${which},这次没有再记。`;
    case 'PREDECESSOR_MOVED':
      return `在你看到之后,这票货已被别人处置过${which},这次没有记。请刷新清单,按最新一次决定再定。`;
    case 'BOUNDARY_NOT_CROSSED':
      return '这票货还没收寄,不走货物处置,这次没有记;要停请走「取消运单」。';
    case 'FINAL_OUTCOME_STANDS':
      return `这票货已经有终局结果(签收、取消等)${body.finalBasis ? `,依据 ${body.finalBasis}` : ''},货物处置改不了它,这次没有记。`;
    case 'REQUEST_ALREADY_ANSWERED':
      return `这份请求已被别的决定回应过${which},这次没有记。`;
    default:
      return `「${label}」已处理,结果见技术详情。`;
  }
}

export function decisionNoteOf(label: string, answer: ApiResult<PostIntakeDispositionDecisionResponseBody>): OperatorNote {
  return commandNoteOf(label, answer, (body) => decisionOutcomeText(label, body), (body) => body.outcome, REQUEST_NOT_VISIBLE);
}

/** 决定落下(DECIDED)或已有同样的决定后,这份请求不再等回应,清单要重读;其余各格清单不变。 */
export function decisionClearsRequest(answer: ApiResult<PostIntakeDispositionDecisionResponseBody>): boolean {
  if (answer.kind !== 'outcome') return false;
  return answer.body.outcome === 'DECIDED' || answer.body.outcome === 'ALREADY_DECIDED' || answer.body.outcome === 'REQUEST_ALREADY_ANSWERED';
}

// ---- 待办事项(待人工处理项) ----

export function manualItemKeyOf(item: PostIntakeManualItemRow): string {
  return `${item.parcelId}#${item.decisionVersion}#${item.owner}#${item.action}`;
}

export interface ManualItemLine {
  id: string;
  /** 屏上的包裹号(去前缀)。 */
  parcelNo: string;
  parcelId: string;
  /** 要办的事,如「网点:在网点扣件」。 */
  task: string;
  /** 来自哪一次决定,如「第 1 次决定(拦截 → 某地)」。 */
  decisionText: string;
  decidedAt: string;
  superseded: boolean;
  supersededText: string;
  resultText: string;
  /** 结果行里处理人是短名，悬停给带全文的那一份。 */
  resultTitle: string;
  technical: TechnicalRow[];
}

const outcomeLabels: Record<string, string> = { HANDLED: '已办', UNABLE: '办不了' };
export function manualOutcomeLabel(outcome: string): string {
  return outcomeLabels[outcome] ?? outcome;
}

export function manualItemLineOf(item: PostIntakeManualItemRow): ManualItemLine {
  const destination = item.destination ? ` → ${item.destination}` : '';
  return {
    id: manualItemKeyOf(item),
    parcelNo: parcelNoOf(item.parcelId),
    parcelId: item.parcelId,
    task: `${ownerLabel(item.owner)}:${actionLabel(item.action)}`,
    decisionText: `第 ${item.decisionVersion} 次决定(${directionLabel(item.direction)}${destination})`,
    decidedAt: formatInstant(item.decidedAt),
    superseded: item.supersededByLaterDecision,
    // 项不因被取代而消失:办之前先看最新决定说了什么(ADR-0341)。
    supersededText: item.supersededByLaterDecision
      ? `之后又做过决定(现在是第 ${item.currentVersion} 次),办之前先看最新一次决定怎么说`
      : '',
    resultText: item.result
      ? `${manualOutcomeLabel(item.result.outcome)} · ${item.result.note} · ${shortPrincipalRef(item.result.handledBy)} · ${formatInstant(item.result.handledAt)}`
      : '',
    resultTitle: item.result ? `处理人 ${item.result.handledBy}` : '',
    technical: [
      { label: '包裹标识', value: item.parcelId },
      { label: '所有者', value: item.owner },
      { label: '动作', value: item.action },
      { label: '决定版本', value: String(item.decisionVersion) },
      { label: '链尾版本', value: String(item.currentVersion) },
    ],
  };
}

export const manualOutcomeOptions: readonly { id: ManualItemOutcome; label: string }[] = [
  { id: 'HANDLED', label: '已办' },
  { id: 'UNABLE', label: '办不了' },
];

/** 说明必填:做了什么,或为什么办不了。办不了也是承接结果,当前控制责任不因此消失。 */
export function manualResultBlockedReason(note: string): string | undefined {
  return note.trim() === '' ? '请写一句说明:做了什么,或为什么办不了。' : undefined;
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
      const now = result ? `现在的结果:${manualOutcomeLabel(result.outcome)}(${result.note})` : '';
      if (status === 201) return `已登记。${now}`;
      return `这一项此前已登记过同样的结果,这次没有再记。${now}`;
    },
    (body) => body.outcome,
    MANUAL_ITEM_NOT_VISIBLE,
  );
}

export function manualSummaryText(body: PostIntakeManualItemsResponseBody, parcel?: string): string {
  if (parcel) return `包裹 ${parcelNoOf(parcel)} 共 ${body.items.length} 项,其中 ${body.pendingCount} 项还没登记结果`;
  const count = `待办 ${body.pendingCount} 项`;
  return body.truncated ? `${count}(先列出 ${body.items.length} 项,登记之后其余的会补进来)` : count;
}

export const manualItemsEmptyState = {
  pending: '现在没有待办事项。处置决定里系统办不了、要人去办的事(如在网点扣件、规划退运路线)才会列在这里。',
  readback: '这票货没有待办事项。',
} as const;
