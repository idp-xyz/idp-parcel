# 02 接入平台编号能力与单据编号规则册

Category: enhancement
Status: in-progress——2026-11-02 作者完工、自验、推分支 `claude-docnum02`（完成记录见文末）；待非作者评审与进 main
父票：[spec](../spec.md)
Blocked by: [01](./01-adr-glossary-context-and-use-case-handoffs.md)；平台库可被正式依赖——用户合并 idp-xyz/idp-platform-go#1 并打 `v0.1.0-rc.1`、合并 idp-xyz/idp-bento-go#5 并在 vanity 主机部署（用户动作），`curl -s "https://go.idp.xyz/idp-platform-go?go-get=1"` 返回平台库的 go-import 即算放行
出处：spec 子票表；[ADR-0355](../../../docs/adr/0355-a-document-number-is-separate-from-internal-identity-and-is-assigned-in-the-issuing-transaction-under-built-in-rules-a-tenant-may-override.md) 决定四、五、六、七

## 做什么

1. **依赖**：`go.mod` 锁定 `go.idp.xyz/idp-platform-go` 的精确版本（`v0.1.0-rc.1`），`go.sum` 两行写入；取证照框架模块解析合同（`GOPRIVATE=go.idp.xyz`，不关全局校验），在空缓存里复跑一次下载与编译并记进完成记录。
2. **迁移**：平台序列技术表作为独立的平台 schema 接入 `internal/platform/migrate` 的合并有序历史——渲染 `numberingpg.Migrations(<平台 schema>)` 的资产、按 ID 记入、像框架版本一样记录平台库精确版本；资产只增不改，不并入任何业务 schema，应用启动不自动改库。在已有数据的库（`demo_live` 的一份副本）上施加验证。
3. **取号编排**（决定四、七、八）：一个共享的取号组件——读该租户该类型此刻生效的模板（租户版或产品默认）→ 定时区（调用方给的开立方业务时区，没有就取租户的单据编号时区，都没有答「单据编号时区未配置」，调用方据此不带号开立）→ 用服务端此刻在该时区下算周期 → 在调用方事务内经平台 `SequenceAllocator` 取（租户，类型代码，周期）的下一个值 → 渲染 → 经调用方给的「是否已用」查询判撞，已用就再取下一个值，跳过上限（产品策略，定值写理由）仍撞答「编号规则撞号」。不用 `Assigner.Assign`；加扰键的拼法照平台入口（越权风险点 1）。事务外调用即失败。
4. **演示种子**：为演示租户登一个单据编号时区（`Asia/Shanghai`，演示租户取值，记 `S`），不登模板。
5. **类型声明**（决定六）：十一类单据的类型代码、默认模板、日期时区来源由各所属上下文各声明一处，装配时汇总；本票把十一条声明都写上（04–09 照用，不另写）。
6. **规则册**：本仓自有的一张册（自己的 schema，不进平台 schema），按（租户，类型代码）存规则版本：模板、生效时刻、登记人与登记时刻；另存租户的单据编号时区版本（IANA 时区名与生效时刻，租户取值、无默认）；都只增不改；同内容重放答已存在。登记校验：模板能被平台 `Parse`、含流水令牌、用了加扰而该租户没配密钥即拒（密钥从实例配置注入，形态本票定，不进仓库、无默认）。
7. **口**：读口（每类单据此刻生效的规则、是租户版还是产品默认、所用时区或「单据编号时区未配置」、历次版本）、登记口（规则版本与单据编号时区各一口，登记册配置写授予）、预览口（给定类型与模板按样例流水渲染，不读也不占序列，并提示与已用号段可能重叠）。端点进端点表，挂操作者 Intake，答复形状照既有登记口。
8. **测试**：单据编号时区未登时答未配置、登了后按它出日期；撞号跳过与跳过上限；PG 集成用例覆盖 ADR-0143 原列的证据——同一开立意图重放（由调用方在 04–09 验，本票验取号组件本身的事务参与）、并发取号不撞、租户隔离、按日 / 月 / 年 / 不重置的周期、事务回滚释放、租户换模板后同周期流水接着数；规则册的登记、重放、拒登与读口；预览不占号。

