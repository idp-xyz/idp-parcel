import type { ModuleInfo } from '../../navigation';
import type { TemplateViewState } from '../../templates/state-slot';
import type { TechnicalRow } from '../../components/TechnicalDetails';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  REQUEST_NOT_VISIBLE_CODE,
  type ApiResult,
  type ControlReleaseRefusalAction,
  type ControlReleaseRefusalHandlingDraft,
  type ControlReleaseRefusalHandlingResponseBody,
  type ControlReleaseRefusalListResponseBody,
  type ControlReleaseRefusalRow,
} from './api';
import { shipmentNoOf } from './ref-no';
import { commandNoteOf, type OperatorNote } from './words';

// 资金释放待处理页(释放未受理册,ADR-0293)的呈现逻辑,与页面组件分开住:这一层没有 React,测试能直接钉动作集、
// 存量行提示与各格结果(票 docs-consistency/code/95)。页面组件只做取数与摆放。屏上说运营的话:「占用」是客户余额或
// 额度上被这票运单占着的钱,「结算侧」是记账那一头;标识与结果码进技术详情。

/** 动作集里的一格。页面组件按 id 配图标;这里不带图标,才能在没有 React 的测试里钉它。 */
export interface RefusalActionOption {
  id: ControlReleaseRefusalAction;
  label: string;
  variant: 'secondary' | 'default';
}

/**
 * 动作集恰是两种(ADR-0293 决定二)。两者都不叫「放行」或「通过」:这里处理的是一笔已越过业务决定的资金占用,
 * 不是对委托的判断。「标记已处理」只留痕、不动结算账本,按主按钮档位画,与确定会去释放的那一格区分开。
 */
export const refusalActionOptions: readonly RefusalActionOption[] = [
  { id: 'RELEASE_AT_SETTLEMENT', label: '核对后释放', variant: 'secondary' },
  { id: 'MARK_HANDLED', label: '标记已处理', variant: 'default' },
];

export function refusalActionLabel(action: ControlReleaseRefusalAction): string {
  return refusalActionOptions.find((option) => option.id === action)?.label ?? action;
}

// 处理留痕的做法(handlingKind)。
const handlingKindLabels: Record<string, string> = {
  MARKED_HANDLED: '标记已处理',
  RELEASED_AT_SETTLEMENT: '结算侧已释放',
};
export function handlingKindLabel(kind: string): string {
  return handlingKindLabels[kind] ?? kind;
}

/** 册行的键:与服务端册主键同三段(委托、被拒收的版本、控制关联),同一委托上不同占用不撞键。 */
export function refusalKeyOf(row: ControlReleaseRefusalRow): string {
  return `${row.shipmentRequestId}#${row.submissionVersionId}#${row.controlResultId}`;
}

export interface RefusalQueueRow {
  id: string;
  /** 屏上的运单号(去前缀)。 */
  title: string;
  subtitle: string;
  statusText: string;
  meta: string;
}

export function refusalQueueRowOf(row: ControlReleaseRefusalRow): RefusalQueueRow {
  return {
    id: refusalKeyOf(row),
    title: shipmentNoOf(row.shipmentRequestId),
    subtitle: `客户 ${row.customerAccountId}`,
    statusText: row.scopeRecorded ? '待处理' : '老数据,只能手工处理',
    meta: formatInstant(row.recordedAt),
  };
}

/**
 * 占用在哪:公司主体 / 结算账户 / 币种;没存下的存量行如实说没记下,并点明后果:释放按存下的那一份认领,存量行交零值,
 * 结算侧一律再拒——这一笔只剩「标记已处理」一条路(先在结算侧手工放掉)。不写「无」了事:那会让人以为没有问题。
 */
export function refusalScopeText(row: ControlReleaseRefusalRow): string {
  if (row.scopeRecorded && row.occupationScope) {
    const scope = row.occupationScope;
    return `${scope.legalEntity} / ${scope.settlementAccount} / ${scope.currency}`;
  }
  return '没有记下(老数据)——「核对后释放」会再被结算侧拒绝,请先在结算侧手工放掉,再「标记已处理」';
}

