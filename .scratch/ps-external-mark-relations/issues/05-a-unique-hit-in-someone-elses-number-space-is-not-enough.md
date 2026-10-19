# 05 号码只在一份已接受委托里在册时，别人贴着同号的实物送站被唯一关联到那一件

Category: bug
Status: ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立并代决（票 26 序 0 研判）
Blocked by: 无
父票：[01](./01-external-mark-relations-have-no-model-in-parcel-shipment.md)；[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：研判簇 C2 第二张草案
地盘：
- docs/adr/（新 ADR，按裁定）
- docs/domain/parcel-shipment/CONTEXT.md
- docs/domain/CONTEXT-MAP.md
- docs/domain/node-operations/CONTEXT.md
- docs/application/node-operations/UC-NO-002-RECEIVE-CUSTOMER-DELIVERED-PARCEL.md
- internal/parcelshipment/adapters/postgres/external_identifier.go
- internal/parcelshipment/ports/external_identifier.go
- internal/nodeoperations/adapters/parcelshipment/identity.go
- internal/nodeoperations/ports/ports.go
- internal/nodeoperations/application/receive_delivered_unit.go
- cmd/parcel-api/assemble_reception_identity_test.go

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 出处

票 ps-external-mark-relations/02 的评审遗留（operator-channel/26 序 0）经研判与两路对抗核验，会话 `idp-parcel-9b`，钉 main `8d22729f`。评审原题是「决定六与 UC-NO-002 的范围冲突」；裁定维持决定六，本票收它留下的另一面。

## 现象

真库实测（经生产装配 `buildReceptionOrchestration`，合成夹具，证据只记 `S`）：

- **跨客户账户。** 客户账户一的委托已接受，客户包裹件号 N 进了册；客户账户二的委托也报了 N，只提交、未接受。客户二的实物贴着 N 送站，答`节点收寄已形成`，关联到客户一的包裹，控制建立，没有身份冲突。`deliveredBy` 写客户二还是一个不明交付方，答案一样。
- **同一客户账户跨委托。** 同一客户账户的两份委托都报了 N，只有第一份已接受。第二份的实物送站，唯一关联到第一份的包裹；同号的另一件实物再到站，也一样（一码两物见 no-reception-identity/01）。第二份也接受之后，才答身份冲突。

落点有三处：

- NO 的 `ReceiveDeliveredUnitHandler.receive` 按候选数分支，恰一个就关联。
- PS 读口只认已接受委托形成的关联（PS CONTEXT「接受前的声明包裹不是正式包裹，它的号码不进身份核对」），看不见另一方的声明。
- PS 采用这份收寄时，`AdoptOnNodeIntakeAdapter.HandleFormedNodeIntake` 按关联到的包裹，经 `FindCurrentAcceptedByParcel` 反查目标委托。所以 UC-PS-003「适用资格判断」里客户账户一致那一项，对节点来源是自己比自己。

另按派发装配读码（未实测）：错关联会随同一封收寄信封，进 PS 的采用（`adoptNodeIntakeConsumer`）与 VE 的追踪投影（`deriveProjectionConsumer`）。

够不够得着：

- 生产形态没有接单入口（`buildIsolatedSubmissionIntake` 在生产形态交回 nil），风险是潜伏的。
- 演示租户经隔离提交入口与派发受理链，走得到`已接受`（合成演示动线 2026-10-14 补记）；提交入口只在同一草案内查重，所以同客户那一格今天就走得到。
- 跨客户那一格，要等第一条多客户接单入口。

## 为什么是缺陷

- NO CONTEXT「实物身份与事实完整性」：「无法可靠识别的到站实物必须建立待识别实物」。
- UC-NO-002 应用流程：「唯一且证据充分时关联」。
- PS CONTEXT「外部标识关联」：「客户账户给的号在该客户账户内算数」。命中别人编号空间里唯一的一条，不是可靠识别。
- NO 端口 `ParcelIdentityView` 的头注「恰一个→关联」，把「唯一」当成了「充分」，仓里没有记录论证过。

## 要裁的一问

唯一命中，什么时候算「证据充分」？这属产品策略（判断方法），归开发方现在做；难逆转的取舍进 ADR。下面的候选可以组合，也可以分期：

- **(a) 歧义信号。** 号码同时被租户内一份未终局、未接受的委托声明时，核对不答唯一命中，改答冲突或待识别。只拦得住另一方已提交的那一格；而且要先改 PS CONTEXT「接受前……不进身份核对」那一句的读法：声明不进候选，但进歧义判断。
- **(b) 客户编号空间的号不自动关联。** 没有可验证的交付范围时，客户包裹件号的唯一命中也只记候选、走待识别。这是最稳的；代价是票 02 的识别效果，在交付范围建模之前整体失效。
- **(c) 先建可验证的交付范围。** 用送站预报或交接单绑定客户账户，落在范围内的唯一命中才关联。这是根治；要 NO 先建模，先改 PS CONTEXT「按外部标识核对包裹身份」那一条与 CONTEXT-MAP 同句，再另立 ADR 部分停用 ADR-0184 决定六。
- **(d) 认作已知代价。** 唯一命中照旧关联，靠一码两物检测（no-reception-identity/01）、测量复核与异常处置兜住。须写进 ADR，由用户认可。

时限：生产形态第一条接单入口（`PAR-INT-01` 真渠道或首方 API）上线之前，须裁定并修好；演示租户同客户那一格的排期，由分诊定。哪个授权交付方可以替哪个客户账户送货，属租户取值，不在本票。

## 裁定（2026-10-18，会话 idp-parcel-9b 代决）

取 (a) 作现在的产品策略：核对时，号码同时被租户内一份未终局、未接受的委托声明，唯一命中不再答唯一，改答身份冲突、走待识别。声明不进候选，只作歧义信号；这要先改 PS CONTEXT「接受前的声明包裹不是正式包裹，它的号码不进身份核对」那句的读法，并出 ADR（用号 0195，会话已占），写明与 ADR-0184 决定六的关系。(c) 可验证交付范围记作终局方向，随一线客户端收寄流程另立；(b) 不取（票 02 的识别效果会整体失效）；(d) 不取。时限：生产形态第一条接单入口上线之前修好。越权风险点：改 PS CONTEXT 读法属 PS owner 的领域语言，复核时看歧义信号会不会把同一客户合法的重复用号也一律打成待识别。

## 做什么（裁后）

- 裁定写进本票。难逆转的进 ADR，必要时部分停用 ADR-0184 的相应决定；先改 CONTEXT，再改引用它的 UC。
- 照裁定改核对或收寄判断；把票 04 的刻画用例按裁定有意翻转，并改写头注；跨客户与同客户两格各有回归用例。

## 不做

- 不按 `deliveredBy` 自报收窄，理由见 ADR-0184 复核记录「决定六与 UC-NO-002「权限、隔离与审计」」。
- 一码两物（no-reception-identity/01）、身份后来被确认（`AT-NO-026`）。
- 授权交付关系这类租户取值。

## 完成判据

- [ ] 「要裁的一问」已裁，写进本票；难逆转的进 ADR，CONTEXT 与 UC 照 AGENTS.md「改文档」的顺序同步。
- [ ] 跨客户与同客户两格按裁定作答，各有经生产装配的真库回归用例；票 04 的刻画用例按裁定翻转。
- [ ] 「两份已接受委托撞号 → 身份冲突」的用例照旧绿。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑受影响包与 `internal/architecture` 全绿；机制清点在本笔的干净检出上重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。
