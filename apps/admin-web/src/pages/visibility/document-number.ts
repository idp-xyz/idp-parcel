// 异常案件与客户索赔项的单据号在管理台上怎么显示（ADR-0355 决定十一；票 document-numbering/06）。纯函数、零依赖，node:test 直接跑。
//
// 有号的单据以号为主列、可按号检索，内部标识进技术详情；没有号的单据照旧显示内部标识，技术详情按单据上记下的无号原因
// 说是哪一种——读口两格（documentNumber / numberlessReason）都照实给，这里不从「没有号」反推原因，也不把原因一律说成
// 「上线前开立」：漏登单据编号时区的租户看到那一句，会以为是老单据而不去补登。

/** 读口答的无号原因（封闭两值，见 internal/visibilityexception/domain 的 NumberlessReason）。 */
export type NumberlessReason = 'ISSUED_BEFORE_NUMBERING' | 'NUMBERING_TIME_ZONE_NOT_CONFIGURED';

/** 技术详情里那一句：没有单据号时说为什么没有。读口没给原因（不该发生）就只说没有号，不替它猜。 */
export function numberlessNote(reason: string | undefined): string {
  switch (reason) {
    case 'ISSUED_BEFORE_NUMBERING':
      return '上线前开立，无单据号';
    case 'NUMBERING_TIME_ZONE_NOT_CONFIGURED':
      return '开立时单据编号时区未配置，无单据号';
    default:
      return '无单据号';
  }
}

/** 主列上那一格：有号显号；没号照旧显内部标识，numbered 为 false 时页面把它显成次要样式、悬停说明为什么没有号。 */
export function primaryReference(
  documentNumber: string | undefined,
  internal: string,
  reason: string | undefined,
): { text: string; numbered: boolean; hint?: string } {
  if (documentNumber) return { text: documentNumber, numbered: true };
  return { text: internal, numbered: false, hint: numberlessNote(reason) };
}
