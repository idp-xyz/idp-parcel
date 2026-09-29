# 17 委托声明的服务产品参与商业依据解析：同一范围多个产品不再必然`适用冲突`

Category: enhancement
Status: resolved——2026-09-29 通道 2；完成记录见文末。进 main 的 SHA 等重放后补。此前 in-progress——2026-09-29 通道 2 认领，从 `mcp2-psb17@0ca53342` 接着做；隔离 worktree `idp-parcel-mcp2-psb17`、分支 `mcp2-psb17b`（基 `a23d1338`）。此前 ready-for-agent——2026-09-25 通道 2 崩后认领释放（通道 4 按用户令独立接手时处置）：半成品在分支 `mcp2-psb17`（已推 origin），`026be1ae`、`51cac469`、`485a6d33` 三笔是通道 2 做完的之一至之三，`0ca53342` 原样封存它未提交的一份测试（mtime 停在 2026-09-24 22:53:38，没跑过）；接手时写明「从 `mcp2-psb17@0ca53342` 接着做」还是「另起」。此前 in-progress——2026-09-24 通道 2 认领（用户令独立承接），隔离 worktree `idp-parcel-mcp2-psb17`、分支 `mcp2-psb17`。此前 ready-for-agent——同日通道 2 立票并按用户令自决裁定（用户原话「按你的建议，你自己全部开工做，独立完成」），裁决见下
Blocked by: 无
地盘：`internal/partycommercial`（闭包解析键、逐项解析键、闭包落库与迁移）、`internal/parcelshipment` 领域读口与 `adapters/partycommercial`；party-commercial `CONTEXT.md`、UC-PC-002、新 ADR。
出处：[票 05](./05-demo-journey-criterion-evidence.md) 格 1（实测 + 探针）与判断项 1——演示动线按种子原样灌时的第一个停点。本票承接[票 06](./06-ps-acceptance-and-label-selection-judgment-methods.md) 第 8 项（通道 4 于 `ca26a1ec` 补入）：立票时漏看了那一项，票 06 该项已改指本票，裁决只记在这里。

## 现象

委托提交后受理链停在可达性段开头：闭包 `APPLICABILITY_CONFLICT`，`ConflictingBases()` 为 `[SERVICE_PRODUCT]`。发布批里 `SYN-PROD-CN-SG-EXPRESS` 与 `SYN-PROD-CN-SG-ECON` 同在 `SYN-SCOPE-01`；解析键登记面按（租户，客户账户）一行、范围写死，委托草案里的 `service.requestedProduct` 只进摘要、不进键（`CommercialResolutionKeys.FormResolutionKey`、`CommercialBasisQuery`）。闭包对每项依据都按范围独立解析（`ClosureResolutionKey.singleBasisKey`），接单规则包对服务产品的指名引用要等全部解完才由 `namedReferencesConfirmed` 核对，收窄不了候选。

## 裁决

1. **取「委托声明的服务产品身份收窄服务产品候选」，不取「一产品一商业范围」。** 商业范围在 party-commercial 是服务、费用或控制范围（`CONTEXT.md` 解析一节），同一份合同、接单规则包、结算政策覆盖一个范围里的多个产品是常规形态；一产品一范围会逼租户把这些对象按产品重复发布。UC-PC-002「同一客户的不同非重叠范围」说的是范围之间，不是同一范围里的产品之间。
2. **收窄的是对象身份，不是版本。** PC 照旧按锚点在该产品的已发布版本里唯一解析，ADR-0080「消费方不指定选中哪个商业版本」不被碰；`CommercialBasisQuery` 头注「绝不指定应当选中哪个商业版本」照样成立，头注补半句说明产品身份不是版本。
3. **纪律与结算、信用选择器同形。** 这一维只在必需依据含服务产品时可在场，其余必缺；在场即按身份收窄，不在场照旧按范围解析——同一范围多个产品仍答`适用冲突`，那是如实的答案，不补默认产品。
4. **正文指名了服务产品的成员随之收窄，没指名的不受影响。** 今天指名服务产品的是接单规则包：同一范围里两个产品各配一份规则包是常规形态，只收窄服务产品而不收窄规则包，规则包那一项仍会答`适用冲突`。所以候选里凡正文指名了另一个服务产品的一律落选，指名了委托声明的那个或根本没指名服务产品的照旧参选；收窄之后仍由 `namedReferencesConfirmed` 事后核对。声明的产品没有配规则包时，规则包那一项答`无适用依据`——那是如实的答案。（实现前修订：初稿写「其余成员不另收窄，指名不同即`适用冲突`」，一则漏了两份规则包那一格，二则现行代码对指名不符答的是`解析未决`（`NamedReferenceNotConfirmed`），不是冲突。）
5. **读法归 PS 领域。** 从提交版本内容里读出委托声明的服务产品，照 `AddressElementsOf` 的形给一个封闭读口，不在适配器里按字符串拼条目名。
6. **这一维进闭包身份与闭包落库。** 第二阶段与提交前重解按标识回读闭包，回读的键必须带着它，否则重解按另一套键解。

