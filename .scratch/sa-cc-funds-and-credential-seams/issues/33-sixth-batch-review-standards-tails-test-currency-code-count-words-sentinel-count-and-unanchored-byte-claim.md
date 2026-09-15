# 第六批评审 Standards 尾巴（27 + 29）：`parcel-settlement-register` 与 `registrationjson` 夹具币种用真 ISO 码 `EUR` 而非测试码 `XTS`；`registrationjson` 头注「采用四格」跨包计数；`assemble_test.go` 一处头注把三只未决哨兵数成两只；`fingerprintEventID` 头注引 19 的「138 字节」作论点却没锚 SHA

Category: chore
Status: resolved——**已进 main，2026-09-15 15:1x 通道 1 推送方**（第七批，重放 `60721f60→3e0f1230`，批 tip `c84e81bd`；评审门推送方自审（零行为：生产两文件滤 `//` 后空、测试只 `EUR`→`XTS` 字面 + 一句头注）；`c84e81bd` 带 DSN 全仓 115 ok / 0 FAIL；见 Comments「进 main 记录」）。此前 resolved——**2026-09-15 15:0x 通道 4**（task-c3f6cfd9-1a6f-4235-9e3b-9f9c71783355；分支 `mcp4-sacc33` 基 `93840328`，代码 tip 即本笔；六件全是夹具字面与注释，零行为，见下方「完成记录」）。此前 ready-for-agent——**2026-09-15 14:4x 通道 1 立票**（sa-cc/27 评审 ← 通道 2 Standards ① ② + Spec ②、sa-cc/29 评审 ← 通道 6 Standards ① ②，推送方处置「合一张 A 类零行为尾巴」）。只测试夹具字面与注释，零行为改动
Blocked by: 无（27 / 29 已进 main，第六批 tip `de822820`）

## 缺口（评审各钉 `aa48912e` / `b2229462`，进 main 后在 `de822820` 同形）

**SA / cmd（27）**

- `cmd/parcel-settlement-register/main_test.go` `adoptInput`、`vertical_test.go` `adoptDocumentFor`、`internal/settlementaccounting/adapters/registrationjson/translate_test.go` `adoptDocument` 的币种字面是 `"EUR"`——真 ISO 码；`domain.NewCurrencyCode` 只要非空，同形先例 `cmd/parcel-customs-register/duty_registers_test.go` 用 ISO 测试码 `XTS`。非预填生产默认、不涉实例敏感数据，但夹具纪律「全 `SYN-` / 测试码」在这一处没跟。
- `internal/settlementaccounting/adapters/registrationjson/translate.go` 包头注与 `translate_test.go` 头注写「采用四格」——数的是 `application` 包里 `AdoptFact` 的格数（AGENTS「计数与行号同构」）；「四格」是 ADR-0137 决定四原词，改法是引 ADR 而不引数，或去数。
- 27 完成记录判断项 ⑤ 的措辞（`TestFundsAnswerCoversEveryOutcome`「钉全覆盖」）推送方已在进 main 簿记里改；代码侧若要真「全覆盖」，是给 `FundsOutcome` 加一个哨兵常量并让用例按范围枚举——**本票不做**，只记：今天新增常量会静默落 `default` → 3。

**CC / cmd（29）**

- `cmd/parcel-dispatch/assemble_test.go` `TestARejectedDutyVerificationHandoffIsNotRegisteredAsUndecided` 头注「与两只未决哨兵也互不 errors.Is」——循环遍历的 `externalFundsFactUndecidedSentinels` 实有三只（含 `ErrAdoptedFactNotVisible`），跨文件计数且已数错；改「与名单里每一只」。
- `internal/customscompliance/adapters/postgres/external_result_handoff.go` `fingerprintEventID` 头注引 19 的「四个引用 + 六十四位 = 138 字节」作论点，锚的是票不是 SHA（AGENTS 例外要求数本身是论点时锚取证 SHA）；改成锚 19 原作者实测那一刻的 SHA，或去数只留「串接形随引用长度增长、超上限即被拒」。

