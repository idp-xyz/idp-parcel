// 运营追踪与目录查阅的结果词表。词取 CONTEXT.md 与传输层的原词,不自造译法。
//
// 刻意没有的几张表:事实类型(kind)与标准里程碑都是原词直示——kind 是源上下文
// 拥有的开放词表,里程碑目录及其映射属版本化登记的实例参数(CONTEXT「追踪投影与
// 里程碑」),本页翻译任何一个都是替租户造第二套口径。同理,分诊规则的信号类型与
// 可信度、通知策略的渠道与义务判据、索赔前置的种类与申请人都是开放引用,原词转写。

import type { VisibilityCatalogueKind, VisibilityRegistrableCatalogueKind } from './catalogue-api';

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
  // 本上下文的查阅口与登记口共用这一格,所以措辞要同时说得通:两侧都表示「这次请求
  // 构造不出命令」,续办动作也同为改请求形状,只是能构造不出的原因各随入口。
  MALFORMED_REQUEST:
    '请求构造不出查询或登记命令(查阅口:定位参数为空、parcel 与 version 同时在场,或 kind 不在封闭集;登记口:快照不是该种类的形状),重发同样的内容不会改变结果。',
  INTAKE_FAILED: '接入解析未能完成,本次没有形成任何业务答案,可稍后重试。',
  NO_ANSWER_FORMED: '服务端处理未能完成,本次没有形成任何业务答案,可稍后重试。',
  // 应用层交回了一个没有名字的枚举:那是服务端缺陷,不是登记方能改的东西。单列出来
  // 而不落进兜底句,是因为兜底句让人去查记录,这两格该做的是报缺陷。
  UNNAMED_OUTCOME: '服务端交回了没有名字的处理结果,属服务端缺陷;重试不会好,请报缺陷。',
  UNNAMED_REFUSAL_REASON:
    '登记被拒但服务端没给出拒绝理由,属服务端缺陷——没有理由就无从知道该改什么;重试不会好,请报缺陷。',
};

export function problemNote(code: string): string {
  return problemCodeNotes[code] ?? '未知错误码。请携带关联标识查询服务端记录。';
}

// ---- 各类目录查阅（/visibility-catalogues,票 admin-web-page-wiring-frontier/02）----

/**
 * 目录种类的页签词。种类命名册子(与登记写口同词根),中文取 CONTEXT 原词。
 * 「异常披露规则」与「披露策略」是相邻两册(0023 / 0012),签词里的「规则」「策略」就是分册
 * 记号,不缩成同一个词。
 */
export const visibilityCatalogueKindLabels: Record<VisibilityCatalogueKind, string> = {
  MILESTONE_MAPPING: '里程碑映射',
  TRIAGE_RULE: '分诊规则',
  NOTIFICATION_POLICY: '通知策略',
  CLAIM_ELIGIBILITY: '索赔资格',
  CLAIM_AUTHORIZATION: '索赔授权',
  DISCLOSURE_POLICY: '披露策略',
  EXCEPTION_DISCLOSURE_RULE: '异常披露规则',
  CONFLICT_SIGNAL_RULE: '冲突信号规则',
};

/**
 * 异常披露规则条目两个布尔列的词(0023 头注原话):disclosable 是「披露条件成不成立」,
 * auto_release 是「批准范围允不允许自动发布」。是布尔就只有两词,不是词表没收录的开放码。
 */
export function disclosableLabel(value: boolean): string {
  return value ? '成立' : '不成立';
}

export function autoReleaseLabel(value: boolean): string {
  return value ? '允许' : '不允许';
}

/**
 * 分诊结果封闭四格(domain TriageOutcome)。中文取 CONTEXT 语言:「自动建立或关联
 * 案件」「必须先进入分诊(人工复核)」;NO_CASE 是规则明说的不立案,不是没答。
 */
export const triageOutcomeLabels: Record<string, string> = {
  AUTO_ESTABLISH: '自动立案',
  ATTACH_TO_EXISTING: '关联既有案件',
  MANUAL_REVIEW: '进入人工复核',
  NO_CASE: '不立案',
};

/** 披露维态封闭三格(domain DimensionState,0012)。 */
export const disclosureStateLabels: Record<string, string> = {
  SHOWN: '展示',
  PENDING_CONFIRMATION: '待确认',
  NOT_DISCLOSED: '不披露',
};

/** 词表没收录的码原样示码,不猜词——开放集扩了、页面还没跟上时如实露码。 */
export function labelOf(table: Record<string, string>, code: string): string {
  return table[code] ?? code;
}

