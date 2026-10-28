import type { ModuleInfo } from '../../navigation';
import type { TemplateViewState } from '../../templates/state-slot';
import type { TechnicalRow } from '../../components/TechnicalDetails';
import { labelFor } from '../../vocabulary/status';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import type {
  ApiResult,
  AuthorizedDispositionChoice,
  AuthorizedDispositionQueueEntry,
  AuthorizedDispositionQueueListResponseBody,
  AuthorizedDispositionResponseBody,
  RestrictedControlItemRecord,
} from './api';
import { authorizedDispositionOutcomeViews, controlFailureDispositionLabels, dispositionChoiceLabels } from './presentation';
import { shipmentNoOf } from './ref-no';
import { commandNoteOf, type OperatorNote } from './words';

// 授权处置页的呈现逻辑,与页面组件分开住:这一层没有 React,测试能直接钉三态与决定集
// (票 sa-preacceptance-policy-view/05)。页面组件只做取数与摆放。屏上说运营的话:码经词典译,
// 标识与版本进技术详情。

/** 决定集里的一格。页面组件按 id 配图标;这里不带图标,才能在没有 React 的测试里钉它。 */
export interface DispositionChoiceOption {
  id: AuthorizedDispositionChoice;
  label: string;
  variant: 'danger' | 'secondary';
}

/**
 * 决定集恰是两去向(ADR-0132 决定一)。`拒绝`按危险档,`交客户补充`按中性档——两者都不是「批准」:
 * 授权处置是对受限委托决定去向,不是对受限控制的表决,所以没有一格用肯定档位画,也不会有第三格。
 */
export const dispositionChoiceOptions: readonly DispositionChoiceOption[] = [
  { id: 'REJECT', label: dispositionChoiceLabels.REJECT, variant: 'danger' },
  { id: 'CUSTOMER_SUPPLEMENT', label: dispositionChoiceLabels.CUSTOMER_SUPPLEMENT, variant: 'secondary' },
];

export interface DispositionQueueRow {
  id: string;
  /** 屏上的运单号(去前缀)。 */
  title: string;
  subtitle: string;
  /** 队列行右侧的状态字:受限项数;受限项为空数组时改说数据异常。 */
  statusText: string;
  /**
   * 停在等处置的委托不该没有受限项——读面对这一格给的是空数组而不是 null,正是为了让它可观察。
   * 页面把它标出来而不是当成「没有受限项」:两种读法要人做的事相反。
   */
  badData: boolean;
  meta: string;
}

export function dispositionQueueRowOf(entry: AuthorizedDispositionQueueEntry): DispositionQueueRow {
  const count = entry.restrictedItems.length;
  return {
    id: entry.shipmentRequestId,
    title: shipmentNoOf(entry.shipmentRequestId),
    subtitle: `客户 ${entry.customerAccountId} · ${entry.declaredParcelCount} 件`,
    statusText: count === 0 ? '没有受限项(数据异常)' : `${count} 项未通过`,
    badData: count === 0,
    meta: formatInstant(entry.submittedAt),
  };
}

/**
 * 一项受限控制的一行文案:哪项检查没过、为什么、策略说不过时怎么办、责任在谁。种类与原因经词典译;
 * 两格采用引用没有就不写——没采用过就是没采用过,写「无」会把「读过了,答案是没有」与「采用之前记下的」并成一句。
 */
export function restrictedItemText(item: RestrictedControlItemRecord): string {
  const parts = [labelFor(item.kind)];
  if (item.basis) parts.push(labelFor(item.basis));
  if (item.failureDisposition) {
    parts.push(`不通过时${controlFailureDispositionLabels[item.failureDisposition] ?? item.failureDisposition}`);
  }
  if (item.responsibility) parts.push(`责任方 ${item.responsibility}`);
  return parts.join(' · ');
}

/** 受限项的原码与顺序,进技术详情。 */
export function restrictedItemTechnicalRows(items: readonly RestrictedControlItemRecord[]): TechnicalRow[] {
  return items.map((item, index) => ({
    label: `受限项 ${index + 1}`,
    value: [item.kind, `#${item.order}`, item.basis ?? '', item.failureDisposition ?? '', item.responsibility ?? ''].filter(Boolean).join(' · '),
  }));
}

export type DispositionCommandNote = OperatorNote;

export function dispositionCommandNoteOf(label: string, answer: ApiResult<AuthorizedDispositionResponseBody>): DispositionCommandNote {
  return commandNoteOf(label, answer, (body) => dispositionOutcomeText(body), (body) => body.outcome);
}

// 每一格 outcome 都是形成了的业务答案,按词表呈现;随附凭据有才显:去向、之后的运单状态、既有决定、
// 释放未受理待人处理(ADR-0282 决定二,处置照样成立)。当前版本与补偿续办引用是标识,进技术详情。
function dispositionOutcomeText(body: AuthorizedDispositionResponseBody): string {
  const view = authorizedDispositionOutcomeViews[body.outcome];
  const parts = [view?.label ?? '已处理'];
  if (body.dispositionChoice) parts.push(`去向:${dispositionChoiceLabels[body.dispositionChoice]}`);
  if (body.requestState) parts.push(`运单现在${labelFor(body.requestState)}`);
  if (body.decisionKind) parts.push(`已有决定:${labelFor(body.decisionKind)}`);
  if (body.compensationReference) parts.push('占用的资金还没释放完,系统会继续处理');
  if (body.controlReleaseNotAccepted) parts.push('占用的资金结算侧没有受理释放,已转到「释放拒收处理」等人处理');
  const head = parts.join(' · ');
  return view ? `${head}——${view.note}` : head;
}

/** 空队列的措辞:空是答案。 */
export const dispositionQueueEmptyState = {
  title: '没有等待授权处置的运单',
  description: '受理前的付款控制没通过、而策略要求由有权的人决定去向时,运单才会停在这里;现在没有积压。',
} as const;

export function dispositionQueueViewState(
  answer: ApiResult<AuthorizedDispositionQueueListResponseBody> | null,
  retry: () => void,
  module: ModuleInfo,
): TemplateViewState {
  const count = answer?.kind === 'outcome' ? answer.body.entries.length : 0;
  return catalogueViewState(answer, count, retry, {
    module,
    endpoint: 'GET /authorized-disposition-queue',
    emptyTitle: dispositionQueueEmptyState.title,
    emptyDescription: dispositionQueueEmptyState.description,
  });
}
