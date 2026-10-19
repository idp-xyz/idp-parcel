# 23 运输委托提交面

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg23（见文末「完成记录」）。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: [22](./22-supplier-agreement-version-point-read-and-tf-check-adapter.md)
地盘：`operation_decision.go`；tfhttp `operator_decision_intake.go`、新 `submit_transport_commission.go`；`application/commission_transport.go`（核对、法人快照、改注释）；`domain/transport_commission.go`、`commission_rehydration.go`；`postgres/transport_commission.go`；迁移 TF 0025（法人列）；新 `cmd/parcel-api/assemble_transport_commission.go` + `endpoints.go` 行；种子；ADR-0202、UC-TF-004 AT、演示文档。
出处：[02](./02-transport-commission-has-no-command-face.md)；方案「02 委托、订舱、承运答复」D02-1、D02-3、D02-4、D02-7，「跨票通则」X1、X3，「需要的 ADR」ADR-0202。

## 做什么

- 运营决定种类 `TRANSPORT_COMMISSION_SUBMISSION`（方案 D02-3）。
- 委托提交经 [22](./22-supplier-agreement-version-point-read-and-tf-check-adapter.md) 的适配器核协议，否定格与未决格照方案 D02-4；采购责任法人从协议版本快照到委托（新列），不由调用方给。
- 接上 `OutboxTransportCommissionHandoff`，路由表照 ADR-0066 不登记；改掉 handler 与端口注释里「交 SA 作成本预期源」的过时说法（方案 D02-1、X3）。
- 演示干线外包写进演示文档（方案 X1）。
- 起草 ADR-0202（要点见方案「需要的 ADR」）；UC-TF-004 补 AT 行。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 四个否定格加未决格。
- 委托与 Outbox 同一笔（真库）。
- 路由不登，`no_subscriber` 如实可见。
- 演示：干线委托答 `SUBMITTED`；服务方填末端承运商答 `PROVIDER_MISMATCH`。

## 规模

M

## 完成记录

2026-10-19 作者子代理在分支 `claude-dcg23` 上做完。派单时分支基于 `6569e31a`；开工前 main 已进 no-reception-source/03（`d7283946`，带来 access-identity 迁移 0007 与决定种类 `PENDING_IDENTITY_RECHECK`），本分支在零提交时快进到 `d7283946` 再开工，所以迁移 0008 的约束与枚举直接接在 0007 之后。

提交：`e0b7708b`（决定种类与迁移 0008）、`08492c8e`（领域、编排、委托册、迁移 TF 0025、信封 ID、门禁清单）、`b782fa76`（tfhttp 委托提交口）、`fe3fb893`（parcel-api 装配与端点表、派发侧用例）、`89c8d24e`（演示种子）、`3aa42d1f`（AT 点名）、`29fb7be2`（ADR-0202 等文档）、`a5a1c4ba`（注释措辞）。

### 做了什么

