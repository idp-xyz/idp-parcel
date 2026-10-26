# 147 已登过关务区域的租户漏登新节点区域时，跨区那一道静默不列

Category: bug
Status: in-progress——2026-10-25 评审无阻断、修订轮完，待进 main，分支 claude-dcg147 tip `19f3c9b7f`（修订轮代码 `4f827bd7e`、文档 `19f3c9b7f`，票面评审记录那一笔在其上；见文末「评审记录」）。此前 in-progress——2026-10-25 作者完工，待隔离评审与进 main，分支 claude-dcg147 tip `938bdbbdc`（代码 `6814c2677`、文档 `938bdbbdc`，票面完成记录那一笔在其上；见文末「完成记录」）。此前 in-progress——2026-10-25 会话 `tops-20`（Claude Code）认领，分支 `claude-dcg147`（worktree `idp-parcel-dcg147`）；新 ADR 取 0300。此前 ready-for-agent——2026-10-20 代决复核立（上线后尽快），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 R12；ADR-0206 越权风险点 12（ADR-0213 越权风险点 7 同根）

## 现象与修法

现象、证据与修法的完整写法以复核报告 R12 一条为准（单一权威），此处只记要点：

代决「是」：起讫有一端未登区域时改答「未配置」并指名那个节点；纯境内租户（一条区域都没登）不受影响。改 ADR-0206 决定二与 CC CONTEXT 对应句各加一个分支（须新 ADR 部分停用）。排在关务在途票（12、psb/18）之后。

## 与门禁读数收窄的交互（2026-10-21，票 146 隔离评审追加）

- 本票落地之后，起讫有一端是未登区域节点的出发，关务对段里不在案卷的每件答「未配置」，出发读数译成未决（ADR-0205 决定二），每件都没得出放行，于是每件都回给设备——[ADR-0237](../../../docs/adr/0237-gate-readings-show-the-executor-only-the-objects-that-did-not-pass.md) 只收窄放行与不受门禁的那几件。
- ADR-0237 决定五已让出发意图的起讫与声明的段绑定，设备挑不了别的方向的班次，所以只有段自己的路线确实经过未登区域的节点时才全列。那是如实的读数，执行方确实要拿下那几件；但问出发前问询的可以是任一场外设备，这一格归 ADR-0237 越权风险点 2 与票 [171](./171-gate-previews-are-scoped-to-the-asking-devices-work-and-leave-a-trace.md)。
- 落本票时：「未配置」那一格的依据会指名未登的节点，依据原文不回给设备（ADR-0205 决定一），不新增暴露。验收里宜补一条：起讫一端未登区域时，出发前问询回给设备的是段里每件、整格未决，并指向票 171。

## 完成记录（2026-10-25，Claude Code 作者子代理，协调方 `tops-20`，分支 `claude-dcg147`，起点 origin/main `f8b25837b`）

授权：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」。裁定的单一权威是代决复核报告 R12；落地时补的裁定写进 [ADR-0300](../../../docs/adr/0300-a-zoned-tenants-unbooked-object-answers-not-configured-on-the-cross-zone-line-naming-the-unregistered-node.md)，这里只列编号与去处。

### 提交

| SHA | 内容 |
|---|---|
| `6814c2677` | 代码与用例：CC 领域、端口、PG 读口、编排；CC / TF 消费侧适配器 / `cmd/parcel-api` 用例 |
| `938bdbbdc` | 文档：ADR-0300 新立；ADR-0206、0213 的 Status 与 Links；CC CONTEXT；UC-CC-013；UC-TF-005；参数登记册 `PAR-CUS-08`；关务专项基线 AT 区间；AT 盘点①补行与追记 |
| 本笔 | 票面完成记录与 Status |

### 做了什么

