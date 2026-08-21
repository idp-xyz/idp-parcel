# ADR-0070: 关务规则登记册分记录侧与选择侧；解释规则的选择侧违反硬句 191，案件要求规则的版本维随同一模型决定裁

Status: Proposed  
Date: 2026-08-21

## Context

[SYN-WALL-DOOR-AUDIT 票 06](../../.scratch/syn-wall-door-audit/issues/06-cc-case-config-registries-have-no-writer.md)（清单 W13）执行中分出两笔欠账，各自立票且都停在 `needs-triage`：

- [解释规则登记册没有版本维](../../.scratch/cc-interpretation-rule-version-dimension/issues/01-interpretation-rule-has-no-version-dimension.md)——`interpretation_rule` 主键只有（租户，结果层），迟到的外部响应结构上只能拿消息到达时刻的当前指针当法定适用时点。
- [`case_requirement_rule` 有读口无写口](../../.scratch/cc-case-requirement-rule-registry/issues/01-case-requirement-rule-has-no-writer.md)——同批迁移的第六本册子，写入方与登记口同为零，挡的是 `EstablishCaseUndecided`。

第二票把「硬句 191 适不适用于案件要求规则」列为待 triage 的一格，并写明「与解释规则版本维那票是同一个模型问题的两个实例，两票一并裁比分开裁省事」「本票不预判」。本记录裁这一格。

裁断卡在一条反向立场上，两票都点名要求先处理它。`internal/customscompliance/domain/compliance_judgment.go` 的 `ComplianceRuleVersionReference` 注释写：

> 规则版本必须记录适用辖区、法定生效区间与适用时点（191）——那些在规则本体上，这里引用。

照这条读，一层一行的指针册子并不违规：判断历史里存了实际采用的规则引用，覆盖不了。若这条成立，两票都该关。

### 裁断前重核的四件事实

裁断输入按代码重核，不据既有票面转述：

| 核的是什么 | 实测 |
|---|---|
| 外部结果是否存下所采用的解释规则 | 存——`receive_external_result.go` 把 `LoadInterpretationRule` 的结果作 `Rule` 传入 `InterpretExternalResult`，`ExternalResult.Rule()` 读得回 |
| 解释规则的选择口有没有时间维 | 没有——`ports.InterpretationRuleView.LoadInterpretationRule(ctx, tenant, layer)` 只有两维 |
| 案件是否存下所采用的要求规则 | 不存——`domain.CustomsCase` 的字段是身份、四维监管范围、包裹、角色、建立时点，无规则位 |
| 要求判断的依据落在哪一格 | 只落「不适用」一格——`establish_customs_case.go` 全文只有 `CaseNotRequired` 那一处给 `basis` 赋值，其余各格留空 |

CONTEXT 自身分两句说这件事，这是本记录全部结论的地基。「外部结果与规则时效」一节的记录侧那句：

> 每项外部结果必须保存来源身份、来源在该结果层的权威角色、原始业务语义、业务发生或适用时间、接收时间、实际采用的解释规则及其与提交版本、提交尝试和明确结果范围的关系。

同节的选择侧那句（即硬句 191）：

> 关务规则版本必须记录适用辖区、法定生效区间和规则声明的适用时点。案件创建时间、消息到达时间或系统当前时间不能统一替代规则的法定适用时点。

## Decision

**一、关务规则登记册分记录侧与选择侧两个问题，硬句 191 只管选择侧。**

- **记录侧**问的是「这次判断实际采用了哪个规则版本」，答案随判断本体存下，事后追得回。
- **选择侧**问的是「在某个法定适用时点上，该用哪个规则版本」，答案由登记册的形状决定。

上引记录侧那句管前者，硬句 191 管后者。`ComplianceRuleVersionReference` 那条注释是**记录侧的真话**：三个维度确在规则本体上，判断里存引用即已履行记录侧义务。但它答的不是 191 问的问题——把一个只能答「当前指针」的册子接上去，记录侧照样满，选择侧照样破。**该注释不构成任何登记册在选择侧的许可**，两票不得据它关闭。

**二、解释规则登记册在选择侧违反硬句 191，但今天修不了，缺的是模型决定不是写口。**

`interpretation_rule` 是纯选择侧的册子：一层一行的当前指针，`LoadInterpretationRule` 无适用时点入参。一份为旧提交迟到的响应，按定义只能取到达那一刻的指针——正是 191 后半句点名的三种替代之一，且结构上没有第二条路。记录侧的完好（`ExternalResult.Rule()` 存得住）不抵消这一条：存下的那个引用本身就是按错误时点选出来的。

修它要给 `LoadInterpretationRule` 补两个入参，两者今天都无来源：**规则的法定适用时点**（`ReceiveExternalResultCommand` 只有 `OccurredAt` 与 `ReceivedAt`，拿任一个顶上又撞同一句硬句）与**多辖区租户下的适用辖区**（该命令只有 `Scope`，辖区在 `ports.CustomsCaseKey` 上，不在外部结果这条链上）。外部结果凭什么定出规则的法定适用时点与适用辖区，是**模型决定**；未确认参数保持显式未决，本记录不猜。

**三、案件要求规则的记录侧今天不受任何硬句约束，照实记，不补。**

