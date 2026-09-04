# 清关报价按票、按 MAWB 计费的项目没有评价主体：计价只逐包裹

Category: enhancement
Status: in-progress（票级，MCP-6 2026-09-04，隔离分支 mcp6-pp-renumber，基线 main 4cc1bc34，task-2b9edfe4 换号批五票之一）／ blocked（主单级，等 `transport-fulfillment` 立承运总单登记册——应立而未立，归 TF owner）——已裁选项 1 + SA 既有分摊（即选项 3），落文 [ADR-0111](../../../docs/adr/0111-shipment-and-mawb-level-billing-units-are-evaluation-subjects-in-parcel-pricing-and-settlement-allocates.md)（2026-09-04，通道 6，owner 授权）；CONTEXT「评价对象」硬句已改口为四种；本票转实施票，范围见「裁决」节末段
Blocked by: 主单级那一半等 TF 承运总单登记册——[tf-carrier-master-document-register/01](../../tf-carrier-master-document-register/issues/01-carrier-master-document-register-does-not-exist.md)（draft，归 TF owner）；票级那一半无

## 为什么立

[E1 形状核对](../report.md) 第 17 项。清关报价表（T01 / T11 两种模式）的计费单位有四种：按 MAWB、按票、按 KG、按件。后两种今天装得下——按 KG 走 `RateTableFamilyUnitPrice`，按件走 `FixedChargeRule`（聚合方式 `AggregationPerPackage`）。**前两种装不下**（核于 `a17bfac`）：

- `AggregationMode` 唯一取值 `AggregationPerPackage`，`PricingPlanVersion` 构造时硬写；CONTEXT 与 [`pp-pricing-rule-model-final-design.md`](../../../docs/design/pp-pricing-rule-model-final-design.md)「概念集合」节都写「聚合方式仍只有逐包裹」。
- `EvaluationSubjectKind` 只有已受理包裹与试算，没有委托（票）或主单（MAWB）这一级的评价对象；`PricingInputSnapshot` 一份对应一个包裹的重量与尺寸。

一笔「每 MAWB 若干美元」的清关费如果按包裹评价，要么每个包裹各收一次（多收），要么由调用方平摊后写成定额（摊法没人声明过）。

## 要裁的一件：落在哪一侧

1. **parcel-pricing 加聚合方式与评价主体**：`AggregationMode` 加 `PER_SHIPMENT` / `PER_MAWB`，`EvaluationSubjectKind` 加对应主体，评价输入快照带成员清单（件数、总重）；费用行上标聚合单位。代价：改规范化形状（`canonicalization` 换号）、CONTEXT「定价方案」与「计价评价」词条改、评价快照与读面全跟；设计文档「本设计不包含…周期级费用范围或独立计价服务形态」那句要看是否被触及（票级不是周期级，但要写清）。优点：价卡语言在一处，SA 继续「不拥有可执行价卡或第二套算价规则」（SA CONTEXT 首段硬句）。
2. **settlement-accounting 以费用发生项 + 版本化分摊规则承接**：清关行的按 MAWB/按票费用作为供应商费用发生项进 SA，按 SA CONTEXT 已有的「成本分摊结果 / 未分摊余额」机制归因到包裹；对客侧同法。代价：**费率本身还是得有地方算**——SA 不拥有算价规则，「每 MAWB 若干美元」这个数从哪张卡查出来仍然没有答案；等于把问题挪到 SA 而不是解决。
3. **两侧各半**：PP 只评价单价与规则（票级评价主体），SA 负责把票级金额分摊到包裹作经营归因。这实际是 1 + SA 既有分摊，不是新选项。

倾向（不是裁决）：1（配合 SA 既有分摊即 3）。判据是 SA CONTEXT 那句硬句——算价规则只能在 PP。

## 红线

