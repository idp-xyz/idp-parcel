// 商业发布五步路径「表单 → 预览摘要 → 存为待批准 → 批准 → 发布」的纯逻辑（票 admin-write-faces/16，
// 前端公共半边；子票 09–17 各册表单共用）。全部是纯函数，node:test 钉着；组件 PublicationDraftFlow.tsx
// 只负责摆。写法照 pricing/series-form.ts：形状、键、分派，没有 React。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 07 硬句）。它只读服务端答复里的 outcome 原词
// 决定流程停在哪一步、下一步按钮叫什么；哪几格不对由预览口的逐格 problems 说，这里只按 JSON 路径归组。
// 放行 / 停留的名字从 Go 的结果代数抄（application/publication_draft.go 四个 Outcome 的 String()），
// Go 那边加一格，这里跟着改这一张表，不在组件里按字符串散拼。

import type { ApiResult } from '../catalogue-api';
import type {
  CommercialPublicationPayload,
  PayloadProblemRecord,
  PublicationDraftPublicationResponseBody,
  PublicationDraftReferencePayload,
  PublicationDraftResponseBody,
  PublicationPreviewResponseBody,
} from './publication-draft-api';

/**
 * 五步。`form` 是起点（还没有对眼前这份载荷放行的预览：未预览、预览被拒、或改动后作废）；其余四步
 * 各以一口的放行答复为「到达」。`reachedStep` 交回的是**对眼前这份载荷已到达的最远一步**，下一步动作
 * 由 `nextAction` 给。
 */
export const flowSteps = ['form', 'preview', 'submit', 'approve', 'publish'] as const;
export type FlowStep = (typeof flowSteps)[number];

export const flowStepLabels: Record<FlowStep, string> = {
  form: '填表',
  preview: '预览摘要',
  submit: '存为待批准',
  approve: '批准',
  publish: '发布',
};

/** 四个动作，与四口一一对应；到达 `publish` 之后没有下一步。 */
export type FlowAction = Exclude<FlowStep, 'form'>;

export const flowActionLabels: Record<FlowAction, string> = {
  preview: '预览',
  submit: '存为待批准',
  approve: '批准',
  publish: '发布',
};

export function nextAction(step: FlowStep): FlowAction | null {
  switch (step) {
    case 'form':
      return 'preview';
    case 'preview':
      return 'submit';
    case 'submit':
      return 'approve';
    case 'approve':
      return 'publish';
    case 'publish':
      return null;
  }
}

/** 一口的答复连同它是对哪一份载荷（键）作出的。键不同的答复不显示、不算数。 */
export interface KeyedAnswer<Body> {
  key: string;
  answer: ApiResult<Body>;
}

export interface PublicationFlowState {
  preview: KeyedAnswer<PublicationPreviewResponseBody> | null;
  submission: KeyedAnswer<PublicationDraftResponseBody> | null;
  approval: KeyedAnswer<PublicationDraftResponseBody> | null;
  publication: KeyedAnswer<PublicationDraftPublicationResponseBody> | null;
}

export function emptyFlow(): PublicationFlowState {
  return { preview: null, submission: null, approval: null, publication: null };
}

// 放行边。每张表只列让流程**前进**的原词；其余一律停在原地，答复原样显给操作者——续办动作各不相同
// （换人 / 登规则 / 重读 / 届期再来），状态机不替人挑。
const previewAdvances: ReadonlySet<string> = new Set(['PREVIEWED']);
const submitAdvances: ReadonlySet<string> = new Set(['DRAFT_SUBMITTED', 'DRAFT_REPLAYED', 'DRAFT_REVISED']);
const approveAdvances: ReadonlySet<string> = new Set(['DRAFT_APPROVED', 'DRAFT_ALREADY_APPROVED']);
// 批准口答`载体已发布`：这一版早已过了批准与发布两格，直接落到发布那一步，不再要人去按一次发布。
const approveReachesPublish: ReadonlySet<string> = new Set(['DRAFT_ALREADY_PUBLISHED']);
const publishAdvances: ReadonlySet<string> = new Set(['DRAFT_PUBLISHED', 'DRAFT_ALREADY_PUBLISHED']);

function outcomeOf<Body extends { outcome: string }>(entry: KeyedAnswer<Body> | null, key: string): string | null {
  const answer = currentAnswer(entry, key);
  return answer !== null && answer.kind === 'outcome' ? answer.body.outcome : null;
}

/** 某一口的答复，只在键相同时交回：键不同的旧摘要留着不显示，免得冒充眼前这一份的。 */
export function currentAnswer<Body>(entry: KeyedAnswer<Body> | null, key: string): ApiResult<Body> | null {
  return entry !== null && entry.key === key ? entry.answer : null;
}

