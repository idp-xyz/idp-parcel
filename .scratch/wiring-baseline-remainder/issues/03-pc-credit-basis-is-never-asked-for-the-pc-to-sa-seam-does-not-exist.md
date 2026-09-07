# 信用政策正文已入册、`CreditBasis` 无人索取：PC→SA 的授信额度缝不存在

Category: enhancement
Status: draft——只读取证（MCP-6，锚 `2efef58e`），PC 地盘归 MCP-3；交 MCP-1 派
Blocked by: 本票「要先裁的一格」（与 `party-commercial-context-gaps/07` 同一轮 `/domain-modeling`）

## 条目

`internal/partycommercial/domain ResolveCreditPolicy`（`credit_policy.go`）。基线理由行：无单独理由，落在 PC 组「三条」里；triage spec 记「整能力未接：信用政策无正文表」——**那句已过期**，正文表随 `party-commercial-context-gaps/03` 落地，见下。

## 它是什么

`ResolveCreditPolicy(policies, query)` 按（责任法人、权限等级、费用类型、时点）在一组信用政策正文里选唯一适用的一条，产出 `CreditBasis`（出自哪个政策版本、授权额度、`applicable`）；零候选答 `ErrNoApplicableCreditPolicy` 而不作答（注释：「缺政策既不是无限信用也不是零额度，该是哪一种只有拥有该商业依据的一方能说」），区间重叠答`适用冲突`。`CreditBasis` 的注释把消费方点了名：「是本上下文交给 settlement-accounting 的东西」。

## 已有的层（锚 `2efef58e`）

- 正文表与写口：`ports.CommercialRegistry.SaveCreditPolicy`（`party-commercial-context-gaps/03`），写入代数 `CreditPolicySaveOutcome`。
- 点读口：`ports.CreditPolicyContentView.LoadCreditPolicy(tenant, version)`，实现 `adapters/postgres/credit_policy.go` 的 `CreditPolicyContents`；目录读 `ListCreditPolicies`。
- 版本壳在闭包封闭集：PS 的商业依据解析键含 `CreditPolicyObject`（`parcelshipment/adapters/partycommercial/commercial_resolution_keys.go`）。

## 缺的层

- `LoadCreditPolicy` 在 PC 之外**零非测试调用**——正文登了没人读。
- SA 消费侧适配器 `settlementaccounting/adapters/partycommercial/pre_acceptance_control_policy.go` 只读结算政策（预付 / 账期方式），正文里没有一处提到信用。
- `settlementaccounting/adapters/postgres/operational_position.go` 注释写「授信额度来自商业侧信用政策」，而 `credit_minor` 是**登记进来的状况事实**，不从 PC 读。于是信用暴露结果（`FinancialControlCreditExposed`，ADR-0047）形成时没有一份出自政策版本的额度依据可比。

## 该有的调用方

UC-SA-002 步 7「按已唯一解析的结算政策范围和商业策略形成估价、冻结、**信用暴露**或限制；不适用时保存依据」的账期分支——SA 消费侧适配器向 PC 索取 `CreditBasis`；PC 侧提供方口（用例或端口）按闭包选出的信用政策版本点读正文，多份候选按 `ResolveCreditPolicy` 选唯一 / 报冲突 / 报无依据。`AT-SA-171` / `AT-SA-172` 描述的正是这一步该答的形状（业务 B 只形成信用暴露 / 限制结果；同时命中预付与账期即模式适用冲突）。

## 三分

**支路未接，且先缺一条缝**（PC→SA 授信额度），与 triage 票 03 对 `FormSupplierExpectedCost`（BUY 评价→SA）的判法同形：缝本身是要做的机制，不是「等」。

## 要先裁的一格

闭包已按范围选出唯一版本壳，正文表一版一行——那 `ResolveCreditPolicy` 的四维选择在哪一层还有多候选可选？

- 若一版恒一行：它退化成对已选版本正文的一次 `covers` 校验（法人 / 等级 / 费用类型对不上即`无适用依据`），选择语义并入闭包——`ports.go` 自注这是「解析语义的改动，不是登记正文的连带」，得裁。
- 若同一政策版本下要按（法人、等级、费用类型）分多条正文：正文表形状要改（一版多行），`ResolveCreditPolicy` 才是它的选择器。

这与 `party-commercial-context-gaps/07`（接受前财务控制策略正文表未建）是同一族——控制**怎么做**与控制**依据多少额度**是相邻两格，建议同一轮 `/domain-modeling` 一起裁，很可能同一篇 ADR。

## 能不能归到已认可的留待

不能。SA 三口目录读口在认可留待，但那是登记面形状等实例证据；本条缺的是缝与执行器。

## 边界

本票不改代码、不改基线。基线行剪掉的时刻是 PC 提供方口真调 `ResolveCreditPolicy` 那一笔。
