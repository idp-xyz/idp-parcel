# CC 没有 inbox：SA 发出的资金事实信封到不了 `ReceiveFundsFact`，税费付款核对只能靠测试喂事实

Category: enhancement
Status: resolved——2026-09-10 22:1x 通道 5（task-764b20a1，分支 `mcp5-sacc03` 基远端 main `b40b1804`）：SA 半边（付款人维取 A、可缺席 + 按键只读视图）`84da8df5`、CC 半边（inbox 消费者 + 消费侧适配器 + dispatch 路由 + CONTEXT-MAP 边）`f96169d2`、清点 `2db0afff`；完成判据 1–3 全部落地，验证与判断题见「完成记录」；实施中新出的一格要裁（付款人在 SA 没有来源）由通道 1 代裁，见「裁决」2 与「越权风险点」；CC 放宽另立 [12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md)（draft）。进 main 的 SHA 由推送方重放后另记。此前 in-progress——2026-09-10 20:5x 通道 5 认领（task-764b20a1；分支 `mcp5-sacc03` 基远端 main `b40b1804`，隔离树 D:/tops/idp-parcel-mcp5-sacc03）。此前 ready-for-agent——2026-09-10 17:3x 通道 5 按通道 1 派单 task-9a2ff746（用户授权代裁）写入裁决：SA 读口按键取（租户 + 引用 + 版本）、走只读口不读写侧也不读目录列表（见「要裁的」下「裁决」），本票再无待裁问题；开工仍等 02 落地。此前 draft——2026-09-10 通道 4 立票（task-9880bbc9），只写票面未动代码；取证锚 `3f485e97`
Blocked by: 无——[02](02-sa-external-funds-fact-adoption-hands-off-an-envelope.md) 已于 2026-09-10 19:4x 进 main（信封类型 `settlement-accounting.external-funds-fact.adopted`，ID `<租户>/funds-fact/<事实>/<版本>`，分区键 `<租户>/funds-fact/<事实>`，载荷只带引用与可缺席的 `corrects`；形状见 `internal/settlementaccounting/adapters/postgres/external_funds_fact_handoff.go`）。此前 Blocked by 02（没有那封信封，消费者无物可收）

## 缺口（取证于 `3f485e97`）

- `git ls-files internal/customscompliance/adapters/inbox` → 空；CC 今天没有任何 inbox 消费者。
- `internal/customscompliance/application/reconcile_duty_payment.go` 的 `ReceiveFundsFact`（步 6 的 CC 半边）在，命令 `ReceiveExternalFundsFactCommand{Registration ports.ExternalFundsFactRegistration}`；`git grep -n ReceiveFundsFact -- cmd/` 零。
- `cmd/parcel-dispatch/assemble.go` 路由表无 CC 消费者（`git grep -n -i 'customscompliance' -- cmd/parcel-dispatch/assemble.go`——开工前核）。

## 语言从哪里来

- CC `CONTEXT.md`：「外部资金事实只有在能够按真实程序和适用范围关联到当前监管核定税费时，才能参与税费付款核对。金额相同、同一包裹、同一客户或同一时间不能单独证明付款覆盖」；「监管核定税费、实际付款……税费付款核对……必须分别保存并由各自责任方拥有」。
- mech/07 CC-c：「`ReceiveFundsFact` 入向命令 + `external_funds_fact` 登记册（按事实引用幂等；『待关联』派生——没有核对引用它的事实就是待关联，无状态推进写口）」。

## 做法

1. `internal/customscompliance/adapters/inbox/external_funds_fact_consumer.go`：收 `settlement-accounting.external-funds-fact.adopted`，按引用回查 SA 读口取事实内容（付款人 / 金额 / 币种 / 业务时间 / 来源身份），译成 `ports.ExternalFundsFactRegistration` 交 `ReceiveFundsFact`。
2. 读 SA 用**消费侧适配器** `internal/customscompliance/adapters/settlementaccounting/`（新，CONTEXT-MAP 加 SA→CC 一条边）；`application` 不得 import `settlementaccounting`。
3. 同引用重放 → `已存在`；同引用换内容 → `内容冲突`（编排已有，消费者只译不判）。
4. `cmd/parcel-dispatch/assemble.go` 路由表加一行（共享接线文件，动前占号）。

## 红线

- 消费者不关联、不核对：关联依据与三轴由 `VerifyPayment` 的调用方交，不在消费者里猜（「金额相同……不能单独证明付款覆盖」）。
- 不复制金额进 CC 以外的第二处权威：CC 登记册存的是引用 + 核对所需维度，来源仍是 SA。

## 完成判据

1. `git grep -w NewReceiveFundsFact -- cmd/` 或等价装配符号有非测试调用点。
2. 应用层：一封 → 一条登记；重投 → `已存在`；换内容 → `内容冲突`。
3. 真库装配用例一正一反；CONTEXT-MAP 与机制清点（消费缝 CC→SA +1）同笔。

