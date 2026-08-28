// 关务目录查阅词表(合规规则库 + 案件配置册 + 门禁条件册 + 口岸/申报路径册)。册子名
// 与查询参数 registry 同词;中文取 customs-compliance CONTEXT。

import type { CaseRegisterRegistry, ComplianceRegistry, PortsPathsRegistry } from './api';

export const registryLabels: Record<ComplianceRegistry, string> = {
  'case-requirement': '案件要求规则',
  interpretation: '解释规则',
};

export const caseRegisterLabels: Record<CaseRegisterRegistry, string> = {
  readiness: '申报就绪判断',
  'submission-authority': '提交授权',
  'closure-obligation': '关闭义务目录',
};

// 口岸/申报路径两册,中文取 CONTEXT 所有权句原词(合规候选口岸、申报路径)。
export const portsPathsRegistryLabels: Record<PortsPathsRegistry, string> = {
  'candidate-port': '合规候选口岸',
  'declaration-path': '申报路径',
};

// 关闭依据项封闭三值的中文取 domain ObligationItemState 的注释原词:已终结、已被
// 有权接收方有效承接(短写「已承接」,承接方另列一栏指名)、未解决。
export const obligationStateLabels: Record<string, string> = {
  CONCLUDED: '已终结',
  HANDED_OVER: '已承接',
  UNRESOLVED: '未解决',
};

// 受门禁约束的方向性动作封闭四值(domain GuardedAction),中文取 CONTEXT 硬句逐词。
// 接收、隔离、测量、查验协作与已授权的处置执行刻意不在此集——「不因此阻止」在领域
// 枚举上就没有格,词表跟着没有,不为它们造一个「不受约束」的假条目。
export const guardedActionLabels: Record<string, string> = {
  OUTBOUND_RELEASE: '出库',
  LOADING_DEPARTURE: '装载出发',
  CROSS_CUSTOMS_MOVEMENT: '跨关务区域移动',
  FINAL_DELIVERY: '交付',
};

// 前置条件认定封闭三值(domain PreconditionState),中文取其注释原词。没有「未知」格,
// 词表也不补一个:集外取值由 labelOf 原样回显,坏数据该露出来,不该被译成一句像样的话。
export const preconditionStateLabels: Record<string, string> = {
  MET: '满足',
  UNMET: '未满足',
  CONFLICTING: '事实冲突',
};

export const directionLabels: Record<string, string> = {
  IMPORT: '进口',
  EXPORT: '出口',
};

// 外部结果六层的中文取 GLOSSARY 与关务用例的领域原词(监管接收、业务受理、
// 监管过程决定、监管核定税费、放行结果、监管处置决定),不自造译法。
export const resultLayerLabels: Record<string, string> = {
  REGULATORY_RECEIPT: '监管接收',
  BUSINESS_ACCEPTANCE: '业务受理',
  PROCESS_DECISION: '监管过程决定',
  ASSESSED_DUTY: '监管核定税费',
  RELEASE_RESULT: '放行结果',
  DISPOSITION_DECISION: '监管处置决定',
};

export function labelOf(table: Record<string, string>, code: string): string {
  return table[code] ?? code;
}

export const problemCodeNotes: Record<string, string> = {
  METHOD_NOT_ALLOWED: '请求方法不被该端点允许。这是调用方式问题,不是业务答案。',
  MALFORMED_REQUEST:
    '请求构造不出查询(registry 缺席或不在封闭集),重发同样的内容不会改变结果。',
  INTAKE_FAILED: '接入解析未能完成,本次没有形成任何业务答案,可稍后重试。',
  NO_ANSWER_FORMED: '服务端处理未能完成,本次没有形成任何业务答案,可稍后重试。',
};

export function problemNote(code: string): string {
  return problemCodeNotes[code] ?? '未知错误码。请携带关联标识查询服务端记录。';
}
