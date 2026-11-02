// 运营追踪与目录查阅的结果词表：源环节、传输层错误、目录签词、登记签标题与提示、登记答复。各页只引用，不另抄。
//
// 事实类型、里程碑、信号类型、可信度、渠道、义务判据这类开放引用不在这里译：它们是租户登记的实例参数，
// 界面词走用语词典（vocabulary/status.ts）的参考译法，查不到的码原样显示。

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
 * 目录种类的签词，查阅签、登记签与计数摘要共用，全管理台只此一处。追踪规则与通知与披露两页的六本册按**管什么事**
 * 称呼（改版第二轮）：配置员认的是「动态归成哪个状态」「客户能看到什么」，不是册名；领域册名原词（里程碑映射、
 * 披露策略……）留在代码与 CONTEXT，界面上进右栏审计（rule-books.ts 的 ruleBookDomainNames）。
 * 「对客户显示」（披露策略，0012）与「异常告知」（异常披露规则，0023）是相邻两本册，签名各说各的事，不缩成同一个词。
 */
export const visibilityCatalogueKindLabels: Record<VisibilityCatalogueKind, string> = {
  MILESTONE_MAPPING: '状态归类',
  TRIAGE_RULE: '异常分流',
  NOTIFICATION_POLICY: '通知时限',
  CLAIM_ELIGIBILITY: '索赔资格',
  CLAIM_AUTHORIZATION: '索赔授权',
  DISCLOSURE_POLICY: '对客户显示',
  EXCEPTION_DISCLOSURE_RULE: '异常告知',
  CONFLICT_SIGNAL_RULE: '动态矛盾',
};

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

/** 词表没收录的码原样示码,不猜词——开放集扩了、页面还没跟上时如实露码。 */
export function labelOf(table: Record<string, string>, code: string): string {
  return table[code] ?? code;
}

// ---- 各类目录登记写面的词表（ADR-0085,票 admin-write-faces/02 切片 02d;异常披露规则与
// 冲突信号规则两册随票 ve-disclosure-policy-view/02 步二加入）----

// 以下几张表覆盖有在线登记写面的每一册,登记签与登记答复只认这里。

/**
 * 登记签的标题：说这个任务做什么,不叫「登记 ×× 版本」。冲突信号规则一租户一条,所以是「设定」不是「新增」。
 * 索赔两册由索赔前置页摆,措辞随那一页。
 */
