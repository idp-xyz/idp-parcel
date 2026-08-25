// 网络目录查阅词表。族名与登记口 kind 同词;中文取 network-routing CONTEXT 原词。

import type { NetworkCatalogFamily } from './api';

export const familyLabels: Record<NetworkCatalogFamily, string> = {
  node: '物流节点',
  connection: '网络连接',
  line: '线路',
  'service-area': '服务区域',
  'service-calendar': '服务日历',
  'availability-adjustment': '网络可用性调整',
  'route-strategy': '路由策略版本',
};

/** 网络目录页 chip:六族,不露服务区域(专页承担)。 */
export const networkCatalogFamilies: NetworkCatalogFamily[] = [
  'node',
  'connection',
  'line',
  'service-calendar',
  'availability-adjustment',
  'route-strategy',
];

// 日历与可用性调整的适用对象类别,封闭三类(ports 的 CatalogTargetKind);中文与
// familyLabels 同词,同一个对象不因出现在不同列而换名。
export const targetKindLabels: Record<string, string> = {
  NODE: '物流节点',
  CONNECTION: '网络连接',
  LINE: '线路',
};

// 调整种类封闭四格,中文取 CONTEXT 原词(临时停运、关闭、恢复、适用范围调整)。
export const adjustmentKindLabels: Record<string, string> = {
  SUSPENSION: '临时停运',
  CLOSURE: '关闭',
  RESUMPTION: '恢复',
  SCOPE_ADJUSTMENT: '适用范围调整',
};

export const problemCodeNotes: Record<string, string> = {
  METHOD_NOT_ALLOWED: '请求方法不被该端点允许。这是调用方式问题,不是业务答案。',
  MALFORMED_REQUEST:
    '请求构造不出查询(family 缺席或不在封闭集),重发同样的内容不会改变结果。',
  INTAKE_FAILED: '接入解析未能完成,本次没有形成任何业务答案,可稍后重试。',
  NO_ANSWER_FORMED: '服务端处理未能完成,本次没有形成任何业务答案,可稍后重试。',
};

export function problemNote(code: string): string {
  return problemCodeNotes[code] ?? '未知错误码。请携带关联标识查询服务端记录。';
}

export function labelOf(table: Record<string, string>, code: string): string {
  return table[code] ?? code;
}
