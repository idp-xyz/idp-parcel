# 31 骨架页的下一批可接线前沿(只读调研)

Category: chore
Status: interrupted(票 02 解锁即中断,断点见文末)

盘于 HEAD `59d79b4`(2026-08-24 晚)。只读调研,未改任何现有文件未写代码。
派单:MCP-1(低优先级,票 02 抢占位)。中断时点:`c6c7914`(票 01 ADR-0076)落库,
票 02 解锁,按硬约束让位。

## 与两份既有盘点的关系(delta 声明)

- `.scratch/port-inventory-r25/report.md`:Status 已 superseded(盘于 `d40b03a`,被产品
  基线 `bfb2063` 一轮覆盖)。本文不重数端口,只复用其「判据 A 虚高/提供方表面缺」的
  方法论词汇。
- `.scratch/syn-wall-door-audit/report.md`(基线 `49a2ab0`,2026-08-20):**大面积已过时,
  引用前必须按本节 delta 修正**。当时判「无门/半门」的墙,以下已开门:
  - **六个登记 CLI 已落**:`cmd/parcel-customs-register`(案件配置+案件要求规则,W13)、
    `cmd/parcel-ve-register`(里程碑映射/分诊/通知/索赔资格/索赔授权/披露六目录,
    W16–W18)、`cmd/parcel-network-register`(网络目录)、`cmd/parcel-governance-register`
    (治理登记,W03 的登记侧)、`cmd/parcel-pricing-register`(价卡目录+参考序列,
    W14/W15 的存储与登记口——迁移 parcel_pricing/0002、0003 已在,审计时「无表无仓储」
    已不成立)、`cmd/parcel-commercial`(商业发布批次)。
  - **parcel-api 十端点全部接真编排/真读口**(90c7742 收官),审计所记「unwired* 编排桩」
    只余装配测试在用。
  - **仍然成立的**:W01/W02 接入认证与渠道无门(PAR-INT-01)——所有端点挂
    「未配置即拒」Intake,这是一切页面接线的共同深度上限:前端发请求、如实渲染
    403 未配置态(与已接线三页同档,ve-operations-tracking-read/spec.md「深度上限」)。

## 现状底数

35 个导航模块条目(不含工作台):已接线 3(shipment-request、shipment-request-inquiry、
cancel-parcel)+ 演示 1(template-preview)+ 骨架 31。tracking-projection 接线在途
(本 scratch 四票,将成第 4),列入下表但批次标「在途」。

## 接线的通用模式(票 02 同款四件套)

一页骨架转接线 = ports 列表读面 + adapters/postgres 读适配器 + adapters/http 查询
端点 + cmd/parcel-api 装配,再加前端(api.ts 收编形状、presentation 词表、页面表格)。
判「能不能接」看两件:**底层存储在不在**(表+写入方),**数据从哪来**(事件链已接
调度器=有真数据流;登记 CLI=租户登记后有;都没有=接了恒空)。

## 逐模块

批次口径:
- **A** = 存储+数据产生链都真(事件驱动写入方已接调度器或端点),照四件套即可接;
- **B** = 存储+登记门(CLI)真,数据属实例半边待租户登记;查阅面机制可现在建,
  未登记时如实空,与 A 同构;
- **C** = 查阅面之前还缺上游机制,先补机制再谈查阅;
- **阻断** = 本体机制未开工或关键件待核实。