export const registrationTitles: Record<VisibilityRegistrableCatalogueKind, string> = {
  MILESTONE_MAPPING: '新增状态归类规则',
  TRIAGE_RULE: '新增异常分流规则',
  NOTIFICATION_POLICY: '新增通知时限',
  CLAIM_ELIGIBILITY: '登记索赔资格声明',
  CLAIM_AUTHORIZATION: '登记申请人授权名单',
  DISCLOSURE_POLICY: '新增对客户显示规则',
  EXCEPTION_DISCLOSURE_RULE: '新增异常告知规则',
  CONFLICT_SIGNAL_RULE: '设定动态矛盾规则',
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

// 索赔两册的形状提示,措辞随索赔前置页（本轮不改那一页的话）。
function snapshotHint(kind: VisibilityRegistrableCatalogueKind, particulars?: string): string {
  return (
    `登记快照 JSON 的形状与受控登记口 parcel-ve-register ${registrationCommands[kind]} -input <file> 吃的同一份` +
    '(两口共用同一份译装,不是两份碰巧同形);本页不逐字段建表单,因为「渠道原始载荷 → 登记快照」' +
    '的翻译属渠道接入契约,随 PAR-INT-01 提供。未知字段一律拒收——打错字段名不会静默变成「没给」。' +
    (particulars ?? '')
  );
}

// 规则册六本的填写提示：只说填什么、填错了会怎样。形状与命令行工具吃的同一份,这一句留给对接的人。
function taskHint(kind: VisibilityRegistrableCatalogueKind, fields: string): string {
  return `${fields} 内容与命令行工具（${registrationCommands[kind]}）吃的同一份；所属公司由系统记下，不用填。写错的字段名会被退回，不会当成没填。`;
}

/** 各册的填写提示。带封闭集或带反直觉判据的册把话说出来：那几件写错之后,从册名上看不出自己错在哪。 */
export const registrationSnapshotHints: Record<VisibilityRegistrableCatalogueKind, string> = {

  MILESTONE_MAPPING: taskHint(
    'MILESTONE_MAPPING',
    '请填 version（这一版的编号）、approvedBy（批准人）、effectiveFrom（从何时起用）和 entries（至少一条）；每条填 source（环节：' +
      'PARCEL_SHIPMENT 运单 / NETWORK_ROUTING 路线 / NODE_OPERATIONS 网点 / TRANSPORT_FULFILLMENT 运输 / CUSTOMS_COMPLIANCE 清关）、' +
      'factKind（动态类型）、milestone（归成的状态）。同一版里同一环节的同一动态只能写一条。',
  ),
  TRIAGE_RULE: taskHint(
    'TRIAGE_RULE',
    '请填 version、approvedBy、effectiveFrom 和 entries（至少一条）；每条填 kind（信号类型）、confidence（可信度）、outcome（处理方式：' +
      'AUTO_ESTABLISH 自动立案 / ATTACH_TO_EXISTING 并入已有案件 / MANUAL_REVIEW 转人工复核 / NO_CASE 不立案），自动立案时可填 team（负责团队）。',
  ),
  CONFLICT_SIGNAL_RULE: taskHint(
    'CONFLICT_SIGNAL_RULE',
    '请填 signalKind（记成哪类信号）、version（识别规则的版本）、confidence（可信度依据）、approvedBy（批准人），四项都必填。' +
      '每家公司只有一条：已经设定过时再提交会被退回，要换规则请联系管理员。',
  ),
  NOTIFICATION_POLICY: taskHint(
    'NOTIFICATION_POLICY',
    '请填 policy（对应的客户显示规则版本）、channel（通知渠道）、deadlineAfter（多久之内，如 "72h"，必须大于零）、' +
      'obligation（怎样算通知到）、approvedBy（批准人）。要换时限就登一条新的，旧的仍保留。',
  ),
  DISCLOSURE_POLICY: taskHint(
    'DISCLOSURE_POLICY',
    '请填 version、approvedBy、effectiveFrom 和 entries（每个客户一条）；每条填 customer（客户编码），以及 milestones（状态）、eta（预计送达）、' +
      'final（最终结果）、note（备注）四项，每项写 state（SHOWN 显示 / PENDING_CONFIRMATION 待确认 / NOT_DISCLOSED 不显示），只有显示时才填 content（文案）。',
  ),
  EXCEPTION_DISCLOSURE_RULE: taskHint(
    'EXCEPTION_DISCLOSURE_RULE',
    '请填 version、approvedBy、effectiveFrom 和 entries；每条填 customer（客户编码）、signalKind（异常类型）、confidence（可信度）、' +
      'disclosable（要不要告诉客户，true / false）、autoRelease（能不能自动发出，只有要告诉客户时才能为 true）；要告诉客户时必填 content（文案），不告诉时不填。' +
      '同一版里同一客户、同一异常类型、同一可信度只能写一条。',
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
};

/**
 * 登记答案（`application.RegisterCatalogOutcome` 原名）。
 *
 * 本口没有「未决」格:目录登记是租户的管理动作,依赖调不通时没有一个如实的中间答案可记,
 * 那一路由传输层答「没形成答案」(5xx),不冒充一种业务答案。
 */
export const registrationOutcomeLabels: Record<string, string> = {
  REGISTERED: '已保存，切回列表就能看到。',
  REFUSED: '没有保存，原因见下；已有的规则没有被改动。',
};

/**
 * 退回的理由（`application.CatalogRefusalReason` 原名）,逐格说怎么改。逐格分开而不折成一句「内容不合法」:各格的
 * 续办动作不同。末两格不是填错,是已有规则挡着,要换版本号或改生效时间（受控登记口按这条界线分退出码 2 与 1）。
 */
export const registrationRefusalReasonLabels: Record<string, string> = {
  SCOPE_MISSING:
    '缺少对象：通知时限要写对应的显示规则版本（policy），索赔资格要写合同、索赔授权要写客户，其余规则按提示写客户或类型。',
  VERSION_MISSING: '缺少版本号（version），请给这一版起一个编号。',
  APPROVAL_MISSING: '缺少批准人（approvedBy），请写上谁批准了这份规则。',
  EFFECTIVE_RANGE_MISSING: '缺少起用时间（effectiveFrom），请写从什么时候开始用。',
  EFFECTIVE_RANGE_REVERSED: '起止时间写反了或首尾相同，请改成先起用、后截止。',
  ENTRIES_MISSING: '一条都没写（条目为空），请至少写一条；索赔资格至少要覆盖一种索赔。',
  ENTRY_INCOMPLETE: '有一条没写全；通知时限必须大于零，要告诉客户的异常必须带文案、不告诉的不能带。',
  ENTRY_DUPLICATED: '同一版里有两条管的是同一件事，请删掉或合并其中一条。',
  VERSION_NOT_OVERWRITABLE: '这个版本号已经用过了，已有的不会被覆盖；请换一个新的版本号。',
  VERSION_OVERLAPS_EXISTING: '同一时间已经有另一版在用，请先核对已有的，再调整起用时间。',
};

/**
 * 登记签上的传输层错误码（不是业务答复）：说哪里错、怎么办。与上面 problemNote 分开,是因为那一份同时给查阅口用
 * （物流动态、索赔前置的列表），措辞得两边都说得通；登记签只有「这次保存没成」这一种语境。
 */
const registrationProblemNotes: Record<string, string> = {
  METHOD_NOT_ALLOWED: '这个入口不接受这种提交方式，请刷新页面后重试；仍然不行请联系系统管理员。',
  MALFORMED_REQUEST: '填写的内容格式不对，系统读不出一份完整的规则；请按提示检查字段名和取值后再提交。',
  INTAKE_FAILED: '登录信息没能核对通过，这次什么都没有保存；请稍后重试。',
  NO_ANSWER_FORMED: '系统暂时没能处理，这次是否保存还不确定；请稍后刷新列表查看。',
  UNNAMED_OUTCOME: '系统返回了无法识别的结果，请联系系统管理员；重试不会好。',
  UNNAMED_REFUSAL_REASON: '被退回了，但系统没说明原因，请联系系统管理员；重试不会好。',
};

export function registrationProblemNote(code: string): string {
  return registrationProblemNotes[code] ?? `这次没有成功（错误码 ${code}）；请记下页面上的技术详情联系系统管理员。`;
}
