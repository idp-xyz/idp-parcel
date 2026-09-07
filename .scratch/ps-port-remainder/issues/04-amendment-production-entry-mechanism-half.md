# 资料修订的生产入口（机制半边）：端点 + `UnconfiguredIntake{}` + 两层边界壳，接上 `AmendCustomerSourceDataHandler`

Category: enhancement
Status: resolved——2026-09-07 通道 2 在分支 `mcp2-ps-ports`（基线 `08f54867`）完成：封存笔 `38c2aa82`（旧会话在途产出原样入库）+ 交叉验证补格 `a1a1d16f` + 清点 `baff5fdd`；完成记录见 Comments。此前 in-progress——MCP-1 代裁（owner 授权，2026-09-07，03 票 Q3「生产入口现在接」）拆出，通道 2 按 task-b77525c9 ② 实施
Blocked by: 无

## 缺口

`NewAmendCustomerSourceDataHandler`（UC-PS-002 的编排）在 `cmd/` 零命中：没有端点、没有装配、没有边界壳——与 ADR-0106 之前的受控补充编排同一形（那时 `FormNewSubmissionVersionHandler` 也只活在测试里）。`BD-PS-009` 说的是**实例半边**（生产入口走哪个渠道、请求方与实际决定方怎么采信）；**入口的机制半边**（端点、`UnconfiguredIntake{}` 起步、来源保全与修订两层事务壳）不依赖它，也不依赖本目录 01–03 三口的 PC 半边——接上之后编排会如实停在两格诚实停点，正好由 02、03 两票日后解。

## 做什么（形照 ADR-0106 决定四与 `assemble_customer_supplement.go`）

1. `internal/parcelshipment/adapters/http/amend_customer_source_data.go`：`SourceDataAmendmentIntake` 接口（把已认证的接入请求译成 `AmendCustomerSourceDataCommand`；**无实现**，理由与 `SupplementIntake` 逐字相同——修订请求自己的来源身份、目标范围、基准版本、请求方全属 `PAR-INT-01` / `BD-PS-009` 待提供）、`AmendmentHandler` 接口、`NewAmendCustomerSourceDataEndpoint`；`UnconfiguredIntake` 补 `IntakeSourceDataAmendment` 一律答 `ErrAccessChannelNotConfigured`。响应形状封闭：`outcome` 取应用结果原名（`RECORDED` 201，其余 200），带版本号、采用判断、未决原因与续办引用；未命名结果与没形成答案照 ADR-0022 分流。
2. `internal/parcelshipment/adapters/partycommercial/`：`UnconfiguredSourceDataAmendmentAuthorizer{}`（一律答 `AuthorizationRulesNotConfigured`）与 `UnconfiguredSourceDataRuleDeclaration{}`（一律答 `NotDeclared`）——PC 那两半（03、02 票）没立之前装配点上就该摆着这两个如实答复，不摆 nil、不造替身。取「未配置」而不是 error：error 那格的恢复动作是重试，重试改不了一个没人建的规则；「未配置」至少说对了要办的事是让规则存在（ADR-0063 的分界）。
3. `cmd/parcel-api/assemble_customer_amendment.go`：`amendmentBoundary`（`Requests.Save` 切一个事务；照 `submissionBoundary.Save`）与 `handoffBoundary`（`SourceDataVersionHandoff` 切自己的事务——`OutboxSourceDataHandoff` 走 `RequireExecutor`，而修订编排是自己在 Save 之后调 `Downstream`，与受控补充「交接挂在 Save 里」不同形，见下「一格诚实的缝」）；`buildCustomerAmendmentOrchestration(db)` 接 `preservationBoundary`、`amendmentBoundary`、两只未配置适配器、`psidentity.NewSourceDataVersions`、`OutboxSourceDataHandoff`。
4. `endpoints.go` 加行 `/shipment-requests/source-data-amendments`（命令面，字面量 `UnconfiguredIntake{}`，隔离读准入与隔离提交放行都换不了）；`main.go` 构造；`unwired_orchestration.go` 加 `unwiredAmendment` 占位；`endpoints_test.go` 探针表加一行。
5. 装配用例（真库）：① 生产装配下修订停在**授权未决**（`SourceDataAmendmentAuthorityRulesNotConfigured`），不形成版本、不入队 `parcel-shipment.source-data-version.formed`；② 换一只放行的授权替身、规则仍是生产的未配置 → 停在**待复核**（`AWAITING_REVIEW`），同样无版本无信封；③ 两层壳自证：`handoffBoundary` 在自己的事务里把同一份意图入队恰好一封、重放不翻倍。

## 一格诚实的缝（不在本票改）

