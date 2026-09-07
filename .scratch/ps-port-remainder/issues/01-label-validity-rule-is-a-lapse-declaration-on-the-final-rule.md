# `LabelValidityRuleView`：「接受时固定的有效期规则」是终局规则上的一格有效期声明，PS 只消费

Category: enhancement
Status: blocked——三问已由 MCP-1 代裁（owner 授权，2026-09-07，见 Comments），裁决与本票「裁决」节一致；**PC 半边等 pc-gaps 批（MCP-3）**：`FinalRuleContent` 长一格有效期声明 + 读口 + 发布通道；**PS 半边（消费适配器 `label_validity_rule.go`）Blocked by PC 半边**，PC 落地后转 ready-for-agent，通道 2 可接
Blocked by: PC 半边（pc-gaps 批，由 MCP-3 立票承接；本目录不替 PC 立票）

## 端口今天说什么

`ports.LabelValidityRuleView.JudgeLabelLapsed(ctx, tenant, transaction, parcel, asOf) (lapsed, configured, error)`：按「接受时固定的有效期规则」判一笔交易上该包裹的成功面单结果是否已不可逆失效；`configured=false` 即规则未配置——「没有规则就没有失效，那笔成功照常阻止终局，不按墙钟推算过期」。唯一消费方 `JudgeLabelServiceFinalHandler.lapsedTransactions`：`Validity == nil` 与 `configured=false` 都不失效。仓内无生产实现，只有 `labelValidityDouble`；`NewJudgeLabelServiceFinalHandler` 在 `cmd/` 也无调用方（面单交易仓储头注：「本口今天没有生产写入方，这是设计而不是欠账」）。

## 语言从哪里来

- PS CONTEXT Rules：「自然失效必须来自**渠道确认**或**接受时固定的有效期规则**」；关闭路径终局条件「已有成功结果均已成功作废或依据接受时固定的规则不可逆失效」。两个来源，本票只管第二个。
- PC CONTEXT Rules（面单服务终局规则那一段）：「……此时才可依据明确失败、成功作废或**接受时固定规则下的不可逆失效**形成终局」；Boundaries：「`party-commercial` 拥有……面单服务终局规则（正文挂在接单规则包版本下）」；「委托被接受时固定其适用的……面单服务终局规则」。
- 代码：PC 已有 `final_rule_content` / `final_rule_declaration`（迁移 `party_commercial/0013`，ADR-0058 决定一「`FinalRuleContent` 归接单规则包版本」），读口 `pcports.FinalRuleContentView.LoadFinalRule`；今天的声明行是「责任结果 → 终局分类」（`PAR-COM-17`），**没有任何一格说有效期**。PS 消费侧已有同族适配器 `DeclaredStageContent`（`adapters/partycommercial/stage_content_declarations.go`）经 `AdoptedStageOwner` 回指接受时固定的规则包版本（ADR-0062）。
- 锚点时间在 PS 手里：`LabelTransaction.ResultObservedAt()` 是渠道结果的业务时间。

## 裁决（通道 2，2026-09-07；owner 授权自决口径；取证锚远端 main `ffa6bd0e`）

**① 机制半边现在能立什么。** 两半，先后有序：

- **PC 半边（先）**：`FinalRuleContent` 长一格**有效期声明**——「自 *某一时刻* 起 *多久* 后，该包裹的成功面单结果不可逆失效」。形状：`起算时刻种类`（封闭集，见问题 1）+ `时长`；一版终局规则至多一条有效期声明；**没有这一格就是没有**，`LoadFinalRule` 交回的内容里有效期缺席即消费方答 `configured=false`——不得因为终局规则其它行在场就把有效期当成已配置。登记随 `PublishCommercialAuthorityHandler` 的 `FinalRuleChannel` 走（同一发布通道多一项正文），读口在 `FinalRuleContentView` 上加一个取有效期的方法或让 `FinalRuleContent` 直接带它。
- **PS 半边（后）**：消费适配器 `adapters/partycommercial/label_validity_rule.go` 实现 `LabelValidityRuleView`：包裹 → `CurrentAcceptedParcelTargetView.FindCurrentAcceptedByParcel` 找到当前已接受委托 → `AdoptedStageOwner.AcceptanceRulePackageFor` 回指接受时固定的规则包版本 → `LoadFinalRule` → 有效期声明 → 以 `transaction.ResultObservedAt()`（或问题 1 裁定的那一格）为锚，`asOf ≥ 锚 + 时长` 即 `lapsed=true`。找不到委托 / 未固定规则包 / 声明无有效期 → `configured=false`；读口出错 → error。**「接受时固定」不在 PS 另存**：闭包已把规则包版本钉在接受那一刻（ADR-0062），PS 读的就是那一版，规则包换版不追溯。

**② 实例半边留什么、在哪一格拒默认。** 时长取值与起算时刻的选择属 `PAR-COM-17`（终局规则实例参数）待提供；任何一版没有有效期声明 → `configured=false` → 不失效（既有语义，一字不改）。**不给「30 天」之类默认，也不拿墙钟推算**。

