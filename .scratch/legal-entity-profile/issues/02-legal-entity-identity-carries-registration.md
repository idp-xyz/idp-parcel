# 02 责任法人身份登记加注册国家 / 地区与终身注册号

Category: enhancement
Status: resolved · 已进 main——2026-09-24 评审 ← 通道 5 须修一条（派单 `task-c4ce2895`）→ 作者通道 3 在同一分支修为 `d1d80542` → 复核 ← 通道 5 只重跑 Spec 轴、可接受（派单 `task-303a9f6f`）；推送方（通道 1）重放进 main：代码笔 `2862bf7d` / `6aed430e` / `b52b008d` / `40269613` / `d165eb79` / `f4dff29b`，票面 `8ac87ebe` / `63edbfd5`，清点 `9f270c03`；分支 `mcp3-lep02`（代码 tip `d1d80542`、票面 tip `2385486a`）作封存出处，新旧 SHA 对照见 Comments「进 main 记录」。迁移编号占 party-commercial `0034`（`0035` 已随 catalogue-read-pagination/02 先进 main，加载器按名排序、允许空号）。完成记录、评审处置与评审原文见下文
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

## Comments

### 评审 ← 通道 5 · 钉 `9e0f3553`（基 `6db15aa5`，只读，门禁未重跑） · 2026-09-24 16:4x（派单 `task-c4ce2895`，改派自通道 4 超时撤回的 `task-f97975ee`；推送方自任务报告代落原文）

**Standards** — 阻断：无。非阻断：
1. `application/register_legal_entity_identity_test.go` 三个 Covers 注释写「票 legal-entity-profile/02 第 1 / 2 / 3 条」。按 AGENTS.md「改文档」：跨文件引用不用行号也不用计数，Go 注释同受约束；条目序号与行号同构，票面增删一条就无声指错。改引条目原句，如「不作变更，录错走更正」。全仓只有 2 个 .go 文件这样写，不是既有惯例。
2. Data Clumps（判断）：身份层五格在 `ports.GroupLegalEntityRow` 与 `ports.LegalEntityRevisionRow` 各抄一份，`identityLayerBodyOf` 收五个散参、两处逐格传。可在 `ports` 抽一个身份层行类型嵌入两行；`LegalEntityRevisionRow` 那句「判据同 GroupLegalEntityRow」就是这份重复的自述。
3. `legalEntityIdentityLayerFrom` 把 `domain.NewLegalEntityIdentityLayer` 的任何错误都说成「一类终身注册号只收一个」；今天可达的只有这一种，构造器以后加校验时理由会答错（判断）。

**Spec** — 阻断：
1. 通道 4 线索成立。`RegisterPartyIdentityHandler.RegisterLegalEntity` 只在 `successor && found` 时调 `domain.CheckLegalEntityIdentitySuccession`，首登带 `IdentityCorrectionBasis` 经 `WithIdentityLayer` 原样落册；迁移 0034 的 `legal_entity_registration_correction_needs_identity` 只要求有身份层，库也放行。内存替身实测 r1 答 `REGISTERED`、册上带依据；现有用例只测 r2。定阻断：ADR-0145 决定二与 CONTEXT Rules「录错按内容更正形成新的登记修订并携带更正依据」，r1 无可更正；完成记录判断项 4「没改带了也拒」对首登不成立；既有修订不改写，错依据会永久挂在修订历史读口上。修：首登带依据按 `ErrIdentityCorrectionWithoutChange` 拒，补用例（最好真库一条）。

非阻断：
1. 历史法人（0034 之前登记）的停用修订经 `LegalEntityRegistration.Deactivate` 落册时仍无身份层，字面偏离做什么 3「自本票起的新登记与新修订必须带两格」；取舍合理（停用命令不收身份层），但判断项没写，宜补一条。

结论：须修——阻断仅 Spec 1（首登带身份更正依据照常落册）；Standards 无阻断。

逐点：① ✓ 缺国家 / 缺号在 `legalEntityIdentityLayerFrom` 与 successor 门拒；`checkLifetimeNumbers` 以 `RegistrationNumberIdentityLayer` 调目录 `Check`、按法人生效时点判，六种结果各译续办理由，判定次序随 lep01 的 `Check`。② r2 起的接续门成立（改号不带依据拒、没改带依据拒、历史补登带依据拒），领域上没有「改号」修订；首登缺口即 Spec 阻断 1。③ ✓ 三列全可空，四条 CHECK 对三格全空的存量行恒真，不拖旧行。④ ✓ `identityLayerBody` 由 `groupLegalEntityBody` 与 `legalEntityRevisionBody` 共用，`identityLayerRegistered` 显式布尔不省略；Intake 形状错包 `ErrMalformedRequest`，走 `registration_transport.go` 既有 `writeProblemWithDetail`。⑤ ✓ `NewRegisterPartyIdentityHandler` 调用点齐：parcel-api 编排与 `register-parties` 接真目录，停用 CLI 给 nil 并注明「停用不判号」；行类型的 postgres 两个读口已补读三列。⑥ ✓ `SYN-LE-01` 补 `CN` + `SYN-CN-LIFETIME` / `SYN-CN-000001`，类型登在 `SYN-TENANT-01` 下、格式为合成。

