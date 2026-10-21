# 152 到期重问没有进程内的驱动：登了重问间隔，也要有人去按

Category: enhancement
Status: needs-triage——2026-10-21 票 [70](./70-delivery-dispatch-left-unformed-has-no-trace.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [ADR-0230](../../../docs/adr/0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 决定五末项与越权风险点 9。
Blocked by: 无

## 现象

- 票 70 立了派送待形成册、按册重问与租户的重问间隔册（ADR-0230 决定一、四、五）。到期重问（`POST /transport-fulfillment-delivery-dispatch-reasks`，`{"scope":"DUE"}`）按间隔筛出该问的那几件，可它只在运营人员按的时候发生。
- 租户登了间隔，也没有任何东西按间隔自己去问；所有者补齐之后，在有人按之前任务不会形成。

## 要做的

- 在派发进程（或另一个常驻进程）里加一个驱动：按部署节拍逐租户调到期重问那一个用例（`ReaskPendingDeliveryDispatchesHandler`，`DUE`）。
- 三类：
  - 驱动本身是机制；
  - 驱动多久醒一次是部署参数，照 `IDP_PARCEL_DISPATCH_INTERVAL` 的纪律，必填、不给默认；
  - 哪一件该问由租户的间隔定（PAR-NET-19），间隔未登记的租户驱动答未配置、跳过它。
- 要裁的：
  - 驱动以什么身份调。它不是管理台上的人，运营决定的授予与准入不适用；进程内直调应用用例，还是另立一种机器身份。
  - 要不要逐租户限量。
  - 驱动失败怎样留痕。
- 地盘：要动 `cmd/parcel-dispatch` 的 main 与装配（ADR-0222 装配写法下，那两个文件不是特性文件），票 70 因此没做。

## 规模

S–M
