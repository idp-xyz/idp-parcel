// 交接范围汇总区的判读与行转写（票 admin-web-audit-followups/06）。
//
// 单独成模块而不写在页面里：这一区的全部风险都在「四格各自该显示什么」上，而页面组件
// 要跑起来得有 React 与 DOM。判读是纯函数就能拿测试钉住（同 party/policy-rows 的先例）。

import type { ModuleInfo } from '../../navigation';
import type { TemplateViewState } from '../../templates/state-slot';
import { catalogueViewState } from '../catalogue-view';
import type { ApiResult } from '../catalogue-api';
import type {
  HandoverScopeSummary,
  HandoverScopeSummaryResponseBody,
} from './records-api';

export interface ScopeSummaryRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

/** 未决成因封闭集（application HandoverScopeUndecidedReason）。 */
export const scopeUndecidedReasonLabels: Record<string, string> = {
  HANDOVER_REGISTRY_UNAVAILABLE: '交接记录暂时读不出来',
};

/**
 * 汇总转写成一行。三格计数与合计各占一列，`全部已交接`取后端派生的那一格布尔值
 * ——它是 CONTEXT「整批结论只能由对象级结果派生」的唯一出处，页面拿三格相加再比对
 * 就是第二个口径。
 *
 * 只有`已汇总`一格有行：另三格没有汇总本体，由状态槽说话（scopeSummaryViewState）。
 * 尤其`不成立汇总`——它在类型上就没有 summary 键，这里因此连「显示三个零」都写不出来。
 */
export function scopeSummaryRowsOf(
  body: HandoverScopeSummaryResponseBody,
): ScopeSummaryRow[] {
  if (body.outcome !== 'SCOPE_SUMMARIZED') return [];
  const summary: HandoverScopeSummary = body.summary;
  return [
    {
      key: `scope:${summary.scope}`,
      values: {
        scope: summary.scope,
        handedOver: String(summary.handedOver),
        refused: String(summary.refused),
        unconfirmed: String(summary.unconfirmed),
        total: String(summary.total),
        allHandedOver: summary.allHandedOver ? '是' : '否',
      },
    },
  ];
}

interface ScopeSummaryViewOptions {
  module: ModuleInfo;
  endpoint: string;
}

/**
 * 四格结果代数各自的呈现态。
 *
 * HTTP 那半（未配置 / 调用方问题 / 未形成答案 / 传输失败）转交 catalogueViewState，
 * 不在这里抄第二份——那套代数是全站共用的，抄过来就要跟着它改两处。本函数只管业务
 * 那半：拿到了 2xx 之后四格分别是什么。
 *
 * 范围未填时不发请求也不报错：汇总按范围派生，没有范围就没有问题可问，这与「问过了、
 * 这个范围还没有交接」不是一件事，两者的文案因此分开写。
 */
export function scopeSummaryViewState(
  answer: ApiResult<HandoverScopeSummaryResponseBody> | null,
  scope: string,
  retry: () => void,
  options: ScopeSummaryViewOptions,
): TemplateViewState {
  if (scope.trim() === '') {
    return {
      kind: 'empty',
      title: '先输入一个交接批次号',
      description:
        '按交接批次汇总这一批交接了几件、拒收几件、待确认几件；在上方输入交接批次号，再点「查询汇总」。',
    };
  }
  if (!answer) return { kind: 'loading' };
  if (answer.kind !== 'outcome') {
    // 行数只在 outcome 那一支被 catalogueViewState 读到，而那一支由下面自己接管。
    return catalogueViewState(answer, 1, retry, {
      module: options.module,
      endpoint: options.endpoint,
      emptyTitle: '',
      emptyDescription: '',
    });
  }

  switch (answer.body.outcome) {
    case 'SCOPE_SUMMARIZED':
      return { kind: 'ready' };
    case 'SCOPE_NOT_SUMMARIZABLE':
      // **不显示三个零。** 零说的是「这个范围有交接，只是这一格没有」，不成立说的是
      // 「这个范围还没有交接」——后端刻意不带 summary 键正是为了不让这两件事同形。
      return {
        kind: 'empty',
        title: `批次 ${scope} 还没有交接`,
        description:
          '这个批次还没有任何一件交接记录，所以汇总不出来；这和「三格都是 0」不一样。',
      };
    case 'SCOPE_UNDECIDED':
      return {
        kind: 'error',
        title: '本次汇总未决',
        description: `${scopeUndecidedReasonLabels[answer.body.reason] ?? answer.body.reason}；续办引用 ${answer.body.continuationReference}。这不是「这个范围没有交接」，重试可能得到答案。`,
        onRetry: retry,
      };
    case 'INPUT_NOT_ACCEPTED':
      return {
        kind: 'error',
        title: '输入未受理',
        description: '没有查到这个交接批次；原样重试还是同一个结果，请核对批次号。',
      };
  }
}
