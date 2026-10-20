# 103 PS 终局重派生进追踪投影时不带前身，新旧两代终局同时算当前有效

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-dcg103（2026-10-19，见文末「完成记录」）。此前 ready-for-agent——2026-10-19 分诊为 A（挡演示/上线），批次见 [triage-2026-10-19](../triage-2026-10-19.md)。此前 needs-triage——2026-10-19 Claude Code 子代理立（票 [97](./97-demo-disclosure-policy-keeps-the-final-dimension-pending.md) 实现时读代码发现；没有实走，证据只是代码阅读，钉 `5f0aa66d`）。
Blocked by: 无
地盘：`internal/visibilityexception/adapters/parcelshipment/derive_on_final_outcome.go`（终局译装器）；要裁的那一格可能还碰 PS 终局采用账的读口。
出处：VE CONTEXT「被替代条目留档不参与派生」与「终局事实有效性变化也不能被伪装为人工重开」；PS 终局同源重派生（`FormParcelFinalHandler.rederive`，ADR-0117 纪律半「按来源声明的更正关系回指前版成链」）；[ADR-0138](../../../docs/adr/0138-voided-carrier-pickup-voids-the-label-service-final-as-a-third-ledger-record-and-rejudges-on-remaining-basis.md)（Proposed，终局失效）。

## 现象（读代码）

- PS 同源重派生一份终局时，采用账多一行新记录：采用键的版本维换成新责任结果的版本，`ParcelFinalOutcome.PriorVersion()` 指回前一代终局。新旧两行都经 `parcel-shipment.final-outcome.formed` 发出。
- VE 的终局译装器 `finalProjectionCommand` 把记录译成已接受源事实。事实引用是 `final-outcome/<包裹>/<责任结果种类>`，版本取采用键的版本维，`Supersedes` 不填。TF 与 CC 的译装器按源侧给的前身填 `Supersedes`（例如有效交付那一只），终局这一只没有。
- 于是同种类重派生之后，投影的当前有效集里同一事实引用有两代都是已采用终局：`CurrentlyEffective` 只按源上下文给出的替代关系滤，源侧这里给了关系，翻译时丢了。

## 影响

- 映射目录若给 `final-outcome-adopted` 登一行里程碑，时间线上同一终局出现两次。演示映射今天没登这一类，所以演示上看不见。
- 票 97 的客户视图终局一维只问「投影里有没有已采用的终局」，两代都在时答案仍对。ADR-0138 若被接受、终局失效落成第三种记录并发给 VE，失效之后被失效的那一代仍在当前有效集里，终局一维会继续展示一份 PS 已经说不成立的终局。

## 三类

- 机制：终局译装器把 PS 给出的前一代关系译成 `Supersedes`。
- 产品策略：无。
- 租户取值：无。

## 要裁的

- 前身怎么译：PS 给的前身是终局版本（`FinalOutcomeVersionID`），VE 的事实版本是采用键的责任结果版本，两套号不同。译装器按前一代终局版本反查它的采用记录、取那条记录的键版本，还是 PS 在记录上另给一格前一代的键版本——前者只在 VE 一侧动，后者动 PS 读口。
- 重派生换了责任结果种类时，事实引用跟着换，两代不在同一引用下，替代关系照 VE 硬句不成立。这一格是否另按冲突处理，要一并裁。**今天不会出现**：`FormParcelFinalHandler` 只在当前终局的来源种类与新结果同种时走重派生，异种一律走不采用。这一格只在 [ADR-0138](../../../docs/adr/0138-voided-carrier-pickup-voids-the-label-service-final-as-a-third-ledger-record-and-rejudges-on-remaining-basis.md)（Proposed）落地、终局失效后按剩余依据改判时才可能出现，分诊时可以先放下，留给那张实现票。（票 97 评审补记，2026-10-19）
- ADR-0138 落地时，终局失效怎样进投影（第三种事实类型，还是一份不采用的新版本替代前代），属那张实现票；本票只把重派生这一半接上。

## 规模

