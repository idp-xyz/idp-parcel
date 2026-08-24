# 解释规则登记册没有版本维，迟到的外部结果只能拿到达时点当法定适用时点

Category: bug
Status: resolved

[ADR-0070](../../../docs/adr/0070-customs-rule-registries-split-recording-from-selection.md)（草案）已裁本票下方那条反向立场：它是**记录侧**的真话，答的不是硬句 191 问的选择侧问题，因此不构成本册子的许可，本票据以成立。转 `ready-for-human` 而非 `ready-for-agent`：缺的两个入参该从哪来是模型决定，不是可交给 agent 的规格。

从 [SYN-WALL-DOOR-AUDIT 票 06](../../syn-wall-door-audit/issues/06-cc-case-config-registries-have-no-writer.md)（清单 W13）执行中分出。该票「缺的最小机制件」要的是**按法定生效区间与适用时点版本化**的登记口；逐类核表后其余四类都做得出，只有解释规则这一类的版本维今天建不出来——不是写口活没干，是缺两个入参，而它们该从哪来是建模问题。W13 因此收窄为不可覆盖的**单版**登记，版本维留给本票。

## CONTEXT 逐字禁掉的正是今天的做法

`docs/domain/customs-compliance/CONTEXT.md`，「监管规则版本与适用」一节：

> 关务规则版本必须记录适用辖区、法定生效区间和规则声明的适用时点。案件创建时间、消息到达时间或系统当前时间不能统一替代规则的法定适用时点。

后半句逐字禁的就是拿消息到达时间顶上。（代码里的既有简称写作「硬句 191」。）

## 册子今天的形状

`migrations/customs_compliance/0007_interpretation_and_case_requirement_rules.sql`：

```sql
CREATE TABLE customs_compliance.interpretation_rule (
    tenant_id    text NOT NULL,
    result_layer text NOT NULL,
    rule_ref     text NOT NULL,
    CONSTRAINT interpretation_rule_pkey
        PRIMARY KEY (tenant_id, result_layer),
    ...
);
```

一层一行的**当前指针**：适用辖区、法定生效区间、适用时点三列都没有。读口 `LoadInterpretationRule(ctx, tenant, layer)` 的签名同样只有这两维。

## 一条反向证据，先记下免得后来人以为没看见

`internal/customscompliance/domain/compliance_judgment.go` 的 `ComplianceRuleVersionReference` 注释明写：

> 规则版本必须记录适用辖区、法定生效区间与适用时点（191）——那些在规则本体上，这里引用。

即本仓既有立场是**规则版本化留在规则本体，库里只存引用**。照这条读，一层一行的指针册子并不违规：每条 `ExternalResult` 都存了实际采用的规则引用，判断历史不会被覆盖。裁本票时要先处理这条立场，不能当它不存在。

## 这条立场在迟到结果上破

一份为旧提交迟到的外部响应，按当前指针解释，就是拿**消息到达那一刻的指针**当规则的法定适用时点——正是硬句后半句点名的三种替代之一。当前指针册子在结构上只能做这个替代，没有第二条路。

## 要修就缺两个入参，而两者今天都无来源

册子按规则版本存多行、按适用时点解析之后，`LoadInterpretationRule` 要多吃两样：

| 缺的入参 | 今天为什么取不到 |
|---|---|
| 解析用的**评估时点** | ~~`ReceiveExternalResultCommand` 只有 `OccurredAt` 与 `ReceivedAt`；把任一个直接当法定适用时点，又是同一句硬句禁的替代~~ **ADR-0070 更正：本行过宽。** 硬句点名的是案件创建时间、消息到达时间与系统当前时间三者——`ReceivedAt` 在其内，`OccurredAt` 不在，且同节另有一句正面支持按业务发生/适用时间接受外部事实。这一半有据可循，候选与推荐见 ADR-0070「问二」 |
| 多辖区租户下的**适用辖区** | 该命令只有 `Scope`（`DecisionScopeReference`）；辖区在 `ports.CustomsCaseKey` 的 `Jurisdiction` 上，不在外部结果这条链上。回指案件那条路断在[申报单元 → 案件关联那票](../../customs-declaration-case-link/issues/01-declaration-unit-has-no-case-association.md)上，见 ADR-0070「问三」 |

**真正无来源的是辖区那一半，属模型决定，不是写口活**；评估时点经 ADR-0070 重核后有据可循。未确认参数仍保持显式未决，不猜。

## 什么时候发作

单辖区、规则从不换版时不发作。**多辖区租户出现那天，或第一次规则换版撞上一份迟到响应，同时发作。** 那一刻既有数据里每条 `ExternalResult` 存的规则引用都是按当前指针取的，追不回当时的法定适用时点——这是个只会越欠越贵的建模欠账。

## 本票不做的事

