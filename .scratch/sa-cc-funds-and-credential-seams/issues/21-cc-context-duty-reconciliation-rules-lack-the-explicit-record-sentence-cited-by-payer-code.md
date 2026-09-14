# CC CONTEXT「税费付款核对」没有「未提供必须明确记录、规则要求而缺失保持未决」那句：付款人一格的代码注释、`0020` 头注与 sa-cc/12 裁决 2 引的都是「监管处置决定」词条——补句并改正引文归属

Category: chore
Status: draft——2026-09-14 16:0x 通道 1（接管会话）立票（sa-cc/12 补评审 ← 通道 3 Standards 非阻断 ①；归 CC owner）。只写票面未动代码、未动 `docs/**`；取证锚 main `01974923`
Blocked by: 无（12 已进 main；要裁的一句归 CC owner）

## 缺口（取证于 `01974923`，逐符号名）

- `docs/domain/customs-compliance/CONTEXT.md`：「未提供或不适用必须明确记录，规则要求但缺失时保持未决」这句只出现在**监管处置决定**词条（数量 / 期限 / 条件 / 证据要求那几维）；**税费付款核对**词条只有「来源提供或真实程序要求的付款人、金额、币种、业务时间等维度」，Rules「税费、放行与案件闭环」段没有这句。
- 引它当作税费付款核对规则的地方（`git grep` 于 `01974923`）：`internal/customscompliance/domain/funds_fact_payer.go` 包头注；`internal/customscompliance/application/reconcile_duty_payment.go` `ReceiveFundsFact` / `DutyReconciliationReason` 头注；`reconcile_duty_payment_test.go` `TestThePayerDimensionIsJudgedByTheProcedureRule` 头注；`migrations/customs_compliance/0020_funds_fact_payer_may_be_unprovided_and_payer_rule.sql` 头注；`internal/customscompliance/adapters/http` `writeDutyReconciliationAnswer` 头注；[12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md)「裁决」2 的 10:5x 改口（「CC CONTEXT 原词是『规则要求但缺失时保持未决』」）。
- 实现没有错：「来源提供**或**真实程序要求」已蕴含同一原则，三停格（要求而未提供 → 未决；不要求 → 明确记「未提供」；未登记 → 未决）与监管处置决定那句一致。错的是**归属**：付款人维的这条规则今天只活在票面与注释里，CONTEXT 上没有它——按 AGENTS.md「改领域语言、不变量 → 先改 CONTEXT，再改引用它的 UC / 代码」与「单一权威」，注释引的应是 CONTEXT 里真有的句子。

## 语言从哪里来

- CC `CONTEXT.md` 税费付款核对词条「来源提供或真实程序要求的付款人……维度」——「或」字是这条规则的种子，没有展开成「没提供怎么记、要求而没有怎么停」。
- 同文监管处置决定词条那句——CC owner 若认为两处是同一原则，补句时直接同形；若付款人维另有措辞，写付款人自己的。

## 做法（待裁后写实）

1. CC `CONTEXT.md` 税费付款核对词条或 Rules「税费、放行与案件闭环」段补一句（原词由 CC owner 定，见「要裁的」1），并在 `GLOSSARY` 若有对应词条处同步。
2. 上列各处注释与 `0020` 头注把引文归属改成新句所在词条；[12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md)「裁决」2 原文不改写（历史），在其下追一句「引文归属见 sa-cc/21」。
3. 不改任何行为、不改测试断言；`go build` / `go vet` 零变化即可。

## 红线

- 只补语言不改规则：三停格与 12 裁决 2 的语义一字不动。
- 不动 `internal/**` 的非注释行；不新增 outcome / reason。
- 不引行号、不数别处。

## 完成判据（待裁后写实）

1. CC `CONTEXT.md` 税费付款核对处有那句（或 owner 定的同义句）；`git grep` 该句在 CONTEXT 命中 ≥ 2 处（监管处置决定 + 税费付款核对）或付款人专句命中 1 处。
2. 上列注释与 `0020` 头注的引文归属全部指向新句所在词条；`git diff --stat -- internal/ migrations/` 只含注释行（用 `git diff -w --ignore-blank-lines` 与 `go build` 零变化核）。
3. UC-CC-009（税费付款核对用例）若引到这一维，同步指向 CONTEXT 新句。

## 地盘

`docs/domain/customs-compliance/CONTEXT.md`（一句）、`docs/domain/GLOSSARY.md`（若有）、`docs/application/customs-compliance/UC-CC-009-*`（若引）、`internal/customscompliance/{domain,application,adapters/http}` 注释行、`migrations/customs_compliance/0020_*` 头注、本票面与 12 票面一句。

## 要裁的

1. 那句写成什么、放在哪——归 CC owner：与监管处置决定同形（「未提供或不适用必须明确记录，规则要求但缺失时保持未决」直接复用于付款人维），还是付款人自己的措辞（如「付款人由来源提供或按真实程序登记的规则要求；来源未提供时明确记录为未提供，规则要求而未提供时核对保持未决，规则未登记时核对不进行」）。裁前注释保持现状。

## 参照

[12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md)（裁决 2、完成记录、15:47 补评审 Standards ①）；CC `CONTEXT.md` 税费付款核对与监管处置决定两词条；AGENTS.md「改文档」第一条与「单一权威」红线。

## Comments

- 2026-09-14 16:0x · 通道 1（接管会话）：立票（sa-cc/12 补评审 ← 通道 3 Standards 非阻断 ①：「建议 CC owner 在 CONTEXT 税费付款核对 Rules 补一句再被引（另立文档票，不改代码）」）。只写票面，未动代码与 `docs/**`。能力边界：`git grep` 核过那句在 CONTEXT 只命中监管处置决定一处、在代码命中 `funds_fact_payer.go` / `reconcile_duty_payment.go` / 其测试三处；`0020` 头注与 HTTP 头注两处按评审原文列入，未逐字重读。
