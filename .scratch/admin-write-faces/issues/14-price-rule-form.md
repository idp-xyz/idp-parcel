# 14 `PRICE_RULE` 版本的运营主路径：逐字段表单（方向 × 方案绑定从价卡目录选 + 口径条件节）

Category: enhancement
Status: in-progress——2026-09-08 19:4x 通道 2 认领（通道 1 派单 task-7afbd72a；分支 `mcp2-awf14`，基线 main `5a209f70`，隔离树 `D:/tops/idp-parcel-mcp2-awf14`）。此前 ready-for-agent——形状已裁清（逐字段表单 + 口径作条件节，显隐是呈现不是裁门；本票无待裁问题），Blocked by 08 未 resolved 前不在前沿（08 已于 2026-09-08 resolved，边解除）；伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`；接管会话 2026-09-07 于 `92579b0a` 逐句核过票面，见伞票 Comments）
Blocked by: 08

## 册与载荷

显示在**政策页·商业价格政策册**（册名 `PRICE_POLICY`，发布类别 `PRICE_RULE`——两条分类轴，票 03 已在册名旁说明）。
`declarations.pricePolicyBody{direction, pricingPlan, planDirection, conversion, scope, effective…, caliber?{taxDisposition,
taxClassification?, volumetricFactor?, fx?{quoteType, asOfSemantics, asOfPolicyVersion}}}`（0010 正文 + 0022 口径）。
口径各格的在场规则钉在库上 CHECK：税务分类只在含税 / 未税时在场、体积口径只在销售方向在场、汇率三格同在同缺。

## 选形与理由（ADR-0101 决定八）

**逐字段表单，口径作条件节。** 频次低、配置员操作、正文十来格无子表。`pricingPlan` 从价卡目录**选**（跨上下文
只传引用，与票 11 同一句）；`planDirection` 与 `conversion` 是发布当时保全的答复与声明（ADR-0057），表单如实收、
不从方案反推。口径节按 `taxDisposition` / `direction` 显隐条件字段——**显隐是呈现，不是裁门**：显了没填、隐了却
传了，都由服务端按 CHECK 同形的构造门答。

## 硬句

伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md)「硬句」一节逐字适用（不在此复述，那一节是唯一口径）；另一条本册特有：口径随正文**同笔**登记是发布编排的纪律（`PricePolicyRow.HasCaliber` 注释），
表单提交的是一份载荷；不给「先发正文、回头补口径」的两步。

## 完成判据

商业价格政策册旁多一签「发布价格政策版本」（表单 → 预览摘要 → 待批准 → 批准 → 发布），结果在同册立刻可见
（含口径列）；tsc / run-tests 绿；Go 侧只加本册规范化一格（覆盖正文与口径）。

## 边界

不动 0010 / 0022；不读方案内容。
