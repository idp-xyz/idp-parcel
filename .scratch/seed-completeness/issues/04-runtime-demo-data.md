# 04 运行时演示数据：只走到已提交

Category: enhancement
Status: resolved
Blocked by: none
地盘：`scripts/demo-seeds/submit-one-shipment.sh` 与本包 README 的已知边界。不改 `seed.sh` 默认步骤，不改命令面，不 INSERT。
出处：派单 `task-1084a20d` ← 通道 1。用户裁的是只做步 0。

## 裁决

演示走隔离写，不等 `PAR-INT-01`。步 0 就够：一笔委托停在`已提交`。轨迹和对账单不在本票。价格政策正文归通道 2。接管保持未开。回汇批次归通道 3。

## 读面核对（先于脚本）

一次性库 `127.0.0.1:55444`，`seed.sh --reset` 之后：

- 只设 `IDP_PARCEL_ISOLATED_WRITE_TENANT=SYN-TENANT-01`：`POST /shipment-requests` 答 `201` `SUBMITTED`；`GET /shipment-request-views` 的列表与详情都答 `403` `ACCESS_CHANNEL_NOT_CONFIGURED`。提交落库之后再查，仍是 403。
- 读、写都设为 `SYN-TENANT-01`（与 `parcel.sh` 相同）：同一笔列表答 `200` `LISTED`、详情答 `200` `REQUEST_VIEW`，状态都是 `SUBMITTED`。轨迹投影答 `200` `PROJECTIONS_LISTED` 且列表为空。

读开关不开时查阅面仍是未配置。脚本不换入口、不写库去绕过这道 403。

## 脚本

`IDP_PARCEL_API_BASE` 指向已按上面两个开关拉起的 API。每次用新的 `SYN-CUSTREF-` / `SYN-PCLREF-` 时间戳，避免同键重放。断言提交 `201`/`SUBMITTED`，再断言列表与详情都读到这一笔且状态为 `SUBMITTED`。任一口 403 即失败退出。

## 验证

脚本提交 `92586f8f`。一次性库 `127.0.0.1:55444`（容器用完即删，未碰 `55432`）上 `seed.sh --reset` 之后，读写真开，`IDP_PARCEL_API_BASE=http://127.0.0.1:19084` 跑脚本退出 0：`已提交 SHR-IS5W4LKRV23DTVEWEOVLX34TF4`，来源键 `SYN-CUSTREF-20260929T044814Z`，列表与详情状态都是 `SUBMITTED`。`seed.sh` 不调用它。
