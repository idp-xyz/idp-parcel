// Package productionhandoff 放 parcel-shipment 面向他方生产权威的出向通道适配器。
//
// 它单独成包而不并入 pilotgovernance：那一包翻译的是治理登记册的读口（答「这一范围归谁」），
// 本包对着那个「谁」交范围（ADR-0128 决定一）——两种证据、两个对方，住在一起会让治理读口的
// 适配器被迫实现一个它答不了的方法。
package productionhandoff

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// UnconfiguredOtherProductionAuthorityChannel 是出向通道在实例半边到位之前的如实答复：对每一次投递
// 不看范围、不看对方是谁，什么都不发出去，一律答`通道未配置`，不带任何对方给的引用（ADR-0128 决定三）。
//
// 通往他方权威的协议、身份与交接证据是 `PAR-GOV-05..07` 的实例半边，没有租户就没有通道。它不答
// `查询不可用`——那一格的证据形要求对方已经给过确认引用，而这里根本没投递；也不交回 error——error
// 那格的恢复动作是重试，重试改不了一条还没人配置的通道（ADR-0063 对「显式未配置」的分界）。编排据以
// 经 AssessSafeHandoff 落「生产归属未决」并给出续办引用，不冒充成功。真通道就位时在装配点替换，
// 本类型随之退场。
type UnconfiguredOtherProductionAuthorityChannel struct{}

var _ ports.OtherProductionAuthorityChannel = UnconfiguredOtherProductionAuthorityChannel{}

// DeliverAdmissionScope 不读投递。参数刻意匿名：连签名都不给「看一眼范围或对方再决定」留位置。
func (UnconfiguredOtherProductionAuthorityChannel) DeliverAdmissionScope(
	context.Context,
	ports.ProductionHandoffDelivery,
) (ports.ProductionHandoffObservation, error) {
	return ports.ProductionHandoffObservation{Observation: domain.HandoffObservationChannelUnconfigured}, nil
}
