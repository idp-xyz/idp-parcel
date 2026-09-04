# 来源连接器框架 + 首个连接器（CFETS 人民币中间价）；免复核声明格

Category: enhancement
Status: resolved——MCP-5（2026-09-04，接管重派 task-31a5aa4a → task-03538334；隔离分支 `mcp5-pricing06`，代码 tip `2c275c9f`、清点笔 `110d60ce`，基线 main `dfd1725b`；main 上的 SHA 由 MCP-1 重放后在 Comments 补）。范围按「裁决」收窄为**连接器契约 + `FileConnector` + 免复核声明格 + `cmd/parcel-pricing-feed`**，四项已落；**CFETS 连接器那一段留 draft**，开工前置是部署侧登记出网能力（见完成记录末条）
Blocked by: 无（`03` 已 resolved `f62d619`）

## 要建什么

按 ADR-0099 决定六：

1. **连接器契约**（`internal/parcelpricing/adapters/sourcefeed/`）：`SourceConnector.Fetch(ctx, spec) (PublishedRecord, error)` → `PublishedRecord`（原文字节、内容 SHA-256、抓取时刻、地址、来源声明的公布日期）；`Transcribe(record, prior ReferenceSeriesRegistration | none) (ReferenceSeriesRegistrationSpec, error)` 产出**整版重述**的登记 spec：前一版全部期次 + 新期次，前一版开放末期在新期次起点闭合；凭证 = 工件引用（`sha256:… @ 抓取时刻 ← 地址`）。走同一登记用例。
2. **首个连接器：CFETS 人民币中间价**（官方、免费、稳定）。种类 `EXCHANGE_RATE`；口径引用来自租户对该来源的绑定配置（哪个商业价格政策版本声明「CFETS 中间价」这一牌价类型），**不在连接器里写死**。
3. **来源配置（实例半边的格）**：租户级「来源连接器绑定」——连接器种类、序列标识、口径引用、抓取节律、`免人工复核: 是/否/未声明`。未声明 = 需人工复核。免复核时由系统写复核记录，复核责任方为连接器身份、依据为该配置的版本。
4. **抓取失败的行为**：不补数、不沿用旧值、不登空版本；留缺口 + 告警（告警通道属实例半边，机制只出一条可观察记录，同 ADR-0095 两层）。
5. **进程口**：`cmd/parcel-pricing-feed`（受控批量口，与 `parcel-pricing-register` 同款：不监听端口，手跑或调度）。

## 先答再开工

- **出网**：parcel-api / worker 进程今天有没有出网能力与代理策略？本机 GitHub 都要走代理（workflow.md）；生产出网属部署实例半边。若无，本票只做契约 + 以本地文件为源的 `FileConnector`（供测试与受控导入），CFETS 连接器留 `draft`。
- **工件存放**：抓取原文放哪？ADR-0008 的 MinIO 证据存储是候选；ADR-0092 「本体存放是一条未配置的出向缝」是同形先例——摘要必备、定位符可缺。

## 裁决（2026-09-04，通道 6，task-f530ad56 裁决批口径：owner 授权自决，写明能力边界）

**一、出网：今天没有，本票不造。** 取证于 `main = 512b419`：全仓非测试 `.go` 搜 `http.Client` / `http.Get` / `http.Post` /
`http.NewRequest` / `http.DefaultClient` / `http.DefaultTransport` / `ProxyFromEnvironment` **零命中**——`parcel-api`、
`parcel-dispatch` 与全部 `cmd/parcel-*-register` 家族没有任何一处发出过 HTTP 请求，也就没有代理策略可言；仓内唯一提到
代理的是 `docs/agents/workflow.md` 记本机装 `gh` 要走 `127.0.0.1:7897`，那是开发机的事。生产出网能力（有没有、走哪个
出口、代理与白名单）属部署实例半边，与 `PAR-INT-02` 同列，眼下无处登记。

因此本票按「无」那一支走：**只做「做什么」第 1、3、4、5 项——契约 `SourceConnector` / `PublishedRecord` / `Transcribe`、
以本地文件为源的 `FileConnector`（供测试与受控导入：运营把来源公布页另存的文件放进受控目录，`Fetch` 读文件、算 SHA-256、
记读取时刻与文件定位符）、来源连接器绑定的免复核声明格、受控批量口 `cmd/parcel-pricing-feed`。** 第 2 项 CFETS 连接器
**留 draft**：它的全部机制（契约、转录、凭证、失败行为）都由 `FileConnector` 先验证；等部署侧把出网能力登记上再开工，
届时改的是加一个 `SourceConnector` 实现与一行装配，不是契约。这条与 ADR-0090 / ADR-0092 对「等第一家真渠道」的处置同形：
不等，先把机制半边做实。

