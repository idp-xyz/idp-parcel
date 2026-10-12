# 10 设备登记与「作业事实登记」能力面：一线作业事实走操作者渠道族

Category: enhancement
Status: 机制半边完工，待评审与重放——2026-10-12 20:2x 通道 3 接续通道 1 前一会话（通道 1 派单 task-cf472ccc）：判据第一条补三处用例，代码 tip `d0361b21`；做什么第 4 条与完成判据第二条留待 07、17，完成记录见文末。此前 in-progress——2026-10-12 14:5x 通道 1 接手认领：通道 1 派单 task-caef2c2f 给通道 4，其新会话列完计划即 crash（用户报；14:52 实测工作树 `/home/tops/workspace/idp-parcel-mcp4-oc10b` 与分支 `mcp4-oc10b` 已建、停在 `ff3b01fa`、零提交零改动），用户令通道 1 接手。分支 `mcp4-oc10b`、基 `ff3b01fa`；从 `mcp4-oc10@382c4efb` 接着做（通道 4 前一会话的未提交现场原样封存，非集成候选），先写自己第一片 red 再对照取用。本轮只做机制半边——做什么第 1–3 条与完成判据第一条；第 4 条更正口与完成判据第二条「换口同笔撤隔离放行」与 [04](./04-registration-write-faces-take-operator-intake.md) 第 1 件同一前提（演示环境接上发行方与合成操作者授予），等 [07](./07-admin-web-login-gate.md)。此前 in-progress——2026-10-11 01:00 通道 4 认领（`2d90f548`，只在分支 `mcp4-oc10` 上，基 `2603f9ad`）。此前 ready-for-agent——2026-09-24 随 ADR-0149 立（用户授权通道 4 自决）
Blocked by: 01、03（均已 resolved）；[07](./07-admin-web-login-gate.md)——只挡做什么第 4 条与完成判据第二条：换口要同笔撤隔离放行（ADR-0150 决定三），前提与 [04](./04-registration-write-faces-take-operator-intake.md) 第 1 件是同一件，演示环境接上发行方与合成操作者授予（2026-10-12 通道 3 补）；[17](./17-demo-issuer-dex-and-synthetic-operators.md)——同一前提里发行方与合成操作者那半（2026-10-12 通道 3 补）
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施
地盘：`internal/accessidentity`（设备册、能力面授予格、铸造里「操作者主体 + 设备」两件）与其迁移、受控登记 CLI、参数登记册增「作业设备」一行；作业事实命令口的装配留给 13、14 齐了之后逐口换。
出处：[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定二；[ADR-0023](../../../docs/adr/0023-work-fact-identity-and-time-are-minted-by-the-device.md)。

## 做什么

1. 操作者册加能力面「作业事实登记」，授予按租户 × 作业范围（节点、场外作业范围）。
2. 设备册：设备标识、所属租户与作业范围、状态（登记 / 停用），受控 CLI 登记；参数登记册增「作业设备」一行（租户取值）。
3. 铸造：令牌有效 + 在册有授予 + 设备已登记且在用，才铸带「操作者主体 + 设备」的信封；答复另加「设备未登记或已停用」一格。
4. 主链事实的更正口随原事实归本能力面（2026-09-24 随 [ADR-0151](../../../docs/adr/0151-unassigned-command-faces-get-their-families.md) 决定四补）：`/transport-fulfillment/handover-corrections`、`/transport-fulfillment/offsite-pickup-corrections`、`/transport-fulfillment/delivery-proof-corrections` 与首登口同一份认证与授予。
5. 关段 `/transport-fulfillment-segment-closures`、手工建派送任务 `/transport-fulfillment-dispatch-task-registrations`、有效时间判断 `/transport-fulfillment-effective-time-judgments` 与承运商首次有效收寄判断 `/transport-fulfillment-carrier-first-effective-pickup-judgments` 不归本能力面（2026-09-25 随 ADR-0151 决定六改，两个判断口同日补）：它们是管理台上的运营写决定，归 [15](./15-operation-decision-faces-take-operator-intake.md) 的「运营决定」能力面；派送发起口照旧在本能力面的范围内。

## 完成判据

- 四格答复各有用例；设备停用后同一操作者在该设备上被拒、换已登记设备照常；事实身份与发生时间照旧由请求携带、服务端不重签。
- 随 [ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定三（2026-09-24 补）：换上作业事实能力面的各口（今天经写开关放行的节点收寄、揽收、交接、移动、派送、交付、派送尝试登记等）同一笔撤下隔离放行，隔离放行用例改写为真渠道答复格用例。

## 完成记录（2026-10-12，机制半边；分支 `mcp4-oc10b`，基 `ff3b01fa`；作者一侧为通道 1 前一会话与通道 3；待非作者评审与重放；证据只记 `S`）

本轮范围照认领笔：做什么第 1–3 条与完成判据第一条。第 4 条与完成判据第二条没做，见「未做」。

**各笔**

| 笔 | 作者 | 内容 |
|---|---|---|
| `5a8a342f` | 通道 1 | 认领：票面 Status，本轮只做机制半边 |
| `ff29ea23` | 通道 1 | 领域与迁移：`CapabilityWorkFactRegistration` 按作业范围授予（`NewWorkFactRegistrationGrant`、`HoldsWorkFactAt`）；作业设备册（`DeviceRegistration`、`DeviceDeactivation`、`RecordedDevice`，端口 `DeviceRegistry` / `DeviceRegistrar`，`access_identity` 0004，postgres 适配器 `DeviceRegistry`）；`WorkFactMinter` 在令牌、授予、设备都过之后铸 `WorkFactEnvelope`，另加 `ErrDeviceNotInUse` 一格 |
| `5f8b3226` | 通道 1 | `parcel-access-register` 增 `device-register`、`device-deactivate`，`operator-grant` 批收 `workScope`；参数登记册 `PAR-INT-10`；`doc.go` 一段 |
| `71272b5d` | 通道 1 | 撞键比对逐列钉住：授予与设备只换范围种类、只换范围引用各答冲突 |
| `d75c1a8a` | 通道 1 | 机制清点在 `71272b5d` 干净树上重生成 |
| `d0361b21` | 通道 3 | 判据第一条补三处用例（见下）；表结构用例的 Covers 原引 01 做什么第 2 条的话而没说出处，改为明说 |
| 本笔 | 通道 3 | 完成记录、Status、Blocked by |

`d0361b21` 只改两份既有测试文件、不增文件；清点在它的干净导出（`git archive`）上重生成，与已提交的一字不差，不另成笔。

**完成判据逐条**（钉 `d0361b21`）

- ✅ **四格答复各有用例**：`TestWorkFactAnswerGradesDoNotStandInForEachOther`，每种情形只被自己那一格的哨兵认出，且都不产出铸成的信封。
  - 发行方未配置 → `ErrAccessChannelNotConfigured`；令牌不过 → `ErrCredentialRejected`。
  - 不在册、绑在别的租户、授予在别的作业范围、只有登记册配置写授予、作业事实授予已撤（`d0361b21` 补）→ `ErrOperatorNotGranted`。
  - 设备不在册、停用早于请求、停用恰在请求时刻、登在别的作业范围、登在别的租户、册交回别的租户的设备（`d0361b21` 补）→ `ErrDeviceNotInUse`。
  - 核验方取不回公钥集、操作者册读不动、设备册读不动各答依赖故障格，不折进上面四格。
- ✅ **设备停用后同一操作者在该设备上被拒、换已登记设备照常**：`TestDeactivatedDeviceRefusesTheOperatorWhileAnotherRegisteredDeviceStillServes`；`TestDeviceServesUntilItsDeactivationInstantAndIsRefusedFromThen`（`d0361b21` 补：同一台设备停用时刻前一纳秒照常铸，自该时刻起答设备不在用）。真库一侧 `TestDeviceDeactivationEndsUseFromItsInstant`（停用自其时刻起生效，`FindDevice` 读回带停用），CLI 一侧 `TestDeviceRegisterAndDeactivateEndToEnd`。
- ✅ **事实身份与发生时间照旧由请求携带、服务端不重签**：`WorkFactRequest` 与 `WorkFactEnvelope` 都没有事实身份与发生时间的格，铸造碰不到它们；`TestWorkFactEnvelopeCarriesNoServerInstant`（同一份出示在两个时钟下铸出的信封逐格相同）。本分支没动任何作业事实命令口与其载荷（`git diff --stat ff3b01fa d0361b21` 不含 `cmd/parcel-api` 与各上下文的 `adapters/http`），载荷照旧带这两格。
- ❌ **完成判据第二条（换口同笔撤隔离放行）**：没做，见「未做」。

**判别力**（`go test -overlay`，变异副本在 `/tmp/oc10mut`，不提交；每个变异对两版用例各跑一遍，旧版经 overlay 换回 `d75c1a8a` 的 `work_fact_test.go`）

- 在 `d75c1a8a` 的用例上全绿、被 `d0361b21` 拦下的——补这三处用例的理由：
  - `HoldsWorkFactAt` 去掉生效判断（`EffectiveAt`）→「作业事实授予已撤」红。
  - `MintWorkFact` 去掉设备的租户比对 →「册交回别的租户的设备」红。原有的「登在别的租户」一格用按键取的替身，查不到就答不在册，碰不到这道比对。
  - `MintWorkFact` 把「此刻在用」换成「有停用即拒」→ `TestDeviceServesUntilItsDeactivationInstantAndIsRefusedFromThen` 红。
- 两版用例都拦下的：核验方各格折成令牌不过；去掉按作业范围核授予；去掉设备的作业范围比对；设备那一格折成未授予；去掉在用判断；`InUseAt` 把停用时刻本身算作在用；信封带上铸造时刻（`TestWorkFactEnvelopeCarriesNoServerInstant` 红）。
- 真库（带 DSN）：`FindDevice` 不读停用 → `TestDeviceDeactivationEndsUseFromItsInstant` 红。

**做什么第 1–3 条的落点**：第 1 条——`TestWorkFactRegistrationIsGrantedPerWorkScope`，真库 `TestWorkFactRegistrationGrantRoundTripsItsWorkScope`，CLI `TestWorkFactRegistrationGrantBatchEndToEnd`。第 2 条——真库 `TestDeviceRegistrationRecordsReplaysAndNeverOverwrites`、`TestDeviceDeactivationEndsUseFromItsInstant`、`TestWorkFactAndDeviceTablesGuardTheInvariantsThemselves`、`TestDeviceRegistrationWritesRefuseToRunOutsideATransaction`，CLI `TestDeviceRegisterAndDeactivateEndToEnd` 与触库前拒收 `TestWorkFactAndDeviceBatchTranslationRefusesBeforeTouchingTheDatabase`。第 3 条——见判据第一条，另有 `TestDeviceIsLookedUpOnlyAfterTheGrantAndAdmissionOnlyAfterTheDevice`。

**参数登记册**：`PAR-INT-10` 当前登记以「待提供」开头，是登记规则所列状态之一；格里只写尚无租户、演示租户的合成设备只记 `S`，没有任何租户取值。核过，未改。

**判断项**（票面没写定、由作者一侧替 owner 定了的，标「越权风险点 · 待 owner 复核」）

1. **设备标识只在租户内唯一**：键为（租户、设备标识），别的租户登的同名设备是另一台。票面与 ADR-0149 只写「设备标识、所属租户」。越权风险点 · 待 owner 复核。
2. **一台设备只登在一个作业范围上，铸造要它与请求的作业范围逐字相等**：登在别的作业范围上同答「设备未登记或已停用」；同一标识改登另一范围答 `CONTENT_CONFLICT`，设备换节点只能停用后另登新标识。票面只写「所属租户与作业范围」，没写设备与请求的范围须相等，也没写一台设备能否跨范围。越权风险点 · 待 owner 复核（跨节点流转的共享设备、随车作业端要不要一机多范围，归 accessidentity 与 NO / TF owner）。
3. **作业范围按种类与引用逐字相等，不按包含关系推**；种类是封闭集 `NODE` / `OFFSITE`。ADR-0149 只列了「节点、场外作业范围」。越权风险点 · 待 owner 复核。
4. **停用是终局**：没有再启用，标识停用后不复用。票面只写「状态（登记 / 停用）」，终局是从 ADR-0023「设备重装、重置或换人换机……必须形成新的设备身份」推出来的。越权风险点 · 待 owner 复核。
5. **在不在用按铸造那一刻判，不按事实自报的发生时间**：后果是离线期间在设备上登下、设备停用之后才排空上传的事实一律被拒。票面与 ADR 都没写。越权风险点 · 待 owner 复核（与 ADR-0021、ADR-0023 离线容忍的取舍，归 NO / TF owner）。
6. **判的次序是令牌、授予、设备、准入范围**：没有授予的人看不出设备登没登，设备不在用的请求看不出租户登没登区间；沿 14 判断项 3 的取法。越权风险点 · 待 owner 复核。
7. **设备册读不动单列 `ErrDeviceRegistryUnavailable`**，不折进设备那一格；沿 03 完成判据「册读失败答依赖故障」。越权风险点 · 待 owner 复核。
8. **请求不带作业范围或设备标识答普通错误、不进任何一格**：算接入口漏传，答哪一格都会把人指去错的恢复动作。越权风险点 · 待 owner 复核。
9. **`WorkFactEnvelope` 在「操作者主体 + 设备」之外还带租户与作业范围**，来源固定为 `FRONTLINE_CLIENT`。越权风险点 · 待 owner 复核。
10. 有出处、不另标的：「作业事实登记」授予不进管理台信封（`MintOperator` 跳过它，`HoldsAt` 整格答否），依 ADR-0149 决定二「缺一不铸」；设备与停用都带非空依据、两个子命令挂在 `parcel-access-register` 上，沿 01 判断项；批文未知字段拒收、型号与序列号不进册，依 ADR-0149 越权风险点 2。

**封存笔 `382c4efb`**：三份件（`operator.go` 的能力面格、`device.go`、`work_fact.go`）都取了骨架与注释，并改了名（`NewWorkFactGrant` → `NewWorkFactRegistrationGrant`、`Scope()` → `WorkScope()`）、把作业范围的哨兵拆成种类未知与缺引用两格、收紧了 `valid()`；铸造、迁移、适配器、CLI、测试与登记册一行封存笔里没有，是本分支新写。

**验证**（作者那层，parallel-sessions「全量只跑一次」；钉 `d0361b21`；WSL，go1.26.8，DSN `postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`）

- 全仓 `go build ./...` 退 0、`go vet ./...` 退 0。
- 本分支动过的 `.go` 经 `gofmt -l` 无输出；0004 迁移入库字节无 CR、无 BOM。
- `go test -count=1 -p 1 -v` 带 DSN，范围是动过的 `internal/accessidentity`、`internal/accessidentity/adapters/postgres`、`cmd/parcel-access-register`、`migrations` 四包，加 `go list -test` 反查的反向依赖与 `./internal/architecture/...`，共 57 包（含 13 个 `cmd/*`）：退 0；56 包 ok、1 包无测试文件；`--- FAIL` 0，`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，助手进程用例，照设计跳过）。**绿（含 PG）**。
- 单跑真库用例 `TestDeviceDeactivationEndsUseFromItsInstant`（`-v`）：PASS，非 SKIP。
- 清点生成器在 `d0361b21` 的干净导出上重跑，与已提交的无差。

**未做**

- 做什么第 4 条（交接、揽收、POD 三个更正口与首登口同一份认证与授予）与完成判据第二条（换口同笔撤隔离放行）：判据第二条所列各口今天经写开关放行，换口要同笔撤放行（ADR-0150 决定三），前提与 04 第 1 件同一件——演示环境接上发行方与合成操作者授予，等 07、17（见 Blocked by）；更正三口随首登口（ADR-0151 决定四）。这一半落在 `cmd/parcel-api` 与各上下文的 Intake，本轮没碰。

**未验**

- 铸造与真库设备册之间没有贯通用例：铸造用例用替身，真库用例止于 `FindDevice`；两段的装配随换口那一半。
- 请求不指名租户时取绑定那条路径，作业事实一侧没有用例（操作者族有 `TestUnnamedTenantIsTakenFromTheOperatorsBinding`）；它不在判据第一条之内，本轮没补。
- `-race` 没跑。

**评审**：非作者评审由通道 1 另派；作者一侧（通道 1 前一会话、通道 3）不自评。
