# 合成 `S` 演示动线脚本

隔离环境下「产品就绪 = 可演示」的一条可执行动线：租户的物流产品经理用合成 `S` 数据，从建服务产品一路走到委托那一步，全程在管理台里看。

**这份文档不是什么**：它不是功能清单，也不是验收依据；它不定义任何领域规则，规则一律以各 `CONTEXT.md`、`UC-*` 与 ADR 为准，本文只引用。它也不承诺一条闭环——**动线在委托那一步如实停住**，停在哪、为什么停、什么条件才会重新往前，是本文的主体内容之一，不是遗漏。

**为什么停住也值得演示**：按 [ADR-0017](../adr/0017-admission-gates-judged-by-blocking-cause.md)（闸门按阻断原因判读）与 [ADR-0029](../adr/0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)（取数失败按恢复动作分格）的口径，`未配置` 不是缺陷而是产品的诚实：实例半边留空、拒绝默认值是[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)的红线要求。一条走到墙前能说清「这不是目录为空，是这一格根本没人配过，配它要先满足什么」的动线，演示的正是机制半边已经做完了。反过来，为了让动线好看而在库里塞几行假委托，破的恰是它要证明的那件事。

## 前置

| 件 | 取值 | 备注 |
|---|---|---|
| 演示库 | `postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable` | 本机隔离库；**任何一步都不得指向生产库** |
| 种子 | `scripts/demo-seeds/seed.sh` | 合成 `SYN-` 主数据，四条登记 CLI 灌入；复灌用 `--reset` |
| 演示发行方 | `scripts/demo-seeds/operator-channel/compose.yaml` | Dex；`docker compose -f … up -d --wait`，用法见 `scripts/demo-seeds/README.md`「操作者渠道」一节 |
| 后端 | `cmd/parcel-api` | 运营查阅各口走操作者渠道：发行方三件加治理自授权，请求带配置员令牌；委托查阅及其子集视图另需 `IDP_PARCEL_ISOLATED_READ_TENANT=SYN-TENANT-01`（票 operator-channel/53 之前）；要演示委托提交另加首方客户族三件（票 first-party-api/00，请求带合成货主系统的令牌）与 `IDP_PARCEL_ISOLATED_WRITE_TENANT`（只给这个租户注入归属坐标），同值（ADR-0091） |
| 管理台 | `apps/admin-web` | dev 服务器**从 WSL 起**（本机 `node_modules` 是 WSL 侧 pnpm 装的 POSIX 链接农场，Windows 进程解析不到属预期），用 `PARCEL_API_TARGET` 指向后端；原样命令见「取证」页面层一节 |

起后端（本机 8080 被 Windows 服务占用，换端口，见 admin-web README 的暗礁一节）：

```powershell
$env:IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable'
$env:IDP_PARCEL_HTTP_ADDR=':19080'
# 操作者渠道（ADR-0100）：运营查阅各口与运营决定各口都走它；治理自授权与种子里权威区间的权威串逐字相同（ADR-0180）
$env:IDP_PARCEL_OPERATOR_OIDC_ISSUER='http://127.0.0.1:5556/dex'
$env:IDP_PARCEL_OPERATOR_OIDC_JWKS_URL='http://127.0.0.1:5556/dex/keys'
$env:IDP_PARCEL_OPERATOR_OIDC_AUDIENCE='idp-parcel-admin-web'
$env:IDP_PARCEL_GOVERNANCE_SELF_AUTHORITY='SYN-AUTH/idp-parcel-pilot'
# 集成客户端渠道（ADR-0149）：外部结果、监管凭证与外部资金事实各口走它；发行方是 compose 里另起的那一只
$env:IDP_PARCEL_INTEGRATION_CLIENT_OIDC_ISSUER='http://127.0.0.1:5558/dex'
$env:IDP_PARCEL_INTEGRATION_CLIENT_OIDC_JWKS_URL='http://127.0.0.1:5558/dex/keys'
$env:IDP_PARCEL_INTEGRATION_CLIENT_OIDC_AUDIENCE='idp-parcel-integration'
# 首方客户渠道（ADR-0139）：提交口走它；发行方与集成客户端族同一只，受众另是一个（票 first-party-api/00）
$env:IDP_PARCEL_CUSTOMER_API_OIDC_ISSUER='http://127.0.0.1:5558/dex'
$env:IDP_PARCEL_CUSTOMER_API_OIDC_JWKS_URL='http://127.0.0.1:5558/dex/keys'
$env:IDP_PARCEL_CUSTOMER_API_OIDC_AUDIENCE='idp-parcel-customer-api'
# 委托查阅及其子集视图仍走隔离读（票 operator-channel/53 之前）
$env:IDP_PARCEL_ISOLATED_READ_TENANT='SYN-TENANT-01'
# 要演示委托提交才需要下面这行：它不放行提交口，只给这个租户的归属注入合成坐标（PS 生产目录立起之前）；两值必须相同（ADR-0091）
$env:IDP_PARCEL_ISOLATED_WRITE_TENANT='SYN-TENANT-01'
go run ./cmd/parcel-api
```

运营查阅各口要带配置员令牌（WSL 内取，脚本走一次 Dex 登录）：

```bash
TOKEN=$(./scripts/demo-seeds/operator-channel/operator-token.sh syn-operator-config-01@syn-tenant-01.example.invalid)
curl -s -H "Authorization: Bearer $TOKEN" http://127.0.0.1:19080/network-catalog?family=node
```

