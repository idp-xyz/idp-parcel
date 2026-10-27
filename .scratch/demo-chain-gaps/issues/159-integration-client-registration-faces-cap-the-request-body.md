# 159 集成客户端族的资金事实与凭证登记口不设请求体读取上限

Category: bug
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R3）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-21 由票 140 的隔离评审立
Blocked by: 无
出处：票 [demo-chain-gaps/140](./140-operators-adopt-external-funds-facts-and-regulatory-credentials.md) 「评审记录」第 8 条

## 现象（读码）

`internal/settlementaccounting/adapters/http/integration_client_intake.go` 与 `internal/customscompliance/adapters/http/integration_client_intake.go` 的 `IntegrationClientIntake` 在资金事实采用、更正与凭证登记几口上调 `readSinglePayload(request.Body)`，对请求体没有读取上限：一份坏掉或恶意的大请求体会被整份读进内存。CC 的外部结果口（`IntakeResult` 走 `decodeClosedDocument`）同样没有上限。票 140 开的手工口已设一兆字节上限（与 CC 登记写面的 `maxRegistrationBytes` 同值），集成客户端族那几口没有跟上。

持有效客户端凭据才走得到这一步，攻击面限于已登记的外部系统；但外部系统本身出错推来一份超大请求体时，进程照单全收。

## 修法（待分诊）

- 集成客户端族各口照手工口加 `io.LimitReader(request.Body, 上限+1)`，超限即坏报文；上限值与手工口同，或按外部结果的实际体量另定（分诊时定）。
- 每口补一条超限用例：填充放在对象里面，不设上限时能译出命令，设了才被截断。
- 顺带核一下别的接入口（首方客户族、作业事实族）是否同病。

## 三类

机制。规模 S。
