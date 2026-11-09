# 18 目录读口的原始查询串解析收进共用件

Category: enhancement
Status: resolved——2026-11-09 进 main（`8b2885be2`）
Blocked by: 02–13、15（各册端点进 main 之后再统一）——已满足，随 `098767b1e` 进 main
地盘：`internal/platform/cataloguepage`；各上下文目录端点里取查询串的那一行。
出处：ADR-0144 决定四、七；04 的非作者评审（钉 `c39f772f2`）。

## 为什么

目录端点取查询串有两种口径：parcel-pricing 的 `decodePageQuery` 照 `url.ParseQuery` 解，转义坏掉或带分号的那一段答 400；其余各上下文
照 `request.URL.Query()` 解，那一段被静默丢掉、答出整册。同一个 `?q=%zz`，两边一个拒、一个当没给。查询契约要一处定义（决定七），
不静默忽略坏参数（决定四）——前一种口径合乎这两条，后一种不合。

## 做什么

- 在 `cataloguepage` 给出从原始查询串解码的入口：解不开的段答 `*MalformedQuery`，理由散文点名「查询串里有解不开的一段」，不回显坏值。
- 各上下文目录端点改用这个入口，删掉 parcel-pricing 自己那份 `decodePageQuery` 里的解析。
- 选择器由端点自己读的那几处，读的也是同一份解好的取值，不另解一遍；选择器的取值同样按 `storable.Text` 判，库存不下的答 400（今天 `Decode`
  只把选择器记进摘要、不判，端点又只判非空，装配处的请求目标检查之外 NUL 仍会进 SQL——01、02 的修订轮复核指出）。

## 完成判据

- 共用件单元用例证坏转义与分号答 400、理由不回显坏值；各端点表驱动用例补一问 `?q=%zz` 答 400。
- 全仓再没有目录端点直接用 `URL.Query()` 喂给声明解码。

## 完成记录

一笔：`da7fa5f48`。

### 共用件（`internal/platform/cataloguepage/query.go`）

- `ParseQuery(raw)`：照 `url.ParseQuery` 解原始查询串，解不开整条答 `*MalformedQuery`，理由「查询串里有解不开的一段：百分号转义不完整，或用了分号分隔」，不回显那一段。头注写明唯一的例外：Intake 之前按查询键在不在分派的口只能照 `URL.Query()` 宽松地看——在那里严格解，坏段就先于 Intake 答 400（ADR-0055 决定三）；分派键本身写坏时，请求落进不带该键的列表那一支，那一支在 Intake 之后照 `ParseQuery` 再解。
- `(*Catalogue).DecodeRaw(raw)`：`ParseQuery` 再 `Decode`，给不自己读任何键的口用。
- `Decode` 的选择器那一格判取值存不存得进库（`storable.Text`），理由点名选择器、不回显坏值；头注写明端点不拿 `URL.Query()` 喂它。
- 用例：`TestARawQueryWithASegmentThatCannotBeDecodedIsRefused`（q、筛选值、键的转义坏掉，分号分隔，坏段混在合规段里；`ParseQuery` 与 `DecodeRaw` 各过一遍，理由说「解不开的一段」、不含 `%` 与坏字节）、`TestDecodeRawAndParseQueryAgreeWithDecode`、`TestTextTheDatabaseCannotStoreIsRefusedInSelectorValues`。

### 各目录端点

三种改法，逐口见 `da7fa5f48` 的 diff：

- **只读本册声明里的键的口**：`Decode(request.URL.Query())` 改成 `DecodeRaw(request.URL.RawQuery)`。parcel-pricing 那份 `decodePageQuery` 删掉，五口直接调 `DecodeRaw`。
- **自己读册名、选择器或筛选键的口**：Intake 之后 `ParseQuery` 一次，坏段答 400 带理由；册名、选择器、筛选键与 `Decode` 都读这一份。settlement 的对账单、费用、经营结果三口把这一份交给各册的 serve；customs 记录四口的外壳把它交给 `read`，外部结果的 `version` 从这里读；node-operations 送站两口的 `catalogueFilterOf` 也读它；渠道择优列表的 `tiedChannelSelectionFilterOf` 改收这一份。
- **分派先于 Intake 的三口**（委托查阅、复核队列、渠道择优）：分派照旧用 `URL.Query().Has(...)`，旁边一句指向 `ParseQuery` 头注；列表那一支在 Intake 之后严格解。

两处次序随之动了：

- settlement 的已审应付原先在 Intake 之前读筛选值（只组结构、不答 400）；现在解析能答 400，挪到 Intake 之后。
- visibility 的运营追踪三支（按包裹、按版本、列表）读同一份。改前 `?parcel=%zz` 被丢下后落进列表那一支、答出整册。

首方客户整单追踪本来就照 `url.ParseQuery` 拒坏段，改调 `cataloguepage.ParseQuery`，行为不变。

### 用例

- 各上下文端点的拒参表各加一问坏转义的 `q`：visibility、transport 两表、governance、node-operations 两表、customs、collection、access-identity、settlement、party-commercial 两表、network 的目录与路线计划、parcel-shipment 的面单交易表与各口共用表。表里同跑未配置渠道的，这一问答 403、与合规一问逐字相同。pricing 的表本来就有这一问。
- customs 与 network 路线计划的表用 `url.Values` 拼请求，编码不出解不开的段，各加一个照原样拼的帮手。
- 反证：只把端点代码换回去、用例不动，上面各表的坏转义一问都失败（改前答 200 带整册），换回来再过。

### 完成判据对照

- 共用件单元用例证坏转义与分号答 400、理由不回显坏值：见共用件一节。
- 各端点表驱动用例补一问 `?q=%zz` 答 400：见用例一节。
- 全仓再没有目录端点直接用 `URL.Query()` 喂给声明解码：`grep -rnE '\.Decode\(request\.URL\.Query\(\)\)'` 在 `internal`、`cmd` 无匹配。`URL.Query()` 余下的用处是分派先于 Intake 的三口，与几个不解目录声明的单件读（词表、收寄链、交接范围汇总、对账单行、账单行、法人画像解析的 `at`、参考数据启用的 `kind`、渠道择优单件、委托查阅单件 Intake 的 `shipmentRequestId`）。它们的坏段被丢下，至多落成「缺必备维」答 400，或被当成没给的多余键，答不出整册；本票不动。

### 验证

改到的各包与 `cmd/parcel-api` 的 `go test`、`go vet` 过，`gofmt` 无输出。全量真库与种子在进 main 那一轮跑。

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
