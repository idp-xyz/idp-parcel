# 派送要求缝三：交付条件引用（`party-commercial`）——`DeliveryConditionSource` 的消费侧适配器

Category: enhancement
Status: resolved——2026-09-10 16:4x 通道 2（单 task-1d5f0b15-49d5-46af-afdb-436f0af8383d），分支 `mcp2-tf14` 基 `ffdf1d5c`（rebase 自 `0c9b846f`，tf/13 在内），代码 tip `595c659c`，完成记录见 Comments 末条；待推送方派非作者评审后重放进 main。此前 in-progress（15:5x 通道 2 认领）、ready-for-agent——三问已裁（2026-09-09，通道 2，task-f5521768，用户授权 PC owner 口径代裁、第 2 问连 PS 口径；见「裁决」，理由、场景与被否替代在 [ADR-0133](../../../docs/adr/0133-delivery-condition-reference-is-the-acceptance-time-commercial-resolution-reference.md)）；做法与完成判据见下。此前 draft——由票 [09](09-arrival-triggers-dispatch-task.md) 裁决④与 ADR-0114 决定三/四拆出（2026-09-07，通道 4，task-79675845）
Blocked by: [ps-port-remainder/07](../../ps-port-remainder/issues/07-commercial-resolution-reference-by-parcel-read-face.md)（PS 按包裹身份答商业解析回指）、[pc-gaps/11](../../party-commercial-context-gaps/issues/11-delivery-condition-declaration-family-and-resolution-keyed-read-face.md)（PC 交付条件声明族 + 按回指答有没有的读口）——两口没落之前本票只能写替身，不能接真

## 缺口

末端派送任务七件里的**条件**（交付方式、收件范围、合同责任）归 `party-commercial`（UC-TF-006 输入契约同一行；票 09 裁决④——`PAR-NET-09` 实例半边，机制只传引用）。TF 侧端口 `ports.DeliveryConditionSource.LoadDeliveryConditions(tenant, object) → (conditions, resolution)` 已随执行器立住：交回的是**条件引用**（落进任务的 `ServiceConditionReference`——「产品、合同与授权处置规则的快照」引用），不是条件内容；哪些交付方式被允许、收件范围怎么算，是 UC-TF-006 步骤 5「按服务/合同规则判断有效交付」那一步读引用去取的事，不在建任务这一拍展开。

PC 侧有相邻的东西：服务阶段正文里的责任结果声明（`DeclaredEffectiveDelivery`，ADR-0058）、客户合同版本与合同委派（ADR-0116）。但没有一个按包裹或按委托答「这个对象适用的交付条件版本引用」的读面；对象到合同的那一跳也不在 PC——委托采用了哪个合同版本是 PS 的商业依据解析结果（ADR-0080：商业闭环先解析合同再据它键结算）。缝要跨两个所有者才能接上。

## 所有者

- 交付方式、收件范围、合同责任的版本生命周期：`party-commercial`。TF 保存采用的条件引用快照，不修改商业版本。
- 对象 → 委托 → 采用的合同版本：`parcel-shipment`（商业依据解析）。TF 不自己算这一跳。
- `internal/transportfulfillment/adapters/partycommercial/` 今天已有一个消费侧适配器（承运主体身份目录，ADR-0103）；本缝的适配器与它同包并列，各答各的，不合并成一个「PC 门面」。

## 缝的形状（TF 这一头已定）

- 端口：`internal/transportfulfillment/ports/delivery_requirement.go` 的 `DeliveryConditionSource`。
- 适配器落位：`internal/transportfulfillment/adapters/partycommercial/`（ADR-0025 消费侧；只翻译不判断；全函数）。若对象→合同那一跳要先问 PS，适配器可能同时依赖 PS 的一个读面——那仍是「TF 消费两个提供方」，两份翻译各自在 TF 侧，不让 PC 去读 PS。
- 消费方：`application.TriggerDeliveryDispatchHandler`。缺席答 `DISPATCH_UNDECIDED` / `DELIVERY_CONDITION_SOURCE_NOT_WIRED`；PC 答「没有」（合同没登交付条件、或对象没有采用的合同版本）答 `REQUIREMENT_MISSING` / `DELIVERY_CONDITION`，任务保持待形成，不填默认——「不以通用签名规则替代合同」（UC-TF-006 输入契约使用约束）在这一拍就成立。