## 不做

不接任何具体单据的开立（04–09）；不做管理台（03）。

## 完成判据

上面各条都有用例；`go build ./...`、`go vet`、`gofmt -l`（判有无输出）、受影响包加 `./internal/architecture/... ./internal/platform/... ./cmd/...` 真库测试全绿；种子灌一次性库退 0。

## 工作方式

只做 `docs/agents/workflow.md`「进 main 的步骤」第 1 步：作者在自己的 worktree 与分支上做完、自验、推分支、写完成记录；非作者评审与进 main 由协调方做。注释中文、无行号与跨文件计数；不手改 `docs/adr/README.md`、AT 盘点总览计数、机制清点。

## 完成记录（2026-11-02，作者，分支 `claude-docnum02`，从 origin/main `02c694238` 起）

三笔代码提交：`4fc95e496`（依赖与迁移）、`acbcc879f`（编号组件、四口、类型声明）、`96beda7b6`（受控登记口与种子），加本记录一笔。

### 做了什么

1. **依赖**：`go.mod` 直接依赖 `go.idp.xyz/idp-platform-go v0.1.0-rc.1`，`go.sum` 两行 `h1:o+oytSOn+EC5/3iiiSWOdL+CzXTwUeol1Br0ERLH1VQ=` 与 `/go.mod h1:GNtiWs3x+t02hdq6QfuTCKebZwGrlcNiWK5QyEhnTks=`，无 `replace` 与 `go.work`。`internal/platform/migrate` 的 `TestPlatformVersionIsTheOneGoModPins` 比对 go.mod 锁定版本、go.sum 两串与迁移计划登记的 `migrate.PlatformVersion`。
2. **迁移**：`migrate.SchemaPlatform`（`platform`）上渲染 `numberingpg.Migrations`，步骤 ID `platform/<资产 ID>`、来源 `PLATFORM`、记 `PlatformVersion`，排在框架之后、业务之前。平台库只交 ID 与 SQL、不交校验和清单，校验和对渲染后的 SQL 算（schema 是常量，同一版本逐字不变；平台库若改写已发布资产，已部署的库以 `ErrChecksumDrift` 拦下）。历史表以追加形状 `historyPlatformDDL` 加 `platform_version` 一列与两条约束，新库旧库走同一条路。规则册迁移 `migrations/document_numbering/0001_document_numbering_rule_register.sql` 照本仓迁移写法并入（schema `document_numbering`，不进平台 schema、不进任何业务 schema）。应用启动不改库，照旧。
3. **取号编排** `docnumber.Assigner.Assign`（`internal/platform/docnumber/assign.go`）：先判事务（事务外答 `ErrTransactionRequired`，单据编号时区未配置那一格也不例外）→ 读此刻生效的模板（租户版或产品默认）→ 定时区（调用方给的开立方业务时区；没有取租户此刻的单据编号时区；都没有答 `NUMBERING_TIME_ZONE_NOT_CONFIGURED`，序列不动）→ 服务端此刻在该时区下算周期 → 经平台 `SequenceAllocator` 取（租户，类型代码，周期）→ `Render` → 调用方的 `IsUsed` 判撞，已用取下一个值，跳过 `MaxSkippedNumbers` 个仍撞答 `NUMBERING_RULE_COLLISION`。不用 `Assigner.Assign`；加扰键照平台入口拼成 `密钥|租户|类型代码`，用例 `TestScrambledNumbersMatchThePlatformAssigner` 对同一输入与平台 `Assigner.Assign` 逐字相同。
4. **演示种子**：`scripts/demo-seeds/steps/050-document-numbering-time-zone.sh` 经新受控登记口为 `SYN-TENANT-01` 登单据编号时区 `Asia/Shanghai`（2026-01-01 起，演示租户取值，记 `S`），不登模板；排在开立单据的各节之前。`seed.sh` 编译清单加新 CLI，README 组成表加行。
5. **类型声明**：六个上下文各在 `domain/document_number_types.go` 声明本上下文的类型（变量 `XxxDocumentNumberType` 加 `DocumentNumberTypes()`），04–09 直接引用；`cmd/internal/docnumbertypes.Catalogue()` 汇总，parcel-api 与受控登记口共用这一份汇总。`TestCatalogueHoldsTheElevenDocumentTypesOfTheDecision` 对照 ADR-0355 决定二、五。
6. **规则册**（`register.go`、`docnumberpg/store.go`）：规则版本按（租户，类型代码，生效时刻）为键存模板、登记人、登记时刻；单据编号时区版本按（租户，生效时刻）存 IANA 名；只增不改，同内容重放答 `VERSION_EXISTING`，同一生效时刻异内容答 `EFFECTIVE_FROM_TAKEN`。登记校验：类型须已声明、模板须过平台 `Parse`、含流水令牌、用了加扰而该租户没配密钥即拒；时区只收 IANA 名（空串、`Local` 拒）。
7. **口**（`docnumberhttp`，端点表 `cmd/parcel-api/endpoints_document_numbering.go`，自建操作者 Intake，同基础数据目录）：`GET /document-numbering/rules`（查阅读）、`POST /document-numbering/previews`（查阅读，同试算）、`POST /document-numbering/rule-registrations` 与 `POST /document-numbering/time-zone-registrations`（登记册配置写）。答复形状照节点作业登记口：成功 `{"outcome",...}`，失败 `{"error":{"code"}}`；契约写在 `docnumberhttp` 包注释。登记人取信封，写成 `OPERATOR/<发行方>#<sub>`；受控登记口记 `CONTROLLED-CLI/parcel-document-numbering-register`。
8. **测试**：见下「验证」。

