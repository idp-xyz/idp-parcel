# 合成 `S` 演示动线脚本

隔离环境下「产品就绪 = 可演示」的一条可执行动线：租户的物流产品经理用合成 `S` 数据，从建服务产品一路走到委托那一步，全程在管理台里看。

**这份文档不是什么**：它不是功能清单，也不是验收依据；它不定义任何领域规则，规则一律以各 `CONTEXT.md`、`UC-*` 与 ADR 为准，本文只引用。它也不承诺一条闭环——**动线在委托那一步如实停住**，停在哪、为什么停、什么条件才会重新往前，是本文的主体内容之一，不是遗漏。

**为什么停住也值得演示**：按 [ADR-0017](../adr/0017-admission-gates-judged-by-blocking-cause.md)（闸门按阻断原因判读）与 [ADR-0029](../adr/0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)（取数失败按恢复动作分格）的口径，`未配置` 不是缺陷而是产品的诚实：实例半边留空、拒绝默认值是[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)的红线要求。一条走到墙前能说清「这不是目录为空，是这一格根本没人配过，配它要先满足什么」的动线，演示的正是机制半边已经做完了。反过来，为了让动线好看而在库里塞几行假委托，破的恰是它要证明的那件事。

## 前置

| 件 | 取值 | 备注 |
|---|---|---|
| 演示库 | `postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable` | 本机隔离库；**任何一步都不得指向生产库** |
| 种子 | `scripts/demo-seeds/seed.sh` | 合成 `SYN-` 主数据，四条登记 CLI 灌入；复灌用 `--reset` |
| 后端 | `cmd/parcel-api` | 需 `IDP_PARCEL_ISOLATED_READ_TENANT=SYN-TENANT-01`；要演示委托提交另加 `IDP_PARCEL_ISOLATED_WRITE_TENANT`，同值（ADR-0091） |
| 管理台 | `apps/admin-web` | dev 服务器**从 WSL 起**（本机 `node_modules` 是 WSL 侧 pnpm 装的 POSIX 链接农场，Windows 进程解析不到属预期），用 `PARCEL_API_TARGET` 指向后端；原样命令见「取证」页面层一节 |

起后端（本机 8080 被 Windows 服务占用，换端口，见 admin-web README 的暗礁一节）：

```powershell
$env:IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable'
$env:IDP_PARCEL_HTTP_ADDR=':19080'
$env:IDP_PARCEL_ISOLATED_READ_TENANT='SYN-TENANT-01'
go run ./cmd/parcel-api
```

启动日志必须出现这一行，否则后面每一页都会是`未配置`态：

```json
{"level":"INFO","msg":"Isolated read admission enabled (ADR-0078): operations query endpoints answer with injected synthetic scope","tenant":"SYN-TENANT-01"}
```

放行必须出声是 [ADR-0078](../adr/0078-isolated-environment-operations-reads-admit-by-assembly-injection.md) 的要求，不是日志噪音：隔离读面把八条运营查阅端点从`未配置`切成可答，这件事要在事后可查。

## 动线

角色：**租户的物流产品经理**。顺序即种子包的数据故事顺序（见 `scripts/demo-seeds/README.md`），一句话串起来是「建产品 → 配价 → 一单会走的网 → 过关的规则 → 一单委托」。

### 第 1 步 · 建产品（party-commercial）

| 看哪页 | 取哪个端点 |
|---|---|
| 服务产品与渠道 | `GET /commercial-service-products` |
| 商业规则与策略 | `GET /commercial-policies?kind=…` |

讲的是：服务产品 `SYN-PROD-CN-SG-EXPRESS`（中国→新加坡合成快递）携待路由许可；接单规则包 `SYN-RULEPKG-01` 的适用性钉住（产品，合同，法人，范围）四维；时点锚策略两条。

**这一步就要指出的一件事**：商业规则与策略页切到「价格政策」与「结算政策」两类时是**空的**，而且是有答案的空——端点答 `200` 带 `COMMERCIAL_POLICIES_LISTED` 和一个空册。原因如实：这两张表的持久化面在、进程级写入口不在（`scripts/demo-seeds/README.md` 的「已知边界」一节记着 `SavePricePolicy` / `SaveSettlementPolicy` 无 cmd 调用方）。按 [ADR-0077](../adr/0077-master-data-catalogue-read-follows-the-operations-read-pattern.md) 空册本身就是内容。

