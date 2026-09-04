# 同来源同对象的更正版本在 PS 采用口被当作第二责任起点：`AT-PS-050` 无代码实现

Category: bug
Status: resolved——MCP-5（2026-09-04，task-f9e0bd40；隔离分支 `mcp5-ps-lc24`，基线 main `6f9436f3`；代码 tip `61698e0e`、清点笔 `8a5846d1`，main 上的 SHA 由 MCP-1 重放后在 Comments 补）。五问裁决落 [ADR-0117](../../../docs/adr/0117-same-source-correction-forms-a-superseding-adoption-version-chained-to-the-current-responsibility-start.md)，实施见「完成记录」；`AT-PS-050` 自此有代码实现，`AT-PS-049` 一字不动
Blocked by: 无

## 量到的事实（main `4cc1bc34` 上读得，逐符号名）

transport-fulfillment 自票 tf-segment-lifecycle-closure/08 起会对同一（租户+对象+尝试）的揽收落**第二个版本**
（`OffsitePickup.Correct` 回指前版，`RegisterOffsitePickupHandler.Correct` 落新行并重交 `OffsitePickupRegistrationIntent`），
并在信封 ID 上加版本段让更正版本各自入队。parcel-shipment 这一侧收到它之后发生的事：

1. `psinbox.OffsitePickupConsumer` 译信封只取 `tenantId`/`object`/`attempt` 三维（`decodeRegisteredOffsitePickup`），
   信封里的 `pickupVersion` 一格不读。
2. `AdoptOnOffsitePickupAdapter.HandleRegisteredOffsitePickup` 按（租户+对象+尝试）`FindByKey` 读回揽收——TF 的
   `OffsitePickupRegistrations.FindByKey` 自迁移 `transport_fulfillment/0015` 起答**当前版**（链尾），所以它拿到的是
   更正后的那一代，不是信封所指的那一代。
3. `AdoptNetworkIntakeHandler.Handle` 的采用键 `ports.IntakeAdoptionKey{TenantID, Parcel, Kind, Version}` 带版本——
   新版本不撞幂等，走到后面。
4. 同一函数随后调 `handler.deps.Adoptions.FindResponsibilityStart(ctx, key.TenantID, source.Parcel())`（`AT-PS-049`
   责任起点唯一）：首登版本已采用时它命中，于是更正版本经 `handler.refuse` 落成一条**不采用**记录，答
   `IntakeSourceNotAdopted`（`SOURCE_NOT_ADOPTED`），`RefusalBasis` 为
   `RESPONSIBILITY_ALREADY_STARTED/OFFSITE_PICKUP/<首登版本>`。

也就是说，PS 今天把「同来源同对象的更正版本」与「另一个来源想开第二个责任起点」当成同一件事处理。前者是
`AT-PS-049` 要挡的竞争，后者是 `AT-PS-050` 要接的更正——代码里没有分这两格的那一道判断。

`FindResponsibilityStart` 的实现（`internal/parcelshipment/adapters/postgres/intake_adoption.go`）靠部分唯一索引
「每租户+包裹至多一行 adopted」承担唯一性；不采用记录不受该索引约束，所以第 4 步的不采用行照常落库。

## 用例原句

`UC-PS-003`「一致性、幂等与并发」节：

> 来源更正、撤销或身份关系变化形成新的采用判断版本。原责任判断和承诺历史不能删除；当前承诺影响通过带原因的新版本表达。

同用例验收表 `AT-PS-050`：

> 来源后来被更正或撤销有效性 → 形成新采用判断和必要的承诺调整版本，不删除原历史

同表 `AT-PS-049`（今天代码实现的那一条）：

> 客户送站与场外揽收都指向同一包裹时不能形成两个责任起点，先合法形成者保留

票 tf-segment-lifecycle-closure/08 裁决里「新版本再采用一次是正确行为——PS 的责任起点与正式承诺生效时间锚在
`OccurredAt` 上，发生时刻被更正时 PS 必须再判一次」是按 `PickupResultVersion` 自注推的；裁决方自陈没读 PS 侧
编排。本票记的是读了之后量到的差。

## 裁决（2026-09-04，通道 5，task-f9e0bd40；owner 授权自决，照通道 6 那批的口径——写明能力边界，难逆转的落 ADR-0117）

五问逐答；理由与被否的替代写在 ADR-0117，此处只列结论。

