# 04 运行时演示数据：一单的一生（方案，确认前不实现）

Category: enhancement
Status: 步 0 resolved——用户 2026-09-29 裁决只做步 0，步 1/2 不做；步 0 实现随本笔进 main（通道 4 作者，通道 3 非作者评审）。方案文本保留在下面。
Blocked by: 无
地盘：本票只写方案。确认前不改种子、不改命令面、不 INSERT。
出处：派单 `task-b125d095` ← 通道 1。空表范围：`parcel_shipment` 委托侧、`settlement_accounting`、`transport_fulfillment`、`node_operations`、visibility 案件类。

## 结论

今天做不到一条脚本从受理走到对账。能走通的只有「隔离写开关下提交一笔委托」。
其后的路由、运输、轨迹、异常、对账，写侧多数仍是 `UnconfiguredIntake`，或者根本没有登记 CLI，
只有判断发生时才落行。绕过领域 INSERT 被派单禁止，本方案也不建议为演示新开那些写入口。

## 已经成立的前置

- 主数据种子（本包）提供租户、账户、价卡、网络目录、治理权威区间。
- 委托提交的生产路径挂 `UnconfiguredIntake`（`cmd/parcel-api/endpoints.go` 的 `submissionIntake`）。
  客户渠道 `PAR-INT-01` 未决，生产请求答 403，这是刻意的（种子 README「已知边界」）。
- 隔离写开关另接一条提交口。票 `demo-intake-admission-paused/01` 已经用阶段评审把
  「无关暂停保守拦下委托」解开：隔离写开着时，`POST /shipment-requests` 可以答 `SUBMITTED`。
  那条路依赖治理种子里的 `shipment-intake` 权威区间和「互不相干」边，不能拆。

## 分步

### 步 0：只演示到「已提交」（小）

隔离库、隔离写开、现有 `seed.sh`，再调一次真提交用例。不新开机制。
风险：部署参数（隔离开关、治理四维与 `isolatedGovernance*` 逐字相同）错一位，答复就和「没登记」分不清。
工作量：一支只打提交口的脚本加一笔断言，大约一天，且必须在一次性库上跑，不能用 55432。

### 步 1：路由三张运行时表（中，且可能没有调用方）

`initial_route`、`plan_applicability`、`reachability_judgment` 没有登记 CLI。
它们要等一次真实的路由判断。提交成功不等于这三张表有行——要先核对受理之后有没有进程把判断跑完。
没有的话，这是主链接线缺口，不是种子缺口。本方案不建议为演示补一个「登记一笔初始路由」的 CLI。

### 步 2：运输、节点作业、轨迹、异常、对账（大，每段一张票）

这些上下文的命令面大量仍挂 `UnconfiguredIntake`。每一段都要先有上一段交出的身份
（委托、运段、案件、费用），再决定是换真 Intake 还是继续空。
空着是 ADR-0077 下的如实内容，不是种子忘了灌。

## 用户必须先裁的

1. 演示走隔离写，还是等 `PAR-INT-01` 客户渠道？后者今天做不了提交。
2. 步 0 就够，还是必须看见轨迹和对账单？后者要先接受步 1、步 2 各自独立开票，不能在本票里做完。
3. 价格政策正文归通道 2 的商业发布批（见票 03），不放进运行时脚本。
4. 接管记录保持未开（ADR-0128 决定五）。回汇批次归通道 3。

（以上「用户必须先裁的」已裁：第 1 项走隔离写；第 2 项只做步 0；第 3、4 项如所列。）

## 裁决

演示走隔离写，不等 `PAR-INT-01`。步 0 就够：一笔委托停在`已提交`。轨迹和对账单不在本票。价格政策正文归通道 2。接管保持未开。回汇批次归通道 3。

## 读面核对（先于脚本）

一次性库 `127.0.0.1:55444`，`seed.sh --reset` 之后：

- 只设 `IDP_PARCEL_ISOLATED_WRITE_TENANT=SYN-TENANT-01`：`POST /shipment-requests` 答 `201` `SUBMITTED`；`GET /shipment-request-views` 的列表与详情都答 `403` `ACCESS_CHANNEL_NOT_CONFIGURED`。提交落库之后再查，仍是 403。
- 读、写都设为 `SYN-TENANT-01`（与 `parcel.sh` 相同）：同一笔列表答 `200` `LISTED`、详情答 `200` `REQUEST_VIEW`，状态都是 `SUBMITTED`。轨迹投影答 `200` `PROJECTIONS_LISTED` 且列表为空。

读开关不开时查阅面仍是未配置。脚本不换入口、不写库去绕过这道 403。

## 脚本

`IDP_PARCEL_API_BASE` 指向已按上面两个开关拉起的 API。每次用新的 `SYN-CUSTREF-` / `SYN-PCLREF-` 时间戳，避免同键重放。断言提交 `201`/`SUBMITTED`，再断言列表与详情都读到这一笔且状态为 `SUBMITTED`。任一口 403 即失败退出。
