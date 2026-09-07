# 16 `CREDIT_POLICY` 版本的运营主路径：逐字段表单（额度「金额 / 比例」二选一）——建议作公共半边的首例

Category: enhancement
Status: blocked——伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`）；等公共半边，且票 08 建议拿本册做规范化摘要的首例——那样两票同批落
Blocked by: 08

## 册与载荷

显示在**政策页·信用政策册**（第七册，pc-gaps/03）。`declarations.creditPolicyBody{legalEntity, authorityLevel, chargeType,
limitMinor | limitRatioBasisPoints, effective…}`（0020 正文）：额度两键恰一（库上 CHECK；读面 `creditLimitCell` 已把
「两键都缺」点名为坏响应）。

## 选形与理由（ADR-0101 决定八）

**逐字段表单，额度二选一控件。** 频次低、配置员操作、正文五格无子表——十册里最小的一类，正因如此票 08 建议先在它
身上把「服务端按册规范化 + 摘要 + 待批准载体 + 预览」整条路走通。二选一控件呈现「金额 / 比例」，**恰一由服务端裁**：
两格都填或都空提交上去，答的是构造门的拒绝。零金额是登记方说出的「授予零信用」，表单不得把 0 折成缺席（读面同一
判据）。

## 硬句

伞票四条逐字适用；另一条本册特有：`CreditBasis` 的消费缝今天不存在（wiring-baseline-remainder/03），本票只管发布，
不替 SA 接。

## 完成判据

信用政策册旁多一签「发布信用政策版本」（表单 → 预览摘要 → 待批准 → 批准 → 发布），结果在同册立刻可见（额度列
按金额或比例恰一上列）；tsc / run-tests 绿；Go 侧本册规范化一格（若为首例，则连同 08 的机制同批）。

## 边界

不动 0020；不动 `ResolveCreditPolicy`。
