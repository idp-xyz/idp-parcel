# 15 委托与履约的运营决定口：操作者渠道增「运营决定」能力面，委托侧五口与 TF 管理台上的决定与判断口换操作者 Intake

Category: enhancement
Status: in-progress——2026-09-25 通道 4 认领（用户令独立完成 ADR-0151 那件；前置 03、14 均已 resolved），分块逐笔进 main：之一、之二、之三已进 main（见文末进展记录）；余下装配换口被 07 挡住，受控关闭与重开等请求方格与 PS owner 对齐。此前 ready-for-agent——2026-09-24 随 ADR-0151 立（用户同日「同意你的决定，开干」）；2026-09-25 通道 2 按用户「开干前，全面审查，确保确实如此」复核后改定范围、阻塞与完成判据，复核记录见文末 Comments
Blocked by: 装配换口一格——[07](./07-admin-web-login-gate.md)（管理台登录门：换口后各口要 Bearer 令牌，管理台今天调复核、拒绝、处置与有效时间判断几口不带令牌），以及演示环境接上发行方与合成操作者授予；受控关闭与重开两口——请求方格与 PS owner 对齐（本票做什么第 3 条）。03、14 已 resolved
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

