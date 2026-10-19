# 05 终止参与与交接更正并发时终点丢失：终止写在被替代的旧链尾上、答 201，更正派生的新链尾没有终点，参与又成了在场

Category: bug
Status: resolved——2026-10-18 进 main（03、05 同批，代码验证钉 `3fae64f8`），见「进 main 记录」。此前 in-progress——2026-10-18 完工；评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-18 作者子代理完工，待评审。此前 in-progress——2026-10-18 会话 `idp-parcel-9b` 代决并认领（分支 `claude-tfa0305`）。此前 needs-triage——2026-10-18 会话 `idp-parcel-9b` 立（操作者与集成客户端渠道逐口核代数研判；实测发现未经对抗核验，裁定待做，动手前先复现）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：逐口核研判（workflow review26-face-algebra-sweep）：/transport-fulfillment-participation-terminations, /transport-fulfillment/handover-corrections
地盘：
- internal/transportfulfillment/adapters/postgres/fulfillment_segment_registry.go 及其测试
- cmd/parcel-api/（新增真库并发回归测试文件）
- （只在取「修复方向」第二条时另加：internal/transportfulfillment/application/ 里调用段参与替代的各重派生调用点，届时重新协调地盘）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 裁定（2026-10-18，会话 idp-parcel-9b 代决）

取第一条「悲观锁」：替代路径在同一事务里先 `SELECT … FOR UPDATE` 锁住被替代的链尾行，再读终点、派生、插新链尾；终止的窄口更新只改链尾行，两者在同一行上排队。改动集中在段登记册的一处读法，不扩散到各编排。与 tf-face-algebra/03 同一分支接连做。

## 现象
终止事务的窄口 UPDATE 已执行、尚未提交时，另一事务提交交接更正 v1→v2（裁决仍是已交接）。之后的经过：
1. 交接更正没有被行锁挡住。核验日志原话：「未被行锁挡住」。
2. 新链尾 v2 按更正前读到的状态派生，终点为空。
3. 终止随后提交，答 201 `PARTICIPATION_ENDED`，但终点落在已被替代的 v1 上。

结果是参与重新在场：同一终止原样重放，又答 201，再把终点写到 v2 上；第三次投递才答已离场。

## 复现（S）
在 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/wt-decide2 下执行：
`IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' go test -overlay <overlay> -count=1 -p 1 -run '<用例>' -v ./cmd/parcel-api/`

两条探针都用钩子确定性复现：钩子包在生产段登记册外面，在另一连接、另一事务里跑生产编排，提交后才返回。
- /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/transport-fulfillment-participation-terminations/adversary/zz_adv_probe_test.go 的 `TestProbeAdvPTCorrectionLandsBetweenTheEndUpdateAndCommit`（同目录 overlay.json，日志 run-adv2.log）。
- 同上级目录下 impl-verify/zz_implverify_test.go 的 `TestImplVerifyPTCorrectionBetweenTheEndUpdateAndCommit`：操作者册用生产的 postgres 实现；日志 run-iv3.log。

## 应当
终点写入与链尾替代必须互斥，二者择一：
- 替代等终止提交之后，再按新状态派生。`ActualFulfillmentSegment.rederive` 只继承终点三件，所以新链尾会带上终点。
- 终止撞上已被替代的链尾时，按读回后的状态作答。

不应出现答了 201、终点却丢失的情况。

## 根因
- 替代路径在 READ COMMITTED 下读段，不加锁。
- 它插入的是新链尾行，不更新旧链尾行。
- 终止的「UPDATE … WHERE activeParticipationPredicate」只锁它更新的那一行。

因此两者不争同一行锁。

## 修复方向（机制，二选一，归 TF owner）
1. **悲观锁**：替代路径在同一事务里先 `SELECT … FOR UPDATE` 锁住被替代的链尾行，再读终点、派生、插新链尾。终止的 UPDATE 也只改链尾行，两者就在同一行上排队。改动落在段登记册：加一个「按键读并锁链尾」的读法，供替代路径用。
2. **乐观校验**：插新链尾时带条件「被替代链尾的终点仍是派生时读到的值」；不成立就答一个新的写结果「链尾已变」，由各调用方读回后重派生。这要动各重派生调用点。

推荐第一条：改动集中在适配器的一处读法上，不扩散到各编排。

## 是否潜伏
已可触发，但窗口窄：交接更正与终止要在同一参与上几乎同时提交。两口都已开真渠道（终止经运营决定渠道，交接更正经作业事实登记渠道）。

## 完成判据
- [ ] 真库并发回归（`cmd/parcel-api` 新测试文件，钩子同上）：
  - 更正落在终止 UPDATE 与提交之间时，事后满足其一：新链尾带终点（继承下来的），或终止不答 201；
  - 此后同一终止重放答已离场，并且不再写库。
