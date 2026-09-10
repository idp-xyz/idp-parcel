# CC 没有 inbox：SA 发出的资金事实信封到不了 `ReceiveFundsFact`，税费付款核对只能靠测试喂事实

Category: enhancement
Status: draft——2026-09-10 通道 4 立票（task-9880bbc9），只写票面未动代码；取证锚 `3f485e97`
Blocked by: [02](02-sa-external-funds-fact-adoption-hands-off-an-envelope.md)（没有那封信封，消费者无物可收）

## 缺口（取证于 `3f485e97`）

- `git ls-files internal/customscompliance/adapters/inbox` → 空；CC 今天没有任何 inbox 消费者。
- `internal/customscompliance/application/reconcile_duty_payment.go` 的 `ReceiveFundsFact`（步 6 的 CC 半边）在，命令 `ReceiveExternalFundsFactCommand{Registration ports.ExternalFundsFactRegistration}`；`git grep -n ReceiveFundsFact -- cmd/` 零。
- `cmd/parcel-dispatch/assemble.go` 路由表无 CC 消费者（`git grep -n -i 'customscompliance' -- cmd/parcel-dispatch/assemble.go`——开工前核）。

## 语言从哪里来

- CC `CONTEXT.md`：「外部资金事实只有在能够按真实程序和适用范围关联到当前监管核定税费时，才能参与税费付款核对。金额相同、同一包裹、同一客户或同一时间不能单独证明付款覆盖」；「监管核定税费、实际付款……税费付款核对……必须分别保存并由各自责任方拥有」。
- mech/07 CC-c：「`ReceiveFundsFact` 入向命令 + `external_funds_fact` 登记册（按事实引用幂等；『待关联』派生——没有核对引用它的事实就是待关联，无状态推进写口）」。

## 做法

1. `internal/customscompliance/adapters/inbox/external_funds_fact_consumer.go`：收 `settlement-accounting.external-funds-fact.adopted`，按引用回查 SA 读口取事实内容（付款人 / 金额 / 币种 / 业务时间 / 来源身份），译成 `ports.ExternalFundsFactRegistration` 交 `ReceiveFundsFact`。
2. 读 SA 用**消费侧适配器** `internal/customscompliance/adapters/settlementaccounting/`（新，CONTEXT-MAP 加 SA→CC 一条边）；`application` 不得 import `settlementaccounting`。
3. 同引用重放 → `已存在`；同引用换内容 → `内容冲突`（编排已有，消费者只译不判）。
4. `cmd/parcel-dispatch/assemble.go` 路由表加一行（共享接线文件，动前占号）。

## 红线

- 消费者不关联、不核对：关联依据与三轴由 `VerifyPayment` 的调用方交，不在消费者里猜（「金额相同……不能单独证明付款覆盖」）。
- 不复制金额进 CC 以外的第二处权威：CC 登记册存的是引用 + 核对所需维度，来源仍是 SA。

## 完成判据

1. `git grep -w NewReceiveFundsFact -- cmd/` 或等价装配符号有非测试调用点。
2. 应用层：一封 → 一条登记；重投 → `已存在`；换内容 → `内容冲突`。
3. 真库装配用例一正一反；CONTEXT-MAP 与机制清点（消费缝 CC→SA +1）同笔。

## 地盘

`internal/customscompliance/adapters/inbox/`（新）、`internal/customscompliance/adapters/settlementaccounting/`（新）、`cmd/parcel-dispatch/assemble.go` 一行、`docs/domain/CONTEXT-MAP.md` 一条边。

## 要裁的

1. **SA 读口用哪个**：`ports.ExternalFundsFactStore` 是 SA 内部写侧登记面，消费侧适配器读它还是读 `catalogue_read.go` 的 `ExternalFundsFactCatalogueRow` 目录读口——前者是按键取一条，后者是列表。归 SA owner 一句（CC 是消费方，形照 PS 读 PC 的消费侧适配器）。

## 参照

[02](02-sa-external-funds-fact-adoption-hands-off-an-envelope.md)；[mech/07](../../mechanism-executor-triage/issues/07-cc-four-executors-behind-existing-uc-steps.md) CC-c；`internal/parcelshipment/adapters/inbox/effective_delivery_consumer.go`（跨上下文 inbox 消费者 + 消费侧读口先例）；ps-port-remainder/05（CC 读 PS 的消费侧适配器先例，`13f3ba65`）。

## Comments

- 2026-09-10 · 通道 4：立票。未动代码。