启动日志应出现这几行。缺发行方那一行，运营查阅与运营决定各页答`未配置`；缺治理自授权那一行，查阅各页照常，运营决定各口答 `OUTSIDE_ADMISSION_SCOPE`；缺隔离读那一行，委托查阅一族答`未配置`：

```json
{"level":"INFO","msg":"Operator channel issuer configured (ADR-0100): operator tokens verified against the issuer's JWKS"}
{"level":"INFO","msg":"Governance self authority configured (ADR-0180): admission scope reads tenant mappings and this product's authority intervals"}
{"level":"INFO","msg":"Isolated read admission enabled (ADR-0078): shipment request views answer with injected synthetic scope","tenant":"SYN-TENANT-01"}
```

放行必须出声是 [ADR-0078](../adr/0078-isolated-environment-operations-reads-admit-by-assembly-injection.md) 的要求，不是日志噪音：隔离读面把委托查阅及其子集视图从`未配置`切成可答，这件事要在事后可查。设了写开关时另有一行 ADR-0091 的同款声明，带 `selfAuthority`。不带令牌打运营查阅各口答 `401 OPERATOR_CREDENTIAL_REJECTED`，持令牌但授予不足答 `403 OPERATOR_NOT_GRANTED`。作业事实各口（收寄、揽收、交接、移动、派送、交付）另带设备三头 `Parcel-Device-Id`、`Parcel-Work-Scope-Kind`、`Parcel-Work-Scope-Reference`，设备取 `scripts/demo-seeds/data/access/04-devices.json` 登的合成设备。收寄只认 `NODE` 范围，运输履约各口只认 `OFFSITE` 范围；种类不对，在核令牌之前就答 `400 MALFORMED_REQUEST`，那个 400 不说明越过了渠道。范围报的是配置员持有授予、却不是这台设备登的那一个，答 `403 DEVICE_NOT_IN_USE`；报一个没有授予的范围，答 `403 OPERATOR_NOT_GRANTED`。外部结果、监管凭证与外部资金事实各口走集成客户端渠道：令牌由 `scripts/demo-seeds/operator-channel/integration-token.sh` 向集成客户端发行方要，合成报关服务商授外部结果与监管凭证，合成银行授外部资金事实；授错一类事实答 `403 INTEGRATION_CLIENT_NOT_GRANTED`，拿操作者令牌来答 `401 INTEGRATION_CLIENT_CREDENTIAL_REJECTED`。

### 起 dispatch（只在演示委托提交时需要）

**`cmd/parcel-api` 只把委托建到`已提交`就交出信封，接受判断链由 `cmd/parcel-dispatch` 驱动**（ADR-0081）。不起它，委托会一直停在`已提交`而没有任何报错——那个静默此前没写进本脚本，实测时踩过一次。

```powershell
$env:IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable'
$env:IDP_PARCEL_ROUTE_SERVICE_PURPOSE='NETWORK_SERVICE'
$env:IDP_PARCEL_DISPATCH_INTERVAL='2s'
$env:IDP_PARCEL_DISPATCH_DELIVERY_TIMEOUT='5s'
$env:IDP_PARCEL_DISPATCH_LIMIT='10'
$env:IDP_PARCEL_DISPATCH_LEASE='1m'
$env:IDP_PARCEL_DISPATCH_MAX_ATTEMPTS='3'
$env:IDP_PARCEL_DISPATCH_RETRY_AFTER='10s'
go run ./cmd/parcel-dispatch
```

七个变量一个都不能省——缺一个进程启动即拒并指名缺哪个（那是刻意的，见各变量在 `cmd/parcel-dispatch/assemble.go` 的解析）。上面的取值是演示用的短拍子，不是生产建议值。

**dispatch 对逐条投递失败一行日志都不打，全程静默不代表链跑通了。** `Dispatcher.DispatchOnce` 把失败码写进库那一行就 `continue`，进程循环只在整拍失败（认领不到、定稿写不进去）时才出声。要看它把委托推到哪一步，查 `bento.outbox` 那一行的 `status` 与 `failure_code`，别只看进程有没有报错——失败码之外的东西（停在哪一站、未决的原因是什么）今天在真进程上根本取不到，那是另一个已知缺口，记在 `.scratch/first-tenant-runway/issues/08`。实测于 2026-09-02：只灌治理权威区间、不灌商业主数据时，链如实答未决，重投烧完预算后那一行落 `ABANDONED` + `dispatch.consumer_undecided`，而委托停在`已提交`——那是一个已知缺口，记在 `.scratch/first-tenant-runway/issues/07`，演示前先读它，免得把它当成现场故障。

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

> 2026-10-10 补记（票 `routing-first-cut/11`）：上两段写于当时，原样保留。这一步的目录今天是演示租户**显式采用**随产品发布的参考配置 `network-routing/network-catalog/SYN-CN-SG@1` 登进去的（[ADR-0147](../adr/0147-reference-configuration-ships-embedded-and-is-adopted-through-ordinary-registration.md)），每一版定义行的依据格是那一版的引用串。网络本身多了几样：线路与路由策略的适用范围是服务目的 `NETWORK_SERVICE`，策略声明成本单维，服务区域带 CN / SG 覆盖与收寄、交付节点，四个节点各一份带处理时长的日历，线路三段挂 BUY 价卡引用。上海枢纽修订 2 是采用后的租户修订，依据换成了演示租户自己的。服务区域覆盖的列已由迁移 `0011` 补上；管理台服务区域页今天怎么显示，本票没核。

