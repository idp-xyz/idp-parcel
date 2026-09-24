# 01 注册号类型目录：按注册国家 / 地区登记注册号类型、格式与所属层

Category: enhancement
Status: resolved——2026-09-24 通道 4 交付（分支 `mcp4-lep01`，代码 tip `6f246025`、含清点的 tip `f0216e34`，基 `47c80a8e`；派单 `task-c785cb7e` ← 通道 3），待非作者评审与推送方重放；分支已推 `origin/mcp4-lep01`（本宿主 gh 设备码登录后 github.com 可达，推分支不起 CI run）
Blocked by: 无
地盘：party-commercial 的领域、应用、postgres 与 http 适配器里新增的一本登记册，`migrations/` 下 party-commercial 模块的新迁移，`scripts/demo-seeds` 的合成条目。
出处：[ADR-0145](../../../docs/adr/0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md) 决定一；CONTEXT Rules「责任法人身份登记必须带注册国家 / 地区……」一句。

## 做什么

1. 一本按租户的登记册，每个类型带：注册国家 / 地区、类型代码、名称、格式校验、所属层（身份层的终身注册号 / 资料层的税务登记号）。按修订版本化、不可覆盖，
   登记与停用都带依据——与本上下文其余登记册同一纪律。
2. 登记写面进端点表（未配置即拒，沿既有登记写面的通例）；目录读口沿 ADR-0077。
3. 给 02、03 用的领域校验：给定国家 / 地区与层，某个号是否属目录里的某一类型且格式合格；目录里没有该国家 / 地区时答「未登记」，不以默认格式代替。
4. 演示种子只在合成租户下登记演示用条目。

## 不做

- 不改责任法人身份登记（归 02）。不带任何国家的生产默认条目。

## 完成判据

- 真库用例：登记、修订、停用；层不符、格式不符、国家未登记三种拒绝各一条。
- 迁移按字节无 CR、无 BOM；`go test ./internal/architecture/ -count=1` 过。

## 完成记录（2026-09-24，分支 `mcp4-lep01`，基 `47c80a8e`）

作者：通道 4（派单 `task-c785cb7e` ← 通道 3）。落点八笔，按次序：

- `7276ada4` 领域段：`domain.RegistrationNumberTypeRegistration` 与生命周期、`RegistrationNumberFormat`（RE2、整串匹配）、
  `RegistrationNumberLayer` 与 `RegistrationNumberLayerNamed`、`RegistrationNumberTypeCatalogue.Check` 与答案代数。
- `12064b12` 持久化段：迁移 `party_commercial/0033_registration_number_type_catalogue.sql`（只建结构不种行）、
  `ports/registration_number_type.go` 三口、`pcpostgres.RegistrationNumberTypes`、`OperationsCatalogue.ListRegistrationNumberTypes`。
- `be92501c` 改名（领域状态加 `Status`、端口落点加 `Registry`，行为零变化）。
- `7419f9aa` 应用段：`application.RegisterRegistrationNumberTypeHandler`（登记与停用）。
- `ac96efe3` http 段：登记与停用两口、目录读口、`UnconfiguredIntake` 补两方法。
- `152d705d` 装配段：`cmd/parcel-api` 端点表三行、事务壳、未接线桩、两份端点清单测试；共享接线文件纯增、无改动行。
- `6f246025` 受控登记口与演示种子：`parcel-commercial register-registration-number-types`、种子 JSON、`seed.sh` 一行、README 一行。
- `f0216e34` 在分支干净检出上重生成机制清点（生成器依赖经 goproxy.cn 取得、按工具 `go.sum` 校验）。

对完成判据：

- ✅ 真库用例「登记、修订、停用」：`TestRegistrationNumberTypeRegistersRevisesAndDeactivates`（修订 1 落册、重放、同修订异内容冲突、
  修订 2 更正、停用成修订 3 往返、目录上列只见最新修订且导出 DEACTIVATED）；进程口另有端到端
  `TestRegisterRegistrationNumberTypesBatchLandsRepliesAndRefuses`。
- ✅ 三种拒绝各一条：`TestRegistrationNumberTypeLookupRefusesLayerFormatAndUnregisteredCountry`——目录经登记册按国家 / 地区取回再判号，
  资料层号填进身份层答 `LAYER_MISMATCH`、按修订 1 合格而修订 2 不合格的号答 `FORMAT_MISMATCH`（钉住只取最新修订）、册上没有的
  国家 / 地区答 `COUNTRY_NOT_REGISTERED`。
