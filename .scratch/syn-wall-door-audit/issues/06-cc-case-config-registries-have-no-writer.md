# 关务案件配置面五类登记册只读,无写入方无登记口

Category: enhancement
Status: ready-for-agent

来源:SYN-WALL-DOOR-AUDIT 走通审计(基线 `49a2ab0`),对应清单 W13。

## 墙

`DECLARATION_UNDECIDED`(`customscompliance/application/submit_declaration.go`)、`RESULT_UNDECIDED`(`receive_external_result.go`)——申报就绪、提交授权、解释规则、关闭义务、门禁条件五类配置空册时,案件链各判断停在未决。

## 现状:有装载无写入

- 表:迁移 0006(readiness 与 submission authority)、0007(interpretation 与 case requirement rules)、0008(closure obligation 与 gate conditions)。
- 装载口:五个只读视图齐(`readiness_view.go`、`submission_authority_view.go`、`interpretation_rule_view.go`、`obligation_inventory_view.go`、`gate_condition_view.go`)。
- 写入方与登记口:零(非测试代码无 INSERT;无登记用例;`/customs/external-results` 端点还在接入渠道墙后)。

## 缺的最小机制件

关务配置登记口:版本化登记用例 + 写入方,覆盖五类配置(按法定生效区间与适用时点版本化,PAR-CUS-04 的机制半边);进程级入口。

## 红线

- PAR-CUS-01..07 实例值(真实程序/服务方/渠道/规则源)待提供是常态;本票只建门,验证用脱敏合成配置,S 级只记 S。
- 规则版本不可覆盖;结果代码与层次映射按版本登记,不得写死。

## 参照

PAR-CUS-01..07(尤其 PAR-CUS-04);`docs/design/customs-slice-0-business-development-handoff.md`。

## Comments

- 2026-08-20 · MCP-3：对 3324ecb 重核四件（只读）。**票面与代码零漂移，完全成立。**
  仓储表：在——0006（readiness_judgment、submission_authority）、0007（interpretation_rule、
  case_requirement_rule）、0008（closure_obligation_catalog/item、gate_condition_catalog/
  finding），三份迁移与五个只读视图最后触碰均为 32cc780（2026-08-14），此后无人动过。
  装载口：五视图在。写入方：仍零——上述配置表的 INSERT 只出现在六个 `*_view_test.go`，
  非测试代码零写入。进程级登记口：仍零——`application/` 无登记用例（现有用例都是案件
  链判断侧）。建议：ready-for-agent，票面原样可开工；PAR-CUS-01..07 实例值待提供是
  常态，机制半边不被阻断。
- 2026-08-20 MCP-1：采纳重核，Status → ready-for-agent。实现另派。
- 2026-08-21 · MCP-2：对 `0ec62ea` 重核四件（只读）。**四件结论与票面一致**，另有一处
  票面讲不出的形状问题，见下。
  仓储：在——0006（`readiness_judgment`、`submission_authority`）、0007
  （`interpretation_rule`、`case_requirement_rule`）、0008（`closure_obligation_catalog`
  /`_item`、`gate_condition_catalog`/`_finding`）。目录里已多出 0009
  （`manifest_candidates_and_execution_facts`），与本票五类无关，**下一号是 0010**。
  装载口：五视图在（另有第六个 `case_requirement_view.go`，票面未纳入，见末条）。
  写入方：仍零——上述八张表的 `INSERT` 只出现在六个 `*_view_test.go`，非测试代码零写入。
  登记口：仍零——`application/` 九个用例全在案件链判断侧，无登记用例；五视图在
  `cmd/` 侧也**零装配**（`assemble.go` 只用到 `NewCustomsCases` 与
  `NewDeclarationSubmissions`），故本票改这五个读口的签名不会碰 `assemble.go`。
