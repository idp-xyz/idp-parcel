// Package networkrouting 是 node-operations 消费 network-routing 的适配器位置（ADR-0025 消费方侧）。今天只有方向
// 依据记录的路由依据那一格要问 NR，而 NR 还没有可问的读口，这里因此只有一只显式未配置的口。
package networkrouting

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// UnconfiguredRoutingBasis 是 ports.RoutingBasisView 的显式未配置实现：对任何作业实物都答路由依据`未配置`。
//
// 它不是替身。NR 今天没有路由指令与成员级集运兼容判断的模型，也没有按包裹答当前有效路由的读口（计划适用性按计划
// 版本存取），生产装配要一个诚实的口：跳过这一格就是替 NR 默认「兼容」——红线「不写死为生产默认」禁的正是这个；
// 在本上下文里自比下一节点又越了 NR 的判断。票 no-directional-basis/02 落地那天换成读 NR 的适配器，编排一行不改。
type UnconfiguredRoutingBasis struct{}

var _ ports.RoutingBasisView = UnconfiguredRoutingBasis{}

func (UnconfiguredRoutingBasis) FindRoutingBasis(context.Context, ports.RoutingBasisQuery) (domain.RoutingBasis, error) {
	return domain.NotConfiguredRoutingBasis(), nil
}