- [ ] 反序也要覆盖（更正先读、终止后到），结论同上；两种次序下都不死锁，用例带超时。
- [ ] 既有用例照过：`TestProbeAdvPTConcurrentLoserBehindASupersedingCorrection` 那一格由〈终止参与回显〉收；本票不回退它。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退出码 0，`gofmt -l .` 无输出；带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...` 全部 ok。
- [ ] 票 15 补记下追加回写行（只加行）。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：作者自验、非作者评审（两轴分开）、推送方在干净检出上带 DSN 跑全量。

## 完成记录（2026-10-18，会话 idp-parcel-9b 的实施子代理，分支 claude-tfa0305）

- 落点：`c9140d96`。`Supersede` 落笔前 `SELECT … FOR UPDATE` 锁住被替代的链尾，新版本离场三列取锁后在册值（`lockedInheritedEnd`），其余各格仍取调用方交来的参与；继承终点早于更正后起点时交错误让编排记欠账，不去撞 0006 的 CHECK 作废整笔事务。
- 越出裁定字面的一处（同文件、同一机制）：`EndParticipation` 先单独 `SELECT … FOR UPDATE` 锁行、再以新语句判在场并写。只锁替代口不够——更正已落替代版本未提交时到来的终止，UPDATE 在行锁上等到对方提交后不重判链尾谓词就照写，终点仍落在旧链尾；实测见下。
- 各格对应用例（`cmd/parcel-api/participation_termination_handover_race_test.go`，钩子包生产段登记册，另一方走生产编排、另一条连接；等对方时用池外独立连接看 `pg_stat_activity` 的等锁，日志记「卡在行锁上，放行」或「没被挡住」，全部带 10 秒超时）：
  - 终止写后未提交时更正到来（票面现象）→ `TestAHandoverCorrectionArrivingInsideAnOpenTerminationKeepsTheEnd`
  - 反序·更正读后终止提交、更正再落笔 → `TestATerminationCommittedBetweenACorrectionsReadAndItsSupersedeIsKept`
  - 反序·更正落笔未提交时终止到来 → `TestATerminationArrivingWhileACorrectionHoldsTheTailKeepsTheEnd`
  - 三条共同判据：终止答 201 则链尾带它的终点；随后同一终止重放答已离场，全库表内容与序列不变。
  - 适配器：`TestSupersedeInheritsAnEndCommittedAfterTheCallerRead`、`TestSupersedeRefusesWhenTheInheritedEndPrecedesTheCorrectedStart`（拒写后同一事务仍可用）。
  - 既有格：`TestAConcurrentTerminationLoserBehindASupersedingCorrectionDoesNotClaimTheEnd`（03 收）照过。
- 先红后绿：适配器红 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa05-adapter-red.log；三条并发用例在修前登记册上全红（终点丢在旧链尾）/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa05-cmd-red.log；只修替代口时第三条仍红 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa05-cmd-partial.log；全修后三条绿、两条等锁格均观察到「卡在行锁上」/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa05-cmd-green.log。
- 复现不了的格：无。
- 未做：完成判据「票 15 补记下追加回写行」越出本票地盘；非作者评审与推送方全量不归作者。
- 门：同 03（/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/tfa-gates.log，全 ok）；gofmt、build、vet 干净。
- 证据层级：S。
- 评审：待会话读 diff 审。

## Comments

- **评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审）· 钉 `3a178474`（基线 `4c2b0daa`）· 2026-10-18**：用户选「精简续做」，评审由会话本身读 diff 做；代码出自本会话起的实施子代理，不算非作者评审，如实记下。
  - **Spec**：阻断无。`Supersede` 落笔前 `lockedInheritedEnd` 以 `FOR UPDATE` 锁住被替代链尾，新版本离场三列取锁后在册值；继承终点早于更正后起点时交错误由编排记欠账，不去撞 0006 的 CHECK 让整笔事务作废。认可超出裁定字面的一处：`EndParticipation` 也先单独锁行再判在场并写——只锁替代口时第三种交错仍红（实施者 overlay 实测）。三种交错的真库并发回归在修前全红、修后全绿，日志可见卡在行锁上再放行。
  - **Standards**：阻断无。死锁面：两条路径各只锁一行、且都是终止读到的那一版，同一行上排队，不形成环；跨段的多行事务在 PG 侦测到死锁时会中止一方，属既有风险，未扩大。

### 进 main 记录（2026-10-18，会话 idp-parcel-9b）

- 分支 `claude-tfa0305`（03、05 同一分支）rebase 到远端 main 之上，快进推送；机制清点那一笔在重放后的树上重新生成。验证钉 `3fae64f8` 之后远端只多一笔纯 `.md`（`4e000051`），rebase 后代码与已验那份逐字相同，未重跑。
- 推送方全量一次：钉 `3fae64f8` 的隔离 detached 检出，带 DSN `go test -p 1 -count=1 -v ./...` 退 0——有测试的包 143 个 ok、0 FAIL；`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）。同一检出上 `go build ./...`、`go vet ./...` 退 0，机制清点重生成零差。
