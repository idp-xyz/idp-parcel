// 参与方与商业目录查阅词表。kind 取值与传输层封闭集同词;中文取 CONTEXT 原词。

import type { CommercialPolicyKind, CommercialRegistrationKind } from './api';
import { labelFor } from '../../vocabulary/status';

// 服务形态封闭集今天两格。LABEL_CHANNEL_SERVICE 此前在这里列过又被撤下,因为那时服务端
// 产生不出来:domain.ServiceProductForm 只认 NETWORK_SERVICE,填进快照会被受理门拒绝。
// ADR-0088 把 PAR-COM-12 由范围裁剪改为纳入,领域封闭集与迁移 CHECK(0017 放宽 0008 立下的
// service_product_form_closed)都已扩到两格,这一格因此装回来——**撤下时那条纪律照旧成立**:
// 词表与同屏那句提示必须一起改,一格词表配一句「只有一格」的提示会让操作者照读面填、
// 而回来的是拒绝。
export const serviceFormLabels: Record<string, string> = {
  NETWORK_SERVICE: '网络服务产品',
  LABEL_CHANNEL_SERVICE: '面单渠道服务',
};

// 状态词表都是用语词典（vocabulary/status.ts）的薄包装：词与色调只在词典一处定义，这里只留本册会出现的码，让筛选项、
// 类型派生与 codeFilterOptions 有一张封闭的键表可用。
export const commercialStatusLabels: Record<string, string> = statusTable(['DRAFT', 'PUBLISHED', 'EFFECTIVE', 'EXPIRED', 'RETIRED', 'SUPERSEDED']);

function statusTable<Code extends string>(codes: readonly Code[]): Record<Code, string> {
  return Object.fromEntries(codes.map((code) => [code, labelFor(code)])) as Record<Code, string>;
}

export const policyKindLabels: Record<CommercialPolicyKind, string> = {
  ACCEPTANCE_RULE_PACKAGE: '接单规则包',
  PRE_ACCEPTANCE_CONTROL: '接受前财务控制',
  PRICE_POLICY: '商业价格政策',
  SETTLEMENT_POLICY: '结算政策',
  // PC CONTEXT 全名「接受判断时点选择策略」；册按钮上取短名，全名在册说明里。
  AS_OF_POLICY: '接受判断时点策略',
  AUTHORIZATION_RULE: '授权规则',
  CREDIT_POLICY: '信用政策',
  // 与「接受前财务控制」是两本册不是一本的两个名字：那一本列合同的「要不要」声明，这一本列策略
  // 版本自己的「控制怎么做」正文（ADR-0115）。中文里把「策略」点出来，让两个 chip 在同一屏分得开。
  PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY: '接受前财务控制策略',
  // PC CONTEXT 原词。VE 那侧的通知义务、索赔类型覆盖两本册与本册正文归谁,pc-gaps/05 记着要走 ADR;这里只列
  // PC 这一侧的正文(适用对象、责任方、索赔期限、最低材料),册名不带「VE」也不带「索赔」,不替那个所有权裁决开口。
  CUSTOMER_SERVICE_RULE: '客户服务规则',
};

export const commercialPolicyKinds: CommercialPolicyKind[] = [
  'ACCEPTANCE_RULE_PACKAGE',
  'PRE_ACCEPTANCE_CONTROL',
  'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY',
  'PRICE_POLICY',
  'SETTLEMENT_POLICY',
  'AS_OF_POLICY',
  'AUTHORIZATION_RULE',
  'CREDIT_POLICY',
  'CUSTOMER_SERVICE_RULE',
];

/**
 * 每本册列的是什么、什么时候看它。页面在选中的册下方原样显示这一句。
 *
 * 读的人是配置员与查阅员，所以只讲业务：发布类别码与声明通道不上屏。册（`?kind=`）与发布类别码是两条
 * 分类轴、刻意不对齐（后端 query_commercial_policies.go 头注）；照册名抄类别码进发布快照会被拒，那份对照
 * 只对 JSON 发布那一签有用，写在 `registrationSnapshotHints.publication` 里，不在这里重复。
 *
 * 随所属版本发布、没有自己版本的两本（接受前财务控制、接受判断时点策略）要说出来，否则「版本」列
 * 会被读成这一册自己的版本。
 */