## 未接时 TF 停在哪

票 [12](12-delivery-place-reference-seam-parcel-shipment.md)、[13](13-delivery-window-seam-network-routing.md) 都接上之后，每一拍停在 `DELIVERY_CONDITION_SOURCE_NOT_WIRED`。停点语义同 12。

## 要裁的（PC owner，其中第 2 问要 PS owner 一起）

1. **条件引用指哪一版。** 候选：(a) 委托接受时解析到的合同版本引用（同版性：与 PS 商业闭环键结算用的是同一个版本，ADR-0080）；(b) 对象进派送段那一拍当前有效的合同版本；(c) 合同版本之下更细的「交付条件」子版本（若 PC 把交付方式允许集独立成版本）。任务的 `Conditions` 是快照引用，取 (a) 与「快照」一词一致；取 (b) 会让同一委托的不同包裹在不同拍拿到不同版本。
2. **对象怎么走到合同。** PS 的商业依据解析结果按什么键读（委托 / 接受基线 / 包裹）；那是 PS owner 的读面。本缝的适配器只翻译两头的答复，不自己解析商业依据。
3. **合同委派（ADR-0116）在这一格怎么读。** 委派让实际决定方与合同相对方不同；条件引用是指合同版本本身还是指委派后的决定版本，影响一线作业端拿引用去 PC 取内容时取到谁的规则。

## 裁决（2026-09-09，通道 2，task-f5521768；用户授权 PC owner 口径代裁、第 2 问连 PS 口径，理由、场景与被否替代在 ADR-0133，此处只记答案）

1. **条件引用指哪一版 → (a) 的方向，写成接受时固定的商业解析回指，不是裸合同版本串。** 条件引用 = 该包裹所属委托接受时固定的 `CommercialResolutionID`（PS 接受决定上的回指，PC 按它持有闭包）；读内容时按回指到闭包取已采用的**服务产品版本与客户合同版本**上的交付条件。不取 (b)：撞客户合同版本生命周期「不改变已经接受委托所保存的合同依据」，且同一委托的包裹会在不同拍拿到不同版。不取裸合同串：交付条件不只在合同上（TF 硬句「适用服务产品、国家地区和客户合同允许」）、两段式串只许 PC 一处拼（ADR-0080 决定七）、TF 给 `ServiceConditionReference` 的注释本就是「产品、合同与授权处置规则的快照」。不取 (c)：今天没有正文，不在空处先造版本身份。
2. **对象怎么走到合同 → PS 按（租户，包裹身份）答商业解析回指，PC 按（租户，回指）答交付条件引用；两个「没有」各答各的。** 引 ADR-0130 决定二那句：**「PS 对外一律按（租户，包裹身份）答，不要求消费方持有委托、接受基线或提交版本；委托与接受基线是 PS 内部走到答案的路，不是键。」** PS 三格：回指 / 不属任何已接受委托（含集运单元、不可见对象）按统一不可见答「没有」/ 已接受却无回指 error。PC 四格：交付条件引用（就是这个回指）/ 闭包在场而产品与合同两层都无交付条件声明 → 「没有交付条件」/ 闭包不在场 error / 闭包未采用客户合同版本 error。PS 的「没有」（对象无采用的合同）与 PC 的「没有」（合同无条件）恢复动作不同（ADR-0029），适配器里各一行、不并。
3. **合同委派在这一格怎么读 → 不改条件引用指向谁。** 委派只答某一授权动作「谁能替谁决定」（ADR-0116），交付条件不是授权动作；一线作业端拿引用去 PC 取到的是那一版合同（连同产品版本）本身声明的交付条件，与谁被委派无关。交付现场若日后长出需要客户决定的动作，走 `AuthorizedAction` 加格 + 委派那条路，不进条件引用。

**顺带裁定（决定四，归属；越权风险点 2 供 owner 复核）**：交付条件本体是服务产品版本声明、客户合同版本只能收紧的商业条件（保价条件先例），不归接单规则包版本（ADR-0058 那一族）；PC 今天没有这一族，没登就是没有——所以今天每一份都会答「没有交付条件」，那是真话。

