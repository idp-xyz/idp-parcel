package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 关务那一问没有形成本上下文认得的答复时，那一件记读不动，依据种类与出发、交出两侧同一套词（read_departure_gate.go 头上的
// departureGateCustoms* 常量）：救法相同，读者不必为交付另学一套。

var errDeliveryGateViewMisanswered = errors.New("transport fulfillment: the delivery gate view answered for another object")

// DeliveryGateReadDeps 是交付门禁读数要问的几口：派送尝试读面（交付的业务时间）、段登记册与交接登记册（at 取哪个节点），
// 以及关务的动作准入判断。
type DeliveryGateReadDeps struct {
	Attempts  ports.DeliveryAttemptView
	Segments  ports.DeliveryGateSegmentView
	Handovers ports.DeliveryGateEntryHandoverView
	Customs   ports.DeliveryGateView
}

// DeliveryGateReader 为一次有效交付形成交付门禁读数，供 RegisterWithGateReading 与交付同一笔落册（ADR-0213 决定五）。
//
// 它由组合根在写事务之前、用外层 ctx 调：关务的判断在它自己的事务里记下（ADR-0206 决定六），嵌进交付登记那一笔会随它
// 回滚；读在池上，不在写事务里，任何一口读不动都只是读数里的一格，挡不住交付落册。
type DeliveryGateReader struct {
	deps DeliveryGateReadDeps
}

// NewDeliveryGateReader 构造期拒缺件：漏装的一口到第一次交付才暴露，会被记成一格读不动，把装配缺陷演成依赖故障。
func NewDeliveryGateReader(deps DeliveryGateReadDeps) (*DeliveryGateReader, error) {
	if deps.Attempts == nil || deps.Segments == nil || deps.Handovers == nil || deps.Customs == nil {
		return nil, errors.New("transport fulfillment: the delivery gate reader needs the attempt view, the segment and handover registries and the customs view")
	}
	return &DeliveryGateReader{deps: deps}, nil
}

// ReadRegistration 为一条首登命令问读数。只为妥投问：受理不了、尝试或对象结果不在册、结果不是妥投，都交回零值读数，由
// 登记编排照常答（未受理、未生效）——那几种登不出交付，没有交付可读。
//
// 写事务之前读派送尝试读不动时，不知道这次交付的业务时间，读数停在「派送尝试读面读不动」：写事务里仍读不动，编排答未决、
// 用不上它；写事务里读得动，交付照登、记这一格。
func (reader *DeliveryGateReader) ReadRegistration(
	ctx context.Context,
	command RegisterEffectiveDeliveryCommand,
) (domain.DeliveryGateReading, error) {
	key, accepted := deliveryKeyFrom(command.TenantID, command.Attempt, command.Object)
	if !accepted {
		return domain.DeliveryGateReading{}, nil
	}
	_, result, found, err := reader.deps.Attempts.LoadDeliveryResult(ctx, key.TenantID, key.Attempt, key.Object)
	switch {
	case err != nil:
		return domain.FormDeliveryGateReading(domain.DeliveryGateReadingSpec{Stop: domain.DeliveryAttemptViewUnavailable})
	case !found || !result.Outcome().Succeeded():
		return domain.DeliveryGateReading{}, nil
	}

	node, stop, err := reader.deliveryNode(ctx, key.TenantID, key.Object)
	if err != nil {
		return domain.DeliveryGateReading{}, err
	}
	if stop != domain.DeliveryGateStopInvalid {
		return domain.FormDeliveryGateReading(domain.DeliveryGateReadingSpec{Stop: stop})
	}

	finding, err := reader.deps.Customs.ReadDeliveryGate(ctx, ports.DeliveryGateQuery{
		TenantID: key.TenantID,
		Object:   key.Object,
		Node:     node,
		At:       result.OccurredAt(),
	})
	if err != nil {
		cause := departureGateCustomsUnavailable
		if errors.Is(err, ports.ErrDeliveryGateAnswerUntranslatable) {
			cause = departureGateCustomsUntranslatable
		}
		if finding, err = domain.NewGateFinding(key.Object, domain.GateUnreadable, "",
			[]domain.GateBasis{{Cause: cause}}); err != nil {
			return domain.DeliveryGateReading{}, err
		}
	}
	if finding.Object() != key.Object {
		// 端口答的是另一件、或交回零值却不报错：那是实现坏了，不是关务读不动，记成一格只会把它藏起来。
		return domain.DeliveryGateReading{}, fmt.Errorf("%w: the customs view answered for %q when asked about %q",
			errDeliveryGateViewMisanswered, finding.Object(), key.Object)
	}
	return domain.FormDeliveryGateReading(domain.DeliveryGateReadingSpec{Node: node, Finding: finding})
}

// deliveryNode 答交付意图的 at：对象此刻在场的那一段（就是这次交付要结束的那一段，结束参与按同一问找它），它凭哪次
// 交接进的这一段，那次交接声明的交出节点（ADR-0213 决定五）。取不到节点时交回停点、不问关务。
//
// 不看段是否声明为派送段：交付已经从这一段发生了，段的服务动作只决定派送任务从哪儿派生（ADR-0114），不改对象从哪个
// 节点出去这件事。对象同时在几段里在场是库面不一致，结束参与那一半同样会拒（ErrObjectActiveInSeveralSegments），这里
// 先一步响亮报错，不挑一段去问。
func (reader *DeliveryGateReader) deliveryNode(
	ctx context.Context,
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
) (domain.NetworkNodeReference, domain.DeliveryGateStop, error) {
	none := domain.NetworkNodeReference{}
	active, err := reader.deps.Segments.FindActiveSegments(ctx, tenant, object)
	if err != nil {
		return none, domain.DeliverySegmentRegistryUnavailable, nil
	}
	switch len(active) {
	case 0:
		return none, domain.DeliveryNoActiveSegment, nil
	case 1:
	default:
		return none, domain.DeliveryGateStopInvalid, ErrObjectActiveInSeveralSegments
	}
	record, found, err := reader.deps.Segments.FindByKey(ctx, active[0])
	if err != nil {
		return none, domain.DeliverySegmentRegistryUnavailable, nil
	}
	participation, present := record.Segment.ParticipationFor(object)
	if !found || !present || !participation.Active() {
		// 两问之间参与刚被结束或替代：此刻已不在场，按「此刻不在任何段里」答，不拿刚离场的那一段顶。
		return none, domain.DeliveryNoActiveSegment, nil
	}
	switch participation.EntryKind() {
	case domain.EnteredByOffsitePickup, domain.EnteredByCarrierFirstEffectivePickup:
		return none, domain.DeliveryEnteredByPickup, nil
	}
	version, named := participation.EntryHandoverVersion()
	if !named {
		return none, domain.DeliveryEntryHandoverUnresolved, nil
	}
	handovers, err := reader.deps.Handovers.FindHandedOverByVersion(ctx, tenant, object, version)
	if err != nil {
		return none, domain.DeliveryHandoverRegistryUnavailable, nil
	}
	var entry domain.TransportHandover
	matched := 0
	for _, handover := range handovers {
		if participation.EnteredByHandover(handover) {
			entry, matched = handover, matched+1
		}
	}
	if matched != 1 {
		return none, domain.DeliveryEntryHandoverUnresolved, nil
	}
	node, declared := entry.ReleasingNode()
	if !declared {
		return none, domain.DeliveryReleasingNodeNotDeclared, nil
	}
	return node, domain.DeliveryGateStopInvalid, nil
}