## 地盘

`internal/customscompliance/adapters/inbox/`（新）、`internal/customscompliance/adapters/settlementaccounting/`（新）、`cmd/parcel-dispatch/assemble.go` 一行、`docs/domain/CONTEXT-MAP.md` 一条边。

## 要裁的

1. **SA 读口用哪个**：`ports.ExternalFundsFactStore` 是 SA 内部写侧登记面，消费侧适配器读它还是读 `catalogue_read.go` 的 `ExternalFundsFactCatalogueRow` 目录读口——前者是按键取一条，后者是列表。归 SA owner 一句（CC 是消费方，形照 PS 读 PC 的消费侧适配器）。

### 裁决

（通道 1 推送方裁、通道 5 写入，2026-09-10 17:0x；task-b941ce87 分类 A、task-9a2ff746 落笔。A 类不落 ADR。）

- **1 → 按键取（租户 + 资金事实引用 + 采用版本），走 SA 的只读口；既不读写侧 `ExternalFundsFactStore`，也不读目录列表。** SA 今天若没有按键取一条的只读半边，就在 SA `ports` 补一个只读视图（形照 PS 的 `LabelTransactionsByParcelView` 那种只读视图：一口一问、不拓宽写口），CC 消费侧适配器 `adapters/settlementaccounting/` 实现 CC 自己的端口去调它。**取信封所指的那一版，不取 latest**（票 lc/24 的教训：信封先后与版本先后不同源，按 latest 读会把后到的更正当成原事实）。补只读口这一步属 SA 地盘，动前占号；单一权威仍在 SA，CC 登记册只存引用 + 核对所需维度（票面红线不变）。

2. **付款人维（实施中新出，票面未预见）**：CC 的 `ports.ExternalFundsFactRegistration.Payer` 在 SA 没有来源——SA `domain.ExternalFundsFact` / `external_funds_fact` 表 / `AdoptFundsFactCommand` 均无付款人，而 CC `ReceiveFundsFact` 对空付款人答 `未受理`、CC 表 `payer_ref NOT NULL`。三条路：A. SA 加付款人维；B. CC 放宽付款人可缺席；C. 机制先落、付款人显式未决。

  **裁决（通道 1，推送方代裁；2026-09-10 20:1x；B 类、用户授权代裁，越权风险点写进本票；通道 5 落笔）**：**取 A，但付款人在 SA 上可缺席（缺席显式）**。口径：① CC CONTEXT「税费付款核对」原句把付款人列为「**来源提供或真实程序要求的**」维度——它是外部事实自带的属性，不是 CC 自己算的；② ADR-0137 决定四：资金事实进产品只有 SA 采用这一口，CC 不另设铸造 / 补录——所以来源提供了付款人而 SA 不登，CC 就永远拿不到，单一铸造口必须保留来源给的每一维；③ 正因为是「来源提供」，来源没给时 SA 不能拒绝采用，所以 **SA 侧 `Payer` 可缺席**（领域上显式「来源未提供」，不是空串默认；列 nullable），SA 自己不对它下任何判断（SA CONTEXT「它不修改外部资金事实」；「付款方身份」是 SA 另一个概念——代垫判断的输入，不混名，用「付款人」一词且头注写明与「付款方身份」不同）。做：SA `ExternalFundsFactSpec.Payer`（可缺席的付款人引用，不接结构化账户——不写任何真实银行字段）+ `AdoptFundsFactCommand.Payer` + 新迁移 `payer_ref text NULL`（SA 序号重取；存量零）+ 只读口带出；更正版本同型；SA CONTEXT 外部资金事实那句补「含来源提供的付款人」半句，引 CC CONTEXT 那句作理由。**CC 侧不动**：`ReceiveFundsFact` 仍要付款人非空；信封所指事实无付款人 → 消费者如实交空 → CC 答未受理，是诚实停点不是 bug；消费门对未受理的归法照 inboxconsume 既有约定与交付消费者先例（入账，见完成记录判断题 ③）。**CC 放宽（B）另立 sa-cc 新票 draft**：[12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md)。

### 越权风险点（供 owner 复核）

- (a) **SA 外部资金事实要不要携带来源提供的付款人维**——SA owner。裁决按「唯一采用口必须保留来源给的每一维」读；若 SA owner 认为付款人不该进 SA 的事实（例如归 CC 直接向来源取），SA `0018` 那一列与 `FundsPayerReference` 要回退，CC 消费侧改走别的来处。
- (b) **付款人在 SA 可缺席而 CC 必填，两侧不对称是有意的**——CC owner 复核 [12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md) 时连看：来源未提供且真实程序不要求的付款人，CC 该「明确记录」还是继续拒收。

