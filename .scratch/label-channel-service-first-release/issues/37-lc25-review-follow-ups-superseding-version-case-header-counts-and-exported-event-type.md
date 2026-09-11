# lc/25 两轴非作者评审的非阻断项一笔收口：替代版本 v2 直接用例、测试文件头计数换点名、`ADR-0049 第三条` 写法统一、UC-PS-004 依据表括注改口、TF 导出收寄事件类型常量供消费方对照

Category: chore
Status: resolved——**完工待非作者评审进 main，2026-09-11 21:4x**（分支 `mcp2-lc37` 基远端 main `02e1dfc4`；五条对应五笔 + 立票一笔，完成记录随条 5 那一笔同提交，见 Comments「完工」；清点零差不另成笔）；此前 in-progress——2026-09-11 21:2x 通道 2 按通道 1 派单 task-c778df52 自立自做，分支 `mcp2-lc37` 基远端 main `02e1dfc4`（树 `D:/tops/idp-parcel-mcp2-lc37`）；要裁的为零
Blocked by: 无（lc/25 已进 main `02e1dfc4`，五条出处全在票 25 Comments）。撞点：`cmd/parcel-dispatch/assemble.go` 通道 5（sa-cc/01）同期加路由行 + 装配函数，本票只改 `judgeLabelFinalOnCarrierFirstEffectivePickupConsumer` 头注一句，不同块，动前占号；`internal/transportfulfillment/adapters/postgres/` 今晚只本票动

## 缺口（出处逐条指到评审原话；取证于 `02e1dfc4`）

[lc/25](25-external-carrier-first-pickup-triggers-label-final-judgment.md) 的两轴非作者评审各留了非阻断（通道 6 Spec 5 条、通道 2 Standards 4 条），处置都是「随票记、另笔」；票 25「进 main 记录」把可做的收成候选后继，本票就是那张。逐条：

1. **通道 6 Spec 非阻断 (1)**——`internal/parcelshipment/adapters/transportfulfillment/judge_on_carrier_first_effective_pickup.go` `HandleRegisteredCarrierFirstEffectivePickup`：替代版本（TF `Supersede` 产物仍是 `CarrierPickupFormed`）走已形成格、以新业务时间判终局，合 ADR-0135 决定六 / 七；但没有用例让信封指向替代代 v2——现有 `TestARegisteredCarrierPickupIsFetchedAtTheEnvelopeVersionAndJudgedAsFinal` 只从 v1 侧证「不取 latest」，三种入队版本里替代格缺直接用例。改法：同文件加一例，信封指 v2，断言按 v2 取回、`EffectiveAt` = v2 的业务发生时间、交采用路径的执行证据与版本都是 v2 的。
2. **通道 2 Standards 非阻断 1**——`judge_on_carrier_first_effective_pickup_test.go` 文件头「三路共用的处理方核」「四个读口」：数的是别处的路数（ADR-0134 定的哪几路）与 `JudgeLabelServiceFinalDeps` 的口数，第四路接上时无声变错（AGENTS.md「计数与行号同构」）。改法：按名指 lc/25 / 26 / 27 与各读口名，不带数。
3. **通道 2 Standards 非阻断 3**——`cmd/parcel-dispatch/assemble.go` `judgeLabelFinalOnCarrierFirstEffectivePickupConsumer` 头注「ADR-0049 第三条」：ADR-0049 Decision 节是编号列表、编号稳定，可接受，但与同文件「ADR-0135 决定七」写法不一。改法：核 ADR-0049 Decision 第 3 项原句「路由表是显式清单，没有订阅者的事件类型显式失败并入账」后写成「ADR-0049 决定三」，与同文件既有写法同形。
4. **通道 6 Spec 非阻断 (4) / ADR-0135 越权风险点 5 / 票 25「随本票或另笔」**——`docs/application/parcel-shipment/UC-PS-004-FORM-PARCEL-FINAL-SERVICE-OUTCOME.md` 依据表「面单渠道服务非取消终局结果」行括注「（外部承运轨迹事实）」自 ADR-0135 起不成立：TF 拥有的是**实际承运商首次有效收寄事实**，外部承运轨迹事实按 TF CONTEXT 本身不构成收寄。改法：只改括注为「（实际承运商首次有效收寄事实，ADR-0135）」，该行其余三格与全表不动。
5. **通道 2 Standards 非阻断 2**——`internal/parcelshipment/adapters/inbox/carrier_first_effective_pickup_consumer_test.go` `TestTheCarrierPickupConsumerAcceptsTheTypeTheWriterEmits` 钉字面串而非 lc/27 那样与提供方导出常量相等：TF `internal/transportfulfillment/adapters/postgres/carrier_first_effective_pickup_handoff.go` 的 `carrierFirstEffectivePickupEventType` 未导出，TF 改常量并改自家测试时 PS 侧测试不红，头注「两串各改一边这里就红」言过。改法：TF 常量导出为 `CarrierFirstEffectivePickupRegisteredEventType`（直接改名 + 改自家引用，不留旧名别名），对照断言放在已同时装配两边的 `cmd/parcel-dispatch/assemble_test.go`（`TestBusinessModulesDoNotReachIntoEachOther` 的 `loadSources` 排掉 `_test.go`，形式上不拦 PS `adapters/inbox` 测试 import TF postgres，但 PS inbox 包不为一条断言去 import 另一个上下文的持久化适配器；lc/27 的对照在 PS 自家 `pspostgres`，是同上下文才能就地比），`TestTheCarrierPickupConsumerAcceptsTheTypeTheWriterEmits` 头注随之写实。

