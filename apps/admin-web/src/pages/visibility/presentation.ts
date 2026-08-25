// 运营追踪查阅的结果词表。词取 CONTEXT.md 与传输层的原词,不自造译法。
//
// 刻意没有的两张表:事实类型(kind)与标准里程碑都是原词直示——kind 是源上下文
// 拥有的开放词表,里程碑目录及其映射属版本化登记的实例参数(CONTEXT「追踪投影与
// 里程碑」),本页翻译任何一个都是替租户造第二套口径。

/**
 * 源上下文的中文词,取 CONTEXT「全程追踪投影」定义句自己数的五源:
 * 「对节点、运输、关务、路由和托运上下文已经接受的事实进行语义化编排」。
 * 词表没收录的源(枚举扩了、页面还没跟上)由调用方原样示码,不猜词。
 */
export const sourceContextLabels: Record<string, string> = {
  PARCEL_SHIPMENT: '托运',
  NETWORK_ROUTING: '路由',
  NODE_OPERATIONS: '节点',
  TRANSPORT_FULFILLMENT: '运输',
  CUSTOMS_COMPLIANCE: '关务',
};

// 传输层错误码说明。它们不是业务原因目录:出现即表示应用层没答过,响应体也
// 刻意不带自由文本(防泄露),所以措辞只指下一步动作。
export const problemCodeNotes: Record<string, string> = {
  METHOD_NOT_ALLOWED: '请求方法不被该端点允许。这是调用方式问题,不是业务答案。',
  MALFORMED_REQUEST:
    '请求构造不出查询(定位参数为空,或 parcel 与 version 同时在场),重发同样的内容不会改变结果。',
  INTAKE_FAILED: '接入解析未能完成,本次没有形成任何业务答案,可稍后重试。',
  NO_ANSWER_FORMED: '服务端处理未能完成,本次没有形成任何业务答案,可稍后重试。',
};

export function problemNote(code: string): string {
  return problemCodeNotes[code] ?? '未知错误码。请携带关联标识查询服务端记录。';
}