越权风险点五条单列在 ADR-0133，与本票实施直接相关的是 1（两个「没有」在 TF 结果上要不要分格——TF 地盘，本票实施时定）与 3（闭包机制接通前 PC 那一格会对每个对象 error 而不是「没有」）。

## 做法（顺序固定）

1. **等两只提供方读口进 main**：[ps-port-remainder/07](../../ps-port-remainder/issues/07-commercial-resolution-reference-by-parcel-read-face.md)（PS：包裹身份 → 回指三格）与 [pc-gaps/11](../../party-commercial-context-gaps/issues/11-delivery-condition-declaration-family-and-resolution-keyed-read-face.md)（PC：回指 → 交付条件引用四格）。没落之前本票可以先写 TF 侧替身与执行器用例，不能接真。
2. **先定两个「没有」在 TF 结果上的落法（ADR-0133 越权风险点 1，TF owner 一句话）**：甲——两行都译成 `RequirementMissing`，`REQUIREMENT_MISSING / DELIVERY_CONDITION` 一格，适配器头注写明两行分别来自哪个所有者；乙——执行器结果多带一格子原因（`OBJECT_HAS_NO_CONTRACT` / `CONTRACT_HAS_NO_DELIVERY_CONDITIONS`），端口代数不动。**默认甲**（票 12 的「未定」那格也走了甲；子原因是 TF 结果形状的事，等三条缝都接上再看要不要统一加）。
3. **适配器** `internal/transportfulfillment/adapters/partycommercial/delivery_condition_source.go`（与 `carrier_identity_directory.go` 同包并列，各答各的，不合并成「PC 门面」）：依赖两个**窄接口**（本包内声明、各只含用到的那一个方法，先例 `BusinessPartySource` / `LegalEntitySource`）——PS 的回指读口与 PC 的交付条件读口；不 import 任何一侧的 `application`。**全函数翻译**，两段各自封闭、不留 `default`：PS 回指 → 进第二段；PS 没有 → `RequirementMissing`（第 2 步甲 / 乙）；PS error → 原样上抛（执行器落 `DELIVERY_CONDITION_SOURCE_UNAVAILABLE`）；PC 交付条件引用 → `RequirementResolved` + 回指的 `String()`；PC 没有交付条件 → `RequirementMissing`；PC 两格 error → 原样上抛；任一侧集外取值 → `ErrUntranslatableAnswer`（本包既有哨兵）。`CarriedObjectReference` → PS 包裹身份、PS 回指 → PC `ResolutionID` 两处翻译都在本适配器内做，两个提供方互不认识对方的词。
4. **装配**：`cmd/parcel-api` 把 `DeliveryConditionSource` 那一格从空换成真适配器（两只读口都要给，缺一只装配期拒——判据同 `NewCarrierIdentityDirectory`「缺一本那一支永远答不出」）；装配测试补一例——生产装配下执行器不再答 `DELIVERY_CONDITION_SOURCE_NOT_WIRED`，停点按 12 / 13 的进度后移或走到 `OpenDispatchTask`。
5. **生产入口那一格**：本票开工时若票 12 / 13 的实施都还没开工，执行器的生产入口归本票，落法同票 12 做法第 5 步（谁按拍调执行器、拍频作配置不作默认、`cmd/parcel-api` 装配接线，拍频取值实例半边留空答「未配置」）；若 12 或 13 先开了，本票不重立，只在完成记录写一句归谁。**取证于 2026-09-09 23:5x**：12 / 13 都尚未开工（12 ready-for-agent Blocked by ps-port-remainder/06；13 在通道 5 代裁中），归属仍未定，谁先开工谁拿。
6. **文档**：TF CONTEXT 不改（「派送要求」词条已写「交付方式、收件范围与合同责任的条件引用归 `party-commercial`」）；ADR-0114 / 0130 / 0133 / 0080 / 0116 正文不改；本票完成记录写 TF 侧 SHA、两只提供方读口进 main 的 SHA、第 2 步取甲还是乙、生产入口归谁。

