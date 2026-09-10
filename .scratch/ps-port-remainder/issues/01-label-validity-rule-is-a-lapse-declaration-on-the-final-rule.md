# `LabelValidityRuleView`：「接受时固定的有效期规则」是终局规则上的一格有效期声明，PS 只消费

Category: enhancement
Status: resolved——**2026-09-10 15:1x 进 main**（推送方通道 1 重放，分支→main 五对 SHA 与验证见文末「进 main 记录」；非作者评审由推送方自跑，两轴 0 阻断）。此前 14:4x 通道 6 完成 PS 半边（task-45b75eb2，分支 `mcp6-psr01`，代码 tip `1a23f5b2`、票面簿记两笔随后，基 `dd5ed934`，见「完成记录」）；此前 in-progress——2026-09-10 14:3x 通道 6 认领 PS 半边（隔离 worktree 分支 `mcp6-psr01`，基 `dd5ed934`）；此前 ready-for-agent——2026-09-10 14:2x 通道 1 解阻（远端 main `5dda0fb2` 上取证）：PC 半边已随 [pc-gaps/09](../../party-commercial-context-gaps/issues/09-final-rule-content-has-no-validity-declaration.md) 进 main（`838b283e` ADR-0119 + CONTEXT / `d06192ed` 迁移 0026 + `LabelValidityDeclaration` / `ValidityAnchorKind` / `FinalRuleContent.Validity()` + `LoadFinalRule` 读回 + `FinalRuleChannel` 折进有效期 + 批文 `finalRuleValidity`；owner 复核 2026-09-09 认可），其完成记录明写「不做的：PS 适配器 `label_validity_rule.go`（ps-port-remainder/01 PS 半边，据此解阻）」；剩 PS 半边（消费适配器），见 Comments 末条——注意 `NewJudgeLabelServiceFinalHandler` 在 `cmd/` 仍无调用方（label-channel/11「不在本票」三张接线票至今未立），本票适配器落地后仍无生产调用点，属诚实状态不属欠账。此前 blocked——三问已由 MCP-1 代裁（owner 授权，2026-09-07，见 Comments），裁决与本票「裁决」节一致；PC 半边等 pc-gaps 批
Blocked by: 无（PC 半边 pc-gaps/09 已进 main）

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

## 完成记录（PS 半边 · 通道 6 · 2026-09-10 · 分支 `mcp6-psr01`，基 `dd5ed934` = 当时的 origin/main）

**每笔（分支上的 SHA，作封存出处；进 main 的 SHA 由推送方重放后并列补记）**

- `560f51a7` feat：新文件 `internal/parcelshipment/adapters/partycommercial/label_validity_rule.go`（`DeclaredLabelValidityRule` + `NewDeclaredLabelValidityRule` + 未导出 `validityAnchorOf`）与 `label_validity_rule_test.go`；票面 Status 转 in-progress 随笔。
- `c9ba714f` docs：机制清点在 `560f51a7` 的干净 detached 检出上重生成——parcelshipment 生产 159→160 / 测试 155→156，PS→PC 消费缝 16→17，端口基线口径缺 15→14、精确口径缺 9→8（`parcelshipment.LabelValidityRuleView` 出两份缺口名单）。数字只作此刻取证，推送方在 tip 上重生成兑底。
- `1a23f5b2` docs：两处注释去掉对 PC 封闭集的计数措辞（AGENTS.md「计数与行号同构」），语义与代码未动；自评审 Standards 轴拿住的唯一一条。

**触及 / 未碰**

- 触及：上列两份新 `.go`、本票面、`docs/product/MECHANISM-INVENTORY.md`（生成物）。
- 未碰：该包既有文件、`ports.go`（`LabelValidityRuleView` 与 `JudgeLabelServiceFinalHandler.lapsedTransactions` 语义一字不改）、`domain/label_transaction.go`、`internal/partycommercial/**`、`cmd/**`、`migrations/**`、ADR-0119 / 0058 / 0062 正文。无 `.sql`。