export const policyKindSources: Record<CommercialPolicyKind, string> = {
  ACCEPTANCE_RULE_PACKAGE: '接单规则包的各个版本及其正文：组装规则、允许的收寄来源、收寄资格与终局规则。',
  PRE_ACCEPTANCE_CONTROL:
    '每份客户合同对「接受前要不要做财务控制」的声明。它随合同一起发布，「版本」列是所属合同的版本；控制具体怎么做，看「接受前财务控制策略」册。',
  PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY:
    '接受前财务控制策略的各个版本：要执行哪些控制项、怎样算共同通过。合同上「明确不做控制」的声明不在这里，看「接受前财务控制」册。',
  PRICE_POLICY: '商业价格政策：每个价格方向绑定哪一个定价方案。它随价格规则的版本一起发布。',
  SETTLEMENT_POLICY: '结算政策的各个版本及其正文。',
  AS_OF_POLICY:
    '接受判断时点选择策略：每类接受判断按哪个时点取依据。它随接单规则包一起发布，「版本」列是所属规则包的版本。',
  AUTHORIZATION_RULE: '授权规则的各个版本，以及每类请求方的取消授权。',
  CREDIT_POLICY: '信用政策的各个版本及其正文；额度按金额或按比例，二者只取其一。',
  CUSTOMER_SERVICE_RULE:
    '客户服务规则的各个版本：适用于哪个服务产品或客户合同、责任方、索赔期限与最低材料。索赔类型、材料与起算事件的解释在可见性与异常那边，这里照原词显示。',
};

// 商业方向封闭三格(domain CommercialDirection),中文与计价方向同词——同一个方向
// 概念不因出现在不同页而换名。
export const commercialDirectionLabels: Record<string, string> = {
  BUY: '买价',
  SELL: '卖价',
  INTERNAL: '内部',
};

// 税务口径封闭三格（domain TaxDisposition，CONTEXT「必须声明含税、未税或税务不适用」）。没有第四格：缺席是「未声明」，
// 由服务端点名，不折成不适用。
export const taxDispositionLabels: Record<string, string> = {
  TAX_INCLUSIVE: '含税',
  TAX_EXCLUSIVE: '未税',
  TAX_NOT_APPLICABLE: '税务不适用',
};

// 方案绑定转换封闭两格（domain PlanBindingConversion，ADR-0057）。NONE 是「明说不转换」，与「没写」是两回事——表单不预选。
export const planBindingConversionLabels: Record<string, string> = {
  NONE: '不转换（政策方向与方案方向一致）',
  FROZEN_BUY_EVALUATION: '引用一次已冻结的采购评价（只许 SELL 政策绑 BUY 方案）',
};

// 结算方式封闭两格(domain SettlementMethod)。
export const settlementMethodLabels: Record<string, string> = {
  PREPAID: '预付',
  TERMS: '账期',
};

// 比例额度的基数封闭两格(domain CreditRatioBase, ADR-0129)。只是中文装饰：码从词表读口来，表单不内置枚举；
// 词表没收录的码原样示出。
export const creditRatioBaseLabels: Record<string, string> = {
  POSTED_BALANCE: '入账余额',
  PRIOR_PERIOD_CONFIRMED_CHARGES: '上一结算周期已确认费用合计',
};

// 接受前财务控制要求封闭两格(domain PreAcceptanceControl)。
export const controlRequirementLabels: Record<string, string> = {
  REQUIRED: '要求',
  NOT_APPLICABLE: '不适用',
};

// 策略正文的三个封闭集（domain PreAcceptanceControlKind / ControlFailureDisposition / JointPassCondition，
// ADR-0115）。控制种类里没有「无控制」不是漏配词：那一句由合同声明，正文表上 CHECK 就进不去。
export const controlKindLabels: Record<string, string> = {
  PREPAID_FREEZE: '预付冻结',
  CREDIT_CHECK: '信用校验',
};

export const controlFailureDispositionLabels: Record<string, string> = {
  REJECT: '拒绝',
  AUTHORIZED_DISPOSITION: '进入授权处置',
};

export const jointPassConditionLabels: Record<string, string> = {
  ALL_CONTROLS_PASS: '全部控制通过',
};

