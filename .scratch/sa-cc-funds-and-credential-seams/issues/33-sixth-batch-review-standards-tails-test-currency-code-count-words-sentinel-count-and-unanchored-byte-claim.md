# 第六批评审 Standards 尾巴（27 + 29）：`parcel-settlement-register` 与 `registrationjson` 夹具币种用真 ISO 码 `EUR` 而非测试码 `XTS`；`registrationjson` 头注「采用四格」跨包计数；`assemble_test.go` 一处头注把三只未决哨兵数成两只；`fingerprintEventID` 头注引 19 的「138 字节」作论点却没锚 SHA

Category: chore
Status: ready-for-agent——**2026-09-15 14:4x 通道 1 立票**（sa-cc/27 评审 ← 通道 2 Standards ① ② + Spec ②、sa-cc/29 评审 ← 通道 6 Standards ① ②，推送方处置「合一张 A 类零行为尾巴」）。只测试夹具字面与注释，零行为改动
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

## 参照

[27](27-sa-external-funds-fact-adoption-and-correction-registration-face.md) Comments「评审 ← 通道 2」；[29](29-cc-handoff-envelope-id-exceeds-framework-limit-and-rederive-path-swallows-handoff-failure.md) Comments「评审 ← 通道 6」；[28](28-sa-sacc20-review-standards-tail-save-header-states-ordered-versus-concurrent-answers.md) / [30](30-cc-sacc19-review-standards-tails-unordered-test-double-and-hand-built-fixture-keys.md)（同款 A 类尾巴先例）；AGENTS「写代码注释」「改文档」。

## Comments

- 2026-09-15 14:4x · 通道 1：立票（第六批两票评审 Standards 尾巴合收）。只写票面，未动代码。
