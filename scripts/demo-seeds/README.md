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

## 组成

| 目录 | 入库通道 | 内容 |
|---|---|---|
| `data/commercial/` | `cmd/parcel-commercial publish` | 发布批：两份服务产品（各带交付条件）、接受前财务控制策略正文、接单规则包（五类规则正文+受理内容+收寄资格+时点锚+终局规则+资料修订允许）、客户合同（正文+受理前控制+合同委派+收紧交付条件）、结算政策（预付，六维范围对齐解析键）、两份供应商协议正文、价格规则（价格政策正文+口径）、信用政策、客户服务规则、三份授权规则（方向两份仍是壳，取消授权带正文） |
| `data/commercial/resolution-key-*.json` | `cmd/parcel-commercial register-resolution-key` | 消费方（parcel-shipment）的解析键登记 1 行：四项必需依据加结算三维——它不是商业权威发布，只是与发布共用一个 CLI |
| `data/commercial/register-products.json` | `cmd/parcel-commercial register-products` | 服务形态两笔（EXPRESS/ECON 均网络服务）+ 产品—渠道映射两笔：EXPRESS 配两个渠道标识引用，ECON 显式登记「未配置」（该产品尚无可用渠道候选）——渠道本体不预造（ADR-0072） |
| `data/commercial/register-registration-number-types.json` | `cmd/parcel-commercial register-registration-number-types` | 注册号类型目录（ADR-0145 决定一）：身份层由演示租户显式采用 CN（统一社会信用代码）、SG（UEN）两份参考配置（ADR-0147，批文 `adopt` 格，依据格随之写成 `REFCFG-1:…@1`），资料层 CN、SG 各登一类 `SYN-` 合成税务登记号，另有一类 SG 资料层旧类型登记后停用，让目录状态的「已停用」有实例可显。产品本身不带任何国家 / 地区的目录条目：参考配置不采用就不进任何租户的目录 |
| `data/commercial/register-parties.json` | `cmd/parcel-commercial register-parties` | 参与方身份：业务参与方（含合成干线承运方 `SYN-PARTY-CARRIER-TRUNK-01`、合成末端承运方 `SYN-PARTY-CARRIER-LM-01`，供两份供应商协议引用）、关系、责任法人、客户账户 |
| `data/commercial/register-legal-entity-profiles.json` | `cmd/parcel-commercial register-legal-entity-profiles` | 法人资料（ADR-0145 决定三）：`SYN-LE-01` 两笔修订——修订 1 不带开票资料（按它生效的时段解析答「资料不全」），修订 2 自 3 月起补上开票抬头；注册地址在 `CN`，与法人身份层的注册国家一致，税号取目录里 `CN` 的资料层合成类型。排在参与方身份之后：资料引用已登记的责任法人 |
| `data/pricing/` | `cmd/parcel-pricing-register` | 两张价卡（SELL 首重续重 / BUY 重量段）+ 两条参考序列（燃油、汇率）+ 两份序列复核 + 一份分区参考目录与其复核（不复核不在用，ADR-0099 / ADR-0109）——由 `seedgen` 生成，勿手改 |
| `data/network/` | `cmd/parcel-network-register` | 七族 14 行：4 节点（含一次换版）、3 连接、1 线路、2 服务区、1 日历、1 台风停运调整、1 路由策略；另加 1 行自动改路事实（判断键对齐 `SYN-ACCOUNT-01` 与 `SYN-RS-CN-SG-01`，不是一笔已受理委托的运行时产物） |
| `data/customs/` | `cmd/parcel-customs-register` | 十册 26 份：就绪与授权（第三单元先就绪再撤销就绪；第二单元只撤授权）、解释规则（含一次换版）、义务目录+两项（已了结/已承接）、门禁目录+判断（含一份只登目录的空清单格）、建案要求两向（要求/显式不要求）、口岸目录（SZX 含一次换版 + SIN）、申报路径两向（CN 出口 / SG 进口）、监管凭证两版（一版写明次数额度、一版来源未提供额度）、税费付款协作两格（核定税费 / 明确无需付款）。税费付款核对不在包内，见已知边界 |
| `data/visibility/` | `cmd/parcel-ve-register` | 里程碑映射、分诊规则、通知策略、索赔资格、索赔授权两格（一格空名单）、披露策略、异常披露规则、冲突信号规则、索赔材料收讫两笔（照片留下、发票收讫后撤销） |
| `data/collection/` | `cmd/parcel-collection-register` | 一条 COD 指令走全程（渠道报收、银行短收、短款、清分进应付客户）+ SGD 分户账只开立；CNY 账两笔回汇批次（已归集 / 已交出汇付主张），SGD 账无批次 |
| `data/access/` | `cmd/parcel-access-register` | 操作者册（ADR-0100 决定二第三条）：两个合成操作者主体绑演示租户，配置员授登记册配置写与主数据与运营查阅读两格，查阅员只授查阅读；发行方是合成值，演示部署接上真 OIDC 发行方后按其标识另登 |
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
   方案结构绑燃油序列 `SYN-SERIES-FUEL-01`；成本卡 `SYN-PLAN-CN-SG-COST-01`（BUY）引
   `SYN-AUTH-COST-DIR-01`，两份供应商协议（干线、末端）都采购这一份买入方案；汇率序列 `SYN-SERIES-FX-CNY-SGD` 的口径引价格规则
   `SYN-PRICE-RULE-CN-SG`（汇率不收裸值）。该价格规则同时带卖出方向的价格政策正文与口径，口径与卖出价卡同一套（未税、材积除数 5000、提交时点汇率）。信用政策挂在同一法人与预付费用上，客户服务规则挂在同一法人与产品 `SYN-PROD-CN-SG-EXPRESS` 上（不引预付费用）。 分区目录 `SYN-CAT-ZONE-CN-SG` 用合成 3 位前缀
   把 `018`/`238` 映到同名的 Z1/Z2，始发覆盖 `200` 与 `510`；价卡不绑这本目录，评价仍走
   调用方给值。