1. **同来源更正的判据**：来源自报的更正关系，不按键推断。`IntakeSource` 加一格可选的「被更正版本」（`Corrects`），由来源所有者的事实带出（揽收那一路 `OffsitePickup.Corrects()`）；三件缺一不成立——来源种类与当前责任起点相同、来源声明了更正哪一版、那一版恰是该包裹当前采用的版本（链尾）。键相同而无更正声明（同一对象新一次尝试）是 `AT-PS-049` 的竞争。
2. **采用账形状**：`intake_adoption` 上长版本链，**只插不改**；「当前责任起点」按链尾派生，不存列。新列 `supersedes_source_version`；原部分唯一索引「每租户+包裹至多一行 adopted」换成两条——`adopted 且无回指`每包裹至多一行（根唯一 = 责任起点唯一，`AT-PS-049` 仍在库面），`adopted 且回指同一前版`至多一行（链线性，并发第二更正撞墙）。`FindResponsibilityStart` 改答链尾。不采另一种结果词：更正版本形成的仍是`正式承诺已形成`，链上位置由记录自身（`SupersedesVersion` + 承诺前版与原因）说。
3. **承诺调整版本**：与采用判断版本**同笔**形成，不另立票。新承诺版本 `intake` 换成更正后的收寄、生效时间随之等于更正后的发生时刻、回指被取代采用行上的承诺版本、原因 `SOURCE_CORRECTED/<来源种类>/<被更正的来源版本>`。领域新增一条收新收寄的构造门（同包裹、同接受基线、同来源种类），`Adjust` 原样留给不改收寄的调整。
4. **消费者按版本读回**：`psinbox.OffsitePickupConsumer` 读 `pickupVersion`（缺席即毒丸），`AdoptOnOffsitePickupAdapter` 按（键+版本）取回并核对版本；TF 侧要一个 `OffsitePickupRegistrations.FindByKeyAndVersion`（与 `EffectiveDeliveries` 同名同形，TF 地盘，经 MCP-1 同意后另占号）。
5. **`AT-PS-049` 一字不动**：当前责任起点存在而不满足第 1 条三件，照旧不采用，依据 `RESPONSIBILITY_ALREADY_STARTED/<种类>/<链尾版本>`（版本从首登换成链尾）。两格例外：更正所指前版**尚无任何记录** → `资格判断未决`，原因 `CORRECTION_PREDECESSOR_UNJUDGED`（重投会改变结果）；前版**有记录但不是链尾**（分叉，或本就是不采用行）→ 不采用，依据 `CORRECTION_TARGET_NOT_CURRENT/<种类>/<链尾版本>`（要人看）。

取消边界、接受基线与资格三道门对更正版本重新走一遍（tf/08 裁决说的「再判一次」），用更正后的内容；在这三道门被拒时根采用原样站着、拒绝行带依据。

**与 first-tenant-runway/10 的关系**：同族「判断版本化」，本裁决走的是它的「甲」方向（旧版本留作历史、当前派生），不实施它，不与它两条形状矛盾。

**能力边界**：读了 UC-PS-003 全文、PS CONTEXT 责任起点与更正各句、`adopt_network_intake.go` / `intake_adoption.go` / 迁移 0004 / `network_intake.go` / `pickup_source.go` / `adopt_on_offsite_pickup.go` / `offsite_pickup_consumer.go`、TF `offsite_pickup_registry.go` 与登记交接、ftr/10 票面；**未读** `node-operations` 节点收寄那一路的版本语义（它今天不落更正版本，`Corrects` 对它恒缺席）与 `network-routing` 复核消费者对第二封采用信封的处置（只确认它按采用键读回）。**越权风险点**（供用户复核）：① 把「同来源更正」的识别权判给来源所有者的更正声明而不是 PS 自己按键推断；② `intake_adoption` 责任起点唯一索引从「adopted」改为「adopted 且无回指」——`AT-PS-049` 的库面守法换了形状；③ 更正版本在取消/基线/资格三道门被拒时根采用不动——PS 不替人裁更正与原判断谁对。

## 完成记录（2026-09-04，通道 5，task-f9e0bd40）

分支 `mcp5-ps-lc24`，基线 main `6f9436f3`。分支上的 SHA 作封存出处；main 上的 SHA 等 MCP-1 重放后补。

