# 合成 S 主数据种子包（仅限隔离环境）

票 `.scratch/master-data-wiring/issues/08-synthetic-seed-pack.md` 的产物：一套 `SYN-` 前缀的
合成主数据种子，经各上下文的登记 CLI（逐个见下面「组成」表的「入库通道」一列）灌入本机演示库，让管理台主数据区的页面有数据可看。

**红线**：全部实例值是合成内容，证据层级只记 **S**——不影射任何真实企业、不进参数登记册、
不写生产默认值。本目录下任何脚本都**不得指向生产库**；`--reset` 与 `-reset` 是破坏性动作，
只属于隔离演示库。

## 用法

```bash
IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' \
  ./scripts/demo-seeds/seed.sh          # 首灌（库上没有 parcel schema 时自动迁移）
  ./scripts/demo-seeds/seed.sh --reset  # 复灌：DROP 全部 parcel schema → 重迁 → 重灌
```

干净库上全程零报错。计价登记对重复输入幂等（已登记/重放答 0）。网络目录的版本行撞
主键会答未决（3），自动改路事实同键再登答治理格（2）——对已灌过的库重跑请用 `--reset`。
责任法人 `SYN-LE-01` 的修订 1
自票 legal-entity-profile/02 起带身份层（注册国家 `CN` 与合成终身注册号，ADR-0145 决定一）；在那之前
灌过的库上这一笔是没有身份层的旧形状，重跑会答内容冲突（2），同样用 `--reset`。
里程碑映射 `SYN-VE-MAP-V1` 自票 demo-chain-gaps/11 起关务那一行只登全部放行（`external-result-release-full` → `CUSTOMS_CLEARED`，类型词表见 ADR-0197），自票 demo-chain-gaps/13 起交接那一行只登已交出（`handover-handed-over` → `IN_TRANSIT`），自票 demo-chain-gaps/33 起多一行出发（`movement-departure` → `DEPARTED`，类型词表见 ADR-0205 文末补记；移动与到达不映）；在那之前灌过的库上版本号已占，重跑答 `REFUSED VERSION_NOT_OVERWRITABLE`、退出码 2，`seed.sh` 随之中止，库里仍是旧行；同样用 `--reset`。

## 操作者渠道：演示发行方与起 parcel-api

票 `operator-channel/17`。管理台的登录门与操作者族各口（端点表里挂 `operatorRegistries.*` 的登记写面与运营决定那几口）
认的是演示发行方 Dex 签的令牌，`parcel-api` 按 Dex 公布的公钥集自己核签名、`iss`、`aud` 与有效期（ADR-0100 决定二）。
四步，都在仓库根：

1. 起发行方：`docker compose -f scripts/demo-seeds/operator-channel/compose.yaml up -d --wait`。发现文档在
   `http://127.0.0.1:5556/dex/.well-known/openid-configuration`，公钥集在 `http://127.0.0.1:5556/dex/keys`。
2. 灌种子：见上面「用法」。操作者册登的就是 Dex 两个静态用户的主体。
3. 起 `parcel-api`，带操作者族的三件部署参数：

   ```bash
   IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' \
   IDP_PARCEL_HTTP_ADDR=:19080 \
   IDP_PARCEL_OPERATOR_OIDC_ISSUER=http://127.0.0.1:5556/dex \
   IDP_PARCEL_OPERATOR_OIDC_JWKS_URL=http://127.0.0.1:5556/dex/keys \
   IDP_PARCEL_OPERATOR_OIDC_AUDIENCE=idp-parcel-admin-web \
   IDP_PARCEL_GOVERNANCE_SELF_AUTHORITY=SYN-AUTH/idp-parcel-pilot \
     go run ./cmd/parcel-api
   ```

   最后一件是本产品在治理登记册里的权威串（ADR-0180 决定三），运营决定与外部事实各口铸信封前拿它认区间：不设，
   这几口授予齐备也答 403 `OUTSIDE_ADMISSION_SCOPE`；设了，种子里演示租户的准入范围对照（`data/access/03`）与
   `data/governance/` 里能力为 `OPERATION_DECISION`、`EXTERNAL_FACT`、`CUSTOMER_COMMAND` 的权威区间才认得上。取值与隔离形态的权威串同串，
   隔离写开关开着而两值不等进程启动即拒。隔离写开关不放行任何命令面（提交口已换首方客户渠道，票 first-party-api/00），
   运营决定各口都只能经这一件认区间。管理台上提交运营决定的页面，如「接受前人工复核」（`AcceptanceReviewPage`：复核完成、
   主动拒绝）、「授权处置」（`AuthorizedDispositionPage`）、「外部轨迹有效时间判断」（`EffectiveTimeJudgmentPage`），不设
   这一件时提交答的就是 `OUTSIDE_ADMISSION_SCOPE`；管理台上没有页面的运营决定口只能直接调 API。

   三件的取值只属这个演示部署，产品侧不给缺省值：三件都不设，操作者族整族答 403 `ACCESS_CHANNEL_NOT_CONFIGURED`；
   只设一部分，进程启动即拒。发行方地址与 Dex 发现文档里的 `issuer` 逐字相同——`parcel-api` 逐字比较 `iss`，多一个
   末尾斜杠就一律答 401。受众是管理台的客户端标识，Dex 把它写进访问令牌的 `aud`。`:19080` 是管理台开发代理的缺省
   目标（`apps/admin-web/vite.config.ts`）。「已知边界」里 `submit-one-shipment.sh` 那条要的隔离读写两件与这三件
   互不替代，可以一起设。
4. 取证：`IDP_PARCEL_API_BASE=http://127.0.0.1:19080 scripts/demo-seeds/operator-channel/probe-operator-channel.sh`。
   对同一个登记写口打三枪、逐格断言：配置员答 200 `REFUSED` / `VERSION_NOT_OVERWRITABLE`（种子在 `SYN-TENANT-01`
   下登过同一条策略，答得出这一格说明令牌解析出的正是这个租户），查阅员答 403 `OPERATOR_NOT_GRANTED`，不带令牌答
   401 `OPERATOR_CREDENTIAL_REJECTED`。再打一个运营决定口（终止参与，票 operator-channel/19）：配置员越过准入范围
   到编排，答 200 `SEGMENT_NOT_FOUND`（段号取不存在的那一个），查阅员没有这种决定的授予答 403 `OPERATOR_NOT_GRANTED`。
   不往库里多落行，可以反复跑；证据层级只记 S。

两个合成操作者，口令都是合成值 `syn-demo-password`——只配回环上的演示发行方，不是任何环境的密钥：

| 登录邮箱 | 授予（自 2026-01-01 生效，不设终点） |
|---|---|
| `syn-operator-config-01@syn-tenant-01.example.invalid` | 登记册配置写、主数据与运营查阅读、运营决定（装载分配、终止参与、关段、建派送任务、有效时间判断、承运商首次有效收寄判断；复核完成、主动拒绝、授权处置、受控关闭、受控重开——委托侧这几种越过 Intake 的前提见表后说明；待识别重核——节点作业复核页上的「重核」；身份冲突处置——同一页上的「处置」；运输委托提交；订舱提交；履约段采用运输委托、取消运输委托、取消订舱）；作业事实登记，按作业范围给：演示网络各节点（收寄），华东场外揽收与新加坡尾程派送两个场外范围（运输履约各口），每个范围配一台合成设备（`data/access/04`） |
| `syn-operator-viewer-01@syn-tenant-01.example.invalid` | 主数据与运营查阅读 |

