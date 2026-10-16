# 25 节点收寄带上 parcel-shipment 的服务结果标记：生产渠道上这一查没人做

Category: bug
Status: resolved——2026-10-16 进 main（`0214a62a`，分支 `claude-oc25` 快进，远端 main 含 `2c540468`）；见「进 main 记录」。此前 in-progress——2026-10-16 完工，待进 main；见「完成记录」。此前 in-progress——2026-10-16 Claude Code 会话裁三问并认领（用户「yes, do」），分支 `claude-oc25`；裁定见「裁定」。生产上的效果要等身份核对缝接真（[ps-external-mark-relations/01](../../ps-external-mark-relations/issues/01-external-mark-relations-have-no-model-in-parcel-shipment.md)，needs-info，缺排期决定）：那之前明确接收停在`收寄待确认`，走不到取标记这一步。此前 needs-triage——2026-10-16 Claude Code 会话立（票 23 实施中量到）。
Blocked by: 无——机制半边不挡；生产效果挂在 ps-external-mark-relations/01 上（见 Status）
父票：无——不属 [psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 的完成判据（那一票只管渠道换真 Intake），是票 10 换口时带过来的业务缺口；不挡任何票
出处：`internal/nodeoperations/application` 的 `ReceiveDeliveredUnitCommand.ServiceMarkers` 头注；UC-NO-002 的「服务结果引用」一行与 `AT-NO-020`–`022`；[08](./08-isolated-release-of-main-chain-command-faces.md) 的「收寄的服务结果标记留空」；[23](./23-node-operations-isolated-command-intake-retires.md)「实施中量到的」。

## 现象

`ReceiveDeliveredUnitCommand.ServiceMarkers` 的契约是「PS 服务结果引用（已取消 / 终局 / 无路由 / 受限），由接入层查好带入；编排原样标记不判断——它们不阻止接收，只限制后续方向性作业」。生产渠道上唯一的收寄 Intake `WorkFactReceptionIntake` 不查，`receptionCommand` 也不从载荷收（收了就是采信调用方自报的 PS 结论）。结果：已取消或已终局的包裹实际到站时，收寄记录不带标记，后续方向性作业不受限——UC-NO-002 的 `AT-NO-020`（包裹已取消但仍实际到站：如实保存接收与控制，不重开服务、不恢复路由或正式承诺）在生产上没有落点。

票 08 当时把它记为隔离形态的已知缺口；票 10 换作业事实渠道时没补，此后无票。

## 要裁的三问

1. **按什么键查。** 收寄时手里有处理单元与外部标识观察（`Mark`），委托与包裹身份是识别之后才有的。候选：(a) 按外部标识向 PS 的外部标识关联查当前服务结果；(b) 收寄只保全，标记在身份识别成立那一步再由编排向 PS 取。
2. **标记词表。** 节点作业这边今天是不透明串数组，没有封闭词表；PS 侧「已取消 / 终局 / 无路由 / 受限」各对应哪个 PS 事实与版本，要在 PS 与 NO 的 CONTEXT 里各写一句。
3. **读口形状。** PS 今天没有按外部标识答服务结果的读口；新读口归 PS 拥有、NO 经防腐适配器消费，查不动时收寄照常保全、标记格答「未查到」而不是空——空与「查过了、没有限制」要分得开。

## 裁定（2026-10-16，Claude Code 会话；产品策略，没有租户取值）

1. **按什么键查 → 取 (b)，在编排里、身份唯一成立时按包裹查。** 收寄编排本身就经 `ParcelIdentityView` 向 PS 核对身份；恰一个候选时它已经知道是哪一件正式包裹，就在同一步经新端口向 PS 取这件包裹此刻的服务结果。不在 Intake（接入层）查：Intake 只做认证与译装，跨上下文读放进编排，与身份核对同一处、同一种失败处理。待识别（零候选）与身份冲突（多候选）不查——没有可问的包裹；明确拒收与仅扫描两支不经身份核对，也不查。身份后来确认时的补标记归身份关联那一步，本票不做。
2. **标记词表 → 封闭两种，带结果引用与版本。** `CANCELLED/<取消决定标识>`（PS 有已成立的取消决定）与 `FINAL/<终局种类>/<终局版本>`（PS 有当前有效终局）。UC-NO-002 要的是「结果及版本」的引用，这两种都给得出。「无路由」「受限」不是 PS 的服务结果——UC-NO-002 把它们归「当前限制与路由」那一行（network-routing 与各限制源），不进本票。已接受不是限制，不标。可供程序分辨的只有首个 `/` 之前的前缀：终局种类是租户配置的引用（管理台占位例就带 `/`），版本也可能带，尾部只作结果引用、不再拆回种类与版本。两边 CONTEXT 不用另写：PS 已定义「取消决定」「终局服务结果」两词，NO 已有「已取消、已经终局或当前无有效路由的包裹仍然到站时，节点必须如实保存接收、控制和其他已发生事实」那条不变量；本词表只是那句话在收寄记录上的表示，不是新的领域语言。
3. **读口形状 → NO 端口 `ParcelServiceResultView`，PS 侧由消费方防腐适配器实现。** 端口归 NO（消费方），实现放 `internal/nodeoperations/adapters/parcelshipment`，读 PS 已有的 `ParcelCancellationView` 与 `CurrentFinalView`，包裹按既有约定 `ParcelAssociationReference.String()` = PS `DeclaredParcelID` 译。**读不动答`收寄待确认`、带续办引用、不落册**——与身份核对读不动同一种处理：设备照常重投，物理事实在重投那一次落；票面原写「标记格答未查到」不取，那会多造一种既不是「没有限制」也不是「有限制」的第三态，而现有代数里「依赖读不动」就是`待确认`。
4. **命令上的 `ServiceMarkers` 字段删去。** 它的契约是「由接入层查好带入」，可生产上没有任何调用方填它；标记改由编排一处取，留着这个字段就是第二个来源。

## 不做

- 不改「标记不阻止接收」的规则（UC-NO-002）。
- 不从载荷收标记。

## 完成判据

- 身份唯一成立时：PS 有取消决定 → 收寄记录带 `CANCELLED/<标识>`；有当前有效终局 → 带 `FINAL/<种类>/<版本>`；两者都无 → 不带；读口读不动 → `收寄待确认` 带续办引用、不落册。
- 待识别、身份冲突、明确拒收、仅扫描四支不问读口。
- 防腐适配器逐格翻译、读口错误原样交回；生产装配接上 PS 真库读口。
- 架构门禁绿（跨上下文翻译只在 `adapters/parcelshipment`）。

## 完成记录（2026-10-16，Claude Code 会话，分支 `claude-oc25`，代码 `0214a62a`）

- **落点**：NO `ports` 加 `ParcelServiceResults` 与 `ParcelServiceResultView`；`ReceiveDeliveredUnitDeps.ServiceResults`，编排在恰一个候选时、签发版本之前问一次，`serviceMarkersOf` 落封闭词表；命令的 `ServiceMarkers` 删去，明确拒收与仅扫描两支的记录不再带标记；防腐适配器 `internal/nodeoperations/adapters/parcelshipment`（`ServiceResultView`，读 PS `ParcelCancellationView` 与 `CurrentFinalView`，终局只认已定案）；`cmd/parcel-api` 的 `buildParcelServiceResultView` 与 `cmd/parcel-frontline-import` 的收寄导入装配都接 PS 真库两册。
- **补的格**：编排——取消与终局各落一枚标记且收寄、控制、意图照常；读不动答`收寄待确认`带续办引用、不落册、不交意图；装配缺读口同样停下；待识别、身份冲突、明确拒收、仅扫描都不问读口。适配器——逐格翻译且两读口各被问一次同一（租户，包裹）；无取消、无终局、终局在册未定案都答空；读口错误原样交回、构造期拒缺件。真库 `TestTheWiredServiceResultViewReadsParcelShipmentCancellationAndFinal`——经 PS 生产写口登一份取消决定与一份已定案终局，生产装配的读口在一笔事务里读回两枚的原料，另一件包裹答空。旧的「命令 `ServiceMarkers` 为空」断言在作业事实 Intake 与前线导入模板两处删去，改注一句为何不再有这一格。
- **判据**：✅ 身份唯一成立时取消 / 终局 / 皆无 / 读不动各有其答；✅ 待识别、身份冲突、明确拒收、仅扫描不问读口；✅ 适配器逐格翻译、错误原样交回、生产装配接真库；✅ `internal/architecture` 绿（跨上下文 import 只在 `adapters/parcelshipment` 与 `cmd/`），机制清点多出一组消费缝 `nodeoperations → parcelshipment`（`0e31b9f3`）。NO、PS、`cmd/parcel-api`、`cmd/parcel-frontline-import`、`internal/architecture` 带 DSN 全绿；`go build ./...`、`go vet ./...` 退 0。
- **生产效果仍挂在身份核对缝上**：`cmd/parcel-api` 的 `unconfiguredParcelIdentityView` 对每一次明确接收答依赖未配置，编排停在`收寄待确认`、走不到取标记这一步；前线导入在身份未配置时开工先提示，`RECEIVED` 行答`收寄待确认`不落库，拒收与仅扫描行照常落库（这两支本来就不取标记）。缝接真之后标记随之生效，这里不用再改。恢复动作在 [ps-external-mark-relations/01](../../ps-external-mark-relations/issues/01-external-mark-relations-have-no-model-in-parcel-shipment.md)，缺排期决定。
- **标记今天没有消费方**：NO 唯一的方向性作业编排是集运（UC-NO-003），它加成员时只核单元在不在、成员是否已在别的袋里，不读收寄标记，也不核路由、兼容与身份；「标记限制后续方向性作业」此刻只体现在记录与复核目录（管理台节点作业复核页的「服务标记」列照原样显示）。立为 [no-directional-basis/01](../../no-directional-basis/issues/01-consolidation-adds-members-without-any-directional-basis.md)，needs-triage。
- **AT 盘点**：`.scratch/at-coverage-inventory.md` 的 `AT-NO-020` 改指新用例（原指的用例不再引它），`AT-NO-021` 由②迁①。
- **评审**：作者自审，不算非作者评审。证据层级 `S`。

### 进 main 记录（2026-10-16）

- 分支 `claude-oc25` 三笔 `0214a62a` / `0e31b9f3` / `2c540468` 叠在远端 main `7a8f56e5` 之上，快进进 main、推的就是这个 SHA。等全量期间另一会话两度推了管理台的票（group-legal-entities-uiux/05–07），本分支两度重放到新 main 上，每次重放后机制清点重生成、与上一版逐字相同。
- 推送方全量：钉 `d646c334`（本分支重放到 `64e78c82` 上的那一版）的干净 detached 检出，带 DSN，`go test -p 1 -count=1 -v ./...` 退 0——有测试的包 142 个 ok、0 FAIL，`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）；同一检出上全仓 `gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0。`2c540468` 与 `d646c334` 在 `apps/admin-web/` 与 `.scratch/` 之外逐字相同；这两处之外唯一读管理台源码的 Go 门禁在 `internal/architecture`，在 `2c540468` 上与 NO、`cmd/parcel-api`、`cmd/parcel-frontline-import` 一起带 DSN 重跑全绿，`go build ./...`、`go vet ./...` 退 0。
- 评审：只有作者自审，没有非作者评审（本机无别的会话可派；仓规 2026-09-16 不用子代理）。
