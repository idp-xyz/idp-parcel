// 路由计划页「并列」一册与并列裁决（ADR-0309 决定二、六；票 docs-consistency/code/97）：哪一件包裹此刻能裁、能在哪几家里择、怎样拼
// 载荷、怎样读答复。
//
// 单独成模块而不写在页面里：最容易错的几格是纯函数——已决定与另行形成的并列不该再给裁决按钮、只能在并列的那几家里择、哪几格答复说的是
// 没形成计划而不是失败。页面组件要 React 与 DOM，判读拿 node:test 就能钉住。

import { formatInstant } from '../catalogue-view';
import type {
  InitialRouteTieDecisionRequest,
  InitialRouteTieDecisionResponseBody,
  InitialRouteTieRecord,
} from './api';
import { labelOf } from './presentation';

/** 并列状态三格（ADR-0309 决定六）。读面只答此刻的大概，成不成立仍由裁决口判。 */
export const initialRouteTieStateLabels: Record<string, string> = {
  PENDING: '待裁决',
  DECIDED: '已裁决',
  FORMED_OTHERWISE: '已另行形成初始路由——并列之后重判选出了唯一一条',
};

/** 裁决口结果代数（application DecideInitialRouteOutcome）。 */
export const initialRouteTieOutcomeLabels: Record<string, string> = {
  INITIAL_ROUTE_DECIDED: '首个计划已形成——从决定的时刻起生效',
  INITIAL_ROUTE_ALREADY_FORMED: '这件包裹已有初始路由——未再形成',
  NO_TIE_REGISTERED: '这件包裹最近一次停住的不是最低成本并列——未形成计划',
  CANDIDATE_NOT_TIED: '所择候选不在并列的那几家里——未形成计划',
  CANDIDATE_RESTRICTED: '所择候选此刻被有效硬限制淘汰——只有限制的来源责任方解除得了，未形成计划',
  CANDIDATE_NOT_QUALIFIED: '所择候选此刻已不合格——另择一家，未形成计划',
  CANDIDATE_EVIDENCE_UNKNOWN: '所择候选此刻证据未知——等登记补齐，未形成计划',
  INITIAL_ROUTE_EVIDENCE_NOT_CONFIGURED: '判断时点没有适用的网络证据——等租户登记，未形成计划',
  UNDECIDED: '依赖暂时读不动——未形成计划，可稍后重试',
};

/** 未决原因（application DecideInitialRouteUndecidedReason）。 */
export const initialRouteTieUndecidedReasonLabels: Record<string, string> = {
  JUDGMENT_LOCK_UNAVAILABLE: '判断键锁取不到',
  NOT_FORMED_REGISTER_UNAVAILABLE: '未形成登记读不回',
  ROUTE_STORE_UNAVAILABLE: '判断库读写不了',
  ACCEPTED_PARCEL_UNREADABLE: '小包托运此刻读不动（受理基线的地理投影）',
  ROUTE_EVIDENCE_UNAVAILABLE: '网络证据读不动',
  ROUTE_COST_SOURCE_UNAVAILABLE: '候选成本读不回',
  ROUTE_IDENTITY_UNAVAILABLE: '计划版本取号故障',
};

const problemNotes: Record<string, string> = {
  MALFORMED_REQUEST: '载荷构造不出裁决（缺格、空白或带了身份键），重发同样的内容不会改变结果。',
  OUTSIDE_ADMISSION_SCOPE: '这笔生产写不在治理登记的生产权威区间内——要先登准入对照与区间。',
  IDENTITY_DEPENDENCY_UNAVAILABLE: '身份依赖（发行方公钥或操作者册）暂时读不动，可稍后重试。',
  INTAKE_FAILED: '接入解析未能完成，本次没有形成任何业务答案，可稍后重试。',
  NO_ANSWER_FORMED: '服务端处理未能完成，本次没有形成任何业务答案，可稍后重试。',
  UNNAMED_OUTCOME: '服务端交回了没有名字的结果，这是实现缺陷，请携带时间查询服务端记录。',
  METHOD_NOT_ALLOWED: '请求方法不被该端点允许。这是调用方式问题，不是业务答案。',
};

export function initialRouteTieProblemNote(code: string): string {
  return problemNotes[code] ?? `未知错误码 ${code}。请携带时间查询服务端记录。`;
}

