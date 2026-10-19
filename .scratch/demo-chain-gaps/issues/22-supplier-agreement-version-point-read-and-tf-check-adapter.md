# 22 供应商协议版本点读口与 TF 协议核对适配器

Category: enhancement
Status: in-progress——完工、隔离评审已修订，待进 main，分支 claude-dcg22（见文末「完成记录」「评审记录」）。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: 无
地盘：PC `ports/ports.go`（加窄口）、`adapters/postgres/supplier_agreement.go`；TF 新 `ports/supplier_agreement.go`、新 `adapters/partycommercial/supplier_agreement_source.go`。
出处：[02](./02-transport-commission-has-no-command-face.md)、[07](./07-transport-charge-occurrences-have-no-production-path.md)；方案「02 委托、订舱、承运答复」D02-4、D02-5，「07 运输收费发生项」D07-2，「跨票通则」X4。

## 做什么

- PC 新开一个窄读口，按（租户、协议对象、版本标签）答 Effective 版本壳加正文；不借管理台列表口（方案 D02-5）。
- TF 消费侧适配器把它译成核对结论与采购责任法人，判法照方案 D02-4；新端口另立文件，不改 TF `ports/ports.go`（方案 X4）。
- 本票只出读口与适配器，[23](./23-transport-commission-submission-face.md) 的委托核对与 [27](./27-transport-charge-occurrences-are-registered-and-handed-off.md) 的采购上下文核对共用它（方案 D07-2）。

## 完成判据

- 按（租户、对象、版本标签）答版本壳与正文。
- 适配器译成：在册 / 未生效 / 已终止 / 服务方不符 / 法人。
- 真库测试覆盖 SYN 两份协议。
- 不动 cmd。

## 规模

S

## 完成记录

2026-10-19 作者子代理在分支 `claude-dcg22`（基于 origin/main `3739390b`）做完，提交 `1ee558b2`（PC 点读口）、`bb84fcdc`（TF 端口与适配器）。不动 cmd，无迁移，无 ADR（方案把接口形状归 ADR-0202，随 23/24 起草）。

### 做了什么

- **PC 窄读口**：`ports.SupplierAgreementVersionView.LoadSupplierAgreementVersion`（加在 `ports/ports.go`，紧随 `SupplierAgreementContentView`），答复 `ports.SupplierAgreementVersionRead`。实现挂在既有只读件 `adapters/postgres.SupplierAgreementContents` 上：一条语句左连接版本壳与正文，壳只信快照列并核它与所问的键同一版。
- **TF 领域值**：`domain.SupplierAgreementVersion`（新文件 `domain/supplier_agreement_version.go`），协议对象与版本标签两格分开收。
- **TF 端口**：`ports.SupplierAgreementSource`、`ports.SupplierAgreementStanding`、`ports.SupplierAgreementCheck`（新文件 `ports/supplier_agreement.go`，未改 `ports/ports.go`）。
- **TF 消费侧适配器**：`adapters/partycommercial.SupplierAgreementSource`（新文件 `supplier_agreement_source.go`）。

### 接口形状（供 23、27 引用；ADR-0202 起草时照此写）

PC 读口：

```go
LoadSupplierAgreementVersion(ctx, tenant domain.TenantID, agreement domain.CommercialObjectID,
    version domain.CommercialVersionLabel) (ports.SupplierAgreementVersionRead, error)

type SupplierAgreementVersionRead struct {
    Version      domain.CommercialVersion  // HasVersion 为真时有意义
    HasVersion   bool
    Agreement    domain.SupplierAgreement  // 只在壳已生效且正文已登时交
    HasAgreement bool
}
```

- 没有这一版 → 两个 Has 都假，无 error。已生效壳无正文、已计划生效壳、已收尾壳 → 只交壳。已生效壳带正文 → 两者都交。
- 正文挂在已计划生效的壳上、快照立不起来、壳与键不符、正文行不完整 → error。读口不判可用与否。

TF 端口：

```go
CheckSupplierAgreement(ctx, tenant domain.TenantID, agreement domain.SupplierAgreementVersion,
    provider domain.ServiceProviderReference, at time.Time) (ports.SupplierAgreementCheck, error)

type SupplierAgreementCheck struct {
    Standing    SupplierAgreementStanding
    Snapshot    domain.AgreementSnapshotReference      // 只在在册格
    LegalEntity domain.ProcurementLegalEntityReference // 只在在册格
    Supplier    domain.ServiceProviderReference        // 在册与服务方不符两格
}
```