> 2026-10-19 补记（票 `demo-chain-gaps/16`，[ADR-0201](../adr/0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md)）：网走得通之后，货怎么运也有了登记。种子给三条连接各登一份按日运行的周期安排（`scripts/demo-seeds/data/transport/`），`GET /transport-fulfillment-records?registry=transport-schedule-template` 列出这三份；`POST /transport-fulfillment-transport-schedule-registrations` 带 `{"template":"SYN-SCHED-SZX-SIN-DAILY","runningDate":"<日期>"}` 显式登一个班次答 `201` `SCHEDULE_REGISTERED`，同一日期再登答 `200` `EXISTING_SCHEDULE`，班次身份是 `SYN-SCHED-SZX-SIN-DAILY/<日期>`。各段的履约方式是演示租户的声明（租户取值，只记 `S`）：`SHA-HUB→SZX-GATE` 自营；`SZX-GATE→SIN-HUB` 外包给 `SYN-PARTY-CARRIER-TRUNK-01`，采用 `SYN-SUPPLIER-TRUNK-CN-SG-01` v1；`SIN-HUB→SIN-LM` 与末端派送自营、经一线设备；`SYN-SUPPLIER-LASTMILE-SG-01` 暂不被任何委托采用。2026-10-19 在一次性库上按 `seed.sh` 灌入退 0，经操作者渠道取证上面两口（实测，只记 `S`），走完已删库。

> 2026-10-19 补记（票 `demo-chain-gaps/17`，[ADR-0201](../adr/0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md) 决定三与文末补记）：三份周期安排各声明了重量与件数两维的硬上限（合成值，见种子 README），按日期登记一班时随之各建一池。种子显式登了干线 `SYN-SCHED-SZX-SIN-DAILY/2026-10-21` 一班，`GET /transport-fulfillment-records?registry=capacity-pool` 列出它的 `…/WEIGHT`（5000 KG）与 `…/PIECES`（1000 PCS）两池；`POST /transport-fulfillment-capacity-pool-registrations` 在那一班上另登一维答 `201` `POOL_ESTABLISHED`、再登答 `200` `EXISTING_POOL`，指向没登过的班次答 `200` `SCHEDULE_NOT_FOUND`。2026-10-19 在一次性库上按 `seed.sh` 灌入退 0，经操作者渠道取证上面三口（实测，只记 `S`），走完已删库。

> 2026-10-19 补记（票 `demo-chain-gaps/23`，[ADR-0202](../adr/0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md)）：外包那一段可以提交运输委托了。`POST /transport-fulfillment-transport-commission-submissions` 挂操作者渠道的「运营决定」能力面，配置员持「运输委托提交」授予（种子已登，准入对照与本产品权威区间同登）。载荷：`{"commission":"SYN-COMMISSION-TRUNK-01","provider":"SYN-PARTY-CARRIER-TRUNK-01","agreement":"SYN-SUPPLIER-TRUNK-CN-SG-01","agreementVersion":"v1","conditions":"…","role":"…","responsibility":"…","members":["<包裹号>"],"submittedAt":"<RFC 3339>"}`。干线协议配干线承运方答 `201` `COMMISSION_SUBMITTED`，回显快照 `SYN-SUPPLIER-TRUNK-CN-SG-01/v1` 与从协议取来的采购责任法人 `SYN-LE-01`；原样重交答 `200` `EXISTING_COMMISSION`；服务方换成末端承运商 `SYN-PARTY-CARRIER-LM-01` 答 `200` `PROVIDER_MISMATCH`，带协议约定的供应商 `SYN-PARTY-CARRIER-TRUNK-01`，不落委托。「委托已提交」信封与委托同一笔入队；派发照 ADR-0066 不给它登消费者，日志里每封都是 `dispatch.no_subscriber` 的红行，烧完预算落 `ABANDONED`，那是认下的代价，不是故障。2026-10-19 在一次性库上按 `seed.sh` 灌入退 0，起 `parcel-api` 与 `parcel-dispatch` 经操作者渠道取证上面三答与那封信封的 `ABANDONED` / `dispatch.no_subscriber`（实测，只记 `S`），走完已停进程并删库。

> 2026-10-19 补记（票 `demo-chain-gaps/24`，[ADR-0202](../adr/0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md) 决定二、五与文末补记）：委托之后可以订舱，承运方可以答复。订舱挂「运营决定」能力面的订舱提交一种（配置员已授）：`POST /transport-fulfillment-booking-request-submissions` 带 `{"booking":"SYN-BOOKING-TRUNK-01","commission":"SYN-COMMISSION-TRUNK-01","schedule":"SYN-SCHED-SZX-SIN-DAILY/2026-10-21","quantity":120,"unit":"KG","requestedAt":"<RFC 3339>"}`——班次是种子按日期物化登记的那一班——答 `201` `BOOKING_SUBMITTED`；改引一个没登记的日期答 `200` `SCHEDULE_NOT_FOUND`。承运答复走集成客户端渠道：合成干线承运商的令牌由 `integration-token.sh syn-integration-carrier-trunk-01 syn-carrier-trunk-secret` 要（这台客户端之前起过的集成客户端发行方里没有，先照 `scripts/demo-seeds/README.md`「集成客户端渠道」一节重建那只发行方），`POST /transport-fulfillment/booking-answers` 带 `{"booking":"SYN-BOOKING-TRUNK-01","answer":"<承运方的答复标识>","outcome":"ACCEPTED","quantity":120,"decidedAt":"<RFC 3339>"}` 答 `201` `BOOKING_ANSWERED`，原样重交答 `200` `EXISTING_ANSWER`。答复方的来源身份取自客户端绑定，必须等于订舱所属委托的服务提供方：在末端委托下订的舱，干线承运商来答答 `200` `SOURCE_NOT_SERVICE_PROVIDER`；银行客户端来答答 `403` `INTEGRATION_CLIENT_NOT_GRANTED`，拿操作者令牌来答 `401` `INTEGRATION_CLIENT_CREDENTIAL_REJECTED`。订舱与答复都不入队，查阅口今天也读不到它们，看结果只看两口的答复（ADR-0202 补记风险点 15）。2026-10-19 在一次性库上按 `seed.sh` 灌入退 0；集成客户端发行方另起一只同配置的 Dex（只换宿主端口，`iss` 不变），起 `parcel-api` 经两族渠道取证上面各答（实测，只记 `S`），走完已停进程、删容器、删库。

