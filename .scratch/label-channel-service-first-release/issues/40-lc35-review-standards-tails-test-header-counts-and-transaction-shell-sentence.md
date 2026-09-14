# lc/35 非作者评审 Standards 非阻断两条一笔收口：两处测试头注跨文件计数换点名、「壳只管事务」一句留组合根一处

Category: chore
Status: in-progress——2026-09-14 19:2x 通道 2 按通道 1 派单 task-e79cc5a9 自立自做（评审尾巴 A 类推送方派单、作者自立票自做，先例 lc/38 / sa-cc/18 / ve-disc/06），分支 `mcp2-tails3` 基远端 main `09596d9a`（树 `D:/tops/idp-parcel-mcp2-tails3`），与 [sa-cc/23](../../sa-cc-funds-and-credential-seams/issues/23-cc-adopted-fact-version-must-match-envelope-and-dispatch-header-two-rows.md) 同分支；要裁的为零
Blocked by: 无（[35](35-establish-replay-decision-register-reconciliation-and-select-result-shape.md) 已进 main `b293a921`，两条出处全在其 Comments「评审 ← 通道 3」Standards ① ②）。撞点：通道 3 在途 lc/39 纯 docs、通道 4 在途 sa-cc/11 只在 `internal/parcelpricing/**`，与本票地盘零重叠；共享树不碰

## 缺口（出处逐条指到评审原话；取证于 `09596d9a`，开工已重量）

1. **测试头注跨文件计数**（35 评审 ← 通道 3 Standards ①；AGENTS.md「改文档」计数条，Go 注释同受约束）。
   - `internal/parcelshipment/application/establish_selected_label_transaction_test.go` `TestAHalfWiredFlowRefusesLoudly` 头注「四口任一为 nil」——数的是另一文件 `EstablishSelectedLabelTransactionDeps` 的字段（前身「三口」同病，35 把 3 改成 4 未去数）。**重量**：`git grep -n '四口' -- internal/parcelshipment/` 于 `09596d9a`：本测试头注一处；另两处在 `establish_selected_label_transaction.go`（`ErrSelectedLabelTransactionFlowMisconfigured` 头注、`EstablishSelectedLabelTransactionDeps` 头注）与被数 struct 同文件，评审明判可接受，不动。
   - `internal/parcelshipment/adapters/partycommercial/channel_selection_basis_test.go` `TestTheTranslatorRefusesNilReadersAndNamesEachUnwiredSource` 头注「两个 PC 读口 / 三个实例半边源」——数的是另一文件 `ChannelSelectionBasisTranslatorDeps` 的字段。**重量**：文件名与评审所记相符（派单提醒按符号 `git grep` 定位，实得同文件）；`channel_selection_basis.go` 里「三个实例半边源与两个 PC 读口」「两个 PC 读口」两处与 struct 同文件，不动。用例正文里子表两张已把五口逐一点名（账号使用授权读口 / 协议内容读口；授权源 / 协议源 / 接受时解析源），头注照抄即可；正文 `Fatalf`「三源允许为 nil」数的是同函数上方那张子表，同文件，不动。
2. **「壳只管事务」一句两处复述**（35 评审 Standards ②；AGENTS.md「单一权威」）。「壳只管事务：Select 不返 error 就提交、返 error 就回滚」在 `internal/parcelshipment/application/establish_selected_label_transaction.go` `ChannelSelector` 头注与 `cmd/parcel-api/assemble_label_channel.go` `transactionalChannelSelection` 头注各写一遍，会各自漂移。**重量**：`git grep -n '壳只管事务'` 于 `09596d9a` 恰两处，与评审所记同。派单建议留 `cmd/parcel-api` 那处（事务壳在它那里），`ChannelSelector` 头注改为引符号名。

**不在本票**：35 评审 Standards ③（建立前那一问读口故障无段标，评审自判可不改）、Spec ①（裁决 1 末句读法，推送方已收进 35「进 main 记录」）；生产文件里与被数 struct 同文件的「四口」「三个源」；任何生产 `.go` 语句；`domain/**`。

