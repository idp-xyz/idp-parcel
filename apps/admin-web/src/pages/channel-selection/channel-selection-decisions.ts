// 渠道择优决定页的判读与行转写（票 label-channel/23）。
//
// 单独成模块而不写在页面里：这一页的风险都在「每一格该显示什么」上——「此刻没有并列冲突」与「接入渠道
// 未配置」分不分得开、按标识查不到是终局答案还是故障、出局因由会不会被折成一句笼统的「失败」——而页面
// 组件要跑起来得有 React 与 DOM。判读是纯函数就能拿测试钉住（同 effective-time-judgment 的先例）。

import type { ModuleInfo } from '../../navigation';
import type { TemplateViewState } from '../../templates/state-slot';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import type { ApiResult } from '../catalogue-api';
import type {
  ChannelSelectionDecisionRecord,
  ChannelSelectionDecisionResponseBody,
  ChannelSelectionSubjectFilter,
  TiedChannelSelectionDecisionsResponseBody,
} from './api';

/** 逐候选结果封闭四格（parcelshipment domain ChannelCandidateOutcome），中文取票 14 裁决原词。 */
export const candidateOutcomeLabels: Record<string, string> = {
  SELECTED: '选中',
  NOT_SELECTED: '落选',
  EXCLUDED: '出局',
  TIED: '并列',
};

/** 结论三格（domain ChannelSelectionConclusion）：与比较器的三种非错误出口一一对应。 */
export const conclusionLabels: Record<string, string> = {
  SELECTED: '选出唯一一条',
  TIED: '并列冲突——交人工裁决',
  NONE_QUALIFIED: '无人参选',
};

/**
 * 出局因由四格（domain ChannelCostUnavailability），一一对应 parcel-pricing 四种非完成结果；措辞取该文件
 * 逐格注释的续办句——四格续办各不相同，页面不把它们折成一句「不可计价」。
 */
export const exclusionLabels: Record<string, string> = {
  PENDING_EVIDENCE: '待判断——补齐事实即可计价',
  RATECARD_EXCLUSION: '不可计价——价卡明确排除',
  CONFLICT: '冲突——规则、区间或版本互斥，待治理裁决',
  NOT_FORMED: '未形成——请求不合法或计算未完成，可重试',
};

/** 择优规则引用（domain ChannelSelectionRule）。首发只有成本单维。 */
export const ruleLabels: Record<string, string> = {
  COST_ONLY: '成本单维',
};

/** 词表没收录的词原样示出，不猜格：某天新增一格时页面宁可显示英文原名，也不让它冒充既有一格。 */
export function wordOf(table: Record<string, string>, code: string): string {
  return table[code] ?? code;
}

export interface DecisionRow {
  key: string;
  decisionId: string;
  values: Readonly<Record<string, string>>;
}

/**
 * 并列冲突一行。tiedCandidates 列出并列的那几家——冲突列表要答的正是「哪几家并列」，人工裁决从这里起步；
 * 出局与落选的不进这一列，逐候选明细在展开面板里。
 */
export function decisionRowsOf(body: TiedChannelSelectionDecisionsResponseBody): DecisionRow[] {
  return body.decisions.map((decision) => ({
    key: `decision:${decision.decisionId}`,
    decisionId: decision.decisionId,
    values: {
      decisionId: decision.decisionId,
      scope: decision.scope,
      mapping: decision.mapping,
      decidedAt: formatInstant(decision.decidedAt),
      assembledAsOf: formatInstant(decision.assembledAsOf),
      rule: wordOf(ruleLabels, decision.rule),
      conclusion: wordOf(conclusionLabels, decision.conclusion),
      candidateCount: String(decision.candidates.length),
      tiedCandidates: decision.candidates
        .filter((candidate) => candidate.outcome === 'TIED')
        .map((candidate) => candidate.candidate)
        .join('、'),
    },
  }));
}

export interface CandidateRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

/**
 * 逐候选明细一行。评价引用缺席显 `—`（没登记价卡的候选没经过评价，不是漏了）；出局因由只在出局行上有字，
 * 其余行留空——非出局不得带因由，是决定记录自己的形状。
 */
