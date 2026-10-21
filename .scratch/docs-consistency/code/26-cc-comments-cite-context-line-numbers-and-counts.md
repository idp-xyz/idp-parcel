# 26 CC 代码注释用行号引 CONTEXT，CLI 包头注带计数与参数区间

Category: chore
Status: ready-for-agent——R3（不急）：只是注释，不改行为；读注释的作者会被行号引到不相干的 CONTEXT 句子上。2026-10-21 文档自洽 03 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc03。
Blocked by: 无
出处：[audit/cc.md](../audit/cc.md) M11、L4 的代码一半（审计钉 `6b0074d0`；2026-10-21 按 main `205eb254` 复核，下列各处原样仍在）。

## 现象

1. **行号引 CONTEXT**（M11）。CC 领域包的注释写着 2026-08 时 CC CONTEXT 的行号，今天全部指错（审计按 `6b0074d0` 逐一核过，没有一处是注释本意）：
   - `domain/compliance_judgment.go`：`ComplianceJudgment` 头注「自动与人工一视同仁（159）」，`Supersede` 头注「谁都删不掉谁（159）」。本意是 CONTEXT Rules「参与方、资料与合规判断」一节「自动与人工合规判断都必须保存适用规则版本、事实依据、决定方式和责任角色。人工判断不能删除自动判断」一句。
   - `domain/follow_up_target.go`：`ReplacementRelation` 头注「……已被有效替代（175）」。本意是 Rules「申报就绪、授权与提交」一节「拟替代目标、撤销请求或本地处理决定不得把原申报改成已撤销、已作废或已被有效替代」一句。
   - `domain/external_result.go`：`SourceAuthorityRole` 头注「……才能形成监管事实（186）」、`ExternalResult` 头注「……改变不了原提交的业务判断（188）」。本意分别是 Rules「外部结果与规则时效」一节「技术中介或报关服务商的响应只有在来源语义明确代表监管机构时，才能形成相应监管事实」与「查询请求、查询尝试、查询执行状态和查询技术结果不得直接改变原提交的业务判断」两句。
   - `domain/case_closure.go`：`ClosureObligationItem` 头注「……都不能证明责任已经移交（219）」，`CustomsCaseClosure` 头注「单个案件不存在部分关闭（218）……重开保留原关闭记录（233）」。本意是 Rules「税费、放行与案件闭环」一节「发送交接、技术送达、任务创建、计划后续处理、部分接受或默认超时接受都不能证明全部责任已经移交」「单个案件不存在部分关闭」两句与 Lifecycles「关务案件」一节「原关闭记录和关闭期间事实继续保留」一句。
2. **计数与参数区间**（L4 的代码一半）。`cmd/parcel-customs-register/main.go` 包头注「关务案件配置面五本登记册的受控进程入口」「配置内容属实例半边（PAR-CUS-01..07 待提供）」：命令表 `translate.go` 的 `allCommands` 今天远不止五本（含口岸、申报路径、凭证、税费协作、付款核对、节点区域等），`node-zone` 子命令对应的是 `PAR-CUS-08`，不在区间里。

## 证据

- AGENTS.md「改文档」：「引另一个文件的某句或某段 → 优先用符号名，无符号可指时用引文，一律**不用行号，也不用计数**」，「**Go 注释里的跨文件引用同受此约束**」。
- ADR-0070 的同类坐标在其 Status 行挂了「引用更正」（文档自洽 03），那是 ADR，正文不改；代码注释可以直接改。

## 三类

机制（注释）。无产品策略、无租户取值。

## 规模

S：只改注释，`go build`、`go vet` 不受影响。

## 最小的诚实修法

- 上列各处的「（NNN）」换成 CONTEXT 小节名加句首引文（如「CONTEXT「单个案件不存在部分关闭」」）。修完在 `internal/customscompliance` 与 `cmd/parcel-customs-register` 下用 `grep -nE '（[0-9]{3}）'` 再扫一遍，只剩 HTTP 状态码一类的括注。
- CLI 包头注去掉「五本」，「一处列全」已指向 `allCommands`；参数区间改为指名参数登记册「关务」一节，不写编号区间。
