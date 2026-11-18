# 02 `PN09-W02` 第一部分：仓的登记与查阅

Category: enhancement
Status: in-progress（2026-11-18 认领，分支 `cursor/wf-w02-warehouse`）
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