// 索赔期限种类封闭三格（domain ClaimDeadlineKind），中文逐字取 visibility-exception CONTEXT「客户首次索赔期限、
// 资料补充期限和结论复核期限是三个独立期限」。起算事件、日历、索赔类型与材料不在这里：它们是开放引用（解释权在
// VE），本台不替它们造词，按引用原词展示。
export const claimDeadlineKindLabels: Record<string, string> = {
  FIRST_CLAIM: '客户首次索赔期限',
  MATERIAL_SUPPLEMENT: '资料补充期限',
  CONCLUSION_REVIEW: '结论复核期限',
};

// 收寄来源封闭二值(domain DeclaredIntakeSource),对应 parcel-shipment 来源联合的两格。
export const intakeSourceLabels: Record<string, string> = {
  NODE_INTAKE: '节点收寄',
  OFFSITE_PICKUP: '场外揽收',
};

// 终局责任结果封闭四值(domain 的终局规则声明),中文取 CONTEXT 原词。
export const finalOutcomeLabels: Record<string, string> = {
  EFFECTIVE_DELIVERY: '有效送达',
  RETURN_COMPLETED: '退回完成',
  SERVICE_TERMINATED: '服务终止',
  REGULATORY_DISPOSITION: '监管处置',
};

// 取消请求方封闭二值(domain DeclaredCancellationParty)。
export const cancellationPartyLabels: Record<string, string> = {
  CUSTOMER: '客户',
  OPERATIONS: '运营',
};

// 下面四张身份族词表的键在类型上是封闭的（`as const` + `keyof typeof`，与 domain/status.tsx 的 domainStatusTones
// 同一写法）：列表模块的筛选码类型、停用表单的种类类型都从键派生，不在别处再抄一份联合——词表扩一格，派生处
// 自动跟上（票 admin-web-group-legal-entities/09 评审 N4）。仍可当 Record<string, string> 传给 labelOf。

// 参与方身份生命周期封闭三格(domain IdentityStatus)。与 commercialStatusLabels 分表:
// 身份状态按时点导出,商业对象状态是发布生命周期,同词 EFFECTIVE 在两套代数里含义
// 不同,并表会让一套的封闭性替另一套背书。
// 词取词典（REGISTERED 上屏是「待生效」，理由在 vocabulary/status.ts）；这里只定本册的三格键。
export const identityStatusLabels = statusTable(['REGISTERED', 'EFFECTIVE', 'DEACTIVATED'] as const);

export type IdentityStatusCode = keyof typeof identityStatusLabels;

// 参与方关系生命周期封闭五格(domain RelationshipStatus),中文取 CONTEXT 原词。
// 词取词典（CANDIDATE 上屏是「待确认」）；这里只定本册的五格键。
export const relationshipStatusLabels = statusTable(['CANDIDATE', 'EFFECTIVE', 'EXPIRED', 'REVOKED', 'SUPERSEDED'] as const);

export type RelationshipStatusCode = keyof typeof relationshipStatusLabels;

// 参与方角色封闭集(domain PartyRole),中文取 CONTEXT 原词。
export const partyRoleLabels = {
  CUSTOMER: '客户',
  SUPPLIER: '供应商',
  CARRIER_AGENT: '承运商代理',
  RESELLER: '转售',
  ACCOUNT_HOLDER: '渠道账号持有',
  AUTHORIZED_DELIVERING_PARTY: '授权交付方',
} as const satisfies Record<string, string>;

export type PartyRoleCode = keyof typeof partyRoleLabels;

/**
 * 参与方按它持有的角色称呼（客户、供应商、承运商代理……）；一个角色都没有时叫「往来单位」——它在册、有名称，只是还没
 * 登记与谁的合作关系。角色词取关系册的封闭集，不另造一套「类型」。
 */
export const partyTypeFallbackLabel = '往来单位';

export function partyTypeLabel(roles: readonly string[]): string {
  if (roles.length === 0) return partyTypeFallbackLabel;
  return roles.map((role) => labelOf(partyRoleLabels, role)).join(' / ');
}

/**
 * 修订号给人看时说「第 N 次更新」：首笔也算一次，所以 r1 就是第 1 次。列表主列不显它，只在变更记录与事实条上出现；
 * 技术详情里仍是裸数字。
 */
export function updateOrdinalLabel(revision: number): string {
  return `第 ${revision} 次更新`;
}

