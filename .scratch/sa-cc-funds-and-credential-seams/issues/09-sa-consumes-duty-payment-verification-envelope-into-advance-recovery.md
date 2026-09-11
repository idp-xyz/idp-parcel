# CC 的付款核对信封发出后 SA 没人接：`customs-compliance.duty-payment-verification.formed` 落进 Outbox，`AssessAdvanceRecoveryHandler` 拿不到「关务税费及付款核对」这一项输入

Category: enhancement
Status: ready-for-agent——2026-09-10 17:3x 通道 5 立票（task-9a2ff746；票 [05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md)「要裁的」2 裁「本目录加一张」时点名），只写票面未动代码；取证锚 `66cad4c4`。要裁的为零：读口选型照 [03](03-cc-inbox-consumer-receives-external-funds-fact.md) 的裁决同口径
Blocked by: [05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md)（没有那封信封，消费者无物可收）——**05 已进 main（2026-09-11 15:1x，main 重放 tip `4ab49c65`）**，信封 `customs-compliance.duty-payment-verification.formed` 载荷 `{tenantId, scope, duty, funds, digest}`、分区键 `租户/duty-payment-verification/申报范围`、CC 读口 `DutyVerificationStore.FindVerification` 按三维键 + 指纹交那一版，本票可开工（2026-09-11 通道 1 推送方记）

## 缺口（取证于 `66cad4c4`）

- `internal/settlementaccounting/application/assess_advance_recovery.go` 在：`AssessAdvanceRecoveryHandler`（`NewAssessAdvanceRecoveryHandler(deps AssessAdvanceRecoveryDeps)`）、`AssessAdvanceCommand` / `FormRecoveryCommand` / `AdjustRecoveryCommand`、`AdvanceResult`——UC-SA-001 的编排本体已落。
- SA 没有 `adapters/inbox`（票 01 缺口节）；票 05 让 CC 在核对**形成**那一格发 `customs-compliance.duty-payment-verification.formed`（载荷只带引用：租户、申报范围、核对版本、监管核定税费版本、资金事实引用），SA 侧消费者票 05 明写「不在本票」。
- SA CONTEXT 要求实际代垫成立判断以「关务税费及付款核对」为输入之一；今天这一项只能由测试喂进 `AssessAdvanceCommand`。

## 语言从哪里来

- SA `CONTEXT.md`：「实际代垫成立判断使用付款方身份、外部资金事实、关务税费及付款核对和合同责任，不由任一单项输入直接推导」；「结算输入已接收：固定关务交接、税费、付款核对……的采用版本；接收不表示代垫或回收已经成立。」
- UC-SA-001 概述：「从 `UC-CC-009` 向结算交接一组范围明确、来源可追溯的监管税费、付款核对和外部资金事实引用开始」；输入表「付款核对｜`UC-CC-009`｜税费版本覆盖、法定义务覆盖、程序付款义务覆盖及各自范围｜只作为关务核对依据，不代替结算责任判断」；步 2「采用明确版本的税费、付款核对、资金事实和合同责任 → 形成结算输入版本；缺失保持待判断」。
- CC `CONTEXT.md`：「税费付款核对……不形成实际付款、客户回收或监管放行」——CC 是核对三态的权威，SA 按引用读、不复制结论。

## 做法

1. `internal/settlementaccounting/adapters/inbox/duty_payment_verification_consumer.go`：收 `customs-compliance.duty-payment-verification.formed`，按引用回查 CC 读口取核对三态与范围维度，译成 `AssessAdvanceCommand` 的「付款核对采用版本」那一格，交 `AssessAdvanceRecoveryHandler`；其余输入（付款方、外部资金事实、合同责任）缺哪一项由编排答「待判断」——消费者只译不判（UC-SA-001 步 2「缺失保持待判断」）。
2. 读 CC 用**消费侧适配器** `internal/settlementaccounting/adapters/customscompliance/`（新；CONTEXT-MAP 加 CC→SA 一条边）；`application` 不得 import `customscompliance`。读口选型照票 03「裁决」同口径：**按键取（租户 + 申报范围 + 核对版本）、走 CC 的只读口、取信封所指版本不取 latest**；CC 若无按键只读半边就补一个（形照 PS `LabelTransactionsByParcelView`），属 CC 地盘、动前占号。
3. 同信封重放 → 编排答`已存在`（结算输入版本已采用）；换内容不可能——核对版本不可变，同键异摘要在 CC 那头就是新版本、新信封。
4. `cmd/parcel-dispatch/assemble.go` 路由表加一行（共享接线文件，动前占号）。
5. 真库装配用例：入队一封 → 消费一次 → 结算输入版本一格采用；重投不翻倍。

## 红线

- 信封与 SA 登记册都不复制覆盖 / 差额 / 有效性的**结论**为第二处权威：SA 存的是核对版本引用 + 采用时刻；三态由 CC 读口按引用答。
- 不把「核对已形成」解释成「代垫成立」（SA CONTEXT 明写任何单项不能推导）；消费者不调 `FormRecoveryCommand`。
- 不改 `assess_advance_recovery.go` 的结果代数；不改 CC 的信封形（归 05）。
- 真实付款条件、真实税费属实例半边，夹具全是合成串。

## 完成判据

1. `git grep -w NewAssessAdvanceRecoveryHandler -- cmd/` 有非测试调用点（dispatch 装配）。
2. 应用层：一封 → 结算输入版本采用付款核对一格；重投 → `已存在`；其余输入缺 → 待判断（不是错误）。
3. 真库装配用例一正一反；CONTEXT-MAP 与机制清点（消费缝 SA→CC +1）同笔。

## 地盘

`internal/settlementaccounting/adapters/inbox/`（与票 01 共目录，文件各自）、`internal/settlementaccounting/adapters/customscompliance/`（新）、`cmd/parcel-dispatch/assemble.go` 一行、`docs/domain/CONTEXT-MAP.md` 一条边；CC 只读口若要补归 CC `ports` / `adapters/postgres`（占号）。不动 `internal/customscompliance/application/**`。

## 要裁的

无。读口选型与版本取法照票 03「裁决」；分区主体由票 05 定（租户 / 申报范围），消费者不关心。

## 参照

票 [05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md)「要裁的」2 与「裁决」；票 [03](03-cc-inbox-consumer-receives-external-funds-fact.md)「裁决」（读口口径）；UC-SA-001 概述、输入表、步 2；`internal/settlementaccounting/application/assess_advance_recovery.go`；`internal/parcelshipment/adapters/inbox/effective_delivery_consumer.go`（跨上下文 inbox 消费者 + 消费侧读口先例）。

## Comments

- 2026-09-10 · 通道 5（task-9a2ff746）：立票，未动代码。能力边界：读过票 05 全文、UC-SA-001 概述 / 输入表 / 步 2、SA CONTEXT 相关两句、`assess_advance_recovery.go` 的类型与构造函数名；没读 `AssessAdvanceCommand` 的字段——「付款核对采用版本」那一格叫什么、今天有没有，开工时以代码为准，没有就加一格（不改代数）。
