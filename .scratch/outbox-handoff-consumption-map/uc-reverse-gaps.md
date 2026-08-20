# UC 正向反查:文档要求跨上下文交付、发布侧连 handoff 都没有的反向缺口

- 任务:`task-b50db94f-c072-4323-b354-a3b3b17ab0a9`(MCP-4,只读;T2 清点收尾第三件)
- 取证基线:origin/main `3b9f212`(detached 树)
- 方法:遍历 `docs/application/` 全部 50 份 `UC-*`(PS7+NR3+NO3+TF7+CC12+SA7+VE8+PC3,parcel-pricing 与 collection-remittance 无 UC)与 CONTEXT-MAP 边清单,反向对照发布侧代码(46 个 Outbox handoff 的清单以 [report.md](./report.md) 为准,另核各上下文 `ports/`)。判据只取 CONTEXT-MAP / CONTEXT / UC 三类权威文档,不靠命名相似推断。
- 与 report.md 的方向相反:那张表答「发布面在、消费者缺不缺」,本表答「文档要求交付、**发布面本身缺不缺**」。「本就不该有」是结论,列在末节,不算缺口。

## 反向缺口五条

### 1. CC → SA:税费、付款核对与代垫责任输入,无任何发布口

- **UC 要求**:UC-CC-009 目标句——「把付款方、税费、付款核对和合同责任输入**交给 `settlement-accounting`** 独立判断实际代垫及客户代垫回收」;其结果语义专设「**客户回收输入交接结束**:结算已接收付款方、监管核定、外部资金事实引用、责任输入和当前核对」;参与者表:「`customs-compliance`:形成税费付款协作、付款核对、放行门禁核对和**跨上下文交接**」。消费端 UC-SA-001:「接收并独立校验关务税费、付款核对、外部资金事实……不把交接成功当成业务成立」;输入契约「监管税费义务 ← `customs-compliance`」「付款核对 ← `UC-CC-009`」;`AT-SA-001`:「**合法关务交接到达** → 只形成结算输入接收和处理入口」。
- **MAP 边**:「`customs-compliance → settlement-accounting`:关务提供监管核定税费、法定义务人、外部资金事实引用、税费付款核对和责任依据;结算通过 UC-SA-001……独立确认实际代垫是否成立」。
- **代码事实**:CC 的九个 handoff(case / declaration_submission / external_result / follow_up / gate_verification / manifest / restriction / verification / case_closure)无一涉及税费、付款核对或代垫输入;`internal/settlementaccounting/` 无 `adapters/customscompliance` 消费面;CC 的税费付款协作事项与税费付款核对在 `internal/customscompliance/` 下无对应实现件。**双向都空,而 UC 双侧都已写明这次交接。**

### 2. CC → NO / TF:关务执行协作事项,无发布口

- **UC 要求**:UC-NO-001 开篇——「本用例从 `node-operations` **收到** `UC-CC-008` 针对明确节点、对象、范围和动作形成的当前有效关务执行协作事项**开始**」;触发行:「当前有效且范围明确的关务执行协作事项**到达目标节点**」。UC-TF-001(监管运输处置承接)是同一链的运输半边(只对明确要求实际移动的范围)。
- **MAP 边**:「`customs-compliance → node-operations / transport-fulfillment`:关务提供已接收且适用范围明确的限制、查验或处置决定、**范围化协作依据**、绑定拟执行动作与监管边界的放行门禁判断及外部监管结果」。
- **代码事实**:CC 九口里最接近的是 `gate_verification`(放行门禁核对)与 `restriction`(限制)——**协作事项本身没有口**;NO/TF 侧均无 `adapters/customscompliance` 消费面。注意 VE 的 `disposition_request`(异常处置请求,表 43 行)是 `visibility-exception` 拥有的另一对象,按 CC CONTEXT「(协作事项)不等于监管决定、节点作业任务……」的同一分辨逻辑,不顶这一格。

### 3. TF → PP / SA:运输收费发生项,域对象在、出不了上下文

- **UC 要求**:UC-TF-002——「对已经实际发生且满足外包协议事实条件的失败或成功尝试形成运输收费发生项,**供结算独立判断供应商成本**」(含应用流程步骤 7「只保存发生范围;金额交给 `settlement-accounting`」);UC-SA-004(供应商账单匹配审核)是其消费端(供应商预期成本上游)。
- **MAP 边**:「`transport-fulfillment → parcel-pricing / settlement-accounting`:运输履约提供订舱、取消、失败尝试及原/替代旅程分别形成的运输收费发生项……计价结合 BUY 商业依据形成纯评价,结算据此形成供应商预期成本并独立匹配账单和审核应付」。
- **代码事实**:域对象 `transportfulfillment/domain/transport_charge_occurrence.go` 存在,但 `transportfulfillment/ports/` 对 `ChargeOccurrence` **零命中**——无仓储端口、无 handoff、无迁移表。发生项今天只活在域内存里,连 TF 自己都存不下来,更到不了 PP/SA。report.md 第 12 行把 `TransportCommissionHandoff` 对到这条边,但该信封载荷是委托提交,不含发生项。

