# 接受前财务控制策略版本：能发布、能被合同引用，正文表未建——「控制怎么做」今天无处落

Category: enhancement
Status: resolved——四问由 [ADR-0115](../../../docs/adr/0115-pre-acceptance-financial-control-policy-content-is-a-row-per-control-and-no-control-stays-with-the-contract.md) 一次答完，正文表 + 领域对象 + 登记口在分支 `mcp3-pcgaps07` 四笔落地（`359b10bd`..`2939d2d9`，基线 `0f84c0ec`；2026-09-07，MCP-3，owner 经通道 1 派工授权自决；完成记录在文末，含验证强度）；SA 读路径不动，后继 [sa-preacceptance-policy-view/02](../../sa-preacceptance-policy-view/issues/02-load-control-policy-reads-policy-content-items.md)（draft）
Blocked by: 无

## 从哪里来

[admin-write-faces/06](../../admin-write-faces/issues/06-pre-acceptance-financial-control-policy-versions-have-no-read-face.md)
问「这一类版本管理台看不见」，取证（report.md A 组，锚 `08e62ec`）答的是比读面更深的一层：
`CommercialObjectKind` 第 5 类 `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY` 在库里**只有 `commercial_version` 壳**，
`migrations/party_commercial/` 无它的正文表；结算侧 `settlementaccounting/adapters/partycommercial/pre_acceptance_control_policy.go`
的 `LoadControlPolicy` 读的是商业闭合 + 合同级声明（`0007`），不读任何策略版本正文。MCP-3 2026-09-04 裁：读面不列壳
（列壳无信息量，票 06 完成判据第二条自己说的），**正文表是缺口本体**，另立本票；票 06 转 blocked 等它。

## CONTEXT 说这一类版本带什么

词条原句（[PC CONTEXT](../../../docs/domain/party-commercial/CONTEXT.md)「接受前财务控制策略」）：

> 客户合同版本针对明确业务范围规定的接单前财务控制依据。策略可以要求预付冻结、信用校验、明确接受前无财务控制，或者
> 合同明确规定且相互不冲突的控制组合；它定义**适用范围、共同通过条件和失败处置**，但不拥有实际估价、余额、冻结或
> 信用暴露结果。

Rules 两句：

> 接受前财务控制策略不得硬编码为永久互斥的三选一枚举。合同要求组合控制时，策略必须明确**每项控制的适用范围、判断顺序、
> 共同通过条件和失败或补偿责任**；某一范围明确无控制不能静默取消其他范围已经规定的控制。

> 每个用于新委托接受的客户合同版本必须明确引用适用接单规则包和接受前财务控制策略；确实不适用的控制必须按明确范围记录
> 不适用依据。规则或策略缺失不得被解释为允许接受。

这几句把正文的**结构**说死了（控制项集合、每项的适用范围、判断顺序、共同通过条件、失败/补偿责任），一格都不依赖租户
取值；取值（哪几项、阈值、账户与价格依据、失败处置的具体动作）是 `PAR-COM-15` 的最低证据列，待提供。**与 ADR-0104
「客户服务规则正文」是同一形**：CONTEXT 有语言、代码只有壳、形状由硬句推得出——机制半边现在就做，不留空。

## 今天两层各答什么，别合并

| 层 | 表 | 答的问题 | 拥有对象 |
|---|---|---|---|
| 合同级声明 | `0007_pre_acceptance_control_declaration` | 这份合同的这个范围**要不要**控制；说`不适用`时凭什么 | 客户合同版本（`object_kind=2`，ADR-0042 归属纪律） |
| 策略版本正文 | **缺** | **控制怎么做**：哪几项、顺序、共同通过、失败处置 | 接受前财务控制策略版本（第 5 类） |

`0007` 头注自己写着「策略版本回答『控制怎么做』，回答不了『这份合同要不要』」以及「本表不存控制方式」——所以今天
「控制方式」实际上是结算侧从结算政策的预付/账期方式**推**出来的（ADR-0044 那条解析），而 `pn-02-w03` 早写过
「账期不能推导无需信用校验」。正文表落地后 SA 的 `LoadControlPolicy` 该不该改读正文，是另一票（SA 地盘），本票不碰。

## 要裁的（`/domain-modeling` 一次答完）

1. **控制项的封闭集**：CONTEXT 点名三种（预付冻结、信用校验、明确无控制）+「不冲突的组合」。封闭集里要不要给「明确
   无控制」留一格——`0007` 的 `NOT_APPLICABLE` 已经在合同层答了「不控制」，策略层再答一次是两处口径；倾向策略层只
   表达**要做的控制项**，「无控制」由合同声明独占。
