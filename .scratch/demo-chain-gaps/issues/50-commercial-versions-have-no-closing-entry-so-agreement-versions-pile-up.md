# 50 商业版本没有收尾写口：同一供应商协议的旧版无法替代或退役，新旧两版并存在册

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 作者子代理立（票 [29](./29-supplier-agreements-declare-charge-occurrence-conditions.md) 完成记录「偏离与待裁」，用户「你作为业务和系统专家自决……解决所有的问题」授权）。是否认领、归哪一位 owner 待分诊。
Blocked by: 无
地盘（建议）：PC `application` 发布用例或新用例、`registrationjson` 批文、`postgres` 版本册；可能需要新 ADR。
出处：[ADR-0209](../../../docs/adr/0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md) 越权风险点 5；PC CONTEXT「供应商商业协议版本」生命周期「协议版本到期、终止或被替代后……」。

## 现象

演示租户的干线协议以 `v2` 重发（声明收费发生条件）之后，`v1` 仍是`已生效`、区间与 `v2` 相同。领域上有 `CommercialVersion.SupersededBy` 与 `Retire`，但发布批、在线发布口与任何登记 CLI 都没有调它们的写口——生产上没有一个商业版本能被收尾（2026-10-19 在 `claude-dcg29` 上 `grep -rn '\.Retire(\|\.SupersededBy('` 只命中 PC 参与方关系与 VE 处置请求，没有商业版本）。

## 影响

- 运输履约按 `v1` 登记发生项答`未配置`（v1 没声明条件），登记方要自己知道该指名 `v2`。
- 真实租户换版时，旧版会一直`已生效`；按版本点读的消费方（委托提交、发生项登记）照旧接受旧版，换版不收口。
- 按范围解析的册（合同、政策）若出现同样的新旧并存，解析答`适用冲突`（票 30 的改判就是为了躲这一格）。

## 三类

机制（版本收尾写口）与产品策略（替代是否必须同对象、是否自动以新版生效时点收尾旧版）。哪一版何时收尾是租户取值。

## 完成判据（草拟，待分诊）

- 有一个受控入口能把一个商业版本以指名后继替代（`SUPERSEDED`）或按决定引用退役（`RETIRED`），写入版本册并留历史。
- 运输履约核对口对已替代 / 已退役的版本答未生效 / 已终止（适配器已有这两格）。
- 演示租户的干线 `v1` 由 `v2` 替代。
