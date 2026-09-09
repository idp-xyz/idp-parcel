# ADR-0124: 评价回放的治理触发面是 `parcel-api` 端点表上的一条命令行（`POST /pricing-evaluation-replays`）带未配置格，不开 CLI；新评价引用与证据层级由触发方声明、不给默认；回放结果不交结算

Status: Accepted（2026-09-07，用户经 IDP 队列通道 1 派单 `task-269d98b6` 授权 owner「自决口径：硬句一字不改、每个决定写理由、拿不准或跨上下文归属的点单列越权风险点」，据此对 [wiring-baseline-remainder/06](../../.scratch/wiring-baseline-remainder/issues/06-pp-replay-pricing-evaluation-has-no-executor.md) 第三件「治理触发面」的形状作出裁决。裁决能力边界见文末）
Date: 2026-09-07

## Context

[PP CONTEXT](../domain/parcel-pricing/CONTEXT.md) 里有好几条只由 `ReplayPricingEvaluation` 执行的硬句：「重放必须使用新的评价引用，不得复用原评价 ID，不修改原评价或来源事实。重算结果与原评价不一致时，结果为冲突，不得当作成功回放」「重放按原评价记录的规范化版本重新规范化后再比对摘要……无法重新规范化时，结果为未形成」「回放以原版本清单必须重现不可计价」「回放执行记录按其证据来源标记为 `S`、`R` 或 `P`，合成输入只能标记为 `S`」。票 06 立票时（2026-09-07，MCP-6）量到：这扇门全仓没有生产调用点，`EvidenceKind` 里那格 `EvidenceReplay = "R"` 没有任何生产代码造出过，`ports.PriceCardCatalog` 只有按（方向 + 适用范围 + 计价基准时点）选卡的 `LoadApplicable`——重放要的是「原评价用的那一版」而不是「此刻适用的那一版」，方案换版之后两者不是同一张。

票面把缺口拆成三件：按版本引用取回原方案的读口、回放编排、治理触发面。前两件已随本票落地（`ports.PriceCardVersionLoader.FindByReference`、`application.ReplayPricingEvaluationHandler`），形状与理由写在各自头注；第三件的形状票面留给裁决：**HTTP 登记面 + 未配置即拒（ADR-0055 / ADR-0085 同形），还是 `cmd/parcel-*` 一族的 CLI**。票面只钉了一句：「它是治理面不是客户面」。

裁之前核出的几件事（实测于 `aad985cf`，只作此刻取证）：

- 谁会按这扇门。两类触发者，都不是客户：PN-08 `W02`「历史回放」——[交接](../design/pn-08-end-to-end-pilot-and-stage-admission-development-handoff.md)要求「使用可追溯、脱敏、版本化的历史来源事实，按对象和业务时间重放候选规则」，结果记 `R`；[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)把 PN-08 分成「记录能力是机制，实际执行回放……是实例」。CONTEXT 点名的「争议复核」走同一扇门，触发者是运营操作者。
- 触发方必须给什么。原评价引用、**新**评价引用、证据层级；租户。四样里没有一样是执行器自己能知道的：新引用是 CONTEXT 硬句要求的、证据层级是回放数据来源的属性、租户是隔离边界。
- 本仓写面今天全走一条缝。ADR-0085 让登记册配置写面进端点表带 `UnconfiguredIntake{}`；ADR-0099 决定二让**治理动作**（序列版本复核）也走这条缝，路径叫 `-reviews` 不叫 `-registrations`；ADR-0113 对总单登记裁了「登记口沿 `parcel-api` 端点不开 CLI」。ADR-0100 把运营操作者身份判为产品自有的接入渠道族——操作者面的真 Intake 现在就能立，不等 `PAR-INT-01`；ADR-0101 把操作者面的载荷形状判给产品定义。
- CLI 一族今天是什么。`cmd/parcel-pricing-register` 头注自述「受控批量口」（ADR-0085 决定一、ADR-0100 决定六）：种子与补录历史走它，答案代数与在线口一致。它从文件里读登记快照，**登记责任方 / 批准责任方都在文件里**——那是受控流程在系统外担的责任，不是产品采信的身份。
- 结算那一侧怎么认评价。`UC-SA-002` 的 `AT-SA-173`：相同输入快照、方向、目的和版本清单重复请求「引用同一 `PricingEvaluation` 并幂等返回既有费用采用」——幂等键是评价标识。一份带**新**引用的回放在 SA 眼里就是另一次评价。

