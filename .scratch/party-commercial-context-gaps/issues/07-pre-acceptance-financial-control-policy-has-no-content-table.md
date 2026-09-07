# 接受前财务控制策略版本：能发布、能被合同引用，正文表未建——「控制怎么做」今天无处落

Category: enhancement
Status: in-progress——四问已由 [ADR-0115](../../../docs/adr/0115-pre-acceptance-financial-control-policy-content-is-a-row-per-control-and-no-control-stays-with-the-contract.md) 一次答完（2026-09-07，MCP-3，owner 经通道 1 派工授权自决）；正文表 + 领域对象 + 登记口在分支 `mcp3-pcgaps07` 上落地中
Blocked by: 无

## 从哪里来

[admin-write-faces/06](../../admin-write-faces/issues/06-pre-acceptance-financial-control-policy-versions-have-no-read-face.md)
问「这一类版本管理台看不见」，取证（report.md A 组，锚 `08e62ec`）答的是比读面更深的一层：
`CommercialObjectKind` 第 5 类 `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY` 在库里**只有 `commercial_version` 壳**，
`migrations/party_commercial/` 无它的正文表；结算侧 `settlementaccounting/adapters/partycommercial/pre_acceptance_control_policy.go`
的 `LoadControlPolicy` 读的是商业闭合 + 合同级声明（`0007`），不读任何策略版本正文。MCP-3 2026-09-04 裁：读面不列壳
（列壳无信息量，票 06 完成判据第二条自己说的），**正文表是缺口本体**，另立本票；票 06 转 blocked 等它。

## CONTEXT 说这一类版本带什么

词条原句（[PC CONTEXT](../../../docs/domain/party-commercial/CONTEXT.md)「接受前财务控制策略」）：

> 客户合同版本针对明确业务范围规定的接单前财务控制依据。策略可以要求预付冻结、信用校验、明确接受前无财务控制，或者
> 合同明确规定且相互不冲突的控制组合；它定义**适用范围、共同通过条件和失败处置**，但不拥有实际估价、余额、冻结或
> 信用暴露结果。

Rules 两句：

> 接受前财务控制策略不得硬编码为永久互斥的三选一枚举。合同要求组合控制时，策略必须明确**每项控制的适用范围、判断顺序、
> 共同通过条件和失败或补偿责任**；某一范围明确无控制不能静默取消其他范围已经规定的控制。

> 每个用于新委托接受的客户合同版本必须明确引用适用接单规则包和接受前财务控制策略；确实不适用的控制必须按明确范围记录
> 不适用依据。规则或策略缺失不得被解释为允许接受。

这几句把正文的**结构**说死了（控制项集合、每项的适用范围、判断顺序、共同通过条件、失败/补偿责任），一格都不依赖租户
取值；取值（哪几项、阈值、账户与价格依据、失败处置的具体动作）是 `PAR-COM-15` 的最低证据列，待提供。**与 ADR-0104
「客户服务规则正文」是同一形**：CONTEXT 有语言、代码只有壳、形状由硬句推得出——机制半边现在就做，不留空。

## 今天两层各答什么，别合并

| 层 | 表 | 答的问题 | 拥有对象 |
|---|---|---|---|
| 合同级声明 | `0007_pre_acceptance_control_declaration` | 这份合同的这个范围**要不要**控制；说`不适用`时凭什么 | 客户合同版本（`object_kind=2`，ADR-0042 归属纪律） |
| 策略版本正文 | **缺** | **控制怎么做**：哪几项、顺序、共同通过、失败处置 | 接受前财务控制策略版本（第 5 类） |

`0007` 头注自己写着「策略版本回答『控制怎么做』，回答不了『这份合同要不要』」以及「本表不存控制方式」——所以今天
「控制方式」实际上是结算侧从结算政策的预付/账期方式**推**出来的（ADR-0044 那条解析），而 `pn-02-w03` 早写过
「账期不能推导无需信用校验」。正文表落地后 SA 的 `LoadControlPolicy` 该不该改读正文，是另一票（SA 地盘），本票不碰。

## 要裁的（`/domain-modeling` 一次答完）

1. **控制项的封闭集**：CONTEXT 点名三种（预付冻结、信用校验、明确无控制）+「不冲突的组合」。封闭集里要不要给「明确
   无控制」留一格——`0007` 的 `NOT_APPLICABLE` 已经在合同层答了「不控制」，策略层再答一次是两处口径；倾向策略层只
   表达**要做的控制项**，「无控制」由合同声明独占。
2. **组合的表达**：每项控制一行子表（项、适用范围引用、判断顺序号、失败/补偿责任引用）+ 父表一格「共同通过条件」；
   还是整份正文一个结构化文档。倾向子表——「判断顺序」「某一范围明确无控制不能静默取消其他范围」这两句要逐项可查。
3. **适用范围与失败处置的取值形态**：机制只给槽（引用 + 封闭的处置种类），取值由 `PAR-COM-15` 供；不为任何一格拟默认。
4. **要不要 ADR**：改的是商业对象正文形状、连带 SA 解析路径的下一步——倾向要，形照 ADR-0104。

## 红线

- 不填 `PAR-COM-15` 任何实例值；不给未声明的合同一个默认控制。
- 不合并两层：合同声明表与策略正文表分开，`0007` 不动。
- 新迁移按序号新开（`party_commercial` 当前最大序号以开工那刻重取），不改已施加迁移。
- SA 读路径的改动不在本票。

## 完成判据

裁决落 CONTEXT（必要时 ADR 编号落进上面某一条）→ 正文表 + 领域正文对象 + 登记口（形照票 03 两张正文表的落法）→
[admin-write-faces/06](../../admin-write-faces/issues/06-pre-acceptance-financial-control-policy-versions-have-no-read-face.md)
据此解阻。`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库。

## Comments

- 2026-09-04 · MCP-3：立票。起因是裁 admin-write-faces/06 时发现「只有壳」不是长期事实——CONTEXT 明写策略定义适用范围、
  共同通过条件与失败处置，壳是机制半边的缺口。**只写票面，未动代码。** 能力边界：读过 PC CONTEXT 词条与 Rules 相关句、
  `0007` 迁移全文、admin-write-faces/06 票面与 report.md A 组取证；**没读** `pre_acceptance_control_policy.go` 全文与
  ADR-0044/0054/0079 正文，上表「结算侧从结算政策方式推控制方式」一句是按 `0007` 头注与 report.md 转述推的，建模时以代码为准。
