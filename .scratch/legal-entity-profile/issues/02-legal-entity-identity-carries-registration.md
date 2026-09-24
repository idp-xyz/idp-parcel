# 02 责任法人身份登记加注册国家 / 地区与终身注册号

Category: enhancement
Status: resolved——2026-09-24 通道 3 在分支 `mcp3-lep02` 上做完（基 `mcp4-lep01` tip `6db15aa5`；代码 tip `9e0f3553`，含清点 tip `4193e0e7`），分支已推 origin。进 main 由推送方安排非作者评审后重放，只取本票的笔：`15dad3e5` 至 `4193e0e7` 与本笔票面；`1d845397` 是 main 上认领笔 `eb0e86f6` 的拣入，重放时为空。迁移编号占 party-commercial `0034`（`0035` 留给 catalogue-read-pagination/02）。完成记录见文末
Blocked by: 01
地盘：party-commercial 责任法人身份登记的领域、应用、postgres 与 http 适配器（含 `query_party_identities.go` 的 `groupLegalEntityBody`），`migrations/` 下
party-commercial 模块的新迁移，演示种子。
出处：[ADR-0145](../../../docs/adr/0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md) 决定一、二；CONTEXT Rules 同句。

## 做什么

1. 身份登记加注册国家 / 地区与终身注册号（可多个，每个带类型）；新登记缺一拒登，号经 01 的校验（身份层）。
2. **不作变更，录错走更正**（决定二）：修订若改了这两格，必须是携带更正依据的内容更正修订；领域上没有「改号」这一种修订。
3. 既有修订不改写：迁移只加格，历史行的两格为空；自本票起的新登记与新修订必须带两格。演示种子补合成值。
4. 读口答复加两格；登记失败的理由散文照既有 `writeProblemWithDetail` 的用法交出。

## 不做

- 不建法人资料（归 03）；不动管理台（归 04）。

## 完成判据

- 真库用例：缺国家、缺号、号不属该国身份层类型、格式不符各拒一条；更正修订改号须带依据，不带即拒；历史修订读回时两格为空且不报错。
- 端点用例：答复带两格。

## 完成记录（2026-09-24，通道 3，分支 `mcp3-lep02`）

**落点**

| 笔 | 段 | 做了什么 |
|---|---|---|
| `15dad3e5` | 领域 | `LifetimeRegistrationNumber`、`LegalEntityIdentityLayer`（国家与至少一个号、一类一个号、按类型代码规整次序）；`LegalEntityRegistration` 加身份层与身份更正依据、`WithIdentityLayer`；停用沿用身份层、不沿用更正依据；`CheckLegalEntityIdentitySuccession` |
| `5eb16753` | 应用 | 命令加三格；处理器加 `ports.RegistrationNumberTypeLookup` 依赖；新登记与新修订的三道门（必带两格 → 按目录判号 → 接续门），重放照册面比对 |
| `4bc10b03` | 持久化 | 迁移 `0034`（三列可空 + 四条 CHECK）；登记册快照三格 omitempty 排末尾、三列与快照同源；目录上列与修订历史读三列；ports 两个行类型加五格 |
| `5bd38b7e` | http | 隔离写 Intake 收三格（形状错包 MALFORMED_REQUEST）；目录与修订历史答复共用 `identityLayerBody`（显式布尔 `identityLayerRegistered`） |
| `9e0f3553` | 装配与种子 | 受控 CLI `register-parties` 与在线口同形收三格、接真目录；`parcel-api` 编排接目录；`SYN-LE-01` 补 `CN` 与合成号，README 记旧库重跑用 `--reset` |
| `4193e0e7` | 清点 | 机制清点在本分支重生成 |

**完成判据**

- ✅ 真库：`TestLegalEntityIdentityLayerAgainstTheRealCatalogue`——缺国家、缺号、号属资料层、格式不符各拒一条且不写；改号不带更正依据即拒、带了落册；目录上列、修订历史、最新修订三处读回身份层与更正依据。
- ✅ 真库：`TestHistoricalLegalEntityRevisionReadsBackWithoutIdentityLayer`——历史形状修订三列落 NULL、快照无新键（内容摘要不变）、三处读回答没有身份层且不报错、原样重放答 `ALREADY_REGISTERED`、库拒只有国家没有号的行。
- ✅ 端点：`TestLegalEntityBodiesCarryTheIdentityLayer`——目录与修订历史各一行带身份层与更正依据、一行历史形状，逐键核在场与否；`TestIsolatedIntakeTranslatesTheLegalEntityIdentityLayer`——三格译入、三键都缺照旧、四种形状错各拒。
- ✅ 应用层（内存替身）：各道门的续办理由、更正依据两向、历史修订重放与补登、未装配目录答技术失败。
- ✅ 演示种子：一次性库上 `seed.sh` 端到端零报错，`SYN-LE-01` r1 `REGISTERED`、三列落值，库已删。

**门**（钉 `4193e0e7`，WSL，go1.26.8，DSN 为门禁库 55432）：`go build ./...`、`go vet ./...` 退 0；改动的 `.go` 按入库字节 `gofmt -l` 无输出；改动包与 `go list` 反查的反向依赖、`internal/architecture`、`migrations` 共 16 包 `-p 1 -count=1` 全 ok，其中 `internal/partycommercial/...` 与两个 cmd 包 1607 个用例 PASS、0 FAIL；迁移 `0034` 无 CR、无 BOM。全量由推送方跑。

**判断项**

1. **按法人生效时点判号**：身份在那一刻生效，号的类型那一刻就得在用；不给处理器加时钟。代价：目录条目的生效时点要按该类号实际启用的时间登记，登得比历史法人晚，补登历史法人时会答「不在用」。
2. **不随登记固定类型修订号**：固定之后，目录修订一次，同一笔登记的重放就从「已登记」变成「内容冲突」；按类型代码 + 登记时点可在目录修订史里查回当时那一版。
3. **三道门只拦新登记与新修订**：修订号落在已有修订上的是重放或冲突，照册面比对——历史修订没有身份层、目录也可能已修订，拿今天的门拦重放会把「已登记」答错。
4. **身份更正依据两向都拦**：改了不带即拒；没改带了也拒，历史修订第一次补登同样不许带——一条不改任何东西的更正依据会让读册的人去找一处不存在的更正。
5. **一类只收一个终身注册号**，号按类型代码规整次序：同一组号只有一种写法，重放判定不随录入次序变。
6. **快照三格 omitempty 排末尾**：历史形状快照逐字节不变、内容摘要不变；库三列全可空，CHECK 钉两格同空同有、国家形状、号数组非空、更正依据不脱离身份层。
7. **答复用显式布尔 `identityLayerRegistered`**，判据同停用两件：页面据它说「本修订登记时尚无此格」，不拿空数组去推。
8. **处理器构造函数加依赖**：不登法人的调用方给 nil；给了 nil 又登法人答技术失败，不替它放行。
9. **演示种子**：本格落地之前灌过的库上 `SYN-LE-01` r1 是旧形状，重跑答内容冲突，用 `--reset`（README 已记）。

**未做 / 风险**

- 同一租户里两个法人登了同一个终身注册号，本票不拦：跨法人查重 ADR-0145 没裁，要做先定「同号即拒」还是「同号提示」，另议。
- 地盘越出派单的两处（开工前已报通道 2）：共享的 `internal/partycommercial/ports/ports.go`（`GroupLegalEntityRow` 尾部加格）；`cmd/parcel-api` 装配与测试、`cmd/parcel-commercial`、机制清点。
- 管理台表单与资料页归票 04；法人资料归票 03。
