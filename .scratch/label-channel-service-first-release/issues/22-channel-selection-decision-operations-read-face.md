# 22 渠道择优决定的运营查阅面：留痕落库了，运营今天没有地方看它

Category: enhancement
Status: draft——读面形状待按 ADR-0077 读面通例裁（读口另立、不拓宽登记册端口）；MCP-2 2026-09-04 随票 14 收口立票，只写票面未动代码
Blocked by: 14

## 缺口

票 [14](./14-rejected-candidate-trace-object.md) 把「渠道择优决定」记成只追加的记录（领域对象、
`ports.ChannelSelectionDecisionRegistry`、`parcel_shipment.channel_selection_decision` 头行 +
`channel_selection_candidate` 子行、择优编排落定后同事务写入）。它今天只有**写**与一个按对象列
历史的读口（`ListBySubject`，给同一对象的择优历史用）——**没有任何运营面能看见它**：哪些择优停在
并列冲突等人工裁决（`PAR-NET-16`「交人工裁决」那一格）、某个候选最近为何总是出局、某笔映射下
这一天做过几次择优。

票 14 的裁决原句：「谁读它：运营查阅面，沿 ADR-0077 读面通例另立读口与票；本票只落登记。」
本票就是那张票。

## 做什么

按 [ADR-0077](../../../docs/adr/0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)
读面通例：

1. **读端口另立**（`ports` 下伴生读端口，不拓宽 `ChannelSelectionDecisionRegistry`——理由同
   `internal/parcelpricing/ports/evaluation_read.go` 头注：扩写既有接口会拆全部替身）。至少两口：
   - 按租户列**并列冲突待人工**的决定（`conclusion = TIED`），按决定时刻倒序、可按对象收窄；
   - 按（租户 + 决定标识）取一条决定的逐候选结果（含出局因由与所用评价引用）。
2. **postgres 读适配器**只读列面（头行 + 子行），不在 SQL 里解释四格之外的任何语义；读回照旧过
   `RehydrateChannelSelectionDecision`。
3. **端点**进 `cmd/parcel-api/endpoints.go`（共享接线文件，先在频道占号）+ 隔离读放行表按
   ADR-0078 判是否入格。
4. **管理台**页：落点先取证——面单交易/包裹侧读面今天有哪几页、择优对象引用（商业范围 + 产品—渠道
   映射）在哪一页显得出来，登在哪里看得见就摆哪里（票 admin-write-faces/02 「写签跟着读签走」同一条
   判据反过来用）。

## 先答再开工

- **并列冲突的人工裁决动作要不要同票**：裁决人与裁决规则属实例半边（`PAR-NET-16` 待提供列「并列时的
  裁决规则和授权」），机制侧「人工裁决」怎么落——是形成一条新的决定记录（规则引用换成人工裁决那一格）
  还是别的形状——**要先 `/domain-modeling`**，不在读面票里顺手定。本票倾向：读面只列冲突，不给动作。
- **保留期与读面**：`PAR-NET-16` 留痕要求待提供，读面不做任何按时间的隐式截断；「近期」之类窄口由
  查询参数显式给。

## 红线

- 不拷候选内容与金额进读面：金额要看去 `parcel-pricing` 的评价（读面只透评价引用）。
- 不填任何实例取值；SYN 夹具只记 `S`。
- 不开任何行级 UPDATE/DELETE；读面对登记册零写入。

## 完成判据

两口读端口有 postgres 实现与真库用例（含租户隔离与「从未择优过」答空）、端点进表且未配置态按
ADR-0055 作答、管理台页面能列并列冲突并展开逐候选结果；`gofmt -l` 空、`go build`/`go vet` 退 0、
`go test -count=1 ./...` 绿并注明含不含真库。

## 参照

票 [14](./14-rejected-candidate-trace-object.md) 裁决节；`internal/parcelshipment/ports/channel_selection_decision.go`；
`migrations/parcel_shipment/0015_channel_selection_decision.sql`；[ADR-0077](../../../docs/adr/0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)；
参数登记册 `PAR-NET-16`。

## Comments

- 2026-09-04 · MCP-2：立票。起因是票 14 裁决把读面划出登记票之外，收口时按裁决原句另立。
  **只写票面，未动代码。**
