// 价卡导入签与草稿签的词表、排法与判读（票 price-card-import/05、06）。词取传输层 outcome 与草稿状态原名，
// 不自造译法；逐格问题按表、行、列排，才能对回模板里的那一格。纯函数，node:test 钉着；两签的组件只负责摆。

import { operatorRefusalSentence, type OperatorRefusalKind } from '../../auth/access-refusal';
import type { ModuleInfo } from '../../navigation';
import type { TemplateViewState } from '../../templates/state-slot';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState } from '../catalogue-view';
import {
  priceCardDraftViewsPath,
  registrationOutcomeLabels,
  type PriceCardDraftListResponseBody,
  type PriceCardDraftProgressBody,
  type PriceCardDraftReferencePayload,
  type PriceCardDraftRow,
  type PriceCardPlanReference,
  type PriceCardProblem,
} from './api';
import { labelOf, problemNote } from './presentation';

export const priceCardPreviewOutcomeLabels: Record<string, string> = {
  VALIDATED: '已校验（带内容摘要与方案概要；这一步不写草稿册）',
  HAS_PROBLEMS: '有逐格问题（没过构造门）',
  NOT_ACCEPTED: '未受理（连方案身份都读不出来，不落行）',
};

export const priceCardDraftOutcomeLabels: Record<string, string> = {
  DRAFT_SUBMITTED: '已存为草稿（本次写下了那一行）',
  DRAFT_REPLAYED: '同版同内容重放（册上那一行没有被换掉）',
  DRAFT_REVISED: '已批准之前换了内容，替换了那一行',
  CONTENT_FIXED: '已批准之后内容已固定（这次没有改册）',
  NOT_ACCEPTED: '未受理（没有行可落）',
};

export const priceCardDraftStatusLabels: Record<string, string> = {
  DRAFT: '草稿',
  VALIDATED: '已校验',
  APPROVED: '已批准',
  PUBLISHED: '已发布',
};

/**
 * 存草稿要先看过这一份文件的读法。未配置、传输失败、未形成答复都没让人看到读法，按「看过预览」
 * 放行就等于把一份没人读过的文件存进草稿册。
 */
export function draftSubmittable(preview: ApiResult<unknown> | null): boolean {
  return preview?.kind === 'outcome';
}

/** 按表、行、列排。同一格多条问题保持原顺序，方便对回模板里的那一格。 */
export function sortPriceCardProblems(problems: readonly PriceCardProblem[]): PriceCardProblem[] {
  return problems
    .map((problem, index) => ({ problem, index }))
    .sort((left, right) => {
      const sheet = left.problem.sheet.localeCompare(right.problem.sheet);
      if (sheet !== 0) return sheet;
      if (left.problem.row !== right.problem.row) return left.problem.row - right.problem.row;
      const column = left.problem.column.localeCompare(right.problem.column);
      if (column !== 0) return column;
      return left.index - right.index;
    })
    .map((entry) => entry.problem);
}

// ---- 草稿签（票 price-card-import/06）----

/**
 * 批准口的答案代数（`application.ApprovePriceCardDraftOutcome` 原名），逐格中文。负向各格都是答案不是失败：
 * 审批职责规则没登记、主体相同、授予不足各有各的续办，折成一句「批准失败」会让人以为重试有用。
 */
export const priceCardApprovalOutcomeLabels: Record<string, string> = {
  DRAFT_APPROVED: '已批准（记下了批准者与批准时刻，可以发布了）',
  NOT_CONFIGURED: '审批职责规则未配置：本租户还没登记价卡发布审批职责规则，批准不放行，也不默认单人或双人',
  NEEDS_ANOTHER_APPROVER: '主体相同：审批职责规则要求批准者不是录入者，换一位批准者',
  APPROVER_NOT_QUALIFIED: '授予不足：批准者不持审批职责规则要求的那一格授予',
  DRAFT_NOT_VALIDATED: '这一版还是草稿（没过构造门），不能批准；改好模板重新导入',
  DRAFT_ALREADY_APPROVED: '这一版已经批准过，不重批',
  DRAFT_ALREADY_PUBLISHED: '这一版已经发布，不再批准',
  DRAFT_CHANGED: '批准那一刻这一行刚被替换（有人重新导入），这次没有批准；刷新后按册上那一行再看',
  DRAFT_NOT_FOUND: '册上没有这一版草稿',
};

/** 发布口的答案代数（`application.PublishPriceCardDraftOutcome` 原名）。登记那一格另按登记答案代数呈现。 */
export const priceCardPublicationOutcomeLabels: Record<string, string> = {
  DRAFT_PUBLISHED: '已发布（登记落定，草稿转已发布）',
  PUBLICATION_NOT_LANDED: '登记没有落定，草稿留在已批准',
  DRAFT_NOT_APPROVED: '这一版还没批准，不能发布',
  DRAFT_ALREADY_PUBLISHED: '这一版已经发布，不重交登记',
  DRAFT_NOT_FOUND: '册上没有这一版草稿',
};

export type PriceCardDraftAction = 'approve' | 'publish';

export const priceCardDraftActionLabels: Record<PriceCardDraftAction, string> = {
  approve: '批准',
  publish: '发布',
};

/**
 * 动作只在对应状态出现：已校验才能批准，已批准才能发布；草稿与已发布没有动作。别的状态名（服务端新增一格）同样
 * 不给动作——给错一个按钮比少一个按钮代价大，服务端照样会按它自己的那一格拒。
 */
