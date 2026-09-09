// 参与方与商业目录查阅词表。kind 取值与传输层封闭集同词;中文取 CONTEXT 原词。

import type { CommercialPolicyKind, CommercialRegistrationKind } from './api';

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

export const commercialStatusLabels: Record<string, string> = {
  DRAFT: '草稿',
  PUBLISHED: '已发布',
  EFFECTIVE: '已生效',
  EXPIRED: '已到期',
  RETIRED: '已退役',
  SUPERSEDED: '已替代',
};

export const policyKindLabels: Record<CommercialPolicyKind, string> = {
  ACCEPTANCE_RULE_PACKAGE: '接单规则包',
  PRE_ACCEPTANCE_CONTROL: '接受前财务控制',
  PRICE_POLICY: '商业价格政策',
  SETTLEMENT_POLICY: '结算政策',
  AS_OF_POLICY: '时点锚声明',
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
 * 每本册列的是什么、由发布口的哪一类版本（或哪个声明通道）喂进来。
 *
 * 册（`?kind=`）与发布口的对象类别是两条分类轴，**刻意不对齐**：后端 query_commercial_policies.go
 * 头注写明「种类命名册子而不是商业对象类别……拿对象类别当种类名会指错拥有者」。同屏只摆两套词
 * 而不说清关系，操作者会照 chip 抄一个 PRICE_POLICY 进发布快照，然后被受理门拒——票
 * admin-write-faces/03 记的正是这一格。所以这里逐册把「谁喂它」写成一句，页面在册名旁原样显示。
 *
 * 声明通道两本（接受前财务控制、时点锚）没有自己的版本：它们是随所属版本一并发布的 `declarations`，
 * 页面上的「版本」列指的是所属版本。
 */
export const policyKindSources: Record<CommercialPolicyKind, string> = {
  ACCEPTANCE_RULE_PACKAGE:
    '列接单规则包版本及其正文；由发布口对象类别 ACCEPTANCE_RULE_PACKAGE 喂入，正文经声明通道 RULE_PACKAGE_BODY 随发布登记。',
  PRE_ACCEPTANCE_CONTROL:
    '列「这份合同要不要接受前财务控制」的声明；它挂在 CUSTOMER_CONTRACT 版本下、经声明通道 PRE_ACCEPTANCE_CONTROL 随合同发布登记——不是 PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY 版本本身（那一类版本的正文在「接受前财务控制策略」册）。',
  PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY:
    '列接受前财务控制策略版本及其正文（要执行的控制项与共同通过条件）；由发布口对象类别 PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY 喂入，正文经声明通道 PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY_BODY 随发布登记。「明确无控制」不在这里——它是合同的声明，看「接受前财务控制」册。',
  PRICE_POLICY:
    '列商业价格政策正文（方向 × 定价方案绑定）；它挂在发布口对象类别 PRICE_RULE 的版本下，经声明通道 PRICE_POLICY_BODY 随发布登记。',
  SETTLEMENT_POLICY:
    '列结算政策正文；由发布口对象类别 SETTLEMENT_POLICY 喂入，正文经声明通道 SETTLEMENT_POLICY_BODY 随发布登记。',
  AS_OF_POLICY:
    '列时点锚声明；它没有自己的版本，挂在 ACCEPTANCE_RULE_PACKAGE 版本下、经声明通道 AS_OF_POLICY 随规则包发布登记。',
  AUTHORIZATION_RULE:
    '列授权规则版本及按请求方逐格的取消授权；由发布口对象类别 AUTHORIZATION_RULE 喂入，取消授权经声明通道 CANCELLATION_AUTHORITY 随发布登记。',
  CREDIT_POLICY:
    '列信用政策正文；由发布口对象类别 CREDIT_POLICY 喂入，正文经声明通道 CREDIT_POLICY_BODY 随发布登记。',
  CUSTOMER_SERVICE_RULE:
    '列客户服务规则版本及其正文（挂在哪个服务产品或客户合同上、责任方、索赔期限与最低材料）；由发布口对象类别 CUSTOMER_SERVICE_RULE 喂入，正文经声明通道 CUSTOMER_SERVICE_RULE_BODY 随发布登记。只列 PC 这一侧的正文；索赔类型与材料目录、起算事件的解释归可见性与异常那侧，这里按引用原词展示。',
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

// 参与方身份生命周期封闭三格(domain IdentityStatus)。与 commercialStatusLabels 分表:
// 身份状态按时点导出,商业对象状态是发布生命周期,同词 EFFECTIVE 在两套代数里含义
// 不同,并表会让一套的封闭性替另一套背书。
export const identityStatusLabels: Record<string, string> = {
  REGISTERED: '已登记',
  EFFECTIVE: '已生效',
  DEACTIVATED: '已停用',
};

// 参与方关系生命周期封闭五格(domain RelationshipStatus),中文取 CONTEXT 原词。
export const relationshipStatusLabels: Record<string, string> = {
  CANDIDATE: '候选关系',
  EFFECTIVE: '已生效',
  EXPIRED: '已到期',
  REVOKED: '已撤销',
  SUPERSEDED: '已替代',
};

// 参与方角色封闭五格(domain PartyRole),中文取 CONTEXT 原词。
export const partyRoleLabels: Record<string, string> = {
  CUSTOMER: '客户',
  SUPPLIER: '供应商',
  CARRIER_AGENT: '承运商代理',
  RESELLER: '转售',
  ACCOUNT_HOLDER: '渠道账号持有',
};

// 法人册对象类型今天只有一格(传输层 kindResponsibleLegalEntity):经营组织没有
// 登记面,如实不上列,不预开空格。
export const legalEntityKindLabels: Record<string, string> = {
  RESPONSIBLE_LEGAL_ENTITY: '责任法人',
};

export const problemCodeNotes: Record<string, string> = {
  METHOD_NOT_ALLOWED: '请求方法不被该端点允许。这是调用方式问题,不是业务答案。',
  MALFORMED_REQUEST:
    '请求构造不出查询(kind 缺席或不在封闭集),重发同样的内容不会改变结果。',
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
  publication: '发布商业权威依据版本',
  'business-party': '登记业务参与方身份修订',
  'legal-entity': '登记责任法人身份修订',
  'customer-account': '登记货主客户账户修订',
  'party-relationship': '登记参与方关系修订',
  'identity-deactivation': '停用身份（形成新修订）',
  'service-product-form': '登记服务产品版本的服务形态',
  'product-channel-mapping': '登记产品—渠道映射修订',
};

// 登记快照形状的提示句。八类共用的前半由一处拼出：抄八遍会让「这一签是什么」那句
// 在其中一遍被改动时悄悄分叉。
//
// `subcommand` 是受控 CLI 的子命令名，与端点路径的种类词不逐字相同（CLI 一个子命令收
// 一整批四类，在线口一类一个端点）——所以提示句里同时说清「本签收一项，不是 CLI 那份
// 整批」。这不是措辞讲究：把整批粘进来会被译装拒绝，而拒绝理由说的是形状不对，操作者
// 看不出自己错在多包了一层。
//
// 这一签的定位按 ADR-0101 决定一：JSON 快照签是受控批量口的在线镜像，不是运营配置员的
// 主路径；各册的逐字段表单由实施票逐册裁形另建。此前这里写的理由（「渠道原始载荷 →
// 登记快照」的翻译属渠道接入契约、随 PAR-INT-01 提供）被 ADR-0101 收窄为只适用客户渠道
// 载荷，对操作者面不成立，故不再这样说。
function snapshotHint(subcommand: string, fields: string): string {
  return (
    `登记快照 JSON 的键与受控登记口 parcel-commercial ${subcommand} -input 吃的同一份;` +
    '在线口收的是其中**一项**,不是整批——批不是聚合,逐项各起事务,在线口把一项作为一次请求。' +
    '本签是受控批量口的在线镜像(ADR-0101),不是运营配置员的主路径;逐字段表单按各册实施票另建。' +
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
    'publish',
    '一项的键为 tenantId / kind / objectId / version / scope / contentDigest / ' +
      'effectiveStartsAt / approval{reference,source,approvedAt} / approvalRoleStanding,' +
      '可选 effectiveEndsAt / references / declarations。kind 是**发布轴的对象类别**,封闭十词:' +
      'SERVICE_PRODUCT / CUSTOMER_CONTRACT / SUPPLIER_AGREEMENT / ACCEPTANCE_RULE_PACKAGE / ' +
      'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY / PRICE_RULE / SETTLEMENT_POLICY / ' +
      'CREDIT_POLICY / AUTHORIZATION_RULE / CUSTOMER_SERVICE_RULE。**它与本台各页的册名是两条分类轴,不逐字对应**:' +
      '前三类各显示在服务产品、客户与合同、供应商协议三页;后七类的版本与正文显示在「商业规则与策略」' +
      '页对应的册里(PRICE_RULE → 商业价格政策册,PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY → 接受前财务控制策略册——' +
      '与「接受前财务控制」册是两本:后者列的是挂在 CUSTOMER_CONTRACT 版本下的声明;CUSTOMER_SERVICE_RULE → 客户服务规则册)。' +
      '本页不代填也不校验 kind。declarations 里的通道(AS_OF_POLICY、PRE_ACCEPTANCE_CONTROL、' +
      'RULE_PACKAGE_BODY、PRICE_POLICY_BODY 等)不是 kind:它们没有自己的版本,随所属版本一并发布,' +
      '各自显示在册名旁写着的那本册。声明只能随发布登记:正文随发布固定,事后补声明等于改一份' +
      '已固定的正文,那要发新版本。',
  ),
  'business-party': snapshotHint(
    'register-parties',
    'businessParties 数组里一项的键为 partyId / name / revision / basis / effectiveFrom,' +
      '外加整批的 tenantId。首笔修订必须是 1,此后必须连续——跳号说明你看到的册面已陈旧,' +
      '会被受理门拒绝而不是替你猜。',
  ),
  'legal-entity': snapshotHint(
    'register-parties',
    'legalEntities 数组里一项的键为 legalEntityId / partyId / revision / basis / effectiveFrom,' +
      '外加整批的 tenantId。参与方必须已登记且在法人生效时点已生效——法人不钉悬空身份。',
  ),
  'customer-account': snapshotHint(
    'register-parties',
    'customerAccounts 数组里一项的键为 accountId / customerPartyId / revision / basis / ' +
      'effectiveFrom,外加整批的 tenantId。引用判据同法人登记;跨租户绑定由领域构造门拒绝。' +
      '结果显示在本页「客户账户」签(客户与合同页)。',
  ),
  'party-relationship': snapshotHint(
    'register-parties',
    'relationships 数组里一项的键为 relationshipId / revision / holder / counterparty / role / ' +
      'scope / basis / effectiveStartsAt,可选 effectiveEndsAt 与 approval{reference,approvedAt};' +
      '外加整批的 tenantId。角色取封闭五词 CUSTOMER / SUPPLIER / CARRIER_AGENT / RESELLER / ' +
      'ACCOUNT_HOLDER。缺 approval 即登记为候选关系,批准另行形成新修订。',
  ),
  'identity-deactivation': snapshotHint(
    'deactivate-party-identity',
    'deactivations 数组里一项的键为 kind / id / revision / basis / at,外加整批的 tenantId。' +
      '身份种类取封闭三词 BUSINESS_PARTY / LEGAL_ENTITY / CUSTOMER_ACCOUNT——关系不在内,' +
      '关系的终止走撤销/到期/替代,不叫停用。revision 是停用落点的修订号(册上最新 + 1):' +
      '你声明自己看到的册面,错位说明册面已被并发推进或意图已陈旧。' +
      '**法人与客户账户的停用也走本签**(一个命令带种类),结果分别显示在集团与法人页、以及' +
      '客户与合同页的「客户账户」签上。',
  ),
  'service-product-form': snapshotHint(
    'register-products',
    'forms 数组里一项的键为 productId / version / form,外加整批的 tenantId 与 scope。' +
      '服务形态取封闭两词 NETWORK_SERVICE / LABEL_CHANNEL_SERVICE(面单渠道服务已由 ADR-0088 ' +
      '纳入首发对客形态,PAR-COM-12 随之改为纳入)。' +
      '版本必须已在册且已生效——形态是解析采用的内容,挂在未生效或已收尾的版本上永远选不中。',
  ),
  'product-channel-mapping': snapshotHint(
    'register-products',
    'mappings 数组里一项的键为 mappingId / revision / productId / productVersion / channels / ' +
      'basis / effectiveStartsAt,可选 effectiveEndsAt;外加整批的 tenantId 与 scope。' +
      'channels 缺席是输入缺件,写 [] 才是登记者说出的“未配置”声明(该产品尚无可用渠道候选)——' +
      '两者不可分辨会让一句商业声明冒充一次漏填。映射标识钉着它的产品版本:改指产品是另一笔' +
      '映射,登记新映射标识,不是本映射的新修订。',
  ),
};