## Decision

**一、治理触发面是 `parcel-api` 端点表上的一条命令行：`POST /pricing-evaluation-replays`，装配以字面量 `UnconfiguredIntake{}` 起步。** 写准入不另立形（ADR-0085 决定二）：它与其余命令面共用同一条 Intake 缝，未配置即拒（403 `ACCESS_CHANNEL_NOT_CONFIGURED`），真操作者 Intake 就位时在装配点换这一行。路径叫 `-replays` 不叫 `-registrations`：本上下文这一格的动词是回放，答案代数说的也是回放，叫成登记会与登记那一族的答案混为一谈——判据同 ADR-0099 决定二让复核叫 `-reviews`。

理由是**身份缝**：触发方是谁（谁发起了这次争议复核、谁在 W02 执行）来自 ADR-0100 的 `OperatorEnvelope`，后端不采信自报身份；端点的 Intake 缝正是信封落地的地方。CLI 没有这条缝——它只能从文件里读一个「触发者」，那是自报身份，恰是 ADR-0100 / ADR-0101 刚把操作者面从上面挪开的东西。登记 CLI 能这么做，是因为它的定位是系统外受控流程的批量口；回放不是批量口的第一个调用方（见二）。

**二、不开 CLI，也不给 `parcel-pricing-register` 加一种 `kind`。** W02 按候选范围逐对象批量重放是**实例半边**（开发主线：「实际执行回放……是实例」），它的输入——脱敏历史事实、候选版本组、回放数据范围——都是 `PAR-GOV-01` 待提供的实例内容。今天造一个批量驱动器，等于替租户拟回放范围的样子；它也没有任何真输入可跑。W02 真要一个受控批量口时，让它消费同一个 `ReplayPricingEvaluationHandler`（ADR-0085 决定一「两口消费同一用例，答案代数一致」那个形状），本记录不预建、不禁止。

**三、命令三样都由触发方声明，一样都不给默认。** 载荷带原评价引用、新评价引用、证据层级；租户只从信封来，载荷里没有这一格。

- **新评价引用由触发方铸**，它同时是幂等键：同一引用第二次到达返原记录（`EXISTING_RESULT`），同一引用装了别的原评价或别的层级是冒名（`IDENTITY_CONFLICT`），原记录不顶替——与评价请求自带标识（`EvaluatePricingHandler` 的「同标识同语义返原、异语义冲突」）是同一纪律。执行器自己铸引用，每次重试都会多出一条回放记录，幂等无从谈起。
- **证据层级由触发方声明**：它是回放数据来源的属性——W02 隔离执行下按脱敏历史事实重放是 `R`，合成输入只能是 `S`，执行器不知道来源是什么。面上不填 `S` 当默认：那个默认在今天走得到的每一个环境里都对、在 W02 要去的那个环境里恰好错，AGENTS 红线两个方向都禁。结构上守得住的只有一件——`S` 不得升级——由领域门 `NewReplayEvaluationRequest` 守，面与编排不重写这条判据。
- 缺任何一样是畸形请求（400 `MALFORMED_REQUEST`），不是「未配置」也不是业务答案。

**四、答案代数只说编排，不替评价说话。** 面上的 `outcome` 八格：`RECORDED` / `EXISTING_RESULT` / `IDENTITY_CONFLICT` / `ORIGINAL_NOT_FOUND` / `PLAN_VERSION_NOT_ON_REGISTER` / `PLAN_CANONICALIZATION_UNSUPPORTED` / `NOT_ACCEPTED` / `UNDECIDED`。重放重现了、冲突了还是结构上算不出来，在回放评价自己的 `status` 与问题项里（`COMPLETED` / `CONFLICT` + `REPLAY_RESULT_MISMATCH` 等 / `FAILED` + `CANONICALIZATION_VERSION_UNSUPPORTED`），随响应原样透出，面上不再折一遍——折了就有两处权威。两格「结构上重放不了」分开是按恢复动作（ADR-0029）：原方案版本不在册，登记那一版就推得动；原方案快照按另一套规范化形状折装、本构建重建不了（ADR-0014），只有支持该形状的构建推得动。状态码照 ADR-0022：`RECORDED` 201，其余形成了的答案 200，依赖故障 500 `NO_ANSWER_FORMED` 不带 `outcome`。

