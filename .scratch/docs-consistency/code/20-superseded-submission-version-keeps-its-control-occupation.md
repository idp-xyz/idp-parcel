# 20 PS 新提交版本取代旧版本时，旧版本已成立的控制项占用无人释放

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code20（见文末「完成记录」）。此前 ready-for-agent——R1（上线前必修）：碰钱、静默。可达性资料不足仍照常进入接受前财务控制并形成占用，任务停在`等待受控补充`；客户补件走新提交版本，旧版本被取代，它那一笔预付冻结或账期额度占用永远不释放——第一周补件就撞，客户可用额度被永久吃掉，没有任何释放入口能按旧版本认领。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/ps.md](../audit/ps.md) PS-01，取证 [audit/ps-code.md](../audit/ps-code.md) 第 1 条（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，PS 代码在此期间未改）。

## 现象

`application` 包里 `ReleasePreAcceptanceControl` 只出现在拒绝、撤回、授权处置三条路，以及 `form_acceptance_decision.go` 的 `releaseIfRejected`。新提交版本形成、旧版本被取代的那一路没有释放调用。占用请求按提交版本认领，新版本的控制顶不掉旧的那一笔。

## 证据

- 文档：
  - PS CONTEXT 词条「控制项结果」：「任一项成立即表示提供方账本上形成了占用；接受确定未成立时必须按原业务关联请求释放，不论接受侧结论如何。」
  - 词条「授权处置」：「两者都按原业务关联释放本版本已成立各项的占用」。ADR-0132 给的理由：选交客户补充的那一刻，这一版的接受已确定不成立——被取代同理。
- 代码（读码推断，审计取证一致）：
  - `internal/parcelshipment/application/form_new_submission_version.go` 与 `domain/submission_supersession.go` 两处都没有释放调用（复核时 `grep Release` 两个文件均为零处）。
  - 可达性答资料不足时 `domain.ReachabilityCheckFor` 记 `CheckUndetermined`，`AdvanceAcceptanceJudgmentHandler` 仍交 `AcceptanceJudgmentAdvanced`；`AdvanceAcceptanceChainHandler` 因此照常进入财务控制并形成占用，`Decide` 再把任务停在`等待受控补充`。
  - `ControlReleaseRequest` 按提交版本认领。

## 三类

机制。无产品策略、无租户取值。

## 规模

S–M：取代那一笔（或经续办信封）对被取代版本 `OccupationFormed()` 为真的各项按原关联发释放；失败按 `ControlReleasePending` 续办；补回归用例。

## 最小的诚实修法

新提交版本形成、旧版本被取代时，同一事务内为旧版本已成立占用的各项登记释放请求（与拒绝、撤回同一个出口），按原业务关联认领；释放未完成的照既有 `ControlReleasePending` 续办。补一条「资料不足 → 已占用 → 补件取代」的回归测试，断言旧版本的占用被释放、新版本重新判控制。

## 要裁的

无（CONTEXT 与 ADR-0132 的理由已覆盖）。存量：首发前只有演示租户，`--reset` 重灌即可。

## 完成记录（2026-10-21，分支 claude-code20，基于 main `3fb2108db`；评审后 rebase 于 main `e013fef6e`，下文 SHA 均为 rebase 后的）

### 做了什么

- `d8c773f5b` 接受链每一轮在三步之前，为被取代各版 `OccupationFormed()` 为真的控制结果按原业务关联请求释放（`internal/parcelshipment/application/release_superseded_occupation.go`，`AdvanceAcceptanceChainHandler.Handle` 调它）。
  - 用形成决定那一步已持有的判断读口与释放端口，不另接释放出口，装配一行不改（生产链在 `cmd/parcel-dispatch` 已把 `Release` 接成带作用域源的 PS→SA 适配器）。
  - 放不掉：停在新阶段 `SUPERSEDED_CONTROL_RELEASE`，原因 `CONTROL_RELEASE_PENDING`，续办引用范围与那一版被拒绝时 `releaseIfRejected` 派生的逐字一致；读不回旧版本判断：同阶段、原因 `RECORDED_JUDGMENTS_UNAVAILABLE`。两格恢复动作都是内部重试，消费门回滚重投，当前版本一步不判。不新增未决哨兵。
  - 每一轮都问，本方不记「放过没有」；认领与幂等归 SA 账本。
  - 编排用例：先放后判且只放旧版本、无占用不放（三格）、放不掉即停、重驱再请求、读不回即停、首版不问释放。
  - 派发真库用例 `TestASupersededVersionsFreezeIsReleasedOnceBeforeTheNewVersionIsControlled`：控制与释放走生产 PS→SA 适配器、作用域源、账户目录、SA 施加与释放编排、两本账与采用册，余额只够冻一笔。首版资料不足照样冻下一笔、停等受控补充；补件后第一拍注入释放失败，整封交回未决哨兵、新版本不判不占；重投后首版那一笔释放、新版本照常冻下一笔并接受；绕过 inbox 再驱一拍，首版释放时刻不变。去掉本票改动时该用例红在新版本的控制受限（余额被旧冻结吃着）。
  - 受控补充闭环夹具改成控制与释放两口可换；试算替身可选答带合计的试算。