**不在本票**：`CarrierTrackingFactReference` 改名（归 PS owner，票 25「随本票改口」只改了头注）；票 25「要裁的」2 失效版本重派生（停 `ErrVoidedCarrierPickupRederivationUndecided` 等 owner 裁）；`JudgeOnCarrierFirstEffectivePickupAdapter` 与 `AdoptOnEffectiveDeliveryAdapter` 同形三段抽 helper（通道 2 Standards 非阻断 4，只记不抽）；通道 6 Spec (2)(3)(5) 已由 `6e10cd8f` 完成记录落下。

## 做法

按上面 1–5 逐条改，每条一笔，提交信按条号点名。条 1 是唯一新增用例，按 `/tdd` 单独成笔——它钉的是既有行为，首跑应绿；「红」由本机临时反向改动适配器（不提交）证用例有牙，结果写进完成记录。条 2 / 3 是 `.go` 注释行；条 4 是 `.md` 一处括注；条 5 是常量改名 + 两处自家引用 + `assemble_test.go` 加一条对照 + PS 测试头注一句。

## 红线

- 除条 1 新增用例外零行为：生产 `.go` 只许注释行与条 5 的常量改名；`HandleRegisteredCarrierFirstEffectivePickup` 分支与结果代数一字不动。
- 不动 `domain/**`、路由表行、任何 CC / SA / VE 文件。
- 注释中文、引文单行可搜、不写行号 / 计数。

## 完成判据

