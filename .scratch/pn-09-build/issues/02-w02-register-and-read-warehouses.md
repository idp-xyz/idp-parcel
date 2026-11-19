# 02 `PN09-W02` 第一部分：仓的登记与查阅

Category: enhancement
Status: resolved（2026-11-19 进 main，见「进 main 记录」）
Blocked by: 01

依据：[UC-WF-001](../../../docs/application/warehouse-fulfillment/UC-WF-001-REGISTER-WAREHOUSES-ZONES-LOCATIONS-AND-PACKAGING-MATERIALS.md)；[仓配履约 CONTEXT](../../../docs/domain/warehouse-fulfillment/CONTEXT.md)「主数据登记」与「身份与边界」；[ADR-0375](../../../docs/adr/0375-warehouse-fulfillment-is-an-optional-capability-on-its-own-pn-09-slice-off-per-deployment-unless-switched-on-and-unconfigured-per-tenant-until-registered.md) 决定四、五；[ADR-0384](../../../docs/adr/0384-warehouse-access-words-customer-faces-and-the-switched-off-answer.md) 决定五、七。

用户 2026-11-18 说「继续」。分工同票 01：拆包、任务说明与验收由主会话做，代码与测试由 Sonnet 5.5 子代理写。

## 为什么只做仓这一册

`PN09-W02` 是五册。本票只做仓，把整条路走通：新上下文的包、库表、登记口、查阅口、开关关着时的答法。库区、库位类型、库位与包材照这条路再加，另成一票。受控批量口、演示种子、管理台页面与货主查仓也各自另成票。

## 会话代决（可推翻）

1. **开关关着时端点的答法，另立一格。** `403`，问题详情，码 `WAREHOUSE_FULFILLMENT_NOT_ENABLED`，不带 `outcome`。在认证之前答，对任何调用方、任何请求内容都是同一份。理由：恢复动作是部署方打开开关，不是去配接入渠道；沿用「接入渠道未配置」会把人指到错的地方。它只加在仓配的端点上，别的端点一行不变。
2. **结果的字面量。** `REGISTERED`、`ALREADY_REGISTERED`、`CONTENT_CONFLICT`、`NOT_ACCEPTED`。`NOT_ACCEPTED` 另带原因：`REVISION_OUT_OF_SEQUENCE`（回显在册最新修订号，没有在册的回 0）、`IMMUTABLE_FIELD`（指出哪一格）、`CONTENT_INVALID`（指出哪一格）。后面四册还要加 `REFERENCE_NOT_FOUND`、`REFERENCE_DEACTIVATED` 与 `NOT_CONFIGURED`，结果的形状要留得下它们，本票不实现。
3. **仓的种类字面量。** `SELF_OPERATED`（自营仓）、`AGENT`（代理仓）。集外的词是坏载荷，不是未受理。
4. **库表。** schema 名 `warehouse_fulfillment`，进同一份迁移计划。开关关着的部署也施加，表是空的。不带任何默认行。

## 仓这一册的规则

以 UC-WF-001 与 CONTEXT 为准，这里只列实现要守的：

- 身份是（租户，仓编码）。仓编码逐字保存。
- 每笔修订是一份完整内容：名称、种类、地址（国家码与地址正文）、业务时区（可缺）、同址物流节点（可缺，逐字保存，不向 `network-routing` 核对）、是否停用。停用与再启用都是新修订。
- 判定次序：先按（身份，修订号）找在册的那一笔，有且内容相同答 `ALREADY_REGISTERED`，有而内容不同答 `CONTENT_CONFLICT`。都不是，再核修订号等于在册最新加一（首笔是 1），核种类没有改，核国家码在内置目录、业务时区认得出。
- 重放与冲突只比内容，不比登记责任方与登记时刻。
- 业务时区不给就是没登记。不取租户、部署或地址所在国的默认值。
- 每笔修订记下登记责任方（信封里的操作者）与登记时刻。
- 并发提交同一修订号的两笔，至多一笔落册，另一笔按内容答重放或冲突。
- 查阅：当前修订的列表（可按是否停用筛），与一个仓的修订史。空册答空列表，不答未配置。
- 日志与指标只带仓编码，不带地址正文。
- 登记口挂登记册配置写面，不判准入范围。查阅口挂主数据与运营查阅读面。

## 分两段交

1. **内半**：`internal/warehousefulfillment` 的领域、端口、应用层，迁移，PostgreSQL 适配器。不接 HTTP，不动 `cmd/`。
2. **外半**：登记报文的译装、HTTP 端点、接入身份的消费侧适配器、`cmd/parcel-api` 的装配与开关关着时的答法、两种开关状态的装配用例。

两段都做完、评审过、全量真库测试通过，才进 main。