**二、工件存放：照 ADR-0092 的形，本体存放是一条未配置的出向缝；本票不引对象存储依赖。** 具体：

- `PublishedRecord` 的**内容摘要（SHA-256）必备、且必须在字节还在手上那一刻算**（ADR-0092 Consequences 那一句逐字适用——
  漏算就永久失去「我们收到过什么」的证据）；抓取时刻与地址同为必备。三者合起来就是 ADR-0099 决定六说的取值凭证，
  期次因此天生 `VERIFIABLE`——**凭证等级不依赖本体在不在**。
- 原文本体走一条出向端口「工件存放」，形照 ADR-0092 决定二：**定位符可缺**，未配置即如实作答，凭证引用里定位符一格留空，
  摘要照旧在。`FileConnector` 场景下本体本来就躺在受控目录里，定位符即文件路径，端口可以先接一个「原地引用」实现。
- **不采 ADR-0008 MinIO 作为本票的硬依赖**，两条理由各自成立：其一，ADR-0092 Alternatives 已否决「现在就引入对象存储依赖」
  ——组件不在本仓也不在框架，现在引是替一个尚不存在的技术组件选型，那条理由一字未变；其二，ADR-0008 划的是**试点证据**
  的存储边界，并明写「不得被扩展为 `idp-parcel` 业务数据库或跨上下文业务数据共享通道」，而抓取原文是计价参考序列的
  **取值凭证本体**，属 `parcel-pricing` 的业务证据，它日后落哪个对象存储是部署侧接端口的事，不是本票替它选。等对象存储
  存在时，接上的是这一个端口的实现，迁移与契约不动。

**能力边界**：读了 ADR-0099 决定六、ADR-0092 全文、ADR-0008 全文，与全仓非测试 Go 代码的 HTTP 客户端符号面；**未读**
`internal/parcelpricing/adapters/` 下现有登记路径的代码（`sourcefeed` 目录于 `512b419` 不存在，本裁决不预设它的内部形状），
也未核 CFETS 公布页的实际协议与格式——那属 CFETS 段开工时的事。本裁决只定范围与两条取舍，不定 `SourceConnector` 的方法签名。

## 红线

- 连接器不算数、不改数、不选口径。
- 不写任何真实来源的启用态；出厂零连接器绑定。
- **本票不写任何出网代码**：`FileConnector` 只读本地受控目录；CFETS 段开工前，仓内不得出现指向外部地址的抓取实现。
- 工件摘要必备、定位符可缺；不把原文本体写进业务库（ADR-0092 Alternatives 第一条的两条理由对本票逐字成立）。

## 完成记录（2026-09-04，通道 5，task-03538334；前身 task-a1425a1f / task-31a5aa4a 两次会话重置后接管）

分支 `mcp5-pricing06`，基线 main `dfd1725b`（中途两次 rebase：`eba019a8` → `7d7b8b5e` → `ae7b4c8a`/`dfd1725b`，五笔代码零冲突）。
分支上的 SHA 作封存出处；main 上的 SHA 等 MCP-1 重放后补。

| 分支 SHA | 范围 |
|---|---|
| `e3af0f3c` | 票面认领（上一 MCP-5 会话所落，Status 转 in-progress） |
| `64156ffe` | 领域与端口：`PublishedRecord`（构造器自算 SHA-256）、`SourceConnectorBinding` + `ReviewExemption` 三格、`SeriesObservation` / `TranscribeSourceFeed` 整版重述；`ports.SourceConnector` / `SourceConnectorResolver` / `SourceArtifactStore` + `ArtifactPlacement` / `SourceFeedObserver`；`adapters/sourcefeed`：`FileConnector`、`InPlaceArtifactStore`、`ConnectorRegistry` |
| `b6fa19c3` | 绑定登记册 `adapters/postgres/source_connector_binding.go`（迁移 `parcel_pricing/0005_source_connector_binding.sql`）；序列最近登记版本读口 `ReferenceSeriesLatestVersionLoader` / `LoadLatestVersion` |
| `c6c1dc5d` | 应用编排 `FeedReferenceSeriesHandler`（抓取 → 存放 → 转录 → 同一登记用例 → 免复核时代写复核）与 `RegisterSourceConnectorBindingHandler` |
| `68c1e9af` | `cmd/parcel-pricing-feed`：`-kind feed` / `-kind source-connector-binding` 两种运行，不监听端口，退出码 0/1/2/3/4；真库往返用例 `TestFeedRoundTripsThroughPostgres` |
| `55109d4c` | 出网红线做进结构：`TestNoOutboundNetworkCodeShipsWithTheFileConnector` 拦 `sourcefeed` 与 `cmd/parcel-pricing-feed` 非测试文件的 `net` / `net/http` 导入（探针一正一反实测） |
| `2c275c9f` | 随 ADR-0108 对齐：转录器读前版指纹、构造改 `NewVersionReferenceWithFingerprint`；绑定的口径引用改三元 + 可选指纹（迁移 0005 列 `quote_basis_fingerprint`、可缺；kind 不落列，恒为商业价格政策，构造门守） |
| `110d60ce` | 机制清点在 `2c275c9f` 干净检出上重生成 |