- `1f525138d` PS CONTEXT 生命周期「已提交期间形成新提交版本」补规则句；UC-PS-001 接受链一段补释放那一步与四格阶段码（逐码对照 `AcceptanceChainStage.String()`：`SUPERSEDED_CONTROL_RELEASE`、`REACHABILITY_JUDGMENT`、`FINANCIAL_CONTROL_JUDGMENT`、`ACCEPTANCE_DECISION`；结果三格 `DECIDED`、`UNDECIDED`、`VERSION_SUPERSEDED` 原已对上），「给开发的交接」`AT-PS-035` 一行改为已补。
- `428c97a17` AT 盘点 `AT-PS-035` 行补点名并追记。
- 评审修订：
  - `d631614a3` SA `ReleasePreAcceptanceControlHandler`：冻结或暴露已是「已释放」即交回原释放、跳过 `Save`，补 `TestARepeatedReleaseWritesNeitherLedgerAgain`（两本账各只写一次、原释放时刻不变；去掉改动即红）。
  - `686e1f6f9` 注释：`releaseSuperseded` 补「释放随整轮一起提交」；`AcceptanceDecisionNotHandedOff` 的「同类」改说清两边各是什么事；`JudgmentPendingReason` 头注断行理顺；替身 `controlByVersion` 点名压过的两格。
  - `ed026c4ac` 立 code/70 至 code/73。

### 代决（用户授权「你作为业务和系统专家自决……解决所有的问题」）

1. **释放落在接受链开头，不落在形成新版本那一笔。** 理由三条：
   - 先放后判：同一轮里释放先于新版本的控制，客户的余额或额度不被旧版本那一笔吃着。
   - 有人续办：放不掉时内部重试，消费门回滚重投；形成新版本是客户的一次请求，失败了只能交回一条补偿引用，之后没有东西去推它。
   - 能放得掉：`cmd/parcel-api` 受控补充那条装配没有作用域源，真有占用要释放时适配器报装配缺口；派发进程里的释放端口是接全的。
2. **放不掉即停，新版本不判。** 不选「照判、留补偿引用」：旧占用在时新版本的控制可能受限、按策略被拒，拒绝不可覆盖，一次可续办的补偿就改写成了业务结果。
3. **放在链上，任一扇门驱到都先收口**，不只挂在「新提交版本已形成」那扇门：不改装配（照 ADR-0222，`assemble.go` 不动），复核、登记各门驱到同一份委托时也一并收口。
4. **所有被取代版本都放**，不只紧邻那一版；不论委托此刻是否已形成决定（被取代的版本不会是接受所在那一版）。
5. **复用 `CONTROL_RELEASE_PENDING`**，只把它的注释从「决定已成立」放宽到「接受已确定不成立（决定已成立或已被取代）」；新立阶段格而不借财务控制那一格，停在这里时当前版本的控制一步都没走。
6. 无须新 ADR：CONTEXT 词条「控制项结果」与 ADR-0132 决定一的理由已覆盖「被取代即释放」；本票定的是落在哪一步、失败怎么停，属机制。

### 越权风险点（待 PS owner 复核）

1. **释放确定失败会挡住新版本。** PS 释放端口上可重试与确定性失败分不开（拒绝、撤回、处置各路同此），SA 若对旧版本那一笔答确定性失败，新版本一直内部重试到失败预算耗尽、信封死信，无人判。去向 [code/71](./71-release-and-control-ports-answer-a-closed-set-instead-of-a-bare-error.md)。
2. **暂停期间仍占着。** Decide 停在`等待受控补充`时旧版本那一笔一直占到补件或撤回。协调方已裁暂停即放，去向 [code/72](./72-a-customer-supplement-pause-releases-the-paused-versions-occupation.md)。
3. **每一轮多一次释放请求。** 有被取代版本且占过的委托，链每一轮都对 SA 请求一次释放（含一次作用域重解与账本读），已释放的答原释放。
4. **并发窗口（按评审结论改写）。** 旧版本那一轮链在途时客户补件：旧版本那一轮要提交，必须先把等待态或决定 `Save` 进聚合，而补件已经推进了聚合版本，那一笔 `Save` 撞乐观锁，整轮连同 SA 那一笔占用一起回滚，重投时链答`版本已换代`。所以旧版本的占用不会在新版本那一轮释放之后才落库；窗口比原先写的窄，本票不另加锁。
5. **处置「交客户补充」的释放今天在 `cmd/parcel-api` 必然留补偿**（作用域源未装配，既有）。补件之后本票这一步把它收口；客户不补件时那一笔仍占着，不在本票内。
6. 阶段码多一格，消费门错误正文多一种字面；按旧三格聚合的运维读面要跟进。
7. 存量：首发前只有演示租户；既存孤儿在下一次链轮次被收口，或 `--reset` 重灌。
8. **释放随整轮一起提交。** 释放与新版本的判断同在消费门那一笔事务里；新版本那一轮因别的原因回滚，释放跟着回滚。新版本那一轮能提交之前，首版一直占着。
9. **SA 账本整册回写的丢失更新**（本票之前就有）：本票让重复释放变多，已止血（重复释放不再回写）；首次释放与施加仍整册写，去向 [code/70](./70-sa-control-ledgers-rewrite-the-whole-scope-and-lose-updates.md)。
10. **终局路径只放当前版本**：新版本那一轮提交之前委托就到了终局，被取代版本那一笔没人放。去向 [code/73](./73-terminal-paths-release-every-superseded-versions-occupation.md)。