委托侧各种运营决定的授予只开门。权威串设了之后，越过 Intake 还要指名的对象在演示租户下查得到：复核完成、主动拒绝、
授权处置按委托标识查，受控关闭与重开按包裹查它当前`已接受`的委托（ADR-0183 决定一）；查不到与越权探针同答 403
`OPERATOR_NOT_GRANTED`（ADR-0151 决定二「委托寻址」）。本包只灌主数据、不造委托，所以只灌种子时打这几口看到的仍是
`OPERATOR_NOT_GRANTED`；`submit-one-shipment.sh` 落下一笔`已提交`委托之后，按委托标识寻址的口查得到它，受控关闭与
重开要等那一笔成了`已接受`才查得到。查得到之后各口停在各自的授权缺口上（ADR-0151 决定七、ADR-0183 决定六）。

配置员另持运营决定「运输收费发生项登记」（ADR-0207；`data/access/02` 的授予、`03` 的对照与 `data/governance/25` 的
权威区间各一行）。种子不登任何发生项：发生项是对已发生履约事实的一次采购判断，要等干线段的参与结束之后经
`POST /transport-fulfillment-charge-occurrence-registrations` 登记，登法见下面「数据故事」第 8 步的补记。

配置员另持运营决定「容量预占」与「容量释放」两种（ADR-0203；`data/access/02` 的授予、`03` 的对照与
`data/governance/28`、`29` 的权威区间各两行）。种子不登任何预占：预占是为一个业务范围占住一个池的一段容量，由配置员
经 `POST /transport-fulfillment-capacity-reservations` 声明，释放经 `POST /transport-fulfillment-capacity-releases`，
登法见下面「数据故事」第 8 步的补记。

配置员另持运营决定「订舱提交」（ADR-0202 决定一、五；`data/access/02` 的授予、`03` 的对照与 `data/governance/30` 那份运营决定
区间各一行），承运答复口另认合成干线承运商的集成客户端（`data/access/05`、`06` 各一行，`03` 的对照与 `data/governance/31` 那份
外部事实区间各一行）。种子不登任何订舱或答复：订舱要先有干线委托，订在 `data/transport/04` 物化登记的
`SYN-SCHED-SZX-SIN-DAILY/2026-10-21` 那一班上，再由合成干线承运商用自己的令牌答复，走法见
`docs/design/synthetic-demo-journey-script.md` 的第 3 步。

配置员另持运营决定「履约段采用运输委托」「取消运输委托」「取消订舱」三种（ADR-0204；`data/access/02` 的授予、`03` 的对照与
`data/governance/32`、`33`、`34` 的权威区间各三行）。种子不登任何采用或取消：采用要先有干线委托与干线段——段由 SZX 那一次交给
干线承运方的交接立起来，再经 `POST /transport-fulfillment-segment-commission-adoptions` 声明（段、委托、服务方）；取消经
`POST /transport-fulfillment-transport-commission-cancellations` 与 `POST /transport-fulfillment-booking-request-cancellations`。
走法同样见演示文档第 3 步。

单要一枚令牌：`scripts/demo-seeds/operator-channel/operator-token.sh <邮箱>` 打印访问令牌，末尾加 `claims` 打印两枚
令牌的载荷。它与管理台走同一个公共客户端、同一条 authorization_code + PKCE 流，管理台不必在跑。

- Dex 的状态只在内存：容器重启后，此前签出的令牌与刷新令牌全部作废，签名钥也换新。`parcel-api` 见到陌生 `kid`
  才重取公钥集，且两次取至少隔一分钟，所以 Dex 刚重启时新令牌可能先答 401，过一分钟再试即好。
- Dex 的版本钉在 `compose.yaml`，不用更新那一版的理由写在镜像那行的注释里。这一版不强制 PKCE：请求不带
  `code_challenge` 也放行，带了才核 verifier；管理台每次都带 S256。
- Dex 启动日志开头那一行 `failed to get signing algorithm … no signing key found` 是签名钥还没生成的那一瞬，紧跟着
  就是 `keys rotated`，不是故障。

## 组成