/** 右栏折叠区:三段标识与结算侧的原因码。 */
export function refusalTechnicalRows(row: ControlReleaseRefusalRow): TechnicalRow[] {
  return [
    { label: '委托标识', value: row.shipmentRequestId },
    { label: '客户账户', value: row.customerAccountId },
    { label: '被拒收的提交版本', value: row.submissionVersionId },
    { label: '控制关联', value: row.controlResultId },
    { label: '结算侧原因码', value: row.reason },
  ];
}

/** 草案:只带三段标识、动作与依据。处理人不在这里,由服务端从认证翻译。 */
export function handlingDraftOf(
  row: ControlReleaseRefusalRow,
  action: ControlReleaseRefusalAction,
  basis: string,
): ControlReleaseRefusalHandlingDraft {
  return {
    shipmentRequestId: row.shipmentRequestId,
    submissionVersionId: row.submissionVersionId,
    controlResultId: row.controlResultId,
    action,
    basis: basis.trim(),
  };
}

/** 页头的计数说明。计数是未处理的全部行数;清单被页大小截断时说清只列了最早登记的几笔。 */
export function refusalSummaryText(body: ControlReleaseRefusalListResponseBody): string {
  const count = `待处理 ${body.unhandledCount} 笔`;
  return body.truncated ? `${count}(先列出最早登记的 ${body.refusals.length} 笔,处理之后其余的会补进来)` : count;
}

/** 空清单的措辞:空是答案。 */
export const refusalQueueEmptyState = {
  title: '没有待处理的资金释放',
  description: '运单撤回、拒绝或处置之后,结算侧没放掉占用的钱时才会记到这里;现在没有积压。',
} as const;

export function refusalQueueViewState(
  answer: ApiResult<ControlReleaseRefusalListResponseBody> | null,
  retry: () => void,
  module: ModuleInfo,
): TemplateViewState {
  const count = answer?.kind === 'outcome' ? answer.body.refusals.length : 0;
  return catalogueViewState(answer, count, retry, {
    module,
    endpoint: 'GET /control-release-refusals',
    emptyTitle: refusalQueueEmptyState.title,
    emptyDescription: refusalQueueEmptyState.description,
  });
}

export type RefusalHandlingNote = OperatorNote;

export function refusalHandlingNoteOf(label: string, answer: ApiResult<ControlReleaseRefusalHandlingResponseBody>): RefusalHandlingNote {
  return commandNoteOf(label, answer, handlingOutcomeText, (body) => body.outcome, (problem) =>
    // 统一不可见结果:这一笔在册上已没有(或不属于当前租户),重发同样内容不会改变;提示重读清单,不暗示是权限问题。
    problem.code === REQUEST_NOT_VISIBLE_CODE ? `「${label}」没有执行:清单上已经没有这一笔,可能刚被别人处理了。请刷新清单再看。` : undefined,
  );
}

// 每一格 outcome 都是形成了的业务答案(ADR-0022),按词表呈现,不画成故障。
function handlingOutcomeText(body: ControlReleaseRefusalHandlingResponseBody): string {
  const kind = body.handlingKind ? handlingKindLabel(body.handlingKind) : '';
  const at = body.handledAt ? formatInstant(body.handledAt) : '';
  const how = [kind, at].filter(Boolean).join(' · ');
  switch (body.outcome) {
    case 'HANDLED':
      return `已处理${how ? `(${how})` : ''}。系统会接着处理这票运单;这一笔已从清单移除。`;
    case 'ALREADY_HANDLED':
      return `这一笔刚被别人处理了${how ? `(${how})` : ''},这次没有再记。`;
    case 'RELEASE_NOT_SETTLED':
      return '结算侧这次没有确认放掉,什么也没记,这一笔还在;可以稍后再试。';
    case 'RELEASE_REFUSED_AGAIN':
      return (
        `结算侧又一次拒绝了释放${body.reason ? `(原因见技术详情)` : ''},什么也没记,这一笔还在。` +
        '再试会是同一个答案;请先在结算侧解决拒绝的原因,或手工放掉之后「标记已处理」。'
      );
    default:
      return '已处理,结果见技术详情。';
  }
}
