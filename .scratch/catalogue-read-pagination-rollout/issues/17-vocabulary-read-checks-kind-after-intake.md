# 17 商业发布词表口先 Intake、后校验 kind

Category: bug
Status: resolved——2026-11-09 进 main（`8b2885be2`）
Blocked by: 11（同一个文件的头注由 11 改过，11 进 main 之后再动）——已满足，11 随 `098767b1e` 进 main
地盘：`internal/partycommercial/adapters/http/publication_vocabulary.go` 及其用例。
出处：ADR-0055 决定三；11 的非作者评审（钉 `5b91b6517`）。

## 为什么

`GET /commercial-publication-vocabularies?kind=` 先校验 `kind` 再 Intake：渠道未配置时，集合外的 `kind` 答 400、合法的答 403，没有凭据的调用方
凭答复的不同就能探出类别的封闭集，与 ADR-0055 决定三「答复不随请求内容变」不符。迁移目录读口时，各票已把同形的选择器校验挪到 Intake 之后；这一口不是
目录读口，没有一并改。

## 做什么

- 先 Intake，再校验 `kind`：配置了的渠道上集合外或缺席照旧答 400、照旧带逐格问题；渠道未配置时与合规的一问逐字节同答 403。头注改成现状。
- 顺手查一遍其余上下文的非目录读口有没有同形的「先校验、后 Intake」；有就列进本票一并改，量大就另立票。

## 完成判据

- 端点用例：渠道未配置时，缺 `kind`、集合外 `kind`、合法 `kind` 三问逐字节同答 403；配置了的渠道上前两问照旧答 400 带逐格问题。

## 完成记录

### 落点

| 笔 | 文件 | 做了什么 |
|---|---|---|
| `940a32ef2` | `internal/partycommercial/adapters/http/publication_vocabulary.go` 及用例 | Intake 挪到读 `kind` 之前，头注改成现状。新用例 `TestVocabularyEndpointAnswersTheSame403ForEveryKindWhenUnconfigured`：缺席、集合外、小写与合法的 `kind` 逐字节同答 403（状态、体、`Content-Type`）；`TestVocabularyEndpointRefusesAnUnknownKindWithAFieldProblem` 改在放行的 Intake 上证 400 带 `kind` 那一格的问题 |
| | `internal/transportfulfillment/adapters/http/query_carrier_first_effective_pickups.go`，用例在 `judge_carrier_first_effective_pickup_test.go` | 普查查到的同形一口：收寄链的 `object` 原先先于 Intake 判。改成先 Intake；新用例 `TestTheCarrierPickupChainAnswersTheSame403ForEveryObjectWhenUnconfigured` 证缺席、空值、空白与合规四问逐字节同答 403 |
| | `internal/transportfulfillment/adapters/http/query_handover_scope_summary.go` 及用例 | 同形第二口：交接范围汇总的 `scope`。头注写着「方法 → 请求形状 → 准入」，`TestHandoverScopeSummaryRefusesABlankScopeBeforeTheChannelCheck` 钉的正是旧次序。改成先 Intake；旧用例拆成 `TestHandoverScopeSummaryRefusesABlankScope`（放行时缺席、空值、空白答 400）与 `TestHandoverScopeSummaryAnswersTheSame403ForEveryScopeWhenUnconfigured` |
| | `internal/transportfulfillment/adapters/http/catalogue_intake.go` | 本包查阅面门次序的头注还写着「方法 → 册名 → 准入」，05 迁移后册名早已在 Intake 之后判；改成现状「方法 → 准入 → 请求内容」。交接范围汇总照的正是这句旧注释 |
| `88a3eb272` | `cmd/parcel-api/endpoints.go` | `probe` 的注释写成现状。原说探针是「走得到 Intake 的请求目标」；三口挪完，不带参数的路径本身就走得到 Intake，探针还得留着，是因为操作者查阅读的装配用例要一问过得了本行请求形状、答到读口那一格 |
| 本笔 | 本票面 | 完成记录与 Status |

### 普查：别的非目录读口有没有同形的「先校验、后 Intake」

取证于基 `6191110eb`，两路：

