# 13 access-identity 操作者与工作设备两册迁到 ADR-0144

Category: enhancement
Status: ready-for-agent——迁移编号预留 access-identity `0041`（进 main 时按 tip 重核）
Blocked by: 01
地盘：`internal/accessidentity` 里 `ListOperators`、`ListDevices` 的端口、postgres 实现与两个端点及其测试；`migrations/access_identity` 的一条新迁移（索引）。
出处：ADR-0144 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（钉 `a1274a6b8`）

| 路由 | 读方法 | 今天 |
|---|---|---|
| `/operators` | `OperatorCatalogue.ListOperators` | 不设上限，一条查询取全部操作者连同授予与撤销（ADR-0363 决定二） |
| `/work-devices` | `DeviceCatalogue.ListDevices` | 不设上限 |

这两册今天的问题与其他各册相反：不截断，而是一次全取；租户上了量（快递员、场站人员与手持设备都以千计）时，一页就是全量。

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：

- 开工先读 ADR-0363 决定二：分页后仍须一条查询取「本页操作者连同其授予与撤销」，绑定与授予不取自两个时点；写不通就回 ADR 改，不在代码里另立口径。
- 这两口今天没有 `limit`：页大小按操作者渠道契约取，与各上下文操作者 Intake 同值，写明出处。
