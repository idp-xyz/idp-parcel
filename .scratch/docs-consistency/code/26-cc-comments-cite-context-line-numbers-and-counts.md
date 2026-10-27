# 26 CC 代码注释用行号引 CONTEXT，CLI 包头注带计数与参数区间

Category: chore
Status: in-progress——完工待评审与进 main，分支 claude-cmtA
Blocked by: 无
出处：[audit/cc.md](../audit/cc.md) M11、L4 的代码一半，与 M5 改 CONTEXT 的连带（审计钉 `6b0074d0`；2026-10-21 按 main `205eb254` 复核，下列各处原样仍在）。

## 现象

1. **行号引 CONTEXT**（M11）。CC 领域包的注释写着 2026-08 时 CC CONTEXT 的行号，今天全部指错（审计按 `6b0074d0` 逐一核过，没有一处是注释本意）：
   - `domain/compliance_judgment.go`：`ComplianceJudgment` 头注「自动与人工一视同仁（159）」，`Supersede` 头注「谁都删不掉谁（159）」。本意是 CONTEXT Rules「参与方、资料与合规判断」一节「自动与人工合规判断都必须保存适用规则版本、事实依据、决定方式和责任角色。人工判断不能删除自动判断」一句。
   - `domain/follow_up_target.go`：`ReplacementRelation` 头注「……已被有效替代（175）」。本意是 Rules「申报就绪、授权与提交」一节「拟替代目标、撤销请求或本地处理决定不得把原申报改成已撤销、已作废或已被有效替代」一句。
   - `domain/external_result.go`：`SourceAuthorityRole` 头注「……才能形成监管事实（186）」、`ExternalResult` 头注「……改变不了原提交的业务判断（188）」。本意分别是 Rules「外部结果与规则时效」一节「技术中介或报关服务商的响应只有在来源语义明确代表监管机构时，才能形成相应监管事实」与「查询请求、查询尝试、查询执行状态和查询技术结果不得直接改变原提交的业务判断」两句。
   - `domain/case_closure.go`：`ClosureObligationItem` 头注「……都不能证明责任已经移交（219）」，`CustomsCaseClosure` 头注「单个案件不存在部分关闭（218）……重开保留原关闭记录（233）」。本意是 Rules「税费、放行与案件闭环」一节「发送交接、技术送达、任务创建、计划后续处理、部分接受或默认超时接受都不能证明全部责任已经移交」「单个案件不存在部分关闭」两句与 Lifecycles「关务案件」一节「原关闭记录和关闭期间事实继续保留」一句。
2. **引文随 CONTEXT 改写而过期**（审计 cc M5 改 CONTEXT 的连带）。文档自洽 03 把 CC CONTEXT Lifecycles「申报单元、就绪与提交」那一条从「只有两者在逻辑提交首次实际发送时均有效，才形成新的提交版本和初始提交尝试」改为「只有两者在固定提交版本时均有效，才形成新的提交版本和`已登记待发送`的初始提交尝试」（ADR-0227 决定四：版本在登记时固定，此时没有发送）。原句的引文还在：`domain/declaration_submission.go`（「CONTEXT「只有两者在逻辑提交首次实际发送时均有效，才形成新的提交版本」」）、`application/submit_declaration_test.go` 的 `TestReadinessAndAuthorityAreTwoSeparateTracks` 与 `domain/declaration_submission_test.go` 两条的 `Covers:` 行。行为不变（代码本来就在固定版本那一刻核），只换引文。
3. **计数与参数区间**（L4 的代码一半）。`cmd/parcel-customs-register/main.go` 包头注「关务案件配置面五本登记册的受控进程入口」「配置内容属实例半边（PAR-CUS-01..07 待提供）」：命令表 `translate.go` 的 `allCommands` 今天远不止五本（含口岸、申报路径、凭证、税费协作、付款核对、节点区域等），`node-zone` 子命令对应的是 `PAR-CUS-08`，不在区间里。

## 证据