- **判法**（领域 `crossZoneMatters`，`domain/action_admission.go`）：`ActionAdmissionInput` 加 `TenantZonesInForce`。跨区判不出时，租户在动作时点已有节点登了区域、对象不在任何案件与单元里，跨区那一道答`未配置`，依据 `NODE_ZONE_NOT_REGISTERED` 指名未登的节点（两端都没登就两个都指名）。对象带跨区的门那一支照旧；其余情形照旧不列——一条区域都没登的租户、在册而不带跨区门的对象、在案未成单元的对象。
- **输入**：`ports.NodeCustomsZoneView` 加 `AnyNodeCustomsZoneInForce`（租户在评估时点有没有任一节点的区域在生效区间内，半开区间与按节点读同一句）；PG 读口 `NodeCustomsZonePointView` 用一条 `EXISTS` 查 CC `0024` 的区域表，走主键租户前缀。
- **编排**：`JudgeActionAdmissionHandler` 只在跨区待定时问这一句；问不动答读不动 `NODE_ZONE_VIEW_UNAVAILABLE`，不拿「没登」顶替。判断标识派生、判断册形状都不变。
- **TF**：无代码改动。消费侧译法（关务`未配置` → `未决`，ADR-0205 决定二）已覆盖这一格，依据原文只落册、不回设备（ADR-0205 决定一）；补了消费侧适配器与 `cmd/parcel-api` 真库用例钉住它。
- **迁移**：无。没有占号。
- **文档**：
  - ADR-0300 新立：部分停用 ADR-0206 决定二「不带：这一道不列。」一句；回答 ADR-0206 越权风险点 12、ADR-0213 越权风险点 7 声明了去向的那一半。
  - ADR-0206 Status 行索引注记里「复核记录 R12 未落地」那一段换成部分停用的前向指针，Links 加一条；ADR-0213 同样换成「越权风险点 7 一半已答」，Links 加一条。两份正文不动。`docs/adr/README.md` 未手改（`-check` 报索引过期属预期，格式无错）。
  - CC CONTEXT Rules「动作准入判断」：跨区判不出那一句改成现行规则；不在册那一句注明「跨区那一道不列」只剩租户一条区域都没登这一种。与 ADR-0300 同一笔。
  - UC-CC-013：权威依据、输入契约「节点关务区域」行、应用流程第 2 步补「租户登没登过区域」一读；`AT-CC-422` 改写（在册不带跨区门、在案未成单元），立 `AT-CC-426`、`AT-CC-427`；待决风险指针加 ADR-0300。
  - UC-TF-005 立 `AT-TF-330`（票面「与门禁读数收窄的交互」一节建议的那一条，指向票 171）。
  - 参数登记册 `PAR-CUS-08` 一句补上新分支与 ADR-0300 链接；关务专项基线 UC-CC-013 的 AT 区间 `411..425` → `411..427`。
  - AT 盘点①：`AT-CC-422` 补点名，补 `AT-CC-426`、`AT-CC-427`、`AT-TF-330` 三行与一条追记；总览计数未动（`-check` 报过期属预期）。
  - 没改 GLOSSARY、CONTEXT-MAP。

### 代决

内容与理由只在 ADR-0300：

1. 「已登记过至少一个节点关务区域」按**动作时点**读（那一刻至少有一个节点的区域在生效区间内），不按此刻、不按「任一时刻登过」：决定二，候选戊。
2. 新分支只落在**不在册**对象（R12 字面）；在案未成单元的对象不扩（每一道已答未决），在册而单元没登跨区目录的那一格不收：决定一，候选丙、庚；越权风险点 2、3。
3. 「租户登没登过」只在跨区待定时问；问不动答读不动、指名区域那一口：决定二。
4. 这一位不进判断标识、判断册不改形、不迁移：决定三，候选己；越权风险点 5。
5. 去向没声明的交出不在本票：没有节点可指名，ADR-0213 决定四不变：决定四，候选辛；越权风险点 4。
6. 依据种类沿用 `NODE_ZONE_NOT_REGISTERED`，不另立一种：同一个续办（去登区域）。
7. 三类：判法是产品策略，读册与依据存取是机制，区域取值是租户取值（`PAR-CUS-08`）；代码里没有任何租户名单、默认或开关。

### 越权风险点

编号与正文以 ADR-0300 为准：