- 不重开 W13 的收窄。解释规则的不可覆盖单版登记（同 `rule_ref` 幂等、异 `rule_ref` 交回`冲突`而非覆盖）随 W13 交付，本票只加版本维。
- 不碰其余四类。关闭义务已带 `applies_from`/`applies_until`，`LoadObligationItems` 已按 `cutoffAt` 半开区间解析，本就是版本化的；就绪判断、提交授权是逐单元的判断与授权（以形成时点加撤销两列表达，撤销不是删除），门禁条件是案内事实——硬句 191 对这三类不适用。
- 不替 `customs-compliance` 定所有权。难逆转的取舍按 [AGENTS.md](../../../AGENTS.md) 走 ADR。

## Comments

- 2026-08-24 MCP-1（**受用户委托裁断，转 ready-for-agent**。基准 `88215a3`）：缺的模型决定
  已由 ADR-0070 的接受补齐——用户 2026-08-24 授权代为拍板，三问均采该记录推荐甲，接受记录
  在其文首。对本票意味着：
  1. **评估时点** = `OccurredAt`（业务发生或适用时间），并带不变式：来源未给出或给出不可信
     值时**显式落未决格**，不得用当前指针兜底——否则本票记的缺陷原样藏进新册子。
  2. **适用辖区** = 从外部结果回指案件取 `CustomsCaseKey.Jurisdiction`。拍板时闸门已开：
     ADR-0073 已落申报单元持久化本体（带案件维）与 `CustomsCaseStore.FindByID` 反查读口；
     外部结果 `Scope` → 单元 → 案件这条链的连通细节，实现开工时按当时代码取证。
  3. **登记面**：主键扩为（租户，结果层，适用辖区，法定生效区间起），按半开区间解析；同
     （租户，层，辖区）下区间不重叠，排他约束或写口串行化照 ADR-0056 形状。
  4. `ComplianceRuleVersionReference` 注释已随 ADR-0070 接受同笔收窄为记录侧陈述。

  **实现范围**（按本票正文与上列四条，余者照「本票不做的事」不动）：`interpretation_rule`
  迁移加辖区与法定生效区间维并扩主键；`LoadInterpretationRule` 加评估时点与辖区两个入参；
  `receive_external_result.go` 编排按 1/2 取值并落未决格；W13 的不可覆盖登记语义在多版本
  形状下保持（同全键幂等、同键异 `rule_ref` 冲突）。

- 2026-08-24 MCP-5（实现落地，转 resolved）：四个面同笔交付，全按上条实现范围。

  1. **登记面**（迁移 `0011_interpretation_rule_version_dimension.sql`）：主键扩为
     （租户，结果层，适用辖区，法定生效区间起）；`applies_until` 可空为「尚无终点」；
     同支不重叠由 `EXCLUDE USING gist`（btree_gist，受信扩展）交付——首条登记没有可锁
     的既有行，ADR-0056 的行锁形状在这一格接不住，故取问一甲给的另一条路。既有单版旧
     行**拒迁不代填**：辖区与区间无从回填，迁移遇行即 RAISE 并指引经登记 CLI 重登。
  2. **写口**：终点不是登记输入——每版以开放区间进册，登记更晚起点的新版时前版终点
     随之落定（与就绪/授权撤销同款状态推进，`rule_ref`/`applies_from` 无改写路径）。
     换版由此可走，正是 ADR-0070 支点第 3 步「今天做不成」的那一步。撞键与撞重叠都折
     `已登记`，读回比对在编排：同规则重放`已存在`、异规则或按请求起点读不回（错序/
     追改历史）`内容冲突`。登记按生效起点升序进行。
  3. **选择面**：`LoadInterpretationRule(ctx, tenant, layer, jurisdiction, evaluatedAt)`
     按半开区间解析（与 `LoadObligationItems` 同口径）；空辖区/零时点/词表外层一律
     响亮报错不折「未配置」。
  4. **接收编排**：评估时点 = `OccurredAt`，不变式落为「零值或晚于 `ReceivedAt` 即
     `EVALUATION_INSTANT_UNTRUSTED` 未决格」（后半是因果判据：业务发生不可能晚于接收，
     问二甲「不可信值」的最小可操作定义）；适用辖区走范围→单元（`DeclarationUnitStore.
     FindByID`）→案件（`CustomsCaseStore.FindByID`）→ `Jurisdiction()`，链上依赖故障落
     `CASE_CHAIN_UNAVAILABLE`、走不通落 `JURISDICTION_UNRESOLVED`，未决原因封闭集合
     4→7。`ReceiveExternalResultDeps` 新增 `Units`/`Cases` 两依赖（装配方注意）。
     登记 CLI `interpretation-rule` 命令加 `jurisdictionRef`/`appliesFrom` 必填格。

  **验证**（2026-08-24，本机门禁容器健康）：换版真库用例单跑 `-v` PASS（非 SKIP）；
  `go test ./internal/customscompliance/... ./cmd/parcel-customs-register/... -count=1`
  全绿（postgres 套件 35.8s，真库实跑）；全仓 `go test ./... -count=1` 全绿——树上当时
  混有 MCP-3 未提交的 parcelshipment 改动，提交态另在临时 worktree 复验。发作场景
  （规则换版撞迟到响应）已由真库用例正面钉住：旧区间时点解析回旧版。