- **决定种类**：`accessidentity.DecisionTransportCommissionSubmission`（`TRANSPORT_COMMISSION_SUBMISSION`），枚举只追加。迁移 `access_identity/0008_transport_commission_submission_decision.sql` 重建决定种类约束，列出截至本迁移的全集（含 `PENDING_IDENTITY_RECHECK`）。
- **领域**：`TransportCommissionDraft`（`DraftTransportCommission`、`Submit`）是成立委托的唯一入口。草稿收登记方声明的一半，协议只指名对象与版本标签（`SupplierAgreementVersion`）；`Submit` 收核过的协议快照与采购责任法人。`TransportCommission` 多 `LegalEntity()`，重建门同步验它。导出的 `SubmitTransportCommission` 与 `TransportCommissionSpec` 退役。
- **编排** `CommissionTransportHandler.SubmitCommission`：草稿门 → 按（租户、委托）分重放与冲突 → 在提交时点经 `ports.SupplierAgreementSource` 核协议 → 四个否定格逐格答、读不动答未决（`SUPPLIER_AGREEMENT_UNAVAILABLE`）→ 以核出的快照与法人提交 → 意图同笔入队。并发首交的输家按摘要分已有与冲突。`CanonicalizeCommissionPayload` 改收协议对象与版本标签。
- **委托册**：迁移 `transport_fulfillment/0025_transport_commission_procurement_legal_entity.sql` 加 `procurement_legal_entity_ref`（非空、非空白），表里有行即拒绝施加；读写两侧带上这一列。
- **委托信封**：接上 `OutboxTransportCommissionHandoff`，路由表照 ADR-0066 不登。信封 ID 改走 `outboxintent.FingerprintEventID`（口名 `transport-commission`），分区键保留可读的「租户/委托/口名」；信封 ID 门禁例外清单剪掉这一行，分区主体登记改写这一行的说明。
- **过时注释**：编排 `handOff` 与端口 `TransportCommissionIntent` 里「交 SA 作成本预期源」改成现状（供应商成本来自发生项加 BUY；本信封今天无消费者）。
- **命令面**：`POST /transport-fulfillment-transport-commission-submissions`，tfhttp `submit_transport_commission.go`，挂 `operatorDecisions.transport`，按自己的决定种类认证。载荷 `commission`、`provider`、`agreement`、`agreementVersion`、`conditions`、`role`、`responsibility`、`members`、`submittedAt`；封闭解码，自报租户或法人即 400。新提交 201，其余格 200。
- **装配**：`cmd/parcel-api/assemble_transport_commission.go`，整次提交一笔事务；`endpoints.go`、`main.go`、`unwired_orchestration.go` 各追加行。
- **演示种子**：配置员授「运输委托提交」（`SYN-GRANT-DEX-CONFIG-TRANSPORT-COMMISSION-SUBMISSION-01`，依据 `SYN-GRANT-BASIS-DEX-16`），准入对照 `SYN-MAP-OD-TRANSPORT-COMMISSION-SUBMISSION-01`，权威区间 `governance/24-…`；`seed.sh` 与 README 授予表各加一项。
- **文档**：ADR-0202（新）；ADR-0149 Links 加前向指针；`docs/adr/README.md` 加 0202 一行；UC-TF-004 补 `AT-TF-105` 至 `AT-TF-107`；演示文档「第 3 步」补委托一节。

### 代决与越权风险点

用户原话「你作为业务和系统专家自决……解决所有的问题」；以下是方案没裁、本票自决的细节，裁定已写进 ADR-0202。

1. **草稿是成立委托的唯一入口**，导出的 `SubmitTransportCommission` / `TransportCommissionSpec` 退役。编排改走草稿后，`TransportCommissionSpec` 在生产代码里不可达，类型可达性棘轮要么加基线、要么收口；收口能让快照与法人只从核对结果来，没有一条不经核对就填上它们的构造路径。
   - 越权风险点：票 24、25、27、28 及其测试要构造委托，只能走草稿加 `Submit`（TF owner）。
2. **重放判在核协议之前；摘要收登记方声明的协议对象与版本标签**，不收快照与法人。
   - 越权风险点：同一委托标识换一版协议重交答冲突，登记方要另起标识（TF owner）。
3. **摘要在 TFC-1 前缀下改形，委托口退役无版本那一形**（删掉 `unversionedCommissionDigest` 及其用例行）。委托表在接线前无写入方，迁移 0025 在表里有行时拒绝施加，所以不存在按旧形状算的已存摘要。
   - 越权风险点：`unversioned_payload_digest.go` 头注写着「函数体一个字节都不能改」，本票整删了委托那一个函数；评审请核「无存量」这一前提（TF owner）。
4. **提交时点取登记方给的提交时刻**（ADR-0023），不取服务端时钟。
   - 越权风险点：倒填到协议有效期内，就能用一版已终止的协议提交（TF owner；ADR-0202 风险点 9）。
5. **`CommissionNotAccepted` 的线上名由 `SOURCE_NOT_ACCEPTED` 改为 `INPUT_NOT_ACCEPTED`**，与 TF 其余运营决定口同名；此前这个结果类型没有线上消费者。
   - 越权风险点：票 24 的承运答复口（集成客户端、外部来源）共用这个结果类型，也会答 `INPUT_NOT_ACCEPTED`；要区分由 24 另立一格（TF owner）。