`CustomsCase` 不存所采用的要求规则，`Basis()` 只在「不适用」格给出——建案成功时没有任何地方记下是哪条规则要求建的案。这看着与记录侧那句同构，**但 CONTEXT 没有对建案下过这条义务**：「案件身份与申报范围」一节只要求「关务案件建立时必须固定监管辖区、进出口方向、监管程序和法定义务范围」，未列规则依据；记录侧的义务句分别落在外部结果与关闭依据项（后者要求「保存义务身份、来源责任方、采用事实或规则版本」）上，都不覆盖建案。

因此**今天的形状不是违规**，本记录不据「与外部结果同构」补一条 CONTEXT 没写的义务。要不要给建案加规则依据留痕，是独立的建模提议，走它自己的记录。

**四、案件要求规则的选择侧问题与实例二同因，但不阻塞该票的写口。**

`case_requirement_rule` 同为关务规则，主键已带 `jurisdiction_ref`（191 三维之一已在），缺的是法定生效区间与适用时点。它是否发作，取决于建案是否会为过去的法律行为迟到发生——而这一问与实例二缺的是同一个模型决定：一次判断凭什么定出规则的法定适用时点。**该问随实例二一并悬置，不单独裁。**

关键在于它**不阻塞票 02 的正题**。那票要的是写口，属机制半边：`INSERT ... ON CONFLICT DO NOTHING`、同键已在册交回`已登记`、内容比对交给编排，加上已经定死不得重开的那条分界——`found=false` 是「规则未登记」不是「不要求」，`basis` 列 `NOT NULL` 是它的守门人。这些一条也不依赖版本维的裁决。写口先做，版本维明确留在票外。

## Consequences

- 两票据本记录脱离 `needs-triage`：解释规则版本维那票转 `ready-for-human`（缺的是模型决定，不是可交给 agent 的规格）；`case_requirement_rule` 写口那票转 `ready-for-agent`，版本维一格显式移出其范围。
- `ComplianceRuleVersionReference` 那段注释**本次不改**。它引 191 却描述记录侧，读起来像给选择侧发了通行证，是本记录认定的缺陷；但 Proposed 尚非依据，据草案改代码是抢跑。**本记录被接受之日，该注释须收窄为记录侧陈述**，否则同一条规则在仓里留两份互相打架的口径，撞 [AGENTS.md](../../AGENTS.md) 的单一权威。此项不得随接受动作遗忘。
- 记录侧完好会持续制造「看着合规」的假象：`ExternalResult` 每条都存着规则引用，追溯查得到、对得上，唯独查不出当时该用的是不是这一条。选择侧的欠账在数据上不留痕迹，只能靠本记录留痕。
- 发作时点不变，仍是两票所记的那个：多辖区租户出现那天，或第一次规则换版撞上一份迟到响应。在那之前既有数据每条都按当前指针取，事后追不回法定适用时点——欠账只会越欠越贵。
- 本记录不重开 W13 的收窄。解释规则的不可覆盖单版登记随 W13 交付；关闭义务、就绪判断、提交授权、门禁条件四类不在本记录范围内，理由见两票各自的「本票不做的事」。

## Alternatives considered

- **据 `ComplianceRuleVersionReference` 的立场关掉两票。** 否决：该立场是记录侧的真话，答的不是 191 问的问题。按它关票，等于用「判断历史存了引用」证明「选出的引用是对的」，而后者恰是迟到响应上唯一破掉的那一环。
- **两个实例合并裁一个结论。** 否决：合并只在选择侧成立，记录侧两者根本不同——解释规则的记录侧完好，案件要求规则的记录侧空白且不受硬句约束。合并会把「无义务」误升为「有义务未履行」，凭空造出一条 CONTEXT 没写的规则。
- **现在就给 `interpretation_rule` 加生效区间与适用时点两列，入参先拿 `OccurredAt` 顶上。** 否决：`OccurredAt` 是业务发生时间，不是规则的法定适用时点，顶上去正是 191 后半句所禁；换来的是一个看着版本化、实则把同一个替代埋深一层的册子，比今天更难发现。
- **把案件要求规则的版本维并入其写口票一起做。** 否决：写口属机制半边今天就能交付，版本维卡在模型决定上无期；并进去等于让一件做得成的事陪一件做不成的事一起停，与票 02 立票时「W13 不扩，照实另立」的同一条理由。
- **给 `CustomsCase` 补规则依据留痕，与外部结果对齐。** 否决：CONTEXT 对建案没下这条义务，本记录裁的是既有两票，不顺手扩模型。要补另走记录，那是提议不是补漏。

## Links

- [customs-compliance CONTEXT](../domain/customs-compliance/CONTEXT.md)：「外部结果与规则时效」一节记录侧与选择侧两句的出处；「案件身份与申报范围」一节建案要素的出处
- [UC-CC-001](../application/customs-compliance/UC-CC-001-ESTABLISH-CUSTOMS-CASE.md)：建案裁决分格，「不适用」格带依据即 `Basis()` 的来处
- 两票：[解释规则版本维](../../.scratch/cc-interpretation-rule-version-dimension/issues/01-interpretation-rule-has-no-version-dimension.md)、[`case_requirement_rule` 写口](../../.scratch/cc-case-requirement-rule-registry/issues/01-case-requirement-rule-has-no-writer.md)
- 来处：[SYN-WALL-DOOR-AUDIT 票 06](../../.scratch/syn-wall-door-audit/issues/06-cc-case-config-registries-have-no-writer.md)（清单 W13）