### AT

- `AT-PS-035`：被取代版本那一格补编排两条、派发真库一条，行仍在①，总览计数未动。
- 未重生成 `docs/product/MECHANISM-INVENTORY.md`、ADR 索引与 AT 计数。

### 验证（首轮，代码同 `d8c773f5b`）

- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/parcelshipment/... ./internal/settlementaccounting/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./tests/bentocontract/... ./internal/architecture/... ./migrations/...`：退 0，37 ok、3 无测试文件、0 FAIL。PS `application` 的反向依赖（`go list` 算）全在这一组内。
- 新用例与改动用例 `-count=3`（PS `application`、`cmd/parcel-dispatch`）：ok。
- 去掉链里那一步重跑：编排四条与派发真库一条变红，恢复后绿。
- `go build ./...`、`go vet ./...`、`gofmt -l` 干净。
- 演示种子：一次性库 `code20seed` 上灌入退 0（「种子灌入完成」），用后 `DROP DATABASE code20seed WITH (FORCE)`。无迁移。

## 评审记录

### 第 1 轮（隔离评审，协调方转达）

没有阻断。变异测试全红，键与幂等核过。逐条去向：

| 条目 | 去向 |
|---|---|
| N1 SA 释放重复时整册回写 | 本票止血：`d631614a3`，已释放即跳过 `Save`，补用例。整册回写的丢失更新本身立 [code/70](./70-sa-control-ledgers-rewrite-the-whole-scope-and-lose-updates.md)（R1）|
| N5 释放随整轮提交 | 已补：`releaseSuperseded` 注释与越权风险点 8 |
| 越权风险点 4 窗口 | 已按评审结论改写（在途旧版本那一轮撞聚合乐观锁、整体回滚）|
| N6 释放时现查作用域、映射变了静默成孤儿 | 作为风险并入 [code/71](./71-release-and-control-ports-answer-a-closed-set-instead-of-a-bare-error.md) |
| N7 注释 | 已修：`controlByVersion` 点名、`AcceptanceDecisionNotHandedOff` 同类说法、`judgment_continuation.go` 头注断行 |
| 释放与控制端口答封闭集 | 立 [code/71](./71-release-and-control-ports-answer-a-closed-set-instead-of-a-bare-error.md)（R2），各释放点一起改，对齐 ADR-0081 决定三 |
| 停在等待受控补充即释放 | 立 [code/72](./72-a-customer-supplement-pause-releases-the-paused-versions-occupation.md)（R2），协调方已裁照做、不另写 ADR，落地时 PS CONTEXT 补一句并引 ADR-0132 |
| 终局路径放被取代各版 | 立 [code/73](./73-terminal-paths-release-every-superseded-versions-occupation.md)（R2），依据 UC-PS-005 步骤 6 |

未采纳：一处措辞。`AcceptanceDecisionNotHandedOff` 的注释没有照字面改成「接受已确定不成立留下的随附事项」：发布意图没交出这一格也覆盖已接受的决定，照字面写会说错它；改为点明两边各是什么事——这里是已成立的决定，`ControlReleasePending` 那里是接受已确定不成立的版本。

验证（rebase 于 main `e013fef6e` 之后，代码同 `686e1f6f9`）：
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/parcelshipment/... ./internal/settlementaccounting/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./tests/bentocontract/... ./internal/architecture/... ./migrations/...`：退出码 0，37 个包 ok、3 个无测试文件、0 FAIL。
- 新用例与改动用例 `-count=3`（PS `application`、SA `application`、`cmd/parcel-dispatch`）：ok。
- `go build ./...`、`go vet ./...`、`gofmt -l` 干净。
- rebase 无冲突，AT 盘点没有撞行。
