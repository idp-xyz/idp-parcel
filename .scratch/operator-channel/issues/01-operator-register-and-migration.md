# 01 操作者册：领域、迁移首个模块、受控登记口与参数登记册一行

Category: enhancement
Status: in-progress——2026-09-24 通道 2 认领（通道 1 派单 task-3d612285，原卡续派）：隔离 worktree `idp-parcel-mcp2-oc01`、分支 `mcp2-oc01`，基 `443a472e`；新迁移模块 `access_identity` `0001`，受控 CLI 落新进程 `cmd/parcel-access-register`，参数登记册行号取 `PAR-INT-08`。同日交活（代码 tip `f2d7dbfd`，含清点 `09f90e5f`），待非作者评审与重放。此前：ready-for-agent——2026-09-24 拆法经用户授权通道 4 自决认可（「参考专业头部软件的做法，你来帮我自决吧」）；ADR-0149 另让本册多一格能力面「作业事实登记」，那一格归 10
Blocked by: 无
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md)「操作者渠道落地」甲轨第一步
地盘：`internal/accessidentity`（操作者册的领域、端口与 postgres 适配器）、`migrations/access_identity/` 首个模块（共享接线文件 `migrations/migrations.go` 与计划装配按 parallel-sessions「占号、同笔、逐块核」办）、受控登记 CLI 一个子命令、参数登记册增一行。
出处：[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定二第三条、决定六与 Consequences。

## 做什么

1. 操作者册：操作者主体（发行方 + `sub`）绑定唯一租户；授予按能力面显式登记（登记册配置写、主数据与运营查阅读；治理登记那一格只预留，按 ADR-0085 决定四另裁），可撤销、带生效区间；不存在跨租户主体。列由 ADR-0100 定死，行是租户取值。
2. 迁移 `access_identity` 首个模块：结构与 CHECK 守上面的不变量，不种任何行。
3. 受控 CLI 登记口（登记主体、授予、撤销），定位是 ADR-0085 决定一的「受控批量口」；演示租户的合成操作者经它登记，只记 `S`。
4. 参数登记册增「运营操作者账户与授予」一行（租户取值：某租户的操作者主体、绑定与授予），写法照登记册既有行，`PAR-INT-01` 不动。

## 不做

- 凭据校验与信封铸造（02、03）；任何端点换线。

## 完成判据

- 真库用例：登记、重放、撤销、区间外不生效、跨租户主体拒收；CLI 端到端一条；干净检出迁移计划施加通过；登记册那一行已增。

## 完成记录（2026-09-24，通道 2，分支 `mcp2-oc01`）

**基线**：分支基 `443a472e`（续派时改定的 origin/main）。认领笔 `e1ff562f` 只动票面 Status 一行；其后六笔如下表，代码 tip `f2d7dbfd`；本记录一笔在其后。

**落点**

| 笔 | 做了什么 |
|---|---|
| `8aaedb7b` | 迁移 `access_identity/0001` 三表与共享接线同笔（`migrations.go` 嵌入行与 `AccessIdentity()`；`plan.go` 的 `SchemaAccessIdentity`、Plan 末尾追加、Schemas 名单）；`internal/accessidentity/operator.go` 领域类型与装载口、登记口；`adapters/postgres` 适配器与真库用例；`doc.go` 一个从句 |
| `f11c6f79` | `cmd/parcel-access-register` 三个子命令；演示种子一节与 README 组成表一行 |
| `eab7daf1` | 参数登记册 `PAR-INT-08` 一行 |
| `6bface24` | 双轴自审修复（见下） |
| `09f90e5f` | 机制清点重生成（在 `6bface24` 的干净检出上） |
| `f2d7dbfd` | 三个写口补 PBC-08 无事务负向证据（作者全量撞出，见「门」） |

**形状要点**（供 accessidentity owner 与 03、10、psb/07 读）

- 册三表只增：`operator` 键为（发行方, sub），租户是列；`operator_grant` 键为（租户, 授予标识），经（发行方, sub, 租户）外键回指绑定；`operator_grant_revocation` 键同授予，一笔授予至多撤一次，撤销不 UPDATE 授予。迁移不种行。
- 能力面：`REGISTRY_CONFIGURATION_WRITE`、`MASTER_DATA_AND_OPERATIONS_READ` 可授；`GOVERNANCE_REGISTRATION` 只预留——领域答 `ErrCapabilityFaceReserved`（与未知格分开），库 CHECK 不收。10 的「作业事实登记」一格要新迁移放开 CHECK、加作业范围列，并同笔改 `CapabilityFace`。
- 区间含起点、不含终点，终点缺席即不设终点；撤销自撤销时刻起（含）不生效，早于起点即从未生效。自然到期不存成状态。
- 答复代数：`RECORDED` / `ALREADY_REGISTERED` / `CONTENT_CONFLICT`，另三格各只由一个登记动作答——`SUBJECT_BOUND_TO_ANOTHER_TENANT`（登记主体）、`OPERATOR_NOT_REGISTERED`（授予；主体绑在别的租户上也答这一格）、`GRANT_NOT_REGISTERED`（撤销）。撞键不覆盖，读回比对时点按库的微秒精度。
- 读口 `OperatorRegistry.FindOperator(subject)` 交 `OperatorStanding`（绑定与名下全部授予连同撤销），`HoldsAt(face, at)` 对时点判；不比对请求所在的租户——那是 03 铸造时的事。

**完成判据**（钉 `f2d7dbfd`；Linux（WSL2），go1.26.8，真库为 55432 门禁库，`-v` 下 PASS 非 SKIP）

- ✅ 登记：`TestOperatorRegistrationRecordsReplaysAndNeverOverwrites`——首登 `RECORDED`；不在册的主体答 found=false 而不是 error。
- ✅ 重放：主体重放答 `ALREADY_REGISTERED`、同主体同租户异依据答 `CONTENT_CONFLICT` 且原行不动（同上用例）；授予重放与同标识异区间在 `TestGrantIsEffectiveOnlyInsideItsInterval`（起点带纳秒，重放照样认得出）；撤销重放与异时刻在 `TestRevocationEndsTheGrantFromItsInstantAndKeepsHistory`。
- ✅ 撤销：`TestRevocationEndsTheGrantFromItsInstantAndKeepsHistory`——撤销时刻起不生效、之前照常；撤本租户册上没有的授予、别的租户拿同一标识撤，都答 `GRANT_NOT_REGISTERED`；撤了再授是另一笔，前一笔连同撤销留册。
- ✅ 区间外不生效：`TestGrantIsEffectiveOnlyInsideItsInterval`——起点前一刻与终点不生效，起点与终点前一刻生效，另一格不顶替。
- ✅ 跨租户主体拒收：`TestSubjectBoundToOneTenantIsRefusedByEveryOtherTenant`——乙租户登甲的主体答 `SUBJECT_BOUND_TO_ANOTHER_TENANT`，授甲的主体答 `OPERATOR_NOT_REGISTERED`；别的发行方下同名 sub 是另一主体，照常登；绕过登记口直接写，主体第二行撞唯一键、跨租户授予撞外键。
- ✅ 结构自守：`TestOperatorRegisterTablesGuardTheInvariantsThemselves`——缺件、预留格与未知格、终点不晚于起点、撤不存在的授予、一笔撤两次，直接写都进不了库；`TestOperatorRegistrationWritesRefuseToRunOutsideATransaction`——三个写口无事务答 `ErrTransactionRequired`。
- ✅ CLI 端到端一条：`TestOperatorRegisterGrantRevokeEndToEnd`——三个子命令落定、整批重放、撤销生效、区间外不生效；乙租户登甲的主体、授未登记主体、撤不存在的授予、同标识异区间各退 2 且回显答复名，册上不变；结果经装载口读回核对。另有触库前拒收 `TestBatchTranslationRefusesBeforeTouchingTheDatabase`、用法错误 `TestUsageMistakesAreTechnicalFailures`。
- ✅ 干净检出迁移计划施加：`internal/platform/migrate` 四条真库用例（首次施加加 CheckSchema、历史记工件身份、重跑不重施、校验和漂移拦截）在本分支检出上带 DSN PASS；`migrations` 的嵌入资产行尾守卫 PASS（`0001` 无 CR、无 BOM）。
- ✅ 登记册那一行：`PAR-INT-08`「运营操作者账户与授予」（`eab7daf1`），当前登记「待提供」，`PAR-INT-01` 与邻行不动。
- ✅ 演示租户的合成操作者只记 `S`：`seed.sh` 在一次性库 `oc01_seed_check` 上整跑退 0，操作者册一节单独重放答 `ALREADY_REGISTERED` 退 0，库已删。

**门**（作者自验，按 parallel-sessions「全量只跑一次」）

- 全仓 `gofmt -l .` 无输出，`go vet ./...`、`go build ./...` 退 0；`tools/mechanism-inventory` 自测 ok，清点在 tip 上重跑无差异。
- 全量一次（钉 `09f90e5f`，带 DSN，`go test -count=1 -p 1 -v ./...`）：有测试的包 119 个 ok、`internal/architecture` 红一个用例——PBC-08 门 `TestEveryPersistenceWriteMethodCarriesTransactionRequiredEvidence` 点名本票三个写口缺 `ErrTransactionRequired` 负向证据（原用例只调一个写口、只判 err 非空）；`--- FAIL` 1、`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，助手进程用例，不作为助手启动时照设计跳过）。
- 修复 `f2d7dbfd` 只动本票适配器的测试文件；受影响的 `./internal/accessidentity/...` 与 `./internal/architecture/...` 带 DSN、`-p 1 -count=1 -v` 重跑全 ok，无 FAIL、无 SKIP。其余包未受影响，不跑第二遍全量。
- 本票三个包与 `migrations`、`internal/platform/migrate` 另带 `-race`（带 DSN）ok。

**双轴自审**：`/code-review` 两轴在主会话串行做（本仓不用子代理），固定点 `443a472e`。Standards 修两条（`6bface24`）：能力面入口闸在解析与构造授予两处各写一遍 → 收成 `CapabilityFace.checkGrantable`；`doc.go` 从句写成「里面只有操作者册」，会随同模块新增的登记册（票 10）无声变假 → 改成不依赖该目录全部内容的说法。Spec 无阻断，判断项见下。修后同一基线复审两轴无新发现。

**判断项**（交评审与推送方）

1. 受控 CLI 落新进程 `cmd/parcel-access-register`（`operator-register` / `operator-grant` / `operator-revoke`），不挂进既有某个 CLI：先例是一模块一个 `parcel-*-register`，派单授权「放哪个 cmd 由你按既有登记口先例定」；地盘行「一个子命令」按做什么第 3 条「登记主体、授予、撤销」读成三个动作。10 的设备册、11 的集成客户端册可续挂在这个进程上。
2. 绑定不可撤销、不改指：ADR-0100 只给授予区间与撤销。操作者离开即撤光授予，绑定作为「这个主体属于哪个租户」的事实留册；绑错租户的补救是在发行方另开一个主体。owner 若要绑定也带生命周期，是一次加列迁移，不在本票。
3. 绑定、授予、撤销都带依据（`basis_ref`，非空）：ADR-0100 没点名，理由是册上每一行都要答得出凭什么登的，撤销「时刻与依据缺一不成立」同 ADR-0093 的判据；登记册该行的最低证据随之列了开户、授予与撤销依据。
4. `SUBJECT_BOUND_TO_ANOTHER_TENANT` 不说绑在哪个租户，但答复本身透露「这个主体在别处有绑定」——任何拒收都会透露存在。受控批量口背后是产品侧的人，可接受；将来租户自助的在线口若要更严，在那一侧折格即可，登记口的代数不必改。
5. 时点精度：库 `timestamptz` 到微秒，重放比对按微秒截断；带纳秒的起点或撤销时刻落库后早几百纳秒生效，不改变任何判定。
6. 演示种子的发行方是合成值 `https://syn-issuer-01.example.invalid`：02 定下演示用的真 OIDC 发行方之后，要按那个发行方的标识另登一批（主体不能改指），`seed.sh` 那一节注释已写。

**地盘外、未动**

- 开发主线「按四项判据重定级」表「横切」行写「`internal/accessidentity` 没有操作者册、OIDC 校验与 `OperatorEnvelope`」：本票进 main 后「没有操作者册」一句不再成立（该格「未满足」的结论照旧成立，OIDC 与信封归 02、03），归开发主线维护方改。
- `doc.go`「本轮既没有登记册的表，也没有凭据形态」整段改写归 03；本票只改了会因本票而变假的那一个从句。
- 共享接线文件（`migrations/migrations.go`、`internal/platform/migrate/plan.go`）的占号保持到本票重放进 main；往既有模块加迁移不碰这两个文件。
