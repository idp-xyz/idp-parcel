# IDP Parcel 应用用例

状态：PN-02 商业权威/接单/撤回、PN-03 初始路由/真实收寄/节点作业、PN-04 常规运输履约/逐包裹取消与终局、PN-06 通用追踪/客户视图/异常和 PN-07 通用运营结算用例已形成，关务控制域用例已有专项闭环；真实参数、联合验证和代码实现仍待按产品主线推进

本目录把已经确认的产品与领域规则组织成开发可执行的应用用例。应用用例描述参与者如何触发业务、各限界上下文如何协作、应用必须返回什么业务结果，以及如何验证实现；它不重新定义领域对象、业务规则或生命周期。

产品总体首发主线见[国际小包网络运营首发产品基线与开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)。关务 `UC-CC-001..012` 的整体完整性和控制域专项工作顺序见[关务与贸易合规专项开发基线](../product/CUSTOMS-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)。

当前用例数量不能代表产品核心优先级。`UC-NO-001`、`UC-TF-001`、`UC-VE-001` 和 `UC-SA-001` 分别只覆盖关务触发的节点、运输、异常和代垫协作；它们不能替代正常运营主链。PN-02 的 `UC-PC-001/002` 与 `UC-PS-001/002/005` 已补齐商业权威、唯一解析、接单和决定前撤回，PN-04 的 `UC-TF-003..007` 与 `UC-PS-004/006` 已补齐常规运输、收费发生事实、逐包裹取消/收寄后处置和包裹终局，PN-06 的 `UC-VE-002..008` 已补齐内部投影、普通客户追踪、ETA/缺口、异常案件、客户通知和证据索赔编排，PN-07 的 `UC-SA-002..007` 已补齐价格评价采用、调整所有权、对账纳入、供应商账单、真实收付映射、核销、经营结果和金额结算编排。

`parcel-pricing` 当前作为 PN-07 的内部纯计价前置能力，由 `UC-SA-002` 编排输入和消费评价；它不单独建立报价、合同或账务用例。后续只有在确认独立报价接受、锁价或外部计价服务责任后，才新增相应 `UC-PP-*` 用例。

PN-08 的阶段准入、暂停恢复、生产权威、对象级接管和 `Go/No-Go` 见[端到端试点与阶段准入开发交接](../design/pn-08-end-to-end-pilot-and-stage-admission-development-handoff.md)。这些是产品级试点治理记录，不是新的业务限界上下文，因此本目录不创建 `UC-GOV-*`，也不把它们写入任何委托、运输、关务或结算生命周期。

## 文档职责

应用用例拥有：

- 用例的起止边界、参与者和应用编排顺序。
- 稳定业务输入与结果语义，不绑定 API、文件或门户等传输方式。
- 业务拒绝、结果未决和技术失败的区分。
- 跨上下文调用目的、状态变化、事件语义和副作用边界。
- 幂等、并发、权限、审计和可测试验收条件。
- 阻塞真实生产实现的业务决策与实例参数清单。

应用用例不拥有：

- 领域术语、业务不变量、生命周期和数据所有权；这些仍以 [`domain`](../domain/CONTEXT-MAP.md) 下的权威文档为准。
- 客户、合同、线路、渠道、国家、字段和阈值等真实实例状态；这些由[首发试点参数与证据登记册](../product/PILOT-PARAMETER-REGISTER.md)维护。
- API、消息、文件、数据库表、事务组件或部署拓扑；它们属于后续接口与技术设计。
- 试点准入、暂停和回退等临时治理规则；用例只通过明确的试点叠加条件引用，不把它们写成长期领域事实。

## 编写约定

- 用例标识采用 `UC-<上下文缩写>-<三位序号>`，例如 `UC-PS-001`。
- 每个用例指定一个业务决定所有者；其他上下文只提供其拥有的事实或判断。
- 权威业务规则通过相对链接引用，不在用例中复制形成第二套定义。
- `已接受`和`已拒绝`是委托业务决定；依赖不可用、处理失败或尚未形成权威判断不能伪装成`已拒绝`。客户资料被权威规则确定为不完整时是否拒绝，仍由适用产品和合同规则决定。
- 未确认的业务选择以 `BD-<上下文缩写>-<三位序号>` 登记；未确认前不得被实现为固定产品规则。
- 接口字段、错误码、超时、重试次数和事件载荷在真实接入与非功能参数确认后另行定义。

## 业务决策简报

| 关联用例 | 决策简报 | 状态 |
|---|---|---|
| `UC-PS-001` | [生产接单业务决策简报](./parcel-shipment/UC-PS-001-BUSINESS-DECISION-BRIEF.md) | 八项业务机制已整体确认；真实合同、角色、规则、时点和 SLA 参数仍待登记 |

