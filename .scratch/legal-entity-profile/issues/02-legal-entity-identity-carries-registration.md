# 02 责任法人身份登记加注册国家 / 地区与终身注册号

Category: enhancement
Status: resolved——2026-09-24 通道 3 在分支 `mcp3-lep02` 上做完（基 `mcp4-lep01` tip `6db15aa5`；代码 tip `d1d80542`，首轮代码 tip `9e0f3553`，含清点 tip `4193e0e7`），分支已推 origin。非作者评审（通道 5）须修一条，已在同一分支修完，待 Spec 轴复核后由推送方重放，只取本票的笔：`15dad3e5` 至 `4193e0e7`、首轮票面 `91949459`、评审处置 `d1d80542` 与本笔票面；`1d845397` 是 main 上认领笔 `eb0e86f6` 的拣入，重放时为空。迁移编号占 party-commercial `0034`（`0035` 留给 catalogue-read-pagination/02）。完成记录与评审处置见文末
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
4. **身份更正依据两向都拦**：改了不带即拒；没改带了也拒，首笔登记与历史修订第一次补登同样不许带——一条不改任何东西的更正依据会让读册的人去找一处不存在的更正。
5. **一类只收一个终身注册号**，号按类型代码规整次序：同一组号只有一种写法，重放判定不随录入次序变。
6. **快照三格 omitempty 排末尾**：历史形状快照逐字节不变、内容摘要不变；库三列全可空，CHECK 钉两格同空同有、国家形状、号数组非空、更正依据不脱离身份层、也不落在修订 1（评审处置补）。
7. **答复用显式布尔 `identityLayerRegistered`**，判据同停用两件：页面据它说「本修订登记时尚无此格」，不拿空数组去推。
8. **处理器构造函数加依赖**：不登法人的调用方给 nil；给了 nil 又登法人答技术失败，不替它放行。
9. **演示种子**：本格落地之前灌过的库上 `SYN-LE-01` r1 是旧形状，重跑答内容冲突，用 `--reset`（README 已记）。
10. **修订 1 不带更正依据由领域判，重放与读回也拦**：拦在 `WithIdentityLayer`，不在接续门——首笔登记没有最新修订可比。拒因沿用 `ErrIdentityCorrectionWithoutChange`：首笔登记是第一次登上两格，与历史修订补登同理，不是更正一个录错的号，续办也同一句（去掉更正依据）。这条不看册面、不随目录与历史形状变，从这一格存在起就成立，所以不落在「三道门只拦新登记与新修订」那条之下：修订 1 在册后带着依据重放答`未受理`而不是`内容冲突`——册上不可能有与它同内容的修订。库加 CHECK `legal_entity_registration_correction_not_first_revision` 同钉；「只随改了身份层的修订出现」要和前一笔比，库判不了，仍归用例。
11. **停用修订不补登身份层**：历史法人（`0034` 之前登记）停用时，那一笔经 `LegalEntityRegistration.Deactivate` 原样沿用最新修订，仍没有身份层，字面上偏离做什么里「自本票起的新登记与新修订必须带两格」。停用命令不收身份层；要补两格走带两格的登记修订，停用只是生命周期的终点，不兼作补登入口。
12. **身份层五格暂不抽行类型**（评审 Standards 2）：`ports.GroupLegalEntityRow` 与 `ports.LegalEntityRevisionRow` 各抄一份五格、`identityLayerBodyOf` 收散参，是真重复。本轮不动：纯重构跨 ports / postgres / http 与测试字面量，而本轮复核只重跑 Spec 轴，不宜夹带没经 Standards 复核的重排。
13. **一类两号的拒因暂按唯一可达的一种说**（评审 Standards 3）：`legalEntityIdentityLayerFrom` 把 `domain.NewLegalEntityIdentityLayer` 的错一律说成「一类终身注册号只收一个」，今天可达的也只有这一种——缺国家、缺号在调它之前已各自拦下。构造器以后加校验时，要给一类两号立哨兵再分流；本轮同上理由不动。

**未做 / 风险**

