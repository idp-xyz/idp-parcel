# 商业价格政策正文没有发布通道，口径因此无处可挂

Category: chore
Status: resolved——`29085fe`（2026-09-03，MCP-2 → MCP-5 → MCP-1 三会话接力），验证见 Comments 末条
Blocked by: 无

## 怎么被看见的

票 02 做完汇率口径的领域类型（`FxCaliber`，`828dbfa`）之后要给六项口径落正文表，动手前先找
「口径挂在哪张正文上」——它们按 CONTEXT 是**商业价格政策版本**声明的。顺着 `SavePricePolicy`
往上找调用方，结果是：**全仓零非测试调用点**（取证 `756b99c`，`grep '\.SavePricePolicy\('`
排除 `_test.go` 为空）。

发布用例 `PublishCommercialAuthorityHandler` 的 `CommercialDeclarations` 有结算政策
（`SettlementPolicyBody`，`commercial-closure-settlement-key/02` 补的）、信用政策与供应商协议
（票 03 补的）三条正文通道，**没有价格政策的**。受控 CLI 的批文文档同样没有 `pricePolicyBody`。

后果：租户今天连一份价格政策的**方向与定价方案**都登不进去——`commercial_price_policy`（0010）
只有测试往里写过。口径是它上面的第二层；先补第一层，第二层才有东西可挂。**这与
`commercial-closure-settlement-key/02` 当年给结算政策补通道是同一个形状**，那张票的 Comment
写的正是「库表、端口、适配器都在，权威册里却一份结算政策也放不进去」。

## CONTEXT 与 ADR 要求什么

- CONTEXT「商业价格政策版本」：规定价格方向、计价范围、基准时点、税务/结算约束和允许的可执行
  价卡绑定；声明计价所需的商业口径（汇率牌价类型、取值时点、加点、销售方向体积系数）。
- ADR-0034：价格政策采用经政策，`PricingPlanStandingLookup` 为入参不为查询。
- ADR-0057：`planDirection` 与 `conversion` 是**发布当时** parcel-pricing 的答复与当时声明的
  转换，随正文落行、装载重跑 `checkPlanBinding`；「`SavePricePolicy` 在签名上显式收两者，调用方
  必须交出发布当时的答复」。
- UC-PC-001 步骤 6：版本与声明同一事务落库。

## 两处要定形的地方

### 一、`planDirection` 从哪来

ADR-0057 说它是「发布当时 parcel-pricing 的答复」，但没说由谁在发布链上去问。三条路：

- (a) **批文声明**：受控 CLI 的批文带 `planDirection`，由写批文的人从 parcel-pricing 的价卡目录
  （`PriceCardCatalogueRow.Direction`）抄来。与批文里 `contentDigest`、`approval` 同一信任级
  ——受控登记口是库网内的治理动作，本就采信批文。
- (b) **发布用例向 parcel-pricing 现问**：新开 PC→PP 的消费侧适配器（ADR-0025）。PP 今天没有
  「按方案引用取方向」的读口（`LoadApplicable` 按方向 + 范围 + 时点），得先在 PP 开一格——
  跨地盘，另一票。
- (c) 不收，装载时重查——ADR-0057 Decision 三明禁。

**本票取 (a)**，理由：它把 ADR-0057「调用方交出发布当时的答复」这条落在今天唯一存在的调用方
（受控 CLI）身上，不发明跨上下文读口；(b) 留给在线发布口（PAR-INT-01 待提供的 Intake）——在线口
采信自报本就是 ADR-0085 拒绝的形状，那时必须走 (b)。批文里 `planDirection` 打错的后果是
`checkPlanBinding` 在发布面按错的方向判，**这一格在 (a) 下守不住**，与 `contentDigest` 抄错同级，
写进批文文档的注释。

### 二、口径正文放哪，与价格政策正文什么关系

口径是价格政策版本的声明，但**不参与选用**（`ResolveCommercialPricePolicy` 按方向 + 范围 + 区间
选）。按票 03 定下的判据——不参与选择的正文不进整册、不进 `ViewRevision`——口径走**族 B**：
独立正文表 `0022_price_policy_caliber.sql`（一行一版，FK 回版本册，类别 CHECK = 6）、具名
`SavePricePolicyCaliber`、点读口 `PricePolicyCaliberView`、目录行上扩字段带 `HasCaliber`。
不动 0010、不动 `CommercialPricePolicy` 结构体（ADR-0057 Decision 三的同一条理由：结构体表达
选用时要观察的正文，口径不是）。

领域上一个 `PricePolicyCaliber` 聚合三格：`TaxCaliber`（必需，CONTEXT「必须声明」）、
`VolumetricCaliber`（必需，按方向的必需与禁止已在类型里）、`FxCaliber`（**可缺**——不涉及外币
的政策没有汇率口径，缺席是合法声明，与忘了填分开）。**一致性**：体积口径的方向必须等于政策
自己的方向，且口径只能随 `PricePolicyBody` 同一次发布登记——在 `declarationWrites` 里核，
两者都在场才写，只给口径不给正文整项拒绝。

