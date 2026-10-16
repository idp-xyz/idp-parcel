# 25 节点收寄带上 parcel-shipment 的服务结果标记：生产渠道上这一查没人做

Category: bug
Status: needs-triage——2026-10-16 Claude Code 会话立（票 23 实施中量到）。要先裁三问（见下）再转 ready-for-agent。
Blocked by: 无
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 之后的遗留；不挡任何票
出处：`internal/nodeoperations/application` 的 `ReceiveDeliveredUnitCommand.ServiceMarkers` 头注；UC-NO-002 的「服务结果引用」一行与 `AT-NO-020`–`022`；[08](./08-isolated-release-of-main-chain-command-faces.md) 的「收寄的服务结果标记留空」；[23](./23-node-operations-isolated-command-intake-retires.md)「实施中量到的」。

## 现象

`ReceiveDeliveredUnitCommand.ServiceMarkers` 的契约是「PS 服务结果引用（已取消 / 终局 / 无路由 / 受限），由接入层查好带入；编排原样标记不判断——它们不阻止接收，只限制后续方向性作业」。生产渠道上唯一的收寄 Intake `WorkFactReceptionIntake` 不查，`receptionCommand` 也不从载荷收（收了就是采信调用方自报的 PS 结论）。结果：已取消或已终局的包裹实际到站时，收寄记录不带标记，后续方向性作业不受限——UC-NO-002 的 `AT-NO-020`（包裹已取消但仍实际到站：如实保存接收与控制，不重开服务、不恢复路由或正式承诺）在生产上没有落点。

票 08 当时把它记为隔离形态的已知缺口；票 10 换作业事实渠道时没补，此后无票。

## 要裁的三问

1. **按什么键查。** 收寄时手里有处理单元与外部标识观察（`Mark`），委托与包裹身份是识别之后才有的。候选：(a) 按外部标识向 PS 的外部标识关联查当前服务结果；(b) 收寄只保全，标记在身份识别成立那一步再由编排向 PS 取。
2. **标记词表。** 节点作业这边今天是不透明串数组，没有封闭词表；PS 侧「已取消 / 终局 / 无路由 / 受限」各对应哪个 PS 事实与版本，要在 PS 与 NO 的 CONTEXT 里各写一句。
3. **读口形状。** PS 今天没有按外部标识答服务结果的读口；新读口归 PS 拥有、NO 经防腐适配器消费，查不动时收寄照常保全、标记格答「未查到」而不是空——空与「查过了、没有限制」要分得开。

## 不做

- 不改「标记不阻止接收」的规则（UC-NO-002）。
- 不从载荷收标记。