6. **核对口违约（在册格缺快照或法人、未知格）答 500 `NO_ANSWER_FORMED`**，不答未决，哨兵 `ErrUnexpectedAgreementCheck`。PC 侧坏数据仍与读不动同落未决（票 22 端口头注已写明分不开）。
7. **委托信封 ID 改指纹在本票做**，不等 envelope-id-bounds/01 的运输履约半边：本票正是让这封信封第一次有生产方的那一笔（方案 X3「新信封一律指纹」）。分区门禁的同源例外行保留：ID 与分区键各取一个以 `key` 为参的函数，门禁剥壳后认作同源，先例是关务案件口。
   - 越权风险点：envelope-id-bounds/01 票面「现状」清单里的 `transportCommissionEventID` 已过时，那张票面本票没改（该票 owner）。
8. **ADR-0025 张力裁为「不是例外」**，不把次序与服务方比较挪进编排（ADR-0202 决定四）。
   - 越权风险点：见 ADR-0202 风险点 6（TF owner）。
9. **演示结果名是 `COMMISSION_SUBMITTED`**，不是票面写的 `SUBMITTED`：沿用编排既有的线上名，票面那一词按语义对应。
10. **装配只接委托册、协议核对、Outbox 与时钟**，订舱与承运应答的册子不装：本口只提交委托。
    - 越权风险点：票 24 要在同一个编排上补这两缝，装配放同一函数还是另立由 24 定（TF owner）。
11. **种子授予依据取 `SYN-GRANT-BASIS-DEX-16`**：15 已被 no-reception-source/03 占用；权威区间文件照占号取 24。

### 用例

- 领域：`TestADraftOnlyBecomesACommissionWithACheckedAgreementAndLegalEntity`；`TestACommissionSnapshotsItsCommercialBasis`（缺快照、缺法人、声明各格）；`TestRehydrateTransportCommissionRoundTripsStartAndCancel`（法人往返、缺法人拒）；`TestCommissionAndBookingGatesJudgeZeroOnTheRegistry`（草稿提交时刻判零）；规范化定值用例改为新形状。
- 编排：`TestACommissionSubmitsOnceWithItsSnapshots`（`AT-TF-106`：快照与法人取自核对、核对问的是指名的那一版与提交时刻、重放不再问协议、协议终止后重放仍答原委托、异版本同标识冲突）；`TestACommissionWhoseAgreementDoesNotHoldIsAnsweredPerCell`（`AT-TF-105`：四格零写入、服务方不符带供应商、读不动未决、声明不合法不问协议、核对口违约）；`TestAConcurrentFirstSubmissionLoserIsJudgedByContent`；`TestCommissionRecoveryDiscipline`（未决原因封闭集）。
- 委托册与信封（真库）：委托往返带法人；`TestTransportCommissionFollowsTheTransactionalTemplate`（ID 改指纹）；`TestATransportCommissionWithLongIdentifiersStillEnqueues`。
- 线格式：`TestTheTransportCommissionSubmissionFaceAnswersOnTheWire`；`TestEachTFDecisionFaceAuthenticatesForItsOwnDecision`、`TestEveryTFDecisionMapsToItsOwnDecisionKind` 各加一口。
- 生产装配（真库，`S`）：`TestTheWiredTransportCommissionSubmissionChecksTheSeededAgreements`——按种子发布批发布两份 SYN 协议，经操作者渠道：干线委托 201 `COMMISSION_SUBMITTED`（快照 `SYN-SUPPLIER-TRUNK-CN-SG-01/v1`、法人 `SYN-LE-01`）、原样重交 200 `EXISTING_COMMISSION`、委托行与信封 `xmin` 相同（同一笔）、末端承运商 200 `PROVIDER_MISMATCH`、未发布版本 `AGREEMENT_NOT_REGISTERED`、生效前 `AGREEMENT_NOT_EFFECTIVE`，核不过的零写入。端点探测表（未配置、方法门、自报身份各用例共用）与换口授予表各加一口。
- 派发（真库）：`TestTheTransportCommissionSubmittedEnvelopeHasNoSubscriber`（`AT-TF-107` 分发半边）——生产适配器入队的委托信封一拍记 `dispatch.no_subscriber`。
- 操作者册（真库）：`TestTheTransportCommissionSubmissionDecisionIsGrantedAndReadBack`（含待识别重核，证约束重建没漏抄）。