**五、回放结果不交 `EvaluationHandoff`，且这一格不在结构上。** `ReplayPricingEvaluationDeps` 没有交付口那一格，装配点装不进去——比一句「不要调」的注释硬。理由是 `AT-SA-173`：SA 的费用采用按评价标识幂等，一份带新引用的回放交出去就是一笔重复的费用采用。争议复核要读回放结果，从评价册读（回放评价是入册的版本化事实），没有任何东西把它推给下游。

**六、不在本记录。** W02 的批量驱动、范围选择与差异分类（缺失 / 额外 / 不同 / 迟到 / 无法解释 / 仅展示）——那是 PN-08 交接的 W02 交付，实例半边；争议复核由谁发起、凭什么——SA / PC 侧另裁（票面边界）；管理台的回放页；本端点的真操作者 Intake——随 ADR-0100 那一族在装配点逐端点换，与其余写面同一批；评价册查阅面要不要透出 `replayOf`——另立票。

## Consequences

- `internal/parcelpricing/adapters/http` 增 `EvaluationReplayIntake`（接口，未配置实现由 `UnconfiguredIntake` 兼任）、`NewReplayEvaluationEndpoint`、载荷解码 `DecodeEvaluationReplayPayload`（逐字段表单——回放低频、结构简单，ADR-0101 决定八自裁）与封闭响应形状；`cmd/parcel-api` 端点表加一行、`assemble_pricing_replay.go` 事务包装与真库探针、`unwired_orchestration.go` 占位一格。
- 生产装配里这条端点在操作者 Intake 就位前一律答 403；`S` 因此仍是唯一走得到的证据层级，`R` 只在 W02 隔离执行下形成——那是实例半边，本记录不缩短任何等待。
- 基线 `production_wiring_baseline.txt` 的 `ReplayPricingEvaluation` 行已随编排落地剪掉；棘轮只量导出工厂，「面接没接进端点表」它量不到，钉住这件的是 `cmd/parcel-api/endpoints_test.go` 的探针表。
- 不改 PP CONTEXT 任何硬句、不加词条：回放的语言（新引用、原版本清单、冲突、未形成、`S`/`R`/`P`）CONTEXT 里已经齐了，本记录只裁触发面的形状与三个「不给默认」。

## Alternatives considered

- **CLI（`cmd/parcel-pricing-replay`，或 `parcel-pricing-register` 加一种 `kind`）。** 否决（现在）：没有身份缝，触发者只能自报；W02 批量执行是实例半边、今天无输入可跑，先造驱动器就得先替租户拟范围；ADR-0085 已否决过「写面长期只走 CLI」。将来作受控批量口消费同一用例不在否决之列。
- **HTTP 与 CLI 同时立。** 否决：CLI 那一半此刻既无输入也无调用方，立出来只是一条没人走的路，且要为它拟一个回放范围的形状。
- **执行器铸新评价引用。** 否决：重试即多一条记录，`EXISTING_RESULT` 那一格从此没有意义；本仓的幂等一律靠调用方带键。
- **面上把证据层级默认为 `S`。** 否决：今天处处都对、W02 那一处恰好错的默认；且它会让「触发方没说」与「触发方说了 `S`」在记录上不可分辨。
- **交 `EvaluationHandoff` 并带一个「这是回放」的标记让 SA 跳过。** 否决：失效方向是重复费用采用；让 SA 学会跳过是把一条本上下文的边界规则塞进邻接上下文；把那一格从依赖里拿掉是编译期保证。
- **做成 `EvaluatePricingHandler` 的一种模式（请求带 `replayOf`）。** 否决：那个编排交下游、解析在用序列与目录版本，回放两件都不得做；两种行为塞进一个编排，每一步都要分叉。

## 裁决方的能力边界

