# UC-CC-003 步 7「记录凭证门禁」只判不记：`JudgeCredentialApplicability` 算得出四格，没有任何编排把它落成就绪判断里的一格

Category: enhancement
Status: draft——2026-09-10 通道 4 立票（task-9880bbc9），只写票面未动代码；取证锚 `3f485e97`
Blocked by: 无

## 缺口（取证于 `3f485e97`）

- `internal/customscompliance/application/judge_credential_applicability.go` 头注原句：「本用例只判、不记：步 7 写的『记录凭证门禁』那半留给就绪判断的编排——今天就绪判断是带依据引用登记进来的事实（`RegisterReadiness` 收 `ReadinessBasisReference`），UC-CC-003 步 3–10 没有逐门禁计算的编排，凭证门禁作为其中一格的持久化随那条编排一起落」。
- `git grep -n JudgeCredentialApplicability -- cmd/` 零：判断口没有生产调用方。
- mech/07「没做、留给后继票的」第 1 条：「UC-CC-003 步 7『记录凭证门禁』的持久化——等就绪判断的逐门禁编排。」

## 语言从哪里来

- UC-CC-003 步 7 行：「核验监管凭证身份、适用性、有效期和截至当前的可用依据 → 记录凭证门禁；不占用、释放或核销」（`CC-RULE`、`CC-LIFE`）；`AT-CC-056`：「凭证门禁满足并保存适用性和截至时点；本用例不占用或核销额度」。
- CC `CONTEXT.md`：本上下文拥有「监管凭证及其适用性和使用关系」；「放行门禁核对必须绑定当前有效的监管程序……门禁满足不生成放行」。

## 做法

1. 先答「要裁的」第 1 条——凭证门禁是就绪判断的**一格**（随 `RegisterReadiness` 的依据引用一并落、就绪判断本身仍是登记进来的事实）还是一条**独立记录**（`credential_gate` 登记册，就绪判断按引用指向它）。
2. 无论哪种：记录带凭证身份 / 版本 / 判断结论四格之一 / 截至时点 / 依据引用；同键同内容重放 `已存在`，换内容新版本不覆盖（与 CC 既有登记册代数一致）。
3. 新迁移序号（`migrations/customs_compliance/` 当前最大 `0017`，开工时重取）；postgres 适配器 + 真库用例。
4. `JudgeCredentialApplicability` 从此有生产调用方；`AT-CC-056` 有代码实现。

## 红线

- 只记门禁判断，不占用、不释放、不核销（那三件是 UC-CC-005 步 7/9、UC-CC-006 步 7 的凭证使用生命周期，时点由真实程序定 `PAR-CUS-04`）。
- 「未登记」与「不适用」两格不得压成一格（头注理由：租户上线前每一次判断都会读成「凭证不适用」）。
- 真实凭证与真实程序属实例半边，不写默认。

## 完成判据

1. `git grep -w JudgeCredentialApplicability -- 'internal/*.go' ':(exclude)*_test.go'` 有 application 层以外的调用（编排或装配）。
2. 应用层：四格各一条落库路径；重放 / 换内容两格。
3. 真库往返；`AT-CC-056` 用例点名 Covers。
4. 基线不加宽；清点 tip 重生成。

## 地盘

`internal/customscompliance/application/`（新编排文件或 `register_readiness` 相邻处，按裁决）、`internal/customscompliance/ports/`、`internal/customscompliance/adapters/postgres/`、`migrations/customs_compliance/`（新序号）。不动 `parcel-api` 端点表（登记面归 [07](07-cc-credential-and-duty-reconciliation-registration-faces.md)）。

## 要裁的

1. **一格还是一册**：凭证门禁随就绪判断的依据引用落（就绪判断仍是登记进来的事实，只多一维），还是独立登记册由就绪判断引用——UC-CC-003 步 3–10 今天没有逐门禁计算编排，选后者等于先立第一册；选前者要动 `RegisterReadiness` 的形。归 CC owner。
2. **谁触发判断**：就绪评估请求到达时算一次，还是凭证登记 / 程序变更时重算——头注写「续办是由凭证责任流程形成有效依据后重新评估（UC-CC-003 门禁表第 4 行）」，触发面本票倾向「评估请求到达时」。归 CC owner。

## 参照

[mech/07](../../mechanism-executor-triage/issues/07-cc-four-executors-behind-existing-uc-steps.md) CC-a 与「没做」第 1 条；UC-CC-003；[remaining-work-dd5ed934.md](../../unresolved-review-20260904/remaining-work-dd5ed934.md) 五-8 ①。

## Comments

- 2026-09-10 · 通道 4：立票。未动代码。
