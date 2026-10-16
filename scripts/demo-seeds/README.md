# 合成 S 主数据种子包（仅限隔离环境）

票 `.scratch/master-data-wiring/issues/08-synthetic-seed-pack.md` 的产物：一套 `SYN-` 前缀的
合成主数据种子，经四个登记 CLI 灌入本机演示库，让主数据区七页有数据可看。

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
   权威区间（`data/governance/09`–`20`）才认得上。取值与隔离形态的权威串同串，隔离写开关开着而两值不等进程启动即拒。
   管理台上的关段、建派送任务、有效时间判断与承运商首次有效收寄判断四口走的也是这一条（票 operator-channel/15 起不再经
   隔离写开关）：不设这一件，配置员登录后在这几页上答的是 `OUTSIDE_ADMISSION_SCOPE`。

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
| `syn-operator-config-01@syn-tenant-01.example.invalid` | 登记册配置写、主数据与运营查阅读、运营决定（装载分配、终止参与、关段、建派送任务、有效时间判断、承运商首次有效收寄判断） |
| `syn-operator-viewer-01@syn-tenant-01.example.invalid` | 主数据与运营查阅读 |

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
| `data/commercial/register-legal-entity-profiles.json` | `cmd/parcel-commercial register-legal-entity-profiles` | 法人资料（ADR-0145 决定三）：`SYN-LE-01` 两笔修订——修订 1 不带开票资料（按它生效的时段解析答「资料不全」），修订 2 自 3 月起补上开票抬头；注册地址在 `CN`，与法人身份层的注册国家一致，税号取目录里 `CN` 的资料层合成类型。排在参与方身份之后：资料引用已登记的责任法人 |
| `data/pricing/` | `cmd/parcel-pricing-register` | 两张价卡（SELL 首重续重 / BUY 重量段）+ 两条参考序列（燃油、汇率）+ 两份序列复核 + 一份分区参考目录与其复核（不复核不在用，ADR-0099 / ADR-0109）；售价卡绑燃油序列与这本分区目录——由 `seedgen` 生成，勿手改 |
| `data/network/` | `cmd/parcel-network-register` | 稳定定义由演示租户显式采用参考配置 `network-routing/network-catalog/SYN-CN-SG@1`（ADR-0147，登记行 `adopt` 格，依据格随之写成 `REFCFG-1:…@1`）：4 节点、3 连接、1 线路（三段成本依据引 BUY 卡）、2 服务区（CN 收寄 / SG 交付覆盖）、7 份日历（4 节点的处理时长与截单、3 连接的衔接缓冲）、1 路由策略（成本单维）；上海枢纽修订 2 是采用后的租户修订，依据换成租户自己的。另直接登记 1 条台风停运调整与 1 行自动改路事实（判断键对齐 `SYN-ACCOUNT-01` 与 `SYN-RS-CN-SG-01`，不是一笔已受理委托的运行时产物）。产品本身不带任何网络：参考配置不采用就不进任何租户的目录 |
| `data/customs/` | `cmd/parcel-customs-register` | 十册 26 份：就绪与授权（第三单元先就绪再撤销就绪；第二单元只撤授权）、解释规则（含一次换版）、义务目录+两项（已了结/已承接）、门禁目录+判断（含一份只登目录的空清单格）、建案要求两向（要求/显式不要求）、口岸目录（SZX 含一次换版 + SIN）、申报路径两向（CN 出口 / SG 进口）、监管凭证两版（一版写明次数额度、一版来源未提供额度）、税费付款协作两格（核定税费 / 明确无需付款）。税费付款核对不在包内，见已知边界 |
| `data/visibility/` | `cmd/parcel-ve-register` | 里程碑映射、分诊规则、通知策略、索赔资格、索赔授权两格（一格空名单）、披露策略、异常披露规则、冲突信号规则、索赔材料收讫两笔（照片留下、发票收讫后撤销） |
| `data/collection/` | `cmd/parcel-collection-register` | 一条 COD 指令走全程（渠道报收、银行短收、短款、清分进应付客户）+ SGD 分户账只开立；CNY 账两笔回汇批次（已归集 / 已交出汇付主张），SGD 账无批次 |
| `data/settlement/` | `cmd/parcel-settlement-register` 的 `settlement-account` / `external-funds-fact` / `funds-mapping` / `pre-acceptance-estimate-adoption` | 结算账户登记册（ADR-0158）1 行：货主 `SYN-PARTY-SHIPPER-01` 在法人 `SYN-LE-01` 下的 CNY 应收账户 `SYN-SETTLEMENT-ACCOUNT-01`，结算政策 `SYN-SETTLEMENT-PREPAID-01`，责任依据是该政策所引的客户合同 `SYN-CONTRACT-01`。五维照发布批里那份结算政策的正文配，受理前财务控制的作用域经它找到账户。另有一笔合成收款 `SYN-FUNDS-RECEIPT-01`（200.00 CNY）映射到该账户（ADR-0182），金额只盖这一件的试算合计，不是生产默认。受理前估价采用册（ADR-0179 决定五）1 行：这一户采用「逐件 SELL 试算合计」（`PER_PARCEL_SELL_TRIAL_SUM`），计价范围 `SYN-SCOPE-01`，与售价卡 `SYN-PLAN-CN-SG-01` 的范围同值，估价按它解析在用售价卡 |
| `data/access/` | `cmd/parcel-access-register` | 操作者册（ADR-0100 决定二第三条）：两个合成操作者主体绑演示租户，配置员授登记册配置写与主数据与运营查阅读两格，查阅员只授查阅读；主体是演示发行方 Dex 给 `operator-channel/dex.yaml` 两个静态用户真签出的 `sub`，换发行方就按新发行方的标识另登一批 |
| `operator-channel/` | — | 演示发行方：Dex 的容器描述（`compose.yaml`，不并进仓库根那份门禁库描述）与配置（`dex.yaml`：管理台公共客户端、两个合成静态用户），取令牌脚本 `operator-token.sh`，操作者渠道取证脚本 `probe-operator-channel.sh`；用法见上面「操作者渠道」一节 |
| `migrate/` | — | 迁移助手（`migrate.Run` 的隔离环境入口；迁移计划刻意没有生产入口） |
| `seedgen/` | — | 计价快照生成器：价卡、序列与参考目录的登记输入带规范化版本号与内容摘要自校，必须经真领域构造函数折装；PPC/PRS/PRC 规范化版本升级时重跑并提交新产物 |

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
   新加坡末端。上海枢纽修订 2 是采用后的租户修订，展示版本轴与「改过的就是租户取值」；一次台风停运
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
5. **对外怎么说、材料收到没有**（visibility-exception）：异常披露规则 `SYN-VE-EXC-DISCLOSE-V1`
   与分诊同一对信号——`SYN-ACCOUNT-01` 的 `CUSTOMS_HOLD`（承运确认）可披露、不自动发布；
   `SYN-ACCOUNT-02` 的 `ETA_GAP` 明确不披露。冲突信号规则一租户一条，钉在 `CUSTOMS_HOLD` /
   `SYN-VE-CONFLICT-V1`。索赔批次 `SYN-CLAIM-BATCH-CN-SG-01` 的同一事项收了照片与发票，
   发票次日撤销，照片留下。
