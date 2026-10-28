// 商业发布五步路径「表单 → 预览摘要 → 存为待批准 → 批准 → 发布」的纯逻辑（票 admin-write-faces/16，
// 前端公共半边；子票 09–17 各册表单共用）。全部是纯函数，node:test 钉着；组件 PublicationDraftFlow.tsx
// 只负责摆。写法照 pricing/series-form.ts：形状、键、分派，没有 React。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 07 硬句）。它只读服务端答复里的 outcome 原词
// 决定流程停在哪一步、下一步按钮叫什么；哪几格不对由预览口的逐格 problems 说，这里只按 JSON 路径归组。
// 放行 / 停留的名字从 Go 的结果代数抄（application/publication_draft.go 四个 Outcome 的 String()），
// Go 那边加一格，这里跟着改这一张表，不在组件里按字符串散拼。

import type { ApiResult } from '../catalogue-api';
import { ADDRESS_KEYWORD_PARAM, hashWithQueryValue } from '../../templates/address-query';
import { codedProblemLine } from './field-problem-text';
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
  preview: '预览',
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
  /** 已变更之后那次没放行的重新存为待批准（见 withResubmission）；放行的那次直接成为 submission。 */
  resubmission: KeyedAnswer<PublicationDraftResponseBody> | null;
}

export function emptyFlow(): PublicationFlowState {
  return { preview: null, submission: null, approval: null, publication: null, resubmission: null };
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
/**
 * 表单里有没发布的内容：载荷与表单起头那一份不同，且这份载荷没有走到「发布」。起头那份是空表单或「基于此版本」的预填，
 * 预填本身不算输入，人动过才算。存为待批准与批准都不算落定：待批准稿现在没有读回的列表，离开页面就找不回来。
 */
export function publicationDraftUnsaved(initialKey: string, currentKey: string, step: FlowStep): boolean {
  return currentKey !== initialKey && step !== 'publish';
}

/** 批准与发布区里一个动作的状态：可以按、已做完、还轮不到、绑的摘要已过时（见 changedDecisionOf）。 */
export type DecisionState = 'ready' | 'done' | 'waiting' | 'stale';

/** 批准口或发布口答`已变更`的原词（application 两个 Outcome 的 String()）。 */
const draftChanged = 'DRAFT_CHANGED';

/**
 * 答`已变更`的那一口。批准与发布绑的是存为待批准时服务端答的那对摘要（ADR-0314）；册上这份载体被别的操作者改动之后，再按一次
 * 仍带旧摘要、仍答已变更，所以那颗按钮要收回，改摆续办：去「待批准发布」页读回现在的内容、在那里批或发；批准那一口答的还可以
 * 用表单上这份重新存为待批准（`待批准期间修订`，覆盖别人的改动）拿到新摘要再批——发布那一口答的不行，载体已批准、内容固定，
 * 录入口只会答 `CONTENT_FIXED`。只看同键的答复；发布口的答复比批准口的后到，先看它。
 */
export function changedDecisionOf(flow: PublicationFlowState, key: string): 'approve' | 'publish' | null {
  if (outcomeOf(flow.publication, key) === draftChanged) return 'publish';
  if (outcomeOf(flow.approval, key) === draftChanged) return 'approve';
  return null;
}

/** 「待批准发布」页的地址，检索词预填对象标识：落地就只剩这个对象的稿，不用再找。 */
export function pendingPublicationsHash(objectId: string): string {
  return hashWithQueryValue('#/pending-publications', ADDRESS_KEYWORD_PARAM, objectId);
}

export interface DecisionPanel {
  /** 存为待批准之后才摆出这一区；此前表单还在录入，批准与发布与它无关。 */
  visible: boolean;
  status: string;
  approve: DecisionState;
  publish: DecisionState;
}

/**
 * 批准与发布是两个动作、可能是两个人：批准由租户的审批职责规则决定谁能做，发布又不能撤回。此前一颗按钮随步骤换名，
 * 批准一落发布就顶在同一个位置上，手快的人会一路点到发布；分成各自的按钮，批准做完发布才亮，两件事各是各的。
 */
export function decisionPanelOf(step: FlowStep, changed: 'approve' | 'publish' | null = null): DecisionPanel {
  switch (step) {
    case 'submit':
      return changed === 'approve'
        ? { visible: true, status: '册上这份发布稿已被别的操作者改动，不是你存下的那份；批准已收回。', approve: 'stale', publish: 'waiting' }
        : { visible: true, status: '已存为待批准，等待批准。', approve: 'ready', publish: 'waiting' };
    case 'approve':
      return changed === 'publish'
        ? { visible: true, status: '册上这份发布稿在批准之后又被改动，不是批准时的那份；发布已收回。', approve: 'done', publish: 'stale' }
        : { visible: true, status: '已批准，等待发布。', approve: 'done', publish: 'ready' };
    case 'publish':
      return { visible: true, status: '已发布。结果在对应册立刻可见。', approve: 'done', publish: 'done' };
    case 'form':
    case 'preview':
      return { visible: false, status: '', approve: 'waiting', publish: 'waiting' };
  }
}

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
  return { preview: { key, answer }, submission: null, approval: null, publication: null, resubmission: null };
}