## 做法

1. 三处 `"EUR"` → `"XTS"`（或同文件既有的合成币种字面，若有）；断言若比对币种字面同改。
2. `registrationjson` 两处头注去数：写「`AdoptFact` 的答案格」并引 ADR-0137 决定四。
3. `assemble_test.go` 那句改「与 `externalFundsFactUndecidedSentinels` 名单里每一只互不 `errors.Is`」。
4. `fingerprintEventID` 头注：138 字节那句锚 SHA（19 完成记录判断项 ④ 所在笔）或去数。

## 红线

- 零行为改动：`git diff --stat -- ':!*_test.go'` 只应有两处注释文件（`translate.go`、`external_result_handoff.go`）且滤 `^[+-]\s*//` 后为空。
- 不改 `FundsOutcome`、不改哨兵名单、不改任何断言语义。
- 注释中文；不写行号、不数别处。

## 完成判据

1. `git grep -n '"EUR"' -- cmd/parcel-settlement-register internal/settlementaccounting/adapters/registrationjson` 零命中。
2. `git grep -n '四格' -- internal/settlementaccounting/adapters/registrationjson` 零命中；`git grep -n '两只未决哨兵' -- cmd/parcel-dispatch` 零命中；`fingerprintEventID` 头注里的字节数要么带 SHA 要么不在。
3. `gofmt -l` 空；`go vet ./cmd/parcel-settlement-register/... ./internal/settlementaccounting/... ./cmd/parcel-dispatch/... ./internal/customscompliance/adapters/postgres/...` 0；不带 DSN 四处包 ok。
4. 完成记录同笔；清点零差（不增删文件）。

## 地盘

`cmd/parcel-settlement-register/{main_test,vertical_test}.go`、`internal/settlementaccounting/adapters/registrationjson/{translate,translate_test}.go`、`cmd/parcel-dispatch/assemble_test.go`、`internal/customscompliance/adapters/postgres/external_result_handoff.go`。撞点：[32](32-sa-external-funds-fact-envelope-id-has-no-length-bound-and-a-rejected-envelope-folds-into-a-continuation.md) 若同期开工会碰 `cmd/parcel-settlement-register` 生产文件与 `external_funds_fact_handoff.go`，与本票不同文件；[34](34-cc-five-remaining-event-ids-still-concatenate-references.md) 碰 `external_result_handoff.go` 的 `externalResultEventID`——与本票只改头注一句同文件，先后进即可。

## 完成记录（2026-09-15 15:0x 通道 4，单笔，基 `93840328`）

### 逐条对完成判据 1–4

1. **`"EUR"` 零命中**：`git grep -n '"EUR"' -- cmd/parcel-settlement-register internal/settlementaccounting/adapters/registrationjson` 退 1。改的字面：`cmd/parcel-settlement-register/main_test.go` `adoptInput` 一处、`vertical_test.go` `adoptDocumentFor` 一处、`internal/settlementaccounting/adapters/registrationjson/translate_test.go` 五处——`adoptDocument` 常量、`TestExternalFundsFactFromJSONCarriesEveryFieldWithFidelity` 里比对 `command.Currency` 的断言字面、`TestExternalFundsFactFromJSONLetsThePayerBeAbsent` 的内联载荷、「币种空白」格 `strings.Replace(adoptDocument, "XTS" → "")` 的匹配串、「带币种」格往 `correctionDocument` 插入的字面。断言字面与夹具同改，断言语义（币种原样保真 / 空白被拒 / 更正载荷不得带币种）一字不动。
2. **计数词零命中、字节数带锚**：`git grep -n '四格' -- internal/settlementaccounting/adapters/registrationjson` 退 1；`git grep -n '两只未决哨兵' -- cmd/parcel-dispatch` 退 1；`fingerprintEventID` 头注的「138 字节」现在带 `a0cb6fef`（见判断项 ①）。
3. **格式 / vet / 无 DSN 测试**（本会话在 `mcp4-sacc33` 工作树实测，`IDP_PARCEL_POSTGRES_DSN` 确认为空）：`gofmt -l ./internal/ ./cmd/` 空；`go vet ./cmd/parcel-settlement-register/... ./internal/settlementaccounting/... ./cmd/parcel-dispatch/... ./internal/customscompliance/adapters/postgres/...` 退 0；`go test -count=1` 四处包（`cmd/parcel-settlement-register`、`registrationjson`、`cmd/parcel-dispatch`、CC `adapters/postgres`）全 `ok`，真库用例按无 DSN 诚实跳过——本票没动任何真库用例的输入，带 DSN 那一跑归推送方全量。没占 55432。
4. **完成记录同笔、清点零差**：本会话在本工作树对 `tools/mechanism-inventory` 重生成（`-out` 到 `%TEMP%`，不落树）与已提交的 `docs/product/MECHANISM-INVENTORY.md` 零差；六件全 M、无增删文件、无迁移。