6. **代收怎么归集**（collection-remittance）：`SYN-ACCOUNT-01` / `SYN-LE-01` / CNY /
   `SYN-CH-SG-POST-STD` 这本账上，`SYN-BATCH-CNY-OPEN` 停在已归集，`SYN-BATCH-CNY-HANDED`
   已交出汇付主张。交出的是主张，不是付款，六个资金位置不因此改数。同客户的 SGD 账仍然没有批次。
7. **受理前控制占哪个账**（settlement-accounting 与规则包的时点格）：规则包 `SYN-RULEPKG-01` 的受理前财务控制
   一格与可达性一格一样，采用产品参考配置 `parcel-shipment/as-of-semantics/submission-receipt@1`，判断时点取该提交
   版本的系统接收时间。控制的作用域从商业解析回显的结算政策 `SYN-SETTLEMENT-PREPAID-01` 派生，按法人、相对方、
   应收、币种、结算政策五维在结算账户登记册上找到 `SYN-SETTLEMENT-ACCOUNT-01`。控制金额不在种子里，它由估价形成：
   种子只登这一户采用的形态「逐件 SELL 试算合计」与计价范围 `SYN-SCOPE-01`（ADR-0179 决定五），结算按委托当前提交
   版本逐件申报的测量与邮编路线，用该范围里在用的售价卡逐件试算、合计成控制金额。

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
  未配置，登记口也不再读它（`parcel-network-register` 只登 0008 目录与 0009 事实）。
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
- 运行时只多一步，而且**不在** `seed.sh` 里：`submit-one-shipment.sh` 对已经在跑的
  `parcel-api` 打 `POST /shipment-requests`（草案带寄 / 收两端国家码 `CN` / `SG`，演示网络的服务区域
  按国家覆盖；另带两端邮编 `200000` / `018000` 与这一件的尺寸，受理前估价按邮编路线经分区目录解出 Z1、
  按 MAX 计费重算体积重），断言 `201` / `SUBMITTED`，再从
  `GET /shipment-request-views` 把同一笔读回来。起进程要与 `parcel.sh` 一样同时设
  `IDP_PARCEL_ISOLATED_READ_TENANT` 与 `IDP_PARCEL_ISOLATED_WRITE_TENANT`，都是
  `SYN-TENANT-01`。只开写、不开读时提交能落成已提交，列表和详情仍答 `403`
  `ACCESS_CHANNEL_NOT_CONFIGURED`——读 Intake 还是未配置，脚本看到 403 就停，不改门。
  两个都不设时提交口自己也是这道 403（`PAR-INT-01`）。路由、运输履约、节点作业、
  轨迹投影、异常案件、结算这些运行时页保持空着：本脚本不灌它们，空是如实答案。
  证据只记 `S`。
  这一步不幂等：跨秒重跑各落一笔委托；同一 UTC 秒内重跑撞同一来源键，提交口答 `200`
  `EXISTING_RESULT`，脚本因要求 `201` 失败退出，不落第二笔。
