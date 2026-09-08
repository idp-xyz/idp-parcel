# 11 `SUPPLIER_AGREEMENT` 版本的运营主路径：逐字段表单（正文四格 + 采购方案从价卡目录选）

Category: enhancement
Status: in-progress——2026-09-08 14:2x MCP-6 认领（task-662822ef；分支 `mcp6-awf11`，基 main `0ef63897`；08 已 resolved，阻塞边解除）。此前 ready-for-agent——形状已裁清（逐字段表单，采购方案从价卡目录选；本票无待裁问题），Blocked by 08 未 resolved 前不在前沿；伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`；接管会话 2026-09-07 于 `92579b0a` 逐句核过票面，见伞票 Comments）
Blocked by: 08

## 册与载荷

显示在**供应商协议页**。`declarations.supplierAgreementBody{supplier, legalEntity, scope, purchasePlan,
effectiveStartsAt, effectiveEndsAt?}`（0021 正文，票 pc-gaps/03）。

## 选形与理由（ADR-0101 决定八）

**逐字段表单。** 频次低、配置员操作、正文六格无子表。`purchasePlan` 是 parcel-pricing 的方案版本引用——表单要能
从价卡目录**选**而不是手抄（跨上下文只传引用；价卡目录读口已在管理台价卡页），选出来的仍是引用串，表单不读方案
内容。供应商与法人引用从主数据读面选。

## 硬句

伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md)「硬句」一节逐字适用（不在此复述，那一节是唯一口径）；本册无特有硬句。

## 完成判据

供应商协议页多一签「发布协议版本」（表单 → 预览摘要 → 待批准 → 批准 → 发布），结果在同页目录读面立刻可见；
tsc / run-tests 绿；Go 侧只加本册规范化一格。

## 边界

不动 0021；采购方案的方向与绑定换算是 `PRICE_RULE` 那册的事，不在这里出现。