## 做法

1. 两处测试头注去数换点名：`TestAHalfWiredFlowRefusesLoudly`「四口任一为 nil」→「任一口为 nil」；`TestTheTranslatorRefusesNilReadersAndNamesEachUnwiredSource`「两个 PC 读口」→ 点名「账号使用授权 / 协议内容两个读口」的名字不留数词、「三个实例半边源」→ 点名「授权源 / 协议源 / 接受时解析源」。用例正文不动。
2. `ChannelSelector` 头注删去「壳只管事务：Select 不返 error 就提交、返 error 就回滚」那半句，改为「壳怎么提交、怎么回滚写在组合根 `transactionalChannelSelection` 的头注」；「并列与无人参选是结果格不是 error，壳不必认任何领域哨兵（票 35 做法二）」是本层对 `ChannelSelectionResult` 形状的承诺，留在本层。`cmd/parcel-api` 那处一字不动。
3. 两条同一笔（都只注释、同一张评审），完成记录随 sa-cc/23 末笔同提交。

## 红线

- 零行为：diff 只许注释行；`go build` 产物不变。
- 不动 `internal/parcelshipment/domain/**`、`cmd/parcel-api/assemble_label_channel.go`、任何 `.scratch/` 待裁票面、`apps/`。
- 注释中文；不写行号、不数别处的东西。

## 完成判据

1. `git grep -n '四口' -- internal/parcelshipment/` 只剩 `establish_selected_label_transaction.go` 两处；`git grep -n -e '两个 PC 读口' -e '三个实例半边源' -- internal/parcelshipment/` 只剩 `channel_selection_basis.go` 与 `establish_selected_label_transaction.go` 里与 struct 同文件的那几处，`_test.go` 零命中。
2. `git grep -n '壳只管事务'` 全仓恰一处，在 `cmd/parcel-api/assemble_label_channel.go`；`ChannelSelector` 头注含符号名 `transactionalChannelSelection`。
3. `gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；`go test -count=1 ./internal/parcelshipment/application/... ./internal/parcelshipment/adapters/partycommercial/... ./internal/architecture/...` ok；`cmd/parcel-api` 带 DSN ok（反向依赖里的 cmd 包一律带 DSN）。
4. 完成记录随 sa-cc/23 末笔同提交（Status → resolved、逐笔 SHA、判据逐项、判断项）；清点预报零差（不增删文件）。

## 地盘

`internal/parcelshipment/application/establish_selected_label_transaction_test.go`（一处头注）、`internal/parcelshipment/adapters/partycommercial/channel_selection_basis_test.go`（一处头注）、`internal/parcelshipment/application/establish_selected_label_transaction.go`（`ChannelSelector` 头注）、本票面、lc spec 一行。

## 参照

[35](35-establish-replay-decision-register-reconciliation-and-select-result-shape.md) Comments「评审 ← 通道 3」Standards ① ②、「进 main 记录」末条「其余非阻断随票记」；[38](38-lc37-review-tails-adr0049-citation-five-values-count-and-fact-reference-rename.md)（同款「注释去数换点名」先例）；`cmd/parcel-api/assemble_label_channel.go`（`transactionalChannelSelection` 头注，留的那处）；AGENTS.md「写代码注释」「改文档」（计数与行号同构、单一权威）。

## Comments

- 2026-09-14 19:2x · 通道 2（task-e79cc5a9）：立票，Status 直接 in-progress，作者自立自做。**只写票面，未动代码。** 两条在 `09596d9a` 上重量过：条 1 两处测试头注文件名与评审所记一致，`channel_selection_basis_test.go` 正文 `Fatalf`「三源」数同函数子表、不动；条 2「壳只管事务」全仓恰两处。前一会话（同通道）18:22 建完 worktree 即 crash，本会话从零接续。