- AGENTS.md「改文档」：「引另一个文件的某句或某段 → 优先用符号名，无符号可指时用引文，一律**不用行号，也不用计数**」，「**Go 注释里的跨文件引用同受此约束**」。
- ADR-0070 的同类坐标在其 Status 行挂了「引用更正」（文档自洽 03），那是 ADR，正文不改；代码注释可以直接改。

## 三类

机制（注释）。无产品策略、无租户取值。

## 规模

S：只改注释，`go build`、`go vet` 不受影响。

## 最小的诚实修法

- 上列各处的「（NNN）」换成 CONTEXT 小节名加句首引文（如「CONTEXT「单个案件不存在部分关闭」」）。修完在 `internal/customscompliance` 与 `cmd/parcel-customs-register` 下用 `grep -nE '（[0-9]{3}）'` 再扫一遍，只剩 HTTP 状态码一类的括注。
- 「引文随 CONTEXT 改写而过期」那几处换成 CONTEXT 新句；`TestReadinessAndAuthorityAreTwoSeparateTracks` 的 `Covers:` 同一笔补点名 `AT-CC-448`（UC-CC-005，ADR-0243 新立，AT 盘点已按这条测试入①）。
- CLI 包头注去掉「五本」，「一处列全」已指向 `allCommands`；参数区间改为指名参数登记册「关务」一节，不写编号区间。

## 附带登记：提交授权没有到期时间（文档自洽 03 第二轮评审）

不是注释问题，按协调方安排登在这里，免得缺口无处可查；实施时宜挪进授权业务角色面的余项票 [product-strategy-boundary/36](../../product-strategy-boundary/issues/36-submission-authorization-business-role-face-remainder.md)。

- 现象：`domain.SubmissionAuthorization.Effective()` 就是 `revokedAt.IsZero()`，CC 领域没有到期字段。UC-CC-004「提交授权不再有效」要求「明确有效期、撤销决定或其他适用授权自身条件在使用前触发」，UC-CC-005 `AT-CC-110` 要求「已到期」答门禁阻断；首发只有撤销一格（UC-CC-005 首发结果码对照、`AT-CC-110` 已注明未实现）。
- 三类：机制（授权带可选的有效截止、`Effective` 按时点判）；有效期取值是租户取值（`PAR-CUS-03`）；没给有效期的授权只随撤销失效，产品不替租户填一个默认期限。
- 规模：S–M（领域一格、授权册迁移另取号、提交门禁按提交时点判）。

## 完成记录（分支 claude-cmtA）

只改注释与测试说明文字，不改任何可执行语句、标识符、字符串字面量。`git diff -U0` 里 Go 文件的非注释行变动为零；`gofmt -l` 无输出；`go build ./...`、`go vet` 通过；`scripts/test-full.sh ./internal/transportfulfillment/... ./internal/customscompliance/... ./internal/visibilityexception/... ./internal/architecture/...` 全绿（三张票同在分支 claude-cmtA，一次跑完，38 个包 ok，无 FAIL）。

改了 `internal/customscompliance` 与 `cmd/parcel-customs-register` 共 33 个文件、52 处，按类：