把这一格讲清楚，后面第 5 步的墙就不用重新解释了：**空态与未配置态是两件事**，同一套页面用两种呈现分开表达，管理台的四态组件就是为这个分的。

### 第 2 步 · 配价（parcel-pricing）

| 看哪页 | 取哪个端点 |
|---|---|
| 价卡目录 | `GET /pricing-price-cards` |
| 计价参考序列 | `GET /pricing-reference-series` |

讲的是：售价卡 `SYN-PLAN-CN-SG-01`（首重 0.5kg ¥55、续重 ¥18/0.5kg，Z1/Z2 两区，MAX 计费重体积系数 5000）与成本卡 `SYN-PLAN-CN-SG-COST-01`（BUY）成对；燃油序列与汇率序列各一条，汇率的口径引价格规则 `SYN-PRICE-RULE-CN-SG`（汇率不收裸值）。

价卡行带 `canonicalization` 与 `contentDigest` 两列，可以顺带讲一句版本化规范化摘要（[ADR-0014](../adr/0014-versioned-canonicalization-shape-for-content-digest.md)）：同一份内容换一次规范化形状就换一次版本号，旧摘要不被改写。

### 第 3 步 · 一单会走的网（network-routing）

| 看哪页 | 取哪个端点 |
|---|---|
| 网络目录 | `GET /network-catalog?family=…`（六族切页签） |
| 服务区域与覆盖 | `GET /network-catalog?family=service-area` |

讲的是：上海枢纽→深圳口岸→新加坡枢纽→新加坡末端四节点、三连接，成线路 `SYN-LINE-CN-SG-01`；上海枢纽 v1→v2 换版展示版本轴；一次台风停运（已解除）展示临时调整与稳定定义分离。

服务区域页的**地理覆盖列刻意不存在**，页面用如实说明交代它属 `PAR-NET-14`、形态定了才以新迁移扩列，并且不为它发请求。这是管理台「骨架有、读面无 → 不上列」通则的先例（见 `apps/admin-web/README.md` 的列表页上列通则），演示时值得点一句：不填假值本身是产品行为。

### 第 4 步 · 过关的规则（customs-compliance）

| 看哪页 | 取哪个端点 |
|---|---|
| 合规规则库 | `GET /customs-compliance-rules?registry=…`（两本册子） |

讲的是：放行结果解释规则 v1→v2 换版；建案要求两向——CN 出口要求建案、SG 进口**显式不要求**。后者是这一步的重点：`显式不要求`与`没登记`在册子里是两行不同的事实，不是同一种空。

### 第 5 步 · 一单委托：动线在此如实停住

| 看哪页 | 取哪个端点 | 这一页会怎样 |
|---|---|---|
| 委托查阅 | `GET /shipment-request-views` | **空态**：`200`，`{"outcome":"LISTED","requests":[]}` |
| 追踪投影 | `GET /tracking-projections` | **空态**：`200`，`{"outcome":"PROJECTIONS_LISTED","projections":[]}` |
| 提交委托 | `POST /shipment-requests` | 见下面「委托侧两种跑法」 |
| 取消包裹 | `POST /shipment-requests/parcel-cancellations` | **未配置态**：`403`，`ACCESS_CHANNEL_NOT_CONFIGURED` |

两种状态挨在一起出现，正好把第 1 步埋的那句话兑现：查阅面已经放行了（读得到，答的是「册里没有」），写面根本没放行（连问都没问成，答的是「这条渠道没人配过」）。管理台在这两态下的呈现不同，且**未配置态不得显示「0 条」**——那会与状态区「这不是目录为空」自相矛盾。

#### 委托侧两种跑法