// 法人册对象类型今天只有一格(传输层 kindResponsibleLegalEntity):经营组织没有
// 登记面,如实不上列,不预开空格。
export const legalEntityKindLabels: Record<string, string> = {
  RESPONSIBLE_LEGAL_ENTITY: '责任法人',
};

// 责任法人身上有两条修订轴——身份登记修订与法人资料修订（ADR-0145 决定三），各自从 1 编号，同一屏常常一起出现；
// 写成裸 rN 时「r1」与「生效修订 r2」读不出哪个是当前。法人页上凡显修订号都经这两个函数带上轴名。业务参与方
// 只有身份一条轴，那几页照旧写裸 rN，不用这两个。
export function identityRevisionLabel(revision: number): string {
  return `身份 r${revision}`;
}

export function profileRevisionLabel(revision: number): string {
  return `资料 r${revision}`;
}

// 可停用的身份种类封闭集（application.PartyIdentityKind 的名称镜像 identityKindFromName），中文取 CONTEXT 原词。
// 关系不在内：关系的终止走撤销 / 到期 / 替代，不叫停用（DeactivatePartyIdentityCommand 注释）。停用表单的种类
// 下拉只从这里派生，不另抄一份封闭集。
export const identityKindLabels = {
  BUSINESS_PARTY: '业务参与方',
  LEGAL_ENTITY: '责任法人',
  CUSTOMER_ACCOUNT: '货主客户账户',
} as const satisfies Record<string, string>;

export type IdentityKind = keyof typeof identityKindLabels;

// 法人与客户账户的「名称」在参与方册上转写而来；转不到是写入门失败才会出现的悬空引用，如实标出让人去查写侧，
// 不补占位文本冒充名称。两页的列、抽屉与三册候选转写说的是同一件事，同一句话只在这里一处。
export const partyNameUnknownNote = '参与方册查无此身份';

/** 修订登记于身份层落地之前（identityLayerRegistered 为假）时身份两格的话；不填假值，句子只在这一处。 */
export const identityLayerAbsentNote = '本修订登记时尚无此格';

/** 法人资料按时点解析的五格（ADR-0145 决定五、六；后端 domain.LegalEntityProfileResolution 原名）。 */
export const profileResolutionOutcomeLabels: Record<string, string> = {
  RESOLVED: '已解析：这一刻有生效的资料修订，开票资料齐',
  PROFILE_INCOMPLETE: '资料不全（明确非成功，开立方据此拒开，不以默认值补齐）',
  LEGAL_ENTITY_NOT_REGISTERED: '法人不在册',
  LEGAL_ENTITY_NOT_EFFECTIVE: '法人在这一刻尚未生效',
  LEGAL_ENTITY_DEACTIVATED: '法人在这一刻已停用',
};

/** 资料不全的两种原因。 */
export const profileIncompleteCauseLabels: Record<string, string> = {
  NO_EFFECTIVE_REVISION: '这一刻没有生效的资料修订',
  NO_INVOICING_DETAILS: '生效的那一笔没有开票资料',
};

export const problemCodeNotes: Record<string, string> = {
  METHOD_NOT_ALLOWED: '请求方法不被该端点允许。这是调用方式问题,不是业务答案。',
  // 一句覆盖读写两侧：problemNote 只拿得到 code，分不出这个 400 来自目录读口（kind）还是登记口（载荷形状），
  // 只写读口那半会让登记被拒的人读到与真相无关的原因。成因对照 isolated_write_intake.go 里包
  // ErrMalformedRequest 的那几道判断；服务端带 detail 之后这一句只需收短。
  MALFORMED_REQUEST:
    '请求的格式不对，原样重发不会改变结果。常见原因：带了 tenantId、有不认识的键、一次粘了多项或一项都没有、' +
    '修订号不是整数、时间格式不对、可选词不在范围内、标识或依据为空，或 JSON 后面还多了内容。',
  INTAKE_FAILED: '接入解析未能完成,本次没有形成任何业务答案,可稍后重试。',
  NO_ANSWER_FORMED: '服务端处理未能完成,本次没有形成任何业务答案,可稍后重试。',
  // 只可能来自登记写面：服务端交回了一个没有名字的答案，那是实现坏了，不是一种新的
  // 业务结果。续办与 NO_ANSWER_FORMED 同为去查服务端记录，但成因不同，因此不合并。
  UNNAMED_OUTCOME: '服务端交回了没有名字的答案。这是服务端实现缺陷,不是业务结果,请报障。',
};

