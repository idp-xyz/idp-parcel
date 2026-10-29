// 界面用语词典 · 状态与枚举码（改版第一步，设计源「四、用语词典」）。
//
// 代码、CONTEXT、ADR 里的领域词不动；**界面上显示的词**一律经这里翻译，一处定义、全站生效。英文枚举值不再直接上屏：
// 查不到的码在开发模式告警一次并原样显示，让漏译的码在开发时就被看见，而不是在用户面前变成一串大写字母。
//
// 中文按运营一线的叫法而不是系统内部的叫法（`SUBMITTED` 对客服是「待受理」，不是「已提交」；`REGISTERED` 的身份对配置员是
// 「待生效」）。领域文档里的原词（`已提交`、`已登记`）仍是领域语言，只是界面上换成一线的话——两套词之间的对照就是这张表。
//
// 色调按设计原则四：待处理琥珀（warn）、推进中蓝（info）、成立 / 有利终局绿（good）、失败 / 阻断红（bad）、中性或已结束灰（muted）。
// 色调是管理台的呈现判断，不是领域状态本身；也不把各源状态拼成统一状态机（GLOSSARY「追踪摘要」的约束同样管着这张表）。
//
// 同一个码在不同册里意思不同时（`REGISTERED` 的身份是待生效、作业设备是在用），按册另给一份放 scopedStatusVocabulary，
// 查时先按册、再落总表。只在意思真不同时才开一册，同义不同册的码只登总表一处。
//
// 本文件不引任何组件库，也不读环境变量：告警钩子由装配点注入（main.tsx 只在开发模式注入 console.warn），node:test 里
// 用桩钉它；测试链按 CommonJS 发射，import.meta 在这里编不过。
//
// 物流动态那一段里的里程碑词（DELIVERED / PICKED_UP…）与各源事实类型（node-intake / effective-delivery…）是开放词表，
// 里程碑目录本身是租户登记的实例参数。这里收的是随产品发布的**参考译法**：演示租户 SYN-TENANT-01 按真实租户对待
// （ADR-0150），它登记的码先译好；别的租户登了新码，页面如实露码并告警，由那张票补译，不猜。

export type StatusTone = 'good' | 'info' | 'warn' | 'bad' | 'muted';

export interface StatusEntry {
  label: string;
  tone: StatusTone;
}

const good = (label: string): StatusEntry => ({ label, tone: 'good' });
const info = (label: string): StatusEntry => ({ label, tone: 'info' });
const warn = (label: string): StatusEntry => ({ label, tone: 'warn' });
const bad = (label: string): StatusEntry => ({ label, tone: 'bad' });
const muted = (label: string): StatusEntry => ({ label, tone: 'muted' });