// ---- 各类目录登记写面的词表（ADR-0085,票 admin-write-faces/02 切片 02d;异常披露规则与
// 冲突信号规则两册随票 ve-disclosure-policy-view/02 步二加入）----

// 以下三张表覆盖有在线登记写面的每一册(VisibilityRegistrableCatalogueKind)。

/**
 * 登记签的标题。册名取读签同一个词;有无版本抬头照各册实情说,不给通知策略补一个,也不给
 * 冲突信号规则补一个——那册一租户一条,version 是识别规则版本引用而不是目录版本抬头。
 * 异常披露规则那册标题带「规则」,与「登记披露策略版本」并列时两字就是分册记号。
 */
export const registrationTitles: Record<VisibilityRegistrableCatalogueKind, string> = {
  MILESTONE_MAPPING: '登记里程碑映射版本',
  TRIAGE_RULE: '登记分诊规则版本',
  NOTIFICATION_POLICY: '登记通知策略',
  CLAIM_ELIGIBILITY: '登记索赔资格声明',
  CLAIM_AUTHORIZATION: '登记申请人授权名单',
  DISCLOSURE_POLICY: '登记披露策略版本',
  EXCEPTION_DISCLOSURE_RULE: '登记异常披露规则版本',
  CONFLICT_SIGNAL_RULE: '登记冲突信号规则',
};

/**
 * 受控登记口的命令名,逐册一个;词取 cmd/parcel-ve-register 已发布的原词。分诊与异常披露
 * 规则两册在 CLI 是复数,读口 kind 与端点路径用单数,各取各入口已发布的写法。
 */
const registrationCommands: Record<VisibilityRegistrableCatalogueKind, string> = {
  MILESTONE_MAPPING: 'milestone-mapping',
  TRIAGE_RULE: 'triage-rules',
  NOTIFICATION_POLICY: 'notification-policy',
  CLAIM_ELIGIBILITY: 'claim-eligibility',
  CLAIM_AUTHORIZATION: 'claim-authorization',
  DISCLOSURE_POLICY: 'disclosure-policy',
  EXCEPTION_DISCLOSURE_RULE: 'exception-disclosure-rules',
  CONFLICT_SIGNAL_RULE: 'conflict-signal-rule',
};

// 登记快照形状的提示句。各册只差命令名一词,所以由一处拼出:逐册抄写会让「不逐字段建
// 表单」这条理由在其中一遍被改动时悄悄分叉。
function snapshotHint(kind: VisibilityRegistrableCatalogueKind, particulars?: string): string {
  return (
    `登记快照 JSON 的形状与受控登记口 parcel-ve-register ${registrationCommands[kind]} -input <file> 吃的同一份` +
    '(两口共用同一份译装,不是两份碰巧同形);本页不逐字段建表单,因为「渠道原始载荷 → 登记快照」' +
    '的翻译属渠道接入契约,随 PAR-INT-01 提供。未知字段一律拒收——打错字段名不会静默变成「没给」。' +
    (particulars ?? '')
  );
}

/**
 * 各册登记快照的形状提示。带封闭集或带反直觉判据的册把话说出来:那几件打错之后,受理门
 * 给的是一句指名拒绝,而从册名上看不出来自己错在哪。
 */