/** 并列的那几家：授权角色只能在它们之间择（ADR-0309 决定二「所择不在并列里」）。 */
export function decidableTiedCandidatesOf(tie: InitialRouteTieRecord): string[] {
  return tie.tied.map((cost) => cost.candidate);
}

/** 打平的金额，一句话：每一家同一个数。 */
export function tiedAmountText(tie: InitialRouteTieRecord): string {
  return tie.tied.map((cost) => `${cost.candidate} ${(cost.amountMinor / 100).toFixed(2)} ${cost.currency}`).join('；');
}

/** 「裁决」动作此刻能不能发起；能时带可择的候选，不能时带一句为什么。 */
export type InitialRouteTieAvailability =
  | { kind: 'available'; candidates: string[] }
  | { kind: 'unavailable'; reason: string };

export function initialRouteTieAvailabilityOf(tie: InitialRouteTieRecord): InitialRouteTieAvailability {
  switch (tie.state) {
    case 'DECIDED':
      return {
        kind: 'unavailable',
        reason: tie.decision ? `已裁决：${tie.decision.candidate}（${tie.decision.decidedBy}）` : '已裁决',
      };
    case 'FORMED_OTHERWISE':
      return { kind: 'unavailable', reason: '判断库已有这件包裹的初始路由，不再据并列裁决' };
    case 'PENDING': {
      const candidates = decidableTiedCandidatesOf(tie);
      return candidates.length > 0
        ? { kind: 'available', candidates }
        : { kind: 'unavailable', reason: '并列行里没有并列的候选' };
    }
    default:
      return { kind: 'unavailable', reason: `认不出的状态 ${tie.state}` };
  }
}

export interface InitialRouteTieDraft {
  candidate: string;
  basis: string;
}

export type InitialRouteTieRequestBuild =
  | { kind: 'ready'; request: InitialRouteTieDecisionRequest }
  | { kind: 'incomplete'; reason: string };

/** 拼载荷：候选必须是并列的一家，依据必填。包裹取自那一行，不让人手填。 */
export function initialRouteTieRequestOf(tie: InitialRouteTieRecord, draft: InitialRouteTieDraft): InitialRouteTieRequestBuild {
  if (!decidableTiedCandidatesOf(tie).includes(draft.candidate)) {
    return { kind: 'incomplete', reason: '先在并列的那几家里择一家' };
  }
  const basis = draft.basis.trim();
  if (basis === '') {
    return { kind: 'incomplete', reason: '决定依据引用必填——人工裁决要留下凭什么' };
  }
  return { kind: 'ready', request: { declaredParcelId: tie.declaredParcelId, candidate: draft.candidate, basis } };
}

export interface InitialRouteTieAnswerNote {
  /** 这一次（或先前）是否已有初始路由在册：页面据此刷新列表。 */
  landed: boolean;
  tone: 'answered' | 'refused';
  headline: string;
  details: string[];
}

/** 读裁决口的业务答案。成立带计划；已有结果带它是计划还是无路由；拒的几格带依据；未决带原因与续办引用。 */
export function describeInitialRouteTieAnswer(body: InitialRouteTieDecisionResponseBody): InitialRouteTieAnswerNote {
  const details: string[] = [];
  if (body.plan) {
    details.push(`所择候选 ${body.plan.candidate}，计划 ${body.plan.version}，${formatInstant(body.plan.effectiveFrom)} 起生效`);
  }
  if (body.existing) {
    details.push(
      body.existing.conclusion === 'ROUTE_FORMED'
        ? `已有计划 ${body.existing.planVersion ?? ''}`
        : '已有的是无当前有效路由判断',
    );
  }
  if (body.registeredReason) {
    details.push(`登记最近一次停在：${body.registeredReason}`);
  }
  if (body.candidateReason) {
    details.push(`依据：${body.candidateReason}`);
  }
  if (body.undecidedReason) {
    details.push(`原因：${labelOf(initialRouteTieUndecidedReasonLabels, body.undecidedReason)}`);
  }
  if (body.continuationReference) {
    details.push(`续办引用 ${body.continuationReference}`);
  }
  const landed = body.outcome === 'INITIAL_ROUTE_DECIDED' || body.outcome === 'INITIAL_ROUTE_ALREADY_FORMED';
  return {
    landed,
    tone: body.outcome === 'INITIAL_ROUTE_DECIDED' ? 'answered' : 'refused',
    headline: labelOf(initialRouteTieOutcomeLabels, body.outcome),
    details,
  };
}