**形状（一处与派单措辞不同，明写供评审判）**：派单写「适配器依赖三口」（`CurrentAcceptedParcelTargetView` / `AdoptedStageOwner` / `pcports.FinalRuleContentView`）。落地依赖两口：`psports.CurrentAcceptedParcelTargetView` + 包内既有 `FinalContentSource`。理由：`FinalContentSource`（`service_stage_rules.go`）就是「来源身份 → 接受时固定那版的 `FinalRuleContent`」这条缝，`DeclaredStageContent.FinalContentFor` 已经实现了 owner 回指 → 租户翻译 → `LoadFinalRule`；再写一遍是同一条路的第二份。运行时路径与派单一字不差（测试夹具就是 `NewDeclaredStageContent(nil, finalView, nil, owner)`），三格语义（owner found=false → 未配置；读口 error → error）由那只既有适配器承重，其自身用例在 `stage_content_declarations_test.go`。生产装配时把已装好的 `DeclaredStageContent` 直接传进来即可，不必再装一遍 owner。

**裁决①②对照**

- ① PS 半边：包裹 → `FindCurrentAcceptedByParcel`（用调用方给的租户与包裹，用例核过）→ `AdoptedStageOwner.AcceptanceRulePackageFor`（经 `DeclaredStageContent`，用例核过读的是接受时固定的那版 `CommercialVersion`，`SameVersionAs`）→ `LoadFinalRule` → `Validity()` → 锚按种类分路：`ChannelResultObservedAnchor` → `transaction.ResultObservedAt()`；`!asOf.Before(锚 + Duration())` 即 `lapsed=true, configured=true`。找不到委托 / owner found=false / `LoadFinalRule` found=false / `Validity()` 第二值为 false → `configured=false`；读口 error / 集外锚种类 → error。**「接受时固定」不在 PS 另存**：适配器无状态、无缓存、无自己的存储。
- ② 不填任何时长：适配器里没有一个 `time.Duration` 字面量；用例里的 `72 * time.Hour` 明写「夹具锚点，不是任何租户的声明（PAR-COM-17 待提供）」。无声明恒不失效见下。不拿墙钟：文件里没有 `time.Now`、没有 `Clock` 依赖。
- 红线第二条：没给 `LabelTransaction` 加任何状态，失效是读时算出来的；`JudgeLabelServiceFinal` 领域判断未动。

**用例对照（派单六例 → 实际）**

| 派单 | 用例 | 结果 |
|---|---|---|
| 有声明且已过期 | `TestDeclaredValidityLapsesOnceAsOfReachesAnchorPlusDuration`：asOf == 锚 + 时长（边界「≥」）、asOf 晚 30 天 | `true/true`；同时核租户、包裹、规则包版本、PC 租户 |
| 有声明未过期 | `TestDeclaredValidityDoesNotLapseBeforeAnchorPlusDuration`：差一纳秒、asOf == 结果观察时刻 | `false/true` |
| 无声明 | `TestFinalRuleWithoutValidityDeclarationLeavesLapseUnconfigured`：`NewFinalRuleContent` 行在场、无有效期 | `false/false`，读口恰点一次 |
| 无委托 | `TestParcelWithoutCurrentAcceptedRequestLeavesLapseUnconfigured` | `false/false`，终局规则读口零次 |
| 未固定规则包 | `TestUnfixedRulePackageLeavesLapseUnconfigured`（`UnconfiguredAdoptedStageOwner{}`） | `false/false`，终局规则读口零次 |
| 读口 error | `TestReadFailuresPropagateAsErrors`：包裹反查 / 终局规则读口各一例 | `errors.Is` 原错，`false/false` |
| （加）无父行 | `TestFinalRuleWithoutParentRowLeavesLapseUnconfigured`：`LoadFinalRule` found=false | `false/false` |
| （加）结果未回 | `TestTransactionWithoutChannelResultIsUntranslatable`：已提交无结果 → `ResultObservedAt` 零值 | `ErrUntranslatableAnswer` |
| （加）nil 依赖 | `TestDeclaredLabelValidityRuleRejectsNilDependencies` | 构造期拒 |

红先绿后：先写测试文件，`go vet` 报 `undefined: adapter.DeclaredLabelValidityRule`，再落实现。落地后做过一次变异核：把 `!asOf.Before(…)` 换成 `asOf.After(…)`，「asOf 恰在锚 + 时长」那例红，换回。