## 完成判据

1. 适配器测试覆盖两段封闭集各格：PS 三格 × PC 四格里可达的组合各一例（PS 没有 / error 不进第二段），加两侧集外取值各一例；断言交回的 `conditions` 串与 PS 回指的 `String()` 逐字相等（不在 TF 侧重拼）。
2. 执行器用例补两例：PC 答交付条件引用 → 走到 `OpenDispatchTask`（12 / 13 已接）或停在它们的格（未接）；PS 答「没有」（集运单元）→ `REQUIREMENT_MISSING / DELIVERY_CONDITION`，任务待形成、续办引用非空、段与交接一行不动。
3. `cmd/parcel-api` 装配测试一例（做法第 4 步）。
4. 红线逐条守住：只传引用不传内容（允许集、证据规则一字不进 TF）；PS 与 PC 的两个「没有」不并成一格；不动 `internal/partycommercial/**` 与 `internal/parcelshipment/**`；不写真实合同、真实交付方式取值。
5. 验证（作者层）：gofmt 空、`go build ./...` / `go vet ./...` 0、`go test -count=1` TF `adapters/partycommercial` + `application` + `./cmd/parcel-api/`（带 DSN）+ `./internal/architecture/...`；清点在干净检出重生成。

## 生产入口

同票 12「生产入口」一节：随第一条接上线的缝的实施票立；本票若先开，那一格归本票。落法与取证见「做法」第 5 步。

## 红线

- 只传引用不传内容：交付方式允许集、收件范围规则是 `PAR-NET-09` / `PAR-COM-05/06` 实例半边，一个字都不进 TF。
- 只翻译不判断：不把「合同未登交付条件」读成「按默认签收」，不合并 PS 与 PC 的两个「没有」成一个。
- 不动 `internal/partycommercial/**` 与 `internal/parcelshipment/**`（本票只立票；两侧读面若要开，由各自 owner 在其地盘立票）。
- 不写真实合同、真实交付方式取值。

## Comments