本记录由 MCP-2 受用户经通道 1 派单授权自决。读过：票 06 全文；PP CONTEXT「Rules and invariants」与「Lifecycles · 价格评价」；ADR-0014 / 0022 / 0029 / 0055 / 0077 / 0085 / 0099 / 0100（Decision 各条标题）/ 0101（Decision 各条）/ 0113（标题与 Context）；PN-08 交接 `W02` 一节与阶段表；开发主线对 PN-08 机制 / 实例的划分句；`PAR-GOV-01/02` 两行；`domain/evaluation.go` 的 `ReplayPricingEvaluation` / `NewReplayEvaluationRequest` / `EvaluatePricing` 头几道门；`fingerprint.go` 的语义摘要构成（不含 `replayOf` 与证据层级）；`ports/price_card_catalog.go`、`ports/reference_series_register.go`、`adapters/postgres/price_card_catalog.go`、`adapters/postgres/evaluation.go`；`application/evaluate_pricing.go`、`review_reference_series.go`；`adapters/http/register_price_card.go`、`review_reference_series.go`、`unconfigured_intake.go`、`query_evaluations.go`；`cmd/parcel-api` 的 `endpoints.go` PP 组行、`assemble_pricing_registration.go`、`endpoints_test.go`、`unwired_orchestration.go` PP 段；`cmd/parcel-pricing-register/main.go`；`production_wiring_baseline.txt` 头注纪律与 PP 段。**没有读**：SA 消费 `OutboxEvaluationHandoff` 的适配器正文（决定五引的是 `AT-SA-173` 的验收句，不是 SA 代码）；`network-routing` 对候选评价的处理；管理台任何页面；`internal/accessidentity` 的 `OperatorEnvelope` 正文（决定一只引 ADR-0100 的裁决）。

因此本记录裁的是**触发面走哪条缝、命令三样由谁给、结果去不去结算**。它没有裁 W02 的批量形状、争议复核的业务触发规则、也没有裁管理台表单。越权风险点单列供复核：（1）决定五引 `AT-SA-173` 判「回放交出去会被采用成第二笔费用」是从 SA 验收句推的，没读 SA 消费适配器怎么认 `replayOf`——若 SA 侧已按 `replayOf` 跳过，本决定仍成立（不交更安全），但理由要改口；（2）决定三让触发方声明证据层级，等于把「这次回放的来源算不算 `R`」这个判断交给操作者面——`R` 该由谁认定属 PN-08 治理（`W09` 评审）与 `PAR-GOV-01`，本记录只保证机制不替他们默认。

## Links

- [wiring-baseline-remainder/06](../../.scratch/wiring-baseline-remainder/issues/06-pp-replay-pricing-evaluation-has-no-executor.md)：三件与「治理面不是客户面」那句的出处
- [PP CONTEXT](../domain/parcel-pricing/CONTEXT.md)：重放各硬句与 `S`/`R`/`P` 标记规则
- [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)、[ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)：未配置格与「写准入不另立形」
- [ADR-0099](./0099-price-card-binds-series-identity-and-in-force-version-is-derived-from-review.md)：治理动作走端点表、路径按动词命名的先例（决定二）；重放不重新解析在用版本（决定四）
- [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)、[ADR-0101](./0101-operator-facing-registration-payload-shape-is-product-defined.md)：操作者身份与操作者面载荷形状归产品——决定一、三的依据
- [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：规范化不支持是未形成不是冲突——决定四第二格
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)、[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：状态码与按恢复动作分格
- [ADR-0113](./0113-carrier-master-document-is-an-independent-register-keyed-by-declared-reference-and-version.md)：「登记口沿 `parcel-api` 端点不开 CLI」的先例
- [UC-SA-002](../application/settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md)：`AT-SA-173`——决定五的依据
- [PN-08 交接](../design/pn-08-end-to-end-pilot-and-stage-admission-development-handoff.md)、[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)、[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)：W02 交付、机制 / 实例划分、`PAR-GOV-01`

## owner 复核记录

- owner 复核 2026-09-09 认可（用户 2026-09-09 12:3x 经 IDP 队列通道 1 授权「你自决，目标是全部解决」，通道 1 代裁，两条逐条）：1. 决定五「回放不交结算」——理由不用改口：`internal/settlementaccounting` 全文 `replayOf` 零命中（09-09 grep），SA 今天没有任何按回放引用跳过的处置，交出去就是第二笔费用；即便日后 SA 加了，不交仍更安全；2. 证据层级由触发方声明——机制不替 PN-08 治理与 `PAR-GOV-01` 默认「算不算 R」，声明缺席即不是 R。票 wiring-baseline-remainder/06 的两处越权点即此。