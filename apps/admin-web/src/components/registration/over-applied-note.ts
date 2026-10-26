/**
 * 外部资金事实更正的超核说明（ADR-0285）：更正已采用、状态码不变，超核金额只在响应体的 `overAppliedMinor` 上。
 * 缺席或不为正即没有超核，不替服务端补一句。词与受控 CLI 的「累计超核」同一个。
 */
export function overAppliedNote(overAppliedMinor: number | undefined): string | undefined {
  if (overAppliedMinor === undefined || !(overAppliedMinor > 0)) return undefined;
  return `累计超核：${overAppliedMinor}（最小货币单位）——更正已采用，可这条事实上未撤销核销的合计多于更正后的金额；入账余额已按新金额重算，撤销哪一笔核销要人判断，本系统不代撤。`;
}
