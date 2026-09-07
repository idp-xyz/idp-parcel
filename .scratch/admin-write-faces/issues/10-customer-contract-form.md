# 10 `CUSTOMER_CONTRACT` 版本的运营主路径：逐字段表单（正文 + 按费用范围的控制约定可加行 + 合同级控制声明）

Category: enhancement
Status: ready-for-agent——形状已裁清（逐字段表单 + 绑定表可加行，两层声明分两节；本票无待裁问题），Blocked by 08 未 resolved 前不在前沿；伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`；接管会话 2026-09-07 于 `92579b0a` 逐句核过票面，见伞票 Comments）
Blocked by: 08

## 册与载荷

显示在**客户与合同页**。版本壳之外，`declarations` 里两格归它：`contractContent{rulePackage, bindings[{chargeScope,
policy | inapplicabilityBasis}]}`（0012 正文：指名接单规则包；按费用范围指名一份接受前财务控制策略**或**给出显式
不适用依据，二者恰一，库上 CHECK）与 `preAcceptanceControl{requirement, notApplicableBasis?}`（0007 合同级
「要不要」声明，依据只在`不适用`时在场）。

## 选形与理由（ADR-0101 决定八）

**逐字段表单，绑定表可加行。** 频次低、配置员操作、正文是几个引用 + 一张几行的约定表，不是矩阵。表单：
规则包引用（从接单规则包册选）、绑定行（费用范围 × 「指名策略 / 显式不适用」二选一：策略从接受前财务控制策略册
选——票 [06](./06-pre-acceptance-financial-control-policy-versions-have-no-read-face.md) 落地后那本册有了）、合同级
控制声明（要求 / 不适用 + 依据）。**恰一与「不适用必带依据」两条由服务端裁**，表单用二选一控件呈现但不代判：
两格都空提交上去，答的是构造门的拒绝，不是表单的静默补齐。

## 硬句

伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md)「硬句」一节逐字适用（不在此复述，那一节是唯一口径）；另两条本册特有：「明确无控制」只能经合同两层声明表达（ADR-0115 Decision 一），表单不得在
策略侧给出「无控制」选项；`preAcceptanceControl` 与 `contractContent.bindings` 是两层（合同级「要不要」与按范围
「用哪份 / 不适用」），表单分两节、不合并。

## 完成判据

客户与合同页多一签「发布合同版本」（表单 → 预览摘要 → 待批准 → 批准 → 发布），结果在同页目录读面（含绑定列）
立刻可见；tsc / run-tests 绿；Go 侧只加本册规范化一格。

## 边界

不动 0007 / 0012；不动闭包解析。