| 目录 | 入库通道 | 内容 |
|---|---|---|
| `data/commercial/` | `cmd/parcel-commercial publish` | 发布批：两份服务产品（各带交付条件）、接受前财务控制策略正文、接单规则包（五类规则正文+受理内容+收寄资格+时点锚+终局规则+资料修订允许）、客户合同（正文+受理前控制+合同委派+收紧交付条件）、结算政策（预付，六维范围对齐解析键）、两份供应商协议正文、价格规则（价格政策正文+口径）、信用政策、客户服务规则、三份授权规则（方向两份仍是壳，取消授权带正文） |
| `data/commercial/resolution-key-*.json` | `cmd/parcel-commercial register-resolution-key` | 消费方（parcel-shipment）的解析键登记 2 行（`SYN-ACCOUNT-01`、`SYN-ACCOUNT-02` 各一）：七项必需依据（含客户服务规则、接受前财务控制策略与信用政策）加结算三维——它不是商业权威发布，只是与发布共用一个 CLI。不把 `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY` 列进键，闭包就不采用合同上绑的 `SYN-FIN-CONTROL-01`，受理前财务控制答策略未配置。不把 `CREDIT_POLICY` 列进键，闭包就不采用 `SYN-CREDIT-01`，信用检查答依据未配置。列了就必须带商业权限等级与费用类型两维，与 `SYN-CREDIT-01` 正文同值 |
| `data/commercial/register-products.json` | `cmd/parcel-commercial register-products` | 服务形态两笔（EXPRESS/ECON 均网络服务）+ 产品—渠道映射两笔：EXPRESS 配两个渠道标识引用，ECON 显式登记「未配置」（该产品尚无可用渠道候选）——渠道本体不预造（ADR-0072） |
| `data/commercial/register-registration-number-types.json` | `cmd/parcel-commercial register-registration-number-types` | 注册号类型目录（ADR-0145 决定一）：身份层由演示租户显式采用 CN（统一社会信用代码）、SG（UEN）两份参考配置（ADR-0147，批文 `adopt` 格，依据格随之写成 `REFCFG-1:…@1`），资料层 CN、SG 各登一类 `SYN-` 合成税务登记号，另有一类 SG 资料层旧类型登记后停用，让目录状态的「已停用」有实例可显。产品本身不带任何国家 / 地区的目录条目：参考配置不采用就不进任何租户的目录 |
| `data/commercial/register-parties.json` | `cmd/parcel-commercial register-parties` | 参与方身份：业务参与方（含合成干线承运方 `SYN-PARTY-CARRIER-TRUNK-01`、合成末端承运方 `SYN-PARTY-CARRIER-LM-01`，供两份供应商协议引用）、关系、责任法人、客户账户 |
| `data/commercial/register-authorized-delivery.json` | `cmd/parcel-commercial register-parties` | 授权交付关系（`PAR-NET-17`，ADR-0212 决定一）：合成交付方 `SYN-PARTY-DELIVERER-01` 一个，加一条它替 `SYN-ACCOUNT-01` 送站的授权交付方关系，范围写 `customerAccount=SYN-ACCOUNT-01`、不收到节点，带批准即生效。相对方是该账户所属的 `SYN-PARTY-SHIPPER-01`，所以排在参与方身份之后。合成取值，证据记 `S` |
| `data/commercial/register-legal-entity-profiles.json` | `cmd/parcel-commercial register-legal-entity-profiles` | 法人资料（ADR-0145 决定三）：`SYN-LE-01` 两笔修订——修订 1 不带开票资料（按它生效的时段解析答「资料不全」），修订 2 自 3 月起补上开票抬头；注册地址在 `CN`，与法人身份层的注册国家一致，税号取目录里 `CN` 的资料层合成类型。排在参与方身份之后：资料引用已登记的责任法人 |
| `data/pricing/` | `cmd/parcel-pricing-register` | 两张价卡（SELL 首重续重 / BUY 重量段）+ 两条参考序列（燃油、汇率）+ 两份序列复核 + 一份分区参考目录与其复核（不复核不在用，ADR-0099 / ADR-0109）；售价卡绑燃油序列与这本分区目录；两张卡都声明金额取整（合计点 0.01 CNY 四舍五入），成本卡这一格是就地改 v1，不另发 v2（两版同时在用，在用解析答适用冲突）——由 `seedgen` 生成，勿手改 |
| `data/network/` | `cmd/parcel-network-register` | 稳定定义由演示租户显式采用参考配置 `network-routing/network-catalog/SYN-CN-SG@1`（ADR-0147，登记行 `adopt` 格，依据格随之写成 `REFCFG-1:…@1`）：4 节点、3 连接、1 线路（三段成本依据引 BUY 卡）、2 服务区（CN 收寄 / SG 交付覆盖）、7 份日历（4 节点的处理时长与截单、3 连接的衔接缓冲）、1 路由策略（成本单维）；上海枢纽修订 2 是采用后的租户修订，依据换成租户自己的。路由策略修订 2 同样是租户修订，补上参考配置装不下的冻结形态：剩余段数，剩余不超过一段即冻结（`23-route-strategy-cn-sg-v2.json`）。另直接登记 1 条台风停运调整与 1 行自动改路事实（判断键对齐 `SYN-ACCOUNT-01` 与 `SYN-RS-CN-SG-01`，不是一笔已受理委托的运行时产物），以及演示线路各段的段计价区域（`22-leg-pricing-zones-cn-sg-v1.json`，ADR-0189；成本卡没绑分区目录，每段登 `Z1`，是租户取值）。产品本身不带任何网络：参考配置不采用就不进任何租户的目录 |
| `data/customs/` | `cmd/parcel-customs-register` | 十一册 30 份：就绪与授权（第三单元先就绪再撤销就绪；第二单元只撤授权）、解释规则（含一次换版）、义务目录+两项（已了结/已承接）、门禁目录+判断（含一份只登目录的空清单格）、建案要求两向（要求/显式不要求）、口岸目录（SZX 含一次换版 + SIN）、申报路径两向（CN 出口 / SG 进口）、监管凭证两版（一版写明次数额度、一版来源未提供额度）、税费付款协作两格（核定税费 / 明确无需付款）、节点关务区域四份（上海枢纽、深圳口岸在 `SYN-ZONE-CN`，新加坡枢纽与末端在 `SYN-ZONE-SG`；ADR-0206，租户取值）。税费付款核对不在包内，见已知边界 |
| `data/visibility/` | `cmd/parcel-ve-register` | 里程碑映射、分诊规则、通知策略、索赔资格、索赔授权两格（一格空名单）、披露策略、异常披露规则、冲突信号规则、索赔材料收讫两笔（照片留下、发票收讫后撤销）、出发门禁阻断信号规则（票 demo-chain-gaps/33） |
| `data/collection/` | `cmd/parcel-collection-register` | 一条 COD 指令走全程（渠道报收、银行短收、短款、清分进应付客户）+ SGD 分户账只开立；CNY 账两笔回汇批次（已归集 / 已交出汇付主张），SGD 账无批次 |
| `data/settlement/` | `cmd/parcel-settlement-register` 的 `settlement-account` / `external-funds-fact` / `funds-mapping` / `pre-acceptance-estimate-adoption` / `sell-billing-moment-adoption` / `fee-item-mapping` / `buy-evaluation-trigger` | 结算账户登记册（ADR-0158）1 行：货主 `SYN-PARTY-SHIPPER-01` 在法人 `SYN-LE-01` 下的 CNY 应收账户 `SYN-SETTLEMENT-ACCOUNT-01`，结算政策 `SYN-SETTLEMENT-PREPAID-01`，责任依据是该政策所引的客户合同 `SYN-CONTRACT-01`。五维照发布批里那份结算政策的正文配，受理前财务控制的作用域经它找到账户。另有一笔合成收款 `SYN-FUNDS-RECEIPT-01`（200.00 CNY）映射到该账户（ADR-0182），金额只盖这一件的试算合计，不是生产默认。受理前估价采用册（ADR-0179 决定五）1 行：这一户采用「逐件 SELL 试算合计」（`PER_PARCEL_SELL_TRIAL_SUM`），计价范围 `SYN-SCOPE-01`，与售价卡 `SYN-PLAN-CN-SG-01` 的范围同值，估价按它解析在用售价卡。SELL 计费时点采用册（ADR-0190 决定三）1 行：这一户在「有效网络收寄已采用」那一拍发起 SELL 评价，计价范围同上。费用项目映射（决定五）2 行：基础运费与偏远附加都映到 `SYN-FEE-ITEM-FREIGHT-01`，是 SYN 简化。售价卡声明了金额取整（总额，0.01 CNY，四舍五入），结算由评价形成客户费用时要这一格。BUY 触发册（ADR-0164、ADR-0208）1 行：原因 `ACTUAL_FULFILLMENT` 采用「发生项形成」，费用项目 `SYN-FEE-ITEM-TRUNK-COST-01`；干线实际履约发生项一形成就发起 BUY 评价，主要范围取发生项上干线协议 `SYN-SUPPLIER-TRUNK-CN-SG-01/v1` 的商业范围 `SYN-SCOPE-01`。别的原因不登，形成了答未配置 |
| `data/access/` | `cmd/parcel-access-register` 的 `operator-register` / `operator-grant` / `admission-scope-mapping-register` / `device-register` / `integration-client-register` / `integration-client-grant` / `customer-client-register` / `customer-client-grant` | 操作者册（ADR-0100 决定二第三条）：合成操作者主体绑演示租户，各自持有哪些授予以上面「操作者渠道」一节的授予表为准，这里不另列；主体是演示发行方 Dex 给 `operator-channel/dex.yaml` 里静态用户真签出的 `sub`，换发行方就按新发行方的标识另登一批。另有演示租户的准入范围对照（ADR-0180 决定一、二）：各（能力、事实类型）的生产写落在哪个治理对象范围，与 `data/governance/` 里本产品的权威区间配上，运营决定与外部事实各口才越得过准入范围。`04-devices.json` 是作业事实各口用的合成设备，每台绑一个作业范围，与配置员的作业事实登记授予一一对上。`05`、`06` 是集成客户端族：合成报关服务商、合成银行与合成干线承运商各一台（干线承运商的来源身份是 `SYN-PARTY-CARRIER-TRUNK-01`，只授承运订舱答复），主体是演示的集成客户端发行方签出的 `sub`，授予按事实类型给。`07`、`08` 是首方客户族：合成货主系统一台绑 `SYN-ACCOUNT-01`，主体是同一只发行方签出的 `sub`，授予按客户面能力给——委托提交，与送站预报的提交、撤回（票 ps-external-mark-relations/09）；预报两口在铸造侧判准入，`03` 里登了它们的对照（能力 `CUSTOMER_COMMAND`，事实类型取能力词，对象范围 `SYN-PILOT-SCOPE/customer-commands@v1`），`data/governance/35`、`36` 是对应的权威区间 |
| `data/transport/` | `cmd/parcel-transport-register schedule-template` | 周期安排登记册（ADR-0201）3 份：演示网络三条连接各一份按日运行的周期安排——`SYN-SCHED-SHA-SZX-DAILY`（`SYN-CONN-SHA-SZX`，当地 22:00 出发、历时 600 分钟，Asia/Shanghai）、`SYN-SCHED-SZX-SIN-DAILY`（`SYN-CONN-SZX-SIN`，10:00、300 分钟，Asia/Shanghai）、`SYN-SCHED-SIN-LM-DAILY`（`SYN-CONN-SIN-LM`，07:00、90 分钟，Asia/Singapore），服务范围都是 `SYN-SCOPE-01`。方向的连接代码与起讫节点取演示网络参考配置，时区取该连接的业务时区；出发时刻与历时是合成分钟数，不是任何线路的真实班期。具体班次不预生成：准备评估时按日期物化，或经 `schedule` 命令 / `POST /transport-fulfillment-transport-schedule-registrations` 按运行日期显式登记 |
| `data/access/` | `cmd/parcel-access-register` 的 `operator-register` / `operator-grant` / `admission-scope-mapping-register` / `device-register` / `integration-client-register` / `integration-client-grant` / `customer-client-register` / `customer-client-grant` | 操作者册（ADR-0100 决定二第三条）：合成操作者主体绑演示租户，各自持有哪些授予以上面「操作者渠道」一节的授予表为准，这里不另列；主体是演示发行方 Dex 给 `operator-channel/dex.yaml` 里静态用户真签出的 `sub`，换发行方就按新发行方的标识另登一批。另有演示租户的准入范围对照（ADR-0180 决定一、二）：各（能力、事实类型）的生产写落在哪个治理对象范围，与 `data/governance/` 里本产品的权威区间配上，运营决定与外部事实各口才越得过准入范围。`04-devices.json` 是作业事实各口用的合成设备，每台绑一个作业范围，与配置员的作业事实登记授予一一对上。`05`、`06` 是集成客户端族：合成报关服务商与合成银行各一台，主体是演示的集成客户端发行方签出的 `sub`，授予按事实类型给。`07`、`08` 是首方客户族：合成货主系统一台绑 `SYN-ACCOUNT-01`，主体是同一只发行方签出的 `sub`，授予按客户面能力给（此刻只有提交） |
| `data/transport/` | `cmd/parcel-transport-register schedule-template` | 周期安排登记册（ADR-0201）：`01`–`03` 是演示网络每条连接各一份按日运行的周期安排——`SYN-SCHED-SHA-SZX-DAILY`（`SYN-CONN-SHA-SZX`，当地 22:00 出发、历时 600 分钟，Asia/Shanghai）、`SYN-SCHED-SZX-SIN-DAILY`（`SYN-CONN-SZX-SIN`，10:00、300 分钟，Asia/Shanghai）、`SYN-SCHED-SIN-LM-DAILY`（`SYN-CONN-SIN-LM`，07:00、90 分钟，Asia/Singapore），服务范围都是 `SYN-SCOPE-01`。方向的连接代码与起讫节点取演示网络参考配置，时区取该连接的业务时区；出发时刻与历时是合成分钟数，不是任何线路的真实班期。具体班次不预生成：准备评估时按日期物化，或经 `schedule` 命令 / `POST /transport-fulfillment-transport-schedule-registrations` 按运行日期显式登记 |
| `data/transport/04-materialized-schedule-szx-sin.json` | `cmd/parcel-transport-register schedule` | 逐维容量与容量池（ADR-0201 决定三，票 demo-chain-gaps/17）：上一行三份周期安排各声明重量（KG）与件数（PCS）两维硬上限——`SHA-SZX` 20000 / 4000、`SZX-SIN` 5000 / 1000、`SIN-LM` 1500 / 300，都是合成值，不是任何车辆或舱位的真实载量。这一份按固定合成日期 2026-10-21 显式物化登记干线一班 `SYN-SCHED-SZX-SIN-DAILY/2026-10-21`，随之建两池 `…/2026-10-21/WEIGHT` 与 `…/2026-10-21/PIECES`，`GET /transport-fulfillment-records?registry=capacity-pool` 列出它们。另一维的池经 `capacity-pool` 命令或 `POST /transport-fulfillment-capacity-pool-registrations` 登在已登记的班次上，池身份由（班次、维度）派生 |
| `data/node-operations/01-pre-advice-window-syn-tenant-01.json` | `cmd/parcel-node-operations-register -kind pre-advice-window` | 预报有效时窗册（ADR-0212 决定二末项，`PAR-NET-18`）1 行：演示租户 `SYN-TENANT-01` 提前 1440 分、宽限 720 分，2026-01-01 起不设终点，登记依据 `SYN-BASIS-PRE-ADVICE-WINDOW-01`。合成取值，不是任何租户的预报惯例；同一行也可经 `POST /node-operations-pre-advice-window-registrations` 登记。没有这一行，凭预报的交付范围核对态答未配置 |
| `data/transport/05-schedule-template-sha-szx-express-1200.json`、`06-…-2000.json` | `cmd/parcel-transport-register schedule-template` | 上海→深圳快线两份（票 demo-chain-gaps/18）：`SYN-SCHED-SHA-SZX-EXPRESS-1200` 与 `-2000`，同走 `SYN-CONN-SHA-SZX`，当地 12:00、20:00 出发、历时 240 分钟，各声明重量 3000 KG、件数 600 PCS（合成值）。计划段窗口随提交时刻移动，首段窗口长十二小时；按日一班 22:00、历时 600 分钟的那一份只接得住一小段提交时刻，加这两班后三班合起来让首段窗口不论落在哪一刻都含一班完整的出发与到达，演示提交因此总有一段 `运输机会已准备`。只调演示租户的班期，不改匹配规则（ADR-0201 决定六） |
| `data/transport/05-schedule-template-sha-szx-express-1200.json`、`06-…-2000.json` | `cmd/parcel-transport-register schedule-template` | 上海→深圳快线（票 demo-chain-gaps/18）：`SYN-SCHED-SHA-SZX-EXPRESS-1200` 与 `-2000`，同走 `SYN-CONN-SHA-SZX`，当地 12:00、20:00 出发、历时 240 分钟，各声明重量 3000 KG、件数 600 PCS（合成值）。计划段窗口随提交时刻移动，首段窗口长十二小时；`01` 那一份按日运行的上海→深圳只接得住一小段提交时刻，加上快线后首段窗口不论落在哪一刻都含一班完整的出发与到达，演示提交因此总有一段 `运输机会已准备`。只调演示租户的班期，不改匹配规则（ADR-0201 决定六）。ADR-0201 的票 18 补记、演示脚本与 `seed.sh` 不复述这些取值，都指到这里 |
| `operator-channel/` | — | 演示发行方：Dex 的容器描述（`compose.yaml`，不并进仓库根那份门禁库描述）与配置（`dex.yaml`：管理台公共客户端、两个合成静态用户），取令牌脚本 `operator-token.sh`，操作者渠道取证脚本 `probe-operator-channel.sh`；另有集成客户端族的发行方配置 `integration-dex.yaml` 与取令牌脚本 `integration-token.sh`，首方客户族的受众与合成客户端也登在 `integration-dex.yaml`，取令牌脚本 `customer-token.sh`。用法见上面「操作者渠道」「集成客户端渠道」「首方客户渠道」三节 |
| `migrate/` | — | 迁移助手（`migrate.Run` 的隔离环境入口；迁移计划刻意没有生产入口） |
| `seedgen/` | — | 计价快照生成器：价卡、序列与参考目录的登记输入带规范化版本号与内容摘要自校，必须经真领域构造函数折装；PPC/PRS/PRC 规范化版本升级时重跑并提交新产物 |