- 1：登了一个节点就要登全，新增节点忘登时从「静默放过」变「每件未决」（CC owner、租户）。
- 2：在册对象的单元没登跨区目录、起讫一端未登区域，仍然放过（CC owner）。
- 3：在案未成单元的对象不指名节点（CC owner）。
- 4：去向没声明的交出不列跨区，ADR-0213 越权风险点 7 的另一半照旧（TF、CC owner）。
- 5：「登过」这一位不进判断记录，事后要按动作时点回读区域册（CC owner）。
- 6：每件未决的暴露，与 ADR-0237 越权风险点 2 同一格，归票 171（TF owner）。

### 完成判据 → 用例

| 判据 | 用例 |
|---|---|
| 已登过区域的租户，不在册对象起讫一端（或两端）未登区域 → 跨区答未配置并指名节点；装载出发 / 出库照旧不受门禁；声明了去向的交出同一判法（`AT-CC-426`） | 领域 `TestAZonedTenantsUnbookedObjectAnswersNotConfiguredNamingTheUnregisteredNode`；编排 `TestTheTenantsZoneRegisterIsAskedOnlyWhenCrossZoneIsUndetermined`；PG 读口 `TestAnyNodeCustomsZoneInForceReadsTheTenantsRegisterAtTheInstant`；真库 `TestAdmissionOnRealPostgresNamesTheUnregisteredNodeOnlyForAZonedTenant`（判断册依据记着节点） |
| 纯境内租户（一条区域都没登）行为不变（`AT-CC-427`） | 领域 `TestUnregisteredNodeZoneIsNotConfiguredOnlyWhenTheObjectHasACrossZoneGate`；编排、真库同上两条的纯境内那一半；`cmd/parcel-api` `TestAZonedTenantMissingANodeZoneShowsEveryUnbookedObjectAsUndecided` 末段 |
| 在册不带跨区门、在案未成单元照旧不列（`AT-CC-422`） | 领域 `TestAZonedTenantStillLeavesTheCrossZoneLineOutForBookedObjects`、`TestUnregisteredNodeZoneIsNotConfiguredOnlyWhenTheObjectHasACrossZoneGate` |
| 「登没登过」问不动答读不动、不当作没登 | 编排 `TestTheTenantsZoneRegisterIsAskedOnlyWhenCrossZoneIsUndetermined` 末段 |
| TF 现有译法覆盖这一格（未配置 → 未决，依据指名节点） | TF 消费侧适配器 `TestAnUnregisteredNodeOfAZonedTenantReadsUndecidedNamingTheNode`（出发与交出）；既有 `TestEveryCustomsAnswerHasExactlyOneDepartureAnswer` |
| 出发前问询回给设备的是段里每件、整格未决，指向票 171；依据原文不回设备（`AT-TF-330`） | `cmd/parcel-api` `TestAZonedTenantMissingANodeZoneShowsEveryUnbookedObjectAsUndecided`（问询与同参出发逐字节相同、每件在列、答复里没有 `NODE_ZONE_NOT_REGISTERED` / `AADJ-` / `CROSS_CUSTOMS_MOVEMENT`，出发落册明细每件指名深圳口岸） |
| 新 ADR 部分停用 ADR-0206 决定二，Status / Links 前向指针，CONTEXT 同笔改 | `938bdbbdc`；`go run ./tools/adr-index -check` 只报索引过期（生成预览：ADR-0300 行标〔代决·待 CC owner 复核〕，0206、0213 两行注记换新） |

### 自验（实测，证据只记 `S`）

- `go build ./...`、`go vet ./...` 全仓通过；`gofmt -l cmd internal` 无输出；`git archive` 出的 `938bdbbdc` 干净目录里 `go build ./... && go vet ./...` 通过。
- **含 PG**（`IDP_PARCEL_POSTGRES_DSN` 指 55432）：`go test -count=1 -p 4` 跑 `./internal/customscompliance/...`、`./internal/transportfulfillment/...`、`./cmd/parcel-api/...`、`./cmd/parcel-customs-register/...`、`./cmd/parcel-dispatch/...`、`./cmd/parcel-transport-register/...`（`go list` 反查出依赖 CC 或 TF 的全部 `cmd`）、`./internal/architecture/...`、`./migrations/...`：28 包 ok，0 FAIL（两包无测试文件）。新真库用例以 `-v` 逐条确认是 PASS 不是 SKIP。
- 变异：把 `crossZoneMatters` 新那一支短路掉，领域、编排、PG 真库、TF 适配器、`cmd/parcel-api` 各一条新用例变红；复原后全绿。
- `go run ./tools/adr-index -check` 与 `go run ./tools/at-coverage-counts -check` 只报生成物过期（预期，推送方重生成）；没重生成机制清点、ADR 索引与 AT 计数，没跑全量。

