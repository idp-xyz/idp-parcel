// 读 parcel-api 的错误体。两种形状并存：首方客户族与 PS 客户面命令口写 RFC 9457 问题详情（ADR-0140 决定三），`code` 与
// `detail` 在顶层；操作者族各口仍写 {"error":{"code","detail"}}，迁到问题详情另立后续票。两边都认，各页的 exchange 只经这里读，
// 不各写一份。

type ProblemFields = { code?: unknown; detail?: unknown };

function fieldsOf(payload: unknown): ProblemFields | undefined {
  if (payload === null || typeof payload !== 'object') return undefined;
  const body = payload as ProblemFields & { error?: unknown };
  if (typeof body.code === 'string') return body;
  if (body.error !== null && typeof body.error === 'object') return body.error as ProblemFields;
  return undefined;
}

/** 错误码；认不出形状答 UNKNOWN。 */
export function problemCodeOf(payload: unknown): string {
  const code = fieldsOf(payload)?.code;
  return typeof code === 'string' ? code : 'UNKNOWN';
}

/** 只谈调用方自己载荷的那句散文；服务端没给一句字符串就没有，不冒充。 */
export function problemDetailOf(payload: unknown): string | undefined {
  const detail = fieldsOf(payload)?.detail;
  return typeof detail === 'string' ? detail : undefined;
}