### 关键取舍与理由

- **跳号上限 20**（`MaxSkippedNumbers`，产品策略）：撞号只撞换格式前的旧号；零星撞几个由跳过吸收，成片地撞说明新模板与旧号段整段重叠，是规则册该改的格式问题，跳得再多也只是把一段流水烧成空号（越权风险点 7）。二十次「取值 + 索引查」在几十毫秒内，不把开立事务与序列行锁拖长。
- **加扰密钥形态**：实例配置项 `IDP_PARCEL_DOCUMENT_NUMBER_SCRAMBLE_SECRETS_FILE` 指一个文件，文件是 `{"<租户>":"<密钥>"}` 的 JSON 对象；每把至少 32 个字符，空租户键、重复键、尾随内容拒。不设即一个租户都没配（不是默认密钥）；设了而读不动或不合规，parcel-api 在开池前带原因退出；启动日志只报配没配与租户数。取文件不取环境变量本身：按租户成表、便于挂机密文件、不随进程环境进诊断输出。演示租户未配。
- **生效时刻可早于登记时刻**：号在开立时取、开立后不变，早于登记的那一段改不到已开立的单据，册上同记登记时刻；拒它会让种子与受控登记口无法用固定生效时刻重放。
- **模板检查多拒两类令牌**：`{site}{owner}{region}{le}`（规则按租户与类型登，开立不带这些值）与 `{check}`（规则版本不带校验位算法），二者渲染出来都是空串；另拒渲染出空白、控制字符或长于 40 字符的号（模板长于 64 字符）。用了哪些令牌只经平台公开的 `Render` 观察（改一项输入看输出变不变），不在本仓另写令牌文法。实测中撞到平台以 NUL 作 `{check}` 占位，哨兵改用 `\x01`。
- **预览重叠提示按位比形状**：逐个流水渲染去碰，十万量级的撞号（按日改按月）一次预览要几十兆内存；改为每份模板按流水位数分段、每段五个探针取逐位上下界，等长且逐位区间有交集即「可能重叠」。重置周期相同的两份模板共用序列，同一位数段渲染一致时跳过——只改流水最少位数这种最常见的改法不误报。加扰与随机段只有探针那几个取值，宁可少报。读口样例与预览的加扰都用预览键，不用租户真密钥。
- **时区来源声明**：只有 `CUSTOMER_STATEMENT`、`DELIVERY_PRE_ADVICE`、`DELIVERY_HANDOVER_NOTE` 声明「开立方有业务时区」（ADR-0355 决定七点名的结算账户与网点）；其余声明「开立方没有业务时区」，按租户单据编号时区。声明「没有」的类型交来业务时区是编程错误（`ErrIssuerTimeZoneNotDeclared`），不悄悄择一。
- **取号每次重跑模板检查**：平台库升级改了令牌渲染时，开立带原因失败，不出一个少了一段的号。
- **装配**：parcel-api 启动时把取号编排连同规则册一起装起来（`buildDocumentNumbering`），今天只为 fail-fast，04–09 接它。