| 分支 SHA | 范围 |
|---|---|
| `9a22f19a` | ADR-0117 + README 一行 + 本票「裁决」节（Status 转 in-progress） |
| `5c09787b` | 领域：`IntakeSourceSpec.Corrects` / `IntakeSource.Corrects()`（自指拒）；`FormalCommitment.RestateOnCorrectedIntake`（同包裹、同基线、同种类三道门；生效随更正后发生时刻；`Adjust` 原样留给不改收寄的调整） |
| `b0e939ef` | 采用账链：迁移 `parcel_shipment/0017_intake_adoption_supersession_chain.sql`（三列同在同缺、责任起点索引改「adopted 且无回指」、加「同一前版至多被取代一次」）；`ports.IntakeAdoptionRecord.SupersedesVersion` / `Supersedes()`；`IntakeAdoptions.FindResponsibilityStart` 答链尾；`adoptionColumns` 两面（根首版 / 更正版带前版与原因）；读回按同一两面重建 |
| `06384ef6` | 编排：`AdoptNetworkIntakeHandler.Handle` 责任起点那一步分三路（竞争照旧不采用 / `supersede` / 前版未判则未决、非链尾则不采用）；新未决原因 `IntakeCorrectionPredecessorUnjudged`；内容摘要对更正版本多带 `corrects:<前版>` |
| `fca342b8` | TF：`OffsitePickupRegistrations.FindByKeyAndVersion`（**端口外读法**，不进 `ports.OffsitePickupRegistry`——理由在方法头注：拓宽会拆 TF 自己 http / application 两处替身，唯一调用方在 PS；与 `EffectiveDeliveries` 同形）+ 真库用例 |
| `0b83b44d` | 消费方：`psinbox.OffsitePickupConsumer` 读 `pickupVersion`（缺即毒丸）；`AdoptOnOffsitePickupAdapter` 经 `OffsitePickupFinder.FindByKeyAndVersion` 取那一代并核版本；`pickupSourceFor` 把 `OffsitePickup.Corrects()` 译进 `Corrects` |
| `3460e8c3` + `61698e0e` | 真库两向用例 `TestASameSourceCorrectionSupersedesAndACompetingSourceIsStillRefusedInTheDatabase`（TF 真登记库 → 真 Inbox 消费门 → 真编排 → 真采用账；后一笔把事务回调里的断言搬到回调外，architecture 门禁拓到的） |
| `8a615270` | UC-PS-003 一致性节补一句「同来源更正以来源所有者声明的更正关系识别」，指向 ADR-0117；硬句不动 |
| `617047e8` | 双轴评审修补：迁移 0017 加自引用外键（被取代版本必须是同租户同包裹同种类下登过的一版：链不跨种类、不悬空）；CHECK 用例补两格；ADR 去一处计数与一处无锚断言 |
| `8a5846d1` | 机制清点在 `617047e8` 干净检出重生成（parcelshipment 测试 132→133；迁移 parcel_shipment 16→17；生产文件面与端口声明不变） |
| `2ddce88f` | auto-reroute-demo-reachability/01 复核结论与拆票（票二，见该目录） |

**五问各答**（细节在「裁决」与 ADR-0117）：① 同来源更正 = 来源自报更正关系 + 种类相同 + 所指恰是链尾；② 采用账只插不改地长链，链尾按回指派生，根唯一 + 每版至多被取代一次两条部分唯一索引 + 自引用外键；③ 承诺调整版本与采用判断版本同笔，`RestateOnCorrectedIntake`；④ 消费方按信封版本读回，TF 加端口外读法；⑤ `AT-PS-049` 一字不动，依据版本从首登换成链尾；两格例外 `CORRECTION_PREDECESSOR_UNJUDGED`（未决）/ `CORRECTION_TARGET_NOT_CURRENT/<种类>/<链尾>`（不采用）。

**与派单字面不同的一处**：派单写「UC-PS-003 验收表措辞若需对齐只改那一行」，实际改的是「一致性、幂等与并发」节的那一条（`AT-PS-050` 表行原句已经对，缺的是判据一句），一处一句。

**验证**（干净 detached 检出 `2ddce88f`，本机 Windows；数字只作此刻取证）：`gofmt -l .` 空；`go build ./...` / `go vet ./...` 退 0；无 DSN `go test -count=1 ./...` 98 包 ok / 0 FAIL（PG 用例跳过）；含 DSN `go test -count=1 -v ./internal/parcelshipment/... ./cmd/parcel-dispatch/... ./cmd/parcel-api/... ./internal/architecture/... ./migrations/...` **1723 PASS / 0 SKIP / 0 FAIL**；探针 `TestASameSourceCorrectionSupersedesAndACompetingSourceIsStillRefusedInTheDatabase` 带 DSN `--- PASS` / 不带 `--- SKIP`；机制清点在同一检出重生成与提交件零差。`-race` 未在本轮跑（本机 Windows 无 cgo，WSL 够不到门禁容器；见 workflow.md 本机环境），由 CI 覆盖。

**越权风险点**（供用户复核，与「裁决」节同）：① 「同来源更正」的识别权判给来源所有者的更正声明；② 责任起点唯一索引从「adopted」改为「adopted 且无回指」；③ 更正版本在取消 / 基线 / 资格三道门被拒时根采用不动，PS 不替人裁。另：TF 适配器加了一个端口外的读法，理由见方法注释，不是忘了往端口里加。

## 与本票相邻、但不在本票的

- 段侧「来源更正 → 参与关系重派生」是 TF 自己的半边，在 tf-segment-lifecycle-closure/10。
- 节点收寄那一路（`node-operations`）今天不落更正版本，`Corrects` 对它恒缺席；它若日后长出更正链，PS 这一侧只要它的消费方适配器把回指译进 `Corrects`。
- 判断账的提交版本维归 first-tenant-runway/10；本票的链是同族「判断版本化」的「甲」方向，不实施它。

## Comments

- 2026-09-04 · MCP-3：立票。起因是 tf/08 落更正链时按 MCP-1 指令核 PS 采用口对第二版本的处置——与裁决预期不同，
  以 PS 票面为准；TF 侧照裁决落新版本并重交意图，**不改 PS**。只写票面，未动代码。
- 2026-09-04 · 通道 5（task-f9e0bd40）：五问裁决落 ADR-0117，按 /implement（/tdd + 双轴评审）落到分支 `mcp5-ps-lc24`，
  转 resolved；完成记录如上。main 上的 SHA 待 MCP-1 重放后补记。