| Standing | `String()` | 何时 |
|---|---|---|
| `SupplierAgreementRegistered` | `AGREEMENT_REGISTERED` | 三项都过；带快照、法人、供应商 |
| `SupplierAgreementNotRegistered` | `AGREEMENT_NOT_REGISTERED` | 没有这一版；或已生效壳没登正文 |
| `SupplierAgreementNotEffective` | `AGREEMENT_NOT_EFFECTIVE` | 正文区间不盖决定时点；壳已计划生效、已到期或已被替代 |
| `SupplierAgreementTerminated` | `AGREEMENT_TERMINATED` | 正文终止时刻不晚于决定时点；壳已退役 |
| `SupplierAgreementProviderMismatch` | `PROVIDER_MISMATCH` | 有效期内但协议供应商不是服务方；带协议约定的供应商 |

- 四个否定格的 `String()` 与方案 D02-4 的业务结果词同拼写，23 可直接用作 200 业务结果。
- `error` 的来历与各自的恢复动作以 `ports.SupplierAgreementSource` 的文档注释为准，此处不复述。要点：PC 读不动与 PC 侧坏数据在值上分不开；能用 `errors.Is` 认出的只有 `ErrUntranslatableAnswer`；零决定时点是调用方违约。23、27 都是命令面，把任一 error 答成 `UNDECIDED` 即可（D02-4「PC 读不动答 `UNDECIDED`」）。
- 入参 `domain.NewSupplierAgreementVersion(object, version)` 任一格空或含 NUL 在构造期拒，23 的载荷据此答 `INPUT_NOT_ACCEPTED`。
- 装配（23 接线时）：`tfpartycommercial.NewSupplierAgreementSource(pcpostgres.NewSupplierAgreementContents(db))`。委托的 `Agreement` 存 `Snapshot`，新法人列存 `LegalEntity`。

### 代决与越权风险点

用户原话「你作为业务和系统专家自决……解决所有的问题」；以下是方案未裁、本票自决的细节。

1. **三项检查的次序**：照 D02-4 列举顺序判，在册 → 时点上支持采购 → 供应商等于服务方。先不过的那一项定结论。
   - 越权风险点：区间外而供应商也不对时只报未生效，登记方改完时点后还会再撞一次服务方不符（TF owner）。
2. **已生效壳没登正文答「未在册」**，不另立一格。供应商与责任法人都在正文里，没有正文就核不了。
   - 越权风险点：「从未发布」与「发布时漏登正文」落同一格。要分开得看 PC 目录上的 `HasContent`（TF / PC owner）。
3. **正文缺席时按壳位置分格**：已计划生效、已到期、已被替代答未生效；已退役答已终止。
   - 理由：PC CONTEXT 写供应商协议版本「到期、终止或被替代」。版本生命周期里按明确决定收尾的是退役，`Retire` 要一份决定引用。
   - 越权风险点：PC 没有明文把供应商协议的「终止」对到版本退役（PC owner）。今天库里只出现已计划生效与已生效两种位置，因为发布册只增、不翻状态；其余映射是为翻译成全函数立的。
4. **快照引用取 PC 的 `CommercialVersion.QualifiedLabel()`**（形如 `SYN-SUPPLIER-TRUNK-CN-SG-01/v1`），TF 不拼不拆（ADR-0080 决定七）。
   - 越权风险点：票 26 的 SA 若要从发生项快照回读 PC 的协议范围，拆分要由 PC 在 `NewQualifiedVersionLabel` 同一处提供反向，SA 不得自拆（SA / PC owner）。
5. **服务方按同一串字面比**，不比名称、不归一。同包其余适配器也是这条约定。
6. **区间更正（ADR-0038）不折进核对**：判据只用正文区间与终止，同 PS `agreementOf` 的先例。
   - 越权风险点：PC 更正了壳的区间之后，TF 仍按正文区间判（PC owner）。
7. **答复多带 `Supplier` 一格**（在册与服务方不符），超出票面「法人」一格，让登记方在不符时知道协议约定的是谁。
   - 越权风险点：无跨租户暴露，同租户内的协议正文本就对登记方可见。

### 偏离

- TF 新增领域文件 `domain/supplier_agreement_version.go`，不在票面地盘内。端口需要类型化的协议版本键，而「对象/版本」两段式指称不许 TF 拼，所以两格分开收。是新文件，不碰在途地盘。
- PC 测试另立 `adapters/postgres/supplier_agreement_version_test.go`，TF 适配器测试分单元（`supplier_agreement_source_test.go`）与真库（`supplier_agreement_source_db_test.go`）两份。

### 验证（证据 `S`）

- `go build ./...`、`go vet ./internal/partycommercial/... ./internal/transportfulfillment/...`、`gofmt -l internal/ cmd/` 干净。
- `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1 ./internal/partycommercial/... ./internal/transportfulfillment/adapters/partycommercial/ ./internal/transportfulfillment/domain/ ./internal/architecture/` 全 ok。
- 真库：`TestTheSyntheticSupplierAgreementsAreCheckedThroughTheRealPointRead` 读 `scripts/demo-seeds/data/commercial/publish-batch.json`，把两份 SYN 供应商协议经 PC 发布用例落库，再核：
  - 各按自己的供应商在册，交回种子里的法人 `SYN-LE-01` 与快照 `对象/v1`；
  - 生效前答未生效；未发布的版本与他租户答未在册；
  - 干线协议填末端承运商答 `PROVIDER_MISMATCH`，即票 23 的演示停点。
