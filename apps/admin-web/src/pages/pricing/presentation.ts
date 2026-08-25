// 计价目录查阅的结果词表。词取 parcel-pricing CONTEXT 与传输层原词,不自造译法。

export const directionLabels: Record<string, string> = {
  BUY: '买价',
  SELL: '卖价',
  INTERNAL: '内部',
};

export const purposeLabels: Record<string, string> = {
  SUPPLIER_COST: '供应商成本',
  CUSTOMER_CHARGE: '客户计费',
  INTERNAL_TRANSFER: '内部划转',
};

export const seriesKindLabels: Record<string, string> = {
  FUEL_RATE: '燃油费率',
  EXCHANGE_RATE: '汇率',
};

export const evidenceGradeLabels: Record<string, string> = {
  VERIFIABLE: '可复核',
  ASSERTION: '断言强度',
};

export const problemCodeNotes: Record<string, string> = {
  METHOD_NOT_ALLOWED: '请求方法不被该端点允许。这是调用方式问题,不是业务答案。',
  MALFORMED_REQUEST: '请求构造不出查询,重发同样的内容不会改变结果。',
  INTAKE_FAILED: '接入解析未能完成,本次没有形成任何业务答案,可稍后重试。',
  NO_ANSWER_FORMED: '服务端处理未能完成,本次没有形成任何业务答案,可稍后重试。',
};

export function problemNote(code: string): string {
  return problemCodeNotes[code] ?? '未知错误码。请携带关联标识查询服务端记录。';
}

export function labelOf(table: Record<string, string>, code: string): string {
  return table[code] ?? code;
}