## 集成客户端渠道：演示发行方

票 `product-strategy-boundary/31`。外部结果、监管凭证与外部资金事实各口认集成客户端族的令牌（ADR-0149 决定三），信任锚与操作者族分开：

1. 发行方是 `operator-channel/compose.yaml` 里的第二只 Dex（`integration-issuer`，配置 `integration-dex.yaml`），上面那条 `up -d --wait` 一并起，地址 `http://127.0.0.1:5558/dex`。它只签客户端凭据许可的令牌；版本与操作者族那只不同，理由写在 compose 注释里。
2. 起 `parcel-api` 时再加三件：`IDP_PARCEL_INTEGRATION_CLIENT_OIDC_ISSUER=http://127.0.0.1:5558/dex`、`IDP_PARCEL_INTEGRATION_CLIENT_OIDC_JWKS_URL=http://127.0.0.1:5558/dex/keys`、`IDP_PARCEL_INTEGRATION_CLIENT_OIDC_AUDIENCE=idp-parcel-integration`。日志出现「Integration client channel issuer configured」。
3. 取令牌：`scripts/demo-seeds/operator-channel/integration-token.sh syn-integration-customs-broker-01 syn-customs-broker-secret`（银行是 `syn-integration-bank-01` / `syn-bank-secret`）。客户端行的「要求证书绑定令牌」填的是 `false`：这只发行方签不出 `cnf`，这是演示租户对这两台客户端的取值，不是放宽核验。
4. 承运订舱答复口（`POST /transport-fulfillment/booking-answers`，ADR-0202 决定二）认合成干线承运商这一台：`integration-token.sh syn-integration-carrier-trunk-01 syn-carrier-trunk-secret`。它只能答干线委托下的订舱——答复方的来源身份要等于订舱所属委托的服务提供方，拿它答末端委托下的订舱答 200 `SOURCE_NOT_SERVICE_PROVIDER`。证书绑定一格同上。这台客户端是后加进 `integration-dex.yaml` 的，发行方只在启动时读配置，`up -d` 也不因挂载文件的内容变了就重建容器：之前起过的 `integration-issuer` 要先 `docker compose -f scripts/demo-seeds/operator-channel/compose.yaml up -d --wait --force-recreate integration-issuer` 重建，否则取令牌答 `invalid_client`。它用内存存储，重建后签名钥换新，之前取的令牌作废，重取即可。