export function withSubmission(
  flow: PublicationFlowState,
  key: string,
  answer: ApiResult<PublicationDraftResponseBody>,
): PublicationFlowState {
  return { ...flow, submission: { key, answer }, approval: null, publication: null, resubmission: null };
}

/**
 * 已变更之后的重新存为待批准。放行了就同 withSubmission：新摘要接上，已变更随批准答复一起清掉。没放行（未受理、403、内容固定）
 * 只记下这口答复给人看，录入与批准各步原样留着——册上那份载体还在，表单仍锁、已变更仍显；若照 withSubmission 覆盖，一次没成的
 * 重录会把流程退回预览、把表单解锁，看着像从没存过。
 */
export function withResubmission(
  flow: PublicationFlowState,
  key: string,
  answer: ApiResult<PublicationDraftResponseBody>,
): PublicationFlowState {
  if (answer.kind === 'outcome' && submitAdvances.has(answer.body.outcome)) return withSubmission(flow, key, answer);
  return { ...flow, resubmission: { key, answer } };
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

/** 批准与发布绑定的那一对摘要：内容摘要盖正文，壳摘要盖适用范围、有效区间与指名引用。 */
export interface DecisionDigests {
  content: string;
  shell: string;
}

/**
 * 批准口与发布口收载体引用：身份三元加操作者看到的那份内容与版本壳的摘要，壳上其余各格与正文一概不带。摘要由调用方从服务端
 * 答复里取来（见 decisionDigestOf），表单不算；服务端拿它们对载体此刻的摘要，任一个不同即答已变更。
 */
export function draftReferenceOf(
  payload: Pick<CommercialPublicationPayload, 'kind' | 'objectId' | 'version'>,
  digests: DecisionDigests,
): PublicationDraftReferencePayload {
  return {
    kind: payload.kind,
    objectId: payload.objectId,
    version: payload.version,
    expectedDigest: digests.content,
    expectedShellDigest: digests.shell,
  };
}

/**
 * 批准与发布绑定的摘要：存为待批准那一口答的、册上这份载体的内容摘要与壳摘要，也就是操作者在页面上存下的那份。两个缺一个
 * （录入答复没有摘要，或还没录入）时是 null，按钮据此收回——不用预览的摘要顶替，那一份是尚未落库的算出值。
 */
export function decisionDigestOf(flow: PublicationFlowState, key: string): DecisionDigests | null {
  const submission = currentAnswer(flow.submission, key);
  if (submission === null || submission.kind !== 'outcome') return null;
  const { contentDigest, shellDigest } = submission.body;
  return contentDigest && shellDigest ? { content: contentDigest, shell: shellDigest } : null;
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

/**
 * 逐格问题按 JSON 路径归组，同格多条保序。表单据此把问题挂到对应格旁。每条问题在这里变成一行：带认得的问题码的按码译好
 * （原话跟在后面），其余就是原话（见 codedProblemLine）。
 */
export function problemsByField(problems: PayloadProblemRecord[] | undefined): Record<string, string[]> {
  const byField: Record<string, string[]> = {};
  for (const problem of problems ?? []) {
    (byField[problem.field] ??= []).push(codedProblemLine(problem));
  }
  return byField;
}

/**
 * 表单没认领的路径上的问题。表单只知道自己渲染了哪几格；服务端点名了别的路径（壳上的引用、还没画出来
 * 的格），这些不能静默丢，由流程组件单独列出。交回记录的 `problem` 是 problemsByField 出的那一行（带码的已按码译好、原话
 * 跟在后面），显示时经 fieldProblemText 拆开。
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