### 偏离与待裁

- **号**：ADR-0300；`AT-CC-426`、`AT-CC-427`（UC-CC-013 原最大号 425 之后的空号，426–430 原为票 42 预分未用）；`AT-TF-330`（UC-TF-005 原最大号 329 之后）；无迁移；没有新立票。
- **地盘之外的两处小改**：ADR-0213 的 Status 行与 Links（原注记写着「复核记录 R12 未落地」，本票落地后它会说错，故换成「越权风险点 7 一半已答」）；参数登记册 `PAR-CUS-08` 与关务专项基线 AT 区间各一处。协调方只报备过 CC CONTEXT 与 ADR-0206，这几处请协调方知会。
- **既有测试夹具**：`cmd/parcel-api` 的 `newDepartureLine` 拆出 `newDepartureLineWithZones`（只登点名的节点区域种子）、`newPreviewLine` 拆出 `previewLineOn`；既有调用不变。
- **待 owner 复核**：上列代决与越权风险点，尤其是代决 2（只落不在册对象）与越权风险点 1（登一个就要登全）。

## 评审记录

以下照抄隔离评审原文（`review-147.md`；只把原文的二级标题降为三级，挂在本节下），修订轮在后。

评审 ← tops-20 隔离子代理 · 钉 `c8183fa4e` · 23:43

结论：无阻断；三条非阻断。

### 阻断
无。

### 非阻断
- Spec P1：`domain/action_admission_test.go::TestUnregisteredNodeZoneIsNotConfiguredOnlyWhenTheObjectHasACrossZoneGate` 的 `outside` 一格（AT-CC-427 领域半边）头注写「租户一个节点都没登（零值即此）」，夹具 `unzonedDestination()` 却让 SZX 已登区域，自相矛盾。生产里这一态到不了：`Handle` 同一时点已由 `LoadNodeCustomsZone` 读到 SZX，`AnyNodeCustomsZoneInForce` 必真；AT-CC-427 的 Given 只在编排与真库两条用例上按实成立。另：`Handle` 在一端已登时仍问这一句，与 ADR-0300 决定二「问了只多一次读、多一处读不动」的理由相悖。修法：领域把「任一读数已登」并进这一位（或拒收矛盾输入），编排在一端已登时不问；测试改成两端都没登。
- Standards S1：ADR-0213 越权风险点 7 原文只有「去向没声明时跨区那一道不列」，没有「声明了去向」那一半；ADR-0300 Status 前段与 ADR-0213 索引注记都写成「7 一半已答」，实际 7 原封未动（ADR-0300 越权风险点 4 自己也承认）。建议改称「7 未答；同根的声明了去向那一格由 ADR-0300 收」。
- Standards S2：ADR-0300 Consequences「演示租户四个节点都登了区域」带计数，碰 AGENTS.md「计数与行号同构」；论点是「演示动线经过的节点都登了」，去掉数。

### 无发现（摘要）
红线、注释、ADR-0206 部分停用写法、ADR-0300 所引原句逐字存在；判法与 R12 字面一致；时点与失败代数与读区域那一口同格；依据不回设备有真库断言；重放探针（中间登第一条区域）不会错答已有判断；AT 号不撞；范围无越界。

### 修订轮（2026-10-25，作者子代理，同一分支 `claude-dcg147`，起点 `c8183fa4e`，未变基）

| SHA | 内容 |
|---|---|
| `4f827bd7e` | P1 代码与用例 |
| `19f3c9b7f` | S1、S2 与 P1 的文档半边 |
| 本笔 | 票面评审记录与 Status |