[ADR-0091](../adr/0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 之后，提交那一行由第二个开关决定，与读开关分设：

| 起进程时 | `POST /shipment-requests` |
|---|---|
| 只设 `IDP_PARCEL_ISOLATED_READ_TENANT` | **未配置态**：`403`，`ACCESS_CHANNEL_NOT_CONFIGURED`——读开关换不了写行 |
| 另加 `IDP_PARCEL_ISOLATED_WRITE_TENANT='SYN-TENANT-01'` | 走到编排：合成种子里那条委托受理维的权威区间在册，归属答本产品承接，委托建成`已提交` |

第一种跑法适合讲「门是分级的」，第二种适合讲「墙拆掉之后链路真的通」。**两个开关取值必须相同**，不同则进程启动即拒——写下的委托挂在一个读面不过滤的租户上，页面就看不见它。

委托侧原本是三堵各自独立的墙，任一堵单独就足以拦住。前两堵已按 ADR-0091 在隔离形态下拆掉，第三堵仍在。演示时按这个顺序讲：

**墙一 · 委托侧的入库通道（隔离形态已开，生产仍拦）。** 各上下文的写端点默认装 `UnconfiguredIntake{}`（`cmd/parcel-api` 的 `assembleBusinessEndpoints`）。ADR-0091 只把 `/shipment-requests` 一行改成按写开关换值，其余命令面仍是不经任何变量的字面量。主数据那四条通道是登记 CLI，委托侧没有对应物；`cmd/parcel-dispatch` 也不是入口，它只转投已在 Outbox 里的信封。
**生产侧的重启条件不变**：`PAR-INT-01` 最低证据到位（该租户渠道的现行流程）。[ADR-0072](../adr/0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md) 已裁定这份能力归共享接入身份技术能力、落点 `internal/accessidentity/`，并**维持** [ADR-0055](../adr/0055-business-endpoint-intake-has-an-unconfigured-grade.md) 对运行时渠道登记表的否决——登记册形状等真实渠道证据，不预先替租户拟。隔离形态的那个 Intake 翻译的是本仓自己那张管理台页面的草案形状，不是任何租户的渠道契约，**不能当成 `PAR-INT-01` 已有答案**。

**墙二 · 生产归属（隔离形态已开，生产仍拦）。** 生产形态下 `buildSubmissionOrchestration` 把 `Directory` 与 `SelfAuthority` 留空（实例半边，不代拟坐标），而归属适配器的 `governanceScope` 在这两样任一缺席时**先于**读治理登记册就返回未配置，归属如实答`权威未确定`，提交停在 `OWNERSHIP_UNRESOLVED`。**注意这一格不是「登记一条权威区间」就能解开的**——目录缺席时那条区间根本不会被读到。
隔离形态下两格由装配注入合成值，但**归属仍要真的读登记册**：把种子里那条 `07-authority-interval-shipment-intake.json` 删掉再灌，提交会退回 `OWNERSHIP_UNRESOLVED`。这一手是现场证明「合成目录不是一句谎话」的最短路径。

**墙三 · 路由拿不到证据。** 就算前两堵都过、委托成`已接受`，初始路由会停在 `RouteEvidenceNotConfigured`：三个证据视图只读 `network_routing.network_definition`，而那张表至今零生产写入方（第 3 步登记的是另一套目录表，`bumpRevision` 推的是目录修订锚，长不出这张表的行）。没有路由就没有下游的费用。
**重启条件**：解析层——把目录折成逐候选事实。被 `PAR-NET-14` 阻断，且 [ADR-0068](../adr/0068-versioned-network-catalog-structure-precedes-rule-content.md) Consequences 已明文接受这段「目录可写可读、尚无人读它产出事实」的时期。

三堵墙对应的机制票都在 `.scratch/syn-wall-door-audit/issues/`（依次为 01、13、04），墙面清单见同目录 `report.md`。

## 对照组：证明门是真的

演示里最容易被质疑的一句是「你们这个 403 是写死的吧」。三态对照可以当场答掉，三条都只动一个环境变量：

| 环境变量 | 结果 |
|---|---|
| 不设 `IDP_PARCEL_ISOLATED_READ_TENANT` | **全部**端点答 `403`，包括第 1–4 步走过的那些 |
| `IDP_PARCEL_ISOLATED_READ_TENANT=SYN-TENANT-01` | 运营查阅端点答 `200`，其余仍 `403`——**含提交口** |
| 再加 `IDP_PARCEL_ISOLATED_WRITE_TENANT=SYN-TENANT-01` | 提交口走到编排，其余命令面仍 `403` |
| 任一开关取 `TENANT-PROD-1`（无 `SYN-` 前缀） | **进程启动即拒**，带原因退出，不静默回落 |
| 两开关取不同的 `SYN-` 租户 | **进程启动即拒**，报文同时点名两个开关 |

第二、三行挨着看是这套门禁分级的证据：读开关开到底也开不了写行，那是 [ADR-0091](../adr/0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 把两个开关分设的全部理由——合一的话，今天所有设了读开关的环境会在升级那一刻静默获得写准入。

后两条是重点：静默回落会让「配置错了」与「刻意拦着」两态的可观察签名变成同一个，那正是 ADR-0078 要避免的「默认值不出声」病。演示时把它们留到最后，比前几条更能说明这套门禁不是摆设。

## 复灌

```bash
IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' \
  ./scripts/demo-seeds/seed.sh --reset
```

`--reset` 是破坏性动作（DROP 全部 parcel schema → 重迁 → 重灌），**只属于隔离演示库**。对已灌过的库不要用不带 `--reset` 的重跑：四条 CLI 各自幂等，但网络目录的版本行撞主键会答未决。

## 取证

以下断言实测于 `627d339`，演示库为本机 `idp-parcel-postgres-gate`（`postgres:16.14`，`127.0.0.1:55432`），种子为票 `master-data-wiring/08` 的合成包。行数写在这里是因为**数本身就是论点**——「这一格有数据」与「这一格是空的」的分界正是动线第 5 步要讲的东西；它们锚在上述 SHA 与那一份种子上，换一份种子就要重取。

放行面全部 `200`。放行的是八条端点，下表按页面实际发出的查询参数展开，因此行数多于八：

| 端点 | outcome | 行 |
|---|---|---|
| `/pricing-price-cards` | `PRICE_CARDS_LISTED` | 2 |
| `/pricing-reference-series` | `REFERENCE_SERIES_LISTED` | 2 |
| `/network-catalog?family=node` | `NODE_VERSIONS_LISTED` | 5 |
| `/network-catalog?family=connection` | `CONNECTION_VERSIONS_LISTED` | 3 |
| `/network-catalog?family=line` | `LINE_VERSIONS_LISTED` | 1 |
| `/network-catalog?family=service-area` | `SERVICE_AREA_VERSIONS_LISTED` | 2 |
| `/network-catalog?family=service-calendar` | `SERVICE_CALENDAR_VERSIONS_LISTED` | 1 |
| `/network-catalog?family=availability-adjustment` | `AVAILABILITY_ADJUSTMENTS_LISTED` | 1 |
| `/network-catalog?family=route-strategy` | `ROUTE_STRATEGY_VERSIONS_LISTED` | 1 |
| `/customs-compliance-rules?registry=case-requirement` | `CASE_REQUIREMENT_RULES_LISTED` | 2 |
| `/customs-compliance-rules?registry=interpretation` | `INTERPRETATION_RULES_LISTED` | 2 |
| `/commercial-service-products` | `SERVICE_PRODUCTS_LISTED` | 1 |
| `/commercial-policies?kind=ACCEPTANCE_RULE_PACKAGE` | `COMMERCIAL_POLICIES_LISTED` | 1 |
| `/commercial-policies?kind=PRE_ACCEPTANCE_CONTROL` | `COMMERCIAL_POLICIES_LISTED` | 1 |
| `/commercial-policies?kind=AS_OF_POLICY` | `COMMERCIAL_POLICIES_LISTED` | 2 |
| `/commercial-policies?kind=PRICE_POLICY` | `COMMERCIAL_POLICIES_LISTED` | 0（空态，见第 1 步） |
| `/commercial-policies?kind=SETTLEMENT_POLICY` | `COMMERCIAL_POLICIES_LISTED` | 0（同上） |
| `/shipment-request-views` | `LISTED` | 0（空态，见第 5 步） |
| `/tracking-projections` | `PROJECTIONS_LISTED` | 0（同上） |

拒绝面：`/customer-tracking-view` 与八条命令端点全部 `403`，包封为 `{"error":{"code":"ACCESS_CHANNEL_NOT_CONFIGURED"}}`。三态对照按上表逐条复现，非 `SYN-` 前缀那次进程以退出码 1 停下并在错误里点名所需前缀。

库内委托侧逐张 0 行：`parcel_shipment` 的 `source_submission` / `shipment_request` / `final_outcome`、`node_operations.reception`、`transport_fulfillment.effective_delivery`、`visibility_exception` 的 `customer_view` / `tracking_projection_current`、`settlement_accounting.customer_charge`；`network_routing.network_definition` 亦为 0 行（墙三）。

**页面层取证（后补，实测于 `9213acf` 树，种子先经 `--reset` 复灌重验）**：本节初版（存于 git 史 `2b37b30`）曾记页面层无法取证并把原因定在链接农场失效上，定因错了——`apps/admin-web/node_modules` 不是坏，是 **WSL 侧 pnpm 装的**（POSIX 符号链接农场，Windows 进程解析不到属预期；这次安装的来历见票 `master-data-wiring/07` 的环境注记）。从 WSL 起 dev 服务器即可用，无需 PAT、无需碰 `github.com`：

```bash
# WSL 内起 dev 服务器（node 22 在 ~/.local/node22；本机镜像网络下
# 127.0.0.1 与 Windows 侧互通——种子脚本连 55432、vite 代理连 API 皆为实证）
cd /mnt/d/tops/idp-parcel/apps/admin-web
PATH=$HOME/.local/node22/bin:$PATH PARCEL_API_TARGET=http://127.0.0.1:19080 \
  node node_modules/vite/bin/vite.js --port 5199
```

页面层结果（Edge 无头 `--dump-dom --virtual-time-budget=9000` 按 hash 路由逐页取默认视图；本轮实测 API 监听 `:18091`、vite `:5199`——端口任选，前后一致即可）：

| 页（hash 路由） | 所见 |
|---|---|
| `#/price-card-catalog` | `SYN-PLAN-CN-SG-01` 等价卡行在列 |
| `#/reference-series` | `SYN-SERIES-FUEL-01` 等序列行在列 |
| `#/network-catalog` | `SYN-NODE-SHA-HUB`（含 v1→v2 版本轴）在列 |
| `#/service-areas` | 2 个版本在列，地理覆盖如实标「尚不存在（PAR-NET-14）」，不虚构覆盖关系 |
| `#/compliance-rules` | 默认册（建案要求）两行在列：`SYN-PROC-CN-EXPORT` 要求、`SYN-PROC-SG-IMPORT` 显式不要求 |
| `#/service-products` | `SYN-PROD-CN-SG-EXPRESS` v1 `EFFECTIVE` 在列 |
| `#/commercial-policies` | `SYN-RULEPKG-01` 在列（默认种类） |
| `#/shipment-request-inquiry` | 空态：「共 0 个 · 当前作用域内没有可见委托 · 空列表是正常业务答案（LISTED）」 |
| `#/tracking-projection` | 空态：「共 0 个 · 当前租户内尚无投影 · 空列表是正常业务答案（PROJECTIONS_LISTED）」 |
| `#/workbench` | 就绪度总览：已接线 11、页面骨架 23、合成 S 演示 1 |

十页 DOM 无一处 `ACCESS_CHANNEL_NOT_CONFIGURED`——未配置态整片退场，与端点层三态对照互为印证。无头取证只覆盖各页默认视图；册子/族/种类的切换分支已在端点层逐参数实测（上表），页面切换走同一代码路径（参数变体的接线核对见票 `master-data-wiring/07` 收口记录）。验完请停掉 api 与 dev 进程，防「api 绑错库」被下一轮当成已接好（同票 07 的清场纪律）。

## 这条动线什么时候会变

- **墙降了**：三堵墙任一堵的重启条件满足并落地后，第 5 步要重写——那时委托侧会有数据，动线才谈得上闭环。降墙的判据不看本文，看对应机制票。
- **页面增减**：`apps/admin-web/src/page-registry.tsx` 的 `liveIds` 是接线事实的唯一登记处。本文按当时的 `liveIds` 挑页，**规划时以那份代码为准，不以本文为准**——代码不会与自己漂。
- **种子换了**：上面「取证」一节的行数随种子包变。种子包换内容时，那张表要重取，不要照抄。
