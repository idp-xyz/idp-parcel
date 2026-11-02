// 路由计划页「改路建议」一册与授权改路决定（ADR-0284 决定一、二、六；票 docs-consistency/code/29）：哪一份建议此刻能决定、能在
// 哪几条候选里择、怎样拼载荷、怎样读答复。
//
// 单独成模块而不写在页面里：最容易错的几格是纯函数——过时与已决定的建议不该再给决定按钮、建议里被淘汰的候选不该给选、哪几格答复
// 说的是没形成决定而不是失败。页面组件要 React 与 DOM，判读拿 node:test 就能钉住。

import { formatInstant } from '../catalogue-view';
import type {
  RerouteDecisionRequest,
  RerouteDecisionResponseBody,
  RerouteSuggestionRecord,
} from './api';
import { labelOf } from './presentation';

/** 建议状态三格（ADR-0284 决定六）。读面只答此刻的大概，成不成立仍由决定口判。 */
export const rerouteSuggestionStateLabels: Record<string, string> = {
  PENDING: '待决定',
  DECIDED: '已决定',
  SUPERSEDED: '已过时——之后又复核过',
};

/** 候选在建议里的结局，中文取 CONTEXT 原词。 */
export const suggestionCandidateOutcomeLabels: Record<string, string> = {
  QUALIFIED: '合格',
  ELIMINATED: '已淘汰',
  EVIDENCE_UNKNOWN: '依据不明',
};

/** 决定口结果代数（application DecideRerouteOutcome）。 */
export const rerouteDecisionOutcomeLabels: Record<string, string> = {
  REROUTE_DECIDED: '改路决定已形成——新计划从决定的时刻起生效',
  SUGGESTION_ALREADY_DECIDED: '这份建议已有决定——未再形成',
  SUGGESTION_NOT_FOUND: '没有这份建议——未形成决定',
  SUGGESTION_SUPERSEDED: '建议已过时：同一包裹已有不早于它的复核——未形成决定',
  CANDIDATE_NOT_SUGGESTED: '所择候选不在建议的合格候选里——未形成决定',
  CANDIDATE_RESTRICTED: '所择候选此刻被有效硬限制淘汰——只有限制的来源责任方解除得了，未形成决定',
  CANDIDATE_NOT_QUALIFIED: '所择候选此刻已不合格——另择一条，未形成决定',
  CANDIDATE_EVIDENCE_UNKNOWN: '所择候选此刻证据未知——等登记补齐，未形成决定',
  REROUTE_EVIDENCE_NOT_CONFIGURED: '判断时点没有适用的网络证据——等补登网络资料，未形成决定',
  UNDECIDED: '依赖暂时读不动——未形成决定，可稍后重试',
};

/** 未决原因（application DecideRerouteUndecidedReason）。 */
export const rerouteUndecidedReasonLabels: Record<string, string> = {
  REASSESSMENT_STORE_UNAVAILABLE: '复核册读不回',
  DECISION_STORE_UNAVAILABLE: '决定册读写不了',
  ROUTING_HISTORY_UNAVAILABLE: '路由历史读不回',
  ACCEPTED_PARCEL_UNREADABLE: '小包托运此刻读不动（受理基线的地理投影）',
  REROUTE_EVIDENCE_UNAVAILABLE: '网络证据读不动',
  REROUTE_COST_SOURCE_UNAVAILABLE: '候选成本读不回',
  REROUTE_IDENTITY_UNAVAILABLE: '计划版本取号故障',
};

const problemNotes: Record<string, string> = {
  MALFORMED_REQUEST: '提交的内容不完整（漏选了路线或没写依据）；补齐后再提交。',
  OUTSIDE_ADMISSION_SCOPE: '当前阶段还没有放开这项决定；什么都没有保存，请联系负责阶段管理的同事。',
  IDENTITY_DEPENDENCY_UNAVAILABLE: '暂时无法核对登录身份，请稍后重试。',
  INTAKE_FAILED: '登录信息没能核对通过，这次什么都没有保存；请稍后重试。',
  NO_ANSWER_FORMED: '系统暂时没能处理，结果还不确定；请稍后刷新查看，必要时重新提交。',
  UNNAMED_OUTCOME: '系统返回了无法识别的结果；请记下时间联系系统管理员。',
  METHOD_NOT_ALLOWED: '这个入口不接受这种提交方式，请刷新页面后重试；仍然不行请联系系统管理员。',
};

