/**
 * 外部资金事实更正随答复带出的两类后话（ADR-0285）：更正已采用、状态码不变，超核金额与没推进的作用域只在响应体上。
 * 两个字段缺席即没有这件事，不替服务端补一句；也不在这里断言「入账余额已重算」——事实没映射到任何客户应收作用域时无账可算，
 * 有作用域被跳过时那一个没推进，响应体只会点名后者。词与受控 CLI 的「累计超核」同一个。
 */

/** 响应体上一个没推进的客户应收作用域：法人、结算账户、币种。 */
export interface UnreconciledScope {
  legalEntity: string;
  account: string;
  currency: string;
}

export function overAppliedNote(overAppliedMinor: number | undefined): string | undefined {
  if (overAppliedMinor === undefined || !(overAppliedMinor > 0)) return undefined;
  return `累计超核：${overAppliedMinor}（最小货币单位）——更正已采用，可这条事实上未撤销核销的合计多于更正后的金额；撤销哪一笔核销要人判断，本系统不代撤。`;
}

export function unreconciledScopesNote(scopes: UnreconciledScope[] | undefined): string | undefined {
  if (scopes === undefined || scopes.length === 0) return undefined;
  const named = scopes
    .map((scope) => `法人 ${scope.legalEntity}、账户 ${scope.account}、币种 ${scope.currency}`)
    .join('；');
  return `更正已采用，但这些作用域账已不平，入账余额没有跟着新金额推进：${named}。先核对指向它们的核销与调整，账回平后下一次映射、核销、撤销或更正会推进它们。`;
}