- 2026-09-07 · 通道 4（task-79675845）：由票 09 裁决④拆出立票，只写票面，未动代码。
- 2026-09-09 · 通道 2（task-f5521768，用户经队列授权 PC owner 口径代裁、第 2 问连 PS 口径，基 main `dec37d78`）：三问过 `/domain-modeling`（限界上下文 party-commercial，对象→合同那一跳归 parcel-shipment，消费方 transport-fulfillment；三个场景——接受后合同发新版本再派送 / 合同委派让实际决定方≠相对方 / 合同没登交付条件）后裁定，落 ADR-0133 + PC CONTEXT「交付条件」词条与 Rules 一条；两只提供方读口另立 [ps-port-remainder/07](../../ps-port-remainder/issues/07-commercial-resolution-reference-by-parcel-read-face.md) 与 [pc-gaps/11](../../party-commercial-context-gaps/issues/11-delivery-condition-declaration-family-and-resolution-keyed-read-face.md) 作本票 Blocked by；Status draft → ready-for-agent。只裁不码，`internal/**` 一行未动。票 12 / 13 未改。分支 `mcp2-tf14`，进 main 的 SHA 由推送方重放后另记。
- 2026-09-10 00:1x · 通道 2 窗口代推送方（通道 1 会话 00:0x 再崩，用户指令本窗口接听其队列；作者与推送方同会话，纯 .md 按前两任「代裁自审重放」的先例办，未作第二人语义评审）：三笔在 `%TEMP%\idp-replay-tf14` 重放到 `4cfb725a`（= 当时 origin/main，已含 tf/13 封存与 README 0131 行）之上，零冲突——`829e5999→70a7c669`（README 那一笔 patch-id 不等，成因是 0131 行已在 0132 之前、上下文变了；`git diff a6e013cf <tip> -- docs/adr/README.md` 只多 main 的 0131 那一行，内容无差）/ `3b941f4b→2db719f2` / `a6e013cf→25102207`（两对 patch-id 相等）。tip 上 README 0128 → 0130 → 0131 → 0132 → 0133 各一行；`git diff --check` 唯一命中仍是 PC CONTEXT 新词条标题行尾两空格（硬换行约定）；五份新改文件内 .md 相对链接逐一解析存在。三票不动 .go/.sql，Go 真值沿用 `f8fa0398` 那一跑。**进 main 的 SHA 以推送方广播「远端 main = …」为准**（本行写在推之前）。
- 2026-09-10 16:4x · 通道 2（task-1d5f0b15）：**完成记录**，分支 `mcp2-tf14` 基 `ffdf1d5c`（main，tf/13 在内；开工基 `0c9b846f`，tf/13 进 main 后 rebase 一次、`--force-with-lease` 推自己分支），代码 tip `595c659c`，清点 `1accc36d`，本笔票面在其上。
  - **两只提供方读口进 main 的 SHA**：PS `ports.CommercialResolutionReferenceView` 随 psr/07 进 main 于 `c0cdebba`（实现 `pspostgres.ShipmentRequests.LoadCommercialResolutionReference`）；PC `ports.DeliveryConditionView` 随 pc-gaps/11 进 main 于 `62c87e73`（实现 `pcpostgres.NewDeliveryConditions`，哨兵 `pcdomain.ErrDeliveryConditionClosureAbsent` / `ErrDeliveryConditionContractNotAdopted`）。
  - **每笔**：`1e22410f` 适配器 `adapters/partycommercial/delivery_condition_source{,_test}.go` + 票面 in-progress；`e7cd7272`（rebase 前 `da58da1d`）执行器两例 `application/trigger_delivery_dispatch_partycommercial_test.go`（新文件，不动既有测试）；`595c659c` `cmd/parcel-api/assemble_delivery_dispatch.go` 填 `Conditions` + 装配测试改为三缝齐的如实格 + `ports/delivery_requirement.go` 头注一段改准（推送方 16:0x 点名的 tf/13 非阻断 1，端口签名一字未动）；`1accc36d` 清点在 `595c659c` 干净 detached 检出重生成。
  - **做法第 2 步取甲**：PS 的「没有」与 PC 的「没有」都译 `RequirementMissing`，落执行器同一格 `REQUIREMENT_MISSING / DELIVERY_CONDITION`；适配器头注写明两行各来自哪个所有者、恢复动作不同（ADR-0029）——PS 的「没有」去 parcel-shipment 问对象为什么不在册，PC 的「没有」是商业责任方去 party-commercial 登声明。理由照票面默认判据：子原因是 TF 结果形状的事，票 12 的「未定」也走了甲，三条缝都接上后若要统一加子原因另起。
  - **PS 三格 × PC 四格对照**（`adapters/partycommercial.DeliveryConditionSource.LoadDeliveryConditions`）：
    | PS（按租户 + 包裹身份） | PC（按租户 + 回指） | TF 端口答法 | 用例 |
    |---|---|---|---|
    | 回指 | 交付条件引用 | `RequirementResolved` + PS 回指 `String()` 逐字 | `TestAResolutionWithDeclaredConditionsIsHandedOverVerbatimAsResolved` |
    | 回指 | 没有交付条件 | `RequirementMissing`（PC 的「没有」） | `TestAContractWithoutDeliveryConditionsIsHandedOverAsMissing` |
    | 回指 | `ErrDeliveryConditionClosureAbsent` | error 原样上抛（执行器落 `DELIVERY_CONDITION_SOURCE_UNAVAILABLE`） | `TestAPartyCommercialFailureIsPropagatedNotReadAsNoConditions/closure absent` |
    | 回指 | `ErrDeliveryConditionContractNotAdopted` | error 原样上抛 | 同上 `/contract not adopted` |
    | 回指 | 读面坏了 | error 原样上抛 | 同上 `/read face down` |
    | 回指 | declared 却零值引用 / 引用指另一份闭包 | `ErrUntranslatableAnswer` | `TestAnAnswerOutsideEitherClosedSetIsUntranslatable`（PC 两子例） |
    | 没有（含集运单元、不可见、仅已提交） | **不问** | `RequirementMissing`（PS 的「没有」） | `TestAnObjectWithoutAnAdoptedContractIsMissingWithoutAskingPartyCommercial` |
    | error（`ErrAcceptedWithoutCommercialResolution` / `ErrAmbiguousParcelTarget` / 读面坏了） | 不问 | error 原样上抛 | `TestAParcelShipmentFailureIsPropagatedBeforeAskingPartyCommercial` |
    | present 却零值回指 | 不问 | `ErrUntranslatableAnswer` | `TestAnAnswerOutsideEitherClosedSetIsUntranslatable/PS present but zero resolution` |
    两处键翻译都在适配器内：`CarriedObjectReference` → PS `DeclaredParcelID` 同一串字面（不分种类——`CarriedObjectReference` 今天没有种类维，集运单元经 PS 答没有；不在本票立种类维，若要 TF 侧先分流另立 TF 小票让地点与条件两缝同时短路）；PS `CommercialResolutionID` → PC `ResolutionID` 经 `pcdomain.NewResolutionID`（ADR-0027 回指入口），不拆不拼。窄口 `CommercialResolutionReferenceSource` / `DeliveryConditionReferenceSource` 本包内声明各只含一法，不 import 任一侧 application / ports。PC 那一侧的测试引用经 PC 公开领域门从一份唯一解析、采用客户合同版本的闭包造出（`ResolveCommercialClosure` + `DeliveryConditionReferenceFor`），不在 TF 测试里拼字面。
  - **生产入口归 tf/12**（做法第 5 步）：端点 `/transport-fulfillment-delivery-dispatch-triggers` + `UnconfiguredIntake{}` 已在 main（`0c9b846f`）；本票不重立，只填 `buildDeliveryDispatchTrigger` 的 `Conditions` 一格（两只窄口都给，缺一只装配期拒）。
  - **停点实际后移到哪**：三缝齐后 `*_SOURCE_NOT_WIRED` 不再出现。装配测试（真库、真编排）：对象凭已交接进 `FINAL_DELIVERY` 段，空库里 PS（地点、回指）与 NR（计划段）各答「没有」，执行器一拍走到向三个所有者取七件，答 `REQUIREMENT_MISSING [DELIVERY_PLACE DELIVERY_WINDOW DELIVERY_CONDITION]`、任务待形成、不留续办引用——这是所有者答法为准的如实格；任一缝退回 nil 本用例即红。没有走到 `OpenDispatchTask` 是因为登记册空，不是缝没接（三缝齐、三所有者都给的一拍在执行器用例 `TestAPartyCommercialConditionReferenceFlowsIntoTheTaskVerbatim` 里证：任务形成，`Conditions` == PS 回指逐字）。
  - **触及**：上列新文件 + `cmd/parcel-api/assemble_delivery_dispatch{,_test}.go`（tf/12 / 13 已在 main 的文件，各填一格 / 改预期）+ `internal/transportfulfillment/ports/delivery_requirement.go`（仅头注一段）+ `docs/product/MECHANISM-INVENTORY.md` + 本票面。**未碰**：`internal/partycommercial/**`、`internal/parcelshipment/**`（红线；`git diff ffdf1d5c..tip -- internal/parcelshipment internal/partycommercial` 为空）；TF ports 签名；`application/trigger_delivery_dispatch.go` 与其既有测试；`endpoints.go` / `main.go` / `unwired_orchestration.go`；TF CONTEXT；ADR-0114 / 0130 / 0133 / 0080 / 0116 正文；`internal/architecture/*_baseline.txt`；`migrations/`。
  - **完成判据逐项**：1 ✓ 对照表所列，含两侧集外各例与 nil 口拒；conditions 串与 PS 回指 `String()` 逐字相等。2 ✓ `trigger_delivery_dispatch_partycommercial_test.go`：PC 答引用 → `DISPATCH_TASK_FORMED`，`Task.Conditions()` == 回指，PS / PC 各被问一次；集运单元 → `REQUIREMENT_MISSING / [DELIVERY_CONDITION]`、任务待形成、段与交接计数不变、PC 未被问。**与票面字面一处不同**（同 tf/12 评审已改口那条）：「续办引用非空」——`REQUIREMENT_MISSING` 按 ADR-0114 决定三与既有用例不留续办，照既有形状断言为空。3 ✓ `assemble_delivery_dispatch_test.go`（带 DSN），见「停点实际后移到哪」。4 ✓ 只传引用（`Conditions` 是回指串，允许集 / 证据规则一字不进 TF）；两个「没有」各一行不并、不读成默认签收；PS / PC 未动；测试只用合成引用（`contract-1` / `scope-a` / `SYN-*`），无真实合同与交付方式取值。5 ✓ 见验证。
  - **验证强度**（作者层，带 DSN `-p 1 -count=1 -v`）：gofmt 空；`go build ./...` / `go vet ./...` 0；`./internal/transportfulfillment/...` + `cmd/parcel-api` + `cmd/parcel-dispatch` + `./internal/architecture/...` **PASS 1739 / SKIP 0 / FAIL 0**。未跑全量。清点在 `595c659c` 干净检出重生成、单独成笔。
  - **与 main 碰面干跑**（16:3x，`origin/main = 8d38a0e5`）：在 detached 临时树把 `1e22410f` / `e7cd7272` / `595c659c` 依次 cherry-pick 全干净，重放树 build / vet 0，cmd/parcel-api + TF adapters/partycommercial + application + architecture ok；清点笔 `1accc36d` 按惯例跳过在 tip 重生成。
  - **给评审的判断题**：(a) 甲——两个「没有」同格，字面分不出是对象无合同还是合同无条件（ADR-0133 越权风险点 1 原文）；反方是执行器加子原因。(b) 适配器对 PC 的引用做一致性核（`reference.Resolution() != 被问回指` → `ErrUntranslatableAnswer`）——正方是 PC 领域门本就保证相等、不等只能是读口违约；反方是消费侧不该核提供方内部一致性。(c) 三缝齐后装配测试的预期从 `NOT_WIRED` 改为空库 `REQUIREMENT_MISSING` 三件——反方是该用真数据走到 `OpenDispatchTask`；正方是那要在 cmd 测试里种 PS 已接受委托 + NR 计划 + PC 闭包三套夹具，是集成用例的量，另立；本票那一拍在执行器层已证。(d) `ports/delivery_requirement.go` 头注改动越出票面地盘——推送方点名，仅注释。(e) 集运单元不分流照 tf/12——票面与推送方都定了「不在本票立种类维」。
  - **越权风险点**（不改 ADR，随票记）：ADR-0133 越权风险点 1 按甲落地，原文不改；风险点 3（闭包机制接通前 PC 那一格对每个对象 error 而不是「没有」）——本票实测空库里 PS 先答「没有」所以 PC 未被问到，闭包缺席那一格要等 PS 有已接受委托、PC 却无闭包时才现，届时执行器落 `DELIVERY_CONDITION_SOURCE_UNAVAILABLE`（提供方缺数据），是如实停点。
