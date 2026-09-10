import type { ModuleInfo } from '../../navigation';
import type { TemplateViewState } from '../../templates/state-slot';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import type {
  ApiResult,
  AuthorizedDispositionChoice,
  AuthorizedDispositionQueueEntry,
  AuthorizedDispositionQueueListResponseBody,
  AuthorizedDispositionResponseBody,
  RestrictedControlItemRecord,
} from './api';
import {
  authorizedDispositionOutcomeViews,
  controlFailureDispositionLabels,
  dispositionChoiceLabels,
  withCode,
} from './presentation';

// 授权处置页的呈现逻辑,与页面组件分开住:这一层没有 React,测试能直接钉三态与决定集
// (票 sa-preacceptance-policy-view/05)。页面组件只做取数与摆放。

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
  title: string;
  subtitle: string;
  /** 队列行右侧的状态字:受限项数;受限项为空数组时改说坏数据征兆。 */
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
    title: entry.shipmentRequestId,
    subtitle: `${entry.customerAccountId} · ${entry.declaredParcelCount} 件 · 版本 ${entry.submissionVersionId}`,
    statusText: count === 0 ? '受限项为空(坏数据征兆)' : `${count} 项受限`,
    badData: count === 0,
    meta: formatInstant(entry.submittedAt),
  };
}

/**
 * 一项受限控制的一行文案。种类、受限原因、失败处置、责任引用一律原词直显;失败处置另附 PC CONTEXT
 * 的中文。两格采用引用没有就不写——没采用过就是没采用过,写「无」会把「读过了,答案是没有」与
 * 「采用之前记下的」并成一句。
 */
export function restrictedItemText(item: RestrictedControlItemRecord): string {
  const parts = [`${item.kind} · 第 ${item.order} 项`];
  if (item.basis) parts.push(`受限原因 ${item.basis}`);
  if (item.failureDisposition) {
    parts.push(
      `失败处置 ${withCode(controlFailureDispositionLabels[item.failureDisposition], item.failureDisposition)}`,
    );
  }
  if (item.responsibility) parts.push(`责任 ${item.responsibility}`);
  return parts.join(' · ');
}

/** 处置命令口的一次答复。渠道未配置是今天的预期答案,不是故障,因此单列一格(与复核页同形)。 */
export type DispositionCommandNote =
  | { tone: 'unconfigured'; text: string }
  | { tone: 'outcome'; text: string }
  | { tone: 'problem'; text: string };

export function dispositionCommandNoteOf(
  label: string,
  answer: ApiResult<AuthorizedDispositionResponseBody>,
): DispositionCommandNote {
  switch (answer.kind) {
    case 'outcome':
      return { tone: 'outcome', text: dispositionOutcomeText(label, answer.body) };
    case 'unconfigured':
      // 命令面挂字面量 UnconfiguredIntake(ADR-0085 / ADR-0055):如实说渠道未配置,不退化成「稍后重试」,
      // 也不造开发用采信身份把它绕过去——处置人与授权依据要由已认证的操作员身份翻译得出。
      return {
        tone: 'unconfigured',
        text: `「${label}」未执行:接入渠道尚未配置(HTTP 403 ACCESS_CHANNEL_NOT_CONFIGURED)。处置人与授权依据要由已认证的操作员身份翻译得出(PAR-INT-01),页面不代签,也不替任何角色决定去向。`,
      };
    case 'callerProblem':
      return { tone: 'problem', text: `「${label}」被拒(HTTP ${answer.status}):${answer.code}` };
    case 'noAnswer':
      return {
        tone: 'problem',
        text: `「${label}」服务端未形成答案(HTTP ${answer.status}):${answer.code}`,
      };
    default:
      return { tone: 'problem', text: `「${label}」未送达:${answer.message}` };
  }
}

// 每一格 outcome 都是形成了的业务答案,按词表呈现;随附凭据有才显:去向(原词与中文并列)、之后的
// 委托状态、既有决定、当前版本、补偿续办引用。
function dispositionOutcomeText(label: string, body: AuthorizedDispositionResponseBody): string {
  const view = authorizedDispositionOutcomeViews[body.outcome];
  const parts = [`「${label}」结果:${withCode(view?.label, body.outcome)}`];
  if (body.dispositionChoice) {
    parts.push(`去向 ${withCode(dispositionChoiceLabels[body.dispositionChoice], body.dispositionChoice)}`);
  }
  if (body.requestState) parts.push(`委托状态 ${body.requestState}`);
  if (body.decisionKind) parts.push(`既有决定 ${body.decisionKind}`);
  if (body.currentVersion) parts.push(`当前版本 ${body.currentVersion}`);
  if (body.compensationReference) parts.push(`补偿续办 ${body.compensationReference}`);
  const head = parts.join(' · ');
  return view ? `${head}——${view.note}` : head;
}

/** 空队列的措辞:空是答案。进队列的条件取 ADR-0132 决定一原句,不另造第二种说法。 */
export const dispositionQueueEmptyState = {
  title: '当前作用域没有等待授权处置的委托',
  description:
    '读取入口已配置,队列为空——只有接受前财务控制按共同通过条件不成立、且全部受限控制项在策略正文里登记的失败处置都是进入授权处置时,委托才停在等待授权处置;空队列是答案,不是缺陷。',
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
