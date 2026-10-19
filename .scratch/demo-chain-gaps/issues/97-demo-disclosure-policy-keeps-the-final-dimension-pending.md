# 97 演示披露策略把 `SYN-ACCOUNT-01` 的终局一维登成待确认，有效交付之后客户视图的终局仍不展示

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg97（2026-10-19 Claude Code 子代理立：演示动线重走，[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md)「重走（钉 `8197fa2f`，2026-10-19）」停点三）。
Blocked by: 无（客户在动线上看得见这一维，要等 [96](./96-customer-tracking-view-has-no-first-party-customer-intake.md)）
地盘：种子 `scripts/demo-seeds/data/visibility/`（披露策略新一版）、`seed.sh` 一行、种子 README 组成表 `data/visibility/` 那一行。
出处：VE CONTEXT 生命周期「当前投影、客户可见 ETA、客户服务终局或已批准披露内容首次满足展示条件 → 形成……客户视图版本」；[UC-VE-008](../../../docs/application/visibility-exception/UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md)。

## 现象（实测，钉 `8197fa2f`，只记 `S`）

有效交付登记、PS 终局 `EFFECTIVE_DELIVERY` 形成之后，VE 派生了新投影（带 `DELIVERED` 与 `final-outcome-adopted` 两条），客户视图随之出新版本，终局一维仍是 `PENDING_CONFIRMATION`；里程碑与 ETA 两维 `SHOWN`，说明 `NOT_DISCLOSED`。

成因：种子 `data/visibility/07-disclosure-policy-v1.json`（`SYN-VE-DISCLOSE-V1`）给两户的终局都登了 `PENDING_CONFIRMATION`；`DeriveCustomerViewHandler` 经 `dimensionsFrom` 逐维照策略取，不看投影里有没有终局。

## 三类

- 租户取值：演示租户对 `SYN-ACCOUNT-01` 的终局披露。种子以新一版策略（生效时点晚于 v1）把这一户的终局改成 `SHOWN` 并给内容来处；`SYN-ACCOUNT-02` 保留待确认，留一户作对比。证据记 `S`。
- 机制（要裁，见下）：策略答 `SHOWN` 而投影里还没有终局时，终局一维答什么。
- 产品策略：无。

## 完成判据

- 灌种子后，演示包裹有效交付之后客户视图的终局一维为 `SHOWN`；`SYN-ACCOUNT-02` 照旧待确认。
- v1 原样在册，版本轴可见。

## 要裁的

- 今天视图只看策略：策略一旦对终局答 `SHOWN`，包裹还在途时终局一维也是 `SHOWN`。改成「策略允许且投影里已有终局才展示，否则待确认」是 VE 的判法，归 VE owner；若裁定要改，另起机制票，本票只做种子。

## 规模