export function rerouteDecisionProblemNote(code: string): string {
  return problemNotes[code] ?? `这次没有成功（错误码 ${code}）；请记下错误码与时间联系系统管理员。`;
}

/** 建议里合格的候选：授权角色只能在它们之间择（ADR-0284 决定二「不在建议之列」）。 */
export function decidableCandidatesOf(suggestion: RerouteSuggestionRecord): string[] {
  return suggestion.candidates.filter((candidate) => candidate.outcome === 'QUALIFIED').map((candidate) => candidate.id);
}

/** 「决定」动作此刻能不能发起；能时带可择的候选，不能时带一句为什么。 */
export type RerouteDecisionAvailability =
  | { kind: 'available'; candidates: string[] }
  | { kind: 'unavailable'; reason: string };

export function rerouteDecisionAvailabilityOf(suggestion: RerouteSuggestionRecord): RerouteDecisionAvailability {
  switch (suggestion.state) {
    case 'DECIDED':
      return {
        kind: 'unavailable',
        reason: suggestion.decision
          ? `已决定：${suggestion.decision.candidate}（${suggestion.decision.decidedBy}）`
          : '已决定',
      };
    case 'SUPERSEDED':
      return { kind: 'unavailable', reason: '已被不早于它的复核取代，不再据以决定' };
    case 'PENDING': {
      const candidates = decidableCandidatesOf(suggestion);
      return candidates.length > 0
        ? { kind: 'available', candidates }
        : { kind: 'unavailable', reason: '建议里没有合格候选' };
    }
    default:
      return { kind: 'unavailable', reason: `认不出的状态 ${suggestion.state}` };
  }
}

export interface RerouteDecisionDraft {
  candidate: string;
  basis: string;
}

export type RerouteDecisionRequestBuild =
  | { kind: 'ready'; request: RerouteDecisionRequest }
  | { kind: 'incomplete'; reason: string };

/** 拼载荷：候选必须是建议里合格的一条，依据必填。包裹与建议关联取自那一行，不让人手填。 */
export function rerouteDecisionRequestOf(
  suggestion: RerouteSuggestionRecord,
  draft: RerouteDecisionDraft,
): RerouteDecisionRequestBuild {
  if (!decidableCandidatesOf(suggestion).includes(draft.candidate)) {
    return { kind: 'incomplete', reason: '先在建议的合格候选里择一条' };
  }
  const basis = draft.basis.trim();
  if (basis === '') {
    return { kind: 'incomplete', reason: '决定依据引用必填——人工改路要留下凭什么' };
  }
  return {
    kind: 'ready',
    request: {
      declaredParcelId: suggestion.declaredParcelId,
      suggestion: suggestion.correlationId,
      candidate: draft.candidate,
      basis,
    },
  };
}

export interface RerouteDecisionAnswerNote {
  /** 这一次（或先前）是否已有决定在册：页面据此刷新列表。 */
  landed: boolean;
  tone: 'answered' | 'refused';
  headline: string;
  details: string[];
}

/** 读决定口的业务答案。成立与已决定都带决定；拒的几格带依据；未决带原因与续办引用。 */
export function describeRerouteDecisionAnswer(body: RerouteDecisionResponseBody): RerouteDecisionAnswerNote {
  const headline = labelOf(rerouteDecisionOutcomeLabels, body.outcome);
  const details: string[] = [];
  if (body.decision) {
    const decision = body.decision;
    details.push(`所择候选 ${decision.candidate}，新计划 ${decision.newPlanVersion}（替代 ${decision.originalPlan}）`);
    details.push(`自 ${decision.effectiveFromNode} 起、${formatInstant(decision.effectiveFrom)} 生效`);
    details.push(`决定方 ${decision.decidedBy}，依据 ${decision.basis}`);
  }
  if (body.candidateReason) {
    details.push(`依据：${body.candidateReason}`);
  }
  if (body.undecidedReason) {
    details.push(`原因：${labelOf(rerouteUndecidedReasonLabels, body.undecidedReason)}`);
  }
  if (body.continuationReference) {
    details.push(`续办引用 ${body.continuationReference}`);
  }
  const landed = body.outcome === 'REROUTE_DECIDED' || body.outcome === 'SUGGESTION_ALREADY_DECIDED';
  return { landed, tone: body.outcome === 'REROUTE_DECIDED' ? 'answered' : 'refused', headline, details };
}