## 首方客户渠道：演示发行方

票 `first-party-api/00`。提交口（`POST /shipment-requests`）认首方客户族的令牌（ADR-0139 决定二）：货主客户的系统以 OAuth 2.0
客户端凭据取令牌，`parcel-api` 自己验签名、`iss`、`aud` 与有效期，再查客户渠道册与授予。隔离写开关不放行提交口（ADR-0150 决定三）。

1. 发行方与集成客户端族是同一只（`integration-issuer`，`http://127.0.0.1:5558/dex`），受众另是一个：`idp-parcel-customer-api`，
   只给合成货主系统 `syn-customer-client-01` 取。各族落不落在同一个发行方实例上是部署决定，受众两两不同是硬条件——
   `parcel-api` 见首方受众与操作者族、集成客户端族任一件相同即启动拒。
2. 起 `parcel-api` 时再加三件：`IDP_PARCEL_CUSTOMER_API_OIDC_ISSUER=http://127.0.0.1:5558/dex`、
   `IDP_PARCEL_CUSTOMER_API_OIDC_JWKS_URL=http://127.0.0.1:5558/dex/keys`、`IDP_PARCEL_CUSTOMER_API_OIDC_AUDIENCE=idp-parcel-customer-api`。
   日志出现「First-party customer channel issuer configured」。三件都不设，提交口答 403 `ACCESS_CHANNEL_NOT_CONFIGURED`；
   只设一部分，进程启动即拒。
3. 取令牌：`scripts/demo-seeds/operator-channel/customer-token.sh`（缺省就是那台合成货主系统；末尾加 `claims` 打印载荷）。
   `submit-one-shipment.sh` 自己调它。客户端行（`data/access/07`）的「要求证书绑定令牌」填 `false`，理由同集成客户端那两台。
4. 答复按恢复动作分格（ADR-0139 决定四）：令牌不过或证书绑定对不上答 401 `CUSTOMER_CLIENT_CREDENTIAL_REJECTED`；不在册或
   没有提交授予答 403 `CUSTOMER_CLIENT_NOT_GRANTED`；发行方公钥集或册读不动答 503 `IDENTITY_DEPENDENCY_UNAVAILABLE`。
   操作者令牌与集成客户端令牌在提交口一律答 401。
5. 生产归属：PS 的生产归属目录还没立，演示提交要越过`权威未确定`仍靠隔离写开关那一格合成坐标（`IDP_PARCEL_ISOLATED_WRITE_TENANT`），
   它不放行任何命令面，只给提交编排读得到委托受理那条合成区间。
6. 送站预报（票 ps-external-mark-relations/09，ADR-0212 决定二）：`POST /node-operations/delivery-pre-advices` 交预报，
   `POST /node-operations/delivery-pre-advice-withdrawals` 撤回，同一台合成货主系统、同一枚首方令牌，各要自己那一格授予
   （`DELIVERY_PRE_ADVICE_SUBMISSION`、`DELIVERY_PRE_ADVICE_WITHDRAWAL`）。预报是生产写，准入范围在铸造侧判：起进程还要设
   `IDP_PARCEL_GOVERNANCE_SELF_AUTHORITY=SYN-AUTH/idp-parcel-pilot`（与操作者渠道那一节同一件），不设答 403
   `OUTSIDE_ADMISSION_SCOPE`；隔离开关与它无关。`scripts/demo-seeds/submit-one-pre-advice.sh [客户包裹件号 ...]` 取令牌、交一份，
   把签发的预报标识单独一行打到标准输出（`PRE_ADVICE_ID=$(…)` 取用），说明打到标准错误；不给号码就用按 UTC 时刻起的合成号，
   节点缺省 `SYN-NODE-SHA-HUB`、预计送达缺省此刻往后两小时。同一 UTC 秒内重跑撞同一接入请求标识：同内容答 200 `EXISTING_RESULT`，
   换了号码答 200 `SOURCE_CONFLICT`、脚本失败退出。撤回只撤这台客户端所绑账户的预报，撤别的账户的与撤一份不存在的同答 404
   `DELIVERY_PRE_ADVICE_NOT_VISIBLE`。预报这一步不判有效时窗：时窗（`data/node-operations/01`）在收寄判范围时才读。
   证据只记 `S`。
