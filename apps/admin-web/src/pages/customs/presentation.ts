// 关务目录查阅词表(合规规则库 + 案件配置册 + 门禁条件册 + 口岸/申报路径册)。册子名
// 与查询参数 registry 同词;中文取 customs-compliance CONTEXT。

import type {
  CaseRegisterRegistry,
  ComplianceRegistry,
  CustomsRegistrationKind,
  PortsPathsRegistry,
} from './api';

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

// 税费付款协作事项的义务依据封闭二值(domain DutyObligationKind),中文取 CONTEXT「税费付款
// 协作事项」词条原词。刻意没有第三格:「缺少税费结果不能被解释为无需付款」,领域枚举上就
// 没有「没有结果所以不用付」,词表跟着没有。
export const dutyObligationKindLabels: Record<string, string> = {
  ASSESSED_DUTY: '已接受监管核定税费',
  EXPLICITLY_NOT_REQUIRED: '明确无需付款依据',
};

// 税费付款核对三轴各自的封闭集(domain DutyCoverage / DutyDelta / DutyFactValidity),中文取
// 其注释原词。三张表分开、不合成一张「付款状态」词表——合成的那张就是 CONTEXT 明禁的互斥
// 总状态(ADR-0137 决定三)。集外取值由 labelOf 原样回显,坏数据该露出来。
export const dutyCoverageLabels: Record<string, string> = {
  NONE: '无覆盖',
  PARTIAL: '部分覆盖',
  COVERED: '已覆盖',
};

export const dutyDeltaLabels: Record<string, string> = {
  NO_DELTA: '无差额',
  SHORT: '不足',
  EXCESS: '超额',
  PENDING: '待确认',
};

export const dutyFactValidityLabels: Record<string, string> = {
  VALID: '有效',
  INVALIDATED: '失效',
  CONFLICTING: '冲突',
  PENDING: '待确认',
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

// ——以下为登记签的页面口径(ADR-0085,票 admin-write-faces/02 切片 02b)。

/**
 * 五类登记签的标题。册名与本文件上方的查阅词表同词——同一本册不因换到写签而换名;
 * 门禁目录登的是目录本身而不是目录里的条件项,标题因此说「目录」不说「条件」。
 *
 * 只有建案要求规则的标题不带「版本」二字:它没有版本维,是键上的当前判断(换判断走同键
 * 重登,由登记册答冲突)。标题跟着册的形状走,不为整齐划一而给它一个不存在的版本维。
 */
export const registrationTitles: Record<CustomsRegistrationKind, string> = {
  'interpretation-rule': '登记解释规则版本',
  'case-requirement': '登记建案要求规则',
  'gate-catalog': '登记门禁前置条件目录',
  'candidate-port': '登记合规候选口岸版本',
  'declaration-path': '登记申报路径版本',
};

// 登记快照形状的提示句。五类只差子命令一词(与端点路径、CLI 子命令同字),所以由一处
// 拼出:抄五遍会让「不逐字段建表单」这条理由在其中一遍被改动时悄悄分叉。
function snapshotHint(kind: CustomsRegistrationKind, fields: string): string {
  return (
    `登记快照 JSON 的形状与受控登记口 parcel-customs-register ${kind} -input 吃的同一份;` +
    '本页不逐字段建表单,因为「渠道原始载荷 → 登记快照」的翻译属渠道接入契约,随 PAR-INT-01 提供。' +
    fields
  );
}

/**
 * 各类登记快照的形状提示。逐类把键名与封闭集词列出来:未知键一律被译装拒绝(打错的键
 * 静默丢弃会让操作员以为登进去的比实际多),而封闭集里的词打错在族名上看不出来。
 *
 * 三本版本册都不收终点:换版是登记一个更晚生效起点的新版,前版终点随之落定,历史区间
 * 不接受追改(ADR-0070)。这句写进提示是因为读面上「持续有效」那一格最容易被读成「可以
 * 回头补个终点」。**建案要求规则不在这三本之列**,它连生效起点都没有,提示句因此不许
 * 照抄那半句——照抄会让登记方去找一个本册没有的字段。
 */
export const registrationSnapshotHints: Record<CustomsRegistrationKind, string> = {
  'interpretation-rule': snapshotHint(
    'interpretation-rule',
    '键为 tenantId / resultLayer / jurisdictionRef / ruleRef / appliesFrom;' +
      '外部结果层取封闭六词 REGULATORY_RECEIPT / BUSINESS_ACCEPTANCE / PROCESS_DECISION / ' +
      'ASSESSED_DUTY / RELEASE_RESULT / DISPOSITION_DECISION。终点不是输入——换版登新起点。',
  ),
  'case-requirement': snapshotHint(
    'case-requirement',
    '键为 tenantId / jurisdictionRef / direction / procedureRef / required / basis;' +
      '申报方向取封闭两词 IMPORT / EXPORT。' +
      'required 必须显式给出:它是布尔而非可省字段,缺席不会被当成「不要求」而是直接被拒——' +
      '「不要求建案」与「规则没登记」在库上分不开,而两者续办动作相反(前者照常推进,后者等实例参数)。' +
      'basis 对「要求」与「不要求」同样必填,理由同上。' +
      '本册没有版本维,不收生效起点:换判断是同键重登,登记册会答内容冲突而不是接受覆盖。',
  ),
  'gate-catalog': snapshotHint(
    'gate-catalog',
    '键为 tenantId / scopeRef / action / boundaryRef / registeredAt;' +
      '拟执行动作取封闭四词 OUTBOUND_RELEASE / LOADING_DEPARTURE / CROSS_CUSTOMS_MOVEMENT / ' +
      'FINAL_DELIVERY。本口登的是目录在场本身,目录里的逐项认定是另一个命令,不在本签。',
  ),
  'candidate-port': snapshotHint(
    'candidate-port',
    '键为 tenantId / portRef / appliesFrom。终点不是输入——换版登新起点。',
  ),
  'declaration-path': snapshotHint(
    'declaration-path',
    '键为 tenantId / pathRef / portRef / direction / declarationMode / appliesFrom;' +
      '进出口方向取封闭两词 IMPORT / EXPORT,申报模式是引用不是封闭词表(真实模式集属实例半边)。' +
      '终点不是输入——换版登新起点。',
  ),
};