**契约签名字面**（`internal/parcelpricing/ports/source_connector.go`）：

```go
type SourceConnector interface {
	Kind() string
	Fetch(ctx context.Context, spec FetchSpec) (domain.PublishedRecord, error)
	Transcribe(input TranscriptionInput) (domain.ReferenceSeriesRegistrationSpec, error)
}
type SourceConnectorResolver interface {
	ConnectorFor(kind string) (SourceConnector, bool)
}
type SourceArtifactStore interface {
	Store(ctx context.Context, record domain.PublishedRecord) (ArtifactPlacement, error)
}
type SourceFeedObserver func(observation SourceFeedObservation)
```

`FetchSpec{Tenant, SeriesID, Locator}`；`TranscriptionInput{Record, Placement, Binding, Prior *ReferenceSeriesRegistration}`；
`PublishedRecord` 四件必备（原文字节、抓取时刻、来源定位符、来源声明的公布日期），摘要由构造器对字节自算、不收外给；
凭证写法 `PublishedRecord.EvidenceReference(storageLocator)` = `sha256:… @ 抓取时刻 ← 定位符`（本体另有存放处时追加 ` → 存放定位符`，原地引用不追加）。

**对完成判据逐项**：

- 契约 `Fetch` / `PublishedRecord` / `Transcribe(record, prior)`：如上；整版重述在 `domain.TranscribeSourceFeed` 一处（前一版全部期次 + 新期次，开放末期在新起点闭合；有界末期不动——改它就是改数），连接器逐家实现时只读观测不重写；spec 走同一登记用例 `RegisterReferenceSeriesHandler`。
- 版本号 = 来源声明的公布日期，引用指纹 = 工件摘要；同一份工件再来一次转录出与前一版逐字相同的 spec，登记册据以答幂等重放（`REPLAYED`），不再代写复核。
- `FileConnector`：只读受控目录（上溯、绝对路径、卷名一律 `ErrLocatorOutsideRoot`）；字节在手那一刻算摘要；定位符 `file:` + 目录内相对路径（根属部署，不进凭证）；文件缺失 `ErrSourceUnavailable`、解不开 `ErrPublicationUnreadable`、文件自报序列与绑定不符 `ErrPublicationSeriesMismatch`。
- 免复核声明格：`ReviewExemption` 封闭三格 `EXEMPT / NOT_EXEMPT / UNDECLARED`，无零值语义、无默认（列 `NOT NULL` 无 `DEFAULT`）；`EXEMPT` 时复核记录由连接器身份 `connector:<种类>` 写、依据 `免人工复核声明 ← 来源连接器绑定 <序列>@<绑定版本>`；`UNDECLARED` / `NOT_EXEMPT` 不写，版本不进在用。四眼门仍在复核用例——租户把登记责任方填成连接器身份会如实被拒。
- 抓取失败：不补数、不沿用旧值、不登空版本；`FeedFetchFailed` / `FeedTranscriptionRefused` 各出恰一条 `SourceFeedObservation`（站点 `FETCH` / `TRANSCRIBE`，原始错误原样带出），怎么出声归装配方——`cmd/parcel-pricing-feed` 写一行结构化日志到标准错误；告警通道属实例半边。
- 工件存放：`SourceArtifactStore` 出向端口，`ArtifactPlacement` 复用 `outbound.Outcome` 代数；`NotConfigured` 是诚实答案不是失败，登记照常、凭证不带定位符、仍 `VERIFIABLE`（`TestFeedUnconfiguredStorageStillRegistersVerifiable`）；`FileConnector` 场景接 `InPlaceArtifactStore`（定位符即来源地址）；未引任何对象存储依赖。
- `cmd/parcel-pricing-feed`：不监听端口；`-source-root` 不给即拒（连接器不猜受控目录）；绑定文档不带任何默认，`reviewExemption` 缺省即拒；出厂零绑定（迁移不种任何行，用例第一步即证「无绑定 → BINDING_UNKNOWN」）。
- 与裁决当时字面不同的一处：**`quoteBasis` 形状随 ADR-0108 改三元 + 可选指纹**——裁决与原单写的是「口径引用（含 digest）」，实施期间 pricing/10 落地把 `VersionReference` 改成 kind + id + version + 可选 `fingerprint`；绑定文档 `quoteBasis.fingerprint` 可省、迁移 0005 列 `quote_basis_fingerprint` 可缺（MCP-1 22:5x 同意）。kind 不落列：口径只能由商业价格政策版本声明，`NewSourceConnectorBinding` 拒别的 kind，读回以常量重建。