- **P1**：取「并进」，不取「拒收」。领域加 `tenantZoned`：另读的那一位，并上起讫读数里已登的那一端——同一时点读同一本册，一端已登就不可能「一条都没登」，零值因此不再与读数相抵，编排也不必替领域算这一位。编排 `Handle` 只在跨区待定、且起讫两端都没登时问 `AnyNodeCustomsZoneInForce`。拒收矛盾输入要编排在一端已登时照样传真值，等于把「一端已登即登过」写两处，所以不取。用例：
  - 领域：`TestUnregisteredNodeZoneIsNotConfiguredOnlyWhenTheObjectHasACrossZoneGate` 的 `outside` 一格改走新夹具 `unzonedBothEnds()`（两端都没登、那一位为零值），头注改成「带跨区门答未配置、没登跨区目录不列，这两格登没登过都一样；一个节点都没登时不在册对象也不列」，与夹具一致。
  - 领域：`TestAZonedTenantsUnbookedObjectAnswersNotConfiguredNamingTheUnregisteredNode` 补一格，起点已登、那一位为零值也答未配置并指名终点。
  - 编排：`TestTheTenantsZoneRegisterIsAskedOnlyWhenCrossZoneIsUndetermined` 改写，钉住五件事：一端已登时不问（问的次数为零）、仍答未配置并只指名未登那一端；两端都没登时问一次、答未配置、两个都指名；纯境内租户问一次、不列；判得出跨区时不问；那一口坏着时，两端都没登答读不动，一端已登照常形成判断。
  - 真库：`TestAdmissionOnRealPostgresNamesTheUnregisteredNodeOnlyForAZonedTenant` 补两端都没登那一格，靠区域册里别的节点认出租户登过。
  - 文档半边：ADR-0300 索引注记、决定二、Consequences；UC-CC-013 输入契约与应用流程第 2 步；AT 盘点 `AT-CC-426` 编排半边那句。
  - 变异：去掉读数那一半的并，领域与编排各一条变红；编排改回「跨区待定就问」，编排那条变红。复原后全绿。
- **S1**：改称「越权风险点 7 未答，同根的『声明了去向而去向未登区域』那一格由 ADR-0300 收」，改了这几处：
  - ADR-0300：Status 前段、「对已接受记录的作用」那一条、越权风险点 4、Links 里 ADR-0213 那一条。
  - ADR-0213：Status 索引注记那一段，以及上一笔自己加的 Links 那一条（同一句说法，不改会与注记相抵）；ADR-0213 正文不动。
  - `adr-index -check` 只报索引过期，格式无错。
  - 本票上文「完成记录」里「回答 ADR-0213 越权风险点 7 声明了去向的那一半」「越权风险点 4：……另一半照旧」两句是当时的写法，以本节为准，不回改。
- **S2**：ADR-0300 Consequences 演示一条改为「演示动线经过的节点都登了区域，演示读数不变」，不再计数。
- **重跑（含 PG，钉 `19f3c9b7f`）**：
  - `go build ./...`、`go vet ./...` 全仓通过；`gofmt -l internal cmd` 无输出。
  - `go test -count=1 -p 4` 跑 `./internal/customscompliance/...`、`./internal/transportfulfillment/adapters/customscompliance/...`、`./cmd/parcel-api/...`、`./cmd/parcel-customs-register/...`、`./internal/architecture/...`：14 包 ok，0 FAIL（`ports` 无测试文件）。
  - 本票的真库用例以 `-v` 逐条确认 PASS：`TestAdmissionOnRealPostgresNamesTheUnregisteredNodeOnlyForAZonedTenant`、`TestAnyNodeCustomsZoneInForceReadsTheTenantsRegisterAtTheInstant`、`TestAnUnregisteredNodeOfAZonedTenantReadsUndecidedNamingTheNode`、`TestAZonedTenantMissingANodeZoneShowsEveryUnbookedObjectAsUndecided`。
  - 没跑全量，没重生成 ADR 索引、AT 计数与机制清点。
