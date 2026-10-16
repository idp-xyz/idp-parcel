# 15 委托与履约的运营决定口：操作者渠道增「运营决定」能力面，委托侧五口与 TF 管理台上的决定与判断口换操作者 Intake

Category: enhancement
Status: resolved——2026-10-16 进 main：`60244fa6` / `0e3bb7df` / `58626ca1` 三笔快进进 main 并推，远端 main = `58626ca1`，SHA 未改写；见文末「进 main 记录（2026-10-16）」。此前 in-progress——2026-10-16 Claude Code 会话接手并完工（用户「你作为业务和系统超级专家，你自己全部独立完成吧」；通道 4 自 10-13 起无在途），分支 `claude-oc15`：余下四口换口同笔撤隔离放行 `60244fa6`，受控关闭与重开按 [ADR-0183](../../../docs/adr/0183-operator-channel-addresses-controlled-closure-and-reopening-by-parcel-and-derives-the-shipper-from-the-accepted-request.md) 换口 `0e3bb7df`；待进 main，见文末「完成记录」。此前 in-progress——2026-10-13 通道 4（通道 1 派单 `task-40698906`，接替裁定未送达的 `task-21973bb2`）：余下四口照裁定 A **本轮没换**，仍挂隔离放行；两条发现记在文末进展记录，四口改挡在 [19](./19-admission-scope-bridge-for-operator-and-integration-client-minters.md)（准入范围桥）；[20](./20-tf-closure-dispatch-task-and-pickup-judgment-canonicalization.md)（TF 三口规范化形状）2026-10-16 resolved（进 main 至 `2d84eb2c`），不再挡。分支 `mcp4-oc15f` 上只有票面几笔。受控关闭与重开仍等 PS owner。此前 in-progress——2026-10-12 通道 4 认领余下四口（通道 1 派单 `task-21973bb2`）：关段、建派送任务、有效时间判断、承运商首次有效收寄判断换「运营决定」操作者 Intake，同笔撤隔离放行（ADR-0150 决定三）；隔离 worktree `/home/tops/workspace/idp-parcel-mcp4-oc15f`、分支 `mcp4-oc15f`（基 `1e02d2ea`）。受控关闭与重开不在这一批，仍等 PS owner。此前 in-progress——2026-09-25 通道 4 认领（用户令独立完成 ADR-0151 那件；前置 03、14 均已 resolved），分块逐笔进 main：之一至之四已进 main（见文末进展记录）；余下关段、建派送任务与两个判断口的换口要等演示环境走通操作者渠道（换口同笔撤隔离放行），受控关闭与重开等请求方格与 PS owner 对齐。此前 ready-for-agent——2026-09-24 随 ADR-0151 立（用户同日「同意你的决定，开干」）；2026-09-25 通道 2 按用户「开干前，全面审查，确保确实如此」复核后改定范围、阻塞与完成判据，复核记录见文末 Comments
Blocked by: 无——2026-10-16 核得 19 已 resolved（进 main 至 `8e1e9777`）、20 已 resolved（至 `2d84eb2c`），受控关闭与重开的请求方格由 ADR-0183 代 PS owner 裁定。此前：隔离放行名单上四口（关段、建派送任务、有效时间判断、承运商首次有效收寄判断）的换口——[19](./19-admission-scope-bridge-for-operator-and-integration-client-minters.md)（准入范围桥：撤放行之后演示租户要靠它越过「不在准入范围」，四口都挡）。[20](./20-tf-closure-dispatch-task-and-pickup-judgment-canonicalization.md) 已 resolved（2026-10-16 进 main 至 `2d84eb2c`：关段只核对并补答复回显、建派送任务只核对、承运商首次有效收寄判断补 TFC-1 + `CONTENT_CONFLICT`），四口的规范化前置都已解除。2026-10-13 通道 4 照裁定 A 改指，原挡的 [17](./17-demo-issuer-dex-and-synthetic-operators.md)、[07](./07-admin-web-login-gate.md) 均已 resolved；受控关闭与重开两口——请求方格与 PS owner 对齐（本票做什么第 3 条）。03、14 已 resolved
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 甲轨
地盘：`internal/accessidentity`（能力面授予格「运营决定」）；`cmd/parcel-api` 端点表里下列各口的 Intake 装配与装配测试；`internal/parcelshipment/adapters/http` 与 `internal/transportfulfillment/adapters/http` 各口的操作者 Intake。
出处：[ADR-0151](../../../docs/adr/0151-unassigned-command-faces-get-their-families.md) 决定一、二、三、六；[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定二、四；[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定四。

## 做什么

1. 操作者册的授予多一个能力面「运营决定」，授予按租户 × 决定种类登记：复核完成、主动拒绝、授权处置、受控关闭、受控重开、关段、建派送任务、装载分配、终止参与、有效时间判断、承运商首次有效收寄判断。
2. 下列各口在装配点换成操作者 Intake，每换一口，该口的「未配置即拒」测试改写为答复格测试：
   - 委托侧：`/shipment-requests/manual-review-completions`、`/shipment-requests/rejections`、`/shipment-requests/authorized-dispositions`、`/shipment-requests/continued-attempt-closures`、`/shipment-requests/continued-attempt-reopenings`；
   - TF：`/transport-fulfillment-segment-closures`、`/transport-fulfillment-dispatch-task-registrations`、`/transport-fulfillment-load-assignment-registrations`、`/transport-fulfillment-participation-terminations`、`/transport-fulfillment-effective-time-judgments`、`/transport-fulfillment-carrier-first-effective-pickup-judgments`。
   关段、建派送任务、有效时间判断与承运商首次有效收寄判断四口今天经写开关放行，换口的同一笔撤下隔离放行（ADR-0150）。
3. 身份格照 ADR-0151 决定二逐口取：
   - 租户与提交操作者只从信封来；
   - 复核人、拒绝决定人、处置人取提交操作者，有没有权仍由编排问 party-commercial；
   - 受控关闭与重开的命令里没有提交操作者的格，决定方、授权角色与授权依据快照由授权答复给出；请求方与重开时的货主账户在操作者渠道上怎么形成，先与 PS owner 对齐，定之前 Intake 不收这两格。
4. 委托寻址：Intake 按信封的租户与请求指名的委托（或包裹）在服务端查出委托的来源身份（租户、客户账户、来源、来源请求键），不从请求体收；查不到与越权探针同答。
5. ADR-0149 决定四两条（ADR-0151 决定三）：
   - 铸信封前接 14 的准入范围判断，不覆盖答「不在准入范围」；
   - 逐口核载荷规范化——领域已有同身份重放与内容冲突代数的口只核对，逐口记下凭哪条代数；没有的补一版带形状版本的规范化形状。未核完的口不开真渠道。

## 不做

- 客户侧各口（ADR-0139，归用户）；主链事实的更正口（10、11）。
- 角色模型与授权请求坐标的推导（[psb/07](../../product-strategy-boundary/issues/07-pc-authorization-coordinates-and-role-models.md)）；party-commercial 授权动作词表里的「授权处置」一格（ADR-0132 越权风险点 2；此前无票，2026-09-25 记到 psb/07 第 2 项下）。
- TF 决定要不要记提交操作者作操作证据（归 TF owner，ADR-0151 越权风险点 5）。

## 完成判据

- 上列各口：发行方未配置 / 令牌无效 / 无此决定种类的授予 / 不在准入范围四格各答其格，带测试；装配测试与端点表仍一一对照。
- 租户、提交操作者与委托来源身份都不从请求体收，请求体带即 `400`；复核人、拒绝决定人、处置人等于提交操作者；受控关闭与重开的命令里不出现提交操作者（带测试）。
- 越过 Intake 之后：TF 各口答业务结果；委托侧五口到达编排，在各自的授权缺口补上之前，生产装配如实停在该口今天的格——复核完成 `NO_ANSWER_FORMED`，主动拒绝「拒绝授权不可用」，受控关闭与重开「未决 · 授权口不可用」，授权处置「授权规则未配置」；装配测试以合成的授权映射或授权器证已授权时答业务结果（同 `manualReviewOrchestrationWith` 的做法）。
- 关段、建派送任务、有效时间判断与承运商首次有效收寄判断四口的隔离放行已撤，其隔离用例改写为真渠道答复格用例。

## Comments

### 复核记录（通道 2，2026-09-25）

用户令「开干前，全面审查，确保确实如此」。通道 4 起草的 ADR-0151 与本票在 23:22 落盘后会话中断，未提交；本记录逐条对照代码与文档复核草稿。

**属实，照旧：**
- 委托侧五口挂字面量 `UnconfiguredIntake{}`、没有隔离放行，也没有任何票接住（08 归类表记「未归」）。
- ADR-0100 决定一已把 `PAR-INT-01` 收回到客户生产委托接入渠道，这几口注释里的「属 `PAR-INT-01`（实例半边）」自 2026-09-03 起就过期了。
- 这几口的操作者是运营企业自己的人，不是货主客户；客户侧的撤回、取消、补充、资料修订等的去处等的是用户对 ADR-0139 的决定，不是租户证据。

**改了的：**
1. **实际决定方从哪来**。草稿写「受控关闭与重开、主动拒绝、授权处置的实际决定方由命令声明、由商业授权核定」，五口没有一口对得上：
   - 拒绝与处置的适配器写「采信自报的决定人 / 处置人等于让任何调用方替任何角色拒单 / 决定去向……Intake 只交出『谁在请求』」；
   - 受控关闭命令写「决定方、授权角色、授权依据快照不在命令里，由授权答复给出」。
   照草稿做就是从请求体采信决定人。改为 ADR-0151 决定二。
2. **TF 四个管理台写面被拆进两族**。关段与建派送任务早由 ADR-0149 决定一列进作业事实登记（要设备），草稿却以「管理台判断、没有设备」把同组的装载分配与终止参与另归新能力面；票 tf-segment-lifecycle-closure/07 把四口一并定为「运营写决定」。改为四口整组归「运营决定」，部分停用 ADR-0149 决定一两项（ADR-0151 决定六）。
3. **ADR-0055 决定五两项**。草稿没说载荷规范化与准入范围对运营决定口适不适用；运营决定形成生产事实，ADR-0100 豁免登记写面的理由对它不成立。补 ADR-0151 决定三，本票 Blocked by 加 14。
4. **委托寻址**。五口命令都带委托来源身份，操作者信封里没有；草稿只列为风险点，改为本票「做什么」第 4 条。
5. **生产上走不到业务结果**。复核、拒绝、受控关闭与重开的授权请求映射在生产装配里是 nil（psb/07 第 1 项），授权处置装的是未配置授权器（PC 授权动作词表缺这一格，ADR-0132 越权风险点 2）；草稿的完成判据「七口答业务结果」在生产装配上不成立。改为逐口如实停在今天的格、装配测试以合成授权证已授权路径。
6. **清扫票 16 不存在**。ADR 与 psb/15 引了「同日做完」的 16，而全仓还有几十处过期注释（含本票五口自己的 Intake 注释）；本次补立并做完。

- 2026-09-25 · 通道 4 · 实施笔记（之一已进 main `961525fb`；之二至之五照此复用现成实现，不另起一套）：
  1. **Intake 形状**：各口已有 `XxxIntake` 接口（`IntakeXxx(ctx, *http.Request)` 交回命令），今天由 `UnconfiguredIntake{}` 满足、部分口另有隔离实现。操作者渠道就是这些接口的又一种实现，在 `cmd/parcel-api/endpoints.go` 装配点逐口换，处理器与路由不动（同端点表头注「逐端点替换」的纪律）。
  2. **答复格映射**：各上下文的哨兵与答复码写在自己的 `adapters/http`（PS 是 `unconfigured_intake.go` 里的 `ErrAccessChannelNotConfigured` 与 `ErrMalformedRequest`）。PS 已有 `writeIntakeProblem`（`query_shipment_request_views.go`），而五个决定口的处理器各自内联了一份同样的映射。新格——令牌不过 401、未授予 403、不在准入范围 403、依赖故障 503——加进 `writeIntakeProblem`，五口改调它，不再各抄一份。TF 照同一办法。`accessidentity` 的哨兵在操作者 Intake 里译成本上下文的哨兵，处理器不认识 `accessidentity`。
  3. **铸造**：`accessidentity.OperatorMinter.MintOperator`，请求 `{TenantID, Face: CapabilityOperationDecision, DecisionKind, Admission}`。十一口各对一个 `DecisionKind`；`AdmissionRequirement` 的能力与事实类型逐口定（ADR-0151 决定三：运营决定口要判准入）。
  4. **委托寻址**（委托侧五口）：复用已有的委托查阅读口（`pspostgres.ShipmentRequestViews` 按委托标识取详情，详情带客户账户、来源、来源请求键），不新写查询。它的作用域（`AuthorizedQueryScope`）还带「可见客户账户」一维，操作者渠道上怎么取值先对齐；查不到与越权探针同答。
  5. **TF 隔离放行**：关段、建派送任务、两个判断口的放行在 `cmd/parcel-api/assemble_isolated_write.go`，换口的同一笔撤下（ADR-0150）。
  6. 动 `cmd/parcel-api/endpoints.go` 前先占号：通道 3 的 ADR-0152 试算实现也要改它。

### 进展记录 ← 通道 4 · 2026-09-25

- **之一** `1d3b9d79`（main `961525fb`）：「运营决定」能力面按十一种决定授予。含领域类型、迁移 `access_identity/0002`（种类列与两道 CHECK）、册适配器、登记 CLI；`OperatorRequest.DecisionKind` 与信封的 `HoldsDecision`。
- **之二** `98925946`（main `76511c67`）：TF 六口的 `tfhttp.OperatorDecisionIntake`。线格式复用各口既有 `…Payload`，装载分配与终止参与两口的载荷随之新定。`commandEndpoint` 加四格答复。防腐适配器在 `internal/transportfulfillment/adapters/accessidentity`。请求没指名租户时，`accessidentity` 取册上绑定的租户。
- **之三** `44ef6a4a`（main `161dc5ca`）：复核完成、主动拒绝、授权处置三口的 `shipmenthttp.OperatorDecisionIntake`。`ports.OperatorDecisionTargets` 按租户与委托标识寻址，postgres 实现在委托查阅适配器上。`writeIntakeProblem` 加四格并成为本包唯一映射。防腐适配器在 `internal/parcelshipment/adapters/accessidentity`。
- 每笔进 main 前都在推的那个 SHA 上跑了带 DSN 的全量（121 / 122 / 123 包 ok，0 FAIL），推送方即作者自审，**不算非作者评审**。

**余下两格与判断项**：
1. **装配换口**（完成判据第 1、2、3 条）：在 `cmd/parcel-api` 装配——用 02 号票的校验器、postgres 操作者册与准入桥（租户对照 + `AuthorityCoverage`，其 SelfAuthority 与 PS 生产归属取同一值）建 `OperatorMinter`，再建两边的防腐认证方与 Intake——然后逐口换，并同笔撤下 TF 四口的隔离放行（ADR-0150）。挡在两处：管理台登录门（07），以及演示环境要能走操作者渠道（Dex 接进 compose、合成操作者与十一种决定的授予、合成租户的准入对照与区间）。现在换，演示动线与管理台这几页会当场断。
2. **受控关闭与重开**：命令里的请求方格（重开另有货主账户格）不采信自报，操作者渠道上怎么形成由 PS owner 裁（ADR-0151 决定二）；定之前两口照旧挂未配置。
3. **判断项**（留给评审与 PS owner）：复核完成的证据格取管理台送来的复核理由；主动拒绝与授权处置的证据格取 `OPERATOR/<发行方>#<sub>`，理由进结构化原因格；复核完成与主动拒绝的提交版本取服务端当前版本（管理台草案不带），授权处置取草案送来的版本；运营决定口的准入要求取能力 `OPERATION_DECISION`、事实类型为决定种类——治理登记册那一侧的能力与事实类型词表尚无先例，首个租户登记区间时要对齐。
- **之四**（本笔之前一笔）：五个不在隔离放行名单上的运营决定口，在装配点换成操作者 Intake——复核完成、主动拒绝、授权处置、装载分配、终止参与。`buildOperatorDecisionIntakes` 用 02 号票的核验方、库里的操作者册与 UnconfiguredAdmissionScope 建铸造器。发行方参数没设时五口照旧答 403 未配置，与换口之前逐字节相同；设了以后答操作者渠道的格，首个租户登记准入对照之前停在「不在准入范围」。**更正**上一条「装配换口被 07 挡住」的说法：对不在隔离放行名单上的口，换口在未配置环境里没有可观察变化，不必等 07；真正要等演示环境走通操作者渠道的，只是在隔离放行名单上的关段、建派送任务与两个判断口（换口要同笔撤放行，ADR-0150）。

### 进展记录 ← 通道 4 · 2026-10-13：余下四口没换（裁定 A）

通道 1 派单 `task-21973bb2`（2026-10-12 22:2x）要关段、建派送任务、有效时间判断、承运商首次有效收寄判断换「运营决定」操作者 Intake、同笔撤隔离放行。认领笔 `3ee62f2e` 之后核出两处越出那张卡的地盘，22:35 报通道 1，给了 A / B / C 三选。通道 1 22:38 取 A，那条裁定没有送到（在队列里过期，进了死信），2026-10-13 11:5x 以 `task-40698906` 重发；本节照它收口。

**本轮没换口。** 四口仍挂隔离放行（`isolatedWriteAdmittedCommandLines` 里这四行没动），分支 `mcp4-oc15f` 上只有票面几笔，没动任何 `.go`。

**裁定 A**：四口都不换，两条发现各立一票——[19](./19-admission-scope-bridge-for-operator-and-integration-client-minters.md) 与 [20](./20-tf-closure-dispatch-task-and-pickup-judgment-canonicalization.md)，Blocked by 已改指。B（在原卡里就地补桥）要动之四各口与登记写面共用的铸造器，先立票分诊再定谁做；C 与做什么第 5 条「未核完的口不开真渠道」相抵，不取。

取证钉 `1e02d2ea`（= 远端 main；本分支在它之上只多票面，代码相同），只读。

**发现一 · 准入范围挡住演示动线**（做什么第 5 条第一项）
- TF 防腐认证方 `OperatorAuthenticator.AuthenticateOperatorDecision`（`internal/transportfulfillment/adapters/accessidentity`）给每口的铸造请求带 `AdmissionRequirement{Capability: "OPERATION_DECISION", FactKind: <决定种类>}`；`OperatorMinter.MintOperator` 见 `Admission` 非空，就按册上绑定的租户问 `AdmissionScope.Admits`。
- 生产装配的 `buildOperatorMinter` 传 `accessidentity.UnconfiguredAdmissionScope{}`。全仓非测试代码里 `AdmissionScope` 的实现只有它；pilot-governance 的 `AuthorityCoverage` 在，`NewAuthorityCoverage` 没有生产调用点——14 判断项 4 说的「租户对照 + `AuthorityCoverage`」桥没立。
- 演示种子里没有能力为 `OPERATION_DECISION` 的区间（`scripts/demo-seeds/data/governance/` 现有区间的能力是 `SYN-CAP/route-planning`、`SYN-CAP/price-evaluation`、`SYN-CAP/shipment-intake`）；`data/access/02-grants.json` 里也没有运营决定授予。
- 演示租户照 ADR-0150 决定一走同一道准入，没有例外可开。换口之后，给配置员补齐这四种决定的授予，四口也答 `403 OUTSIDE_ADMISSION_SCOPE`——就是之四 `TestSwappedDecisionFacesAnswerFromTheOperatorChannel` 里 "granted, admission not registered" 那一格。管理台 `effective-time-judgment-api.ts` 的 `judgeEffectiveTime` 发 `POST /transport-fulfillment-effective-time-judgments`，今天经隔离放行；换口即退成 403。
- 补桥要改 `buildOperatorMinter`、`main.go` 与治理、接入两份种子，不在原卡地盘内 → 19。

**发现二 · 逐口核载荷规范化**（做什么第 5 条第二项，ADR-0151 决定三）
- 有效时间判断 ✅ **只核对**，凭 `JudgeEffectiveTimeHandler.Judge` 的代数：当前版已按显式判断（`EffectiveTimeJudgedExplicitly`）判过、且有效时间与命令相同 → `ALREADY_JUDGED_AS_GIVEN`，交回原版并重发意图；不同 → 换新版本回指前版，答 `EFFECTIVE_TIME_JUDGED`。命令的内容只有 `EffectiveAt`（事实是身份），全数在比；异内容是改判，换一版而不是冲突。重放只比当前版：其后另判过一版，同一条旧命令再到时会按改判再换一版——命令不带请求键，这一格不是规范化形状补得了的。
- 关段 ❌：`CloseFulfillmentSegmentHandler.Close` 同段再关一律答 `SEGMENT_ALREADY_CLOSED`，`ClosedAt` 不比。
- 建派送任务 ❌：`OpenDispatchTaskHandler.Open` 同任务再投答 `DISPATCH_TASK_ALREADY_OPEN` 并回显原任务，工作范围与 `OpenedAt` 不比。
- 承运商首次有效收寄判断 ❌：`JudgeCarrierFirstEffectivePickupHandler.Judge` 同一依据在非待确认链尾上答 `ALREADY_RECORDED`，读法、承运主体、业务时间不比。
- 后三口都在 13「未定形状」清单的 TF 一行里（`CloseFulfillmentSegment`、`OpenDispatchTask`、`JudgeCarrierFirstEffectivePickup`）→ 20，归 TF owner。
- **回写（2026-10-16）**：[20](./20-tf-closure-dispatch-task-and-pickup-judgment-canonicalization.md) 逐口裁毕（TF owner = 用户），转 ready-for-agent：
  - 关段 ✅ **只核对**：`SEGMENT_ALREADY_CLOSED` 够用，不补形状；20 另补答复回显在册关闭时刻，那是答复形状的事，不挡换口。
  - 建派送任务 ✅ **只核对**：`DISPATCH_TASK_ALREADY_OPEN` 回显原任务够用。
  - 承运商首次有效收寄判断 ❌→20：补 TFC-1 形状，同依据异内容答 `CONTENT_CONFLICT`；20 实施完之前这一口不换。
  - 四口换口的形状前置只剩承运商判断一口。
- **回写（2026-10-16，进 main）**：20 resolved，承运商首次有效收寄判断的 TFC-1 形状与 `CONTENT_CONFLICT`、关段答复回显随 `857453d8` 进 main（远端 main = `2d84eb2c`）。四口换口的形状前置全部解除。

**更正**上一节「余下两格与判断项」第 1 条与「之四」所说的挡点：07、17 都已 resolved，演示环境有了发行方与合成操作者；四口换口还缺的是合成租户的准入对照、区间与它们背后的桥（19），以及三口的规范化形状（20）。给配置员补这四种决定的授予，仍随换口那一笔做。

### 进 main 记录（2026-10-13 通道 1）

分支 `mcp4-oc15f` 四笔纯 `.md`，原样重放到 main `ee48fa54` 之上：`3ee62f2e`→`f3ec4448`、`749c6f90`→`eaa255b8`、`aca18256`→`584d20b9`、`af5a83a6`→`d4590c66`，四份文件对分支 tip 逐字相同；分支上的 SHA 作封存出处。推送后拆隔离树 `idp-parcel-mcp4-oc15f`，分支指针改名 `merged/mcp4-oc15f`。纯 `.md`，推送方自审。本票仍 in-progress。

### 评审 ← 会话 idp-parcel-9b 起的隔离子代理 · 钉 `60244fa6`（基线 `858280a2`；之五 · 余下四口）· 2026-10-16 10:0x–10:2x UTC

票 [26](./26-non-author-review-backlog-for-claude-code-landings.md) 序 4 补评。非作者：代码出自会话 `idp-parcel-c8`。一个子代理在隔离只读 detached 检出上按 `/code-review` 两轴串行（先写完 Spec 再做 Standards，两轴不互相改判）；由会话 `idp-parcel-9b` 代落。评审侧验证（无 DSN）：`cmd/parcel-api` 与 TF `adapters/http` 的 `go vet` 退 0、`gofmt -l` 无输出；`go test -count=1` 下 `cmd/parcel-api`、TF `adapters/http`、TF `adapters/accessidentity` 全 ok。真库用例 `TestTransportFulfillmentCommandLinesAnswerBusinessOutcomesAgainstARealDatabase` 与 `TestARecordedDeliveryAttemptLetsTheEffectiveDeliveryThrough` 因无 DSN 而 SKIP，「越过 Intake 后答业务结果」这一格本次没有独立证实，依据的是作者带 DSN 跑的记录。

- **Spec**：**阻断 无**。已核：四口的端点行直接挂 `operatorDecisions.transport`，它在生产装配里始终非 nil；`assembleBusinessEndpoints` 删掉了隔离 TF 入参，写开关在结构上换不动这四行；被删的 `if isolatedTransportFulfillment` 只换这四口，没有别的口因为撤放行而失去入口；完成判据要的各格由 `TestEveryAssembledEndpointAnswersUnconfigured`、`TestSwappedDecisionFacesAnswerFromTheOperatorChannel` 和新增的 `…RefuseAGrantOfAnotherDecisionKind` 钉住；方法、名单行、隔离用例在同一笔里撤掉，符合 ADR-0150 决定三「撤下该口的隔离放行」。
  **非阻断 1**：`scripts/demo-seeds/README.md` 的 `IDP_PARCEL_GOVERNANCE_SELF_AUTHORITY` 说明段称「管理台上的关段、建派送任务、有效时间判断与承运商首次有效收寄判断四口……在这几页上答的是 `OUTSIDE_ADMISSION_SCOPE`」，与事实不符：`apps/admin-web/src` 里只有 `effective-time-judgment-api.ts` 调用其中一口，另外三口在换口前后都没有管理台页面，只能走 API；本票 10-13 的取证也只提到这一页。`seed.sh` 对照段的注释同病。
- **Standards**：**阻断 无**。**非阻断 3**：
  (1) 本笔让这些注释和命名与代码不再相符：TF `OperatorDecisionIntake` 头注「它与 IsolatedCommandIntake 走同一套线格式……两者只差租户从哪来」，四个方法撤掉后两个类型已没有一口重合（`7ec601ab` 已改）；`operationDecisionFacesLeftOnIsolatedWrite` 意为「还留在隔离写上」，本笔之后已不成立；`assembleBusinessEndpoints` 头注新写了「写开关此刻只剩这一行可换」，同一函数里派送发起、派送尝试、交接、节点作业查阅几处的行注仍说「经写开关放行」（票 10 遗留，本笔改了旁边几行却没清）。
  (2) 变更说明，违反「写代码注释」的「不写变更说明」：`isolatedWriteAdmission` 的字段注释「运输履约那一格随运营决定四口换操作者渠道撤下」描述的是本笔删掉的字段；`assembleBusinessEndpoints` 里「这段原是 ADR-0091 的逐口隔离放行……各口已随真渠道撤下放行」。
  (3) 计数与序号引用，违反「改文档」「一律不用行号，也不用计数」：`decisionLines` 头注「本包六个运营决定口」；`TestTransportFulfillmentCommandLines…` 头注「票 15 完成判据第三、四条」，按位置指一个没有编号的列表，也没有引原文。
  判断题 Duplicated Code 不成立：四口的认证件共用 `tfaccess.OperatorAuthenticator.AuthenticateOperatorDecision` 与 `tenantFor`，按决定种类参数化；本笔反而删掉了隔离侧同形的译装代码。重复只出现在测试里：同一组四口分列在几张表中，(1) 里那个过时的表名正是这几张表没有同步的结果。
- **无发现**：Spec「已核」所列各格。
- **结论**：两轴 0 阻断，不立修复票。除 (1) 里 `OperatorDecisionIntake` 头注那处已由 `7ec601ab` 改掉，其余各条在 main `4dd937eb` 上都还在，随下一笔碰到这些文件时清。

### 评审 ← 会话 idp-parcel-9b 起的隔离子代理 · 钉 `0e3bb7df`（基线 `60244fa6`；之六 · 受控关闭与重开）· 2026-10-16 10:0x–10:2x UTC

票 [26](./26-non-author-review-backlog-for-claude-code-landings.md) 序 1 补评。非作者：代码出自会话 `idp-parcel-c8`。一个子代理在隔离只读 detached 检出上按 `/code-review` 两轴串行（先写完 Spec 再做 Standards，两轴不互相改判）；由会话 `idp-parcel-9b` 代落。评审侧验证（无 DSN）：经 `go test -overlay` 挂领域与 HTTP 两份探针（不提交）取证；http 包与 `cmd/parcel-api` 相关用例绿；gofmt、vet、CR/BOM 干净。

- **Spec**：**阻断 1**。`cmd/parcel-api/endpoints.go` · `assembleBusinessEndpoints` 里 continued-attempt 两行已接上真渠道，而本票「做什么」要求「逐口记下凭哪条代数……未核完的口不开真渠道」（ADR-0151 决定三）；「之六」与 ADR-0183 里都查不到这两口的核对记录。补核也过不了：决定标识由本上下文现铸，命令里没有请求键，关闭也不拒已经在关闭中的册。同一个 POST 重试一次就会追加第二条关闭，生效关闭换成新标识；之后指名第一条关闭的重开答 `NOT_ADMITTED`。评审会话在 main `4dd937eb` 上复核属实：领域探针以同样的关闭内容、同一生效时刻、新铸的标识再 `Append` 一次，返回 nil，`StandingClosure()` 换成第二条。生产装配上这两口的授权映射为空，停在 `UNDECIDED / AUTHORITY_UNAVAILABLE`，形成不了决定，潜伏未发作；授权映射接上之前必须修。→ 修复票 [28](./28-controlled-closure-and-reopening-have-no-replay-algebra.md)。子代理顺带提到之三那三口也查不到逐口核对记录；之三不在票 26 的表里，本次未复核。
  **非阻断 3**：
  (1) `internal/parcelshipment/adapters/http/operator_decision_intake.go` · `locateParcel` / `shipperOf`：客户授权边界没有越过——来源身份取自包裹所属的已接受委托，PC 代问的就是这个客户账户，跨委托代问的口子已经堵上。残余：ADR-0183 Context 自己提出的「货主的指令被冒用了两次」没有堵住，决定二只防住冒用别的账户；操作者选 `SHIPPER-INSTRUCTION` 就得到请求方，重开时写任意非空串就能过门。货主重授权证据过门后「不落册」（`FormReopeningCommand` 头注），操作者也不进命令，所以册上的请求方无据可查，PS CONTEXT「parcel-shipment 拥有……包裹级关闭或重开请求证据」在这里落空。证据落不落册，请 PS owner 一并裁。
  (2) `IntakeReopening`：Intake 用 `!= ""` 判断「带了证据」，编排那道门用 `TrimSpace`。全空白的证据会让请求方记成客户，门却当作没带，与决定四「没带时请求方缺席」不符（探针实测）。
  (3) `IntakeControlledClosure`：两道纯载荷的责任主体校验排在 `locateParcel` 之后，包裹查不到时答 403，而决定三要求这种载荷答 400（探针实测）。
  (2)(3) 已记进票 28「一并看」。
- **Standards**：**阻断 无**。**非阻断 3**：
  (1) `cmd/parcel-api/assemble_operator_decisions.go` · `buildOperatorDecisionIntakes` 头注「委托侧按委托标识寻址的三口用 targets，受控关闭与重开两口……」数的是 `shipmenthttp` 包里的口，违反「改文档」「计数与行号同构」与「Go 注释里的跨文件引用同受此约束」，应改用符号名。
  (2) `shipperOf` 在 HTTP 适配器里拼请求方种类段 `CUSTOMER-ACCOUNT/`，靠它判相等的门 `shipperReauthorizationGate` 却在 application。同性质的 `<种类>/<主体>` 按 label-channel/30 裁决 ② 放在 application，「合成与解析收在相邻一对」。ADR-0183 复核记录（`c7a462cd`）要求客户渠道日后「照决定二写同一种带种类段的引用」，到时就会出现第二处拼串，碰「一决策一处定义」（判断题：Primitive Obsession / Shotgun Surgery）。
  (3) `parseClosureResponsibilitySource` 在适配器里又列了一遍责任来源，和 application 的 `closureResponsibilitySourceKindOf` 是同一份封闭词表（判断题：Repeated Switches）。
- **无发现**：红线其余几条（只用 `SYN-` 取值、证据记 `S`、所有权、敏感实例外置）；领域包不依赖 HTTP/`pgx`；注释中文。
- **结论**：Spec 1 阻断，按票 26 立修复票 28，不回滚已进 main 的提交。main `4dd937eb` 上相关代码未变。

## 完成记录（2026-10-16，Claude Code 会话，分支 `claude-oc15`）

**基线**：分支基 `858280a2`（= 远端 main）。代码两笔：`60244fa6`（余下四口）、`0e3bb7df`（受控关闭与重开 + ADR-0183）；本记录一笔在其后。接手理由：通道 4 自 2026-10-13 起无在途（分支 `mcp4-oc15f` 已 `merged/`），本机 `/list-agents` 无别的会话；用户令独立完成。

**之五 · 余下四口**（`60244fa6`）。前置：19（准入范围桥，`8e1e9777`）、20（三口规范化，`857453d8`）都已进 main。
- 关段、建派送任务、有效时间判断、承运商首次有效收寄判断四个端点行直接挂 `operatorDecisions.transport`；同笔撤隔离写放行（ADR-0150 决定三）：放行名单删这四行（写开关只剩 `/shipment-requests`），`isolatedWriteAdmission` 的运输履约那一格与 `assembleBusinessEndpoints` 的 `isolatedTransportFulfillment` 入参删去，`IsolatedCommandIntake` 上这四口的方法撤下，四口的译装用例迁到 `OperatorDecisionIntake`，自报租户 / 尾随内容 / 未知键对 TF 六个运营决定口逐口跑。
- 演示种子：配置员加这四种决定的授予，准入对照四行，治理区间 17–20。
- 管理台已经统一经 `withBearer` 带令牌（`postMasterData`），前端不用改；不设 `IDP_PARCEL_GOVERNANCE_SELF_AUTHORITY` 时这几页答 `OUTSIDE_ADMISSION_SCOPE`，README 写明。

**之六 · 受控关闭与重开**（`0e3bb7df`）。做什么第 3 条要「与 PS owner 对齐」的那一格，由 [ADR-0183](../../../docs/adr/0183-operator-channel-addresses-controlled-closure-and-reopening-by-parcel-and-derives-the-shipper-from-the-accepted-request.md) 代 PS owner 裁定（越权风险点见该记录）：载荷只指名包裹，服务端反查当前已接受委托取来源身份；请求方只可能是那份委托的客户账户（`CUSTOMER-ACCOUNT/<标识>`），关闭由责任来源推出、重开随货主重授权证据出现，货主账户恒取委托客户；提交操作者不进命令。演示种子补两种决定的对照与区间 21、22（同委托侧另三口，不给配置员授予）。

**完成判据**（钉 `0e3bb7df`；Linux（WSL2），go1.26.8，真库为 55432 门禁库）

- ✅ 上列各口四格各答其格，装配测试与端点表一一对照：发行方未配置——`TestEveryAssembledEndpointAnswersUnconfigured`；令牌缺失 401 与不在准入范围 403——`TestSwappedDecisionFacesAnswerFromTheOperatorChannel`；无此决定种类的授予——新增 `TestSwappedDecisionFacesRefuseAGrantOfAnotherDecisionKind`（只缺这一种授予即答 `OPERATOR_NOT_GRANTED`）。两表都已含本票全部十一口。
- ✅ 租户、提交操作者与委托来源身份不从请求体收，带即 400：TF 四口 `TestEveryDecisionFaceRefusesSelfReportedTenantTrailingContentAndUnknownKeys` 与真库表的自报租户格；受控关闭与重开 `TestControlledClosureTakesTheShipperFromTheAcceptedRequestOfTheParcel`、`TestReopeningTakesTheShipperAccountFromTheAcceptedRequestOfTheParcel`（夹带请求方、委托标识、租户、货主账户各一格）。复核人等三口在之三已证。受控关闭与重开的命令没有提交操作者格，运营操作关闭的请求方为空（同上用例）。
- ✅ 越过 Intake 之后：TF 四口答业务结果——`TestTransportFulfillmentCommandLinesAnswerBusinessOutcomesAgainstARealDatabase`（四种决定各登对照与本产品区间，经生产的准入范围桥越过准入）；受控关闭与重开在生产装配上停在 `UNDECIDED / AUTHORITY_UNAVAILABLE`、合成授权下答业务结果——`TestClosureAndReopeningThroughTheOperatorChannelAgainstARealDatabase`（令牌 → Intake → 编排 → 册；货主指令关闭 201，册上请求方与责任来源由服务端推出；不带证据的重开 `SHIPPER_REAUTHORIZATION_MISSING`，带证据的重开 201）。
- ✅ 四口隔离放行已撤，隔离用例改写为真渠道答复格用例（之五）；隔离写二分表只剩 `/shipment-requests`。
- 先红后绿：已换口表与只缺一种授予两条在装配换口前跑红（四口答 `ACCESS_CHANNEL_NOT_CONFIGURED`）；PS Intake 四条先挂骨架跑出行为红。受控关闭与重开那两行的装配用例写在换口之后，补做变异取证（端点行退回 `UnconfiguredIntake{}` 即红）。真库表与端到端用例、TF 译装迁移用例是对已有路径的取证，一写即绿。

**门**（作者自验）：两笔各自在其上跑过全仓 `go build` / `go vet`（退 0）、改动 `.go` 的 `gofmt`（无输出）与 CR / BOM 检查；`cmd/parcel-api`、TF / PS 的 `adapters/accessidentity` 与 `adapters/http`、`./internal/architecture/...` 带 DSN `go test -p 1 -count=1` 全 ok；一次性库上 `seed.sh` 整跑退 0、新登的授予 / 对照 / 区间逐行查得到，库已删；机制清点重生成两次都与现行文档逐字相同。全仓全量归推送方那一跑。

**评审**：作者自审，两轴串行（本仓不用子代理）。不算非作者评审。

**证据层级**：全部 `S`。

**发现，不做（另立票）**
- `tfhttp.IsolatedCommandIntake` 已无装配使用者：余下作业事实各口的方法是票 10 换口时没有同笔撤的（ADR-0150 决定三）。它的译装用例是那几口线格式的主要覆盖，`WorkFactCommandIntake` 在本包没有解码用例；要撤得先把用例迁过去。`register_effective_delivery.go` 与 `register_transport_handover.go` 头注说它是「唯一的实现」，同属过期。
- `nodeopshttp.IsolatedCommandIntake` 同样已无装配使用者（`isolatedWriteAdmission` 头注自承），同一处理。
- 演示配置员没有委托侧五种决定的授予（区间与对照都在）：管理台在这五口上答 `OPERATOR_NOT_GRANTED`。授权缺口（psb/07）补上之前给了授予也只会停在各自的授权未决格；补授予随 psb/07 那一笔做更合适。

### 进 main 记录（2026-10-16，Claude Code 会话自落）

- 分支 `claude-oc15` 三笔 `60244fa6` / `0e3bb7df` / `58626ca1` 直接叠在远端 main `858280a2` 之上，**快进**进 main，不重放、SHA 未改写；推送前紧贴 push 再 fetch，远端仍为 `858280a2`，`origin/main..58626ca1` 只有这三笔。`git push origin 58626ca1:main` 退 0，`ls-remote` 为 `58626ca1`。
- 推送方全量一次：钉 `58626ca1` 的全新 detached 检出，带 DSN，`go test -p 1 -count=1 -v ./...` 退 0——有测试的包 141 个 ok、0 FAIL；`--- FAIL` 0、`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）。同一检出上全仓 `gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0。
- 评审：只有作者自审，**没有非作者评审**——本机无其他会话可派，用户令独立完成。parallel-sessions「合入前独立评审」这一格未满足，如实记下；补评可钉 `858280a2..0e3bb7df` 另做，ADR-0183 的越权风险点 1 一并请 PS owner 过目。

### 补记（2026-10-16，票 operator-channel/28）：受控关闭与重开两口凭哪条重放代数

本票做什么第 2 条要逐口记下凭哪条代数，这两口当时漏记、也确实没有（票 28 现象节）。现在凭 ADR-0183「复核记录二」与 `domain.ContinuedAttemptRegister.RecordedRequest`：册上已有同类、请求内容逐格相同的决定即同一次请求的重放，答 `EXISTING_RESULT` 交回原决定。与 TFC-1 / CCC-1 一类「同键同内容是重放、同键异内容是冲突」不同属，三点要分清：**异内容即新决定**，照旧经领域追加；**没有内容冲突那一支**；**比对扫全册**，不只看生效中的那一条。