修订编排在 `Requests.Save` 之后自己调 `Downstream.HandOffSourceDataVersion`，两步各在各的事务里：版本落了库而意图没入队时编排交回 `技术未形成`（`SourceDataVersionNotHandedOff`）与续办引用，重放走 `existing` 路径重发同一份意图（`EnqueueOnce` 按版本标识认领，不翻倍）——这是编排头注写明的设计（「重放一律重发同一意图，由下游按版本标识认领」），本票照它接。要做到「版本与意图同一事务」得让编排不再自己调 `Downstream`（把交接挂进 Save 壳，形照 `supplementBoundary`），那是改编排不是改装配，归 PS owner 另裁。

## 红线

- 入口壳不采信任何自报身份（ADR-0091 那句）；`UnconfiguredIntake{}` 起步。
- 不给授权、矩阵任何默认；两只未配置适配器不读请求内容。
- 不改 `AmendCustomerSourceDataHandler` 的判断顺序与结果集；不改 UC-PS-002。

## 完成判据

端点在探针表里且未配置态答 403 `ACCESS_CHANNEL_NOT_CONFIGURED`；三条装配用例 PASS 非 SKIP（含真库）；`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库；机制清点重生成（端点数 +1）。

## 参照

`cmd/parcel-api/assemble_customer_supplement.go` 与其测试；`internal/parcelshipment/adapters/http/form_new_submission_version.go`；`internal/parcelshipment/application/amend_customer_source_data.go` 的 `handOff` 头注；`internal/parcelshipment/adapters/postgres/source_data_handoff.go`；ADR-0106 决定四、ADR-0055、ADR-0091、ADR-0022；本目录 02、03。

## Comments

- 2026-09-07 · 通道 2：按 MCP-1 代裁立票并开工（in-progress）。
- 2026-09-07 · 通道 2（新会话，task-d5558bc6 接管）：**完成记录。**
  - **落点**（分支 `mcp2-ps-ports`，基线 `08f54867`）：`38c2aa82` 封存旧会话在途十二件（一字未改；封存依据只记 mtime 11:39–11:45、分支 tip、12:03 点名自报）；`a1a1d16f` 交叉验证补两格；`baff5fdd` 机制清点重生成。
  - **做什么 1–5 逐条**：1 端点文件（`SourceDataAmendmentIntake` / `AmendmentHandler` / `NewAmendCustomerSourceDataEndpoint`，`UnconfiguredIntake.IntakeSourceDataAmendment` 一律 `ErrAccessChannelNotConfigured`，响应形状封闭）、2 两只未配置适配器、3 `amendmentBoundary` + `sourceDataHandoffBoundary` + `buildCustomerAmendmentOrchestration(db)`、4 端点行 / `main.go` / `unwiredAmendment` / 探针行、5 三条真库用例——全部在封存笔里，形状与票面一致。
  - **交叉验证**（parallel-sessions「先写自己第一片 red 再读对方代码」）：只从本票判据与编排头注另写两条真库用例，对着封存那份实现首跑即绿，判据与装配用例①②逐条对得上，算一次独立印证，副本不留。对不上的两格补进 `assemble_customer_amendment_test.go`：①加「停点之后修订请求的来源保全行仍在」；新增④ `TestTheHonestStopIsObservableAtTheAmendmentEndpoint`——生产编排接在真端点后面答 200、`outcome=UNDECIDED`、`pendingReason=SOURCE_DATA_AMENDMENT_AUTHORITY_RULES_NOT_CONFIGURED`、带 `continuationReference`、不带 `sourceDataVersionId`（端点用例头注说好逐格映射由本包经真编排补，此前没人补）。
  - **完成判据**：端点在探针表且未配置态 403 `ACCESS_CHANNEL_NOT_CONFIGURED`（`TestEveryAssembledEndpointAnswersUnconfigured` 与 `TestUnconfiguredIntakeRefusesWithoutReadingTheBody/amendment`）；四条装配用例含 DSN PASS 非 SKIP；`gofmt -l` 空、`go build` / `go vet` 退 0；含 DSN 全仓 `go test -p 1 -count=1 -v ./...` 于 `baff5fdd` 退 0：`--- FAIL` 0、`--- SKIP` 0（真库门禁全部实跑）、`--- PASS` 6922（下界）、99 个包 `ok`；`gofmt -l .` 空、`go vet ./...` 退 0；清点端点 98→99。
  - **清点读法**：端口两口径各降 2（`SourceDataAmendmentAuthorizer` / `SourceDataRuleDeclaration` 出了缺口名单）是两只**未配置**适配器被判据 B 计为实现，不是提供方半边落地——02、03 的 PC 侧仍开。
  - **未在本票补的一格**：`RECORDED` 201 无真编排用例——走到它要`已接受`委托加登记为`允许`的矩阵，本包今天没有把委托推到`已接受`的夹具，矩阵那半属 02 的 PC 半边；由端点代码与 `UNNAMED_OUTCOME` 用例守形。PC 半边落地时随「换真适配器」一并补。
  - 「一格诚实的缝」（版本与意图不同事务）照票面未动，仍归 PS owner 另裁。