**③ 形状照哪个先例。** ADR-0058（声明归拥有规则对象、产品与合同只采用、无父行=未配置、父行在而子行坏=error）；0013 的 `final_rule_*` 表族加一张子表或加列；PS 消费侧照 `DeclaredStageContent` + `AdoptedStageOwner`；「规则版本化、显式登记、无登记即未配置、值属实例」的纪律与 label-channel/19 的轨迹源有效时间规则目录同一条。

**④ 要不要 ADR。** 不要。归属由 ADR-0058 决定一那一行（`FinalRuleContent` → 接单规则包版本）直接覆盖，本票只是给已有的声明加一格；PC owner 落地时若认为改了 `FinalRuleContent` 的领域形状要记，按 ADR-0104 的先例自裁。

## 要你答的问题

1. **起算时刻种类的封闭集取哪几格？** 候选：渠道结果业务时间（`ResultObservedAt`，即渠道受理那一刻）、面单文件签发时刻（今天聚合上没有这一格）、委托接受时刻。我的倾向：**首发只开「渠道结果业务时间」一格**，其余等真规则出现再加——封闭集加格是新版本规则的事，不是默认。
2. **有效期是不是按渠道产品不同？** 若真实规则是「某承运商面单 N 天失效」，它更像产品—渠道映射上的渠道约束（PC「可复用渠道约束」）而不是接单规则包的声明。CONTEXT 两处都写「接受时固定的有效期规则」，我按字面归终局规则；若你认为它随渠道走，本票 PC 半边改落映射侧，PS 适配器的回指路径随之换成「该交易实际使用的映射」（PC CONTEXT「后续发生渠道选择……再固定该次决定实际使用的映射」）。
3. **「渠道确认」的失效**（另一来源）走哪条入口——渠道适配缝的入向事实还是运营登记？不在本票，但答了它这两条来源在 `JudgeLabelServiceFinal` 里才对得齐。

## 红线

- 不填任何时长；不拿系统时间推算过期；无声明恒不失效。
- 不动 `JudgeLabelServiceFinal` 的领域判断；不给 `LabelTransaction` 加「已失效」状态——失效是读时按规则判出来的，不是存下来的状态（CONTEXT：受控关闭「不使既有面单失效」，失效来源只有两个）。
- PC 半边不在 PS 地盘动手：`internal/partycommercial/**` 归 PC owner。

## 参照

`internal/parcelshipment/ports/ports.go` 的 `LabelValidityRuleView` 头注；`application/judge_label_service_final.go` 的 `lapsedTransactions`；`domain/label_transaction.go` 的 `ResultObservedAt`；`adapters/partycommercial/stage_content_declarations.go`；`migrations/party_commercial/0013_stage_content_declarations.sql` 的 `final_rule_*`；`pcports.FinalRuleContentView`；ADR-0058、ADR-0062、ADR-0025、ADR-0084；PS CONTEXT 与 PC CONTEXT 上引各句；`PAR-COM-17`。

## Comments

- 2026-09-07 · 通道 2：立票（draft），一次 `/domain-modeling` 的产物。**只写票面，未动代码。** 能力边界：读过端口头注、消费编排、`LabelTransaction` 的结果时间、PC 0013 迁移的表名与 `FinalRuleContentView` 声明、ADR-0058 全文、两处 CONTEXT 的相关句；**没读** `FinalRuleContent` 领域类型全文与 `publish_commercial_authority.go` 的 `FinalRuleChannel` 分支细节——「加一格」在那两处怎么落归 PC owner。
- 2026-09-07 · 通道 2（task-b77525c9 ④ 取证）：**`NewJudgeLabelServiceFinalHandler` 无生产入口是 label-channel/11 有意留的，不是本口欠的。** 该票 Answer「不在本票」节明写三个触发点（TF 首次有效收寄事实到达的 PS 侧 inbox 消费者、面单交易定案那一拍、受控关闭/重开决定生效）「各自一张接线票」且「`cmd/parcel-api` / `cmd/parcel-dispatch` 无装配：与上面第一条同落」。**但那三张接线票至今没立**——`unresolved-review-20260904/remaining-work-a3a4814.md`「面单渠道链」第 2 条已记为余工并指出 label-channel spec 没有一张子票承接；本目录不替那边立票（地盘归 label-channel 目录持有者 / MCP-1 派单）。另：lc/11 把有效期规则读口记为「实例登记面，无 PAR 编号，随首个面单渠道产品的实例登记一起立」，本票裁决把它归到终局规则的声明（`PAR-COM-17`），以本票为准——两处口径不同，读到 lc/11 那句的人以这里为新。
- 2026-09-07 · MCP-1 代裁，owner 授权（task-b77525c9，由通道 2 落票面）：**Q1** 起算时刻种类首发只开「渠道结果业务时间」一格；**Q2** 按 CONTEXT 字面归终局规则（接受时固定），不落映射侧——若日后真规则按渠道走，那是新一版声明不是改归属；**Q3** 不在本批，归 `label-channel-service-first-release/20`（第一家真源）。PC 半边并入 PC 批队列（MCP-3 当前批后），PS 半边 Blocked by 它。Status 由 draft 改 blocked。
