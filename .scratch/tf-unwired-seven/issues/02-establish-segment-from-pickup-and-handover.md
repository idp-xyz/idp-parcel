# 两条立段入口接进现有两例编排

Category: enhancement
Status: draft
Blocked by: 01

## 要接的两条

- `EstablishSegmentWithPickup`（自注「由首个对象的有效收寄成立段（CONTEXT 生命周期①）」）
  → 接进 `application/register_offsite_pickup.go` 与/或 `perform_offsite_pickup.go`
- `EstablishSegmentWithHandover`（自注「由首个对象的『已交接』权威交接成立段」）
  → 接进 `application/register_transport_handover.go`

两例编排今天都在造领域对象（`FormOffsitePickup` / `FormTransportHandover`），只是不立段。

## 三处必须想清楚的

**一、首个对象与后续对象走不同的路。** 两个函数名里都写着「由**首个**对象成立段」，后续对象
是 `JoinWith*` 加入既有段。所以编排要先判「这个范围的段是否已成立」——**这一判要落在段登记册
的取回上，不能靠编排自己记**。

**二、交接三裁决里只有「已交接」立段。** CONTEXT 与既有交接登记编排都把三裁决分开
（已交接 / 拒收 / 待确认），而`拒收`与`待确认`**不给转出引用**。立段同理：只有`已交接`能成立
段，另两格不成立也不报错——那是正当的业务结果，不是失败。

**三、这一步不能让登记失败。** 收寄登记与交接登记本身是控制事实的保全，CONTEXT 把接货时间
称为「责任起点锚」。**立段失败不得回滚登记**——否则一次段登记故障会抹掉一条已经发生的物理
事实。处置形状照本仓既有纪律：来源保全一侧的失败上抛，派生一侧的失败形成本上下文自己的
未决结果并带续办引用。

## 陷阱

- 别把「段已成立」缓存在编排里。并发两个对象同时到达时，那个缓存就是一次竞态。
- 别用「登记成功即立段」的隐式耦合表达 CONTEXT 那条边界；边界的判断要显式，因为 CONTEXT
  专门列举了七种**不能**替代它的单项事实。

## 完工判据

`EstablishSegmentWithPickup` 与 `EstablishSegmentWithHandover` 出现在非测试生产调用路径上，
棘轮基线两行可剪；剪前按基线要求核全仓同名声明。