加点规则按票 02 裁决 (a) 不进本表；重启条件在票 02。

## 边界

地盘：`internal/partycommercial/`、`migrations/party_commercial/`（占 0022）、`cmd/parcel-commercial/`
（批文翻译）。**不动 `parcel-pricing`**，不动 0010，不动 `CommercialPricePolicy`。
HTTP 发布口的 Intake 仍是未配置，本票不碰。

## Comments

- 2026-09-03 · MCP-1：**落地于 `29085fe`，转 resolved。** 「两处要定形的地方」照裁决落：一、
  `planDirection` 走 (a) 批文声明，批文文档注释写明抄错 `checkPlanBinding` 守不住、在线口那天必须
  改为现问；二、口径走族 B——独立表 `0022_price_policy_caliber`、具名 `SavePricePolicyCaliber`、
  点读口 `PricePolicyCaliberView`、`PricePolicyRow` 扩字段带 `HasCaliber`。

  **一处票面没写、落地时定的，写清是决定不是越界**：0022 在 0010 的表上补了一条
  `UNIQUE (…4 键…, direction)` 作外键落点，让口径表的外键带上方向。票面「不动 0010」指不改写
  0010 文件与 `CommercialPricePolicy` 结构体，加约束不违背；不这样做，「口径只能挂在同方向的
  政策正文行上」在库上就没有落点，只剩用例里 `ConsistentWithDirection` 一道守——而两张表两条写入
  的一致性只靠编排守，正是 `ConsistentAcceptanceRulePackage` 那条注释说的会把 A 方向的系数装进
  B 方向的政策的形状。真库用例 `TestAPricePolicyCaliberNeedsABodyRowOfTheSameDirection` 钉住了两半
  （无正文行拒、方向不符拒）。

  **批文里体积口径不另收方向**：`pricePolicyCaliberDocument` 没有 direction 键，体积方向直接取政策
  方向。批文口因此在结构上造不出方向不一致的口径；用例那道核守的是绕开翻译的调用方。

  **越出票面地盘的改动：无。** `cmd/parcel-api/unwired_orchestration.go` 未动——`PricePolicyRow`
  是结构体加字段，不拆调用点。

  **「生产可达」差什么**：写侧经受控 CLI 可达；HTTP 发布口 Intake 仍未配置（PAR-INT-01，与本票
  无关）；`PricePolicyCaliberView` 今天**零生产调用方**——消费方是 `parcel-pricing` 登记参考序列时
  按 `quoteBasis` 冻结口径正文（票 02 裁「登记时冻结」），尚未接，落在哪一组票由 owner 定（与
  MCP-3 新开的 `.scratch/pricing-reference-series-operations/` 是邻居）。

  **接力与中断点（证据是会话转录与文件 mtime，不是推测）**：MCP-2 写完领域、端口、应用层与
  HTTP 桩后转录停在 12:15:12，最后一次落盘 12:15:02，postgres 适配器缺 `SavePricePolicyCaliber`
  让全仓 `go build` 红了约一小时；MCP-5 于 13:17 前后广播接手，写完 0022、postgres 写口与点读口、目录
  左连接、HTTP 行体、CLI 批文与全部真库用例，跑过一次含 PG 的全仓并修掉 0022 的 CRLF，转录停在
  13:35:00；MCP-1 于 13:39 经 owner 指示接手，只做复核、验证、提交、票面。

  **验证**：先在共享树跑，再按 parallel-sessions 处方在 detached 临时 worktree 检出 `29085fe`
  重跑，两处都是——`gofmt -l .` 为空、`go build ./...` 与 `go vet ./...` 退 0、
  `go test -p 1 -count=1 ./...` 零 FAIL。**这是「绿（含 PG）」**：DSN 指向门禁容器
  `idp-parcel-postgres-gate`，本票真库用例在 `-v` 下逐条 `PASS`（不是 `SKIP`）——口径带/不带汇率
  往返、缺口径 found=false、重放与冲突（含汇率格从缺席变在场）、租户绑定、无正文行与方向不符
  两半被外键拒、耦合破缺（含税无分类、不适用带分类、采购带系数、销售无系数、汇率半缺）被 CHECK
  拒、目录左连接。`-race` 本笔未跑，与本批其余票同归收尾。
  临时 worktree 拆前 `git status --untracked-files=all` 零行，未加 `--force`。

  **两轴评审**：接手方 MCP-1 串行复核，不是独立子代理。Spec 轴对照票面判据逐条——`planDirection`
  不推断、`conversion` 缺席不代填 `NONE`、口径随正文同笔且正文先写、方向一致双守、汇率缺席不落
  零值、found=false 只表缺行而坏数据（税务耦合破缺、汇率半缺）走 error、加点不落类型且批文
  `markup` 键拒收——无发现。Standards 轴：注释无变更叙述、无行号引用、无新增计数；无发现。
