// 价卡导入签的词表与排法（票 price-card-import/05）。词取传输层 outcome 与草稿状态原名，
// 不自造译法；逐格问题按表、行、列排，才能对回模板里的那一格。

import type { PriceCardProblem } from './api';

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