### 红线自查

`git diff --stat -- ':!*_test.go' ':!.scratch'` 只列 `external_result_handoff.go`（+3/−2）与 `translate.go`（+2/−1）；两件的 `-U0` diff 滤掉 `^[+-]\s*//` 后为空——生产代码零非注释行。`FundsOutcome`、`externalFundsFactUndecidedSentinels` 名单、各用例断言语义未碰。注释全中文、无行号、无跨包计数。

### 判断项

1. **「138 字节」选了锚 SHA 而不是去数，且数是本会话重算的**：这一句里数就是论点（「短合成引用都已超上限」），去掉只剩「会超」就弱成一句推测；而它描述的串接形已经不在代码里，锚住 SHA 之后它是一条不会再变旧的历史引文。锚 `a0cb6fef`（19 作者 tip、19 完成记录判断项 ④ 钉的那一笔）而非 19 完成记录进 main 的簿记笔 `1f3ed3d4`——前者是那个函数还是串接形的最后一笔，`git show a0cb6fef:internal/customscompliance/adapters/postgres/duty_payment_verification_handoff.go` 能直接看到拼接体；后者只是转记它的文档。数自己重算过：`tenant-a` + `/duty-payment-verification/` + `SYN-UNIT-RD` + `/` + `SYN-DUTY-RD/v1` + `/` + `bank-fact-2` + `/` + 六十四位 = 8 + 27 + 11 + 1 + 14 + 1 + 11 + 1 + 64 = 138，与 19 原作者实测同数；四个引用字面也一并写进头注，读的人不必回票面找。
2. **`translate.go` 头注把「更正的链头判断」也点到符号**：原句「采用四格与更正的链头判断」两半都在数 / 概述别处，改成 `AdoptFact` 的答案格与 `CorrectFact` 的链头判断，与 `MapExternalFundsHandler` 上的两个方法名一一对上；ADR-0137 决定四引的是它「SA 采用是事实进产品的唯一入口」那句，「四格」这个词留给 ADR 自己说。`translate_test.go` 头注多加一句「夹具币种取 ISO 4217 测试码 `XTS`，不用任何流通币的真码」——不在做法 2 字面里，加它是因为 `"XTS"` 单看像个拼错的币种，下一个改夹具的人没有这句会顺手改回真码；只多一句注释，零行为。
3. **`assemble_test.go` 那句改成引切片名而不写数**：「与 `externalFundsFactUndecidedSentinels` 名单里每一只互不 `errors.Is`」——名单增减不再让这句变错；同句后半「名单取该切片本身」顺带去掉了重复一次的长名。
4. **本票不做的两件，照票面记**：`TestFundsAnswerCoversEveryOutcome` 的「全覆盖」若要成真得给 `FundsOutcome` 加哨兵常量按范围枚举，今天新增常量会静默落 `default` → 3（27 评审 Spec ②）；`SYN-` 纪律在 SA 其它夹具上有没有别的真码没扫（只按票面点名的三处改）。