/** 总表：码 → 界面词与色调。 */
export const statusVocabulary: Record<string, StatusEntry> = {
  // —— 运单（委托）生命周期 · parcel-shipment ——
  SUBMITTED: warn('待受理'),
  ACCEPTED: good('已受理'),
  REJECTED: bad('已拒绝'),
  WITHDRAWN: muted('已撤回'),
  CANCELLED: muted('已取消'),
  PARCEL_CANCELLED: muted('已取消'),
  DISPOSITION_PENDING: warn('待处置'),
  // 接受判断任务：判完了（已完成）与没人再判了（已停止）是两个事实，词不合并。
  RUNNING: info('处理中'),
  COMPLETE: good('已完成'),
  COMPLETED: good('已完成'),
  STOPPED: muted('已停止'),
  UNDECIDED: warn('未决'),
  // 准入控制：OPEN 只表示没有生效暂停，不表示允许建单，所以不是 good。
  OPEN: info('开放'),
  PAUSED: warn('已暂停'),

  // —— 身份与关系 · party-commercial ——
  // 已登记但生效时点未到，对配置员就是「待生效」；候选关系是等批准，对配置员就是「待确认」。
  REGISTERED: warn('待生效'),
  EFFECTIVE: good('已生效'),
  DEACTIVATED: muted('已停用'),
  // 本公司对国家 / 地区与币种的启用范围登记（ADR-0345）：说的是“有没有登记过”，不是“能不能用”。未登记的码照常可用，
  // “未配置”这个词取总表已有的那一条；基础数据页里它占绝大多数行，页面在胶囊上另指灰色，免得满屏琥珀。
  ENABLED: good('已启用'),
  DISABLED: muted('已停用'),
  CANDIDATE: warn('待确认'),
  EXPIRED: muted('已到期'),
  // 撤销是携依据的主动终止，边界之后仍拿这段关系做决定就是错，要点出来；到期与替代是时间到了、有后继接着，中性。
  REVOKED: warn('已撤销'),
  SUPERSEDED: muted('已替代'),

  // —— 商业版本、价格表草稿与发布稿 · party-commercial / parcel-pricing ——
  DRAFT: muted('草稿'),
  VALIDATED: info('已校验'),
  PENDING_APPROVAL: warn('待批准'),
  APPROVED: good('已批准'),
  PUBLISHED: info('已发布'),
  RETIRED: muted('已退役'),

  // —— 费用与账务 · settlement-accounting ——
  // 预估、暂估是费用的正常阶段不是风险，归中性；争议与未分配等人核，琥珀。
  ESTIMATED: muted('预估'),
  PROVISIONAL: muted('暂估'),
  CONFIRMED: good('已确认'),
  SETTLED: good('已结算'),
  PARTIALLY_ACCEPTED: warn('部分接受'),
  PENDING_REVIEW: warn('待复核'),
  DISPUTED: warn('争议'),
  UNAPPLIED: warn('未分配'),

  // —— 物流动态 · 标准里程碑（租户登记的实例参数，这里是参考译法，见文件头）——
  PICKED_UP: good('已揽收'),
  IN_TRANSIT: info('运输中'),
  DEPARTED: info('已发出'),
  ARRIVED: info('已到达'),
  OUT_FOR_DELIVERY: info('派送中'),
  DELIVERED: good('已签收'),
  DELIVERY_FAILED: bad('派送失败'),
  CUSTOMS_SUBMITTED: info('已申报'),
  CUSTOMS_HOLD: warn('海关查验'),
  CUSTOMS_MOVEMENT: info('关区转运'),
  CUSTOMS_CLEARED: good('已放行'),
  EXCEPTION: bad('异常'),
  // 终局责任结果（parcel-shipment 终局规则的四值）：有效送达对一线就是签收。
  EFFECTIVE_DELIVERY: good('已签收'),
  RETURN_COMPLETED: muted('已退回'),
  SERVICE_TERMINATED: bad('服务终止'),
  REGULATORY_DISPOSITION: bad('监管处置'),

  // —— 物流动态 · 各源事实类型（源上下文拥有的开放词表，参考译法）——
  'final-outcome-adopted': good('已完结'),
  'initial-route-formed': info('已规划路线'),
  'route-reassessed': info('路线已复核'),
  'node-intake': good('已收寄'),
  // 节点间交出是路上的一步（蓝），与运输的权威交接结果「已交接」（绿、成立）不是一个词，免得同词异色。
  'node-handover': info('已交出'),
  'effective-delivery': good('已签收'),
  'declaration-submitted': info('已申报'),
  'external-result-release-full': good('已放行'),
  'external-result-release-partial': warn('部分放行'),
  'external-result-release-conditional': warn('附条件放行'),
  'transport-departed': info('已发出'),
  'transport-arrived': info('已到达'),
  // 异常动线上客户拒收后形成退运的那条事实（演示租户四段动线之一）。
  'exception-journey-return': warn('退运中'),

  // —— 清关 · customs-compliance ——
  FULL: good('全部放行'),
  PARTIAL: warn('部分放行'),
  CONDITIONAL: warn('附条件放行'),
  CLEARED: good('已放行'),
  HELD: warn('查验中'),
  REGISTERED_PENDING_SEND: warn('待发送'),
  ACKNOWLEDGED: good('已确认'),
  FAILED: bad('失败'),
  PENDING_CONFIRMATION: warn('待确认'),
  PENDING: warn('待确认'),
  MET: good('满足'),
  UNMET: bad('未满足'),
  CONFLICTING: bad('冲突'),
  // 放行检查的准入答案：ADMITTED 对一线就是「可放行」；NOT_GUARDED 是本就不用检查，不是没检查。
  NOT_GUARDED: muted('无需检查'),
  ADMITTED: good('可放行'),
  NOT_CONFIGURED: warn('未配置'),
  BLOCKED: bad('已拦截'),
  CONCLUDED: muted('已结束'),
  HANDED_OVER: good('已交接'),
  UNRESOLVED: warn('未解决'),
  NONE: muted('无覆盖'),
  COVERED: good('已覆盖'),
  NO_DELTA: good('无差额'),
  SHORT: bad('不足'),
  EXCESS: warn('超额'),
  VALID: good('有效'),
  INVALIDATED: muted('已失效'),

  // —— 运输与网点 · transport-fulfillment / node-operations / network-routing ——
  REFUSED: bad('已拒绝'),
  INTAKE_FORMED: good('已收寄'),
  PENDING_IDENTIFICATION: warn('待识别'),
  SEALED: info('已封装'),
  CLOSED: muted('已关闭'),
  CURRENTLY_EFFECTIVE: good('当前有效'),
  LAPSED: muted('已失效'),

  // —— 异常与索赔 · visibility-exception ——
  AWAITING_RESPONSE: warn('待响应'),
  IN_PROGRESS: info('处理中'),
  SUPPLEMENT_REQUIRED: warn('要求补充'),
  AUTO_ESTABLISH: info('自动立案'),
  ATTACH_TO_EXISTING: info('关联既有案件'),
  MANUAL_REVIEW: warn('进入人工复核'),
  NO_CASE: muted('不立案'),
  ELIGIBLE: good('符合条件'),
  INELIGIBLE: bad('不符合'),
  AWAITING_SUPPLEMENT: warn('等待补充材料'),
  FULLY_ESTABLISHED: good('全部成立'),
  PARTIALLY_ESTABLISHED: warn('部分成立'),
  NOT_ESTABLISHED: bad('不成立'),
  UNDETERMINABLE: warn('无法认定'),
  PREPARED: muted('已准备'),
  CHANNEL_ACCEPTED: info('渠道已接受'),
  SUBMISSION_FAILED: bad('提交失败'),
  SHOWN: good('展示'),
  NOT_DISCLOSED: muted('不披露'),
  CANCELLATION_ACCEPTED: good('取消已接受'),
  PARTIALLY_CANCELLED: warn('部分取消'),
  NO_LONGER_CANCELLABLE: muted('已不可取消'),
  CANCELLATION_REFUSED: bad('取消被拒'),

  // —— 计价与渠道择优 · parcel-pricing / parcel-shipment ——
  CONFLICT: bad('冲突'),
  UNRATABLE: bad('不可计价'),
  SELECTED: good('选中'),
  NOT_SELECTED: muted('落选'),
  EXCLUDED: muted('出局'),
  TIED: warn('并列'),

  // —— 账务 · settlement-accounting 的其余封闭集（费用页、对账单页、收付款页、经营核算页）——
  // 确认条件目录里「要哪一种依据」：词取各上下文的事实名。
  EFFECTIVE_DELIVERY_FINALIZED: muted('签收终局'),
  NODE_INTAKE_RECORDED: muted('网点收寄'),
  // 供应商预期成本的收费发生原因。
  ACTUAL_FULFILLMENT: muted('实际履约'),
  PLANNED_FULFILLMENT: muted('计划履约'),
  CORRECTION: warn('纠错'),
  // 外部资金事实三种：只有到账是收到了钱；付款失败与退回都要人去看。
  RECEIPT_CONFIRMED: good('已到账'),
  PAYMENT_FAILED: bad('付款失败'),
  FUNDS_RETURNED: warn('资金退回'),
  // 收付方向（是收付不是借贷）。
  RECEIVABLE: muted('应收'),
  PAYABLE: muted('应付'),
  // 后续账期纳入的来源。
  ADJUSTMENT: muted('调整'),
  LATE_CHARGE: warn('迟到费用'),
  // 经营结果组成项的增减向与角色（ADR-0087 决定三按口径分组的采用物）。
  INCREASES: good('增加'),
  DECREASES: bad('减少'),
  CUSTOMER_ESTIMATE: muted('客户预估费用'),
  SUPPLIER_EXPECTED_COST: muted('供应商预期成本'),
  CUSTOMER_OPERATING_RECEIVABLE: muted('客户应收'),
  AUDITED_PAYABLE: muted('审核应付'),
  SUPPLIER_CREDIT_NOTE: muted('供应商贷项'),
  SETTLED_CUSTOMER_RECEIVABLE: muted('已核销客户应收'),
  SETTLED_AUDITED_PAYABLE: muted('已核销审核应付'),
  SETTLED_SUPPLIER_CREDIT_NOTE: muted('已核销供应商贷项'),
  // 资金可核到的目标。
  STATEMENT: muted('客户对账单'),
  CREDIT_NOTE: muted('供应商贷项'),
  ACCOUNT: muted('结算账户'),
  CUSTOMER_COMPENSATION: muted('客户赔付或退款'),
  RECOVERY_ACKNOWLEDGEMENT: muted('追偿认可金额'),
  COLLECTED_PRINCIPAL: muted('代收货款本金'),

  // —— 代收货款 · collection-remittance 回汇批次 ——
  COLLECTED: info('已归集'),
  HANDED_FOR_PAYMENT: good('已交付汇付'),

  // —— 清关 · customs-compliance 的其余封闭集（清关单页、合规限制与税费页、口岸路径页、规则库页）——
  IMPORT: muted('进口'),
  EXPORT: muted('出口'),
  // 受放行检查约束的动作四值，词取 CONTEXT 硬句逐词。
  OUTBOUND_RELEASE: muted('出库'),
  LOADING_DEPARTURE: muted('装载出发'),
  CROSS_CUSTOMS_MOVEMENT: muted('跨关区移动'),
  FINAL_DELIVERY: muted('交付'),
  // 放行检查问的是哪一步动作。
  DEPARTURE: muted('出发'),
  HANDOVER: muted('交出'),
  DELIVERY: muted('交付'),
  // 跨关区判断。
  NOT_ASKED: muted('未问'),
  SAME_ZONE: muted('同关区'),
  CROSSING: info('跨关区'),
  UNDETERMINED: warn('未能判定'),
  // 海关结果六层（监管接收、业务受理、过程决定、核定税费、放行结果、处置决定）。
  REGULATORY_RECEIPT: info('海关已接收'),
  BUSINESS_ACCEPTANCE: info('海关已受理'),
  PROCESS_DECISION: info('海关过程决定'),
  ASSESSED_DUTY: warn('核定税费'),
  RELEASE_RESULT: good('放行结果'),
  DISPOSITION_DECISION: bad('海关处置决定'),
  // 税费付款的义务依据：明确无需付款是一条带依据的结论，不是「没有结果所以不用付」。
  EXPLICITLY_NOT_REQUIRED: muted('明确无需付款'),
  // 清关单的最新进展（页面按已有事实取最靠后的一层，见 pages/customs/record-rows.ts 的 caseProgress）。
  CUSTOMS_PENDING_DECLARATION: warn('待申报'),
  DECLARATION_PENDING_SEND: warn('待发送'),
  DECLARATION_SENT: info('已发送'),
  DECLARATION_SEND_FAILED: bad('发送失败'),
  CUSTOMS_RELEASED: good('已放行'),
  CUSTOMS_CASE_CLOSED: muted('已关案'),

  // —— 扫描设备与试点治理的作业范围种类 ——
  NODE: muted('网点'),
  OFFSITE: muted('场外'),

  // —— 试点治理 · 生产权威区间与接管上的能力与事实种类（ADR-0083；码取各命令面的事实名，词按一线叫法）——
  OPERATION_DECISION: muted('运营决定'),
  WORK_FACT_REGISTRATION: muted('作业事实登记'),
  CUSTOMER_COMMAND: muted('客户指令'),
  EXTERNAL_FACT: muted('外部事实'),
  SHIPMENT_REQUEST_SUBMISSION: muted('新建运单'),
  SHIPMENT_REQUEST_SUPPLEMENT: muted('补充运单资料'),
  SHIPMENT_REQUEST_WITHDRAWAL: muted('撤回运单'),
  SOURCE_DATA_AMENDMENT: muted('修改来源资料'),
  MANUAL_REVIEW_COMPLETION: muted('人工复核完成'),
  ACTIVE_REJECTION: muted('主动拒收'),
  AUTHORIZED_DISPOSITION: muted('授权处置'),
  AUTHORIZED_REROUTE: muted('授权改路'),
  ALTERNATE_JOURNEY_START: muted('启用备选路线'),
  PARCEL_CANCELLATION: muted('取消运单'),
  POST_INTAKE_SERVICE_DISPOSITION: muted('收寄后处置'),
  POST_INTAKE_MANUAL_ITEM_HANDLING: muted('收寄后人工处理'),
  IDENTITY_CONFLICT_DISPOSITION: muted('身份冲突处置'),
  PENDING_IDENTITY_RECHECK: muted('待识别复核'),
  CONTROL_RELEASE_REFUSAL_HANDLING: muted('释放拒收处理'),
  NODE_COLLABORATION_ACCEPTANCE: muted('网点协作受理'),
  NODE_COLLABORATION_EXECUTION: muted('网点协作执行'),
  DISPATCH_TASK_REGISTRATION: muted('派送任务登记'),
  DELIVERY_DISPATCH_REASK: muted('重新派送'),
  DELIVERY_ATTEMPT_RESULT_CONFIRMATION: muted('派送结果确认'),
  PICKUP_ATTEMPT_RESULT_CONFIRMATION: muted('揽收结果确认'),
  DELIVERY_HANDOVER_CONFIRMATION: muted('交付交接确认'),
  DELIVERY_HANDOVER_NOTE_OPENING: muted('交接单开立'),
  DELIVERY_HANDOVER_NOTE_ASSIGNMENT: muted('交接单分派'),
  DELIVERY_HANDOVER_NOTE_CLOSURE: muted('交接单关闭'),
  DELIVERY_PRE_ADVICE_SUBMISSION: muted('预报提交'),
  DELIVERY_PRE_ADVICE_WITHDRAWAL: muted('预报撤回'),
  CARRIER_FIRST_EFFECTIVE_PICKUP_JUDGMENT: muted('承运商首次有效揽收判断'),
  CARRIER_BOOKING_ANSWER: muted('承运商订舱答复'),
  BOOKING_REQUEST_SUBMISSION: muted('订舱申请'),
  BOOKING_REQUEST_CANCELLATION: muted('订舱取消'),
  CAPACITY_RESERVATION: muted('舱位预留'),
  CAPACITY_RELEASE: muted('舱位释放'),
  LOAD_ASSIGNMENT: muted('装载分派'),
  SEGMENT_CLOSURE: muted('运段关闭'),
  TRANSPORT_COMMISSION_SUBMISSION: muted('运输委托提交'),
  TRANSPORT_COMMISSION_CANCELLATION: muted('运输委托取消'),
  TRANSPORT_COMMISSION_AGREEMENT_REVISION: muted('运输委托协议修订'),
  FULFILLMENT_SEGMENT_COMMISSION_ADOPTION: muted('运段委托采用'),
  FULFILLMENT_SEGMENT_COMMISSION_ADOPTION_WITHDRAWAL: muted('运段委托采用撤回'),
  TRANSPORT_CHARGE_OCCURRENCE_REGISTRATION: muted('运输收费发生登记'),
  TRANSPORT_CHARGE_OCCURRENCE_INVALIDATION: muted('运输收费发生作废'),
  EFFECTIVE_TIME_JUDGMENT: muted('外部轨迹时间确认'),
  PARTICIPATION_TERMINATION: muted('参与终止'),
  CONTROLLED_CLOSURE: muted('受控关闭'),
  CONTROLLED_REOPENING: muted('受控重开'),
  CUSTOMS_CASE_ESTABLISHMENT: muted('立清关单'),
  CUSTOMS_CASE_CLOSURE: muted('清关单关案'),
  CUSTOMS_DECLARATION_SUBMISSION: muted('报关申报'),
  CUSTOMS_SUBMISSION_AUTHORIZATION: muted('申报授权'),
  CUSTOMS_EXTERNAL_RESULT: muted('海关结果'),
  INTERNAL_RESTRICTION_ESTABLISHMENT: muted('建立内部限制'),
  INTERNAL_RESTRICTION_DECISION: muted('内部限制决定'),
  RELEASE_GATE_VERIFICATION: muted('放行检查'),
  REGULATORY_CREDENTIAL: muted('监管凭证'),
  REGULATORY_CREDENTIAL_REGISTRATION: muted('监管凭证登记'),
  REGULATORY_DISPOSITION_ACCEPTANCE: muted('海关处置接受'),
  CHARGE_CONFIRMATION: muted('费用确认'),
  STATEMENT_PUBLICATION: muted('对账单发布'),
  SUPPLIER_BILL_RECEPTION: muted('供应商账单接收'),
  SUPPLIER_BILL_AUDIT: muted('供应商账单审核'),
  EXTERNAL_FUNDS_FACT: muted('外部收付款'),
  EXTERNAL_FUNDS_FACT_ADOPTION: muted('收付款登记'),
  CLAIM_SUBMISSION: muted('索赔提交'),
  // —— 接受判断(复核、授权处置页的已记录判断)· parcel-shipment ——
  REACHABLE: good('可送达'),
  UNREACHABLE: bad('不可送达'),
  INSUFFICIENT_EVIDENCE: warn('依据不足'),
  NOT_APPLICABLE: muted('不适用'),
  SATISFIED: good('已通过'),
  RESTRICTED: warn('受限'),
  ALL_CONTROLS_PASS: good('全部通过'),
  CREDIT_EXPOSED: warn('信用敞口'),
  PREPAID_FREEZE: info('预付冻结'),
  CREDIT_CHECK: info('信用检查'),
  CREDIT_LIMIT_EXCEEDED: bad('超出信用额度'),
  RECORDED: good('已记录'),
  FORMED: good('已形成'),
  ALREADY_COMPLETED: muted('已有记录'),
  COMMERCIAL_BASIS_UNAVAILABLE: warn('商务依据暂不可用'),

  // —— 面单交易(交易级结果与后续动作)· parcel-shipment ——
  ESTABLISHED: info('已建立'),
  SUBMITTED_TO_CHANNEL: info('已提交渠道'),
  RESULT_UNCERTAIN: warn('结果待确认'),
  SUCCEEDED: good('成功'),
  PARTIALLY_SUCCEEDED: warn('部分成功'),
  CHANNEL_VOID: muted('渠道作废'),
  CHANNEL_REFUND: muted('渠道退款'),
  REPLACEMENT: info('替代'),
  RETRY: info('重试'),
};