- PC 点读口的在场组合、租户与类别隔离、坏数据各有真库用例。适配器的五格、读不动不折、读口违约、零时点各有单元用例。
- 未重生成机制清点（进 main 时统一做）。种子未改。

### 待裁

- ADR-0202（票 23）把上面的接口形状与代决 1–4 写进正文，并写明两件评审登记的事（见「评审记录」）：
  - 检查次序与服务方比较是 TF 的内置判法，却放在消费侧适配器里，与 ADR-0025「适配器只翻译，不判断」有张力。ADR-0202 要么写明这一例外的理由，要么由 23 把比较挪进委托编排。
  - 区间更正（ADR-0038）没有折进核对（代决 6），记为已知缺口。若 PC owner 裁定要折入，由 PC 在点读口里交出更正后的区间，TF 不自己读更正。
- 票 26：SA 从快照回读 PC 时的反向拆分归 PC 一处（代决 4）。
- 本票不接线：生产接线棘轮与类型可达性棘轮在本票都没有新增条目。新领域值经 TF 端口可达，PC 新类型在 ports 不在 domain。23 接线时照常看棘轮。

## 评审记录

2026-10-19 隔离评审：阻断 0 项，非阻断 6 项，其中 3 项建议进 main 前修。三项都已修，另修了一项不要求进 main 前修的。修订提交 `2c614ca9`。

### 已修

1. **测试 Covers 注释用序号指方案子项。** 「方案 D02-4 第二项 / 第三项」改成引文：「在提交时点 `SupportsProcurementAt`」「协议供应商等于服务提供方」。「方案 D02-4 三项都过」改成「方案 D02-4 的检查都过」。
   - 顺手清了同类引用：适配器 `CheckSupplierAgreement` 的「按三项检查的次序」改成按 `tfports.SupplierAgreementStanding` 所列检查的次序；真库测试的「演示租户已发布的两份供应商协议」和 Covers 里的「SYN 两份协议」不再数种子里的条目。
2. **核对口注释说 error 只有两种来历，与实现不符。** `ports.SupplierAgreementSource` 的文档注释改为如实列出各种来历和各自的恢复动作：
   - PC 那一侧失败：读不动或 PC 侧坏数据。两者原样包裹，在值上分不开。
   - `ErrUntranslatableAnswer`：确定性错误。
   - 零决定时点：调用方违约，不带哨兵。

   注释还写明命令面把任一 error 答成未决即可，并写明派发消费者落未决哨兵时哪些不进名单、PC 坏数据会随读不动一起被重试。完成记录的「error 的来历」一条改为引这段端口注释，不再复述。
   - 没有给零时点另立哨兵：23、27 都是命令面，判时点用的是自己的提交或登记时刻，碰不到零值。等真有派发消费者需要区分时，再由它来立。
3. **适配器文件头注「三种收尾」数的是 PC CONTEXT 的条目。** 删去「三种」，只留引文。完成记录代决 3 的理由一句也删去「三种」。
4. **包注释说本包「只答一件事」，已经不准。** 这一项不要求进 main 前修，但改动小，又落在本票的适配器包里，所以本票一并修了。包注释改成本包是 TF 对 PC 各读口的消费侧适配器，各文件各答一件事，不列数目。承运主体身份那一段挪到 `carrier_identity_directory.go` 的文件头注。

### 未在本票改，已登去向

5. **检查次序与服务方比较放在消费侧适配器，与 ADR-0025「适配器只翻译，不判断」有张力。** 这样放是方案「票 22」那一行明文派给适配器的，PS 的 `channel_selection_basis.go` 也有先例，所以不算越过方案。但这个例外要在 ADR 里记下来，归 ADR-0202（票 23），已写进「待裁」。本票不改。
6. **点读口绕开了区间更正（ADR-0038）。** 已在代决 6 登为 PC owner 的越权风险点。ADR-0202 要把它写成已知缺口，已写进「待裁」。若要折入，由 PC 在点读口一处交出更正后的区间。本票不改。

### 验证

修订后重跑：
- `go build ./...` 干净；`go vet` 跑 PC 与 TF 两个上下文，干净；`gofmt -l internal/ cmd/` 干净。
- 带 DSN、`-p 1 -count=1` 跑 PC `adapters/postgres`、TF `adapters/partycommercial`、TF `domain`、`./internal/architecture/`，全 ok。
- 只改了注释与票面，行为未变。
