import { operatorRefusalSentence, type OperatorRefusalKind } from '../../auth/access-refusal';
import type { ModuleInfo } from '../../navigation';
import type { TemplateViewState } from '../../templates/state-slot';
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

// 释放未受理册查阅与处理页的呈现逻辑,与页面组件分开住:这一层没有 React,测试能直接钉动作集、存量行提示与各格结果
// (票 docs-consistency/code/95;ADR-0293)。页面组件只做取数与摆放。

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
  { id: 'RELEASE_AT_SETTLEMENT', label: '核对后在结算侧释放', variant: 'secondary' },
  { id: 'MARK_HANDLED', label: '标记已处理', variant: 'default' },
];

const actionLabels: Record<ControlReleaseRefusalAction, string> = {
  RELEASE_AT_SETTLEMENT: '核对后在结算侧释放',
  MARK_HANDLED: '标记已处理',
};

export function refusalActionLabel(action: ControlReleaseRefusalAction): string {
  return actionLabels[action];
}

/** 册行的键:与服务端册主键同三段(委托、被拒收的版本、控制关联),同一委托上不同占用不撞键。 */
export function refusalKeyOf(row: ControlReleaseRefusalRow): string {
  return `${row.shipmentRequestId}#${row.submissionVersionId}#${row.controlResultId}`;
}

export interface RefusalQueueRow {
  id: string;
  title: string;
  subtitle: string;
  statusText: string;
  meta: string;
}

export function refusalQueueRowOf(row: ControlReleaseRefusalRow): RefusalQueueRow {
  return {
    id: refusalKeyOf(row),
    title: row.shipmentRequestId,
    subtitle: `${row.customerAccountId} · 被拒收的版本 ${row.submissionVersionId}`,
    statusText: row.scopeRecorded ? '作用域已存下' : '未存作用域(存量行)',
    meta: formatInstant(row.recordedAt),
  };
}

/**
 * 存下的作用域三段;没存下的存量行如实说没存下,并点明后果:释放按存下的那一份认领,存量行交零值,提供方一律再拒收——
 * 这一笔只剩「标记已处理」一条路(先在结算侧手工放掉)。不写「无作用域」了事:那会让人以为没有问题。
 */
export function refusalScopeText(row: ControlReleaseRefusalRow): string {
  if (row.scopeRecorded && row.occupationScope) {
    const scope = row.occupationScope;
    return `${scope.legalEntity} / ${scope.settlementAccount} / ${scope.currency}`;
  }
  return '未存下(施加控制时还不存作用域的存量行)——「核对后在结算侧释放」会再被提供方拒收,只能先在结算侧手工放掉再「标记已处理」';
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
  const count = `未处理 ${body.unhandledCount} 笔`;
  return body.truncated ? `${count}(清单只列出最早登记的 ${body.refusals.length} 笔,处理之后其余的会补进来)` : count;
}

/** 空清单的措辞:空是答案。 */
export const refusalQueueEmptyState = {
  title: '当前租户没有待处理的释放拒收',
  description:
    '读取入口已配置,清单为空——撤回、拒绝、授权处置、自动拒绝与停进等待受控补充各路遇到结算确定不受理的释放时才会记进这本册;没有积压是答案,不是缺陷。',
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

/**
 * 处理命令口的一次答复。渠道未配置是预期会遇到的答案,不是故障,单列一格;需要登录与没有授予同理,`refused` 多带那一格,
 * 页面据它在「需要登录」时接一颗重新登录(与授权处置页同形)。
 */
export type RefusalHandlingNote =
  | { tone: 'unconfigured'; text: string }
  | { tone: 'refused'; kind: OperatorRefusalKind; text: string }
  | { tone: 'outcome'; text: string }
  | { tone: 'problem'; text: string };

export function refusalHandlingNoteOf(
  label: string,
  answer: ApiResult<ControlReleaseRefusalHandlingResponseBody>,
): RefusalHandlingNote {
  switch (answer.kind) {
    case 'outcome':
      return { tone: 'outcome', text: handlingOutcomeText(label, answer.body) };
    case 'unconfigured':
      return {
        tone: 'unconfigured',
        text: `「${label}」未执行:接入渠道尚未配置(HTTP 403 ACCESS_CHANNEL_NOT_CONFIGURED)。处理人要由已认证的操作员身份翻译得出,页面不代签。`,
      };
    case 'loginRequired':
    case 'notGranted':
      return { tone: 'refused', kind: answer.kind, text: operatorRefusalSentence(answer.kind, `「${label}」未执行`) };
    case 'callerProblem':
      if (answer.code === REQUEST_NOT_VISIBLE_CODE) {
        // 统一不可见结果:这一笔在册上已没有(或不属于当前租户),重发同样内容不会改变;提示重读清单,不暗示是权限问题。
        return {
          tone: 'problem',
          text: `「${label}」未执行:册上已没有这一笔(统一不可见结果,HTTP 404 ${answer.code})。请重读清单。`,
        };
      }
      return { tone: 'problem', text: `「${label}」被拒(HTTP ${answer.status}):${answer.code}` };
    case 'noAnswer':
      return { tone: 'problem', text: `「${label}」服务端未形成答案(HTTP ${answer.status}):${answer.code}` };
    default:
      return { tone: 'problem', text: `「${label}」未送达:${answer.message}` };
  }
}

// 每一格 outcome 都是形成了的业务答案(ADR-0022),按词表呈现,不画成故障。
function handlingOutcomeText(label: string, body: ControlReleaseRefusalHandlingResponseBody): string {
  const kind = body.handlingKind ?? '';
  const at = body.handledAt ? formatInstant(body.handledAt) : '';
  switch (body.outcome) {
    case 'HANDLED':
      return (
        `「${label}」结果:HANDLED——处理留痕已落库(${kind}${at ? ` · ${at}` : ''}),同笔已交一封续办信封,` +
        '接受链会拿这份委托此刻的当前版本再驱一拍;那一笔从清单里消失。'
      );
    case 'ALREADY_HANDLED':
      return (
        `「${label}」结果:ALREADY_HANDLED——这一笔先被别人处理了(先到的做法 ${kind}${at ? ` · ${at}` : ''}),` +
        '本次没有再写,也没有再交续办信封。'
      );
    case 'RELEASE_NOT_SETTLED':
      return `「${label}」结果:RELEASE_NOT_SETTLED——结算侧这一回没能收口(账本读不回或依赖答不出),什么也没写,这一笔仍未处理;稍后可再试。`;
    case 'RELEASE_REFUSED_AGAIN':
      return (
        `「${label}」结果:RELEASE_REFUSED_AGAIN——结算侧又一次确定不受理${body.reason ? `(原因 ${body.reason})` : ''},什么也没写,这一笔仍未处理。` +
        '再试会得到同一个答案;要么先在结算侧把拒收的缘由解掉,要么在结算侧手工放掉之后「标记已处理」。'
      );
    default:
      return `「${label}」结果:${String(body.outcome)}`;
  }
}
