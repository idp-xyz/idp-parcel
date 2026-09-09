# 派送要求缝三：交付条件引用（`party-commercial`）——`DeliveryConditionSource` 的消费侧适配器

Category: enhancement
Status: ready-for-agent——三问已裁（2026-09-09，通道 2，task-f5521768，用户授权 PC owner 口径代裁、第 2 问连 PS 口径；见「裁决」，理由、场景与被否替代在 [ADR-0133](../../../docs/adr/0133-delivery-condition-reference-is-the-acceptance-time-commercial-resolution-reference.md)）；做法与完成判据见下。此前 draft——由票 [09](09-arrival-triggers-dispatch-task.md) 裁决④与 ADR-0114 决定三/四拆出（2026-09-07，通道 4，task-79675845）
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
