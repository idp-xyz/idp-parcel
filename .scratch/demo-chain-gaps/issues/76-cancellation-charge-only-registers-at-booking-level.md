# 76 取消费只认订舱一级：整份委托的取消、未订舱的取消没有登法，部分接受的订舱不核数量

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [28](./28-booking-and-cancellation-occurrences-and-procurement-context-from-adoption.md) 完成记录与 [ADR-0207](../../../docs/adr/0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md) 文末补记越权风险点 8、9；回答 [ADR-0209](../../../docs/adr/0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md) 越权风险点 2 的那一半在票 28 里定为「不登」，本票是它的去处。与 [58](./58-adoption-does-not-check-commission-members-against-segment-objects.md)（委托取消只有整份、不往下游传）、[37](./37-confirmed-booking-capacity-relationship.md)（确认订舱的容量关系）相邻。
Blocked by: 用户决定（TF、PC owner 复核：租户协议对整份委托的取消、对尚未订舱的取消要不要收费；部分接受的订舱按订舱计时数量怎么认）

## 现象

票 28 把取消发生项的事实依据定为「尚未开始范围的订舱取消」：只认订舱一级，订舱已取消、取消之前已被承运方接受、按业务时间取消早于委托开始（或订舱提出于开始之后）。于是：

- 一份委托还没有任何订舱就被取消，取消费没有登法：命令只收订舱，委托一级的取消没有事实路径。ADR-0209 给取消配的数量形只有按订舱与按件，整份委托也没有一形配得上。
- 委托取消不往下游订舱传（ADR-0204 决定三），委托下的舱位要运营人员经取消订舱口逐个停掉，取消费也只能逐次订舱登记。
- 订舱费按订舱计时，部分接受的订舱照样计一项；按件计时成员由登记方指名，不核成员件数与接受量是否相符。

## 要裁的

- 协议对「委托取消」「未订舱的取消」收费是不是真实租户会有的条款；有的话，依据取委托取消（新的依据前缀）还是要求先有订舱。
- 委托取消的数量形：在 PC 的配表里给取消加一形（例如按委托），还是只允许按件、成员取委托成员。
- 部分接受：按订舱计的一项是否要带接受量；按件计时成员件数要不要不超过接受量（与票 37 逐订舱的消费关系一并考虑）。

## 三类

判据与配表是产品策略（PC 配表改动要换规范化版本，ADR-0014）；某个租户协议收不收这几种费是租户取值。
