package ports

import "context"

// 移动事实已形成信封的交出口（ADR-0205 决定五）。另立文件而不并进 movement_fact.go：那一份只管登记册的存取，交出口
// 是另一个关注点，分开也免得共享树上改登记册的人与改交出的人碰同一个文件。

// MovementFactIntent 把一个已登记的移动事实版本交给消费方。意图由事实键认领：重放重发同一份，由入队那一步按信封 ID
// 吞掉（ADR-0043）。
type MovementFactIntent struct {
	Record MovementFactRecord
}

// MovementFactHandoff 由 OutboxMovementFactHandoff 实现：意图与事实同一事务入队。写口按框架合同无事务即拒，事务边界
// 归装配点；两者分两笔，就会有一条出发落了册而全程追踪永远等不到它，阻断读数也就派生不出异常信号。
type MovementFactHandoff interface {
	HandOffMovementFact(ctx context.Context, intent MovementFactIntent) error
}