### 4. NO → PS / NR / VE:物理拆分合并事实,无发布口

- **MAP 边**:「`node-operations → parcel-shipment`」边的内容清单含「节点收寄、实测与**物理拆合事实**」(report.md 第 8 行判据栏引过同句)。
- **CONTEXT 要求**(消费侧的依赖是硬句):NR CONTEXT——「真实拆分或合并发生后,被替代包裹的当前路由终止适用;**每个后继包裹从身份变化节点分别形成新路由**,并关联共同的已执行前缀和来源路由依据」——NR 必须获知拆合;PS 拥有「包裹身份谱系」且 MAP 有 `PS→VE` 谱系边,谱系变化的源头正是 NO 的物理拆合。
- **代码事实**:NO 四口(node_intake / execution_fact / sealed_snapshot / collaboration_acceptance)无拆合口;`ConsolidationUnits`(集运单元)域件与仓储在,拆分对象与发布面都没有。
- **诚实注记**:哪个 UC 承接拆合消费(PS 谱系更新?NR 重判?)在 UC 层未明文——本条的判据是 MAP 边+NR CONTEXT 硬句,UC 半边缺档照实记,不属「本就不该有」。

### 5. PS / TF → CR 与 SA ↔ CR:四条边全空(已知例,补齐出处)

- **MAP 边四条**:「`parcel-shipment / transport-fulfillment → collection-remittance`:小包托运拥有并提供包裹与客户代收服务要求快照;运输履约拥有并提供履约侧代收证据、渠道代收报告和渠道回款通知」;「`settlement-accounting ↔ collection-remittance`:结算拥有 COD 服务费和有效协议允许的运费抵扣金额,代收与清分拥有代收本金及汇付」;图上另有「CR →|代收、清分与汇付结果| SA」。CONTEXT-MAP 有 CR 的完整所有权节(拥有/不拥有)。
- **代码与文档事实**:`internal/` 下无 collection-remittance 实现、`docs/application/collection-remittance/` 无任何 UC、无任何指向它的 handoff。report.md 矛盾清单第 3 条已档("有边无发布面"),本表按③口径正式列为反向缺口:**边在、UC 缺、两侧实现全空**。

## 扫描到但不是缺口的(结论,非缺口)

- **party-commercial 无任何 handoff——本就不该有(现行文档口径下)**:PC 的交付走消费侧同步适配器(ADR-0025 消费侧适配器 + ADR-0063 消费侧窄口取证;PS/CC/SA/PP 各自持有 `adapters/partycommercial`),UC-PC-002 的解析是同步问答,UC-PC-001 发布的是册面版本不是事件。三份 UC-PC-* 无一要求事件交付。
- **parcel-pricing 无 UC**:MAP 三条入边(PS→PP 包裹与声明快照、PC→PP 商业闭包、TF→PP 发生项)的消费方式在 UC 层未定义——属缺档,不属发布口缺口(发生项那条的发布侧缺口已记第 3 条;PP 的 `EvaluatePricing` 零生产构造已在 report.md 附带 a)。
- **pilot-governance 全部**:无 CONTEXT、无 UC(本次 glob 复核仍零文件),消费方向不可判,维持 report.md 的「说不清」。
- **对外段不算跨上下文**:申报对外发送(UC-CC-005)、监管机构、银行/支付(UC-SA-001/UC-CC-009 的资金事实来源)、消息能力(UC-VE-006)、门户展示——对端不是限界上下文,发布形态属集成设计,不入本表。
- **SA 经营指标(cost-allocation / operating-result)**:MAP 无 SA 向内部上下文供指标的边,report.md 已判「本就不应该有跨上下文消费者」,维持。

## 保质期

代码断言(「无端口」「零实现」「零构造」)取证于 `3b9f212`,HEAD 前进后须重验;UC 与 MAP 引文不随代码变化。五条缺口按「谁该先动」天然分两类:1/2/3 的发布侧对象在 CC/TF 自己的 CONTEXT 里已有语言(税费付款协作事项、关务执行协作事项、运输收费发生项),补的是持久化与发布面;4/5 还缺 UC 级消费入口或整个上下文实现,动手前先补档。
