# 17 TF 的交接与外场取件两类事实没有在线登记口

Category: enhancement
Status: resolved——`03` 已裁定不走这条路，本票判定**不做**，依据见文末
Blocked by: 03（已 resolved）

## 缺口

`cmd/parcel-api/endpoints.go` 里 `transport-fulfillment` 的命令端点只有两个（实测于 `9e6d53a`）：
`/transport-fulfillment/deliveries` 与 `/transport-fulfillment/delivery-proof-corrections`。

而 TF 侧产出 VE 事实的译装器有四个：`derive_on_transport_handover.go`、
`derive_on_offsite_pickup.go`、`derive_on_effective_delivery.go`、`derive_on_exception_journey.go`。
对应的登记用例 `register_transport_handover` 与 `register_offsite_pickup` **没有在线口**——
它们今天只能由内部路径或受控进程调用。

若 `03` 裁定外部轨迹译成 TF 的交接或外场取件事实，**这两类事实进不来**：收编执行器要么无口可用，
要么被迫绕过用例直插仓储。

## 为什么被 `03` 阻塞

**这一票不一定要做。** 若 `03` 裁定收编走别的路径（例如另立一类渠道轨迹事实、或走受控进程
而不进端点表），那么这两个在线口就没有必要，补了就是给写面凭空多两个入口。
**先裁再决定做不做**，是本票存在的全部理由。

## 做什么（若 `03` 裁定需要）

照登记写面的既有形状补两个登记口——`XxxRegistrationIntake` 接口 + 端点构造函数、
`UnconfiguredIntake` 补实现、传输层测试含「隔离读 Intake 装不进登记口」的编译期断言、
端点表挂字面量 `UnconfiguredIntake{}`、生产装配接真（登记用例 + `db.Transactor()` 事务包装）。
形状照 [ADR-0085](../../../docs/adr/0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)
与票 `admin-write-faces/02` 已落的十七个登记口，**不重新裁**。

## 红线

- 写准入不另立形：隔离读准入（ADR-0078）不得扩到写行。
- 复用既有登记用例，不为在线口另造一套受理逻辑。
- 不填任何实例取值。

## 完成判据

若判定要做：两个在线口四件齐，装配测试钉住未配置态 403 与隔离读启用态仍 403；
`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库。
若判定不做：在票面写明依据并置 `resolved`，**不留一张永远 draft 的票**。

## 参照

[轨迹源盘点](../tracking-source-seam-inventory.md)第二段；票 `03`；ADR-0085。

## 判定：不做

`03` 裁定末端渠道轨迹译成 TF **新立**的一类「外部承运轨迹事实」，不复用既有自营作业事实
类型。本票上面「为什么被 `03` 阻塞」那一节写的正是这个分支：

> 若 `03` 裁定收编走别的路径（例如另立一类渠道轨迹事实、或走受控进程而不进端点表），
> 那么这两个在线口就没有必要，**补了就是给写面凭空多两个入口**。

该分支成立，因此本票按自身完成判据置 `resolved`，不留一张永远 `draft` 的票。

**要说清这不等于「TF 那两个口不该有」。** 缺口本身是真的——`register_transport_handover`
与 `register_offsite_pickup` 今天确实没有在线登记口，这一条实测于 `9e6d53a` 仍然成立。
不做的理由只有一个：**本 feature 不是它的成因**。面单渠道服务用不到这两个口，为它补口
属于「顺手」，而顺手补出来的写面入口没有任何用例在守。真要补，应由 TF 自己的切片按
[ADR-0085](../../../docs/adr/0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)
立票，判据是那两类事实自己需不需要在线登记，不是这里需不需要。

缺口的登记处仍在[轨迹源盘点](../tracking-source-seam-inventory.md)第二段与其汇总表，
本票 `resolved` 不抹掉那条记录。