- 同一租户里两个法人登了同一个终身注册号，本票不拦：跨法人查重 ADR-0145 没裁，要做先定「同号即拒」还是「同号提示」，另议。
- 地盘越出派单的两处（开工前已报通道 2）：共享的 `internal/partycommercial/ports/ports.go`（`GroupLegalEntityRow` 尾部加格）；`cmd/parcel-api` 装配与测试、`cmd/parcel-commercial`、机制清点。
- 管理台表单与资料页归票 04；法人资料归票 03。
- 评审 Standards 2、3 本轮未改，取舍见判断项「身份层五格暂不抽行类型」「一类两号的拒因暂按唯一可达的一种说」。

## 评审处置（2026-09-24，通道 3，分支 `mcp3-lep02`，代码 tip `d1d80542`）

非作者评审 ← 通道 5（钉 `9e0f3553`）结论须修，阻断仅 Spec 1；评审原文由推送方重放时代落 Comments。

**阻断 Spec 1：首笔登记带身份更正依据照常落册** → 已修，笔 `d1d80542`。

- 领域：`WithIdentityLayer` 拒修订 1 上的更正依据，答 `ErrIdentityCorrectionWithoutChange`；登记册读回同经此处。
- 应用：`WithIdentityLayer` 的拒绝经 `identityCorrectionRefusal`（原名 `identitySuccessionRefusal`）译成续办，续办句点名首笔登记。
- 迁移 `0034`：加 CHECK `legal_entity_registration_correction_not_first_revision`（`identity_correction_basis IS NULL OR revision > 1`），头注释去掉计数。`0034` 未进 main，就地改；它的校验和随之变，施加过旧版的常驻库会报校验和漂移，重建即可——本票没有留下这样的库。
- 用例：领域 `TestFirstLegalEntityRevisionCarriesNoCorrection`；应用层 `TestLegalEntityIdentityCorrectionNeedsABasis` 加首登带依据即拒且不写、修订 1 在册后带依据重放也拒；真库 `TestLegalEntityIdentityLayerAgainstTheRealCatalogue` 加首登带依据即拒且不写、库拒修订 1 上的更正依据。
- 反证：撤掉领域那道检查，应用层首登答 `REGISTERED`（即评审复现的结果），领域与应用层新用例红；撤掉 `0034` 那条 CHECK，真库用例在改库那一步红。
- 取舍见判断项「修订 1 不带更正依据由领域判，重放与读回也拦」；判断项「身份更正依据两向都拦」补上首笔登记。

**非阻断**

- Standards 1（Covers 注释按条目序号引票面）→ 已改，同一笔：改引条目原句「新登记缺一拒登，号经 01 的校验」「不作变更，录错走更正」「既有修订不改写」。
- Standards 2 → 未改，见判断项「身份层五格暂不抽行类型」。
- Standards 3 → 未改，见判断项「一类两号的拒因暂按唯一可达的一种说」。
- Spec 非阻断 1 → 补判断项「停用修订不补登身份层」。

**门**（钉 `d1d80542`，WSL，go1.26.8，DSN 为门禁库 55432）：`go build ./...`、`go vet ./...` 退 0；改动的 `.go` `gofmt -l` 无输出；迁移 `0034` 无 CR、无 BOM；先单跑真库用例 `TestLegalEntityIdentityLayerAgainstTheRealCatalogue` 是 `PASS` 不是 `SKIP`。改动包（PC 领域、应用、postgres 适配器与 `migrations`）与 `go list` 反查的反向依赖、`internal/architecture` 共 37 包 `-p 1 -count=1 -v`：35 包 ok、2 包无测试文件、0 FAIL；`--- PASS` 3787、`--- FAIL` 0、`--- SKIP` 1（`pgtest` 的 `TestHelperTemplateOwnerProcess`，只由另一用例以子进程驱动，与 DSN 无关）。迁移一改，反向依赖就扩到各上下文的真库包与全部依赖迁移的 `cmd/*`。全量由推送方跑。

自上次已验 SHA `4193e0e7` 以来动过的 `.go` / `.sql`：`internal/partycommercial/domain/` 下 `party_identity.go`、`legal_entity_identity_layer.go`、`legal_entity_identity_layer_test.go`；`internal/partycommercial/application/` 下 `register_party_identity.go`、`register_legal_entity_identity_test.go`；`internal/partycommercial/adapters/postgres/legal_entity_identity_layer_test.go`；`migrations/party_commercial/0034_legal_entity_identity_layer.sql`。