- 不写任何真实清关费率进仓；SYN 夹具只记 `S`。
- 裁定前 E2 转换工具**不把按 MAWB / 按票项平摊成按件定额**，如实列「未转换：等本票」。
- 不给 SA 加第二套算价规则。

## 裁决（2026-09-04，通道 6，task-f530ad56 裁决批口径：owner 授权自决，写明能力边界）

**取选项 1 配 SA 既有分摊（即选项 3），落文 [ADR-0111](../../../docs/adr/0111-shipment-and-mawb-level-billing-units-are-evaluation-subjects-in-parcel-pricing-and-settlement-allocates.md)。** 判据两条硬句：SA CONTEXT「不拥有可执行价卡或第二套算价规则」——「每 MAWB 若干」这个数只能在 PP 按卡算，选项 2 把问题挪到 SA 而不是解决；CONTEXT-MAP 把「承运总单、运输舱单及外部承运凭证的身份和版本」判给 TF——主单主体的身份从 TF 引，PP 不铸。ADR 五条：`AggregationMode` 加逐委托、逐主单，`EvaluationSubjectKind` 加委托、承运总单，费用行标聚合单位；票级 / 主单级快照带成员清单与合计量、进语义摘要；归因到包裹归 SA 既有「成本分摊结果 / 未分摊余额」机制按版本化分摊规则做，PP 不带摊法；主体身份各引其所有者——**TF 今天没有承运总单登记册**（`query_transport_fulfillment_records.go` 注释与 admin-skeleton-closure-batch/05 记明「无处可登」），主单级那一半**形状定、身份来源缺**，生产上形成一次主单级评价要等 TF 立册，之前 E2 对按 MAWB 的行如实列「未转换：等 TF 承运总单登记册」；CONTEXT「评价对象」硬句从两种改四种，规范化换号与 ADR-0107–0110 同期合并为一次。设计文档 `pp-pricing-rule-model-final-design.md`「聚合方式仍只有逐包裹」那句由 ADR 取代，不在本批改它（docs/design 不在本批地盘，列给 MCP-1）。

**能力边界**：读了本票与 `report.md` 第 17 项、SA CONTEXT 首段、CONTEXT-MAP 的 TF 拥有节、PP CONTEXT「评价对象」「计价输入快照」词条、`query_transport_fulfillment_records.go` 关于承运总单一区的注释；**未读** `parcel-shipment` CONTEXT 里「委托」身份的精确词条（只据 ADR-0045 的「提交版本」语言转述——委托主体引用的是委托身份还是某一提交版本，实施开工前对 PS owner 确认一句）、SA 分摊机制的代码形状；未打开任何客户清关报价表。

### 本票转实施票，范围

1. 领域（票级先做）：`AggregationMode` 与 `EvaluationSubjectKind` 各扩两格（封闭集改动走三步法或单独 worktree）；`PricingInputSnapshot` 加成员清单形状（成员包裹引用 + 件数 + 合计实重 / 体积重），构造门按主体种类拒错配；评价在票级 / 主单级主体上：按 KG 行用合计计价重量查表、按件行按件数乘定额、按票 / 按主单行取定额一次；费用行标聚合单位；规范化换号（同期合并）。
2. 主单级：领域形状随 1 一起落，快照里的承运总单引用暂无生产来源——不造替身、不用包裹引用顶替；等 TF 立册后接真引用。
3. 消费侧：SA-c 缝与成本分值桥读到聚合单位与成员清单；SA 按既有分摊归因，分摊规则是 SA 实例参数（本票不填）。
4. E2：按票的行转成票级评价的卡内容；按 MAWB 的行如实列「未转换：等 TF 承运总单登记册」；不平摊、不折进按件定额。

## 验证

裁决记进 CONTEXT，必要时 ADR 编号落进上面某一条，再按所选拆实施票；本票 `resolved` 的判据是那条引用在。

## Comments

- 2026-09-04 · MCP-1：立票。起因是 E1 核对第 17 项。**只写票面，未动代码。**