3. **一单会走的网**（network-routing）：上海枢纽→深圳口岸→新加坡枢纽→新加坡末端四节点
   三连接成线路 `SYN-LINE-CN-SG-01`（applicable_scope 同 `SYN-SCOPE-01`）；上海枢纽 v1→v2
   换版展示版本轴；一次台风停运（SUSPENSION，已解除）展示临时调整与稳定定义分离。
   自动改路事实一行把四条件陈述折在策略 `SYN-RS-CN-SG-01/v1` 上，判断键是合成的，库里
   没有对应的委托。
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

## 已知边界（如实记录，不是缺陷）

- 商业价格政策正文随发布批里的 `PRICE_RULE` 经 `pricePolicyBody` 与口径发布。价卡仍是
  `parcel-pricing` 的登记；两套数字要对上是种子自己的事，发布口不替价卡校对。
  `RehydrateAdoptedBasisSpec` 的快照重建至今缺席，补发布通道不等于补重建（记于票
  `commercial-closure-settlement-key/02`）。
- `pilot_governance.takeover_record` 没有登记入口。`parcel-governance-register` 对未知种类
  答「接管未开」；ADR-0128 决定五写明接管记录的语义与写侧不在该记录内，`PAR-GOV-05..07`
  仍是待提供的租户取值。这是刻意没开，不是种子漏调用。演示不造接管行。
- 网络这四张空表不是种子漏灌：`network_definition`（0007）头注写明今天没有写入方，读口恒答
  未配置，登记口也不再读它（`parcel-network-register` 只登 0008 目录与 0009 事实）。
  `initial_route`、`plan_applicability`、`reachability_judgment` 是一次路由判断留下的运行时
  记录，没有登记 CLI。自动改路事实有 CLI，本包登了一行合成陈述，它填不满这三张表。
- `service_product_form`（0008）那半边已经补上（票 `admin-remainder-mechanism-batch/02`）：
  `register-products` 子命令是 `SaveServiceProduct` 的进程级调用方，本包两个产品的形态随
  种子落册，服务产品页的形态列不再为空。
- 结算政策那一格已经补上（票 `commercial-closure-settlement-key/02`）：发布批里的
  `SYN-SETTLEMENT-PREPAID-01` 是本包唯一一份结算约定，六维与解析键那三维加闭包解出的合同
  版本严丝合缝——差一维就不再被采用，本上下文不许借宽泛客户关系跨维归集。
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
  `parcel-api` 打 `POST /shipment-requests`，断言 `201` / `SUBMITTED`，再从
  `GET /shipment-request-views` 把同一笔读回来。起进程要与 `parcel.sh` 一样同时设
  `IDP_PARCEL_ISOLATED_READ_TENANT` 与 `IDP_PARCEL_ISOLATED_WRITE_TENANT`，都是
  `SYN-TENANT-01`。只开写、不开读时提交能落成已提交，列表和详情仍答 `403`
  `ACCESS_CHANNEL_NOT_CONFIGURED`——读 Intake 还是未配置，脚本看到 403 就停，不改门。
  两个都不设时提交口自己也是这道 403（`PAR-INT-01`）。路由、运输履约、节点作业、
  轨迹投影、异常案件、结算这些运行时页保持空着：本脚本不灌它们，空是如实答案。
  证据只记 `S`。
