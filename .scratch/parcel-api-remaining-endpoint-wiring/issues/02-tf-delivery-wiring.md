# TF 交付登记与 POD 更正接线

Category: enhancement
Status: ready-for-agent

阻塞条件(非票号):工作树里 MCP-3 未提交批次先落提交,见父规格「排序约束」。

## 要做什么

把 `POST /transport-fulfillment/deliveries` 与 `POST /transport-fulfillment/delivery-proof-corrections` 背后的 `unwiredDelivery` 换成真编排:新建 `cmd/parcel-api/assemble_delivery.go`,构造 `tfapp.NewRegisterEffectiveDeliveryHandler`。一个处理器带 Register 与 Correct 两方法,对应两个端点,一笔接完。

## 依赖缝逐条

`RegisterEffectiveDeliveryDeps` 的五条:

- `Deliveries`(ports.EffectiveDeliveryStore)— TF postgres 交付登记库已存在(`is_current` 部分唯一索引、更正翻旧插新),接真;
- `Attempts`(ports.DeliveryAttemptView)— 交付尝试读面。查 TF postgres 适配器现状:有则接真,无则显式未配置(编排会如实停在尝试读不回的未决,不造尝试事实);
- `Versions`(ports.DeliveryIdentityFactory)— 接既有标识签发实现;
- `Downstream`(ports.EffectiveDeliveryHandoff)— 接 TF 的 Outbox handoff(交付→PS 终局的意图与登记同一提交);
- `Clock` — 生产时钟。

## 验收

同票 01:真库实跑装配测试、端点表对照不破、未配置 Intake 仍在前、gofmt / vet / 全仓 test 绿、单独成 commit。
