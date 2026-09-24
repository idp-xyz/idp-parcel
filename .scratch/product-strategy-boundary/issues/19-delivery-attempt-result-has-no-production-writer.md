# 19 派送尝试结果没有生产写入方

Category: enhancement
Status: resolved · 已进 main `30e2c929`——2026-09-24 通道 4 完成（分支 `mcp4-psb19` 基 `18733e8d`，纯快进进 main、SHA 不换；评审为推送方自审，不算非作者评审），完成记录见文末。此前 in-progress——2026-09-24 通道 4 认领（用户同日令「做你自己的」，选通道 2 承接清单之外的票），隔离 worktree `idp-parcel-mcp4-psb19`、分支 `mcp4-psb19`。此前 ready-for-agent——2026-09-24 通道 2 立票（用户令通道 2 独立承接）；纯机制缺口，UC-TF-006 已写明来源与不变量，无待裁项
Blocked by: 无
地盘：`internal/transportfulfillment`（写口、持久化写入、编排、`adapters/http` 端点）、`cmd/parcel-api` 端点表一行与隔离写准入。
出处：[票 05](./05-demo-journey-criterion-evidence.md) 格 12「08 重走」新停点：交付答 `SOURCE_NOT_ACCEPTED`。

## 现象

有效交付只能落在已登记的派送尝试结果上（`RegisterEffectiveDelivery` 经 `DeliveryAttemptView` 读），而派送尝试登记册 `DeliveryAttempts` 只读、全仓没有生产写入方。它的头注写着派送尝试「由执行侧登记（与揽收侧的 PickupAttempts 对称）」——揽收侧那一半在，派送侧这一半没有。领域面在：`FormDeliveryAttemptResult`（逐对象、失败带依据、妥投不带依据、不早于到场），库表 `delivery_attempt` 与 `delivery_attempt_result` 在。

## 做什么

1. 写口与持久化：派送尝试与逐对象结果追加式登记，照揽收侧同形——同一尝试身份同内容重放答已存在，换内容答冲突；每次到场是新尝试，不覆盖旧尝试（UC-TF-006「派送尝试来源」一行）。
2. 编排：登记派送尝试结果，派送任务须已开且覆盖所报对象；业务时间与执行方随命令进来，不在服务端代铸（ADR-0023）。
3. 入口：`adapters/http` 一口，进 `cmd/parcel-api` 端点表；隔离形态照 operator-channel/08 经写开关放行，生产形态照常答 `ACCESS_CHANNEL_NOT_CONFIGURED`，等操作者渠道（前线操作者族，operator-channel/10 的作业事实能力面）。

## 不做

- 外部履约方的派送尝试来源（入向连接器）：连接器形态归[票 09](./09-tf-fulfillment-judgment-methods-and-connectors.md)。
- 由派送尝试形成运输收费发生项：UC-TF-006 另一步，本票不碰。

## 完成判据

- 领域与编排用例：首登、同身份重放、换内容冲突、任务未开或对象不在任务范围各自答复。
- 真库（含 DSN）：写入与 `LoadDeliveryResult` 读回同一份。
- 演示动线（只记 `S`）：隔离形态下登记一次妥投结果后，交付不再答 `SOURCE_NOT_ACCEPTED`，结果写回票 05 格 12。

## 完成记录（2026-09-24，通道 4）

**落点**（分支 `mcp4-psb19`，基 `18733e8d`；进 main 为纯快进 `18733e8d..30e2c929`，SHA 不换）：

| 笔 | 内容 |
|---|---|
| `7ded1466` | 存取口 `ports.DeliveryAttemptStore`（只首登与读回）与只读的 `ports.DispatchTaskReader`；编排 `application.RecordDeliveryAttemptHandler` |
| `6f2ed5f0` | 写入方 `tfpostgres.DeliveryAttempts` 的 `Save` / `FindByKey`（先父后子、撞键答已有记录、无事务即拒）；`DispatchTasks` 声明满足只读口 |
| `bc113e1c` | 登记口 `tfhttp.NewRecordDeliveryAttemptEndpoint` 与 `DeliveryAttemptIntake`；`UnconfiguredIntake` 堵住、`IsolatedCommandIntake` 逐口放行 |
| `bf1e8ef2` | `cmd/parcel-api`：`buildDeliveryAttemptOrchestration`（整笔一事务）、端点表一行、隔离放行名单一行；写口补无事务负向证据与回滚证据 |
| `30e2c929` | 机制清点在分支干净检出上重生成 |

**完成判据**：

- ✅ 领域与编排用例：首登 `TestAFirstDeliveryAttemptIsRecordedWithEachObjectResult`；同身份重放 `TestReplayingTheSameAttemptWithTheSameContentReturnsTheExistingResult`；换内容冲突 `TestTheSameAttemptWithDifferentContentIsAConflictAndKeepsTheOriginal`；任务未开 `TestAnAttemptOnATaskThatIsNotAnOpenDeliveryTaskIsNotRecorded`（不存在、揽收任务、已终止）；对象不在任务范围 `TestAnAttemptReportingAnObjectOutsideTheTaskIsNotRecorded`。另有未受理、未决与并发落败各格。
- ✅ 真库（含 DSN）：`TestASavedDeliveryAttemptReadsBackThroughBothReads`——写入后经 `FindByKey` 与 `LoadDeliveryResult` 读回同一份；另有同一尝试再存不覆盖、无事务即拒、回滚不留行。
- ◑ 演示动线（只记 `S`）：`TestARecordedDeliveryAttemptLetsTheEffectiveDeliveryThrough`——建任务、登记尝试、交付三口走生产装配与进程自己的隔离 Intake，尝试登记之前交付 `200 SOURCE_NOT_ACCEPTED`，登记一次妥投之后 `201 DELIVERY_REGISTERED`；已写回票 05 格 12「19 重走」。半格在于没有按 `seed.sh` 端到端实跑动线，证据是同一组装配上的真库用例。

**判断项**（留给评审）：

1. 幂等键取（租户，尝试）：派送尝试表以尝试身份为主键、没有来源身份与摘要列；内容指纹两侧都从领域记录现算，未加迁移。
2. 「任务未开」一格并了三种情形（不存在、揽收任务、已关闭），恢复动作同一个；「对象不在任务范围」另成一格。
3. 先查重放再核任务：任务关闭之后重投同一份，照旧答原结果。
4. 未核尝试的地点、计划窗口与任务的地点、时间窗是否一致：票面没要，窗口外的到场也是真事实；要不要核归 TF owner。
5. 路径取 `/transport-fulfillment/delivery-attempts`（执行方回传的来源事实那一组），不取运营写面的 `transport-fulfillment-` 前缀。
6. 不交发布意图：由尝试形成运输收费发生项是 UC-TF-006 另一步，票面「不做」。

**验证**：钉 `30e2c929` 干净检出——`gofmt -l` 空，`go build` / `go vet` 全仓 0，`tools/mechanism-inventory` vet 与 test ok，带 DSN `go test -p 1 -count=1 ./...` 退 0、无 FAIL（取证时刻）。

**评审**：推送方自审（两轴串行），不算非作者评审——用户同日令通道 4「不要广播和派发」，无从点名非作者通道。Standards：注释中文、无行号与计数引用、领域包未动、应用层只经端口、写口有无事务负向证据；Spec：做什么三条与完成判据三条逐条对上，「不做」两条未做进去。

**未做**：外部履约方的入向连接器（票 09）；由派送尝试形成运输收费发生项（UC-TF-006 另一步）；生产渠道（operator-channel/10 的作业事实能力面）。
