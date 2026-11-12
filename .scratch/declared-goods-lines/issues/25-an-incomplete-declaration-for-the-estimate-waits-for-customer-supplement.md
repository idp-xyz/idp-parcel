# 25 估价因申报不全没形成时等客户补充

Category: enhancement
Status: resolved——2026-11-12 进 main
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
## 进 main 记录

- 2026-11-12 进 main，main 尖 `c5001b3de`。重放基线：main 在 `c6e00cc4b`（分叉点 docs `8d6f5ffba`、代码 `f55370512`）。main 上的提交：docs 链 22 笔重放 `d43dd28a4`…`145718838`（末笔是落地改号——分支 0378、0380 改为 ADR-0382、ADR-0383，用户 2026-11-12 裁定 main 的 ADR-0378/0380 保号）；落地对齐一笔 `a5a10ea51`（ADR-0382、ADR-0379 挂先进 main 的 ADR-0380 对齐注记，ADR-0380 越权风险点 5 补落地指针，商业票 18 备注改号结果）；代码链 42 笔重放 `05e1ecfe1`…`f10e46b6e`；生成物三笔：ADR 索引 `7fc9cf179`、AT 盘点总览 `b1125f29e`、机制盘点 `8b9c29506`；清理一笔 `c5001b3de`（删随重放误入库的临时助手脚本，只动 `.scratch` 下的 `.py`/`.sh`）。出处：分支 `docs/declared-goods-lines` 尖 `670f21a5f`、`feat/goods-details-admin` 尖 `086a513b2`（均已推远端），工作树 `idp-parcel-landing`。
- 重放与并法：cherry-pick 逐笔重放，冲突按 main 的 ADR-0380 越权风险点 5 的规则并——同一句两边各挂注记的并排写、两份替换文字合成一句（叠加读法：逐个来源问，每个来源之内先指名后不声明选到底）。docs 侧与分支尖有刻意差异的 19 份文件都是这一类语义合并（ADR-0179/0190/0229/0260/0368、UC-PP-001、UC-PC-002、UC-SA-002、CONTEXT-MAP、GLOSSARY、计价/商业/结算/仓配 CONTEXT、开发主线、冻结清单、AT 盘点，及 0379/0382 落地对齐注记）；代码侧只有 `docs/README.md` 与 `internal/partycommercial/ports/ports.go` 两份共享文件带 main 侧内容，其余链文件与分支尖逐字相同。
- 生成物：`adr-index -check` 与 `at-coverage-counts -check` 退 0，机制盘点重生成后再跑无变化。
- 门禁（在 `8b9c29506` 上跑，`c5001b3de` 只删 `.scratch` 脚本、不进构建与测试）：`scripts/test-full.sh` 全绿（175 包 ok，EXIT=0，专用一次性 PG）；admin-web 三道门（Node 24）：`tsc -b --noEmit` 干净、`node scripts/run-tests.mjs` fail 0、`vite build` 成；`scripts/demo-seeds/seed.sh` 对一次性 PG 全程零报错（33/33 项落库）。
- 推送：推的是确切 SHA `8b9c29506`，随后收口清理推 `c5001b3de`；两推之间 main 无他人提交。
- 评审：链上各票均经非作者隔离评审与修订（各票「评审」节）；收口与进 main 记录是票面簿记，自审。
- 票 28（只有实重规则的卡在缺尺寸时仍停「没有合计」）留 needs-triage，不随本链。