## 做什么

1. ADR（Proposed，按用户授权接受）记裁决 1–4；party-commercial `CONTEXT.md` 解析一节与 UC-PC-002「解析键至少包含」一句按它改，验收补一条：同一范围两个产品按委托声明各自唯一解出；未声明仍答`适用冲突`；声明的产品在该范围无已发布版本答`无适用依据`。
2. PC 领域：`ClosureResolutionKey` 与逐项 `ResolutionKey` 加请求服务产品一维（最小身份、指纹、逐项键派生），服务产品候选按身份收窄。
3. PC 持久化：闭包落库带上这一维，回读重建时复验。落在既有闭包快照 JSON 上，不另开迁移：这一维是闭包键的一部分，键已经在那份快照里；不声明时不多出这一键，内容摘要不变。（更正：初稿写「新迁移」。新列迁不了「没声明的旧快照读回仍是缺席」，`omitempty` 已经做到，且仓里没有对应 `.sql`。）
4. PS：领域读口；`CommercialBasisQuery` 携带；`CommercialResolutionKeys.FormResolutionKey` 放进键。

## 不做

- 同一客户多个非重叠范围按委托的目的服务范围折范围：同族缺口（登记面一客户一行、一个范围），演示动线不经过，不在本票。
- 不改演示种子：两个产品同在一个范围正是本票要撑住的形态。

## 完成判据

- PC 领域用例：声明产品按身份收窄后唯一解出；未声明多候选答`适用冲突`；声明的产品无已发布版本答`无适用依据`；正文指名另一个服务产品的成员落选，指名声明的那个或没指名的照旧参选，声明的产品没有配规则包时规则包那一项答`无适用依据`；闭包键在不请求服务产品依据时携带这一维，最小身份不成立。（更正：原写「规则包指名另一产品答`适用冲突`」「非服务产品依据携带这一维即最小身份不成立」，与修订后的裁决 4 不一致。指名另一产品的是落选，不是整份闭包冲突；请求了服务产品的闭包把这一维带给每一项成员，非服务产品依据携带它并不因此使最小身份不成立。）
- PS 用例：读口取出声明的产品；适配器把它放进键；未声明时键上缺席。
- 真库（含 DSN）：闭包落库与回读带这一维。
- 演示动线（只记 `S`）：种子原样灌、委托声明 `SYN-PROD-CN-SG-EXPRESS` 时受理链越过格 1，结果写回票 05 格 1。

## 完成记录（2026-09-29，通道 2）

从 `mcp2-psb17@0ca53342` 接着做。旧分支指针不动。新分支 `mcp2-psb17b`，基 `a23d1338`（取证时 `origin/main` 同此）。进 main 的 SHA 等重放后补；下面是分支上的 SHA。

重放四笔（出处在 `mcp2-psb17`）：
- `ba84fc4e` 裁决 4 实现前修订（出处 `bf7a9a30`）
- `d79ac993` 解析键带委托声明的服务产品（出处 `026be1ae`）
- `e0791932` 闭包快照带上这一维（出处 `51cac469`）
- `0ebb2682` 封闭条目名读口与提交版本（出处 `485a6d33`）

续做：
- `7b1bd8f1` 认领
- `3b756f67` 登记面折出的键上叠声明的产品。`0ca53342` 封存的 `requested_service_product_keys_test.go` 已采用为集成候选（该封存笔写明非集成候选且当时编译红，没有整笔 cherry-pick）
- `04f70633` ADR-0153、CONTEXT 解析一节、UC-PC-002、判据按修订后的裁决 4 收口
- 本记录所在的提交：票面转 resolved，票 05 格 1 写回 17 重走

验收：
- 领域用例在 `internal/partycommercial/domain` 与 `internal/parcelshipment/domain`；键来源半边在 `requested_service_product_keys_test.go`。
- 真库：`TestResolutionRoundTripsTheDeclaredServiceProduct` 在本机 55432 门禁上 `-v` 为 PASS，不是 SKIP。
- 自验：`go build ./...` 与 `go vet ./...` 退出 0。`go test -count=1 -p 1` 跑了动过的包及其反向依赖，外加 `./internal/architecture/...`，带 `IDP_PARCEL_POSTGRES_DSN`，全部 ok，没有 FAIL。
- 演示动线只记 `S`：见票 05 格 1「17 重走」。代码钉 `04f70633`。

没有非作者评审。评审由通道 1 另派。
