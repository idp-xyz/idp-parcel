# 13 `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY` 版本的运营主路径：逐字段表单（共同通过条件 + 控制项可加行）

Category: enhancement
Status: ready-for-agent——形状已裁清（逐字段表单 + 控制项可加行；本票无待裁问题），读面已随票 [06](./06-pre-acceptance-financial-control-policy-versions-have-no-read-face.md) 进 main（`a1890506` / `ea2293e5`），Blocked by 08 未 resolved 前不在前沿；伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`；接管会话 2026-09-07 于 `92579b0a` 逐句核过票面，见伞票 Comments）
Blocked by: 08

## 册与载荷

显示在**政策页·接受前财务控制策略册**（票 06，第九册）。`declarations.preAcceptanceFinancialControlPolicyBody{
jointPassCondition, controls[{control, chargeScope, order, onFailure, responsibility}]}`（0024 正文，ADR-0115）：
三个封闭集（控制种类两值、失败处置两值、共同通过条件首发一值）、判断顺序版本内唯一且从 1 起、（种类 × 范围）唯一、
至少一项。

## 选形与理由（ADR-0101 决定八）

**逐字段表单，控制项可加行。** 频次低、配置员操作、正文是一格 + 一张几行的表。三个封闭集由服务端词表读口供
下拉——**控制种类下拉里没有「无控制」**，那一格属合同声明（ADR-0115 Decision 一），表单不得在这里长出它。顺序
唯一、范围 × 种类唯一、至少一项，都由构造门答；表单可以在提交前提示重复，但拒绝的话由服务端说。

## 硬句

伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md)「硬句」一节逐字适用（不在此复述，那一节是唯一口径）；另一条本册特有：责任引用与费用范围引用是开放引用（0024 头注），表单收串不校验存在性。

## 完成判据

接受前财务控制策略册旁多一签「发布策略版本」（表单 → 预览摘要 → 待批准 → 批准 → 发布），结果在同册立刻可见
（正文列从「未登记」变「已登记」）；tsc / run-tests 绿；Go 侧只加本册规范化一格。

## 边界

不动 0024；不动 settlement-accounting 的读路径（sa-preacceptance-policy-view/02）。
