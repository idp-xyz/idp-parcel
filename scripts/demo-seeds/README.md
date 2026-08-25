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
版本行撞主键会答未决（3）——对已灌过的库重跑请用 `--reset`。

## 组成

| 目录 | 入库通道 | 内容 |
|---|---|---|
| `data/commercial/` | `cmd/parcel-commercial publish` | 发布批 8 项：服务产品、财务控制策略、接单规则包（五类规则正文+受理内容+收寄资格+时点锚+终局规则）、客户合同（正文+受理前控制）、价格规则、三条授权规则 |
| `data/pricing/` | `cmd/parcel-pricing-register` | 两张价卡（SELL 首重续重 / BUY 重量段）+ 两条参考序列（燃油、汇率）——由 `seedgen` 生成，勿手改 |
| `data/network/` | `cmd/parcel-network-register` | 七族 14 行：4 节点（含一次换版）、3 连接、1 线路、2 服务区、1 日历、1 台风停运调整、1 路由策略 |
| `data/customs/` | `cmd/parcel-customs-register` | 六册 11 份：就绪、授权、解释规则（含一次换版）、义务目录+两项（已了结/已承接）、门禁目录+判断、建案要求两向（要求/显式不要求） |
| `migrate/` | — | 迁移助手（`migrate.Run` 的隔离环境入口；迁移计划刻意没有生产入口） |
| `seedgen/` | — | 计价快照生成器：价卡与序列的登记输入带规范化版本号与内容摘要自校，必须经真领域构造函数折装；PPC/PRS 规范化版本升级时重跑并提交新产物 |

## 数据故事（同一合成租户 SYN-TENANT-01，范围 SYN-SCOPE-01）

「建产品 → 配价 → 一单的一生」的主数据半边，跨上下文互引全部指名：

1. **建产品**（party-commercial）：服务产品 `SYN-PROD-CN-SG-EXPRESS`（中国→新加坡合成快递）
   携待路由许可；接单规则包 `SYN-RULEPKG-01` 的适用性钉住（产品，合同 `SYN-CONTRACT-01`，
   法人 `SYN-LE-01`，范围）四维；合同正文绑财务控制策略 `SYN-FIN-CONTROL-01`（预付适用、
   到付显式不适用）。
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
   跨关区移动门禁（前置条件已满足）、建案要求两向（CN 出口要求建案、SG 进口显式不要求）。

## 已知边界（如实记录，不是缺陷）

- `commercial_price_policy`（0010）、`commercial_settlement_policy`（0011）与
  `service_product_form`（0008）三张表的持久化面存在，但**没有进程级写入口**
  （`SavePricePolicy` / `SaveSettlementPolicy` / `SaveServiceProduct` 无 cmd 调用方）。
  商业策略页的价格政策与结算政策两列、服务产品页的形态列因此如实为空——按 ADR-0077
  空册本身就是内容；补写入口属机制半边，不归种子票。
- 七页真数据展示还依赖查询端点的目录 Intake 配置（PAR-INT-01 未决期间装配
  UnconfiguredIntake，生产路径 403 是刻意的）；本包只负责库内数据态，页面接线归票 07。