## 用例索引

| ID | 用例 | 主责上下文 | 状态 |
|---|---|---|---|
| `UC-PC-001` | [维护并发布商业权威依据](./party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md) | `party-commercial` | 商业对象独立身份、不可覆盖发布、冲突和替代边界已形成，真实来源与批准参数待提供 |
| `UC-PC-002` | [按范围与时点解析商业依据](./party-commercial/UC-PC-002-RESOLVE-COMMERCIAL-BASIS.md) | `party-commercial` | 独立商业选择锚点、唯一解析和逐项 `asOf` 两阶段机制已形成，真实锚点策略与商业版本待提供 |
| `UC-PS-001` | [客户提交国际小包请求并取得生产归属或接单结果](./parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md) | `parcel-shipment` | 接单机制已确认，真实规则与参数待提供 |
| `UC-PS-002` | [补充或更正已接受委托的客户原始资料](./parcel-shipment/UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md) | `parcel-shipment` | 资料版本、阶段边界和下游交接已形成，真实字段与授权参数待确认 |
| `UC-PS-003` | [形成有效网络收寄与正式承诺](./parcel-shipment/UC-PS-003-ESTABLISH-NETWORK-INTAKE-AND-FORMAL-COMMITMENT.md) | `parcel-shipment` | 两类来源统一采用、逐包裹责任起点和正式承诺机制已形成，真实资格与承诺参数待提供 |
| `UC-PS-004` | [依据运输责任结果形成包裹终局服务结果](./parcel-shipment/UC-PS-004-FORM-PARCEL-FINAL-SERVICE-OUTCOME.md) | `parcel-shipment` | 有效交付、退运完成或其他合同责任结果到逐包裹终局的交接已形成，真实终局规则待提供 |
| `UC-PS-005` | [撤回尚未决定的客户委托](./parcel-shipment/UC-PS-005-WITHDRAW-SUBMITTED-SHIPMENT-REQUEST.md) | `parcel-shipment` | 撤回、接受、拒绝并发决定边界及冻结释放补偿已形成，真实授权与回执参数待提供 |
| `UC-PS-006` | [取消包裹或协调收寄后服务处置](./parcel-shipment/UC-PS-006-CANCEL-PARCEL-OR-COORDINATE-POST-INTAKE-DISPOSITION.md) | `parcel-shipment` | 收寄前逐包裹取消、收寄后处置决定和下游承接/执行分层已形成，真实规则、伙伴与费用参数待提供 |
| `UC-NR-002` | [评估包裹接受前可达性](./network-routing/UC-NR-002-ASSESS-PARCEL-REACHABILITY.md) | `network-routing` | 三值判断和提交前失效重判机制已闭合，真实网络与时点参数待提供 |
| `UC-NR-001` | [为已接受包裹形成初始路由](./network-routing/UC-NR-001-CREATE-INITIAL-ROUTE.md) | `network-routing` | 稳定业务骨架已形成，真实网络与非功能参数待提供 |
| `UC-NR-003` | [按网络收寄与节点实测复核路由](./network-routing/UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md) | `network-routing` | 两个强制复核点、受控改路和无路由安全边界已形成，真实策略与参数待提供 |
| `UC-NO-001` | [承接并执行节点监管协作](./node-operations/UC-NO-001-ACCEPT-AND-EXECUTE-CUSTOMS-NODE-COLLABORATION.md) | `node-operations` | 节点承接、任务、现场事实和关务回接已形成，真实动作、授权与证据参数待提供 |
| `UC-NO-002` | [接收客户送达节点的作业实物](./node-operations/UC-NO-002-RECEIVE-CUSTOMER-DELIVERED-PARCEL.md) | `node-operations` | 客户送站节点收寄、控制和待识别实物机制已形成，真实节点与收寄参数待提供 |
| `UC-NO-003` | [执行正常节点作业、集运与封签](./node-operations/UC-NO-003-PROCESS-CONSOLIDATE-AND-SEAL-PARCELS.md) | `node-operations` | 测量、分拣、集运、快照、封签和非 WMS 边界已形成，真实设备、SOP 与规则待提供 |
| `UC-TF-001` | [承接并履行监管运输处置](./transport-fulfillment/UC-TF-001-ACCEPT-AND-FULFILL-REGULATORY-TRANSPORT-DISPOSITION.md) | `transport-fulfillment` | 监管运输承接、独立退运旅程、交接和移动事实已形成，真实路径与责任参数待提供 |
| `UC-TF-002` | [执行场外揽收并形成运输控制](./transport-fulfillment/UC-TF-002-PERFORM-OFFSITE-PICKUP.md) | `transport-fulfillment` | 任务/尝试/逐对象结果和运输控制机制已形成，真实模式、履约方与来源参数待提供 |
| `UC-TF-003` | [准备常规运输机会、班次与容量](./transport-fulfillment/UC-TF-003-PREPARE-TRANSPORT-OPPORTUNITY.md) | `transport-fulfillment` | 计划段到运输机会、班次、资源和多维容量的分层已形成，真实资源与容量参数待提供 |
| `UC-TF-004` | [形成运输委托、订舱与承运接受](./transport-fulfillment/UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md) | `transport-fulfillment` | 委托、订舱、承运接受、容量预占和运输单证分层已形成，真实伙伴与来源参数待提供 |
| `UC-TF-005` | [形成权威运输交接与实际履约](./transport-fulfillment/UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md) | `transport-fulfillment` | 逐对象交接、实际承运、实际履约段和移动事实已形成，真实交接与外部来源参数待提供 |
| `UC-TF-006` | [执行派送并形成交付证明](./transport-fulfillment/UC-TF-006-PERFORM-DELIVERY-AND-CAPTURE-POD.md) | `transport-fulfillment` | 派送任务、尝试、POD、有效交付及负向结果已形成，真实尾程与交付规则待提供 |
| `UC-TF-007` | [启动替代伙伴或退运独立旅程](./transport-fulfillment/UC-TF-007-START-ALTERNATE-OR-RETURN-JOURNEY.md) | `transport-fulfillment` | 备用切换和退运新旅程边界已形成，真实路径、伙伴和授权参数待提供 |
| `UC-CC-001` | [为已接受包裹建立关务案件](./customs-compliance/UC-CC-001-ESTABLISH-CUSTOMS-CASE.md) | `customs-compliance` | 稳定案件边界已形成，真实监管程序、角色与触发参数待提供 |
| `UC-CC-002` | [组建申报单元并形成正式申报资料](./customs-compliance/UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md) | `customs-compliance` | 单元、字段溯源与合规判断边界已形成，真实字段与规则参数待提供 |
| `UC-CC-003` | [评估申报就绪并管理提交前失效](./customs-compliance/UC-CC-003-ASSESS-DECLARATION-READINESS.md) | `customs-compliance` | 就绪门禁、逐单元判断与提交前失效边界已形成，真实程序门禁与凭证参数待提供 |
| `UC-CC-004` | [形成申报提交授权](./customs-compliance/UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md) | `customs-compliance` | 具体授权、人工/自动决定与失效边界已形成，真实角色矩阵和授权政策待提供 |
| `UC-CC-005` | [冻结申报提交版本并发起提交尝试](./customs-compliance/UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md) | `customs-compliance` | 最终门禁、不可覆盖快照、授权使用与防重复发送边界已形成，真实渠道合同和凭证占用参数待提供 |
| `UC-CC-006` | [接收、查询并分层记录申报外部结果](./customs-compliance/UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md) | `customs-compliance` | 来源保全、权威分层、原提交关联与待确认查询边界已形成，真实来源矩阵、代码映射和凭证定案参数待提供 |
| `UC-CC-007` | [管理申报后续动作与替代关系](./customs-compliance/UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md) | `customs-compliance` | 补充/更正、撤销/重报、替代层次及拟替代/有效替代边界已形成，真实程序动作和顺序门禁待提供 |
| `UC-CC-008` | [协作查验扣留并核对监管处置执行](./customs-compliance/UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md) | `customs-compliance` | 监管决定、关务协作事项、执行事实与处置核对边界已形成，真实执行方、证据和差异规则待提供 |
| `UC-CC-009` | [核对税费付款、代垫回收与放行门禁](./customs-compliance/UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md) | `customs-compliance` | 监管税费、外部资金、实际代垫/客户回收和逐动作放行门禁边界已形成，真实付款、回收和程序门禁参数待提供 |
| `UC-CC-010` | [关闭、受控重开关务案件并建立后续案件](./customs-compliance/UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md) | `customs-compliance` | 逐义务关闭、有效责任承接、受控重开和后续案件边界已形成，真实义务目录、责任来源和权限参数待提供 |
| `UC-CC-011` | [管理内部合规限制及解除](./customs-compliance/UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md) | `customs-compliance` | 内部限制形成、范围化覆盖、来源解除、当前适用性和下游消费边界已形成，真实来源、动作矩阵、解除条件和权限参数待提供 |
| `UC-CC-012` | [接收并关联承运商外部监管舱单](./customs-compliance/UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md) | `customs-compliance` | 出口和进口均由承运商在外部形成并提交的责任边界已确认，外部引用、版本、范围、关系和当前采用判断已形成，真实责任承运商、来源契约和结果语义待提供 |
| `UC-VE-001` | [协调关务异常并管理客户披露](./visibility-exception/UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md) | `visibility-exception` | 关务事实接收、异常信号/案件、处置协调、披露决定和消息结果边界已形成，真实类型、规则与渠道参数待提供 |
| `UC-VE-002` | [接收事实并形成全程追踪投影](./visibility-exception/UC-VE-002-BUILD-TRACKING-PROJECTION.md) | `visibility-exception` | 已接受事实、标准里程碑、各维度投影、冲突和未归类结果边界已形成，真实来源与映射参数待提供 |
| `UC-VE-003` | [形成 ETA 与可见性缺口判断](./visibility-exception/UC-VE-003-FORM-ETA-AND-VISIBILITY-GAP.md) | `visibility-exception` | ETA、可见性缺口和客户可见门槛已分层，真实窗口、质量和合同参数待提供 |
| `UC-VE-004` | [分诊异常信号并建立案件](./visibility-exception/UC-VE-004-TRIAGE-SIGNALS-AND-OPEN-CASES.md) | `visibility-exception` | 通用信号、发作期、分诊、案件范围和响应边界已形成，真实异常清单与响应参数待提供 |
| `UC-VE-005` | [管理异常案件并协调处置请求](./visibility-exception/UC-VE-005-MANAGE-CASES-AND-COORDINATE-ACTIONS.md) | `visibility-exception` | 案件响应、影响范围、关闭/重开/归并和跨上下文处置请求边界已形成，真实响应与授权参数待提供 |
| `UC-VE-006` | [形成客户可见异常并管理通知结果](./visibility-exception/UC-VE-006-FORM-CUSTOMER-DISCLOSURE-AND-NOTIFICATION.md) | `visibility-exception` | 客户隔离、披露决定、通知义务和消息结果分层已形成，真实合同、模板、授权与渠道参数待提供 |
| `UC-VE-007` | [管理证据、索赔项与追偿关系](./visibility-exception/UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md) | `visibility-exception` | 证据、逐项索赔、责任复核和追偿边界已形成，首发真实能力与金额仍由参数和结算上下文决定 |
| `UC-VE-008` | [提供客户普通全程追踪视图](./visibility-exception/UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md) | `visibility-exception` | 客户账户/对象授权、普通里程碑、获准 ETA、终局、异常只读展示和更正边界已形成，真实公开策略待提供 |
| `UC-SA-001` | [判定关务实际代垫并形成客户代垫回收](./settlement-accounting/UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md) | `settlement-accounting` | 实际代垫、客户责任、回收本金和调整边界已形成，真实合同、账户、币种与资金来源参数待提供 |
| `UC-SA-002` | [计算、确认并调整运营费用](./settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md) | `settlement-accounting` | 已接入 `parcel-pricing` 纯评价边界，客户/供应商计费重量采用、费用生命周期和接受前财务控制结果已形成；真实价卡、账户和控制策略待提供 |
| `UC-SA-003` | [截单、发布并处理客户对账单](./settlement-accounting/UC-SA-003-CUT-OFF-PUBLISH-AND-RECONCILE-CUSTOMER-STATEMENT.md) | `settlement-accounting` | 截单快照、对账单、金额争议和既有调整的后续账期纳入边界已形成；本用例不创建调整，真实账期与争议规则待提供 |
| `UC-SA-004` | [接收、匹配并审核供应商账单](./settlement-accounting/UC-SA-004-RECEIVE-MATCH-AND-AUDIT-SUPPLIER-BILL.md) | `settlement-accounting` | 预期成本、供应商主张、金额范围匹配和审核应付边界已形成，真实账单入口与账期待提供 |
| `UC-SA-005` | [映射外部收付款并形成运营核销](./settlement-accounting/UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md) | `settlement-accounting` | 产品内唯一真实收付采用、运营映射、未分配、部分/完整核销和撤销边界已形成，真实财务交换待提供 |
| `UC-SA-006` | [分摊成本并派生经营结果](./settlement-accounting/UC-SA-006-ALLOCATE-COSTS-AND-DERIVE-OPERATING-RESULTS.md) | `settlement-accounting` | 共享成本、未分摊余额、法人间结算和版本化经营结果边界已形成，真实分摊/毛利口径待提供 |
| `UC-SA-007` | [形成索赔、赔付与追偿金额](./settlement-accounting/UC-SA-007-SETTLE-CLAIMS-AND-RECOVERY-AMOUNTS.md) | `settlement-accounting` | 客户赔付义务、应追偿、对方认可和追加调整已分层；真实到账与核销只由 `UC-SA-005` 形成，真实金额规则待提供 |