**真库往返**（`cmd/parcel-pricing-feed/roundtrip_test.go`，经 `assemble` 与 `execute`、真事务，夹具全部 SYN）：受控目录一份 SYN 文件 → 抓取（摘要 / 时刻 / `file:` 定位符）→ 转录 → 登记 `REGISTERED SYN-PRC-USD-CNY@2026-09-04 review=RECORDED`，`evidence_grade = VERIFIABLE`、复核行 `reviewer = connector:FILE`、`decision = APPROVED`、依据指回绑定 `b1`，在用解析到 `7.1234`；重跑同一文件 `REPLAYED`、复核与版本各仍一行；第二天文件延展成 `@2026-09-05`、前一版开放末期在新起点闭合；`UNDECLARED` 绑定登记但零复核行、在用答 `SeriesHasNoApprovedVersion`；文件缺失 / 解不开 → 退出码 4、零版本行、可观察记录各恰一条停在 `FETCH`；更早日期的文件 → `TRANSCRIPTION_REFUSED`、停在 `TRANSCRIBE`、版本行不增。

**验证**（干净 detached 检出 `110d60ce`，本机 Windows）：`gofmt -l .` 空；`go build ./...` / `go vet ./...` 退 0；无 DSN `go test -count=1 ./...` 98 包 ok / 0 FAIL（PG 用例跳过）；含 DSN `go test -count=1 -v ./internal/parcelpricing/... ./cmd/parcel-pricing-feed/... ./internal/architecture/... ./migrations/...` **549 PASS / 0 SKIP / 0 FAIL**；探针 `TestFeedRoundTripsThroughPostgres` 带 DSN `--- PASS` / 不带 `--- SKIP`；机制清点在同一检出上重生成与提交件零差。`-race` 走 WSL（`CGO_ENABLED=1 go test -race -count=1 ./internal/parcelpricing/... ./cmd/parcel-pricing-feed/...`）全 ok，但 WSL 够不到门禁容器，PG 用例在那一跑里是跳过的——竞态与真库两半分开覆盖，两样同时成立只有 CI 有（workflow.md 本机环境）。

**CFETS 段留 draft**：它的全部机制（契约、转录、凭证、失败行为、免复核格）已由 `FileConnector` 走通；开工前置是**部署侧登记出网能力与代理策略**（与 `PAR-INT-02` 同列，眼下无处登记）。届时要动的是：加一个 `ports.SourceConnector` 实现、`cmd/parcel-pricing-feed` 装配里 `NewConnectorRegistry(...)` 多传一个连接器、以及把 `TestNoOutboundNetworkCodeShipsWithTheFileConnector` 的禁导入名单按那时的裁决调整——那一动本身就是「前置已满足」的可见记录。契约与迁移不动。建议前置满足时另立票，不复用本票。

## Comments

- 2026-09-04 · 通道 6：两问裁决（见「裁决」节），Status 转 ready-for-agent。
- 2026-09-04 · 通道 5（上一会话）：认领，Status 转 in-progress；五笔落到 `0508f230`（基 `eba019a8`）后会话重置，未报完工。
- 2026-09-04 · 通道 5（本会话，task-03538334）：接管收口。rebase 两次；补出网门禁一笔、ADR-0108 对齐一笔、清点一笔；完成记录如上，Status 转 resolved。main 上的 SHA 待 MCP-1 重放后补记。