### 验证（2026-11-02，本机 PG `127.0.0.1:55432`）

- `go build ./...`、`go vet ./...` 退 0；`gofmt -l .` 无输出；`go mod tidy -diff` 无输出。
- `go test -count=1 -p 1` 覆盖 `./internal/architecture/... ./internal/platform/... ./cmd/... ./migrations/... ./tests/...` 与六个上下文的 `domain` 包：全绿（含 PG）。`-v` 抽查本票新增与改动的 21 条用例（多数走真库）全 PASS、0 条 SKIP：`docnumberpg` 八条（事务参与与回滚释放、并发、租户隔离、日 / 月 / 年 / 不重置、换模板接着数、规则册登记重放与读、预览不占号、写口无事务即拒）、受控登记口两条、`migrate` 新旧共七条（含首版历史表接上平台来源）、parcel-api 四条。`bash scripts/ci/generated-artifacts-pr.sh` 退 0（机制清点过期，留推送方重生成）。
- **空缓存复跑**：全新 `HOME`、`GOMODCACHE`、`GOCACHE`，`GOPRIVATE=go.idp.xyz`、不关全局校验：探针模块 `go get go.idp.xyz/idp-platform-go@v0.1.0-rc.1` 成功，go.sum 两串同上；本分支在同样的空缓存里 `go build ./...` 退 0（经 `https://go.idp.xyz/idp-platform-go?go-get=1` 200 → `git ls-remote https://github.com/idp-xyz/idp-platform-go.git`），`go mod verify` 答 all modules verified。另查公共路径：`proxy.golang.org` 对该版本 `.info` 答 200，`sum.golang.org` 记的两串与 go.sum 一致——CI 不设 `GOPRIVATE` 也取得到。
- **种子**：一次性库 `docnum02_seed` 上 `scripts/demo-seeds/seed.sh` 退 0，050 答 `VERSION_REGISTERED`；库里 `platform/platform_numbering_0001_numbering_sequence` 记 `PLATFORM`、`v0.1.0-rc.1`。用后删库。
- **迁移施加在 `demo_live` 副本**：`pg_dump demo_live` 还原到 `docnum02_try`（历史 362 行，首版形状、无平台库版本列，已有 4 组重复 `sequence`），`scripts/demo-seeds/migrate`（不带 `-reset`）退 0：施加 2 步（平台序列表、规则册），历史 364 行，平台步骤记 `v0.1.0-rc.1`，既有业务行不动；再跑 `applied=0`。用后删库。`demo_live` 本身只被 `pg_dump` 读过一次。

### 留给协调方

- **04–09 的接法**：在 cmd/parcel-api 经 `buildDocumentNumbering(db, sources.scrambleSecrets).assigner` 取编排，接到各上下文的消费侧端口；`Assign` 必须在开立事务的 ctx 里调，且与开立同一个 `*bentopg.DB`（框架按 DB 认事务归属）；`IsUsed` 查本上下文的单据表；类型代码用本上下文 `domain` 里的声明变量。
- **时区来源待实现票核定**：`STATEMENT_DISPUTE`、`AUDITED_PAYABLE` 是否取结算账户业务时区（05），`SHIPMENT` 是否有可用的发件网点业务时区（04）；改法是改本上下文那一处声明，`cmd/internal/docnumbertypes` 的对照用例随之改。
- **待裁**：参数登记册是否为「单据编号时区」「加扰密钥」两项租户取值补行（本票未动 `docs/product/PILOT-PARAMETER-REGISTER.md`）；ADR-0355 越权风险点 1 所说「向平台库提议能传模板与时刻的取号入口」本票未提。
- 新增 CLI、端口与适配器会让机制清点过期，由推送方在 main 的 tip 上重生成；本票未立 AT 行。