> 2026-10-19 补记（票 `demo-chain-gaps/29`，[ADR-0209](../adr/0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md)）：干线协议以 `v2` 重发，正文声明收费发生条件（实际履约按件计、失败尝试不计费，订舱与取消不声明）；`v1` 原样在册、不带条件。上面各条里干线「采用 v1」自此读作 `v2`：段采用、委托与发生项登记都指名 `v2`。外包干线段的参与在 SIN-HUB 以下一次交接结束之后，配置员 `POST /transport-fulfillment-charge-occurrence-registrations`，载荷 `{"reason":"ACTUAL_FULFILLMENT","segment":"<段>","objects":["<包裹号>"],"journey":"SYN-JRN-<包裹号>","provider":"SYN-PARTY-CARRIER-TRUNK-01","agreement":"SYN-SUPPLIER-TRUNK-CN-SG-01","agreementVersion":"v2"}`，不再带断言、数量与单位：答 `201` `REGISTERED`，数量 1、单位 `PIECE`，`conditionBasis` 为 `AGREEMENT_DECLARED`，`registeredBy` 是令牌的操作者；原样重登答 `200` `EXISTING`；指名 `v1` 答 `200` `NOT_CONFIGURED` 带回 `…/v1`；载荷带 `conditionAssertion` 或 `quantity`/`unit` 答 `400`。2026-10-19 在一次性库上按 `seed.sh` 灌入退 0，起 `parcel-api` 与 `parcel-dispatch` 经操作者渠道取证上面各答（实测，只记 `S`）：交接用场外设备立段 `SYN-SEG-SZX-SIN-DCG29`，包裹号用合成的 `SYN-PARCEL-DCG29-01`（本机集成客户端发行方的配置早于首方客户端，取不到提交令牌，没走 `submit-one-shipment.sh`）；派发一拍 `charge-occurrence.formed` `PUBLISHED`、BUY 请求一份（协议 `…/v2`），计价因合成包裹不是申报包裹停在计价输入不可得——用提交过的真包裹重走时停点照票 27 的记录（`ZONE_UNRESOLVED`，票 43）。走完已停进程并删库。

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

