# 12 `ACCEPTANCE_RULE_PACKAGE` 版本的运营主路径：分节表单——正文一节、每条声明通道各一节，空节即未声明

Category: enhancement
Status: ready-for-agent——形状已裁清（分节逐字段表单，空节即未声明；伞票点名的「先答声明随发布怎么在表单里表达」在本票「选形与理由」答，无待裁问题），Blocked by 08 未 resolved 前不在前沿；伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`；接管会话 2026-09-07 于 `92579b0a` 逐句核过票面并改正 pc-gaps/09、10 两票落地后各加什么，见伞票 Comments）
Blocked by: 08

## 册与载荷

显示在**政策页·接单规则包册**。这是十类里声明通道最多的一类，`declarations` 里归它的有：`rulePackageBody{serviceProduct,
contract, legalEntity, scope, effective…, rules[{category, reference}]}`（0014 正文）、`asOfPolicies[{judgment, semantics,
policyVersion}]`（0005 时点锚）、`acceptanceContent{applicableGroups[], manualReview}`（接单规则正文声明）、
`pendingRoutingBasis`、`intakeQualification{sources[], qualifications[]}`（0013 收寄资格）、`finalRules[{outcome, finalKind}]`
（0013 终局规则）。pc-gaps/09（终局规则上的有效期声明——随 `FinalRuleChannel` 同一通道多一项正文，**不是**新通道）落地后
`finalRules` 一节多一格；pc-gaps/10（资料修订允许声明——0013 两族之外的第三族阶段内容声明，新通道）落地后多一节。

## 选形与理由（ADR-0101 决定八）

**分节的逐字段表单，不走模板导入。** 频次低、配置员操作；载荷虽宽但每一节都是「几格 + 一张几行的表」，没有一节
是矩阵——模板导入是为上百格的价卡设计的（ADR-0101 Alternatives 第二条），拿它装几行声明是把工具用错对象。

**声明随发布怎么表达**（伞票要先答的那一格）：一版规则包的发布是**一次**提交，正文与全部声明在同一份载荷里、同一
个摘要下——这是 ADR-0042/0058 归属纪律与「声明只能随发布登记」的落法，表单不改它。所以表单是一份、分节：正文
一节 + 每条声明通道一节；**某一节整节留空 = 该通道未声明**（消费方照旧译 `NotDeclared` / `未配置`），表单不给
任何一节默认值、不把「留空」写成「无」。每一节内的封闭集（判断类型、来源、终局种类等）由服务端词表读口供
下拉，表单不内置。pc-gaps/10 的新通道落地时是**加一节**、pc-gaps/09 落地时是终局规则节**加一格**，都不改本票的形。

## 硬句

伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md)「硬句」一节逐字适用（不在此复述，那一节是唯一口径）；另一条本册特有：`manualReview` 那一格答的是「要不要人工复核」（接单规则正文），与
`ManualReviewRequirementFor`「谁有权」那一问是两件（wiring-baseline-remainder/04），表单只收前者。

## 完成判据

政策页接单规则包册旁多一签「发布规则包版本」（分节表单 → 预览摘要 → 待批准 → 批准 → 发布），结果在同册立刻
可见、各节声明在册上各自的列里可见；tsc / run-tests 绿；Go 侧只加本册规范化一格（覆盖正文与全部声明通道）。

## 边界

不动任何声明表；不改「声明只能随发布」；pc-gaps/09 的新格与 pc-gaps/10 的新节在那两票落地后由它们自己加。