export function candidateRowsOf(decision: ChannelSelectionDecisionRecord): CandidateRow[] {
  return decision.candidates.map((candidate) => ({
    key: `candidate:${decision.decisionId}#${candidate.candidate}`,
    values: {
      candidate: candidate.candidate,
      outcome: wordOf(candidateOutcomeLabels, candidate.outcome),
      evaluation: candidate.evaluation ?? '—',
      exclusion: candidate.exclusion ? wordOf(exclusionLabels, candidate.exclusion) : '',
    },
  }));
}

interface TiedListViewOptions {
  module: ModuleInfo;
  endpoint: string;
}

/**
 * 列表区的呈现态。HTTP 那半（未配置 / 调用方问题 / 未形成答案 / 传输失败）转交 catalogueViewState，不在
 * 这里抄第二份；本函数只管空册措辞：收窄与不收窄各一句。
 *
 * 空册是真答案不是缺陷：没有并列冲突说的是「此刻没有等人工裁决的择优」——与 403 的「接入渠道未配置」
 * 是两件事，也与「择优编排今天没有生产装配点」是两件事（后者让册子在接线前恒空，页面在描述里说明）。
 */
export function tiedListViewState(
  answer: ApiResult<TiedChannelSelectionDecisionsResponseBody> | null,
  subject: ChannelSelectionSubjectFilter | undefined,
  rowCount: number,
  retry: () => void,
  options: TiedListViewOptions,
): TemplateViewState {
  const empty = subject
    ? {
        title: `对象 ${subject.scope} / ${subject.mapping} 此刻没有并列冲突`,
        description:
          '读取入口已配置，这一格是真答案：该商业范围下按这笔产品—渠道映射做过的择优里，没有停在并列冲突的记录。去掉对象收窄可看租户内全部并列冲突。',
      }
    : {
        title: '当前租户此刻没有并列冲突',
        description:
          '读取入口已配置，这一格是真答案：没有择优停在「并列且无法选出唯一一条」等人工裁决。择优编排今天没有生产装配点，接线之前本册恒空是设计而不是缺陷。',
      };
  return catalogueViewState(answer, rowCount, retry, {
    module: options.module,
    endpoint: options.endpoint,
    emptyTitle: empty.title,
    emptyDescription: empty.description,
  });
}

/**
 * 单份查阅的呈现态。404 CHANNEL_SELECTION_DECISION_NOT_VISIBLE 是终局业务答案（`统一不可见结果`：不存在与
 * 属别的租户同答），自成一格、不当错误重试；其余 4xx 与 5xx 各归调用方问题与未形成答案。
 */
export type DecisionDetailState =
  | { kind: 'idle' }
  | { kind: 'loading' }
  | { kind: 'decision'; decision: ChannelSelectionDecisionRecord }
  | { kind: 'notVisible'; decisionId: string }
  | { kind: 'unconfigured' }
  | { kind: 'error'; message: string; canRetry: boolean };

export function decisionDetailState(
  answer: ApiResult<ChannelSelectionDecisionResponseBody> | null,
  decisionId: string,
): DecisionDetailState {
  if (!answer) return { kind: 'loading' };
  switch (answer.kind) {
    case 'outcome':
      return { kind: 'decision', decision: answer.body.decision };
    case 'unconfigured':
      return { kind: 'unconfigured' };
    case 'callerProblem':
      if (answer.status === 404 && answer.code === 'CHANNEL_SELECTION_DECISION_NOT_VISIBLE') {
        return { kind: 'notVisible', decisionId };
      }
      return {
        kind: 'error',
        message: `调用方式问题（HTTP ${answer.status}，${answer.code}）；重发同样内容不会改变结果。`,
        canRetry: false,
      };
    case 'noAnswer':
      return {
        kind: 'error',
        message: `服务端未形成答案（HTTP ${answer.status}，${answer.code}），可稍后重试。`,
        canRetry: true,
      };
    case 'transport':
      return { kind: 'error', message: `请求未到达 parcel-api：${answer.message}`, canRetry: true };
  }
}