export function draftActionOf(status: string): PriceCardDraftAction | null {
  if (status === 'VALIDATED') return 'approve';
  if (status === 'APPROVED') return 'publish';
  return null;
}

/** 两口的载荷只有草稿引用；租户与批准者在操作者信封里，这里不放任何身份格。 */
export function draftReferencePayload(plan: PriceCardPlanReference): PriceCardDraftReferencePayload {
  return { planId: plan.id, planVersion: plan.version };
}

export function draftKey(plan: PriceCardPlanReference): string {
  return `${plan.id}@${plan.version}`;
}

/**
 * 推进成功的那一口带回推进后的行，就地换上——册上那一行此刻就是它；其余各行不动。没带行的答案（负向各格、发布没落定、
 * 渠道那一格）不改表：表上仍是册上最后读到的样子。
 */
export function withProgressedDraft(
  rows: readonly PriceCardDraftRow[],
  answer: ApiResult<PriceCardDraftProgressBody>,
): PriceCardDraftRow[] {
  if (answer.kind !== 'outcome' || !answer.body.draft) return [...rows];
  const next = answer.body.draft;
  return rows.map((row) => (draftKey(row.plan) === draftKey(next.plan) ? next : row));
}

/** 一次动作的答复。`refused` 多带那一格，页面据它在需要登录时接一颗重新登录。 */
export type DraftActionNote =
  | { tone: 'answered'; text: string }
  | { tone: 'unconfigured'; text: string }
  | { tone: 'refused'; kind: OperatorRefusalKind; text: string }
  | { tone: 'problem'; text: string };

/**
 * 答复逐格如实呈现：业务答案带状态码、outcome 原名与中文，发布交给了登记时另接登记那一格的原名与中文；未收录的原名
 * 原样过线，不归进既有说法。渠道那一格与调用方问题、未形成答案、传输失败各说各的，不折成一句「失败」。
 */
export function draftActionNoteOf(
  action: PriceCardDraftAction,
  answer: ApiResult<PriceCardDraftProgressBody>,
): DraftActionNote {
  const verb = priceCardDraftActionLabels[action];
  switch (answer.kind) {
    case 'outcome': {
      const labels = action === 'approve' ? priceCardApprovalOutcomeLabels : priceCardPublicationOutcomeLabels;
      const { outcome, registration } = answer.body;
      const head = `${verb}（HTTP ${answer.status}）：${outcome} —— ${labelOf(labels, outcome)}`;
      return {
        tone: 'answered',
        text: registration
          ? `${head}；登记：${registration} —— ${labelOf(registrationOutcomeLabels, registration)}`
          : head,
      };
    }
    case 'unconfigured':
      return {
        tone: 'unconfigured',
        text: `${verb}：接入渠道未配置（403 ACCESS_CHANNEL_NOT_CONFIGURED）。这一口挂的是操作者渠道，部署没设发行方参数时如实拒绝；重试不会改变结果。`,
      };
    case 'loginRequired':
    case 'notGranted':
      return { tone: 'refused', kind: answer.kind, text: operatorRefusalSentence(answer.kind, verb) };
    case 'callerProblem':
      return {
        tone: 'problem',
        text: `${verb}被拒（HTTP ${answer.status}）：${problemNote(answer.code)}${answer.detail ? ` 原因：${answer.detail}` : ''}`,
      };
    case 'noAnswer':
      return {
        tone: 'problem',
        text: `${verb}未形成答案（HTTP ${answer.status}）：${problemNote(answer.code)}；${action === 'publish' ? '登记与否未知，' : ''}可稍后重试。`,
      };
    case 'transport':
      return { tone: 'problem', text: `${verb}的请求未到达 parcel-api：${answer.message}` };
  }
}

/**
 * 草稿册的页面态：空册、就绪、渠道各格与故障照目录页同一套判读（catalogueViewState），只把未配置那一格的说明换成
 * 草稿册自己的——它挂的是操作者渠道，不是目录读口；答 403 不等于「册上没有草稿」。
 */
export function draftListViewState(
  answer: ApiResult<PriceCardDraftListResponseBody> | null,
  retry: () => void,
  module: ModuleInfo,
): TemplateViewState {
  const state = catalogueViewState(answer, answer?.kind === 'outcome' ? answer.body.drafts.length : 0, retry, {
    module,
    endpoint: `GET ${priceCardDraftViewsPath}`,
    emptyTitle: '当前租户的价卡草稿册是空的',
    emptyDescription:
      '空册是正常业务答案（PRICE_CARD_DRAFTS_LISTED），不是故障；在「导入价卡」签存为草稿之后，这里即可见。',
  });
  if (state.kind !== 'unconfigured') return state;
  return {
    kind: 'unconfigured',
    title: '接入渠道未配置',
    description:
      `草稿查阅读口（GET ${priceCardDraftViewsPath}）挂的是操作者渠道，部署没设发行方参数时如实答 403 ` +
      'ACCESS_CHANNEL_NOT_CONFIGURED。这不是「册上没有草稿」；页面不拿演示数据顶替。',
    facts: {
      owner: module.owner,
      source: module.source,
      unlock: '部署设好操作者渠道的发行方参数即放行；导入、批准、发布各口同属这一族，同时放行。',
    },
  };
}
