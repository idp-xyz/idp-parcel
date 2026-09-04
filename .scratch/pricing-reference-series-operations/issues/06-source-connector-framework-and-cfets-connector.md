# 来源连接器框架 + 首个连接器（CFETS 人民币中间价）；免复核声明格

Category: enhancement
Status: in-progress——MCP-5（2026-09-04，接管重派 task-31a5aa4a；基线 `eba019a8`，隔离分支 `mcp5-pricing06`）。两问已裁（2026-09-04，通道 6，owner 授权；见「裁决」）：范围收窄为**连接器契约 + `FileConnector` + 免复核声明格 + `cmd/parcel-pricing-feed`**；CFETS 连接器那一段留 draft，随部署侧登记出网能力后另开工
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
