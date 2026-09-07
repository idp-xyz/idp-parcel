# 09 `SERVICE_PRODUCT` 版本的运营主路径：逐字段表单（版本壳 + 引用）

Category: enhancement
Status: ready-for-agent——形状已裁清（逐字段表单，本票无待裁问题），Blocked by 08 未 resolved 前不在前沿；伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`；接管会话 2026-09-07 于 `92579b0a` 逐句核过票面，见伞票 Comments）
Blocked by: 08

## 册与载荷

显示在**服务产品页**。发布载荷只有版本壳：`kind / objectId / version / scope / effectiveStartsAt / effectiveEndsAt? /
references{}`，`declarations` 里没有为它开的正文通道（`translate.go` 的 `declarationsDocument` 十几格里没有服务产品
正文）。服务产品的属性与渠道映射走另一条登记路（`register_products`，票 admin-remainder-mechanism-batch/02），
这里发布的是**商业版本壳**——它给合同、接单规则包、价格政策一个可引用的版本身份。

## 选形与理由（ADR-0101 决定八）

**逐字段表单。** 频次低（一个产品一年几版）、操作者是运营配置员、载荷是几格标识与一个区间——没有矩阵、没有
子表。表单字段：对象标识、版本号、范围引用、有效起止、引用表（键值对可加行，键的词汇由服务端答、表单不代填）。
摘要与批准照 08 的机制来，表单只呈现服务端答的摘要。

## 硬句

伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md)「硬句」一节逐字适用（不在此复述，那一节是唯一口径）；另一条本册特有：**表单不得替操作者拟引用键**——`references` 是开放词汇，键名从哪来、指向什么
由发布用例与领域答，表单给的是「加一行」不是「从这几个里挑」，除非服务端提供了词表读口。

## 完成判据

服务产品页多一签「发布版本」（表单 → 预览摘要 → 存为待批准 → 批准 → 发布），结果在同页的目录读面立刻可见；
tsc / run-tests 绿；Go 侧只在 08 落的机制上加本册的规范化一格。

## 边界

不动 `register_products` 那条登记路；不动服务产品目录读面。
