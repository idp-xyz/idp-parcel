# sa-cc/19 评审 Standards 尾巴：适配器测试替身 `rederiveStores.ListVerificationsByFundsFact` 遍历 map 交回、不守端口口径「核对时刻升序、同刻按指纹字典序」；`duty_registers_test.go` 两处 seed 手拼 `tenant + "/" + fact + "/" + version` 不走同文件的 `fundsVersionKey`

Category: chore
Status: ready-for-agent——2026-09-15 12:4x 通道 1 立票（sa-cc/19 非作者评审 ← 通道 6 Standards 非阻断 ① ②，推送方处置「合一张 A 类零行为尾巴」）。只测试文件，零生产改动
Blocked by: 无（sa-cc/19 已进 main `49ffc96c`）

## 缺口（评审钉 `a0cb6fef`，进 main 后在 `07341b8b` 同形）

- `internal/customscompliance/adapters/settlementaccounting/receive_on_adopted_funds_fact_test.go` 替身 `rederiveStores` 的 `ListVerificationsByFundsFact` 遍历 map 交回、无序；端口 `ports.DutyVerificationStore.ListVerificationsByFundsFact` 头注写死「核对时刻升序、同一时刻按指纹字典序」，`latestVerificationPerLineage` 只在**同刻并存**时依赖这个序（严格更晚才换人）。适配器那两格今天没有同刻两版被列到的路径（重投走`已存在`不触发、v3 那格列之前就故障），所以绿；将来谁在这本替身上铺同刻两版会 flake。应用层同名替身 `dutyStoreDouble`（`reconcile_duty_payment_test.go`）已按口径 `sort.Slice`。
- `cmd/parcel-customs-register/duty_registers_test.go` `seedFundsFact` 用 `tenant+"/"+fact+"/"+version.String()` 作 `fakeDutyBook.funds` 的键、`seedFundsFactWithoutPayer` 用 `tenant + "/" + fact + "/" + fact + "/v1"`，都不走同文件为此加的 `fundsVersionKey(tenant, fact, version)`——键的拼法有两份，改一处另一处不跟。

## 做法

1. `rederiveStores.ListVerificationsByFundsFact` 交回前照 `dutyStoreDouble` 那份 `sort.Slice`（先 `VerifiedAt` 升序、同刻按 `Key.Digest` 字典序）；替身头注补一句「与端口口径同序，同刻两版靠它分先后」。
2. **可选**（作者定，做了写判断项）：在该文件加一格「同刻两版在册、v2 到达 → `latestVerificationPerLineage` 取指纹字典序小的那版承前」，把 19 判断项 ② 写成断言。
3. `seedFundsFact` / `seedFundsFactWithoutPayer` 的键改走 `fundsVersionKey`——后者的 `fact + "/v1"` 先经 `domain.NewFundsFactVersion` 再传，与前者同一路。

## 红线

- 零生产改动：`git diff --stat -- ':!*_test.go'` 为空；`internal/customscompliance/**` 非测试文件零 diff。
- 不改端口口径、不改 `latestVerificationPerLineage`；替身只是守口径，不替生产排序。
- 注释中文；不写行号、不数别处。

## 完成判据

1. `rederiveStores.ListVerificationsByFundsFact` 有排序且与 `dutyStoreDouble` 同键序；若做了做法 2，那一格 PASS。
2. `git grep -n 'tenant+"/"+fact+"/"\|tenant + "/" + fact + "/"' -- cmd/parcel-customs-register/duty_registers_test.go` 只剩 `fundsVersionKey` 自己那一行。
3. `gofmt -l` 空；`go vet ./internal/customscompliance/... ./cmd/parcel-customs-register/...` 0；不带 DSN 两处包 ok。
4. 完成记录同笔；清点零差（不增删文件）。

## 地盘

`internal/customscompliance/adapters/settlementaccounting/receive_on_adopted_funds_fact_test.go`、`cmd/parcel-customs-register/duty_registers_test.go`（只测试）。撞点：[29](29-cc-handoff-envelope-id-exceeds-framework-limit-and-rederive-path-swallows-handoff-failure.md) 若同期开工会碰 `receive_on_adopted_funds_fact.go`（生产文件）与 `assemble_test.go`，与本票两份测试文件不同文件。

## 参照

[19](19-cc-new-funds-fact-version-forms-a-new-verification-version.md) Comments「评审 ← 通道 6」Standards ① ②、「完成记录」判断项 ②；[28](28-sa-sacc20-review-standards-tail-save-header-states-ordered-versus-concurrent-answers.md)（同款 A 类尾巴先例）；AGENTS「写代码注释」。

## Comments

- 2026-09-15 12:4x · 通道 1：立票（评审尾巴，推送方处置时点名）。只写票面，未动代码。