- ✅ 迁移按字节 0 个 CR、首三字节 `2d 2d 20`（无 BOM）；`migrations` 包行尾守卫 ok。
- ✅ `go test ./internal/architecture/ -count=1` ok（两道棘轮不变长：校验相关类型经 `ports.RegistrationNumberTypeLookup` 的签名
  由生产代码可达，没有新增非 `New*` 的未接线领域函数）。

对「做什么」逐条：① ✅ 按租户、按修订、登记与停用都带依据；② ✅ 两个写口挂字面量 `UnconfiguredIntake{}`，目录读口沿 ADR-0077；
③ ✅ 校验入口见下；④ ✅ 种子只在 `SYN-TENANT-01`。「不做」两条都守住：责任法人身份登记一行未动；迁移不种行，产品不带任何国家 / 地区的条目。

门禁（钉 `f0216e34`，worktree 无未提交改动、HEAD 即该提交）：`gofmt -l .` 0 个文件；`go build ./...` / `go vet ./...` 0；
带 DSN `go test -count=1 -p 1` 跑改动包及其反向依赖共 17 包：16 ok、1 无测试文件（`ports`），2367 条用例 pass、0 fail、0 skip。
种子另在一次性库 `idp_mcp4_seedcheck` 上端到端验过：迁移计划 176 步、6 项落定、重放全答 `ALREADY_REGISTERED`，库已删，未触共享演示库。
全量由推送方跑。

给票 02 / 03 的校验入口：

```go
// ports.RegistrationNumberTypeLookup；读失败走 error，不折成「未登记」
catalogue, err := lookup.LoadRegistrationNumberTypeCatalogue(ctx, tenant, country)
// 身份层用 RegistrationNumberIdentityLayer，法人资料的税务登记号用 RegistrationNumberProfileLayer
check, err := catalogue.Check(code, domain.RegistrationNumberIdentityLayer, number, at)
// check.Outcome()：ACCEPTED / COUNTRY_NOT_REGISTERED / TYPE_NOT_REGISTERED / TYPE_NOT_EFFECTIVE / LAYER_MISMATCH / FORMAT_MISMATCH
numberType, consulted := check.Type() // 所对照的类型修订（代码 + 修订号），可随登记一并固定
```

生产实现 `pcpostgres.NewRegistrationNumberTypes(db)` 同时实现 `RegistrationNumberTypeRegistry` 与 `RegistrationNumberTypeLookup`。

判断项（票面与 ADR 没有定、由本票取的，留给评审与 PC owner）：

1. 注册国家 / 地区只收两位大写拉丁字母（ISO 3166-1 alpha-2 那一种形状），不内置码表；库内同款 CHECK。理由：一个国家 / 地区在
   目录里只能有一个键，cn、CN、CHN 并存会把登过的国家答成未登记。
2. 格式取 RE2 正则、按整串匹配（包 `^(?:…)$`；包之前先单独编译，挡住 `a)|(b` 这类不配平正文拆开锚点）。校验位算法不在其内——
   那是格式之外的另一层语义，要进目录得另立列与规则，归 PC owner。
3. 所属层钉在类型上：新修订改层不受理，改层即登记新类型代码。ADR-0145 越权风险点 1 说的「在目录上把那类号标为资料层」，本实现
   按「新登一个资料层类型」承接，不允许旧类型翻层——翻层会让已按它登记的终身注册号说不清还算不算。
4. 目录条目带生效时点，生命周期同身份册三格；`Check` 因此要时点，未到生效与已停用都答 `TYPE_NOT_EFFECTIVE`，细分可由
   `Type().Lifecycle().StatusAt(at)` 读出。
5. `Check` 判断次序：国家 / 地区 → 类型 → 层 → 生效期 → 格式；层排在生效期之前，交错了层要改号，不论类型此刻在不在用。
6. 演示种子用真实国家 / 地区码 CN、SG（形状门只收两位字母，SYN 前缀放不进去），类型代码与格式全为 `SYN-`，合格号必以 `SYN-`
   开头；测试夹具一律用 ISO 3166 用户自定义码 XA、XB。
7. 目录读口沿 ADR-0077 的 `limit` 形，未采 ADR-0144：其决定七「一处定义，逐册迁移」，共用件在 `catalogue-read-pagination/01`，本册
   等随那批迁移。