## 验收

对应 UC-WF-001 里只涉及仓的那一部分：`AT-WF-001`、`AT-WF-002`、`AT-WF-004`（国家）、`AT-WF-005`、`AT-WF-006`、`AT-WF-009`（两种查阅答空；登记仓不以已有仓为前提）、`AT-WF-010`、`AT-WF-012`（端点那一半）。用例头注写 `Covers: AT-WF-…（仓的部分）`。

每张实施票的两条：开关关着时其他上下文的答复不变；本票不消费、也不交出任何信封。

## 不在本票

- 库区、库位类型、库位、包材。
- 受控批量口与登记 CLI；演示种子；管理台页面。
- 货主的客户系统查阅仓（要先有客户面能力 `WAREHOUSE_READ`）。
- 停用对入库预报、建账导入的作用（那些用例还没有实现）。

## 开关关着时的答法：选定与指针

本票是第一张落仓配端点的票，照 [ADR-0384](../../../docs/adr/0384-warehouse-access-words-customer-faces-and-the-switched-off-answer.md) 决定七选定「另立一格」：`403`，RFC 9457 问题详情，码 `WAREHOUSE_FULFILLMENT_NOT_ENABLED`，不带 `outcome`。这是会话代决，可以推翻。

它碰到的两处既有记录：

- [ADR-0055](../../../docs/adr/0055-business-endpoint-intake-has-an-unconfigured-grade.md) 决定三「状态码取 `403`，不取 404、401 或任何 5xx」。「接入渠道未配置」那一格的恢复动作不变。仓配的行在开关关着时不经任何 Intake，答旁边新立的这一格，同样 403，同样不是 404。开着时它们照旧从「接入渠道未配置」起步。
- [ADR-0222](../../../docs/adr/0222-composition-roots-register-endpoints-and-routes-per-feature-file.md) 闭合用例那句「只有声明了那一面的行答复离开「接入渠道未配置」」。登记处的 `endpointLine` 多一格 `warehouseFulfillment`。装配测试只对带这一格的行、且只在开关关着时，认 `WAREHOUSE_FULFILLMENT_NOT_ENABLED`。别的行的断言不变。

两处例外，都照既有端点的做法：方法不对仍答 `405` 并带 `Allow`；读口的请求目标里有 NUL 或非法 UTF-8，登记处在处理器之前答 `400`。

## 评审记录

作者是 Sonnet 5.5 子代理，分内外两半交。评审是主会话，钉分支 `cursor/wf-w02-warehouse` 的 `10b92a259`。一轮，没有阻断。

- 读过的：登记用例、领域类型、迁移、报文译装、登记端点、开关关着的处理器、接入身份的登记写适配器、`cmd/parcel-api` 的特性文件与装配、共享件的全部改动。没有逐行读的：PostgreSQL 适配器、两个读端点与各测试文件。
- 评审自己重跑：`gofmt -l .` 为空，`go vet ./...`、`go build ./...`、分片核对退 0；仓配各包、`internal/architecture`、`cmd/parcel-api` 整包通过。
- 仓配的包只导入 `accessidentity` 与 `platform`，位置在 `adapters/accessidentity`。
- 作者报告做过变异检查（`build` 忽略开关、`mount` 忽略开关、登记行声明错面），对应用例都变红。评审没有重做。

不挡的几处，照收：

- 同一个端点上两种问题体。开关关着那一格是 RFC 9457。信封各格、`400`、`405`、`500` 沿用操作者两个面在别的上下文的旧形状 `{"error":{"code"}}`，因为装配处的请求目标检查按字节比旧形状。这两个面整族迁到问题详情时一起迁。
- 装配测试的未接线占位是开关开着的那一份。关着的那一格由生产装配的真库用例认。
- 业务时区认不认得出取决于运行环境有没有时区库。仓库里没有内嵌时区库，别的上下文同样如此。镜像里没有时区库时，非空的业务时区都会答内容不合。
- 登记时刻取应用时钟，多实例有时钟偏差时不保证随修订号单调。
- 列表按仓编码排用数据库默认排序规则，不一定等于字节序。

## 进 main 记录

- 出处：分支 `cursor/wf-w02-warehouse` 的 `10b92a259`。main 在这期间多了别的会话的五笔，所以重放：七笔 cherry-pick 到 `7edbd1f50` 之上，无冲突，内容在 `c61bdfba4`。收口是随后这一笔。
- 全量真库测试：`scripts/test-full.sh --detach` 对 `c61bdfba4` 跑，`FULL-EXIT=0`。
- 演示种子没有灌：本票不带种子，开关默认关。库表多一个空的 `warehouse_fulfillment` schema。
- 生成物不重算。