7. 送站带预报标识（票 ps-external-mark-relations/11，ADR-0212 决定四、七、十）：客户包裹件号的送站此后要有可验证交付范围才唯一
   识别，演示动线因此改成**送站前先交预报**。`scripts/demo-seeds/receive-one-delivery.sh <外部标识>` 用配置员令牌、上海枢纽那台合成
   设备打 `POST /node-operations/receptions`；设了 `IDP_PARCEL_PRE_ADVICE_ID` 就把它作 `deliveryScopeBasis` 带上，没设就不带依据。
   顺序：`submit-one-shipment.sh` 记下客户包裹参考 → `PRE_ADVICE_ID=$(scripts/demo-seeds/submit-one-pre-advice.sh <那个参考>)` → 委托
   接受之后 `IDP_PARCEL_PRE_ADVICE_ID=$PRE_ADVICE_ID scripts/demo-seeds/receive-one-delivery.sh <那个参考>` 答 201 `INTAKE_FORMED`、核对态
   `VERIFIED`；不带预报再送一件同号，答 200 `PENDING_IDENTIFICATION`、核对态 `UNVERIFIED` / `NO_BASIS`，不记冲突。预报的节点要是收寄
   设备的节点（两个脚本缺省都是 `SYN-NODE-SHA-HUB`），收寄时刻要落在〔预计送达 − 提前量，预计送达 + 宽限量〕里（种子时窗提前一天、
   宽限半天）。证据只记 `S`。

## 数据故事（同一合成租户 SYN-TENANT-01，范围 SYN-SCOPE-01）

「建产品 → 配价 → 一单的一生」的主数据半边，跨上下文互引全部指名：

1. **建产品**（party-commercial）：服务产品 `SYN-PROD-CN-SG-EXPRESS`（中国→新加坡合成快递）
   携待路由许可，并声明当面与自提柜两种交付方式；接单规则包 `SYN-RULEPKG-01` 的适用性钉住（产品，合同 `SYN-CONTRACT-01`，
   法人 `SYN-LE-01`，范围）四维，并允许在已受理未收寄阶段更正收件地址；合同正文绑财务控制策略 `SYN-FIN-CONTROL-01`（预付冻结与信用检查都要过），
   把来源资料修订委派给客户账户，并把交付方式收紧到当面交付。第二个产品 `SYN-PROD-CN-SG-ECON`（合成经济线）只声明当面交付，用来撑渠道绑定的另一格：
   EXPRESS 的映射 `SYN-MAP-CN-SG-EXPRESS-01` 配 `SYN-CH-SG-POST-STD` 与 `SYN-CH-AGG-SEA-01`
   两个渠道标识引用，ECON 的映射 `SYN-MAP-CN-SG-ECON-01` 显式登记「未配置」——渠道接入后
   以新修订配置绑定，历史修订保留。
2. **配价**（parcel-pricing）：售价卡 `SYN-PLAN-CN-SG-01`（首重 0.5kg ¥55 续重 ¥18/0.5kg，
   Z1/Z2 两区，MAX 计费重体积系数 5000）方向授权引商业授权对象 `SYN-AUTH-PRICE-DIR-01`，
   方案结构绑燃油序列 `SYN-SERIES-FUEL-01` 与分区目录 `SYN-CAT-ZONE-CN-SG`；成本卡 `SYN-PLAN-CN-SG-COST-01`（BUY）引
   `SYN-AUTH-COST-DIR-01`，两份供应商协议（干线、末端）都采购这一份买入方案；汇率序列 `SYN-SERIES-FX-CNY-SGD` 的口径引价格规则
   `SYN-PRICE-RULE-CN-SG`（汇率不收裸值）。该价格规则同时带卖出方向的价格政策正文与口径，口径与卖出价卡同一套（未税、材积除数 5000、提交时点汇率）。信用政策挂在同一法人与预付费用上，客户服务规则挂在同一法人与产品 `SYN-PROD-CN-SG-EXPRESS` 上（不引预付费用）。 分区目录 `SYN-CAT-ZONE-CN-SG` 用合成 3 位前缀
   把 `018`/`238` 映到同名的 Z1/Z2，始发覆盖 `200` 与 `510`；售价卡绑这本目录，分区由
   委托申报的邮编路线经它解析，不收调用方给的分区。
3. **一单会走的网**（network-routing）：演示租户采用随产品发布的演示网络
   `network-routing/network-catalog/SYN-CN-SG@1`——上海枢纽→深圳口岸→新加坡枢纽→新加坡末端四节点
   三连接成线路 `SYN-LINE-CN-SG-01`，线路与路由策略 `SYN-RS-CN-SG-01` 的适用范围都是服务目的
   `NETWORK_SERVICE`（与 `cmd/parcel-dispatch` 的 `IDP_PARCEL_ROUTE_SERVICE_PURPOSE` 同值），策略选首个
   内置排序形态成本单维；线路三段的成本依据都引成本卡 `SYN-PLAN-CN-SG-COST-01/v1`（与两份供应商协议
   同一张）。服务区域 `SYN-AREA-CN-EAST` 覆盖 CN、收寄节点上海枢纽，`SYN-AREA-SG` 覆盖 SG、交付节点
   新加坡末端。上海枢纽修订 2 与路由策略修订 2 是采用后的租户修订，展示版本轴与「改过的就是租户取值」；一次台风停运
   （SUSPENSION，已解除）展示临时调整与稳定定义分离。自动改路事实一行把四条件陈述折在策略
   `SYN-RS-CN-SG-01/v1` 上，判断键是合成的，库里没有对应的委托。
4. **过关的规则**（customs-compliance）：中国出口侧就绪+提交授权、放行结果解释规则 v1→v2
   换版、案件 `SYN-CASE-CN-SG-01` 的关闭义务（一项已了结、一项承接给 `SYN-BROKER-01`）、
   跨关区移动门禁（前置条件已满足）、建案要求两向（CN 出口要求建案、SG 进口显式不要求）；
   合规候选口岸 `SYN-PORT-SZX-01`（v1→v2 换版）与 `SYN-PORT-SIN-01`，申报路径
   `SYN-PATH-CN-EXPORT-01`（经 SZX、EXPORT、舱单模式）与 `SYN-PATH-SG-IMPORT-01`
   （经 SIN、IMPORT、正式模式）——路径以标识引用口岸，两册对照属读侧（票 03 裁量）。
   第三申报单元 `SYN-UNIT-CN-EXPORT-03` 曾就绪、随后按 `SYN-CAUSE-READINESS-WITHDRAWN` 不再就绪。
   报关行 `SYN-BROKER-01` 持两版监管凭证，程序都是 `SYN-PROC-CN-EXPORT`：`SYN-CRED-CN-EXPORT-01`
   写明 12 次额度，`SYN-CRED-CN-EXPORT-02` 不写次数（来源未提供额度）。出口单元上的税费协作
   按核定 `SYN-DUTY-CN-SG-01` 形成，义务人是 `SYN-LE-01`、交接给 `SYN-BROKER-01`；进口单元
   `SYN-UNIT-SG-IMPORT-01` 另有一格明确无需付款（依据 `SYN-BASIS-NO-DUTY-SG-IMPORT`）。
   演示网络四个节点登了关务区域（上海枢纽、深圳口岸在 `SYN-ZONE-CN`，新加坡两个在 `SYN-ZONE-SG`）：动作准入判断
   据此判 SHA→SZX 同区、只适用装载出发，SZX→SIN 跨区、另加跨关务区域移动（ADR-0206）。演示包裹不在任何案件与
   单元里（逐件建案等 psb/18），所以同区那一步答不受门禁，跨区那一步答未决。
