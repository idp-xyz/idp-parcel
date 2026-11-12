# 25 估价因申报不全没形成时等客户补充

Category: enhancement
Status: in-progress
Blocked by: 无（「要定的」两项已由 [ADR-0381](../../../docs/adr/0381-an-incomplete-estimate-declaration-waits-for-customer-supplement-or-operator-registration-by-who-supplies-the-gap.md) 定下；实现在票 26、27）

依据：[ADR-0383](../../../docs/adr/0383-an-estimate-the-price-card-side-cannot-form-waits-for-operator-registration-and-the-attempt-records-why.md) 决定一与越权风险点「申报不全照旧内部重试、用完放弃」。

## 现状

结算答 `ESTIMATE_DECLARATION_INCOMPLETE`（申报缺了计价要的某一格，例如价卡绑了始发维度目录而没报寄件邮编）时，小包托运照旧内部重试，整笔回滚、重投用完放弃，委托悬着、看不出缺什么。

## 要定的

- 怎样接进受理判断的客户补充那一路：续办方是客户，缺项说清哪一件缺哪一格（同「必要资料」组的缺项，ADR-0376）。
- 计价的「申报缺哪一格」怎样经结算带回来，今天只有一个原因码。

## 不做的

价卡一侧的未形成（票 23）。

## 决定（2026-11-12）

按「缺的那一格该谁补」分流，写成 [ADR-0381](../../../docs/adr/0381-an-incomplete-estimate-declaration-waits-for-customer-supplement-or-operator-registration-by-who-supplies-the-gap.md)：

- **本票「现状」里举的例子归错了格**：价卡绑始发维度目录而没报寄件邮编的，今天计价组出空始发的路线、评成 `ZONE_UNRESOLVED` 的「有试算无合计」，经结算折成 `ESTIMATE_TRIAL_WITHOUT_TOTAL` 落在票 23 的等运营登记那一路，根本走不到 `ESTIMATE_DECLARATION_INCOMPLETE`。ADR-0381 决定二让计价在估价口把「卡要而申报没给」一律点名为缺项（封闭集加 `ORIGIN_POSTAL`、`DIMENSIONS` 两码），这个例子此后按申报不全走。
- 计价点名的缺项里只有客户补得了的（测量、尺寸、收件邮编、寄件邮编）停进`等待受控补充`：财务控制那一步先存等待、缺口按件或按委托级记在任务上（「接受前财务控制」组，四个本上下文原因码），受控补充成新提交版本后续办。缺分区、缺地址性质是这张卡做不了受理前估价，归等运营登记（票 23 那一路）；两类同场时等运营登记先；一格都点不出的照旧内部重试。
- 逐件缺项由结算原样带回（不折叠不改写）；等待落库的处理尝试照 ADR-0383 决定三另记两格，再记缺项码。
- 实现拆成票 26（后端，跨计价、结算、小包托运）、27（管理台），本票收口以两票为准。

## 收口（2026-11-12）

- 实现两票均完成并过非作者评审：
  - 票 26（后端，跨计价、结算、小包托运）：作者提交 `5c5eda0ed`（分支 `feat/estimate-incomplete-waits`），评审无阻断无应改，拣入 `feat/goods-details-admin` 为 `cd2474100`（迁移 `0039` 两边 sha256 一致）。
  - 票 27（管理台）：`feat/goods-details-admin` 上 `8e9054004`，评审无阻断、两条应改随评审修订 `d93e85a01` 落，详见票 27 记录。
- 拣入后合并头全量门禁（代码工作树，含 PG 真库、串行 `-p 1`）：`gofmt -l` 空、`go vet` 过、`go build ./...` 过、全量 `go test ./internal/... ./cmd/... ./referenceconfig/... ./migrations/... -count=1` 退 0。管理台三道门禁在分支头 `d93e85a01`（Node 24）：`tsc -b --noEmit` 退 0、`node scripts/run-tests.mjs` 1414 过 0 败、`vite build` 退 0。
- 落点：代码在 `feat/goods-details-admin`（头 `d93e85a01`，叠 `cd2474100` → `8e9054004` → `d93e85a01` 三笔）；ADR-0381、ADR-0383 部分停用注记、PS/PP 文档改动与票面在 `docs/declared-goods-lines`。均未推送、未进 main；代码注释引 ADR-0381，合并时文档先行或同批。
- 衍生：票 28（needs-triage）——票 26 评审发现的既有边界：只带实重规则的卡在无尺寸申报时经急读包裹派生量落无合计、归等运营登记，登记解不了、客户也收不到信号；不在本链内，另行承接。
- 未做（照各票「不做的」原样留存）：「委托待受控补充」信封（ADR-0371 决定三，已记 UC-PS-001 交接表）；补充届满处置；演示种子不加这条路；浏览器手点；按原因或计价范围收窄重驱。

## 评审

- 非作者评审一轮（2026-11-12）：阻断一处已改——删掉「多张卡取缺项并集」条款（受理前估价只解析唯一在用卡，试算照 ADR-0152 逐卡交回，并集没有发生地、落下去会无指针撞 ADR-0152），对应越权风险点一并删。另按应改修订：封闭集外的缺项码报错交人处理、不静默归内部重试；`ORIGIN_POSTAL` 的判据读在用分区目录版本的始发维度声明（卡结构看不出按不按前缀分），没有在用版本不点名；`DIMENSIONS` 两处检测（判定条件与重量策略的体积重）；点名了与 Decide 一侧「客户侧缺口在场时先让客户补」方向相反、各管各的入口；ADR-0383 注记补「部分停用」标签与「适用场景」；「缺口逐件记在任务上」改「按件或按委托级」；换卡判据补 ADR-0234 收尾（直接另登在用卡落适用冲突）。