| 模块 id | 主责上下文 | 存储/读面现状(盘于 59d79b4) | 缺件与半边归类 | 批次 |
|---|---|---|---|---|
| group-legal-entities | party-commercial | 无法人/集团本体表(PC 14 张迁移全是发布/解析/授权/声明类) | 本体机制未开工(建模先行) | 阻断 |
| business-parties | party-commercial | 无参与方本体表 | 同上 | 阻断 |
| party-contracts | party-commercial | 仅 customer_contract_content(0012,接受判断消费的内容声明),无合同本体生命周期表 | 本体机制未开工;拿声明表冒充合同本体会造第二套口径 | 阻断 |
| supplier-agreements | party-commercial | 无供应商协议表 | 同上 | 阻断 |
| service-products | party-commercial | service_product_form(0008)在,有仓储写入方 | 缺列表读面+端点;登记进程口归 cmd/parcel-commercial 范围待核 | B |
| channel-product-catalog | party-commercial | 未见专表;疑在 commercial_publication/commercial_version 的 kind 维内 | 存储覆盖待核(断点) | 待核 |
| commercial-policies | party-commercial | acceptance_rule_package(0014)/price_policy(0010)/settlement_policy(0011)/pre_acceptance_control(0007)/as_of_policy(0005)在;发布走 cmd/parcel-commercial | 缺列表读面+端点 | B |
| price-card-catalog | parcel-pricing | price_card_catalog 表(0002)+登记用例+CLI 全在 | 缺列表读面+端点 | B |
| reference-series | parcel-pricing | reference_series_register 表(0003)+登记用例+CLI 全在 | 缺列表读面+端点 | B |
| network-catalog | network-routing | network_catalog 表(0008)+CLI(parcel-network-register)在 | 缺列表读面+端点 | B |
| service-areas | network-routing | network_definition 表(0007)在;审计记「无写入方,生产恒答未配置」,parcel-network-register 是否覆盖此表未核(断点) | 写入方待核;若仍无写入方,接线恒空 | 待核 |
| customs-ports-paths | customs-compliance | 规划占位页;口岸/申报路径是否已被 case_config_registry 覆盖未核(断点) | 存储覆盖待核 | 待核 |
| compliance-rules | customs-compliance | 五配置只读视图+case_config/requirement registry+CLI(parcel-customs-register)在 | 缺列表读面+端点 | B |
| acceptance-review | parcel-shipment | acceptance_judgments(判断记录)+shipment_request(任务态)在,数据由接受链产生 | 缺复核队列读面+端点;「复核动作」写端点另议,查阅面可先行 | A |
| label-transactions | parcel-shipment | 面单交易机制零代码(全包 grep 无 LabelTransaction) | 机制未开工(渠道适配 PAR-INT-02 亦未登记) | 阻断 |
| pricing-evaluation | parcel-pricing | evaluation 表(0001)+EvaluationStore 在;但 evaluate_pricing 未接任何进程(parcel-dispatch 装配零 pricing 字样) | 先接评价编排(机制),否则查阅面恒空;价卡实例另属 B 线 | C |
| route-plans | network-routing | initial_route/route_reassessment/route_judgments/plan_applicability 在,NR 消费者已接调度器 | 缺列表读面+端点 | A |
| node-operations-review | node-operations | reception/consolidation/collaboration 在,收寄端点+调度器已接 | 缺列表读面+端点 | A |
| transport-fulfillment-review | transport-fulfillment | transport_schedule/capacity_pool/handover_registry/effective_delivery/delivery_attempt/booking 在,交付双端点已接 | 缺列表读面+端点 | A |
| customs-cases | customs-compliance | declaration_unit_store/declaration_submission/customs case 链在,外部结果端点已接 | 缺列表读面+端点 | A |
| customs-restrictions | customs-compliance | case_restriction_gate/disposition_verification 在 | 缺列表读面+端点 | A |
| tracking-projection | visibility-exception | 投影库在;运营读口即本 scratch 票 02(ADR-0076 已落) | 在途 | 在途 |
| exception-triage | visibility-exception | signal_episode/triage 链/disposition_request 在;分诊规则 CLI 已开 | 缺列表读面+端点 | A |
| exception-cases | visibility-exception | active_case 在 | 缺列表读面+端点 | A |
| claims-recovery | visibility-exception | claim_recovery 在,索赔端点已接 | 缺列表读面+端点 | A |
| charges-billing | settlement-accounting | customer_charge/charge_confirmation_condition/cost_allocation 在;审计对照组记全链业务事实写入方已接调度器 | 缺列表读面+端点 | A |
| reconciliation | settlement-accounting | customer_statement/statement_dispute 在 | 缺列表读面+端点 | A |
| settlement-application | settlement-accounting | settlement_application/funds_mapping/external_funds_fact 在 | 缺列表读面+端点 | A |
| operating-metrics | settlement-accounting | operating_result/operational_position 在 | 缺列表读面+端点 | A |
| cod-ledger | collection-remittance | 上下文零代码(internal 下无任何 collectionremittance) | 上下文未开工 | 阻断 |
| stage-admission | pilotgovernance | governance_records/incident_records/scope_version_relations+register_authority_interval 用例+CLI 在 | 缺查阅读面+端点(治理查阅不在 parcel-api 十端点内,挂载位置需一并裁) | B |

## 批次汇总与一句建议

- **A(存储+数据链真,12 页)**:acceptance-review、route-plans、node-operations-review、
  transport-fulfillment-review、customs-cases、customs-restrictions、exception-triage、
  exception-cases、claims-recovery、charges-billing、reconciliation、settlement-application、
  operating-metrics(13 计入 A 者以表为准)。建议按上下文分票(每票一个上下文的
  读面+端点+装配+前端),复用票 02 的 ADR-0076 作用域模型先例——每票都要过
  「运营查阅作用域」的同类裁决,建议各上下文一篇小 ADR 或引用 0076 的通例。
- **B(登记门真、数据待租户登记,6 页)**:service-products、commercial-policies、
  price-card-catalog、reference-series、network-catalog、compliance-rules、stage-admission。
  接线价值:操作员能看登记结果,未登记如实空;与 A 同构可混批。
- **C(1 页)**:pricing-evaluation——先立「评价编排接进程」的机制票。
- **阻断(6 页)**:PC 本体四页(group-legal-entities、business-parties、party-contracts、
  supplier-agreements)、label-transactions、cod-ledger——都是上下文/聚合机制未开工,
  属建模先行,不是接线票能解的。
- **待核(3 页,断点)**:channel-product-catalog(存储覆盖)、service-areas(写入方)、
  customs-ports-paths(存储覆盖)。

## 断点(中断于票 02 解锁)

1. channel-product-catalog:commercial_publication/commercial_version 的 kind 枚举是否
   含渠道产品目录——看 internal/partycommercial/domain 的版本 kind 定义即可核。
2. service-areas:cmd/parcel-network-register 的命令面是否写 network_definition 表
   ——看该 CLI main.go 的命令清单即可核。
3. customs-ports-paths:case_config_registry 的配置维是否覆盖口岸与申报路径
   ——看 RegisterCaseConfiguration 的命令形状即可核。
4. SA 四页「数据链已接调度器」引自审计对照组,未逐消费者复核(A 批开工时顺手核)。
5. 批次 A 逐页的 CONTEXT 查阅语义(哪些维度上列)未做——那属各接线票的词汇对照步。
