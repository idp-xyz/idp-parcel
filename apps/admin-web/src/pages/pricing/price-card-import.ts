// 价卡导入签与草稿签的词表、排法与判读（票 price-card-import/05、06）。词取传输层 outcome 与草稿状态原名，
// 不自造译法；逐格问题按表、行、列排，才能对回模板里的那一格。纯函数，node:test 钉着；两签的组件只负责摆。

import { operatorRefusalSentence, type OperatorRefusalKind } from '../../auth/access-refusal';
import type { ModuleInfo } from '../../navigation';
import type { TemplateViewState } from '../../templates/state-slot';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState } from '../catalogue-view';
import { httpRows, type TechnicalRow } from '../../components/TechnicalDetails';
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
  VALIDATED: '检查通过，可以存为草稿',
  HAS_PROBLEMS: '有问题：按下表逐格改好，再重新上传',
  NOT_ACCEPTED: '读不出这是哪张价卡（方案编号或版本没填），请用模板重新填写',
};

export const priceCardDraftOutcomeLabels: Record<string, string> = {
  DRAFT_SUBMITTED: '已存为草稿，到「草稿与审批」签里批准、发布',
  DRAFT_REPLAYED: '同一版、同样的内容早已存过，这次没有变化',
  DRAFT_REVISED: '这一版还没批准，已换成这次上传的内容',
  CONTENT_FIXED: '这一版已经批准，内容不能再改；要改请换一个新版本号',
  NOT_ACCEPTED: '没能存为草稿，请先按预览里的问题改好',
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
  DRAFT_APPROVED: '已批准，可以发布了',
  NOT_CONFIGURED: '审批规则未配置：贵公司还没设置价卡由谁批准，暂时不能批准；请管理员先设置审批规则',
  NEEDS_ANOTHER_APPROVER: '录入人不能自己批准：按贵公司的审批规则，请换一位同事批准',
  APPROVER_NOT_QUALIFIED: '没有批准权限：请让有价卡审批权限的同事批准',
  DRAFT_NOT_VALIDATED: '这一版检查没通过，不能批准；改好后重新导入',
  DRAFT_ALREADY_APPROVED: '这一版已经批准过了',
  DRAFT_ALREADY_PUBLISHED: '这一版已经发布了',
  DRAFT_CHANGED: '批准时这一版刚被人重新导入替换，这次没有批准；刷新后再看',
  DRAFT_NOT_FOUND: '找不到这一版草稿，刷新后再看',
};

/** 发布口的答案代数（`application.PublishPriceCardDraftOutcome` 原名）。登记那一格另按登记答案代数呈现。 */
export const priceCardPublicationOutcomeLabels: Record<string, string> = {
  DRAFT_PUBLISHED: '已发布，「全部价卡」里可以看到了',
  PUBLICATION_NOT_LANDED: '没有发布成功，草稿仍是已批准',
  DRAFT_NOT_APPROVED: '这一版还没批准，不能发布',
  DRAFT_ALREADY_PUBLISHED: '这一版已经发布了',
  DRAFT_NOT_FOUND: '找不到这一版草稿，刷新后再看',
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
  | { tone: 'answered'; text: string; technical: TechnicalRow[] }
  | { tone: 'unconfigured'; text: string }
  | { tone: 'refused'; kind: OperatorRefusalKind; text: string }
  | { tone: 'problem'; text: string; technical: TechnicalRow[] };

/**
 * 答复逐格如实呈现：正文只说中文，发布交给了登记时另接登记那一格的中文；状态码与 outcome 原名进技术详情（改版第一步：
 * 英文枚举不上屏）。未收录的原名原样过线，不归进既有说法。渠道那一格与调用方问题、未形成答案、传输失败各说各的，
 * 不折成一句「失败」。
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
      const head = `${verb}：${labelOf(labels, outcome)}`;
      return {
        tone: 'answered',
        text: registration ? `${head}；登记：${labelOf(registrationOutcomeLabels, registration)}` : head,
        technical: [
          { label: 'HTTP 状态', value: String(answer.status) },
          { label: '结果码', value: outcome },
          ...(registration ? [{ label: '登记结果码', value: registration }] : []),
        ],
      };
    }
    case 'unconfigured':
      return {
        tone: 'unconfigured',
        // 渠道未配置是诚实答案（ADR-0055）：部署没设发行方参数时如实拒绝，重试不会变。
        text: `${verb}：访问通道尚未配置（403），这次没有${verb}；重试也不会变，请管理员先配置访问通道。`,
      };
    case 'loginRequired':
    case 'notGranted':
      return { tone: 'refused', kind: answer.kind, text: operatorRefusalSentence(answer.kind, verb) };
    case 'callerProblem':
      return {
        tone: 'problem',
        text: `没能${verb}：${problemNote(answer.code)}${answer.detail ? ` 原因：${answer.detail}` : ''}`,
        technical: httpRows(answer.status, answer.code),
      };
    case 'noAnswer':
      return {
        tone: 'problem',
        text: `这次${verb}没处理完${action === 'publish' ? '，发没发布成功还不确定' : ''}：${problemNote(answer.code)}`,
        technical: httpRows(answer.status, answer.code),
      };
    case 'transport':
      return { tone: 'problem', text: `连不上系统，没能${verb}（${answer.message}）；检查网络后重试。`, technical: [] };
  }
}

/**
 * 没形成答案时按码换成草稿册自己的说法：目录页那套说明有的点名「目录」，而草稿不进价卡目录，照搬会让人去查一个
 * 没出问题的读口。没收录的码照目录页原样。
 */
const draftRegisterNoAnswerNotes: Record<string, string> = {
  NO_ANSWER_FORMED: '草稿读不出来，这不代表没有草稿；请稍后重试。',
  INTAKE_FAILED: '系统这次没能处理请求，没有读草稿；请稍后重试。',
};

/**
 * 草稿册的页面态：空册、就绪、渠道各格与故障照目录页同一套判读（catalogueViewState），只把说明里讲成目录读口的
 * 换成草稿册自己的——未配置那一格它挂的是操作者渠道，答 403 不等于「册上没有草稿」；没形成答案的说明见
 * draftRegisterNoAnswerNotes。
 */
export function draftListViewState(
  answer: ApiResult<PriceCardDraftListResponseBody> | null,
  retry: () => void,
  module: ModuleInfo,
): TemplateViewState {
  const state = catalogueViewState(answer, answer?.kind === 'outcome' ? answer.body.drafts.length : 0, retry, {
    module,
    endpoint: `GET ${priceCardDraftViewsPath}`,
    emptyTitle: '还没有价卡草稿',
    emptyDescription: '在「新建价卡」或「导入价卡」签里存为草稿后，会出现在这里等批准与发布。',
  });
  if (answer?.kind === 'noAnswer' && state.kind === 'error') {
    const description = draftRegisterNoAnswerNotes[answer.code];
    return description ? { ...state, description } : state;
  }
  if (state.kind !== 'unconfigured') return state;
  return {
    kind: 'unconfigured',
    title: '访问通道尚未配置',
    description: '草稿暂时读不到。这不代表没有草稿；页面不会用演示数据代替。',
    facts: {
      owner: module.owner,
      source: module.source,
      endpoint: `GET ${priceCardDraftViewsPath}`,
      unlock: '请管理员配置访问通道；配置后导入、批准、发布一起可用。',
    },
  };
}