## 参照

[02](02-sa-external-funds-fact-adoption-hands-off-an-envelope.md)；[mech/07](../../mechanism-executor-triage/issues/07-cc-four-executors-behind-existing-uc-steps.md) CC-c；`internal/parcelshipment/adapters/inbox/effective_delivery_consumer.go`（跨上下文 inbox 消费者 + 消费侧读口先例）；ps-port-remainder/05（CC 读 PS 的消费侧适配器先例，`13f3ba65`）。

## 完成记录

分支 `mcp5-sacc03`，基远端 main `b40b1804`（开工时 fetch + ls-remote 核）。下表 SHA 为分支上的，作封存出处；进 main 的 SHA 由推送方重放后并列补记。共享接线文件：`cmd/parcel-dispatch/assemble.go` 路由表只加自己一行（另加一个装配函数与一份哨兵名单，各成块）、`assemble_test.go` 一条用例 + 两组 import；`docs/domain/CONTEXT-MAP.md` 只加一行边。

| 分支 SHA | 内容 |
|---|---|
| `0ecb72c3` | docs(scratch)：Status → in-progress |
| `84da8df5` | feat(settlementaccounting)：裁决 2（A、可缺席）——`FundsPayerReference` / `ExternalFundsFactSpec.Payer` / `Payer() (ref, bool)` / 更正同型 / `RehydrateExternalFundsFactSpec.Payer`；`AdoptFundsFactCommand.Payer` 进内容摘要（同引用换付款人是冲突）；迁移 `settlement_accounting/0018` `payer_ref text NULL` + 拒空白 CHECK；postgres Save / FindByKey 带列（读回扫描抽成 `scanExternalFundsFact`，写侧与视图共用）；**新只读视图 `AdoptedFundsFactView.LoadAdoptedFundsFact(tenant, fact, version)`**（裁决 1：版本进 WHERE，取信封所指那一版；不拓宽 `ExternalFundsFactStore`）；ports `AdoptedFundsFactView`；SA CONTEXT Boundaries 补半句 |
| `f96169d2` | feat(customscompliance)：`adapters/inbox`（新包 ccinbox）`ExternalFundsFactConsumer`——消费者名 `customs-compliance/receive-external-funds-fact`、只译三维、corrects 不进译码；CC ports `AdoptedFundsFactSource` / `AdoptedFundsFact`；`adapters/settlementaccounting`（新包）`SettlementAdoptedFundsFactSource`（调 SA 只读视图、译成本上下文几维）+ `ReceiveOnAdoptedFundsFactAdapter`（回查 → 译 → `ReceiveFundsFact` → 消费两格）；`cmd/parcel-dispatch` `receiveExternalFundsFactConsumer` + `externalFundsFactUndecidedSentinels` + 路由一行 + 真库装配用例一正一反；CONTEXT-MAP `settlement-accounting → customs-compliance` 一边 |
| `2db0afff` | docs(inventory)：清点在 `f96169d2` 干净检出重生成（CC 生产 75→78、SA 83、消费缝 22→23（CC→SA 2 文件）、SA 迁移 17→18、消费适配器 27→28、路由表 18→19、端口 382→384）——只对该检出成立，推送方在 tip 兑底 |
| （本笔） | docs(scratch)：本票 Status → resolved + 裁决 2 + 越权风险点 + 本完成记录；新立 [12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md)（draft） |

**逐条对完成判据**：1 `git grep -w NewExternalFundsFactConsumer -- cmd/` 非测试调用点在 `cmd/parcel-dispatch/assemble.go` 的 `receiveExternalFundsFactConsumer`（`NewReceiveFundsFact` 这个名字仓里不存在，等价装配符号是 CC 的 `NewDutyPaymentReconciliationHandler` + 消费者构造，两者都在该函数里）。2 应用层——`TestAnAdoptedFundsFactIsRegisteredOnceAndReplayOrConflictStillSettles`：一封 → 一条登记（来源 / 付款人 / 币种 / 金额 / 业务时间照 SA 那一版转述）；重投 → 编排答 `已存在`、登记条数仍 1；同引用换内容 → 编排答 `内容冲突`、已登记内容不被顶替；三者消费者都入账不重投。3 真库装配一正一反——`TestAnAdoptedExternalFundsFactReachesTheCustomsRegisterThroughTheRouteTable`（生产依赖图）：正：SA 采用过的事实（带付款人）经信封落 CC `external_funds_fact` 一行且各维同值；反：SA 还没有的版本 → `dispatch.consumer_undecided`、不定稿不毒丸。CONTEXT-MAP 边与清点各成笔。