/** 对眼前这份载荷（键）已到达的最远一步。 */
export function reachedStep(flow: PublicationFlowState, key: string): FlowStep {
  const preview = outcomeOf(flow.preview, key);
  if (preview === null || !previewAdvances.has(preview)) return 'form';
  const submission = outcomeOf(flow.submission, key);
  if (submission === null || !submitAdvances.has(submission)) return 'preview';
  const approval = outcomeOf(flow.approval, key);
  if (approval !== null && approveReachesPublish.has(approval)) return 'publish';
  if (approval === null || !approveAdvances.has(approval)) return 'submit';
  const publication = outcomeOf(flow.publication, key);
  if (publication === null || !publishAdvances.has(publication)) return 'approve';
  return 'publish';
}

/**
 * 载体一旦在册（存为待批准及其后），表单锁定：改任何一格都会让键变、下游整链作废，而册上那份载体
 * 不会跟着变——锁住是让「眼前这份」与「册上那份」在操作者眼里保持同一份。要改内容走「重新开始」，
 * 再录一次，登记册按`待批准期间修订`就地更新。
 */
export function isFormLocked(step: FlowStep): boolean {
  return step === 'submit' || step === 'approve' || step === 'publish';
}

// 每一步的答复覆盖下游：一次新预览意味着操作者重新进入这条路径，上一链的录入 / 批准 / 发布答复说的
// 是另一份载荷（或同一份的上一轮），留着会冒充这一轮的。录入清批准与发布、批准清发布，同理。

export function withPreview(
  _flow: PublicationFlowState,
  key: string,
  answer: ApiResult<PublicationPreviewResponseBody>,
): PublicationFlowState {
  return { preview: { key, answer }, submission: null, approval: null, publication: null };
}

export function withSubmission(
  flow: PublicationFlowState,
  key: string,
  answer: ApiResult<PublicationDraftResponseBody>,
): PublicationFlowState {
  return { ...flow, submission: { key, answer }, approval: null, publication: null };
}

export function withApproval(
  flow: PublicationFlowState,
  key: string,
  answer: ApiResult<PublicationDraftResponseBody>,
): PublicationFlowState {
  return { ...flow, approval: { key, answer }, publication: null };
}

export function withPublication(
  flow: PublicationFlowState,
  key: string,
  answer: ApiResult<PublicationDraftPublicationResponseBody>,
): PublicationFlowState {
  return { ...flow, publication: { key, answer } };
}

/** 批准口与发布口只收载体引用：身份三元，壳上其余各格与正文一概不带。 */
export function draftReferenceOf(payload: CommercialPublicationPayload): PublicationDraftReferencePayload {
  return { kind: payload.kind, objectId: payload.objectId, version: payload.version };
}

/**
 * 载荷的稳定文本，用来判「预览过的是不是眼前这一份」：预览之后再改一格，键就变，下游按钮收回。
 * 它**不是摘要**，只是页面自己比对两份载荷用的键；内容摘要只有服务端算（写法同 pricing/series-form.ts
 * 的 payloadKey）。
 */
export function payloadKey(payload: CommercialPublicationPayload): string {
  return JSON.stringify(payload, Object.keys(flatten(payload)).sort());
}

function flatten(value: unknown, into: Record<string, true> = {}): Record<string, true> {
  if (Array.isArray(value)) {
    value.forEach((item) => flatten(item, into));
  } else if (value !== null && typeof value === 'object') {
    Object.entries(value as Record<string, unknown>).forEach(([key, nested]) => {
      into[key] = true;
      flatten(nested, into);
    });
  }
  return into;
}

/** 逐格问题按 JSON 路径归组，同格多条保序。表单据此把问题挂到对应格旁。 */
export function problemsByField(problems: PayloadProblemRecord[] | undefined): Record<string, string[]> {
  const byField: Record<string, string[]> = {};
  for (const problem of problems ?? []) {
    (byField[problem.field] ??= []).push(problem.problem);
  }
  return byField;
}

/**
 * 表单没认领的路径上的问题。表单只知道自己渲染了哪几格；服务端点名了别的路径（壳上的引用、还没画出来
 * 的格），这些不能静默丢，由流程组件单独列出。
 */
export function unclaimedProblems(
  byField: Record<string, string[]>,
  claimed: readonly string[],
): PayloadProblemRecord[] {
  const claimedSet = new Set(claimed);
  const unclaimed: PayloadProblemRecord[] = [];
  for (const [field, problems] of Object.entries(byField)) {
    if (claimedSet.has(field)) continue;
    for (const problem of problems) unclaimed.push({ field, problem });
  }
  return unclaimed;
}
