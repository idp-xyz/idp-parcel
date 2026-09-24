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

干净库上全程零报错。四个 CLI 对重复输入各自幂等（已登记/重放答 0），但网络目录的
版本行撞主键会答未决（3）——对已灌过的库重跑请用 `--reset`。责任法人 `SYN-LE-01` 的修订 1
自票 legal-entity-profile/02 起带身份层（注册国家 `CN` 与合成终身注册号，ADR-0145 决定一）；在那之前
灌过的库上这一笔是没有身份层的旧形状，重跑会答内容冲突（2），同样用 `--reset`。

## 组成

| 目录 | 入库通道 | 内容 |
|---|---|---|
| `data/commercial/` | `cmd/parcel-commercial publish` | 发布批：服务产品、财务控制策略、接单规则包（五类规则正文+受理内容+收寄资格+时点锚+终局规则）、客户合同（正文+受理前控制）、结算政策（预付，六维范围对齐解析键）、供应商协议、价格规则、授权规则 |
| `data/commercial/resolution-key-*.json` | `cmd/parcel-commercial register-resolution-key` | 消费方（parcel-shipment）的解析键登记 1 行：四项必需依据加结算三维——它不是商业权威发布，只是与发布共用一个 CLI |
| `data/commercial/register-products.json` | `cmd/parcel-commercial register-products` | 服务形态两笔（EXPRESS/ECON 均网络服务）+ 产品—渠道映射两笔：EXPRESS 配两个渠道标识引用，ECON 显式登记「未配置」（该产品尚无可用渠道候选）——渠道本体不预造（ADR-0072） |
| `data/commercial/register-registration-number-types.json` | `cmd/parcel-commercial register-registration-number-types` | 注册号类型目录（ADR-0145 决定一）：身份层由演示租户显式采用 CN（统一社会信用代码）、SG（UEN）两份参考配置（ADR-0147，批文 `adopt` 格，依据格随之写成 `REFCFG-1:…@1`），资料层 CN、SG 各登一类 `SYN-` 合成税务登记号，另有一类 SG 资料层旧类型登记后停用，让目录状态的「已停用」有实例可显。产品本身不带任何国家 / 地区的目录条目：参考配置不采用就不进任何租户的目录 |
| `data/commercial/register-legal-entity-profiles.json` | `cmd/parcel-commercial register-legal-entity-profiles` | 法人资料（ADR-0145 决定三）：`SYN-LE-01` 两笔修订——修订 1 不带开票资料（按它生效的时段解析答「资料不全」），修订 2 自 3 月起补上开票抬头；注册地址在 `CN`，与法人身份层的注册国家一致，税号取目录里 `CN` 的资料层合成类型。排在参与方身份之后：资料引用已登记的责任法人 |
| `data/pricing/` | `cmd/parcel-pricing-register` | 两张价卡（SELL 首重续重 / BUY 重量段）+ 两条参考序列（燃油、汇率）+ 两份序列复核（不复核不在用，ADR-0099）——由 `seedgen` 生成，勿手改 |
| `data/network/` | `cmd/parcel-network-register` | 七族 14 行：4 节点（含一次换版）、3 连接、1 线路、2 服务区、1 日历、1 台风停运调整、1 路由策略 |
| `data/customs/` | `cmd/parcel-customs-register` | 八册 20 份：就绪与授权（各含第二单元，授权含一次撤销）、解释规则（含一次换版）、义务目录+两项（已了结/已承接）、门禁目录+判断（含一份只登目录的空清单格）、建案要求两向（要求/显式不要求）、口岸目录（SZX 含一次换版 + SIN）、申报路径两向（CN 出口 / SG 进口） |
| `data/access/` | `cmd/parcel-access-register` | 操作者册（ADR-0100 决定二第三条）：两个合成操作者主体绑演示租户，配置员授登记册配置写与主数据与运营查阅读两格，查阅员只授查阅读；发行方是合成值，演示部署接上真 OIDC 发行方后按其标识另登 |
| `migrate/` | — | 迁移助手（`migrate.Run` 的隔离环境入口；迁移计划刻意没有生产入口） |
| `seedgen/` | — | 计价快照生成器：价卡与序列的登记输入带规范化版本号与内容摘要自校，必须经真领域构造函数折装；PPC/PRS 规范化版本升级时重跑并提交新产物 |

## 数据故事（同一合成租户 SYN-TENANT-01，范围 SYN-SCOPE-01）