**做法逐条**：1 消费者只译引用 ✓；2 读 SA 走消费侧适配器 `internal/customscompliance/adapters/settlementaccounting/`（`internal/<consumer>/adapters/<provider>/` 位置，边界门禁绿），`application` 不 import SA ✓；3 重放 / 冲突由编排答、消费者只译 ✓；4 路由表一行 ✓。红线：消费者不关联不核对（`VerifyPayment` 的调用方不在本进程）；CC 登记只存引用 + 核对所需维度；无真实渠道 / 银行实例（夹具值 `source-bank-feed-1` / `payer-customer-7` / `bank-fact-1` 均合成）。

**验证**（隔离树 `D:/tops/idp-parcel-mcp5-sacc03`，带 DSN `-p 1 -count=1`，21:5x）：CC 九包 + SA 七包 + `cmd/*` 十一包 + `internal/architecture` + 反向依赖（PS→SA、VE→CC 适配器）+ `migrations` + `platform/migrate` 共 32 包全 ok，47 s；CC inbox / CC adapters-settlementaccounting / `cmd/parcel-dispatch` / SA adapters/postgres 四包 `-v` **PASS 242 / SKIP 0 / FAIL 0**，票内用例逐条 PASS（SA 切片单独那一跑 PASS 134 / SKIP 0）。干净 detached 检出 `%TEMP%\idp-verify-sacc03` @ `f96169d2`：`gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0、清点生成器跑出上表差异。反向依赖由 `go list` 反查 SA/CC ports 得出；`cmd/*` 全带 DSN。不跑全量；`-race` 本机无 cgo 未跑。

**/code-review 自评**（基线 `b40b1804`，作者串行两轴——非作者评审由推送方另派，本段不顶替）：Standards 0 阻断 / 2 非阻断——① `assemble_test.go` 新增 `saTestValue` 泛型助手与仓内多处同形助手（`saValue` / `pcValue` / `value`）重复（Duplicated Code，判断题 ⑥）；② `receive_on_adopted_funds_fact_test.go` 的 `fundsFactRegisterDouble` 与 CC application 测试里的替身同形（测试替身不跨包共用，接受）。Spec 0 阻断 / 1 非阻断——完成判据 1 字面 `NewReceiveFundsFact` 仓里无此符号，以等价装配符号作答（见上）。

**判断题**（供非作者评审 / owner 拍）：
① SA 只读视图签名带 `version` 进 WHERE 而不是读回再比：库里将来一事实多行时照样只答被问的那一版；今天一事实一行、问 v2 得 false 也正是「不拿 latest 顶替」的形。
② CC 读端口 `AdoptedFundsFact` 只带入向登记要读的五维（来源 / 付款人 / 币种 / 金额 / 业务时间），不带 Kind / Version / Corrects：登记今天没有那几格，带了就是预留（Speculative Generality）；要用时再加。
③ 编排答 `未受理`（含 SA 那一版无付款人）消费者**入账不重投**：照 `finalconsume.Consumption` 对 `REQUEST_NOT_ACCEPTED` 的处置——重投同样内容不会长出付款人，落毒丸账又把「本上下文的诚实停点」记成「信封坏了」。代价是那封信封定稿后没有人再看它；CC 放宽（[12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md)）落地前，缺付款人的事实在 CC 侧只留 inbox 已处理的痕迹。若评审认为该进未决名单重投到上限，改一行哨兵即可。
④ `ErrAdoptedFactNotVisible` 同时盖住「读口出错」与「那一版不在」：两者重投都会改变结果，与 pstf `ErrDeliveryNotVisible` 同形；「键与本体不符」那一格这里没有——视图按键取回的就是键所指，没有第二个对象可指错。
⑤ 消费者不读 `corrects`：更正回指是事实本体的一维，处理方按版本读事实时自会看到；消费者转述它就是第二处口径。
⑥ `assemble_test.go` 里的 `saTestValue` 助手：`main` 包测试没有现成的值对象助手，抬一个共享助手到别的测试文件会碰邻行，故就地一个。
⑦ 装配函数把 `DutyPaymentReconciliation` 三口都接给编排 Deps（`Collaborations` / `Funds` / `Verifications`）虽然这条线只用 `Funds`：同一只 postgres 适配器实现三口，接全比接一半再留两个 nil 更诚实——nil 那两口在别处被调用会 panic，而编排的 Deps 今天没有 nil 校验。

## Comments

- 2026-09-10 · 通道 4：立票。未动代码。
- 2026-09-10 · 通道 5（task-764b20a1）：实施中发现付款人在 SA 无来源 → 报通道 1 → 裁决 2（A、可缺席）；SA 半边 `84da8df5`、CC 半边 `f96169d2`、清点 `2db0afff`，票面转 resolved；越权风险点 (a)(b) 与新 draft 票 12 见上。分支已推 origin；等推送方派非作者评审后重放。