1. **行号引 CONTEXT**（票面全部七处，仍在原位）：`compliance_judgment.go`（`ComplianceJudgment`、`Supersede`）、`follow_up_target.go`（`ReplacementRelation`）、`external_result.go`（`SourceAuthorityRole`、`ExternalResult`）、`case_closure.go`（`ClosureObligationItem`、`CustomsCaseClosure`）的「（NNN）」换成 Rules 小节名加逐字原句：「自动与人工合规判断都必须保存适用规则版本、事实依据、决定方式和责任角色」「人工判断不能删除自动判断」「拟替代目标、撤销请求或本地处理决定不得把原申报改成已撤销、已作废或已被有效替代」「技术中介或报关服务商的响应只有在来源语义明确代表监管机构时，才能形成相应监管事实」「查询请求、查询尝试、查询执行状态和查询技术结果不得直接改变原提交的业务判断」「发送交接、技术送达、任务创建、计划后续处理、部分接受或默认超时接受都不能证明全部责任已经移交」「单个案件不存在部分关闭」，以及 Lifecycles「关务案件」一节「原关闭记录和关闭期间事实继续保留」。`ExternalResult` 头注里的「八件」一并去掉（计数）。修完用 `grep -nE '（[0-9]{3}）'` 在两处目录再扫，只剩 HTTP 状态码括注。
2. **引文随 CONTEXT 改写而过期**：`domain/declaration_submission.go`、`domain/declaration_submission_test.go` 两条 `Covers:`、`application/submit_declaration_test.go` 的 `TestReadinessAndAuthorityAreTwoSeparateTracks` 由「在逻辑提交首次实际发送时均有效」改为现句「只有两者在固定提交版本时均有效，才形成新的提交版本」；`TestReadinessAndAuthorityAreTwoSeparateTracks` 的 `Covers:` 同一笔补点名 `AT-CC-448`（UC-CC-005，「unconfigured readiness is undecided」那一格）。
3. **CLI 包头注**：`cmd/parcel-customs-register/main.go` 去掉「五本」，「一处列全」仍指向 `allCommands`；「PAR-CUS-01..07」改指试点参数与证据登记册「三、关务、渠道与外部集成」一节，不写编号区间（`node-zone` 对应 `PAR-CUS-08` 本来就在区间外）。同文件 `buildRegistrar` 的「八本册子」、`translate.go` 命令表注释里的「前九个」「九个方法」「第六本」「再两个」「再三个」、`translate_test.go` 的「全部十个」、`main_test.go` 的「五本册子」「第六本册子」一并去掉计数。
4. **同类残留（票未点名）**：
   - 计数/序数：「五本登记册」「第六本册子」「三本册子」「九个只读视图」等换成「案件配置各册」「建案要求规则册」等名称，共约 25 处，分布在 `application/register_case_configuration.go`、`register_case_requirement_rule(.go|_test.go)`、`ports/ports.go`、`adapters/postgres/case_config_registry.go`、`case_requirement_registry(.go|_test.go)`、`case_register_catalogue.go`、`parcel_declaration_facts_view.go`、`view_support_test.go`、`adapters/http/query_case_registers.go`。
   - 位置词：「上一条用例」「下一条用例」「上一例」「上一条」六处改指测试函数名。
   - 不是原文的引文两处：`duty_release_test.go` 的「放行可以针对全部、部分或附条件范围形成」改为 CONTEXT 原句，`duty_release.go` 的「CONTEXT 明禁『实现为一组互斥总状态』」改为原句「不能实现为一组互斥总状态」。
   - 变更叙述：`reconcile_duty_payment.go`（「此前 application/ 里没有……」）、`record_credential_gate.go`（「此前只判不记」）、`case_config_registry.go`（「此前只有读没有写」）、`view_support_test.go`（「原先的理由……不再成立」）、`compliance_judgment.go`（「此前没有出口」）、`payload_canonicalization.go`、`register_credential_test.go`（「原先逐字段比的」）改成现状陈述，共 8 处。

### 已漂移或已不存在的位置

票上列的位置全部仍在，没有已被后来的票顺手改掉的。

### 代决

- `case_closure.go` 一处原句缩写（「发送交接、技术送达、部分接受或默认超时接受都不能证明责任已经移交」）改为 CONTEXT 全句，保证逐字。
- 带 `…` 的引文各段分别逐字存在即保留。
- `internal/customscompliance/adapters/postgres/duty_payment_reconciliation(.go|_test.go)` 里「三张表」「三个端口」描述的是迁移 0016 一次建的具体表，不是登记册计数，没动；若干处「今天没有……」是现状陈述（没有消费方、没有写入方），不是变更叙述，没动。
- 「附带登记：提交授权没有到期时间」不是注释问题，本票不碰，仍待 product-strategy-boundary/36。

### 越权风险点

无。