### 验证（证据 `S`）

- `go build ./...` 干净；`go vet` 跑 TF、access-identity、architecture、`cmd/parcel-api`、`cmd/parcel-dispatch`，干净；`gofmt -l internal cmd migrations scripts` 干净。
- 带 55432 DSN、`-p 1 -count=1`：`./internal/architecture/`、`./internal/accessidentity/...`、`./internal/transportfulfillment/...`、`./internal/partycommercial/...`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./cmd/parcel-access-register/`、`./cmd/parcel-governance-register/`、`./cmd/parcel-commercial/`、`./migrations/...`、`./scripts/demo-seeds/...`、`./tests/...` 全 ok。没跑全量。
- 一次性库 `dcg23_seed`：`seed.sh` 灌入退 0（授予、对照、区间各 RECORDED / REGISTERED）。起 `parcel-api`（操作者发行方 + 治理自授权）用配置员令牌实走：
  - 干线委托（`SYN-PARTY-CARRIER-TRUNK-01` + `SYN-SUPPLIER-TRUNK-CN-SG-01` v1）→ `201` `COMMISSION_SUBMITTED`，回显 `agreement` `SYN-SUPPLIER-TRUNK-CN-SG-01/v1`、`legalEntity` `SYN-LE-01`；
  - 原样重交 → `200` `EXISTING_COMMISSION`；
  - 服务方填末端承运商 `SYN-PARTY-CARRIER-LM-01` → `200` `PROVIDER_MISMATCH`，`agreementSupplier` `SYN-PARTY-CARRIER-TRUNK-01`；
  - 不带令牌 → `401` `OPERATOR_CREDENTIAL_REJECTED`。
  - 库里委托行与信封同一 `xmin`；起 `parcel-dispatch` 跑 12 秒，那封信封 `ABANDONED`、`dispatch.no_subscriber`、3 次尝试。
  - 走完已停进程并删库。
- 未重生成机制清点（进 main 时统一做）。

### 偏离与待裁

- **地盘外改动**（都是新文件、测试或只追加，不碰在途地盘）：
  - `internal/accessidentity/adapters/postgres` 新增真库用例文件；
  - `internal/architecture` 的信封 ID 门禁例外清单剪一行、分区主体登记改一行说明；
  - `cmd/parcel-dispatch` 新增用例文件（不碰 `assemble.go` / `assemble_test.go`）；
  - TF `ports/ports.go` 只改注释，签名不动（方案 X4 守的是签名）；
  - TF 领域的 `supplier_agreement_version.go` 加一个未导出的 `valid()`；`payload_canonicalization.go` 改委托一口（TF 规范化令牌序 16 → 17 → 23）；
  - TF 领域与编排的既有测试随构造入口收口改写，含 `registry_zero_gates_test.go` 的委托子用例与 `unversioned_payload_digest*.go`；
  - TF 接入身份适配器的映射用例加一行。
- **迁移号**：access-identity 0008（照占号）；TF 0025（方案建议号，即现有最大号的下一号）。
- **没改的**：TF CONTEXT（委托生命周期「固定……供应商协议依据」已盖住法人快照）；AT 覆盖盘点（票 16、19 加的 `AT-TF-099`–`104` 也未入表，进 main 时统一重盘）。
- **待裁 / 交接**：
  - 票 24：承运答复集成客户端面按 ADR-0202 决定二、五落；同一编排的订舱与应答两缝在装配里接哪；`INPUT_NOT_ACCEPTED` 是否另分来源格（代决 5）。
  - ADR-0202 风险点 6（检查次序与服务方比较的归属）待 TF owner 复核；风险点 5、7 待 PC owner。
  - `docs/adr/README.md` 里 ADR-0206 一行出现两次，是之前合批的遗留，本票没动。