/**
 * 分册覆盖：同一个码在这几本册里意思与总表不同。册名是页面自己起的呈现分组名，不是限界上下文名——
 * 一本册里的码只在那本册里这么读。
 */
export const scopedStatusVocabulary: Record<string, Record<string, StatusEntry>> = {
  // 作业设备册：REGISTERED 是在用、不是等生效（设备没有生效时点）。
  'work-device': { REGISTERED: good('在用'), DEACTIVATED: muted('已停用') },
  // 价格评价：PENDING 等的是下游判断，FAILED 是评价没形成。
  'pricing-evaluation': { PENDING: warn('待判断'), FAILED: bad('未形成') },
  // 追偿动作的过程节点：SUBMITTED 是向对方提交了，DELIVERED 是送达对方，ACKNOWLEDGED 是对方确认。
  'claim-recovery': { SUBMITTED: info('已提交'), DELIVERED: good('已送达'), ACKNOWLEDGED: good('对方已确认') },
  // 权威运输交接结果：REFUSED 是收货方拒收这一票实物。
  handover: { REFUSED: bad('已拒收') },
  // 关务关闭义务：HANDED_OVER 是义务交由他方承接，CONCLUDED 是义务终结。
  'customs-obligation': { HANDED_OVER: good('已承接'), CONCLUDED: muted('已终结') },
  // 技术回执层：ACKNOWLEDGED / FAILED 说的是报文有没有送达，不是业务受理。
  'customs-receipt': { ACKNOWLEDGED: good('技术成功'), FAILED: bad('技术失败') },
  // 集运单元阶段：OPEN 是还能往里装。
  consolidation: { OPEN: info('开放装入') },
  // 税费覆盖：PARTIAL 是部分覆盖，不是部分放行。
  'duty-coverage': { PARTIAL: warn('部分覆盖') },
  // 对账单异议的裁定：ACCEPTED 是异议被接受（客户说得对），不是运单受理。
  dispute: { ACCEPTED: good('接受'), REJECTED: bad('拒绝') },
  // 资金核到的目标种类：PAYABLE 是供应商审核应付这一本，不是收付方向。
  'funds-target': { PAYABLE: muted('供应商审核应付') },
  // 基础数据目录里的封闭词（ADR-0335）：大洲与计量单位类别。COUNT、VOLUME 这类通用词只在这两本册里取这个意思，不进总表。
  'refdata-region': {
    ASIA: muted('亚洲'),
    EUROPE: muted('欧洲'),
    AFRICA: muted('非洲'),
    NORTH_AMERICA: muted('北美洲'),
    SOUTH_AMERICA: muted('南美洲'),
    OCEANIA: muted('大洋洲'),
    ANTARCTICA: muted('南极洲'),
  },
  // 计价费用代码（parcel-pricing 的「计价费用代码」）：开放词表，由价卡声明；这里收随产品发布的参考译法，演示租户
  // 按真实租户对待（ADR-0150），它价卡上的码先译好，别的租户的新码如实露码并告警，由那张票补译，不猜。
  'charge-code': {
    BASE_FREIGHT: muted('基础运费'),
    FUEL_SURCHARGE: muted('燃油附加费'),
    REMOTE_AREA_SURCHARGE: muted('偏远地区费'),
    REMOTE_AREA_EXTENDED_SURCHARGE: muted('偏远地区费（二档）'),
    OVERSIZE_SURCHARGE: muted('超尺寸费'),
    OVERWEIGHT_SURCHARGE: muted('超重费'),
    RESIDENTIAL_DELIVERY_SURCHARGE: muted('住宅派送费'),
    MINIMUM_CHARGE: muted('最低收费'),
  },
  'refdata-unit-category': {
    WEIGHT: muted('重量'),
    LENGTH: muted('长度'),
    VOLUME: muted('体积'),
    COUNT: muted('件数'),
  },
  // 接受前财务控制的结果:HELD 是占用已成立,不是查验。
  'financial-control': { HELD: good('已占用'), RESTRICTED: warn('受限'), NOT_APPLICABLE: muted('不适用'), CREDIT_EXPOSED: warn('信用敞口') },
  // 接受判断任务停在哪一步等谁(ResumePath):词说「等谁」,不说分诊结论。
  'resume-path': {
    MANUAL_REVIEW: warn('等人工复核'),
    AUTHORIZED_DISPOSITION: warn('等授权处置'),
    CUSTOMER_SUPPLEMENT: warn('等客户补充'),
    INTERNAL_RETRY: info('系统重试中'),
    OPERATOR_REGISTRATION: warn('等运营登记'),
  },
};