8. 地盘补一格 `cmd/parcel-commercial`（新子命令文件 + `main.go` 分派一处、用法串两处）：种子只能经登记 CLI 入库（ADR-0077
   Consequences）；开工时已经 `report_task` 报备通道 3。
9. 管理台未接这本目录的页面（票面未要，`liveIds` 不变）；三个新端点目前没有前端消费方。

## Comments

### 评审 ← 通道 3 · 钉 `6f246025`（基 `47c80a8e`，只读，门禁未重跑）· 2026-09-24 15:2x（推送方自通道 3 来信代落原文）

引通道 4 在 `f0216e34` 实跑：gofmt 0、build / vet 0、带 DSN -p 1 改动包 + 反向依赖 17 包 2367 pass、architecture ok、种子在一次性库端到端过。评审人是 ADR-0145 起草人，非代码作者。

**Standards** — 阻断：无。非阻断：
1. `internal/partycommercial/adapters/http/unconfigured_intake.go` 的接口断言 var 块被 gofmt 整块重对齐（新名 `RegistrationNumberTypeRegistrationIntake` 更长），语义纯增，但既有十几行都成了改动行——任何同期往这块里加东西的分支重放时会冲突，请排重放次序时留意（lep02 大概率也会动它）。无发现：注释中文、引 CONTEXT 原句与 ADR 决定号不引行号；共享接线 cmd/parcel-api 纯增。

**Spec** — 阻断：无。无发现（实核）：ADR-0145 决定一——`RegistrationNumberTypeCatalogue.Check` 目录无该国家即答 `COUNTRY_NOT_REGISTERED`、不以默认格式代替；层钉在类型上（`RegistrationNumberLayer` 封闭集，迁移 `layer IN ('IDENTITY','PROFILE')`），错层答 `LAYER_MISMATCH`；判定次序国家→类型→层→生效期→格式，各格续办不同故分格交回；`Check.Type()` 交回所对照的类型修订供随登记固定。格式 RE2 整串匹配、先单独编译正文挡住不配平写法，防回溯爆炸。迁移 0033 按租户（主键租户 + 国家 + 类型 + 修订）、只建结构不种行；种子五条类型代码与格式全是 `SYN-` 形，不带真实 UEN / 统一社会信用代码格式——「产品不带生产默认」成立。票 01 判据：登记写面挂 `UnconfiguredIntake{}`（未配置即拒）、目录读口随商业目录隔离读放行沿 ADR-0077（未采 ADR-0144 与决定七「未迁的册维持今天」一致）。一个国家的类型全被停用时答 `TYPE_NOT_EFFECTIVE` 而非「国家未登记」，合 ADR 原意（国家登过、类型不在用）。

非阻断：
2. **只校格式、不算校验位**（作者判断项已列）：UEN 与统一社会信用代码都带校验位，形状对而校验位错的号会被放过。首版接受；若要校验位，它是按类型可配的算法，另立票，不在本分支补。

结论：**可接受**（Standards 0 / 1 · Spec 0 / 1）。

**处置**（推送方 · 通道 1）：评审无阻断，按 parallel-sessions「别人分支上的活怎么进 main」重放，与 catalogue-read-pagination/03 同批、同一次全量。分支 `mcp4-lep01` 的十笔在 `ef9b8225` 上
cherry-pick 为 `d53c9fe3`（← `7276ada4`）/ `f93568fc`（← `12064b12`）/ `71163777`（← `be92501c`）/ `d2cc4007`（← `7419f9aa`）/ `b09574c4`（← `ac96efe3`）/
`dcc79a76`（← `152d705d`）/ `af5aec60`（← `6f246025`）/ `f2c903b6`（← `f0216e34`）/ `dd80b6b6`（← `983ce80e`）/ `6b9a76f3`（← `6db15aa5`）。代码与票面二十六个文件
与分支 tip 逐字一致（`git diff origin/mcp4-lep01 <重放 tip> -- <本票改过的文件，清点与 spec 除外>` 为空）；两处冲突都不在代码上：清点笔 `f0216e34` 与 main 上
catalogue-read-pagination/01 的清点笔同改合计行，照原笔「推送方重放到 tip 后照例重生成」在重放 tip 上重生成；`983ce80e` 改 spec 子票表 01 行，与 main 上
lep02 认领笔 `eb0e86f6` 改的 02 行相邻，取 01 行本票的、02 行 main 的。非阻断 1 已照办（lep02 叠在本分支上，不另排次序）；非阻断 2 首版接受、校验位另立票，
归作者 / 用户定。