5. **对外怎么说、材料收到没有**（visibility-exception）：异常披露规则 `SYN-VE-EXC-DISCLOSE-V1`
   与分诊同一对信号——`SYN-ACCOUNT-01` 的 `CUSTOMS_HOLD`（承运确认）可披露、不自动发布；
   `SYN-ACCOUNT-02` 的 `ETA_GAP` 明确不披露。冲突信号规则一租户一条，钉在 `CUSTOMS_HOLD` /
   `SYN-VE-CONFLICT-V1`。索赔批次 `SYN-CLAIM-BATCH-CN-SG-01` 的同一事项收了照片与发票，
   发票次日撤销，照片留下。
   出发门禁阻断信号规则一租户一条：出发时关务判阻断的那一件形成 `DEPARTURE_GATE_BLOCKED`
   （`SYN-VE-DEPARTURE-GATE-V1`，可信度 `SOURCE_CONFIRMED`）信号；分诊规则没登这一类，信号进人工复核。
   演示动线上自营 SHA→SZX 的出发读数是不受门禁，所以这一行在动线上不会命中。
6. **代收怎么归集**（collection-remittance）：`SYN-ACCOUNT-01` / `SYN-LE-01` / CNY /
   `SYN-CH-SG-POST-STD` 这本账上，`SYN-BATCH-CNY-OPEN` 停在已归集，`SYN-BATCH-CNY-HANDED`
   已交出汇付主张。交出的是主张，不是付款，六个资金位置不因此改数。同客户的 SGD 账仍然没有批次。
7. **受理前控制占哪个账**（settlement-accounting 与规则包的时点格）：规则包 `SYN-RULEPKG-01` 的受理前财务控制
   一格与可达性一格一样，采用产品参考配置 `parcel-shipment/as-of-semantics/submission-receipt@1`，判断时点取该提交
   版本的系统接收时间。控制的作用域从商业解析回显的结算政策 `SYN-SETTLEMENT-PREPAID-01` 派生，按法人、相对方、
   应收、币种、结算政策五维在结算账户登记册上找到 `SYN-SETTLEMENT-ACCOUNT-01`。控制金额不在种子里，它由估价形成：
   种子只登这一户采用的形态「逐件 SELL 试算合计」与计价范围 `SYN-SCOPE-01`（ADR-0179 决定五），结算按委托当前提交
   版本逐件申报的测量与邮编路线，用该范围里在用的售价卡逐件试算、合计成控制金额。

8. **货怎么运**（transport-fulfillment，演示租户的租户取值，证据只记 `S`）：三条连接各有一份按日运行的周期安排
   （上表 `data/transport/`），具体班次由（周期安排代码、运行日期）派生，例如 `SYN-SCHED-SZX-SIN-DAILY/2026-10-21`。
   各段由谁履约（自营还是外包、外包采用哪份协议）是演示租户的声明，不从有没有协议或委托推出来；声明只写在
   `docs/design/synthetic-demo-journey-script.md`「第 3 步 · 一单会走的网」的 2026-10-19 补记里，这里不复述。
   每份周期安排还声明了重量与件数两维的硬上限，按日期登记一班时随班次各建一池；种子只登了干线 2026-10-21 那一班，
   它的两池在 `capacity-pool` 查阅册上（票 demo-chain-gaps/17）。
   干线（`SZX-GATE→SIN-HUB`，外包给 `SYN-PARTY-CARRIER-TRUNK-01`、采用 `SYN-SUPPLIER-TRUNK-CN-SG-01` 的 `v1`）的参与在
   SIN-HUB 以下一次交接结束之后，配置员登一项 `ACTUAL_FULFILLMENT` 运输收费发生项（ADR-0207）：事实给那个干线段与
   包裹，服务方与协议版本由登记方给出、经 party-commercial 核对，旅程按演示约定写 `SYN-JRN-<包裹号>`，数量 1、单位
   `SEGMENT`，并断言协议 v1 的发生条件已满足——协议正文还不声明发生条件，这一格由人判断并留痕。登成之后结算按上表
   BUY 触发册那一行发起 BUY 评价（ADR-0208）。数量、单位与断言是演示租户的取值，证据只记 `S`。断言人不在载荷里，取
   登记令牌的操作者，随发生项回显。同一干线段上有几件包裹、协议按段计费时，等它们的参与都结束后作为一项的成员一并
   登记；逐件各登「数量 1、单位 `SEGMENT`」会把同一段算成几份段费，成员交集保护按成员判、拦不住这一种（ADR-0207
   越权风险点 4）。
   干线那一班的重量池 `SYN-SCHED-SZX-SIN-DAILY/2026-10-21/WEIGHT`（5000 KG）上，配置员可以声明一份预占（ADR-0203）：
   预占引用自起（如 `SYN-RES-SZX-SIN-01`），数量与单位按池的单位写（`KG`，写成别的单位答 `UNIT_MISMATCH`，不换算），
   来源写 `{"kind": "OPERATOR_DECLARATION", "reference": "SYN-DEMAND-DECL-01/v1"}`，判断时点与有效期由声明方给。新预占
   答 201 `CAPACITY_RESERVED`，原样重交答 200 已有；超出余量答 200 `CAPACITY_EXCEEDED` 并带余量，没有任何审批能越过。
   需求量与来源是演示租户的声明值，证据只记 `S`。
   装载分配要带预占（ADR-0203 决定一、二，票 demo-chain-gaps/21）：班次挂了池，分配就要对每个池各带一行、引用那个池上
   一份当前有效的预占；干线 2026-10-21 那一班挂重量与件数两池，所以先在两池上各预占一份再分配。载荷怎么写、各格怎么答
   只写在 `docs/design/synthetic-demo-journey-script.md`「第 3 步 · 一单会走的网」的票 21 补记里，这里不复述。
   种子不登任何分配；分配量与来源是演示租户的声明值，证据只记 `S`。

   提交一单、初始路由形成之后，运输履约逐段形成准备结果（ADR-0201 决定六至八，票 demo-chain-gaps/18）：`GET
   /transport-fulfillment-records?registry=transport-preparation` 列出这一件的三段，各带结论、段窗口、按申报取的需求量
   （重量、外廓乘出的体积、一件）与候选依据。首段总有一班快线或按日那一班落在窗口里、两池都够，答
   `OPPORTUNITY_PREPARED`，选中的班次多半只是评估时按日期物化出来的（候选 `version` 为 0）——要订舱或装载分配先显式登记
   那一班。后两段的窗口按演示网络的处理时长与衔接缓冲算，与按日那两份的班期大多错开，答 `NO_CURRENT_OPPORTUNITY`；
   这是如实答案。准备不预占、不建班次、不发信封。

   2026-10-19 起（票 demo-chain-gaps/29，ADR-0209）上段断言期的登法作废：干线协议以 `v2` 重发（发布批里紧跟 `v1`
   的那一项，摘要 `PCC-2`），正文声明收费发生条件——实际履约按件计、失败尝试不计费，订舱与取消不声明；`v1` 原样在册、
   不带条件。演示的段采用与发生项登记都指名 `v2`。登记载荷只给事实、旅程、服务方与协议版本，不再带断言、数量与单位
   （带了答 400）；数量与单位由协议的数量形定，一件包裹是 1 件 `PIECE`，依据记 `AGREEMENT_DECLARED`，登记者取登记令牌
   的操作者。指名 `v1` 登记答 200 `NOT_CONFIGURED` 并带回 `…/v1`。BUY 触发册那一行的主要范围取发生项上那一版协议
   （`v2`）的商业范围，与 `v1` 同为 `SYN-SCOPE-01`。按件计时同一段的几件包裹可以各登一项；协议若改按段计，一段只能登
   一项，第二项答 `SCOPE_ALREADY_CHARGED`。