export function problemNote(code: string): string {
  return problemCodeNotes[code] ?? '未知错误码。请携带关联标识查询服务端记录。';
}

export function labelOf(table: Record<string, string>, code: string): string {
  return table[code] ?? code;
}

// ——以下为登记签的页面口径（ADR-0085，票 admin-write-faces/02 商业片）。

/**
 * 八类登记签的标题。册名与本文件上方的查阅词表同词——同一本册不因换到写签而换名。
 *
 * 发布那一格说「发布」不说「登记」：UC-PC-001 的动词就是发布，答案代数也是发布的
 * （已生效/已计划生效/未决），改叫登记会让操作者拿它与身份、映射两族的登记答案对齐。
 */
export const registrationTitles: Record<CommercialRegistrationKind, string> = {
  publication: '发布新版本',
  'business-party': '新增往来单位',
  'legal-entity': '新增公司主体',
  'legal-entity-profile': '更新公司资料',
  'customer-account': '新增客户账户',
  'party-relationship': '建立合作关系',
  'identity-deactivation': '停用',
  'service-product-form': '登记服务形态',
  'product-channel-mapping': '登记产品可用渠道',
};

// 登记快照形状的提示句。各类共用的开头由一处拼出：抄几遍会让「这一签是什么」那句在其中一遍被改时悄悄分叉。
//
// 这一签的定位按 ADR-0101 决定一：JSON 快照签是受控批量口的在线镜像，给熟悉登记格式的人用，不是配置员的主路径。
// 「一次一项，不是整批」要说：把受控 CLI 的整批粘进来会被拒，而拒绝理由只说形状不对，看不出自己多包了一层。
//
// 「不要带 tenantId」只给在线口已放行的那几签：身份族的隔离 Intake（`refuseSelfReportedTenant`）对载荷里的
// tenantId 键在场即拒、含 null——租户由接入渠道填入，不采信自报（ADR-0100 决定二）。发布口与产品渠道族那几签的
// 提示句照批文形状说，等那几口放行时随其票改。
const tenantGridFilledByChannel = '不要带 tenantId：租户由你的登录身份决定，带了（含 null）会被拒。';

function snapshotHint(fields: string, options?: { tenantGridFilledByChannel: true }): string {
  return (
    '这是高级入口：粘贴一项登记的 JSON，一次一项，不是整批。' +
    (options?.tenantGridFilledByChannel ? tenantGridFilledByChannel : '') +
    fields
  );
}

/**
 * 各类登记快照的形状提示。逐类把键名与封闭集词列出来：未知键一律被译装拒绝（打错的键
 * 静默丢弃会让操作员以为登进去的比实际多），而封闭集里的词打错在类名上看不出来。
 *
 * 身份三册与关系册都不收「改内容」：更正占下一个修订号翻旧插新，停用走 identity-
 * deactivation 那一签形成新修订。这句写进提示，是因为读面上「最新登记修订」那一格最
 * 容易被读成「改这一行」。
 */
