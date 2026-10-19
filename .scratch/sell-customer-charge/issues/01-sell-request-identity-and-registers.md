# 01 结算：SELL 评价请求身份、包裹计费时点采用册与费用项目映射册

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。
Blocked by: 无
地盘：`internal/settlementaccounting` 的 domain、ports、application（只登记那几只）、adapters/postgres、adapters/registrationjson、adapters/identity；`migrations/settlement_accounting/` 0037–0039；`cmd/parcel-settlement-register`；`internal/architecture` 里分区主体、标识前缀两本登记的对应行。
出处：[ADR-0190](../../../docs/adr/0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 决定一、三、五（映射册那一半）。

## 做什么

1. **SELL 评价请求**（决定一）：领域 `SellEvaluationRequest`。
   - 铸造标识，新前缀，过标识前缀门禁。自然键（租户、包裹、计费时点）。
   - 字段：租户、包裹、结算账户、结算币种（登记时从不可变的结算账户行快照）、计价范围、计费时点、来源事实引用、计价基准时点。
   - 册 `sell_evaluation_request`（迁移 0037；号被占就取下一个空号），状态格留给形成那一半写：已提交、已形成、待判断、不可计价、冲突；已形成时另记那笔预估客户费用的标识。
   - 端口：登记（同自然键答已存在）、按标识读、按包裹读当前请求。
   - Outbox 交接：信封 `settlement-accounting.sell-evaluation-request.submitted`，载荷只有（租户、请求标识），ID 走 `outboxintent.FingerprintEventID`，分区键（租户/包裹）。
2. **SELL 计费时点采用册**（决定三，迁移 0038）：键（租户、结算账户），值（计费时点、计价范围），不可变元组，照受理前估价采用册的先例。计费时点是封闭词：`NETWORK_INTAKE_ADOPTED`、`EFFECTIVE_DELIVERY_FINALIZED`。
3. **费用项目映射册**（决定五，迁移 0039）：键（租户、计价费用代码），值（费用项目、登记依据），只增不改。
4. 两册的登记用例、批文译装与 `parcel-settlement-register` 两条命令（`sell-billing-moment-adoption`、`fee-item-mapping`）。没登记的读答未找到，不给默认。

## 不做

- 不发起请求、不消费任何信封（票 02、04）。不碰计价。

## 完成判据

- 真库用例：三册的登记、幂等、租户隔离、无事务拒（架构门禁要的负向证据）。
- 登记口的译装与路由用例；分区主体、标识前缀两道门禁绿。