## 已知边界（如实记录，不是缺陷）

- 早先灌过的库上，挂合成发行方 `https://syn-issuer-01.example.invalid` 的两个主体与三笔授予还在操作者册里。没有
  哪个发行方签得出那个 `iss`，它们在册上与能登录的主体长得一样，却永远登不进来，所以种子不再登它们。册只增不改，
  要清掉就 `--reset`。
- 商业价格政策正文随发布批里的 `PRICE_RULE` 经 `pricePolicyBody` 与口径发布。价卡仍是
  `parcel-pricing` 的登记；两套数字要对上是种子自己的事，发布口不替价卡校对。
  `RehydrateAdoptedBasisSpec` 的快照重建至今缺席，补发布通道不等于补重建（记于票
  `commercial-closure-settlement-key/02`）。
- `pilot_governance.takeover_record` 的写入口已开：`parcel-governance-register takeover`
  （ADR-0154）。查阅口已开：`GET /governance-registers?register=takeover`（ADR-0155），
  空册答空列表。本包不造接管行。停写证据、继任权威和在途盘点要等证据责任方给出隔离 `S`
  或自然发生的 `P`；`PAR-GOV-05..07` 的租户值仍待提供，开发方不填。空表是如实的。
- 网络这四张空表不是种子漏灌：`network_definition`（0007）头注写明今天没有写入方，读口恒答
  未配置，登记口也不再读它（`parcel-network-register` 只登 0008 目录、0009 事实与 0019 段计价区域）。
  `initial_route`、`plan_applicability`、`reachability_judgment` 是一次路由判断留下的运行时
  记录，没有登记 CLI。自动改路事实有 CLI，本包登了一行合成陈述，它填不满这三张表。
- `service_product_form`（0008）那半边已经补上（票 `admin-remainder-mechanism-batch/02`）：
  `register-products` 子命令是 `SaveServiceProduct` 的进程级调用方，本包两个产品的形态随
  种子落册，服务产品页的形态列不再为空。
- 结算政策那一格已经补上（票 `commercial-closure-settlement-key/02`）：发布批里的
  `SYN-SETTLEMENT-PREPAID-01` 与 `SYN-SETTLEMENT-PREPAID-02` 各是一户的结算约定，六维与该户解析键那三维加闭包解出的合同
  版本严丝合缝——差一维就不再被采用，本上下文不许借宽泛客户关系跨维归集。
- 第二客户 `SYN-ACCOUNT-02`（票 customer-service-rule-contract-applicability/06）：合同 `SYN-CONTRACT-02` 正文指名该账户（ADR-0177），
  闭包按键上的客户账户只让这一户的合同参选；同范围并存挂产品的 `SYN-SERVICE-RULE-01`（首次索赔 14 天、LOSS 要 POD）与壳指名合同 02
  的 `SYN-SERVICE-RULE-CONTRACT-02`（只登首次索赔 7 天）。账户 02 的闭包合同优先采纳后者，期限读合同版、材料行继承产品版；
  账户 01 的合同层零候选，回落产品版。演示今天停在已提交，这一对比面只能以闭包与资格读面的答复取证（S），见票 06。
- 七页真数据展示还依赖查询端点的目录 Intake 配置（PAR-INT-01 未决期间装配
  UnconfiguredIntake，生产路径 403 是刻意的）；本包只负责库内数据态，页面接线归票 07。
- 税费付款核对（`duty-payment-verification`）不进本包。核对要三样前置同时在册：已接收的
  外部资金事实版本、同一范围上已形成的协作事项、该监管程序的付款人规则。资金事实只经
  settlement-accounting 的采用信封进关务（本登记 CLI 故意没有这条命令）；付款人规则也没有
  种子能调用的登记入口。协作事项两格已经落下，核对仍会答前置未齐（退出码 3）。不绕过 CLI
  往核对表插行。
- 索赔材料收讫落在 `visibility_exception.claim_material_receipt`（及撤销表）。管理台没有
  读这一册的页，灌进去也没有页可看。
- 回汇批次不再整段留空。早先「一笔不造、页面显未配置」把演示租户的合成实例当成了不许填的
  租户取值；同段的指令、事实和记账已经是 `SYN-` 实例。现在 CNY 账有已归集与已交出两格，
  SGD 账仍无批次，分户账页对空批次继续写「未配置」。不带 `--reset` 重放本节仍会在既有记账处
  因余额不足中止，批次与交出这两步本身重放走 0。
- 运行时只多一步，而且**不在** `seed.sh` 里：`submit-one-shipment.sh` 先替合成货主系统取一枚首方客户端令牌，再对已经在跑的
  `parcel-api` 打 `POST /shipment-requests`（草案带寄 / 收两端国家码 `CN` / `SG`，演示网络的服务区域
  按国家覆盖；另带两端邮编 `200000` / `018000` 与这一件的尺寸，受理前估价按邮编路线经分区目录解出 Z1、
  按 MAX 计费重算体积重），断言 `201` / `SUBMITTED`，再从
  `GET /shipment-request-views` 把同一笔读回来。起进程要带首方族三件（上面「首方客户渠道」一节），并与 `parcel.sh` 一样同时设
  `IDP_PARCEL_ISOLATED_READ_TENANT` 与 `IDP_PARCEL_ISOLATED_WRITE_TENANT`，都是
  `SYN-TENANT-01`：读开关给委托查阅注入可见账户，写开关给归属注入合成坐标。只开写、不开读时提交能落成已提交，列表和详情仍答 `403`
  `ACCESS_CHANNEL_NOT_CONFIGURED`——读 Intake 还是未配置，脚本看到 403 就停，不改门。
  不设写开关时提交停在 `OWNERSHIP_UNRESOLVED`（生产形态的如实答复）；首方族三件不设时提交口答 403 `ACCESS_CHANNEL_NOT_CONFIGURED`。路由、运输履约、节点作业、
  轨迹投影、异常案件、结算这些运行时页保持空着：本脚本不灌它们，空是如实答案。
  证据只记 `S`。
  这一步不幂等：跨秒重跑各落一笔委托；同一 UTC 秒内重跑撞同一来源键，提交口答 `200`
  `EXISTING_RESULT`，脚本因要求 `201` 失败退出，不落第二笔。
- 干线发生项往下的 BUY 评价停在待判断（票 demo-chain-gaps/27 实测，证据 `S`）。登成 `ACTUAL_FULFILLMENT` 发生项之后，
  结算发起一份 BUY 请求、计价形成一份评价，评价答 `PENDING` / `ZONE_UNRESOLVED`：演示成本卡没绑分区目录，按发生项
  造的计价输入也不带分区（ADR-0189 越权风险点 3）。结算按 ADR-0200 把它入账，不形成供应商预期成本。演示成本卡绑
  分区目录这一格另立票 demo-chain-gaps/43，要等 routing-first-cut/20 落地。
