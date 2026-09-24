# 08 隔离形态：主链命令面按 ADR-0091 逐口放行合成写

Category: enhancement
Status: resolved · 已进 main——2026-09-24 评审 ← 通道 1（非作者）两轴无阻断、非阻断四条随票记；推送方通道 1 在 main `8cd50199` 之上逐笔重放分支 `mcp6-oc08` 的 `443a472e..29030a8e`（代码 `f043ed03`…`8dd1b339`、完成记录 `5d7ed969`），清点重生成 `ebafb877`；分支作封存出处，新旧 SHA 对照见 Comments「进 main 记录」。此前 in-progress——2026-09-24 通道 6 认领（通道 1 改派 task-a25599eb；原卡通道 4 零提交已撤），分支 `mcp6-oc08`，基 `443a472e`；实现、自验与 psb/05 重走已交（见文末「完成记录」）；拆法经用户授权通道 4 自决认可；ADR-0149 决定五写明本票的隔离放行不因生产渠道落地而退场
Blocked by: 无
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 乙轨——**不是操作者渠道**，是让隔离环境里的主链先答业务结果的那条路
地盘：`cmd/parcel-api` 端点表里主链命令面的装配行与各自的隔离 Intake 类型（照 `IsolatedPartyIdentityIntake` 与隔离提交口的形状），各上下文 `adapters/http` 里需要的隔离 Intake 实现。
出处：[ADR-0091](../../../docs/adr/0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 逐口放行（先例：隔离提交口、`/commercial-*` 身份族）；[psb/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 格 7、11、12、22 实测 `403 ACCESS_CHANNEL_NOT_CONFIGURED`，端点表注释写明这几行「两个开关都换不了」。

## 做什么

1. 逐口归类（与 04 同一张表）：节点收寄、场外揽收与尝试、承运商首次有效收寄判断、交接、移动、派送任务登记与发起、交付、段关闭、有效时间判断、关务外部结果、监管凭证登记、外部资金事实——ADR-0100 决定四、五明文不给这些口开操作者渠道。
2. 按 ADR-0091 逐口放行：只收 `SYN-` 租户的合成写，来源身份与时间照 ADR-0023 由请求如实携带、不代铸；一口一笔、一口一个变量，不并进既有开关。按上下文拆笔（NO、TF、CC、SA）。
3. 放行后在隔离环境按 psb/05 的动线重走这几格，答复写回 psb/05（那张票的第 3 步归通道 2 或其接手者）。

## 不做

- 生产上这些口的真渠道（09）；不放宽 `SYN-` 前缀门禁。

## 完成判据

- 隔离环境里上列各口对合成写答业务结果而不是 `ACCESS_CHANNEL_NOT_CONFIGURED`；生产形态（开关为 nil）逐字节不变（带装配测试）。

## 完成记录（通道 6，分支 `mcp6-oc08`；待非作者评审与重放；证据只记 `S`）

### 逐口归类（第 1 步，与 04 同一张表）

范围：基 `443a472e` 端点表里挂字面量 `UnconfiguredIntake{}` 的全部写行，加上前票已换成隔离变量的身份族。判据只看两件：票面与 psb/05 点没点名、所属 ADR 把它归哪一族；归属未裁的照实写「未归」，不替别的票定。

| 归属 | 口 | 本票处置 |
|---|---|---|
| **08 放行**（票面「做什么」第 1 条；生产渠道归 ADR-0149：作业事实走操作者族「作业事实登记」能力面，外部结果与资金事实走集成客户端族） | NO：`/node-operations/receptions`；TF：`/transport-fulfillment/offsite-pickups`、`/transport-fulfillment/offsite-pickup-attempts`、`/transport-fulfillment-carrier-first-effective-pickup-judgments`、`/transport-fulfillment/handovers`、`/transport-fulfillment/movement-facts`、`/transport-fulfillment-dispatch-task-registrations`、`/transport-fulfillment-delivery-dispatch-triggers`、`/transport-fulfillment/deliveries`、`/transport-fulfillment-segment-closures`、`/transport-fulfillment-effective-time-judgments`；CC：`/customs/external-results`、`/customs-regulatory-credential-registrations`；SA：`/settlement-external-funds-fact-registrations` | 逐口放行，见下一节 |
| 同族未列（主链事实的更正口，票面与 psb/05 都没点名） | `/transport-fulfillment/handover-corrections`、`/transport-fulfillment/offsite-pickup-corrections`、`/transport-fulfillment/delivery-proof-corrections`、`/settlement-external-funds-fact-correction-registrations` | 不放；照旧字面量，且在类型上装不进各隔离 Intake。要放另票 |
| 票面未列、族归属待定（ADR-0085 引入的 TF 运营写面里另两口） | `/transport-fulfillment-load-assignment-registrations`、`/transport-fulfillment-participation-terminations` | 不放；同上 |
| 04（ADR-0085 决定一登记册配置写面 → ADR-0100 决定四操作者渠道） | PP：价卡登记、序列登记 / 复核 / 预览、参考目录登记；NR：网络目录各族登记；CC：解释规则、门禁目录、候选口岸、申报路径、建案要求、税费协作、税费核对登记；TF：承运凭证登记与适用改变、有效时间规则登记、总单登记与修订；PC：服务形态、产品—渠道映射、注册号类型登记与停用、渠道账号使用授权与撤销，以及前票已隔离放行的身份族各口与法人资料登记；VE：目录各类登记 | 不碰。**知会 04**：`/customs-regulatory-credential-registrations` 同属 ADR-0085 决定一登记口，但票面、psb/05 格 11 与 ADR-0149 决定一（「监管凭证」归外部结果族）都把它点给了本票；隔离形态已由本票放行，生产渠道归属以 ADR-0149 为准 |
| 06（商业发布与计价回放） | `/commercial-publications`、`/pricing-evaluation-replays`；ADR-0126 发布路径四口（`-previews`、`-drafts`、`-draft-approvals`、`-draft-publications`）是否一并归 06 由 06 定 | 不碰 |
| 客户渠道（ADR-0139 至 0142，Proposed，接受与否归用户） | `/shipment-requests/withdrawals`、`/shipment-requests/parcel-cancellations`、`/shipment-requests/supplements`、`/shipment-requests/source-data-amendments`、`/claims` | 不碰 |
| 未归（委托侧运营决定口，04 / 08 / ADR-0149 都没覆盖） | `/shipment-requests/continued-attempt-closures`、`/shipment-requests/continued-attempt-reopenings`、`/shipment-requests/manual-review-completions`、`/shipment-requests/rejections`、`/shipment-requests/authorized-dispositions` | 不碰；族归属待裁 |

### 逐口放行（第 2 步）

每个上下文一个隔离命令 Intake（`nodeopshttp` / `tfhttp` / `customshttp` / `settlementhttp` 的 `IsolatedCommandIntake`），只实现已放行口的 Intake 接口；装配点一口一个变量、一个上下文一个 `if`，不并进读开关与身份族的 `if`。租户格只来自写开关（`SYN-` 前缀门禁一字未动）；载荷带 `tenantId` 即拒。事实身份与发生时间照 ADR-0023 从载荷收、缺席交编排、不代铸。

| 口 | 提交 | 注入的格（契约点名来自认证结果的） | 真库用例答复 |
|---|---|---|---|
| 收寄 | `0ed1d78f` | 租户、节点（`SYN-NODE-SHA-HUB`） | 拒收 `INTAKE_NOT_FORMED`；明确接收 `RECEPTION_UNDECIDED` |
| （铺缝）| `833af4b1` | 交接、交付两组 Intake 按口拆成单方法接口，零行为变化 | — |
| 场外揽收 | `5c8f90cc` | 租户 | `201 PICKUP_REGISTERED` |
| 揽收执行 | `9a9af1f0` | 租户 | `201 ATTEMPT_RECORDED` |
| 承运商首次有效收寄判断 | `2bdb80cf` | 租户 | `200 PICKUP_PENDING` |
| 交接首登 | `101d050c` | 租户 | `201 HANDOVER_REGISTERED` |
| 移动事实 | `81338d91` | 租户、来源（`SYN-SOURCE/self-operated-executor`） | `201 MOVEMENT_FACT_RECORDED` |
| 派送任务登记 | `9e40f056` | 租户 | `201 DISPATCH_TASK_OPENED` |
| 派送发起 | `aacda89b` | 租户 | `200 NOT_A_DELIVERY_TRIGGER` |
| 交付首登 | `05aefa3f` | 租户 | `200 SOURCE_NOT_ACCEPTED` |
| 段关闭 | `c5f03169` | 租户 | `200 SEGMENT_NOT_FOUND` |
| 有效时间判断 | `114074d2` | 租户 | `200 INPUT_NOT_ACCEPTED` |
| 关务外部结果 | `0c6dc343` | 租户；接收时刻取服务端时钟 | `200 UNATTRIBUTABLE` |
| 监管凭证登记 | `1a38d8f8` | 租户（拼回登记快照，交 `registrationjson` 同一份译装） | `201 REGISTERED` |
| 外部资金事实采用 | `21bbc0e2` | 租户（同上） | `201 FUNDS_FACT_ADOPTED` |
| （自审修复）| `06a35347` | 端点表与 Intake 契约注释跟上现状、去掉数别处字段的计数 | — |

### 完成判据对照

- ✅ 隔离环境各口答业务结果：上表真库用例逐口（Intake 走 `buildIsolatedWriteAdmission`、编排走生产装配、端点走生产构造函数），另 psb/05 真进程重走逐口越过 403（第 3 步）。
- ✅ 生产形态逐字节不变：`TestProductionFormAnswersTheAdmittedCommandLinesByteForByteUnconfigured`（隔离入参全 nil 时放行名单上每一行的状态、内容类型、响应体逐字节等于字面量未配置那一份）+ 既有 `TestEveryAssembledEndpointAnswersUnconfigured`；`TestIsolatedReadAdmissionCannotOpenTheAdmittedCommandLines` 证读开关换不了这些行。
- ✅ 放行出声：启动日志 `admittedCommandLines` 与二分表同一份名单（`TestIsolatedWriteAdmissionNamesTheAdmittedCommandLines`）。

### 作者自验（钉 `06a35347`）

`go build ./...` 与 `go vet ./...` 退 0，`gofmt -l .` 无输出。真库单跑 `TestIsolatedReceptionAnswersBusinessOutcomesAgainstARealDatabase -v` 为 `PASS`（不是 `SKIP`）。受影响范围一次：`go test -count=1 -p 1 -v`（带 `IDP_PARCEL_POSTGRES_DSN`）跑 `cmd/parcel-api`、NO / TF / CC / SA 的 `adapters/http` 与 `internal/architecture/...`——退 0，`--- FAIL` 0 条、`--- SKIP` 0 条。反向依赖按 `go list … Deps` 反查只有 `cmd/parcel-api`。推送方自审过一轮 `/code-review` 两轴（不算非作者评审），发现与修复在 `06a35347`。

### psb/05 重走（第 3 步）

2026-09-24 20:44，代码钉 `06a35347`，一次性库 `idp_mcp6_oc08_rewalk` 按 `seed.sh` 原样灌，`parcel-api` 两个隔离开关同取 `SYN-TENANT-01`，走完删库。答复逐格写在 psb/05 格 7、11、12、22 下的「08 重走」一条。摘要：四格各口全部越过 `403`；越过之后停在各自下一处已知的缝——收寄停在身份核对缝（格 8 同缝）、外部结果停在「归属不上」（格 9 没有提交）、派送发起停在 `REQUIREMENT_MISSING`（格 13）、有效时间判断停在轨迹事实不在册（格 14）；**新暴露一处**：交付 `SOURCE_NOT_ACCEPTED`，派送尝试登记册全仓没有生产写入方，psb/05 格 12 已记为待立票。对照组：更正口仍 `403`、自报租户 `400`、同一份收寄重放 `EXISTING_RESULT`。

### 判断项（交评审看）

1. **收寄的节点与移动事实的来源是注入的合成常量，不从载荷收**：两格的 Intake 契约都写着来自认证结果（ReceptionIntake「租户与节点身份只能来自认证结果」、MovementFactIntake「自营还是外部由认证结果说」），ADR-0091 决定一第二判据不许采信自报。代价是隔离环境里收寄只落在一个合成节点上；要换节点改 `isolatedReceptionNode`，不是去采信请求。
2. **收寄的服务结果标记留空**：命令注释写明它「由接入层查好带入」，隔离形态没有那道查询，收了就是采信调用方自报的 PS 结论——已取消或终局的包裹在隔离环境里收寄时不带标记。
3. **`833af4b1` 改了 tfhttp 导出签名**（交接、交付两组 Intake 拆成单方法接口、原名留作合集、四个端点构造函数参数收窄）：为了让「未放行的口在类型上装不进隔离 Intake」这把编译期锁在这两组上也成立；既有调用点一行未改照编，认领广播里已预告。
4. 各上下文的封闭解码与「拼回租户」助手各写一份，不抽到平台包：上下文之间不共享适配器代码，与 PC 身份族那份同形。

## Comments

### 评审 ← 通道 1（推送方自跑；非作者，隔离子代理认证失败未起，两轴串行、各写各的）· 钉 `29030a8e`（基 `443a472e`，只读，不跑全仓）· 2026-09-24 22:0x

前一评审会话停在 Standards 轴读 `cmd/parcel-api/endpoints.go`、未留结论；本条从头重跑两轴，不沿用。

**Standards** — 阻断：无。非阻断三条：
① `tfhttp.DeliveryIntake` 注释称 `IsolatedCommandIntake`「是唯一的实现」，而它只实现 `DeliveryRegistrationIntake`，本票 `TestIsolatedCommandIntakeServesOnlyAdmittedLines` 正断言它装不进 `DeliveryProofCorrectionIntake`——注释与代码相反；照同批 `HandoverIntake`「只实现首登那一口」改一句即可。
② `assembleBusinessEndpoints` 头注「这句从前说的是……因此改成现在这句」又续一环变更史，AGENTS.md「写代码注释」：不写变更说明；可收成只述现行规则。
③ 判断题 · Duplicated Code：收寄三态搭配门在 `nodeopshttp.applyReceptionClaim` 与 `parcel-frontline-import` 的 `intakeRowFrom` 各一份，此刻核过一致，但只靠手工同步；要收成一份得下沉 NO 应用层，另立票。
无发现：ADR-0091 决定一、三、四（注入值带 `SYN-`；自报租户、节点、来源即拒；读开关换不了写行）；ADR-0023（身份与发生时间从载荷收、缺席交编排、不代铸；CC `ReceivedAt` 是服务端自己的接收时刻）；一口一变量、编译期锁；注释中文，无跨文件计数与行号。

**Spec** — 阻断：无。非阻断一条：
① 完成记录「作者自验」节末句「推送方自审过一轮」应为作者自审（修复笔 `06a35347` 是作者的）；下方进 main 记录按作者自审计，原句不改。
无发现：放行的恰是「做什么」1 点名的各口，与基 `443a472e` 端点表逐行对过，其余挂字面量的写行都落在归类表某格；一口一笔、按 NO / TF / CC / SA 拆，`SYN-` 前缀门禁未动；psb/05 格 7、11、12、22 写回且只记 `S`，格 12「派送尝试无写入方」核过 `.scratch` 下无在途票；完成判据两条都有用例（真库用例逐口；`TestProductionFormAnswersTheAdmittedCommandLinesByteForByteUnconfigured` 覆盖放行名单每一行）；判断项四条与 ADR-0091 / 0023 及身份族先例相容，判断项 2 的标记不挡接收、受控批量口同样不填，是本票之前就在的缝。

结论：可接受。

### 进 main 记录（推送方 · 通道 1）

- **门**：评审 ← 通道 1（非作者）两轴无阻断，合 parallel-sessions「推送方只在评审为无阻断时重放」；非阻断四条随票记、不挡合入，作者可另立票。通道 1 是本票派单方（task-a25599eb 写明「我派非作者评审后重放」）；通道 6 未对该卡报 done，交活以分支上的完成记录为准。
- **重放**：在 main `8cd50199` 之上 cherry-pick `443a472e..29030a8e`，无冲突。其间进 main 的 operator-channel/01 与 routing-first-cut/07 与本票没有重叠文件（两侧 `git diff --name-only` 取交为空），按 parallel-sessions 直接在新 tip 上重放；本票动过的每份文件 blob 与分支 `29030a8e` 全同。分支上无清点笔，批 tip 干净检出重生成为 `ebafb877`。新旧 SHA 对照（分支 → main）：`ec70f4ac`→`4bfc9c3e`（认领）、`0ed1d78f`→`f043ed03`（NO）、`833af4b1`→`853e5587`（铺缝）、`5c8f90cc`→`822c75a4`、`9a9af1f0`→`0ad294cc`、`2bdb80cf`→`99f74a54`、`101d050c`→`25d6ab4d`、`81338d91`→`0aa5ce42`、`9e40f056`→`d3a7dcdf`、`aacda89b`→`6817fc4b`、`05aefa3f`→`23f3f556`、`c5f03169`→`019db8ad`、`114074d2`→`6f5a9903`（TF 各口）、`0c6dc343`→`d4f52c1c`、`1a38d8f8`→`1f5e8d73`（CC）、`21bbc0e2`→`7393c1d0`（SA）、`06a35347`→`8dd1b339`（自审修复）、`29030a8e`→`5d7ed969`（完成记录与 psb/05 重走）。
- **验证**：钉 `ebafb877`（与本记录一笔只差 `.md`）：全仓 build / vet 退 0，`gofmt -l cmd internal` 无输出；先单跑本票真库用例（`cmd/parcel-api` 下 `TestIsolated…AgainstARealDatabase` 各条）是 PASS 非 SKIP；带 DSN `go test -p 1 -count=1 ./...` 120 包 ok、0 FAIL（另 15 包无测试文件）。
- **推送**：推前 `ls-remote` 远端 main 仍是 `8cd50199`，`8cd50199..ebafb877` 只有本票重放各笔与清点一笔；`git push origin ebafb877:main`。本记录随后单独一笔。