export const registrationSnapshotHints: Record<VisibilityRegistrableCatalogueKind, string> = {
  MILESTONE_MAPPING: snapshotHint(
    'MILESTONE_MAPPING',
    '源上下文取封闭五词 PARCEL_SHIPMENT / NETWORK_ROUTING / NODE_OPERATIONS / TRANSPORT_FULFILLMENT / CUSTOMS_COMPLIANCE;' +
      '标准里程碑是租户自己的实例参数,登记口不校对词表。',
  ),
  TRIAGE_RULE: snapshotHint(
    'TRIAGE_RULE',
    '分诊走向取封闭四词 AUTO_ESTABLISH / ATTACH_TO_EXISTING / MANUAL_REVIEW / NO_CASE;' +
      '信号类型与可信度是开放引用,原词收下。',
  ),
  NOTIFICATION_POLICY: snapshotHint(
    'NOTIFICATION_POLICY',
    '这册没有版本抬头:版本化由披露策略引用本身承担,换版即换引用。时限收 Go 时长字面(如 "72h")' +
      '且必须为正——非正时限算出的截止点在披露决定之前,那样的通知一生成就已逾期。',
  ),
  CLAIM_ELIGIBILITY: snapshotHint(
    'CLAIM_ELIGIBILITY',
    '覆盖索赔种类至少一项:一份不覆盖任何种类的责任范围声明会把索赔核成永久的「不予受理」,' +
      '所以宁可不登这份声明——没有声明行时缺一个种类是「没人声明过」,那一格还能续办。',
  ),
  CLAIM_AUTHORIZATION: snapshotHint(
    'CLAIM_AUTHORIZATION',
    '申请人名单字段必须在场:不授权任何人写 [],那是「此账户当前不授权任何人代提」的显式决定;' +
      '整个字段缺席是漏填,两者恢复动作不同,登记口不压成一格。',
  ),
  DISCLOSURE_POLICY: snapshotHint(
    'DISCLOSURE_POLICY',
    '四维各取封闭三态 SHOWN / PENDING_CONFIRMATION / NOT_DISCLOSED;内容只在 SHOWN 时在场,' +
      '另两态带了内容即矛盾输入,登记口拒收而不是替登记方丢掉那半句声明。',
  ),
  EXCEPTION_DISCLOSURE_RULE: snapshotHint(
    'EXCEPTION_DISCLOSURE_RULE',
    '这册与披露策略是相邻两册,条目键是客户 × 信号类型 × 可信度三维(0023 主键),同一版里同键两条即撞键;' +
      '两条成对纪律在受理门就拒:disclosable 为真必带 content、为假必不带,autoRelease 只在 disclosable 为真时才可为真' +
      '——违背任一条答 ENTRY_INCOMPLETE,不会落到库上的 CHECK 变成依赖故障。',
  ),
  CONFLICT_SIGNAL_RULE: snapshotHint(
    'CONFLICT_SIGNAL_RULE',
    '这册一租户一条、没有版本抬头与生效区间:version 是识别规则版本引用,signalKind / version / confidence 三件任缺答 ENTRY_INCOMPLETE,' +
      '缺 approvedBy 答 APPROVAL_MISSING;租户已有一行时再登答 VERSION_NOT_OVERWRITABLE——换版是治理动作,原行不被顶替。',
  ),
};

/**
 * 登记答案代数（`application.RegisterCatalogOutcome` 原名）,两格中文。
 *
 * 本口没有「未决」格:目录登记是租户的管理动作,依赖调不通时没有一个如实的中间答案可记,
 * 那一路由传输层答「没形成答案」(5xx),不冒充一种业务答案。
 */
export const registrationOutcomeLabels: Record<string, string> = {
  REGISTERED: '已登记',
  REFUSED: '未登记——拒绝理由逐格指名;原行不被顶替',
};

/**
 * 拒绝理由（`application.CatalogRefusalReason` 原名）,逐格中文。
 *
 * 逐格分开而不折成一句「内容不合法」:各格的续办动作不同——缺版本号要给一个,缺发布批准
 * 责任要走审批,条目撞键要改内容,版本号已登记要换号。末两格更是登记册的治理答案而不是
 * 打错字,受控登记口正按这条界线分退出码 2 与 1 两路。
 */
export const registrationRefusalReasonLabels: Record<string, string> = {
  SCOPE_MISSING:
    '缺管辖或身份维——目录行只在租户内唯一;索赔资格与索赔授权两册还要指出合同责任范围或客户账户,通知策略要指出策略引用',
  VERSION_MISSING: '缺版本号——版本由登记方给,登记口不代拟',
  APPROVAL_MISSING: '缺发布批准责任——册面记的是「登记者声明了谁批准」,不代填',
  EFFECTIVE_RANGE_MISSING: '缺生效时间——零时刻不是能用的生效边界,拿它登记等于这一版从公元元年起适用',
  EFFECTIVE_RANGE_REVERSED: '生效区间倒序或为空——半开区间两端相等即不覆盖任何时点',
  ENTRIES_MISSING: '缺条目——空册不当作一版收;索赔资格的空覆盖集尤其通向永久的「不予受理」',
  ENTRY_INCOMPLETE: '条目缺件——某一条没说全;通知策略的非正时限也落这一格',
  ENTRY_DUPLICATED: '条目撞键——同一版里两条落在同一个键上,说不出以哪条为准',
  VERSION_NOT_OVERWRITABLE:
    '该版本号已登记——登记册不比对内容,同号再登一律不覆盖;换个版本号续办（治理答案,不是失败）',
  VERSION_OVERLAPS_EXISTING:
    '同一时点已有另一适用版本——原行未被顶替,人工核对后改区间续办（治理答案,不是失败）',
};
