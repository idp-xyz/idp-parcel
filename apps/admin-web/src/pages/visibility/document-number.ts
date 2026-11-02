// 异常案件与客户索赔项的单据号在管理台上怎么显示（ADR-0355 决定十一；票 document-numbering/06）。纯函数，node:test 直接跑。
//
// 有号的单据以号为主列、可按号检索，内部标识进技术详情；没有号的单据照旧显示内部标识，技术详情按读口答的无号原因说是
// 哪一种。号叫什么、无号原因怎么说都取共用词典 vocabulary/document-numbering，这里只管「有号 / 无号」这一个分支，
// 本区各页（异常案件、处置请求、索赔与追偿）的单据号格都经它与 DocumentNumberCell，不各写一遍。

import { documentNumberAbsenceNote, documentTypeWords } from '../../vocabulary/document-numbering';

/** 本区两类单据的号名，取共用词典。 */
export const caseNumberName = documentTypeWords.EXCEPTION_CASE.numberName;
export const claimNumberName = documentTypeWords.CLAIM.numberName;

/**
 * 没有号时那一句。读口答了无号原因就按词典说；处置请求册、追偿册只带所属案件的号、不带案件的无号原因，那时只说没有号，
 * 原因到异常案件页看——不替读口猜是哪一种。
 */
export function absenceText(absence: string | undefined): string {
  return absence ? documentNumberAbsenceNote(absence) : '无单据号';
}

/** 主列上那一格：有号显号；没号照旧显内部标识，numbered 为 false 时页面显成次要样式、悬停说明为什么没有号。 */
export function primaryReference(
  documentNumber: string | undefined,
  internal: string,
  absence: string | undefined,
): { text: string; numbered: boolean; hint?: string } {
  if (documentNumber) return { text: documentNumber, numbered: true };
  return { text: internal, numbered: false, hint: absenceText(absence) };
}