1. `git grep -n -E 'CONTEXT 规则|三路|四个读口' -- internal/parcelshipment/adapters/transportfulfillment` 零。
2. `git grep -n carrierFirstEffectivePickupEventType -- internal/` 零；`judgeLabelFinalOnCarrierFirstEffectivePickupConsumer` 头注内 `ADR-0049 第三条` 零（立票时写的「`-- cmd/` 全零」实施时核出不成立：`assemble.go` 其余路与 `assemble_test.go` 里「ADR-0049 第三条」是 lc/25 之前既有写法、且落在通道 5 同期动的块附近，不在本票地盘，见 Comments 判断项 ①）。
3. 条 1 用例：信封指 v2 → 取回键 = v2、`FirstEffectivePickup.EffectiveAt` = v2 业务发生时间、采用命令执行证据 `CFEP-1@CFEV-2`；`-v` PASS。
4. 对照断言在字面改一边时会红——本机把 TF 常量或 PS 常量任一侧临时改一字、跑 `cmd/parcel-dispatch` 该用例见红后还原，写进完成记录。
5. UC-PS-004 该行只差括注一处（`git diff --stat` 该文件 1 行改）。
6. `gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；`go test -count=1` PS `adapters/transportfulfillment` + PS `adapters/inbox`（带 DSN，占 55432 前后广播）+ TF `adapters/postgres`（带 DSN）+ `cmd/parcel-dispatch`（带 DSN）+ `./internal/architecture/...`；清点 tip 重生成核零差（不增删文件、不加端口）。

## 地盘

`internal/parcelshipment/adapters/transportfulfillment/judge_on_carrier_first_effective_pickup_test.go`（文件头一句 + 新增一例）、`cmd/parcel-dispatch/assemble.go` 一句头注（占号）、`cmd/parcel-dispatch/assemble_test.go` 加一条对照用例、`docs/application/parcel-shipment/UC-PS-004-FORM-PARCEL-FINAL-SERVICE-OUTCOME.md` 一处括注、`internal/transportfulfillment/adapters/postgres/carrier_first_effective_pickup_handoff.go` 与同包 `carrier_first_effective_pickup_test.go`（常量改名与自家引用）、`internal/parcelshipment/adapters/inbox/carrier_first_effective_pickup_consumer_test.go` 一句头注。**不动** `domain/**`、路由表、生产逻辑。

## 参照

[lc/25](25-external-carrier-first-pickup-triggers-label-final-judgment.md) Comments「评审 ← 通道 6」非阻断 (1)(4)、「评审 ← 通道 2」非阻断 1 / 2 / 3、「进 main 记录」候选后继；[lc/36](36-ps-label-final-review-follow-ups-header-comments-and-constructor-guards.md)（同族收口票的形）；[lc/27](27-controlled-close-reopen-decision-triggers-label-final-judgment.md)「取舍两处」①（导出只为测试对照）；[ADR-0135](../../../docs/adr/0135-carrier-first-effective-pickup-is-a-judged-control-fact-with-its-own-registry-and-enters-the-segment.md) 越权风险点 5；[ADR-0049](../../../docs/adr/0049-publish-channel-is-in-process-delivery-until-load-evidence.md) Decision；AGENTS.md「写代码注释」。

## Comments

- 2026-09-11 21:2x · 通道 2（task-c778df52）：立票，Status 直接 in-progress，作者自立自做。**只写票面，未动代码。** 五条全部抄自票 25 两份评审原话与「进 main 记录」候选后继；条 5 的门禁读法（PS inbox 测试不 import TF postgres）取自派单原话，实施时以 `TestBusinessModulesDoNotReachIntoEachOther` 实跑为准。
- **2026-09-11 21:2x–21:4x · 通道 2（task-c778df52）· 完工**。分支 `mcp2-lc37` 基远端 main `02e1dfc4`，六笔：`b89540cc` 立票 + spec 行；**条 1** `5cd5c099`（`TestASupersedingCarrierPickupVersionIsJudgedAtItsOwnOccurrenceTime`：信封指 v2 → 取回键 = v2、`FirstEffectivePickup` = {CFEP-1, CFEV-2, 更正后的业务发生时间 09:05}、采用证据 `CFEP-1@CFEV-2` / 版本 CFEV-2 / 生效 09:05；先断言 `Supersede` 产物 `Result() == CarrierPickupFormed`，前提不成立时报的是前提不是格）；**条 2** `aa1a570b`（文件头「三路」→ 点名 lc/26 / lc/27 那两路，「四个读口」→ 逐名列 TF 收寄登记册 / 包裹反查 / 面单交易册 / 继续尝试登记册 / 取消视图）；**条 3** `ca228065`（`judgeLabelFinalOnCarrierFirstEffectivePickupConsumer` 头注「ADR-0049 第三条」→「ADR-0049 决定三」并带原句引文「没有订阅者的事件类型显式失败并入账」，`git grep -c` 于 ADR-0049 单行命中 1）；**条 4** `ed64901a`（UC-PS-004 该行括注 →「（ADR-0135；外部承运轨迹事实本身不构成收寄）」，`--numstat` 1/1）；**条 5** 最后一笔（TF 常量导出 `CarrierFirstEffectivePickupRegisteredEventType` + 头注照 lc/27 `ContinuedAttemptDecisionEventType` 的形、自家唯一引用随之改、TF 真库用例仍钉字面；`cmd/parcel-dispatch/assemble_test.go` 加 `TestTheCarrierPickupConsumerAndTheTFHandoffAgreeOnTheEventType` 与 `tfpostgres` import；PS `TestTheCarrierPickupConsumerAcceptsTheTypeTheWriterEmits` 头注去「两串各改一边这里就红」改成只钉自己一侧、相等由 cmd 那条对照）——**完成记录随这一笔同提交**（票面 + spec 行）。**验（本机，钉最后一笔）**：`gofmt -l` 空；`go build ./...` / `go vet ./...` 0；不带 DSN PS `adapters/transportfulfillment` + `internal/architecture` ok；21:4x 占号，带 DSN `go test -p 1 -count=1` PS `adapters/inbox` + PS `adapters/transportfulfillment` + TF `adapters/postgres` + `cmd/parcel-dispatch` + `internal/architecture` **五包 ok**（21:42:24→21:42:48），`-v -run 'CarrierPickup|PickupRoute|SupersedingCarrierPickup|AgreeOnTheEventType'` PASS 21 / SKIP 0 / FAIL 0；21:43 释号；清点 `tools/mechanism-inventory` 重生成 porcelain 空（不增删文件、不加端口）。未跑全量。**判据逐项**：1 ✓ `git grep -n -E 'CONTEXT 规则|三路|四个读口' -- internal/parcelshipment/adapters/transportfulfillment` 零；2 ✓ `carrierFirstEffectivePickupEventType` 于 `internal/` 零，目标头注内「第三条」零（全 `cmd/` 非零，见判断项 ①）；3 ✓ 条 1 用例 `-v` PASS，且本机把 `firstEffectivePickupSpecFor` 的时间源临时换成 `record.RecordedAt` 后该用例红（EffectiveAt 10:00:01 ≠ 09:05）、还原后绿，生产文件零改动；4 ✓ 本机把 TF 常量尾巴加一字后 `TestTheCarrierPickupConsumerAndTheTFHandoffAgreeOnTheEventType` 红（报两串不等）、还原后绿；5 ✓ UC-PS-004 `--numstat` 1/1、`ls-files --eol` i/lf w/lf；6 ✓ 见上。**红线**：`HandleRegisteredCarrierFirstEffectivePickup` 分支与结果代数一字未动（生产 `.go` 差只有 `assemble.go` 一行注释与 TF 常量改名 + 头注）；`domain/**`、路由表行、CC / SA / VE 零 diff；新注释无计数、无行号，引文单行可搜。**撞点**：`assemble.go` 只改一句头注（21:3x 占号广播）；`assemble_test.go` 只在文件中段加一条用例 + import 一行；TF postgres 只本票动。**判断项（归评审）**：① 条 3 的「ADR-0049 第三条」在 `assemble.go` 其余各路头注与 `assemble_test.go` 里还有多处，是 lc/25 之前既有写法且散在通道 5 同期动的块附近；本票按派单只改 lc/25 那一句，于是同文件对 ADR-0049 出现「第三条」「决定三」两种写法——是把其余各处一并统一（另笔、占号）还是把本票这一句退回「第三条」，归评审 / owner 定；引文那半不受影响。② 条 4 括注没有照派单原话写成「（实际承运商首次有效收寄事实，ADR-0135）」——那会与主语同词成「X（X，ADR-0135）」；改成「（ADR-0135；外部承运轨迹事实本身不构成收寄）」，含义同且留下旧括注错在哪一句。③ 条 1 用例钉的是既有行为，`/tdd` 的「红」是靠本机临时反向改动生产文件证的（不提交），不是先写红用例再改生产代码；完成记录如实写法。④ 条 5 对照用例放 `cmd/parcel-dispatch` 而非 PS inbox 测试：门禁形式上不拦 `_test.go`，放 cmd 是按 ADR-0025 精神与「组合根本来就同时装配两边」，若评审认为 PS inbox 测试就地 import TF postgres 更直接（lc/27 同形），换位置零行为。
