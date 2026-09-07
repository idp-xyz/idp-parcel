# 15 `SETTLEMENT_POLICY` 版本的运营主路径：逐字段表单（方式 + 法人 / 对手方 / 合同引用 + 范围 + 币种）

Category: enhancement
Status: blocked——伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`）；等公共半边
Blocked by: 08

## 册与载荷

显示在**政策页·结算政策册**。`declarations.settlementPolicyBody{method, legalEntity, counterparty, contract{objectId,
version}, chargeScope, currency, effective…}`（0011 正文）。`method` 是封闭集（预付 / 账期一族），`contract` 是一个
客户合同版本的二维引用。

## 选形与理由（ADR-0101 决定八）

**逐字段表单。** 频次低、配置员操作、七格无子表。`method` 下拉由服务端词表读口供；`contract` 从客户与合同目录
**选**（对象 + 版本两格一起选，不让操作者手拼版本号）；币种收 ISO 代码串、存在性由构造门答。

## 硬句

伞票四条逐字适用；另一条本册特有：结算政策答的是「怎么结」，不答「要不要接受前控制」——那是 0007 / 0024 两层
（pc-gaps/07 完成记录里 SA 今天从结算方式**推**控制方式那条是 SA 侧另一票的事），表单不在这里长出控制字段。

## 完成判据

结算政策册旁多一签「发布结算政策版本」（表单 → 预览摘要 → 待批准 → 批准 → 发布），结果在同册立刻可见；
tsc / run-tests 绿；Go 侧只加本册规范化一格。

## 边界

不动 0011；不动 SA 消费侧。