2. **组合的表达**：每项控制一行子表（项、适用范围引用、判断顺序号、失败/补偿责任引用）+ 父表一格「共同通过条件」；
   还是整份正文一个结构化文档。倾向子表——「判断顺序」「某一范围明确无控制不能静默取消其他范围」这两句要逐项可查。
3. **适用范围与失败处置的取值形态**：机制只给槽（引用 + 封闭的处置种类），取值由 `PAR-COM-15` 供；不为任何一格拟默认。
4. **要不要 ADR**：改的是商业对象正文形状、连带 SA 解析路径的下一步——倾向要，形照 ADR-0104。

## 红线

- 不填 `PAR-COM-15` 任何实例值；不给未声明的合同一个默认控制。
- 不合并两层：合同声明表与策略正文表分开，`0007` 不动。
- 新迁移按序号新开（`party_commercial` 当前最大序号以开工那刻重取），不改已施加迁移。
- SA 读路径的改动不在本票。

## 完成判据

裁决落 CONTEXT（必要时 ADR 编号落进上面某一条）→ 正文表 + 领域正文对象 + 登记口（形照票 03 两张正文表的落法）→
[admin-write-faces/06](../../admin-write-faces/issues/06-pre-acceptance-financial-control-policy-versions-have-no-read-face.md)
据此解阻。`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库。

## 完成记录（2026-09-07，MCP-3 接管会话；分支 `mcp3-pcgaps07`，基线 `0f84c0ec`，不推——MCP-1 重放进 main）

旧会话 11:15–11:35 落下四笔后中断，没留完成记录、没发完工报、没记验证强度；12:03 点名后由本会话接管——先封存它在 awf/06 上的在途现场，再在干净检出上补验证，然后写本记录。四笔代码 + 本收口笔，每笔 pathspec 提交：

| 笔 | SHA | 内容 |
|---|---|---|
| A | `359b10bd` | [ADR-0115](../../../docs/adr/0115-pre-acceptance-financial-control-policy-content-is-a-row-per-control-and-no-control-stays-with-the-contract.md) 四问一次答完；`docs/adr/README.md` 索引一行；PC CONTEXT「接受前财务控制策略」词条补两句（正文只表达要执行的控制项、「明确无控制」独由合同声明；失败处置只答去向不拥有拒绝决定）；本票转 in-progress |
| B | `4e5fc6a6` | 迁移 `migrations/party_commercial/0024_pre_acceptance_financial_control_policy.sql` 父子两表——父行持 `joint_pass_condition`，子行 种类 × 费用范围 × 判断顺序 × 失败处置 × 责任引用；主键含（种类, 范围）、UNIQUE 含顺序、CHECK 钉三个封闭集与正顺序、`object_kind` = 5、外键回 `commercial_version`。领域 `PreAcceptanceFinancialControlPolicy` / `PreAcceptanceControlItem` 与三个封闭集 `PreAcceptanceControlKind`（`PREPAID_FREEZE` / `CREDIT_CHECK`，没有「无控制」）、`ControlFailureDisposition`（`REJECT` / `AUTHORIZED_DISPOSITION`）、`JointPassCondition`（`ALL_CONTROLS_PASS` 一值）；构造门守类别、已生效、至少一项、顺序唯一、（种类 × 范围）唯一。端口 `PublicationRegistry.SavePreAcceptanceFinancialControlPolicy` + `PreAcceptanceFinancialControlPolicySaveOutcome`、`PreAcceptanceFinancialControlPolicyContentView`；postgres 写口先读回再写（同内容重放 / 异内容冲突且一行不写）、读口父子一条语句取回过构造门（有父无子走 error，不折成未登记）；发布用例 `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY_BODY` 声明通道本体；三处 `PublicationRegistry` 替身跟随 |
| C | `d2d375ac` | 发布用例：正文随自己那一版发布、控制项按判断顺序原样到达持久化面、挂在客户合同版本上整项拒绝一行不写、零项拒绝、内容冲突折进报告不是 error；受控 CLI 批文 `preAcceptanceFinancialControlPolicyBody { jointPassCondition, controls[] }` 一节（控制项 `control × chargeScope × order × onFailure × responsibility`；三个封闭集集外拒收含 `NO_CONTROL`、共同通过条件缺席不折成全部通过、未知字段拒收）；真库端到端——经进程口发出去的正文按闭包选中的版本壳经点读口读回、两项按顺序俱在 |
| D | `2939d2d9` | 机制清点在分支干净检出重生成：partycommercial 生产 84→86 / 测试 84→86、端口 27→28、迁移 23→24 |
| — | `8dc99b9f` | **不属本票**：awf/06 在途现场的封存笔（`chore(salvage)`，非集成候选，仅防丢）；重放本票时跳过它 |

**四问的答**（正文在 ADR-0115，此处只对号）：① 封闭集只有要执行的控制项，「明确无控制」独由合同两层声明（`0007` / `0012`），Decision 一；② 子表逐行 + 父行共同通过条件，Decision 二；③ 槽位——范围 `ChargeScopeReference` 开放引用、顺序正整数版本内唯一、处置两值、责任开放引用、共同通过条件一值必填，任何一格不拟默认，Decision 三；④ 要 ADR，即 0115。

**验收对照**（票面「完成判据」逐条）：裁决落 CONTEXT ✓（A）；正文表 + 领域正文对象 + 登记口 ✓（B / C，形照票 03 的 `0020` / `0021` 与票 05 的 `0023`：迁移 + 领域 + 具名 Save + 点读口 + 发布通道 + CLI 批文；在线发布口 `/commercial-publications` 消费同一 UC-PC-001 命令，本通道随之可达，Intake 仍是未配置）；awf/06 据此解阻 ✓（本票 resolved 那一刻其 Blocked by 解除，读面那一格是它自己的活，在同一分支上接着做）。红线逐条：`PAR-COM-15` 零实例值、`0007` / `0012` 未动、迁移新开 `0024` 未改已施加迁移、SA 未动。

**验证强度**（本会话在干净 detached 检出 `2939d2d9` 上首次记录）：`gofmt -l .` 零输出；`go build ./...`、`go vet ./...` 退 0；**含 DSN**（门禁容器 `127.0.0.1:55432`）`go test -p 1 -count=1 ./...` 99 包 ok / 0 FAIL / 退 0。探针一正一反 `go test ./internal/partycommercial/adapters/postgres/ -run ControlPolicy -count=1 -v`：无 DSN `--- SKIP` 6 / `--- PASS` 0；有 DSN `--- SKIP` 0 / `--- PASS` 6（六例：组合往返、缺正文 found=false、重放与改项冲突且原行不动、租户绑定、有父无子 error、CHECK 拒集外三集 / 零顺序 / 空白 / 同键与同序第二行）。未跑 `-race`（本机走不了，见 workflow.md 本机环境）。机制清点：`2939d2d9` 已提交版与 `8dc99b9f` 干净检出重生成零差。

**要 MCP-1 落的装配行**：无。写口走既有 `/commercial-publications` 端点；不需要新端点行、不需要 `main.go` 改动。

**解析不改**：`ResolveCommercialClosure` 对 `PreAcceptanceFinancialControlPolicyObject` 一视同仁，零改动（Decision 五）。

**SA 后继票**：[sa-preacceptance-policy-view/02](../../sa-preacceptance-policy-view/issues/02-load-control-policy-reads-policy-content-items.md)（draft，SA 地盘）。`LoadControlPolicy` 的`要求`格今天仍从闭包里已采用的结算政策取方式（`policyFrom` → `adoptedSettlementPolicy` → `NewRequiredControlPolicy(method, adoptedPolicy)`），不读本票落的正文；SA 结果形状「一种方式 + 一份采用政策」装不下多项控制，要先在 SA 侧建模。落地前接受前控制链在生产上的行为一字不变（Decision 五）。

**父 spec**：`party-commercial-context-gaps/spec.md` 状态行不由本票改，完工对齐归 MCP-1。

## Comments

- 2026-09-07 · MCP-3（接管会话）：收口。四笔是旧会话的产出，本会话对 ADR-0115 六条 Decision 逐条核过再写完成记录；验证强度是本会话补测的，不是转述。
- 2026-09-04 · MCP-3：立票。起因是裁 admin-write-faces/06 时发现「只有壳」不是长期事实——CONTEXT 明写策略定义适用范围、
  共同通过条件与失败处置，壳是机制半边的缺口。**只写票面，未动代码。** 能力边界：读过 PC CONTEXT 词条与 Rules 相关句、
  `0007` 迁移全文、admin-write-faces/06 票面与 report.md A 组取证；**没读** `pre_acceptance_control_policy.go` 全文与
  ADR-0044/0054/0079 正文，上表「结算侧从结算政策方式推控制方式」一句是按 `0007` 头注与 report.md 转述推的，建模时以代码为准。