**验证强度（作者层）**：`gofmt -l` 空；`go build ./...` / `go vet ./...` 全仓 0；`go test -count=1` 于 `./internal/parcelshipment/adapters/partycommercial/ ./internal/parcelshipment/application/ ./internal/architecture/...` 全 ok；以上四项在 `1a23f5b2` 的**干净 detached 检出**上重跑同绿，且在该检出上重生成机制清点无漂移。反向依赖照 `go list -f '{{.ImportPath}} {{.Deps}}'` 反查：`cmd/parcel-api`、`cmd/parcel-commercial`、`cmd/parcel-dispatch`、`internal/parcelshipment/adapters/postgres` 四个包依赖本包——本笔只新增导出符号、未改任何既有签名，四包对它零引用，`go build` / `go vet` 全仓绿即证它们编译不变；按派单「不跑全量」，未带 DSN 跑 `cmd/*`（无 `.sql`、无 `cmd` 改动，真库用例的答案不可能被一个没人引用的新类型改变）。未跑：全量、`-race`、PG。基 `dd5ed934` 到远端 main `9dddaf65`（14:4x `ls-remote`）之间只动了 `.scratch/**` 四份 `.md`，与本分支文件零重叠，重放应干净。

**与生产接线的关系（如实写）**：`NewJudgeLabelServiceFinalHandler` 在 `cmd/` 零调用方（label-channel/11「不在本票」三个触发点各一张接线票至今未立，见 `unresolved-review-20260904/remaining-work-a3a4814.md`「面单渠道链」第 2 条），所以本适配器落地后**无生产调用点，等三张接线票**；届时装配处只需把本适配器填进 `JudgeLabelServiceFinalDeps` 的 `Validity`（`targets` 给 PS `ShipmentRequests` 仓储、`final` 给已装好的 `DeclaredStageContent`）。本笔不在 `cmd/` 装一个没人调的编排。适配器包不在接线棘轮扫描范围，未加基线。

**给评审的判断题**

1. **「无声明恒不失效」怎么证**：`TestFinalRuleWithoutValidityDeclarationLeavesLapseUnconfigured` 用 `NewFinalRuleContent`（终局规则行在场、`Validity()` 第二值 false）且 asOf 取结果后一年，仍答 `configured=false`——分辨这一格的是 `Validity()` 的第二个返回值，不是 `LoadFinalRule` 的 found；实现里没有任何从「其它行在场」推出「已配置」的路径。请核实现的 `if !declared { return false, false, nil }` 位置在 `LoadFinalRule` found 之后、锚翻译之前。
2. **「不拿墙钟」怎么证**：实现文件无 `time.Now` / `Clock`；用例时刻全在过去（结果 2026-08-20 11:00Z，72 小时后到点），「未过期」两例若适配器偷拿墙钟会在 2026-08-23 之后的任何一台机器上答 `true`，它们绿着就是证据；时间只单向前进，这条证据只会更强。
3. **两处派单没点名的守卫要不要**：(a) `ResultObservedAt` 零值（结果未回）报 `ErrUntranslatableAnswer` 而不是拿零值当锚——零值当锚会让任何 asOf 都算已过期，消费编排只对已受理的结果问本口，走到这里是契约被打破；(b) nil 依赖构造期拒绝，形照 `NewControlDispositionAdapter`。若评审认为 (a) 该答 `lapsed=false, configured=true`（「没结果就没失效」）而不是 error，那是口径之争，改一行加改一例即可。
4. **两口而非三口**（见「形状」）：评审若认为必须直接依赖 `AdoptedStageOwner` + `pcports.FinalRuleContentView`，改法是把 `FinalContentSource` 换成两口并在适配器里重写 owner → 租户 → `LoadFinalRule` 那十来行，用例夹具从 `NewDeclaredStageContent(...)` 改成直接传 owner 与 view，断言不变。

## Comments