export const registrationSnapshotHints: Record<CommercialRegistrationKind, string> = {
  publication: snapshotHint(
    '一项的键：tenantId、kind、objectId、version、scope、contentDigest、effectiveStartsAt、' +
      'approval{reference,source,approvedAt}、approvalRoleStanding，可选 effectiveEndsAt、references、declarations。' +
      'kind 是发布类别码，共十个：SERVICE_PRODUCT、CUSTOMER_CONTRACT、SUPPLIER_AGREEMENT、ACCEPTANCE_RULE_PACKAGE、' +
      'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY、PRICE_RULE、SETTLEMENT_POLICY、CREDIT_POLICY、AUTHORIZATION_RULE、' +
      'CUSTOMER_SERVICE_RULE。类别码与本台的页名、册名不是一一对应：前三类分别显示在服务产品、客户与合同、' +
      '供应商协议三页；其余七类显示在「商业规则与策略」页对应的册里（PRICE_RULE 显示在商业价格政策册；' +
      'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY 显示在接受前财务控制策略册，它与「接受前财务控制」册是两本，' +
      '后者列的是随客户合同发布的声明；CUSTOMER_SERVICE_RULE 显示在客户服务规则册）。本页不替你填写或检查 kind。' +
      'declarations 里的各项（如 AS_OF_POLICY、PRE_ACCEPTANCE_CONTROL、RULE_PACKAGE_BODY、PRICE_POLICY_BODY）不是 kind：' +
      '它们没有自己的版本，随所属版本一起发布。声明只能随发布一起登记，发布后要改须发新版本。',
  ),
  'business-party': snapshotHint(
    'businessParties 数组里一项的键：partyId、name、revision、basis、effectiveFrom。' +
      '首笔修订号必须是 1，之后必须连续；跳号说明你看到的列表已过时，会被拒，不会替你猜。',
    { tenantGridFilledByChannel: true },
  ),
  'legal-entity': snapshotHint(
    'legalEntities 数组里一项的键：legalEntityId、partyId、revision、basis、effectiveFrom；' +
      '身份部分可带 registrationCountry、lifetimeRegistrationNumbers[{typeCode, number}]、identityCorrectionBasis' +
      '（首笔登记两格缺一不可；更正依据只在更正修订上填）。' +
      '所钉的业务参与方必须已登记，且在法人生效时已生效。',
    { tenantGridFilledByChannel: true },
  ),
  'legal-entity-profile': snapshotHint(
    'profiles 数组里一项的键：legalEntityId、revision、basis、effectiveFrom，可选 registeredAddress{country, lines}、' +
      'taxRegistrationNumbers[{typeCode, number}]、invoiceTitle、contacts[{name, email, phone}]。新修订从它的生效时点起' +
      '取代前一修订，可以登记未来生效的修订；注册地址的国家须与法人身份上的注册国家 / 地区一致。不填 invoiceTitle 表示' +
      '这一笔不带开票资料，填空字符串会被拒。',
    { tenantGridFilledByChannel: true },
  ),
  'customer-account': snapshotHint(
    'customerAccounts 数组里一项的键：accountId、customerPartyId、revision、basis、effectiveFrom。' +
      '所指的业务参与方要求同法人登记；不能绑定别的租户的参与方。' +
      '登记结果显示在客户与合同页的「客户账户」签。',
    { tenantGridFilledByChannel: true },
  ),
  'party-relationship': snapshotHint(
    'relationships 数组里一项的键：relationshipId、revision、holder、counterparty、role、scope、basis、effectiveStartsAt，' +
      '可选 effectiveEndsAt 与 approval{reference,approvedAt}。' +
      'role 只能取 CUSTOMER、SUPPLIER、CARRIER_AGENT、RESELLER、ACCOUNT_HOLDER 之一。' +
      '不带 approval 登记为候选关系，批准另登一笔新修订。',
    { tenantGridFilledByChannel: true },
  ),
  'identity-deactivation': snapshotHint(
    'deactivations 数组里一项的键：kind、id、revision、basis、at。' +
      'kind 只能取 BUSINESS_PARTY、LEGAL_ENTITY、CUSTOMER_ACCOUNT 之一；关系不在其中，' +
      '关系的结束走撤销、到期或替代，不叫停用。revision 是停用这一笔的修订号（列表上最新的修订号 + 1）；' +
      '对不上说明有人刚登过一笔，或你看到的列表已过时。' +
      '法人与客户账户也在这里停用，结果分别显示在集团与法人页、客户与合同页的「客户账户」签。',
    { tenantGridFilledByChannel: true },
  ),
  'service-product-form': snapshotHint(
    'forms 数组里一项的键：productId、version、form，外加整批的 tenantId 与 scope。' +
      'form 只能取 NETWORK_SERVICE（网络服务产品）或 LABEL_CHANNEL_SERVICE（面单渠道服务）。' +
      '所指的产品版本必须已发布且已生效：挂在未生效或已结束的版本上永远不会被选中。',
  ),
  'product-channel-mapping': snapshotHint(
    'mappings 数组里一项的键：mappingId、revision、productId、productVersion、channels、basis、effectiveStartsAt，' +
      '可选 effectiveEndsAt；外加整批的 tenantId 与 scope。' +
      '不带 channels 算漏填；写 [] 才表示「这个产品暂无可用渠道」。映射钉在它的产品版本上：' +
      '要改指别的产品，登记一个新的映射标识，而不是本映射的新修订。',
  ),
};

