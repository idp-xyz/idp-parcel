# 信用政策正文已入册、`CreditBasis` 无人索取：PC→SA 的授信额度缝不存在

Category: enhancement
Status: in-progress——2026-09-08 11:0x，MCP-1 代裁**甲**（owner 授权自决口径，见 Comments 末条）：闭包解析加 `resolveCreditPolicyBasis` 一步、`ResolveCreditPolicy` 作四维选择器、`CreditBasis` 随 Resolution 交出，独立一篇 **ADR-0127**；判据 1 已裁，判据 2（提供方口 + SA 消费适配器，可碰 `settlementaccounting/adapters/partycommercial`）与 3（剪行按第二种）由 MCP-6 在 `mcp6-wbr03-05` 接。此前 blocked：2026-09-08，MCP-6（task 5f716c71，用户 02:0x 自 MCP-3 改派；分支 `mcp6-wbr03-05` 基 `4524cfd4`）：完成判据 4 的理由行已补进基线（条目上方七行），判据 1 那格「四维选择在哪一层还有多候选」经取证是**解析语义的改口、要 ADR**，按派单纪律停下报 MCP-1 裁。此前 draft：只读取证（MCP-6，锚 `2efef58e`），PC 地盘归 MCP-3；交 MCP-1 派
Blocked by: 无（「要先裁的一格」已由 MCP-1 2026-09-08 裁甲；与 `party-commercial-context-gaps/07` 的 ADR-0115 分立，独立 ADR-0127）

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

## 完成判据（落地那笔连理由行一起改；MCP-1 2026-09-07 裁）

1. 「要先裁的一格」有裁决（一版一行→并入闭包的 `covers` 校验，或一版多行→`ResolveCreditPolicy` 作选择器），与 `party-commercial-context-gaps/07` 同轮；落 ADR 时向 MCP-1 取号。
2. PC 侧有向 SA 供 `CreditBasis` 的提供方口（用例或端口），按裁决调 `ResolveCreditPolicy` 或经闭包读 `LoadCreditPolicy`；SA 消费侧适配器在 UC-SA-002 步 7 账期分支真索取它，形成信用暴露 / 限制结果时有出自政策版本的额度依据可比（`AT-SA-171` / `AT-SA-172`）。
3. 剪基线行：先按头注三分成因（全仓 `ResolveCreditPolicy` 只此一处声明才是第二种；若裁决把它并入闭包而删掉，则是第一种、头注记一句），在自己那笔的干净检出上两法同得记数、钉 SHA。
4. **若 1–2 之前先要补理由行**（今天这条**没有**理由行，PC 组注释只讲了另两条的族界），在条目上方加：

   > 信用政策选择门，`CreditBasis` 是本上下文交给 settlement-accounting 的授信依据。**调用方是 PC 侧向 SA 供授信额度的提供方口**（UC-SA-002 步 7 账期分支的消费侧适配器索取它），那条 PC→SA 缝今天不存在：缺提供方口、缺 SA 消费侧适配器，且「四维选择在哪一层还有多候选」那格未裁（与 party-commercial-context-gaps/07 同轮）。三件落地（wiring-baseline-remainder/03）那天这一条出名单。

## 边界

本票不改代码、不改基线。基线行剪掉的时刻是 PC 提供方口真调 `ResolveCreditPolicy` 那一笔。（立票时的边界；落地笔见 Comments。）

## Comments

- 2026-09-08 02:3x · MCP-6（task 5f716c71；分支 `mcp6-wbr03-05` 基 `4524cfd4`）：**补理由行 + 取证「要先裁的一格」，停下报 MCP-1。**
  基线 `ResolveCreditPolicy` 条目上方按完成判据 4 加了理由行（调用方是谁、缝缺哪两半、未裁的一格是什么、哪天出名单），名单一行
  未动（两法同得 6 / PC 4，与 05 剪后同）。**取证**（锚 `4524cfd4`）：`0020_credit_policy.sql` 的 `credit_policy` 主键是四元组
  （tenant, object_kind, object_id, version_label），**一版一行**；`domain.CreditPolicy` 一行带（法人, 等级, 费用类型, 额度），
  `ResolveCreditPolicy(policies, query)` 对一组行按 `covers` 选唯一 / 报冲突 / 报无依据。所以「多候选」不可能来自同一版本的多行，
  只能来自**同一范围内多个信用政策对象各自的生效版本**——这与结算政策同形：`commercial_resolution.go` 的 `resolveSettlementPolicyBasis`
  已经在闭包解析里按六维选结算政策（零候选`无适用依据`、多候选`适用冲突`），信用政策今天却没有对应的 `resolveCreditPolicyBasis`
  一步，闭包只把 `CreditPolicyObject` 当版本壳按范围选。**两条路**：(甲) 照结算政策的形，在闭包解析里加一步、`ResolveCreditPolicy`
  作四维选择器、`CreditBasis` 随闭包交出——`ports.go` 自注这是「解析语义的改动，不是登记正文的连带」，要 ADR；(乙) 闭包只认唯一
  版本壳、`ResolveCreditPolicy` 退化成对已选版本正文的 `covers` 校验——同样改解析语义（对不上答`无适用依据`），也要 ADR，且
  多法人 / 多等级的租户只能把每一格拆成不同范围。我的倾向是甲（与结算政策一致、不逼租户拆范围），但这是难逆转的解析语义取舍，
  派单纪律写明「先停下报 MCP-1，不在实施票里顺手定」，故本笔不裁、不建缝。SA 侧的形已看过：`settlementaccounting/adapters/
  partycommercial/pre_acceptance_control_policy.go` 只读结算政策，账期分支要的 `CreditBasis` 无处来；接线时是那只适配器旁加一只
  读 PC 闭包交出的信用依据、装配在 SA 的接受前控制编排。**待 MCP-1**：裁甲 / 乙 + 取 ADR 号；裁后本票判据 2、3 另笔接（可能落
  同一 ADR 于 pc-gaps/07 的 ADR-0115 之后作补充记录，或独立一篇）。
- 2026-09-08 11:0x · MCP-1 代裁（owner 授权自决口径；由 MCP-6 落票面）：**裁甲。** 照结算政策的形，在闭包解析里加
  `resolveCreditPolicyBasis` 一步（镜像 `resolveSettlementPolicyBasis`：候选先按版本的租户与范围收窄，再由 `ResolveCreditPolicy`
  按（法人、等级、费用类型、时点）选唯一 / `适用冲突` / `无适用依据`），`CreditBasis` 随 Resolution 交出。理由：价格规则与结算
  政策两个政策类对象已在同一闭包里用类别专属选择器解析（ADR-0044 镜像 `resolvePriceRuleBasis`），信用政策是第三个；乙会让同一
  闭包里两套解析语义并存、且逼租户把（法人、等级）拆成范围——正是 `ports.go` 自注那句要拦的。**号 ADR-0127**（0126 在
  `mcp5-awf08`，0127 全 ref 无文件，MCP-1 10:5x 查），独立一篇、不作 ADR-0115 补充；Context 里写「此前 `ResolveCreditPolicy`
  零生产调用」时钉 SHA 不写行号。判据 2 的 SA 消费侧适配器在 SA 地盘，本单可碰 `settlementaccounting/adapters/partycommercial`；
  判据 3 剪行按**第二种**（真接上）。Status blocked→in-progress，Blocked by 清；ADR-0127 + 判据 2–3 由 MCP-6 接着做，每小步提交并推。