### 能力边界

读了：本票全文；六件里改动处及其所在头注 / 用例全文；`map_external_funds.go` 的 `AdoptFact` / `CorrectFact` 签名行；ADR-0137 决定四正文；`a0cb6fef` 版 `dutyPaymentVerificationEventID` 函数体；`cmd/parcel-customs-register/duty_registers_test.go` 的 `XTS` 先例一行。**没读**：`MapExternalFundsHandler` 实现体；`TestFundsAnswerCoversEveryOutcome` 正文；SA 其余夹具。没跑带 DSN 测试；没改 `spec.md` / `tasks.md`。

## 参照

[27](27-sa-external-funds-fact-adoption-and-correction-registration-face.md) Comments「评审 ← 通道 2」；[29](29-cc-handoff-envelope-id-exceeds-framework-limit-and-rederive-path-swallows-handoff-failure.md) Comments「评审 ← 通道 6」；[28](28-sa-sacc20-review-standards-tail-save-header-states-ordered-versus-concurrent-answers.md) / [30](30-cc-sacc19-review-standards-tails-unordered-test-double-and-hand-built-fixture-keys.md)（同款 A 类尾巴先例）；AGENTS「写代码注释」「改文档」。

## Comments

- 2026-09-15 14:4x · 通道 1：立票（第六批两票评审 Standards 尾巴合收）。只写票面，未动代码。
- **2026-09-15 15:1x · 进 main 记录 · 通道 1 推送方**：**评审门推送方自审**（照 [28](28-sa-sacc20-review-standards-tail-save-header-states-ordered-versus-concurrent-answers.md) / [30](30-cc-sacc19-review-standards-tails-unordered-test-double-and-hand-built-fixture-keys.md) 先例）：`git diff --stat 93840328 60721f60 -- ':!*_test.go' ':!.scratch'` 只 `translate.go` / `external_result_handoff.go`，`-U0` 滤 `^[+-]\s*//` 后为空；测试 hunk 逐行读——七处 `"EUR"`→`"XTS"`（含比对同一字面的断言与 `strings.Replace` 匹配串）+ `assemble_test.go` 一句头注改引切片名；`fingerprintEventID` 头注 138 字节锚 `a0cb6fef` 并把四个引用字面写进去（作者判断项 ① 重算 138 与原作者同）；`translate.go` 头注改引 `AdoptFact` / `CorrectFact` + ADR-0137 决定四、测试头注多一句「`XTS` 是 ISO 4217 测试码」（判断项 ②，接受——防下一人改回真码）。判断项 ①–④ 接受。**重放**：`%TEMP%\idp-replay-wave7` @ `93840328`，`cherry-pick 60721f60` → `3e0f1230` 零冲突，六件对作者 tip 零 diff；同批 pp-seams/07（`0b3e5de9→e1840144`）与 sa-cc/32 取证笔（`d2af6ba1→c84e81bd`），零文件重叠。清点在 tip 重生成零差。**验证（推送方全量一次，`c84e81bd`）**：`gofmt -l` 空；`go build` / `go vet` 0；15:06 占号 → 带 DSN `go test -p 1 -count=1 ./...` **115 ok / 0 FAIL / 16 无测试 / 0 cached**（136 s）；`-v` 探针 `cmd/parcel-settlement-register` + `registrationjson` PASS 非 SKIP → 15:09 释号。簿记一笔在其上（本票 + 07 Status / 记录、sa-cc spec 32 / 33 行、pp-seams spec 07 行、tasks.md），纯 .md 自审；`ls-remote` 核 `93840328` 未动 → `merge --ff-only` → `push <sha>:main`。分支 `mcp4-sacc33@60721f60` 作封存出处、改名 `merged/`、远端删；作者树由通道 4 比内容后拆。