提交口挂首方客户渠道的 Intake（[ADR-0139](../adr/0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) 决定四），两个隔离开关都换不了它（[ADR-0150](../adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定三）。两种跑法分在写开关设不设：它只管下面「墙二」那一格，给开关那个租户的委托受理注入合成归属坐标。表里前两行是两种跑法共有的门。

| 起进程时 | `POST /shipment-requests` |
|---|---|
| 不设首方客户族三件（隔离开关设不设都一样） | **未配置态**：`403`，`ACCESS_CHANNEL_NOT_CONFIGURED`——开关换不了提交口 |
| 设首方客户族三件，请求不带令牌 | `401`，`CUSTOMER_CLIENT_CREDENTIAL_REJECTED` |
| 设首方客户族三件，不设写开关，请求带合成货主系统的令牌 | 走到编排：归属答`权威未确定`，委托停在 `OWNERSHIP_UNRESOLVED`——这是生产形态的答法，PS 生产归属目录未立（票 [first-party-api/04](../../.scratch/first-party-api/issues/04-ps-production-ownership-directory-before-the-write-switch-retires.md)） |
| 再加 `IDP_PARCEL_ISOLATED_WRITE_TENANT='SYN-TENANT-01'`（`submit-one-shipment.sh` 自己取令牌） | 走到编排：合成种子里那条委托受理维的权威区间在册，归属答本产品承接，委托建成`已提交`。客户端绑的若不是开关那个租户，照旧停在 `OWNERSHIP_UNRESOLVED`（[ADR-0091](../adr/0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 决定六「也不因它出现而获得任何真实租户」） |

管理台的提交页不带首方客户端令牌，在它上面提交答 `401`：运营代客提交是另一个用例（ADR-0139 决定三；票 [first-party-api/05](../../.scratch/first-party-api/issues/05-admin-web-submission-page-has-no-customer-credential.md)）。演示提交走 `submit-one-shipment.sh`。

不设写开关那一种适合讲「门是分级的」，设了的那一种适合讲「墙拆掉之后链路真的通」。**两个开关取值必须相同**，不同则进程启动即拒——写下的委托挂在一个读面不过滤的租户上，页面就看不见它。

委托侧原本是三堵各自独立的墙，任一堵单独就足以拦住。墙一由首方客户渠道拆掉，生产形态同样开；墙二只在隔离形态下、只为开关那个租户拆掉；第三堵的现状见它下面的补记。演示时按这个顺序讲：

**墙一 · 委托侧的入库通道（首方客户渠道，生产形态同样开）。** `/shipment-requests` 在 `cmd/parcel-api` 的 `assembleBusinessEndpoints` 里挂首方客户渠道的 Intake：货主客户的系统以 OAuth 2.0 客户端凭据取令牌，`parcel-api` 验令牌、查客户渠道册与授予，来源信封的租户、客户账户与来源只取自册上那一行，请求里自报什么都不看。它不经任何隔离开关，生产装配里就是这一行；首方族三件不设时整族答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`。撤回、取消等其余客户面命令仍挂不经任何变量的字面量 `UnconfiguredIntake{}`，逐口换上首方 Intake（各自另立票）。`cmd/parcel-dispatch` 不是入口，它只转投已在 Outbox 里的信封。
**生产形态下过了这堵墙仍停在墙二**：PS 生产归属目录未立，任何租户经首方口提交都答 `OWNERSHIP_UNRESOLVED`（票 first-party-api/04）。`PAR-INT-01` 只管给某租户的客户账户开哪一条渠道，与该客户既有渠道（标准文件、门户、非本产品契约的 API）的适配（ADR-0139 决定一），不挡首方渠道；首方 API 的载荷形状是产品契约，不是任何租户的渠道契约，**不能当成 `PAR-INT-01` 已有答案**。

**墙二 · 生产归属（隔离形态下只为开关租户开，生产仍拦）。** 生产形态下 `buildSubmissionOrchestration` 把 `Directory` 与 `SelfAuthority` 留空（实例半边，不代拟坐标），而归属适配器的 `governanceScope` 在这两样任一缺席时**先于**读治理登记册就返回未配置，归属如实答`权威未确定`，提交停在 `OWNERSHIP_UNRESOLVED`。**注意这一格不是「登记一条权威区间」就能解开的**——目录缺席时那条区间根本不会被读到。
隔离形态下两格由装配注入合成值，坐标只给写开关那个租户的拟受理范围——别的租户经首方口进来，照生产形态答`权威未确定`。**归属仍要真的读登记册**：把种子里那条 `07-authority-interval-shipment-intake.json` 删掉再灌，提交会退回 `OWNERSHIP_UNRESOLVED`。这一手是现场证明「合成目录不是一句谎话」的最短路径。

**墙三 · 路由拿不到证据。** 就算前两堵都过、委托成`已接受`，初始路由会停在 `RouteEvidenceNotConfigured`：三个证据视图只读 `network_routing.network_definition`，而那张表至今零生产写入方（第 3 步登记的是另一套目录表，`bumpRevision` 推的是目录修订锚，长不出这张表的行）。没有路由就没有下游的费用。
**重启条件**：解析层——把目录折成逐候选事实。被 `PAR-NET-14` 阻断，且 [ADR-0068](../adr/0068-versioned-network-catalog-structure-precedes-rule-content.md) Consequences 已明文接受这段「目录可写可读、尚无人读它产出事实」的时期。
> 2026-09-24 补记（票 `routing-first-cut/07` 进 main，merge `f2645d86`）：上两段写于当时，原样保留；成因一句自本合入起过时——证据视图不再读 `network_definition`，改读第 3 步登记的那套目录，`未配置`由目录修订锚与适用于服务目的的路由策略版本答（[ADR-0148](../adr/0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定六），阻断也不再归 `PAR-NET-14`（ADR-0146 决定七）。可观察的停点不变：种子策略的适用范围是 `SYN-SCOPE-01`，与调度器的服务目的 `NETWORK_SERVICE` 不一致，种子服务区域也没登覆盖，初始路由照旧停在 `RouteEvidenceNotConfigured`。演示网络经参考配置采用、形成初始路由归 `routing-first-cut/11`。
> 2026-10-10 补记（票 `routing-first-cut/11`）：演示网络已经由演示租户经参考配置采用，`submit-one-shipment.sh` 的草案也带上了寄 / 收两端国家码。按本脚本重走（实测，只记 `S`），这堵墙在可达性那一段降了：可达性判断形成、答`可达`，受理链往下走到受理前财务控制，停在 `FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED`。那是 [psb/05](../../.scratch/product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 的格 5，不是网络的事。委托因此还成不了`已接受`，初始路由在动线上没被触发。初始路由本身的现状（证据视图已配置，成本那一格的计价输入还没接）与两次取证的命令、SHA 都记在 `routing-first-cut/11` 票面，这里不复述。
> 2026-10-12 补记（票 `routing-first-cut/19`）：上一条原样保留。停点自票 `routing-first-cut/16` 进 main 起前移：演示种子的财务控制时点也采用了「提交接收」，演示租户的结算账户也登了，受理链越过时点与作用域，停在财务控制那一步的 `FINANCIAL_CONTROL_NOT_FORMED`（金额未配置），不再是 `FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED`。这一停按会自愈的依赖处置：「委托已提交」信封整笔回滚重投，烧完预算落 `ABANDONED`，任务上的判断与处理尝试都不落，委托停在`已提交`。钉 `ff3b01fa` 重走一遍（实测，只记 `S`），命令与逐表计数记在 `routing-first-cut/19` 票面「分诊」节，这里不复述。
> 2026-10-14 补记（票 `product-strategy-boundary/26`）：上一条原样保留。售价卡绑上分区目录、演示委托带上两端邮编、结算账户登了「逐件 SELL 试算合计」采用行之后，按本脚本在一次性库 `psb26_mcp4` 上重走（代码 `45247ce0`，种子灌入退 0；API `:19084`，读写真开关同取 `SYN-TENANT-01`，dispatch 七个变量照「前置」；实测，只记 `S`）。格 5 没越过。可达性判断形成、答`可达`（`SYN-LINE-CN-SG-01@1` 为 `QUALIFIED`）。受理链停在受理前财务控制：处理尝试落 `FINANCIAL_CONTROL_NOT_CONFIGURED` / `等待运营登记`，委托仍是`已提交`，「委托已提交」信封入账。同一笔用生产装配再问一次控制（事务回滚，探针未入库），原因是 `SA-CONTROL_POLICY_NOT_CONFIGURED`。闭包采用集里没有 `SYN-FIN-CONTROL-01`；演示解析键的 `requiredBases` 不含 `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY`。冻结、估价留痕、运营余额都是 0 行——没读到余额，停点不是运营余额没有写入方那一格。委托 `SHR-YUYNANNZLNPGUH76IVC6ZFV3SE`，来源键 `SYN-CUSTREF-20261014T083249Z`。走完已停进程并删库。
> 2026-10-14 再补（同一票，解析键已列 `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY`，代码 `21b102fd`）：一次性库 `psb26_mcp1`，种子 `--reset` 退 0；API `:19086`，读写真开关同取 `SYN-TENANT-01`，dispatch 七个变量照「前置」；实测，只记 `S`。格 5 仍没越过。委托 `SHR-ADEUHGQ2ADR5C7FDKSZCPBKURI` 建成`已提交`（来源键 `SYN-CUSTREF-20261014T085846Z`）。dispatch 对「委托已提交」信封答 `dispatch.consumer_undecided`：受理链未决，阶段 `FINANCIAL_CONTROL_JUDGMENT`，原因 `FINANCIAL_CONTROL_NOT_FORMED`。不再是策略未配置。处理尝试随未决回滚，表上 0 行；冻结、估价留痕、运营余额仍是 0 行。进程日志没有把结算侧的原因串带出来。走完已停进程并删库。
> 2026-10-14 再补（同一票，通道 4，派单 `task-b02a598a`；解析键仍是 `21b102fd` 那一版，行为代码未改）：一次性库 `psb26_mcp4b`，种子灌入退 0；API `:19088`（19080 仍是遗留探针，未碰），读写真开关同取 `SYN-TENANT-01`，dispatch 七个变量照「前置」；实测，只记 `S`。格 5 仍没越过。委托 `SHR-TWC4EII6DIC3ZMFE37N32LELWA` 建成`已提交`（来源键 `SYN-CUSTREF-20261014T094857Z`，提交版本 `SUBV-Q2PSBXS33JFF623QE44ZJNJAEU`）。dispatch 对「委托已提交」信封答 `dispatch.consumer_undecided`：受理链未决，阶段 `FINANCIAL_CONTROL_JUDGMENT`，原因 `FINANCIAL_CONTROL_NOT_FORMED`。处理尝试随未决回滚，表上 0 行；冻结、估价留痕、运营余额仍是 0 行。同一笔用生产装配再问一次控制（事务回滚，探针未入库），原因是 `SA-ESTIMATE_DECLARATION_INCOMPLETE`。快照上有两端邮编、没有声明画像。提交草案已经带了这一件的 1.50 KG 与 20×15×10 CM；隔离接单入口把它们编进摘要的服务条目，没有放上声明画像。再改种子里的数值推不动这一格。执行器未改。走完已停本轮进程并删库。遗留库 `psb26_walk` 与其上的探针进程未碰。
> 2026-10-14 再补（同一票，通道 4，派单 `task-018784d1`；隔离接单把草案测量编进声明画像，代码 `15a7e77b`）：一次性库 `psb26_mcp4c`，种子灌入退 0；API `:19088`（19080 仍是遗留探针，未碰），读写真开关同取 `SYN-TENANT-01`，dispatch 七个变量照「前置」；实测，只记 `S`。格 5 仍没越过。委托 `SHR-6FJEOCCZ3AHYS2NYYNJXTPAA5A` 建成`已提交`（来源键 `SYN-CUSTREF-20261014T100701Z`，提交版本 `SUBV-UFZP627PMHBGYINODZ3ZXLQFHQ`）。快照上这一件有声明画像：1.50 KG，20×15×10 CM，成员与现签包裹标识相同，两端邮编仍在。dispatch 对「委托已提交」信封答 `dispatch.consumer_undecided`：受理链未决，阶段 `FINANCIAL_CONTROL_JUDGMENT`，原因 `FINANCIAL_CONTROL_NOT_FORMED`。处理尝试随未决回滚，表上 0 行。同一笔用生产装配再问一次控制（事务回滚，探针未入库），原因是 `SA-BALANCE_UNAVAILABLE`。冻结、估价留痕、运营余额都是 0 行。没有往 `operational_balance` 或 `credit_standing` 写行，也没有给解析键加 `CREDIT_POLICY`。走完已停本轮进程并删库。遗留库 `psb26_walk` 与其上的探针进程未碰。
> 2026-10-14 再补（同一票，通道 2，派单 `task-0e5c89bc`；[ADR-0182](../adr/0182-receivable-account-is-a-funds-mapping-target.md)）：一次性库 `psb26_mcp2`，种子灌入退 0；API `:19092`，读写真开关同取 `SYN-TENANT-01`，dispatch 七个变量照「前置」；实测，只记 `S`。委托 `SHR-HKFJE6I5RMGT423QGPW5235Z2A` 建成`已提交`（来源键 `SYN-CUSTREF-20261014T103803Z`）。格 5 的财务控制不再是停点：dispatch 答 `dispatch.consumer_undecided`，阶段 `ACCEPTANCE_DECISION`，原因 `ACCEPTANCE_JUDGMENT_INCOMPLETE`，没有 `SA-` 串。受理链后一步未决，整笔回滚，冻结、估价留痕、信用暴露、处理尝试都是 0 行。下一停点是接受判断未完成，不是运营余额或信用状况。走完已停本轮进程并删库。遗留库 `psb26_walk` 未碰。
> 2026-10-14 再补（同一票，通道 2，派单 `task-2e6a7f57`；规则包适用组拿掉没有生产者的 `REQUIRED_DOCUMENT`）：一次性库 `psb26_mcp2b`，种子灌入退 0；API `:19093`，读写真开关同取 `SYN-TENANT-01`，dispatch 七个变量照「前置」；实测，只记 `S`。委托 `SHR-LP2T5FOTHYPN37HRASSBSHVPZ4` 成`已接受`（来源键 `SYN-CUSTREF-20261014T104918Z`，提交版本 `SUBV-6DSHGAWF54KLTO5INHRQMMJZSM`）。格 5 越过：冻结 `FRZ-0001` 金额 9600，估价留痕合计 9600，信用暴露金额 9600。下一停点是初始路由：`acceptance-decision.formed` 消费答未决，包裹 `DPCL-R2CWSPM5MAUZT7MMLM4GCDUAJM` 停在 `CANDIDATE_COSTS_PENDING`，信封烧完预算落 `ABANDONED`。走完已停本轮进程并删库。遗留库 `psb26_walk` 与 `psb26_mcp4d` 未碰。
> 2026-10-18 补记（票 `routing-first-cut/18`，[ADR-0189](../adr/0189-leg-pricing-zones-are-supplied-per-card-and-unbound-cards-take-a-registered-leg-zone.md)）：种子给演示线路各段登了段计价区域 `Z1` 之后，按本脚本在一次性库 `c8_walk18` 上重走（种子灌入退 0，`leg-pricing-zone: REGISTERED`；API 与 dispatch 照「前置」；实测，只记 `S`）。委托 `SHR-7IPK7KPV75KVZTE7ZSIQMKHHOM` 成`已接受`，`acceptance-decision.formed` 一拍 `PUBLISHED`。初始路由越过 `CANDIDATE_COSTS_PENDING`：`network_routing.initial_route` 一行 `ROUTE_FORMED`，计划 `RPV-000000000001` 三段带时间窗口，交接记录一行。本轮没往下走。往下已知的停点是有效网络收寄的采用：2026-10-16 在 `c68e4342` 上经受控 CLI 灌一行收寄取证，种子规则包声明了硬资格 `SYN-QUAL-REALNAME-01`，装配点的资格证据口恒答未证明，采用答 `ELIGIBILITY_NOT_ESTABLISHED`。走完已停进程并删库。
> 2026-10-18 再补（票 `product-strategy-boundary/30`、`/32`）：上一条记的收寄资格停点不再是停点。演示规则包不再声明那条没有证明方的硬资格，种子登了作业设备与作业事实授予。经收寄口收寄一件已接受委托的包裹（标识用客户包裹号），PS 采用落行。下一停点是路由复核：采用信封的消费答未决 `PLAN_REVIEW_INCONCLUSIVE`，因为复核取证据时没带两端国家，关务适用性答未知（票 `routing-first-cut/21`）。一次性库 `c8_walk3`，实测只记 `S`，走完已删库。
> 2026-10-18 三补（钉 `851e9da7`，一次性库 `c8_walk7`，实测只记 `S`，走完已删库）：带上 `routing-first-cut/21`、`/22` 与 `product-strategy-boundary/34` 之后，实物主链从提交一路走到包裹终局。路由复核答 `STILL_APPLICABLE`。随后用新加坡尾程场外设备依次打四口：末端交接（立派送段）、派送发起、派送尝试、有效交付。PS 采用为终局 `EFFECTIVE_DELIVERY`。往下的金额链、运输机会与委托、关务旁路的停点与去处记在 [product-strategy-boundary/05](../../.scratch/product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 同日的重走一节。
> 2026-10-19 补记（票 `demo-chain-gaps/32`，[ADR-0205](../adr/0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md)）：自营段的出发有了门禁读数。种子登了四个节点的关务区域（`customs/27`–`30`）与上海→深圳的周期安排之后，显式登一个 `SYN-SCHED-SHA-SZX-DAILY/<日期>` 班次，用交接口带 `segment` 立 SHA→SZX 自营段，再经作业事实渠道（`OFFSITE` 作业范围）`POST /transport-fulfillment/movement-facts`，载荷带同一个 `segment` 报出发：答 `201` `MOVEMENT_FACT_RECORDED`，响应里 `gateReading.answer` 为 `NOT_GUARDED`——两端都在 CN，包裹不在任何关务案卷里，关务只适用装载出发并答不受门禁。出发不再因门禁拒收：读数阻断、未决或读不动都照样 `201`；载荷不带 `segment` 时读数停在 `SEGMENT_NOT_DECLARED` 答未决。取证是生产装配上按种子原样登记的真库用例（`cmd/parcel-api` 的 `TestADemoSelfOperatedDepartureFromShanghaiToShenzhenAnswersCreatedNotGuarded`，只记 `S`），本票没有起进程按本脚本重走。
> 2026-10-19 补记（票 `demo-chain-gaps/33`，[ADR-0205](../adr/0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md) 决定五与文末补记）：上一条那次出发登记成立的同一笔里入队 `transport-fulfillment.movement-fact.recorded`；派发进程把它交给全程追踪，按出发声明的段取出发那一刻在控的那几件，各派生一条 `movement-departure`。演示映射 `SYN-VE-MAP-V1` 多了出发一行，于是 `/tracking-projections` 上那件包裹出现 `DEPARTED`（已灌过的库要 `--reset` 才拿得到这一行）。载荷不带 `segment` 的出发照记，但全程追踪无对象可投。读数里关务判阻断的那一件按演示租户采用的出发门禁阻断信号规则（`visibility/13`）形成 `DEPARTURE_GATE_BLOCKED` 信号、进人工复核；SHA→SZX 这一步读数不受门禁，不会命中。取证是生产登记编排与生产路由表上的真库用例（`cmd/parcel-dispatch` 的 `TestADepartureClassifiesAsDepartedUnderTheDemoMappingThroughTheProductionRoute` 与 `TestABlockedDepartureRaisesTheAdoptedSignalThroughTheProductionRoute`，只记 `S`），本票没有起进程按本脚本重走。

> 2026-10-19 补记（票 `demo-chain-gaps/18`，[ADR-0201](../adr/0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md) 决定六至八与票 18 补记）：提交一单之后，初始路由信封除了投追踪投影，也投运输履约逐段准备。`GET /transport-fulfillment-records?registry=transport-preparation` （操作者渠道，查阅员即可）列出这一件的三段：每段带结论、段窗口、按申报取的需求量（`1.50 KG`、外廓乘出的 `3000 CM3`、`1 PCS`）与候选依据。首段 `SHA-HUB→SZX-GATE` 答 `OPPORTUNITY_PREPARED`，选中的班次是评估时按日期物化的那一班（候选 `version` 为 0，要订舱或装载分配先显式登记它）；种子另登了上海→深圳快线（班期见 `scripts/demo-seeds/README.md` 组成表的 `data/transport/` 几行），首段窗口不论提交落在哪一刻都含一班。后两段与按日那两份的班期大多错开，答 `NO_CURRENT_OPPORTUNITY`，那是如实答案。准备不预占、不建班次、不发信封；初始路由信封在 VE 与 TF 两个 inbox 名下各入账一行。2026-10-19（14:27Z 提交，上海当地 22:27）在一次性库上按 `seed.sh` 灌入退 0，起 `parcel-api` 与 `parcel-dispatch`，经首方客户渠道提交一单，操作者渠道读回三段：`RPV-000000000001#1` 准备在 `SYN-SCHED-SHA-SZX-EXPRESS-1200/2026-10-20`，`#2`、`#3` 无当前运输机会（实测，只记 `S`），走完已停进程并删库。

三堵墙对应的机制票都在 `.scratch/syn-wall-door-audit/issues/`（依次为 01、13、04），墙面清单见同目录 `report.md`。

## 对照组：证明门是真的

演示里最容易被质疑的一句是「你们这个 403 是写死的吧」。下面几行对照可以当场答掉，每行只动一处：

| 环境变量 | 结果 |
|---|---|
| 不设发行方三件 | 运营查阅与运营决定各口答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`，包括第 1–4 步走过的那些 |
| 设发行方三件、请求不带令牌 | 运营查阅各口答 `401 OPERATOR_CREDENTIAL_REJECTED`；带配置员令牌才答 `200` |
| `IDP_PARCEL_ISOLATED_READ_TENANT=SYN-TENANT-01` | 只有委托查阅及其子集视图不带令牌也答 `200`，其余照上两行——**提交口仍 `403`** |
| 再加 `IDP_PARCEL_ISOLATED_WRITE_TENANT=SYN-TENANT-01` | 提交口仍答首方族的 `403`（写开关不放行任何命令面）；再加首方族三件、带合成货主系统令牌才走到编排。挂字面量未配置的命令面仍 `403`，挂操作者渠道的各口照上面两行 |
| 写开关开着，经首方口提交的客户端绑的不是开关那个租户 | 走到编排，归属答`权威未确定`，停在 `OWNERSHIP_UNRESOLVED`——合成坐标只给开关租户（ADR-0091 决定六） |
| 任一开关取 `TENANT-PROD-1`（无 `SYN-` 前缀） | **进程启动即拒**，带原因退出，不静默回落 |
| 两开关取不同的 `SYN-` 租户 | **进程启动即拒**，报文同时点名两个开关 |

读开关与写开关那两行挨着看是这套门禁分级的证据：读开关开到底也开不了写行，那是 [ADR-0091](../adr/0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 把两个开关分设的全部理由——合一的话，今天所有设了读开关的环境会在升级那一刻静默获得写准入。

后两条是重点：静默回落会让「配置错了」与「刻意拦着」两态的可观察签名变成同一个，那正是 ADR-0078 要避免的「默认值不出声」病。演示时把它们留到最后，比前几条更能说明这套门禁不是摆设。

## 复灌

```bash
IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' \
  ./scripts/demo-seeds/seed.sh --reset
```

`--reset` 是破坏性动作（DROP 全部 parcel schema → 重迁 → 重灌），**只属于隔离演示库**。对已灌过的库不要用不带 `--reset` 的重跑：四条 CLI 各自幂等，但网络目录的版本行撞主键会答未决。

## 取证

**2026-10-18 起（票 operator-channel/35）**：运营查阅各口挂操作者查阅读。下表的 `200`，委托查阅以外各行今天只在带配置员令牌时成立；只设隔离读开关、不带令牌时，除委托查阅及其子集视图外全答 `403`（目录四族那一半 2026-10-16 在 `c68e4342` 上重走已实测）。下表锚在 `627d339`，保留为当时的取证。

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
