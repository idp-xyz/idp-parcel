package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 关务那一问没有形成本上下文认得的答复时，那一件记读不动，依据种类与其余门禁读数同一套词（departureGateCustoms* 常量）：
// 救法相同，读者不必为交付另学一套。

var errDeliveryGateViewMisanswered = errors.New("transport fulfillment: the delivery gate view answered for another object")

// DeliveryGateReadDeps 是交付门禁读数要问的几口：派送尝试读面（交付的业务时间）、段登记册与交接登记册（at 取哪个节点），
// 以及关务的动作准入判断。
type DeliveryGateReadDeps struct {
	Attempts  ports.DeliveryAttemptView
	Segments  ports.DeliveryGateSegmentView
	Handovers ports.EntryHandoverView
	Customs   ports.DeliveryGateView
}

// DeliveryGateReader 为一次有效交付形成交付门禁读数，供 RegisterWithGateReading 与交付同一笔落册（ADR-0213 决定五）；
// 派送出发前的交付门禁问询（PreviewDeliveryGateHandler，ADR-0238）走同一条路，读数交回设备、不落册。
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
// 登记编排照常答（未受理、未生效）——那几种登不出交付，没有交付可读。受理不了要连方式、接收方与 POD 一起认（与登记编排
// 共用 deliverySpecFrom）：只认键就去问，缺 POD 那种必答未受理的命令也会让关务记下一笔判断，而这次交付从没落册。
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
	if _, badInput := deliverySpecFrom(command); badInput {
		return domain.DeliveryGateReading{}, nil
	}
	_, result, found, err := reader.deps.Attempts.LoadDeliveryResult(ctx, key.TenantID, key.Attempt, key.Object)
	switch {
	case err != nil:
		return domain.FormDeliveryGateReading(domain.DeliveryGateReadingSpec{Stop: domain.DeliveryAttemptViewUnavailable})
	case !found || !result.Outcome().Succeeded():
		return domain.DeliveryGateReading{}, nil
	}
	return reader.readIntent(ctx, deliveryGateIntent{tenant: key.TenantID, object: key.Object, at: result.OccurredAt()})
}

// deliveryGateIntent 是交付读数从输入里只取的那三样：租户、对象与交付时点；其余取决于此刻的段册、交接册与关务。交付登记的时点
// 取派送尝试里那件对象的结果时刻，交付前问询（ADR-0238）的时点取设备给的拟交付时点；两边收成它之后走同一个 readIntent——
// 「同参同读数」靠的是只有这一条路，不靠两边各写一遍再比对。
type deliveryGateIntent struct {
	tenant domain.TenantID
	object domain.CarriedObjectReference
	at     time.Time
}

// deliveryGateIntentOf 是交付前问询把输入收成 deliveryGateIntent 的那一处。租户与对象的判据与交付登记的键同一处
// （deliveredObjectFrom）：同一件对象在两口受理与否一致。时点按登记精度为零即不受理——不拿服务端时钟补（ADR-0023）。
func deliveryGateIntentOf(tenant domain.TenantID, object string, at time.Time) (deliveryGateIntent, bool) {
	if domain.AtRegistryPrecision(at).IsZero() {
		return deliveryGateIntent{}, false
	}
	objectRef, accepted := deliveredObjectFrom(tenant, object)
	if !accepted {
		return deliveryGateIntent{}, false
	}
	return deliveryGateIntent{tenant: tenant, object: objectRef, at: at}, true
}

// readIntent 先取交付意图的 at（deliveryNode），取不到就停在本侧、不问关务；取到了才问关务那一件。error 只留给读数自己拼不
// 出来或读口答错了对象（实现坏了）。
func (reader *DeliveryGateReader) readIntent(ctx context.Context, intent deliveryGateIntent) (domain.DeliveryGateReading, error) {
	node, stop, err := reader.deliveryNode(ctx, intent.tenant, intent.object)
	if err != nil {
		return domain.DeliveryGateReading{}, err
	}
	if stop != domain.DeliveryGateStopInvalid {
		return domain.FormDeliveryGateReading(domain.DeliveryGateReadingSpec{Stop: stop})
	}

	finding, err := reader.deps.Customs.ReadDeliveryGate(ctx, ports.DeliveryGateQuery{
		TenantID: intent.tenant,
		Object:   intent.object,
		Node:     node,
		At:       intent.at,
	})
	if err != nil {
		cause := departureGateCustomsUnavailable
		if errors.Is(err, ports.ErrDeliveryGateAnswerUntranslatable) {
			cause = departureGateCustomsUntranslatable
		}
		if finding, err = domain.NewGateFinding(intent.object, domain.GateUnreadable, "",
			[]domain.GateBasis{{Cause: cause}}); err != nil {
			return domain.DeliveryGateReading{}, err
		}
	}
	if finding.Object() != intent.object {
		// 端口答的是另一件、或交回零值却不报错：那是实现坏了，不是关务读不动，记成一格只会把它藏起来。
		return domain.DeliveryGateReading{}, fmt.Errorf("%w: the customs view answered for %q when asked about %q",
			errDeliveryGateViewMisanswered, finding.Object(), intent.object)
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
	entry, recognised := entryHandoverOf(ctx, reader.deps.Handovers, tenant, participation)
	switch recognised {
	case entryByPickup:
		return none, domain.DeliveryEnteredByPickup, nil
	case entryHandoverNotOnRegister:
		return none, domain.DeliveryEntryHandoverNotOnRegister, nil
	case entryHandoverAmbiguous:
		return none, domain.DeliveryEntryHandoverAmbiguous, nil
	case entryHandoverUnreadable:
		return none, domain.DeliveryHandoverRegistryUnavailable, nil
	}
	node, declared := entry.ReleasingNode()
	if !declared {
		return none, domain.DeliveryReleasingNodeNotDeclared, nil
	}
	return node, domain.DeliveryGateStopInvalid, nil
}
