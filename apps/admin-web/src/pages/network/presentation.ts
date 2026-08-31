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

// ——以下为路由判断两册的词表(票 admin-skeleton-closure-batch/03 阶段二)。
// 库面原词见 migrations/network_routing/0002 与 0005 的 CHECK;中文取 CONTEXT 原词。

/** register 查询参数封闭两册,词与查阅口、库表同(端点纪律:同一册同一个词)。 */
export type RoutePlanRegister = 'initial-route' | 'reassessment';

export const routePlanRegisters: RoutePlanRegister[] = ['initial-route', 'reassessment'];

export const routePlanRegisterLabels: Record<RoutePlanRegister, string> = {
  'initial-route': '初始路由判断',
  reassessment: '路由复核',
};

// 初始判断结论封闭两格。「无当前有效路由」是明确判断而非空行(CONTEXT 硬句),
// 词表给它完整的一格,不折进成计划行。
export const initialRouteConclusionLabels: Record<string, string> = {
  ROUTE_FORMED: '已成计划',
  NO_CURRENT_ROUTE: '无当前有效路由',
};

// 计划适用性封闭四态,中文取 CONTEXT 生命周期原词(当前有效/已被替代/已失效/已结束)。
export const applicabilityStateLabels: Record<string, string> = {
  CURRENTLY_EFFECTIVE: '当前有效',
  SUPERSEDED: '已被替代',
  LAPSED: '已失效',
  CONCLUDED: '已结束',
};

// 复核走向封闭四格。
export const reassessmentConclusionLabels: Record<string, string> = {
  STILL_APPLICABLE: '仍适用',
  PLAN_LAPSED: '计划失效',
  FIRST_PLAN_FORMED: '首次成计划',
  REROUTED: '已改路',
};

// 候选评估封闭三态;缺席(NULL)表示本走向不评估,缺席的呈现归页面,词表不设假格。
export const candidateStateLabels: Record<string, string> = {
  CANDIDATES_AVAILABLE: '有可用候选',
  NO_QUALIFIED_CANDIDATES: '无合格候选',
  CANDIDATE_REVIEW_UNDECIDED: '候选评审未决',
};

// 改路判定封闭三态;缺席(NULL)表示没评估过改路。
export const rerouteStateLabels: Record<string, string> = {
  AUTOMATIC_ALLOWED: '允许自动改路',
  SUGGESTION_ONLY: '仅出建议',
  BARRED: '禁止改路',
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