复现方式：/tmp detached worktree（`9e0f3553`）加一个一次性内存用例，单跑 `internal/partycommercial/application`，输出 `outcome=REGISTERED`、`revision=1 correction="SYN-CORRECTION-01" hasCorrection=true`；跑完删文件并 `git worktree remove`（未加 --force）。共享树与作者树未动，未 commit / push，未写票面。

### 复核 ← 通道 5 · 钉 `d1d80542`（基 `9e0f3553`，票面取 `2385486a`，只读） · 2026-09-24 17:1x（派单 `task-303a9f6f`；推送方自任务报告代落原文）
**Spec** — 阻断：无。非阻断：无。
结论：可接受——上轮 Spec 阻断 1 已修住。
逐点：
① ✓ 领域 `party_identity.go` 的 `WithIdentityLayer` 拒修订 1 带依据（`ErrIdentityCorrectionWithoutChange`）；应用 `RegisterLegalEntity` 经 `identityCorrectionRefusal` 答未受理且不写；迁移 0034 加 `legal_entity_registration_correction_not_first_revision`。/tmp 反证：去掉领域那道检查，`TestFirstLegalEntityRevisionCarriesNoCorrection` 与 `TestLegalEntityIdentityCorrectionNeedsABasis` 红、应用层回到 REGISTERED；还原后 PASS，真库两例带 DSN PASS（非 SKIP）。
② ✓ 同一 handler 里缺格（`legalEntityIdentityLayerFrom`）与「依据只随身份层出现」本就不看 successor 答未受理；判断项 3 管的是依册面 / 目录的门，修订 1 这条只看修订号与字段、任何册面都不可能收过，判断项 10 成立，合 ADR-0145 决定二「录错按内容更正形成新的登记修订」。
③ ✓ `party_identity_registry.go` 的法人快照读回无身份层即提前返回，不经 `WithIdentityLayer`；停用修订无依据；更正修订修订号 ≥ 2。
④ ✓ NULL 安全（revision 自 0015 起 NOT NULL），存量行恒真，写法同 0034 其余 CHECK；就地改的校验和影响票面「评审处置」已写。
⑤ ✓ 无夹带；受理文案扩到三种情形对「没改却带」仍成立；CLI、隔离写口、种子同走此 handler，停用经 `Deactivate` 清依据，无半修入口。
⑥ ✓ 判断项 11–13 与「评审处置·非阻断」如实：Standards 1 已改，2、3 留并写取舍，Spec 非阻断 1 成判断项 11。
更正上轮一处：Standards 1 里「全仓只有 2 个 .go 文件这样写、不是既有惯例」是按带票名的窄模式数的；放宽到「第 N 条」，PC 旧测试注释里还有（如 `isolated_write_intake_test.go`）。AGENTS.md 那条规矩照样成立，只是「非既有惯例」一说偏强，结论不变。

### 进 main 记录（推送方 · 通道 1）

- **门**：首轮评审须修，阻断只有 Spec 1（首登带身份更正依据照常落册）；作者在同一分支修为 `d1d80542`，复核只重跑 Spec 轴，无阻断、无非阻断。首轮非阻断按上文「评审处置」一段处置，不挡合入。推送方先试起隔离子代理跑复核，认证失败未成，复核改派原评审人通道 5。
- **重放**：在共享树 main `d0eb2e18` 之上 cherry-pick 为 `2862bf7d`（← `15dad3e5`）/ `6aed430e`（← `5eb16753`）/ `b52b008d`（← `4bc10b03`）/ `40269613`（← `5bd38b7e`）/ `d165eb79`（← `9e0f3553`）/ `f4dff29b`（← `d1d80542`）/ `8ac87ebe`（← `91949459`）/ `63edbfd5`（← `2385486a`）。两笔票面在本目录 `spec.md` 子票表上与 main 冲突，按意图合：02 行取分支，01 与 03 行留 main。分支清点笔 `4193e0e7` 不重放，在批 tip 干净检出重生成为 `9f270c03`（partycommercial 生产文件 138 → 140、测试 147 → 151、postgres 37 → 38；迁移 177 → 178）；认领笔 `1d845397` 与 main 上 `eb0e86f6` 等价，跳过。
- **验证**：同一组代码笔先落在 `d5abc96b` 上成 `a569439b`，隔离树钉它：改动 `.go` gofmt 无输出，全仓 build / vet 退 0，迁移 `0034` 无 CR / BOM；先单跑真库用例 PASS 非 SKIP，带 DSN `go test -p 1 -count=1 ./...` 117 包 ok、0 FAIL。挪到 main `d0eb2e18` 之上后与 `a569439b` 只差 `.md`（其间 main 上通道 2 / 4 的票面与登记册笔，以及本笔）；中途一版落在 `17865872` 上的 tip `25afcdda` 另带 DSN 全量一次，117 包 ok、0 FAIL。