- 2026-09-07 · 通道 2：立票（draft），一次 `/domain-modeling` 的产物。**只写票面，未动代码。** 能力边界：读过端口头注、消费编排、`LabelTransaction` 的结果时间、PC 0013 迁移的表名与 `FinalRuleContentView` 声明、ADR-0058 全文、两处 CONTEXT 的相关句；**没读** `FinalRuleContent` 领域类型全文与 `publish_commercial_authority.go` 的 `FinalRuleChannel` 分支细节——「加一格」在那两处怎么落归 PC owner。
- 2026-09-07 · 通道 2（task-b77525c9 ④ 取证）：**`NewJudgeLabelServiceFinalHandler` 无生产入口是 label-channel/11 有意留的，不是本口欠的。** 该票 Answer「不在本票」节明写三个触发点（TF 首次有效收寄事实到达的 PS 侧 inbox 消费者、面单交易定案那一拍、受控关闭/重开决定生效）「各自一张接线票」且「`cmd/parcel-api` / `cmd/parcel-dispatch` 无装配：与上面第一条同落」。**但那三张接线票至今没立**——`unresolved-review-20260904/remaining-work-a3a4814.md`「面单渠道链」第 2 条已记为余工并指出 label-channel spec 没有一张子票承接；本目录不替那边立票（地盘归 label-channel 目录持有者 / MCP-1 派单）。另：lc/11 把有效期规则读口记为「实例登记面，无 PAR 编号，随首个面单渠道产品的实例登记一起立」，本票裁决把它归到终局规则的声明（`PAR-COM-17`），以本票为准——两处口径不同，读到 lc/11 那句的人以这里为新。
- 2026-09-07 · MCP-1 代裁，owner 授权（task-b77525c9，由通道 2 落票面）：**Q1** 起算时刻种类首发只开「渠道结果业务时间」一格；**Q2** 按 CONTEXT 字面归终局规则（接受时固定），不落映射侧——若日后真规则按渠道走，那是新一版声明不是改归属；**Q3** 不在本批，归 `label-channel-service-first-release/20`（第一家真源）。PC 半边并入 PC 批队列（MCP-3 当前批后），PS 半边 Blocked by 它。Status 由 draft 改 blocked。
- 2026-09-10 14:2x · 通道 1（解阻簿记，未动代码；取证于远端 main `5dda0fb2`）：**PC 半边在 main 上了**，PS 半边转 ready-for-agent。对号本票裁决①的 PC 半边三件：有效期声明一格——`final_rule_content` 加 `validity_anchor` / `validity_duration`（迁移 0026，同在同缺 / 时长为正 / 种类封闭三条 CHECK），领域 `ValidityAnchorKind`（首发只 `ChannelResultObservedAnchor` 一格，串值 `CHANNEL_RESULT_OBSERVED`，与 Q1 裁决一致）、`LabelValidityDeclaration`、`FinalRuleContent.Validity()` 与分立构造器 `NewFinalRuleContentWithValidity`；读口——`LoadFinalRule` 读回按在不在场选构造门，**没有这一格就是没有**（`Validity()` 缺席即消费方答 `configured=false`）；发布通道——折进同一 `FinalRuleChannel`，批文 `declarations.finalRuleValidity{anchor, duration}`（ISO-8601 子集）。均在 `d06192ed`；ADR-0119（`838b283e`）越权点 owner 复核 2026-09-09 认可。**PS 半边今天剩一件**：适配器 `adapters/partycommercial/label_validity_rule.go` 实现 `ports.LabelValidityRuleView`——包裹 → `CurrentAcceptedParcelTargetView.FindCurrentAcceptedByParcel` → `AdoptedStageOwner.AcceptanceRulePackageFor` → `LoadFinalRule` → `Validity()`；锚 = `transaction.ResultObservedAt()`；`asOf ≥ 锚 + 时长` 即 `lapsed=true`；找不到委托 / 未固定规则包 / 声明无有效期 → `configured=false`；读口出错 → error。**与生产接线的关系要如实写**：`NewJudgeLabelServiceFinalHandler` 在 `cmd/` 仍零调用方（label-channel/11「不在本票」三张接线票至今未立，`unresolved-review-20260904/remaining-work-a3a4814.md`「面单渠道链」第 2 条），所以本适配器落地后暂无生产装配点可换；适配器包不在接线棘轮扫描范围（门禁只扫 `internal/*/domain`），不必加基线，但完成记录要写一句「无生产调用点，等三张接线票」。派单另记。
- 2026-09-10 14:4x · 通道 6（task-45b75eb2，隔离 worktree 分支 `mcp6-psr01`，基 `dd5ed934`）：PS 半边落地，`560f51a7` + `c9ba714f` + `1a23f5b2`，Status 转 resolved，细节全在上方「完成记录」。开工前与落笔前各排过一次队列，期间只有通道 1 的派单结果广播，无撞号。等非作者评审与推送方重放；进 main 的 SHA 由推送方补记。
- **评审 ← 通道 1（推送方自跑 `/code-review`，非作者；3 / 4 / 5 各在立票 / 只读任务上，2 在 psr/03）· 钉 `14db4a1a` · 15:0x**。子代理不可用（`Task` 报认证错），两轴由本窗口串行、互不引用。
  - **Standards**：**0 阻断 / 0 非阻断**。核过：注释全中文、只写取舍与依据；跨文件引用用 ADR-0119 Decision 一 / 二 / 六 的编号标题（有标签的节，不是无标签列表项的序位；Decision 六首条正是「不得因为终局规则其它行在场就把有效期当成已配置」）与 ADR-0025 / 0062，无行号、无对别处的计数（`1a23f5b2` 那笔已自剪）；无 `time.Now` / `Clock`、无时长字面量（夹具 `72h` 明写非租户声明）；构造器拒 nil 与 `ErrUntranslatableAnswer` 用法照包内 `judgment_as_of.go` / `NewControlDispositionAdapter` 先例；`validityAnchorOf` 单 case + default 报错是 ADR-0119 Decision 二写明的扩展点，不算 Speculative Generality；无 Duplicated Code——恰恰是复用 `FinalContentSource` 免掉了 owner → 租户 → `LoadFinalRule` 的第二份。
  - **Spec**：**0 阻断 / 1 非阻断**。裁决① PS 半边逐格对上：`FindCurrentAcceptedByParcel` 用调用方租户 → `FinalContentFor`（= `AcceptanceRulePackageFor` → `pcTenantOf` → `LoadFinalRule`，读的是接受时固定那版）→ `Validity()` 第二值分辨缺席（`if !declared` 在 found 之后、锚翻译之前，判断题 1 核实）→ `!asOf.Before(锚 + Duration())` 即「≥」。六例全在且各多一格断言（读口点几次），加三例合理。判断题 4「两口而非三口」：**等价，认可**——`DeclaredStageContent.FinalContentFor` 就是派单那两跳，夹具 `NewDeclaredStageContent(nil, finalView, nil, owner)` 走的是真实路径，不是替身镜像。**非阻断 1**（判断题 3(a)）：`ResultObservedAt` 零值答 `ErrUntranslatableAnswer` 而非「没结果就没失效」——认可作者口径：消费方 `lapsedTransactions` 只对 `result.Accepted()` 的结果问本口，零值锚到这里是调用契约被破，error 让装配缺陷可见（端口头注「依赖调不通作为错误返回」同一精神，ADR-0025 不吸收）；不改。判断题 2 核实：用例时刻全在过去，「未过期」两例绿即不拿墙钟的证据。未做规格外的事：未动既有文件、`ports.go`、`cmd/**`、`internal/partycommercial/**`；未装没人调的编排；完成记录写了「无生产调用点，等三张接线票」。