「建产品 → 配价 → 一单的一生」的主数据半边，跨上下文互引全部指名：

1. **建产品**（party-commercial）：服务产品 `SYN-PROD-CN-SG-EXPRESS`（中国→新加坡合成快递）
   携待路由许可；接单规则包 `SYN-RULEPKG-01` 的适用性钉住（产品，合同 `SYN-CONTRACT-01`，
   法人 `SYN-LE-01`，范围）四维；合同正文绑财务控制策略 `SYN-FIN-CONTROL-01`（预付适用、
   到付显式不适用）。第二个产品 `SYN-PROD-CN-SG-ECON`（合成经济线）用来撑渠道绑定的另一格：
   EXPRESS 的映射 `SYN-MAP-CN-SG-EXPRESS-01` 配 `SYN-CH-SG-POST-STD` 与 `SYN-CH-AGG-SEA-01`
   两个渠道标识引用，ECON 的映射 `SYN-MAP-CN-SG-ECON-01` 显式登记「未配置」——渠道接入后
   以新修订配置绑定，历史修订保留。
2. **配价**（parcel-pricing）：售价卡 `SYN-PLAN-CN-SG-01`（首重 0.5kg ¥55 续重 ¥18/0.5kg，
   Z1/Z2 两区，MAX 计费重体积系数 5000）方向授权引商业授权对象 `SYN-AUTH-PRICE-DIR-01`，
   方案结构绑燃油序列 `SYN-SERIES-FUEL-01`；成本卡 `SYN-PLAN-CN-SG-COST-01`（BUY）引
   `SYN-AUTH-COST-DIR-01`；汇率序列 `SYN-SERIES-FX-CNY-SGD` 的口径引价格规则
   `SYN-PRICE-RULE-CN-SG`（汇率不收裸值）。
3. **一单会走的网**（network-routing）：上海枢纽→深圳口岸→新加坡枢纽→新加坡末端四节点
   三连接成线路 `SYN-LINE-CN-SG-01`（applicable_scope 同 `SYN-SCOPE-01`）；上海枢纽 v1→v2
   换版展示版本轴；一次台风停运（SUSPENSION，已解除）展示临时调整与稳定定义分离。
4. **过关的规则**（customs-compliance）：中国出口侧就绪+提交授权、放行结果解释规则 v1→v2
   换版、案件 `SYN-CASE-CN-SG-01` 的关闭义务（一项已了结、一项承接给 `SYN-BROKER-01`）、
   跨关区移动门禁（前置条件已满足）、建案要求两向（CN 出口要求建案、SG 进口显式不要求）；
   合规候选口岸 `SYN-PORT-SZX-01`（v1→v2 换版）与 `SYN-PORT-SIN-01`，申报路径
   `SYN-PATH-CN-EXPORT-01`（经 SZX、EXPORT、舱单模式）与 `SYN-PATH-SG-IMPORT-01`
   （经 SIN、IMPORT、正式模式）——路径以标识引用口岸，两册对照属读侧（票 03 裁量）。

## 已知边界（如实记录，不是缺陷）

- `commercial_price_policy`（0010）的持久化面存在，但**没有进程级写入口**（`SavePricePolicy`
  无 cmd 调用方）。商业策略页的价格政策列因此如实为空——按 ADR-0077 空册本身就是内容；补写
  入口属机制半边，不归种子票。价格政策还多一道：`RehydrateAdoptedBasisSpec` 的快照重建至今
  缺席，补发布通道不等于补重建（记于票 `commercial-closure-settlement-key/02`）。
  `service_product_form`（0008）那半边已经补上（票 `admin-remainder-mechanism-batch/02`）：
  `register-products` 子命令是 `SaveServiceProduct` 的进程级调用方，本包两个产品的形态随
  种子落册，服务产品页的形态列不再为空。
- 结算政策那一格已经补上（票 `commercial-closure-settlement-key/02`）：发布批里的
  `SYN-SETTLEMENT-PREPAID-01` 是本包唯一一份结算约定，六维与解析键那三维加闭包解出的合同
  版本严丝合缝——差一维就不再被采用，本上下文不许借宽泛客户关系跨维归集。
- 七页真数据展示还依赖查询端点的目录 Intake 配置（PAR-INT-01 未决期间装配
  UnconfiguredIntake，生产路径 403 是刻意的）；本包只负责库内数据态，页面接线归票 07。