- 2026-08-21 · MCP-2：**「版本化」在解释规则这一类今天建不出来，本票据此收窄。**
  票面「缺的最小机制件」要的是按法定生效区间与适用时点版本化的登记口。CONTEXT 硬句
  191 把「版本化」定死为三件：*适用辖区*、*法定生效区间*、*规则声明的适用时点*；
  硬句 192 再加一条原判断不得被覆盖。逐类对表核：
  * 关闭义务：`closure_obligation_item` 已带 `applies_from`/`applies_until`，
    `LoadObligationItems` 已按 `cutoffAt` 半开区间解析——**这一类今天就是版本化的**，
    只缺写入方与登记用例。
  * 就绪判断、提交授权：不是规则而是逐单元的判断与授权，本就以形成时点加撤销两列
    表达（撤销不是删除），硬句 191 不适用。缺的同样只有写入方与登记用例。
  * 门禁条件：目录加逐项判断，属案内事实，同上。
  * **解释规则：版本维今天建不出来。** `interpretation_rule` 主键是（租户，结果层），
    一层一行，无辖区、无生效区间、无适用时点三列。
    先记下一条反向证据，免得后来人以为没看见：`ComplianceRuleVersionReference` 的注释
    明写「规则版本必须记录适用辖区、法定生效区间与适用时点（191）——**那些在规则本体
    上，这里引用**」。即本仓既有立场是规则版本化留在规则本体，库里只存引用；照这条
    读，一层一行的「当前指针」册子并不违规，且每条 `ExternalResult` 都存了**实际采用**
    的规则引用，判断历史不会被覆盖。
    但这条立场在**迟到结果**上破：一份为旧提交迟到的外部响应，按当前指针解释就是拿
    「消息到达那一刻的指针」当规则的适用时点，而硬句 191 的后半句正是「案件创建时间、
    消息到达时间或系统当前时间**不能统一替代**规则的法定适用时点」。当前指针册子结构上
    只能做这个替代，没有第二条路。
    要修就得让册子按规则版本存多行并按适用时点解析，`LoadInterpretationRule` 随之要吃
    适用时点、多辖区租户下还要吃辖区；而**这两个入参在外部结果这条路上今天都没有来源**：
    `ReceiveExternalResultCommand` 只有 `Scope`（`DecisionScopeReference`）与
    `OccurredAt`/`ReceivedAt`，案件键上的 `Jurisdiction` 不在这条链上，而把 `OccurredAt`
    直接当法定适用时点又是同一句硬句禁的另一种替代。
  这一格是模型决定（外部结果如何解析规则的法定适用时点与适用辖区），不是本票的写口活，
  **不猜**。本票因此按下述收窄执行，版本维另立票。
- 2026-08-21 · MCP-2：本票（A 票）实际交付范围，与 MCP-1 派票口径「只做写口与仓储
  半边」一致：
  1. 五类配置的写入方（`adapters/postgres`）与登记用例（`application`），登记**一律
     不可覆盖**——已在册的登记交回`已登记`，不做 UPSERT；就绪与授权的撤销走状态推进
     （`UPDATE` 置 `revoked_*`），不删行。
  2. 关闭义务按适用区间登记（这一类的版本化今天成立）。
  3. 解释规则本轮只交付**不可覆盖的单版登记**：重复登记同一 `rule_ref` 幂等，登记不同
     `rule_ref` 交回`冲突`而非覆盖。版本维（辖区/生效区间/适用时点）留给新票。
  4. 不动 `assemble.go` 与 `endpoints.go`；进程级入口属 B 票。
  连带必改一处：`adapters/postgres/view_support_test.go` 的夹具注释现写着「视图这一侧
  没有写口——登记册的内容属实例半边，由带外落入」，本票落地后该句不再成立，随本票改。
- 2026-08-21 · MCP-2：票面未纳入但同族的第六本册子——`case_requirement_rule`（0007）
  与 `case_requirement_view.go`，写入方与登记口同样为零，挡的是 `establish_customs_case`
  一侧的墙，不在 W13 的两堵墙内。本票不扩，照实登记在此。
- 2026-08-21 · MCP-2：上两条按 MCP-1 指示各开一张 needs-triage，免得随本票 resolved
  一起埋进 Comments 里搜不到：
  * 版本维 →
    [cc-interpretation-rule-version-dimension/01](../../cc-interpretation-rule-version-dimension/issues/01-interpretation-rule-has-no-version-dimension.md)
  * 第六本册子 →
    [cc-case-requirement-rule-registry/01](../../cc-case-requirement-rule-registry/issues/01-case-requirement-rule-has-no-writer.md)
- 2026-08-21 · MCP-2：**A 半边交付完毕**，分支 `t1-06-cc-case-config`，已 rebase 到
  `82aa1d3`，头 `2cc7146`。**本票不转 resolved**——B 半边（进程级入口）未做。
  * `171e08b`：五本册子的写入方（`adapters/postgres/case_config_registry.go`）与 23 个
    真库往返用例；`ports` 加五个 `*Registry` 接口与结果代数；domain 加两个
    `Revocation()` 出口。三条纪律各有用例钉住：不可覆盖、撤销走状态推进不走删除、
    目录与明细分开写。
  * `2cc7146`：五类的登记用例（`application/register_case_configuration.go`）与 18 个
    用例。冲突判定落在编排——写口交回`已登记`后读回既有登记逐字段比，同则`已存在`
    异则`内容冲突`。这是不做 UPSERT 的全部收益所在。
  * 交付时发现并顺带钉住的一格：写口走 `RequireExecutor`，**无环境事务即拒绝**，
    回滚不留行。事务由进程级入口给出，属 B 票。
  * 验证：另开 detached worktree 按 `2cc7146` 检出，`gofmt`／`git diff --check`／
    `go build ./...`／`go vet ./...` 四项零信号，`go test -count=1 ./...` 全绿且
    **含真库**（单跑真库用例 `-v` 为 PASS 非 SKIP）。
  * 票面「缺的最小机制件」里「按法定生效区间与适用时点版本化」一句只对关闭义务成立
    （它本就有区间列）；解释规则那一类的版本维建不出来，已收窄为不可覆盖的单版登记，
    另票见上条。