type UnknownCodeHandler = (code: string) => void;

let warnUnknown: UnknownCodeHandler | null = null;
const warned = new Set<string>();

/**
 * 装配点注入「查不到的码」的告警去处（开发模式给 console.warn，生产不注入）；传 null 关掉。每个码只告警一次——
 * 同一个码在一张表里出现几十次，警几十遍只会把真正的新码淹掉。
 */
export function configureVocabularyWarnings(handler: UnknownCodeHandler | null): void {
  warnUnknown = handler;
  warned.clear();
}

/** 码的界面词与色调；查不到的码原样作词、色调中性，并按文件头说的告警一次。 */
export function statusEntryFor(code: string, scope?: string): StatusEntry {
  const scoped = scope !== undefined ? scopedStatusVocabulary[scope]?.[code] : undefined;
  const entry = scoped ?? statusVocabulary[code];
  if (entry !== undefined) return entry;
  if (warnUnknown !== null && !warned.has(code)) {
    warned.add(code);
    warnUnknown(code);
  }
  return { label: code, tone: 'muted' };
}

export function labelFor(code: string, scope?: string): string {
  return statusEntryFor(code, scope).label;
}

/** 计价费用代码的界面词；查不到的码原样作词并告警一次（与状态码同一机制）。 */
export function chargeCodeLabel(code: string): string {
  return labelFor(code, 'charge-code');
}

export function toneFor(code: string, scope?: string): StatusTone {
  return statusEntryFor(code, scope).tone;
}

/**
 * 按界面词反查色调，给只拿得到词、拿不到码的格用（行模型为了能在 node:test 里跑只交字符串）。同一个词被几个码共用时
 * 它们的色调一致——这是词典自己守的约定（status.test.ts 钉着），所以取第一个命中的就够。查不到答中性。
 */
export function toneForLabel(label: string): StatusTone {
  for (const entry of Object.values(statusVocabulary)) {
    if (entry.label === label) return entry.tone;
  }
  for (const scoped of Object.values(scopedStatusVocabulary)) {
    for (const entry of Object.values(scoped)) {
      if (entry.label === label) return entry.tone;
    }
  }
  return 'muted';
}

/** 总表收录的全部码，给测试与工具数覆盖面用。 */
export const knownStatusCodes: readonly string[] = Object.keys(statusVocabulary);