S（种子）

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg97`，基于 `origin/main` `5f0aa66d`）

授权：用户原话「你作为业务和系统专家自决……解决所有的问题」。[handover-dispatch-plan](../handover-dispatch-plan.md) 与 [tf-chain-plan](../tf-chain-plan.md) 都没有裁这一票，下面的代决与越权风险点以本节为准；没有另起 ADR，理由见代决二。

### 判断

两样都缺，不是二选一。

- **种子缺一格租户取值**：v1 给 `SYN-ACCOUNT-01` 的终局登的是待确认。策略不换版，这一维永远不展示。
- **机制缺一道**：`DeriveCustomerViewHandler` 逐维照策略取，不看投影。只补种子的话，`SYN-ACCOUNT-01` 的包裹在途时终局一维就答展示，带上内容来处，而 PS 还没有终局。这违反 CONTEXT「它不得从内部案件、原始来源消息或单一交付扫描自行推导终局」与 `AT-VE-160`。
- 这一道是执行 CONTEXT 已写的不变量，不是 VE owner 还没作的产品取舍。所以归机制，在本票里做，不另起机制票（代决一）。

### 做了什么

- **领域**（`internal/visibilityexception/domain`）：
  - 两个常量 `FinalOutcomeAdoptedFactKind`、`FinalOutcomeNotAdoptedFactKind`。终局两格事实类型的字面量从 PS 译装器挪进领域包一处定义，译装器改引常量。
  - `TrackingProjection.CarriesCustomerServiceFinal`：当前有效条目里有没有来源为 PS、类型为已采用的终局。
  - `FinalDimensionOver`：策略对终局一维答展示、投影里却没有终局时，降为待确认；策略答待确认或不展示的原样放行。
- **编排**：`dimensionsFrom` 多收投影，终局一维过 `FinalDimensionOver`。端口、存储、线格式、装配都没动，没有迁移。
- **种子**：
  - `data/visibility/14-disclosure-policy-v2.json`：`SYN-VE-DISCLOSE-V2`，2026-03-01 起生效。`SYN-ACCOUNT-01` 的终局 `SHOWN`，内容来处 `SYN-VE-CONTENT-FINAL-OUTCOME-V1`；`SYN-ACCOUNT-02` 照抄 v1 那一行。
  - `seed.sh` 在 VE 一段末尾追加一行（必须登在 v1 之后）。
  - 种子 README 组成表 `data/visibility/` 一格改写，故事第 5 条末尾追加四行。
- **另立票**：[103](./103-a-rederived-final-outcome-enters-the-projection-without-its-predecessor.md)，PS 终局重派生进投影时不带前身。

### 代决

1. 终局一维要「策略准许展示」且「投影里有 PS 已采用的终局」才展示，否则答待确认。票面「要裁的」原写「归 VE owner；若裁定要改，另起机制票，本票只做种子」，改为在本票里做，理由见上「判断」。
2. 不另起 ADR，也没有补记。判法执行的是 CONTEXT 既有句（客户视图只基于 PS 提供的客户服务终局；某一维信息待确认只对该维答待确认）与 UC-VE-008 的 `AT-VE-159`、`AT-VE-160`，可逆，只有一处实现。没有已接受 ADR 裁过视图四维怎么派生，补记无处可挂。
3. 策略准许而终局还没形成时，答待确认，不答不展示。不展示是一次披露决定，而策略恰恰准许了；终局一形成，同一份策略就要展示它。
4. 「有终局」只认来源为 PS、类型为已采用的当前有效条目。不采用判断不算，别的上下文借用同一字面量也不算。不看归类：演示映射没登终局这一类，终局照样形成。
5. 字面量挪进领域包：领域认读与译装器写入必须是同一处定义，各抄一份时改名无声失配。
6. 只给终局一维加这一道。ETA 一维要的是 `UC-VE-003` 的对客可见判断，那份判断不在投影里（`AT-VE-158`，AT 盘点记为机制未落地），不在本票补。
7. 种子 v2 的生效时点取 2026-03-01：晚于 v1 的 2026-01-05，早于任何演示运行。视图按投影派生时点选版本，动线上的包裹都落在 v2。v2 不带终点，登记时接续闭合 v1。`SYN-ACCOUNT-02` 照抄 v1：v2 若没有它那一行，它会四维全落未配置。
8. 种子序号 14 是自取的：本票要点没给种子序号，取号前查过本地各分支 `data/visibility/` 下都没有 14 及以上。并行的票若也往这一目录加种子，要协调方对号。

### 越权风险点

1. 视图判法归 VE owner。票面原意是另起机制票、由 owner 裁，本票直接做了（VE owner）。
2. 「有没有终局」靠认 PS 译装器写入的事实类型。PS 终局日后多出一种改变「当前有效终局」的记录（ADR-0138 的终局失效即是一例），这里要跟着改（VE、PS owner）。
3. 判断按「任一已采用条目在场」答有。同种类重派生后新旧两代都在场，答案仍对；终局失效落地后被失效那一代仍在场，会误答有。前身没译进投影这一半记在票 103（VE owner）。
4. `SYN-ACCOUNT-01` 的终局展示是演示租户的取值，`SYN` 值，证据只记 `S`。

### 用例

- 领域：`TestTheFinalDimensionShowsOnlyWhenTheProjectionCarriesAnAdoptedFinal`，表驱动：在途、只有有效交付、终局已采用、只有不采用判断、别的上下文借用字面量、策略不展示、策略待确认。点名 `AT-VE-159` 的展示半边与 `AT-VE-160` 的终局半边（评审后改，见「评审记录」）。
- 编排：
  - `TestAPermittedFinalPendsUntilTheProjectionCarriesTheAdoptedFinal`：点名 `AT-VE-160` 的终局半边与 `AT-VE-159` 的展示半边。在途、有效交付已进投影而终局未采用这两版，终局都待确认、不带内容，其余三维照展示；终局进投影后新版本指回有效交付那一版、终局转展示。有效交付那一拍是评审后补的。
  - `TestANotAdoptedFinalJudgmentDoesNotShowTheFinal`。
  - `TestAFinalThePolicyDoesNotShowStaysUnshownAfterItForms`。
  - 夹具 `trackedProjection` 改为带一条已采用终局，既有用例「四维全展示」的语义不变；在途另用 `inTransitProjection`。
- 变异检查：把 `FinalDimensionOver` 改成恒放行，上面领域那个与编排前两个失败；改回即过。

### 验证

- `go build ./...`、`go vet`、`gofmt -l` 干净。
- 真库：`go test -p 1 -count=1 ./internal/visibilityexception/... ./internal/architecture/ ./cmd/parcel-dispatch/ ./cmd/parcel-ve-register/` 全过。`./scripts/demo-seeds/...` 无测试文件，编译通过。
- 一次性库实走（实测，只记 `S`）：
  - 库 `dcg97_walk`，`seed.sh` 原样灌入退 0。`disclosure-policy` 两行都答 `REGISTERED`，库上 v1 区间止于 2026-03-01，v2 不带终点。
  - 共享集成客户端发行方仍答 `invalid_client`，另起私有 Dex：同一份 `integration-dex.yaml`，宿主端口 5572，`iss` 不变。`parcel-api` 起在 `:19197`，照动线「前置」（读写隔离开关同取 `SYN-TENANT-01`、治理自授权、三族发行方）；dispatch 七个变量照「前置」。取证时段 21:10Z–21:14Z。
  - 提交：`submit-one-shipment.sh` 答已提交 `SHR-XXNTHQYTUOTULF55YEK6BJTBKQ`，下一拍`已接受`，计划 `RPV-000000000001`，包裹 `DPCL-VVFIXT2YHFED3VN65FNUI5SVEU`，账户 `SYN-ACCOUNT-01`。
  - 客户视图首版（投影里只有初始路由）：里程碑、ETA `SHOWN`，终局 `PENDING_CONFIRMATION`，说明 `NOT_DISCLOSED`。v2 准许展示终局，投影里没有终局，这一维答待确认。
  - 预报 `PREADV-INRGRJ23DZ35P7TNGTZJVNTS4A`，带它收寄答 `201 INTAKE_FORMED`。视图出第二版、指回首版，终局仍待确认。
  - 新加坡尾程设备登尾程交接：`plannedSegment` `#4`、`FINAL_DELIVERY`、`releasingNode` SIN-HUB，答 `201 HANDOVER_REGISTERED`，读数 `NOT_GUARDED`。派送任务随交接信封形成。视图第三版，终局仍待确认。
  - 派送尝试答 `201 ATTEMPT_RECORDED`；有效交付答 `201 DELIVERY_REGISTERED`，读数 `NOT_GUARDED`、`at` SIN-HUB。
  - PS 终局一行，已采用、当前。投影多 `effective-delivery` 与 `final-outcome-adopted` 两条。当前客户视图 `CVW-BS32OF5AKEFX66HWGBC3G56JKE` 指回 `CVW-VWFQBAAM557NQLDK3HSMLHTBPM`：终局 `SHOWN`，内容来处 `SYN-VE-CONTENT-FINAL-OUTCOME-V1`，其余三维不变。`customer-view.published` 每版一封。
  - 版本轴：操作者渠道 `GET /visibility-catalogues?kind=DISCLOSURE_POLICY` 答 200，两版并列。V2 不带终点；V1 `effectiveTo` 2026-03-01，条目原样。`SYN-ACCOUNT-02` 在 v2 下终局仍 `PENDING_CONFIRMATION`。
  - `GET /customer-tracking-view` 用首方客户令牌答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`，同票 96。
  - 走完已停进程、删私有 Dex 容器、删库，二进制与令牌已清。

### 偏离与待裁

- 票面地盘只写了种子，本票另动了 VE 领域与编排（代决一）。
- 有效交付之后、终局采用之前那一版视图（`CVW-VWFQBAAM557NQLDK3HSMLHTBPM`）只存在了约四秒就被终局那一版替代。视图库只存当前版，实走没读到它的维度；这一格由编排用例钉（评审前编排用例没有这一拍，评审后补上）。
- `AT-VE-159` 的「及其依据版本」与 `AT-VE-160` 的「可展示获准交付里程碑」两半今天都没有实现：视图每一维只带策略给的内容来处（演示里终局是 `SYN-VE-CONTENT-FINAL-OUTCOME-V1`），不带 PS 终局的依据版本，也不列里程碑本身。测试只按半边点名。依据版本要进视图，得先裁视图维度带不带源侧版本，没有另起票。
- `SYN-ACCOUNT-02` 「照旧待确认」只在策略目录上取证，没有为它另走一单。
- 客户在动线上看见这一维仍要等票 96。本次取证读的是 VE 库与操作者渠道的目录读口。
- 顺带观察：通知策略种子 `visibility/03` 按 `SYN-VE-DISCLOSE-V1` 登，而披露决定带的策略引用取的是异常披露规则版本（`SYN-VE-EXC-DISCLOSE-V1`）。通知编排按后者查，会答未配置。与本票无关。评审核过属实，已立票 [107](./107-demo-notification-policy-names-the-view-disclosure-policy.md)。
- 机制清点没有重生成，进 main 时统一做。

## 评审记录（2026-10-19，隔离评审一轮，作者子代理修订，分支 `claude-dcg97`）

阻断 0 项。非阻断 5 项：3 项改代码或注释，2 项落在票面；没有不采纳的。

1. **AT 半条点名**（评审建议进 main 前修）：已修。两处测试把 `AT-VE-159`、`AT-VE-160` 整条点名，实际只钉了终局一维那一半。点名改为「`AT-VE-159` 的展示半边」「`AT-VE-160` 的终局半边」，引文照 UC-VE-008 写全，并在 Covers 里写明交付里程碑与依据版本两半没有钉。编排用例原本没有有效交付条目，对不上 `AT-VE-160` 的前提，补了一拍：在途 → 有效交付已进投影、终局未采用 → 终局采用，前两版终局都待确认。依据版本那一半今天没有实现，记在上面「偏离与待裁」，没有另起票。
2. **`Handle` 头注折行**：已修，措辞不动。
3. **`CarriesCustomerServiceFinal` 头注末句**：已修。写明这是调用方前提、不是类型不变量，并照符号指名：`DeriveTrackingProjection` 与 `Rederive` 不滤，过滤是 `DeriveProjectionHandler` 经 `CurrentlyEffective` 做的。
4. **通知策略种子挂错版本**：已立票 [107](./107-demo-notification-policy-names-the-view-disclosure-policy.md)（bug，needs-triage）。评审建议由协调方在号段内立票，本次没有给号，按「号接现有最大号」自取 107：取号前查过本地各分支本目录，最大到 106（dcg98）。进 main 前请协调方对号。
5. **票 103「要裁的」第二条**：已注明。今天 `FormParcelFinalHandler` 只在同来源种类时走重派生，换种类走不采用，这一格只在 ADR-0138（Proposed）落地后才可能出现。

复跑：`go build ./...`；`go vet ./internal/visibilityexception/...`；`gofmt -l` 干净。真库 `-p 1 -count=1` 下 `./internal/visibilityexception/...` 与 `./internal/architecture/` 全过。变异检查：`FinalDimensionOver` 改成恒放行后，领域表驱动里策略准许而没有已采用终局的各格全部失败，编排的 `TestAPermittedFinalPendsUntilTheProjectionCarriesTheAdoptedFinal` 与 `TestANotAdoptedFinalJudgmentDoesNotShowTheFinal` 失败；改回即过。种子与机制清点本轮没动。