- 读代码：`internal/*/adapters/http` 与 `cmd/parcel-api` 里每个端点构造、每个共用外壳，看 Intake 之前跑了什么。除上表三口，都是方法门之后紧跟 Intake 或认证；各上下文的 Intake 实现先认证、后读载荷。委托查阅、复核队列与渠道择优按查询键在不在分派，分派先于 Intake，两支未配置时同答 403，不算校验。
- 跑装配：登记处每一行带探针的，拿不带参数的路径去问全未配置的装配（一次性用例，跑完即删）。改前只有交接范围汇总答 400 `MALFORMED_REQUEST`、不答未配置的 403；改后全部答 403。收寄链与词表口的探针带着合规参数，这一路查不出来，是读代码查到的。

不在本票：装配处的请求目标检查（`httpapi.RefuseUnstorableReadTarget`）在一切 GET 读口之前拦含 NUL 或不合法 UTF-8 的目标、答 400。那是对整条目标的编码检查，对每一口同答一份，理由在它自己的头注（票 document-numbering/15），不是某一口的逐格校验。

### 完成判据对照

- 渠道未配置时缺 `kind`、集合外 `kind`、合法 `kind` 逐字节同答 403：`TestVocabularyEndpointAnswersTheSame403ForEveryKindWhenUnconfigured`，另加小写一问。
- 配置了的渠道上缺席与集合外照旧答 400 带逐格问题：`TestVocabularyEndpointRefusesAnUnknownKindWithAFieldProblem`。
- 反证：把运输履约两口临时换回旧次序，两条新用例都失败（缺席一问答 400）；换回来再过。

### 验证

`go test`、`go vet` 过两个端点包与 `cmd/parcel-api`，`gofmt` 无输出。全量真库与种子在进 main 那一轮跑。

## 评审记录

- 2026-11-09 非作者两轴评审（推送方 Claude Code 会话另起的隔离子代理，只读，钉分支尖 `ef6f5e31d`，基线 `6191110eb`，17、18 合看）。跑过：全仓 `go build`、`go vet`；`cataloguepage`、十二个上下文的 `adapters/http`、带 DSN 的 `cmd/parcel-api` 各 `go test -count=1`，全 ok。
  - Spec 轴：无阻断。核过 17 的三口先 Intake 再判 `kind` / `object` / `scope`，未配置渠道一律同一个 403；18 的目录解码全经 `ParseQuery` / `DecodeRaw`，新调用都在 Intake 之后，选择器的库存判在进读口之前，坏段答 400 不回显原值。非阻断一条：坏转义的理由说「不完整」，而 `%zz` 是无效转义。
  - Standards 轴：阻断一条——`da7fa5f48` 删了 `decodePageQuery` 连同说明「为什么在 Intake 之后解码」的头注，价格五个读口的头注仍指着它。非阻断两条：`businessEndpointProbe` 的注释仍以「先于 Intake 判的分派参数」为由，与 `endpointLine.probe` 已改的说法不一；`cataloguepage` 用例的 Covers 行引了 ADR-0144 里没有的原句。
  - 修订：推送方在重放线上修于 `8b2885be2`（五处头注改为直接写理由与出处 ADR-0055 决定三；理由与用例名改说「无效」；两处注释对齐；Covers 行只写决定号）。同一评审者在 `8b2885be2` 上复评 Standards 轴：无发现；并用 `git patch-id --stable` 核过重放的四笔与它评的四笔内容一致。

## 进 main 记录（2026-11-09，推送方 Claude Code 会话）

- 重放：在隔离 worktree 里把分支四笔 cherry-pick 到 `origin/main` 尖 `a0305c7d4`，无冲突（自分支基点以来 main 上没有与这四笔重叠的 `.go` / `.sql`）。分支 `940a32ef2` → main `c9f08fafc`（17）、`88a3eb272` → `08cf2a718`（17）、`da7fa5f48` → `889ff5ffe`（18）、`ef6f5e31d` → `a2da99917`（票面）；评审修订 `8b2885be2`。推 `8b2885be2:main`。另一处早先的重放（`crpr-land/1718`，基 `a74aa0e39`）没有用，main 已越过它的基点。
- 生成物：`adr-index -check`、`at-coverage-counts -check` 退 0；机制清点重生成无变化，未成笔。
- 全量真库：`scripts/test-full.sh` 在 `a2da99917` 与 `8b2885be2` 上各跑一次，都退 0、175 个包 ok、无 FAIL。
- 种子：`scripts/demo-seeds/seed.sh` 在 `a2da99917` 上灌进一次性库（`seed_1718_*`），退 0，用后删库。`8b2885be2` 只改注释、一句理由文字与用例名，种子未重跑。