S–M（译装器一处加反查；若改 PS 读口则 M）。

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg103`）

授权：用户原话「你作为业务和系统专家自决……解决所有的问题」，并同意只做分诊 A 组。下面各条代决待 VE / PS owner 复核。

### 做了什么

- **PS 读口**：`ports.FinalOutcomeByVersionView`（`FindByFinalVersion(租户, 包裹, 终局版本)`），已翻下当前位的历史版照样读得回；由 `adapters/postgres.FinalOutcomes` 实现。迁移 `migrations/parcel_shipment/0028_final_outcome_version_unique.sql` 加部分唯一索引 `final_outcome_version_unique`（租户，包裹，终局版本；不采用行不收）。
- **VE 终局译装器**：`DeriveOnFinalOutcomeAdapter.predecessorOf`——已采用且带 `PriorVersion` 的记录，按终局版本找回前一代，核对它是同包裹、同责任结果种类的已采用终局，取它采用键上的版本译进 `Supersedes`；首派生与不采用不问。前一代读时依赖不通答 `ErrFinalNotVisible`（本路未决名单里已有）；找不到或对不上答 `ErrFinalRecordInconsistent`（不在名单）。`FinalOutcomeFinder` 内嵌上面那个读口；生产接线传的 `pspostgres.NewFinalOutcomes` 原样满足，`cmd/parcel-dispatch/assemble.go` 与本路哨兵名单都没动。
- **用例文档**：UC-VE-002 验收表加 `AT-VE-174`–`AT-VE-176`。
- **立票**：[117](./117-projection-entries-are-stored-without-their-supersession.md)（投影条目落库不带前身，运营查阅 `supersedes` 永远空——写真库用例时撞上）、[118](./118-final-voidance-and-cross-kind-rejudgment-into-the-projection.md)（终局失效与失效后异种改判怎样进投影，Blocked by ADR-0138）。

### 代决

1. **前身怎么译**：取「VE 一侧按前一代终局版本反查采用记录、取其键版本」，PS 只多一个读口，不在记录或信封上另加「前一代键版本」一格。`PriorVersion` 已经是源上下文给出的关系，VE 只翻译；另加一格要动 PS 领域对象与表列，且是同一关系的第二处表达。
2. **读口落哪**：PS ports 立窄口 `FinalOutcomeByVersionView`，不并进 `FinalOutcomeStore`（并进去，每个 `FinalOutcomeStore` 替身都得跟着加一法，而 PS 自己的编排用不上它）；VE 适配器的消费侧接口内嵌它（ADR-0025；同形先例是 NO 适配器直接引 `psports.CurrentFinalView`）。
3. **包裹进读口参数，并加库面唯一索引**：前后两代必在同一包裹；读口不依赖签发器全局铸号，两行同号时 `QueryRow` 只会静默取其一，所以钉进库面。
4. **前一代读不到判确定性不一致，不当滞后**：前一代先于后一代提交（后一代正是从它那一行翻过来的），后一代可见时它必已可见。
5. **异种前身拒收**：前一代不是同种（以及不是已采用、在他包裹、采用版本与本代相同）一律 `ErrFinalRecordInconsistent`，既不译成无前身的首登，也不跨事实引用指名前身。今天 PS 只同种重派生，这一格只能是账坏了；ADR-0138 落地后怎么接见票 118。
6. **不新增哨兵**：复用本路既有两个，未决名单不变——省得碰在途重构的 `assemble.go`。
7. **投影条目不带前身另立票 117，不在本票修**：本票的不变量（当前有效集只剩一代）由派生编排从事实库现算，事实库里前身是在的；落库格属 VE 投影库，不在本票地盘。

### 越权风险点

- 迁移 0028 加唯一约束：若某个既有库里同一包裹已有两行同终局版本（签发器全局唯一铸号，正常不会有），迁移会失败，要人工查账。
- 「前一代读不到即账坏」的前提是 PS 采用账读写同库同序；将来若改走不保序的只读副本，这一格应回到未决。
- **不回补旧账**：本修之前已经进 VE 的重派生终局没有前身，那些投影里两代仍同时当前有效；同一信封重投时前身一格变了，派生编排按内容冲突入账（`SOURCE_CONFLICT`），旧事实不改。演示库按种子重灌即干净，真实租户未上线；要回补需另立数据修复票。
- 异种前身确定性拒收：ADR-0138 落地前若有人另开一条异种重派生路，那封终局信封会卡在本路且不进未决名单，响亮但不自愈。
- `AT-VE-174`–`AT-VE-176` 按协调方分配的号段使用，验收表措辞待 VE owner 复核。

### 用例

- `internal/visibilityexception/adapters/parcelshipment/derive_on_final_outcome_postgres_test.go`：`TestACorrectedFinalOutcomeLeavesOnlyOneGenerationCurrentInTheProjection`、`TestALatePredecessorFinalNeverBecomesCurrentAgain`（`AT-VE-174`，真库：PS 采用账照 `FormParcelFinalHandler` 的写法落首派生与重派生，VE 真实事实库、投影库与派生编排；映射夹具给已采用终局登一行里程碑）。证更正后当前投影只有一代已采用终局、事实库两代留档且后一代指名前一代的采用版本、更正前那一版投影可按版本读回；倒序到达结论不变。把 `Supersedes` 那一译拿掉时两条都红：「当前投影里有 2 代已采用终局」。
- `derive_on_final_outcome_test.go`：`TestARederivedFinalOutcomeSupersedesItsPredecessorsAdoptionVersion`（`AT-VE-174` 译装半边）、`TestAnUnreadablePredecessorIsContinuableUndecided`（`AT-VE-175`）、`TestAMissingOrForeignPredecessorIsInconsistentNotUndecided`（`AT-VE-176`，六格）、`TestOnlyARederivedFinalOutcomeAsksForItsPredecessor`。
- `internal/parcelshipment/adapters/postgres/adoption_cancellation_final_test.go`：`TestAFinalOutcomeIsFoundAgainByItsFinalVersion`（读口往返、他包裹/他租户/没签过的版本读不到、包裹内同号被唯一索引挡、他包裹同号不挡）。

### 验证

- `go build ./...`、`go vet ./...`、`gofmt -l` 干净。
- `scripts/test-full.sh` 跑改动包与反向依赖（cmd 反向依赖全带：parcel-api、parcel-dispatch、parcel-frontline-import 及各 register 命令）、`./internal/architecture/...`、`./migrations/...`、`./internal/parcelshipment/...`、`./internal/visibilityexception/...`：70 ok / 0 FAIL。
- 没跑全量，没重生成 `MECHANISM-INVENTORY.md`；种子未改，未重灌。证据记 `S`。