- **进 main 记录 · 通道 1 · 2026-09-10 15:1x**：分支 `mcp6-psr01` 五笔在 `3f485e97` 上 cherry-pick 全干净——`560f51a7→0a603cc8`（适配器 + 测试 + 票面 in-progress）/ `c9ba714f→a557485a`（清点）/ `1a23f5b2→bc67de03`（注释去计数）/ `ff9af9bd→e9d23fd1`（完成记录）/ `14db4a1a→25fc0e82`（Status 措辞）；作者清点笔**未跳过**（`dd5ed934`→`3f485e97` 之间 main 只动 `.scratch/**`，清点在 tip 重生成与之零差，所以直接沿用）。验证钉 `25fc0e82`（隔离 detached 树 `%TEMP%\idp-replay-psr01`）：`gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；清点在 tip 重生成零差；占 55432 广播后含 DSN `go test -p 1 -count=1 ./...` 退 0，**104 ok / 0 FAIL / 15 无测试 / 0 cached**（119 s）；探针 `settlementaccounting/adapters/postgres -run TestFreezeScopesAreInvisibleToEachOther -v` PASS 非 SKIP；新适配器包 `-run 'Validity|Lapse|Untranslatable' -v` 8 例 PASS；释号。本笔（票面两处 + tasks.md）在 `25fc0e82` 之上，纯 .md，自审；`ls-remote` 核 `3f485e97` 未动后 ff 并 `push <sha>:main`，SHA 见推后广播。分支指针改 `merged/mcp6-psr01`、远端删、树由作者拆。