- **评审 ← 通道 5 · 钉 `47e1f13b`（代码 tip `595c659c`，基 `ffdf1d5c`）· 16:4x**（task-2eddaa4a，非作者，/code-review 两轴隔离检出、只读；无 DSN `go vet` TF + parcel-api 退 0，TF adapters/partycommercial + application 两包 ok；`git diff --stat ffdf1d5c 47e1f13b -- internal/partycommercial internal/parcelshipment` 为空；ports/delivery_requirement.go 只改头注）。**Standards**：阻断无；非阻断 (1) `adapters/partycommercial/delivery_condition_source.go` 头注那句「今天 PC 没有这一族，每一份都会答这一格，那是真话」在本分支基 `62c87e73` 之后已不成立——pc-gaps/11 已进 main（0030 两表与 SaveDeliveryConditions），今天缺的是租户登的声明（实例半边），不是「这一族」；parallel-sessions「断言有保质期」，建议改一句；(2) 同包 `adapters/partycommercial` 同时 import psdomain 与 pcdomain（三个上下文在一包）而包名只点一个提供方——ADR-0133 Consequences 明写「TF 适配器 adapters/partycommercial/ 里与承运主体目录并列的第二只，消费两个提供方」，是 ADR 选的形，判断题不是缺陷（可能的 Divergent Change 信号，归 owner）。无发现：注释全中文；跨文件引用用「决定一 / 二 / 三」、符号名与标题，无行号 / 计数 / 序位（「第一段 / 第二段」是本适配器自己的两跑）；无真实合同 / 交付方式取值；两只窄接口各含一法、不 import 任一侧 application / ports；无 Duplicated Code。**Spec**：阻断无；非阻断 (1) 两个「没有」取甲，头注写清两行来自两个所有者、恢复动作不同（ADR-0029）而执行器今天只有一格承接，符合票面默认判据，归 owner（ADR-0133 越权风险点 1）；(2) 完成判据 2「续办引用非空」作者照 tf/12 改口「MISSING 不留续办」（`TestAConsolidationUnitWithoutAnAdoptedContractLeavesTheTaskUnformed` 断言 ContinuationReference==""）——确认作者对，判据同 tf/12 评审。无发现（逐项）：① 两段各自封闭无 default——PS 回指 → 第二段 / PS 没有 → Missing 不进第二段 / PS error 原样上抛；PC 引用 → Resolved + 回指 String() 逐字（并核 `reference.Resolution()` == 回指，不等上抛）/ PC 没有 → Missing / PC 两格哨兵 error 原样上抛不折 Missing（`TestAPartyCommercialFailureIsPropagatedNotReadAsNoConditions`）/ 任一侧零值 → ErrUntranslatableAnswer。② 窄接口 `CommercialResolutionReferenceSource` / `DeliveryConditionReferenceSource` 各一法。③ 两处翻译都在适配器内、经 `NewResolutionID` 回指入口不拆不拼。④ `NewDeliveryConditionSource` 两只读口缺一拒；装配用例 `TestTheWiredDeliveryDispatchTriggerReachesTheOwnersAndReportsWhatTheyLack` 空库答 REQUIREMENT_MISSING [DELIVERY_PLACE DELIVERY_WINDOW DELIVERY_CONDITION]、无 NOT_WIRED、无续办、无任务，且对未决单独报错。⑤ ports 头注改后三条各点适配器与 ADR，准确（也收了 tf/13 那条非阻断）。⑥ 见非阻断 2。⑦ PC / PS 零改动。红线：不读条件内容；不自解析商业依据；两个所有者各答各的。**总：Standards 0 阻断 / 2 非阻断；Spec 0 阻断 / 2 非阻断。可重放。**
- **推送方处置 + 进 main 记录（通道 1 · 16:5x）**：Standards 非阻断 (1) 那一句过时断言由推送方在簿记笔里改成「这一族的表与写口 pc-gaps/11 已落，今天缺的是租户登的声明——实例半边；没有租户登过之前每一份都会答这一格，那是真话」（纯注释，改后 gofmt / build / vet / 该包重跑 ok）；其余三条非阻断记录归 owner，不改代码。隔离树 `%TEMP%\idp-replay-tf14` 在 `8d38a0e5`（= 当时 origin/main）之上 cherry-pick 四笔全干净：`12020372→89d601d5`（pick 的是 rebase 前的同内容对象 `1e22410f`，内容与 rebase 后逐字同，已核）/ `e7cd7272→e58e1483` / `595c659c→de1f9de2` / `47e1f13b→a731e362`；作者清点笔 `1accc36d` 与 main 同文件冲突，**跳过**，推送方在 tip 重生成为 `95b07c3f`（单独一笔）。本票七件与分支逐字相同。`95b07c3f` 上 `gofmt -l` 零、`go build ./...` / `go vet ./...` 0、`./internal/architecture/...` ok；占 55432 广播后**带 DSN** `go test -p 1 -count=1 -v ./...`：104 ok / 0 FAIL / 15 无测试，`--- PASS` 8067 / `--- SKIP` 1（仅 `pgtest.TestHelperTemplateOwnerProcess`）/ `--- FAIL` 0 / 0 cached，125 s，探针 PASS，释号广播。本条 + 那一句注释 + **tf/09 父票 Status → resolved**（三缝全接、执行器再无 NOT_WIRED 停点）随推送方簿记一笔落在 `95b07c3f` 之上，ff 后 `push <sha>:main`，SHA 在推后广播里。
