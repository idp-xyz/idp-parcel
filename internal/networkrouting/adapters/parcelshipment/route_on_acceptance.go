package parcelshipment

import (
	"context"
	"errors"
	"fmt"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// ErrEnvelopeContradictsAuthority 说明信封声称已接受，而 parcel-shipment 的委托聚合
// 不这么说（状态不是已接受、没有接受基线，或基线指着另一个提交版本）。
//
// 它响亮上抛而不是静默跳过：静默会把一份该路由的委托悄悄丢掉，而路由义务没有别的
// 东西会来补。上抛让消费门回滚重投——投递因此卡住并看得见，那正是要的：信封与权威
// 长期不一致是数据问题，得有人来看，不该由一次自动跳过掩盖。
var ErrEnvelopeContradictsAuthority = errors.New(
	"network routing parcelshipment adapter: the envelope contradicts the shipment authority")

// ErrRouteHandoffUndecided 说明本轮至少有一个包裹停在未决（依赖读不回、证据换代、
// 标识签不出）。它上抛让消费门整体回滚重投，而不是让投递就此入账。
//
// 这一格是有代价的取舍：回滚会连同本轮已经合法形成的其他包裹计划一起撤掉，而
// `AT-NR-012` 要的是「任一结果不回滚或掩盖其他结果」。取它是因为另一条路更糟——
// 消费门只有提交、回滚重投、拒收三格，就此入账等于把未决包裹的路由义务永久丢掉，
// 而本仓今天没有任何重驱动器会回来捡它。回滚重投不丢东西：判断库按判断键幂等，
// 重投时已成的计划照原样再形成一次，未决的那个再试一次。
var ErrRouteHandoffUndecided = errors.New(
	"network routing parcelshipment adapter: initial route handoff is undecided")

// AcceptedRequestSource 是适配器内部协作者：按完整来源身份取回委托聚合。它由
// parcel-shipment 的委托仓储满足（*pspostgres.ShipmentRequests 的同名方法）——本
// 适配器只声明自己要什么，不导入对方的端口包。
type AcceptedRequestSource interface {
	FindBySourceIdentity(
		ctx context.Context,
		identity psdomain.SourceIdentity,
	) (psdomain.ShipmentRequest, bool, error)
}

// RouteOnAcceptanceAdapter 把 PS 接受决定信封接到 NR 的初始路由编排（UC-NR-001），
// 是 nrinbox.AcceptanceConsumer 的真实处理方。
//
// Purpose 是装配参数，与 ReassessOnIntakeAdapter 同一条纪律：判断键含服务目的，而
// 目的属服务产品的话语，未配置时停下不猜。
type RouteOnAcceptanceAdapter struct {
	requests AcceptedRequestSource
	route    *nrapplication.CreateInitialRouteHandler
	purpose  nrdomain.ServicePurpose
}

func NewRouteOnAcceptanceAdapter(
	requests AcceptedRequestSource,
	route *nrapplication.CreateInitialRouteHandler,
	purpose nrdomain.ServicePurpose,
) (*RouteOnAcceptanceAdapter, error) {
	if requests == nil {
		return nil, fmt.Errorf("network routing parcelshipment adapter: request source is nil")
	}
	if route == nil {
		return nil, fmt.Errorf("network routing parcelshipment adapter: route handler is nil")
	}
	return &RouteOnAcceptanceAdapter{requests: requests, route: route, purpose: purpose}, nil
}

var _ nrinbox.DecisionHandler = (*RouteOnAcceptanceAdapter)(nil)

// HandleAcceptedDecision 按引用取回基线再路由。
//
// 它整个跑在消费门的事务里：路由库写入、交接登记与发布意图都用 RequireExecutor 加入
// 同一事务，所以「处理成功与消费入账同一事务」原样成立，不需要本适配器再做什么。
func (adapter *RouteOnAcceptanceAdapter) HandleAcceptedDecision(
	ctx context.Context,
	decision nrinbox.AcceptedDecision,
) error {
	if !adapter.purpose.Valid() {
		return fmt.Errorf("%w: service purpose is not configured", ErrUntranslatableAnswer)
	}
	// 同一个事件类型同时承载接受与拒绝决定（PS 侧 acceptance-decision.formed），
	// 因此这里按封闭集合逐格分派，不用「非接受即拒绝」一刀切。
	//
	// 拒绝没有基线也没有可路由的东西，是终局答案不是失败：入账收工，不重投。
	// 此外的状态字（本类型不该承载的 SUBMITTED/WITHDRAWN、或译码器放行不了的空值）
	// 说不出该不该路由，一律报错让投递卡住看得见——与「拒绝」同格静默入账会把一份
	// 该路由的委托永久丢掉，而路由义务没有别的东西会来补。
	switch decision.State {
	case psdomain.ShipmentRequestAccepted.String():
	case psdomain.ShipmentRequestRejected.String():
		return nil
	default:
		return fmt.Errorf("%w: decision state %q is not carried by this event type",
			ErrUntranslatableAnswer, decision.State)
	}

	identity, err := sourceIdentityOf(decision)
	if err != nil {
		return err
	}
	request, found, err := adapter.requests.FindBySourceIdentity(ctx, identity)
	if err != nil {
		return fmt.Errorf("load accepted shipment request: %w", err)
	}
	if !found {
		// 委托与它的意图在 PS 侧同一事务落库，读不着通常是可见性滞后。回滚重投会
		// 再来一次；当成终局跳过则会丢掉这次路由。
		return fmt.Errorf("%w: shipment request %q is not visible",
			ErrEnvelopeContradictsAuthority, decision.ShipmentRequestID)
	}

	baseline, err := routableBaseline(request, decision)
	if err != nil {
		return err
	}
	resolution, err := acceptedResolutionOf(request)
	if err != nil {
		return err
	}
	spec, err := adapter.handoffSpecFor(decision, baseline)
	if err != nil {
		return err
	}

	result, err := adapter.route.Handle(ctx, nrapplication.CreateInitialRouteCommand{
		Handoff:    spec,
		Purpose:    adapter.purpose,
		Resolution: resolution,
	})
	if err != nil {
		return fmt.Errorf("create initial route: %w", err)
	}
	return outcomeToConsumption(result)
}

// routableBaseline 核对信封与权威是否说的是同一件事，并交回可路由的基线。
//
// 三道核对同一条理由（UC-NR-001 启动条件：「只有状态字符串而没有基线引用时不得
// 继续」）——信封给的是状态字，路由的业务输入是基线本体，两者不一致时按信封往下走
// 就是拿一个状态字当基线用。
func routableBaseline(
	request psdomain.ShipmentRequest,
	decision nrinbox.AcceptedDecision,
) (psdomain.AcceptanceBaseline, error) {
	none := psdomain.AcceptanceBaseline{}
	if request.State() != psdomain.ShipmentRequestAccepted {
		return none, fmt.Errorf("%w: envelope says ACCEPTED, authority says %q",
			ErrEnvelopeContradictsAuthority, request.State())
	}
	baseline, present := request.AcceptanceBaseline()
	if !present {
		return none, fmt.Errorf("%w: an accepted request carries no acceptance baseline",
			ErrEnvelopeContradictsAuthority)
	}
	// 基线指着另一个提交版本，说明这份信封是换代前那一版的：按当前基线路由会把一次
	// 陈旧的接受决定挂到新基线上，而判断键正是以基线认身份的。
	//
	// 这里不再为空提交版本留口子：消费门的译码器已把它列为必备字段，空值到不了这里；
	// 留着「空就跳过核对」等于给这道守卫留一条只要少个字段就能绕开的路。
	if baseline.SubmissionVersionID().String() != decision.SubmissionVersion {
		return none, fmt.Errorf("%w: envelope carries submission version %q, baseline is fixed on %q",
			ErrEnvelopeContradictsAuthority, decision.SubmissionVersion, baseline.SubmissionVersionID())
	}
	return baseline, nil
}

// acceptedResolutionOf 取出已接受决定上的解析标识，译成 NR 引用（ADR-0064）。
// 没有决定或快照是依赖不可用：静默跳过会把这次路由义务入账丢掉。
func acceptedResolutionOf(request psdomain.ShipmentRequest) (nrdomain.CommercialResolutionReference, error) {
	none := nrdomain.CommercialResolutionReference{}
	decision, present := request.AcceptanceDecision()
	if !present {
		return none, fmt.Errorf("%w: an accepted request carries no acceptance decision",
			ErrRouteHandoffUndecided)
	}
	resolution, err := nrdomain.NewCommercialResolutionReference(decision.Basis().ResolutionID().String())
	if err != nil {
		return none, fmt.Errorf("%w: commercial resolution: %v", ErrRouteHandoffUndecided, err)
	}
	return resolution, nil
}

func sourceIdentityOf(decision nrinbox.AcceptedDecision) (psdomain.SourceIdentity, error) {
	none := psdomain.SourceIdentity{}
	tenant, err := psdomain.NewTenantID(decision.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	customer, err := psdomain.NewCustomerAccountID(decision.CustomerAccountID)
	if err != nil {
		return none, fmt.Errorf("%w: customer: %v", ErrUntranslatableAnswer, err)
	}
	source, err := psdomain.NewSource(decision.Source)
	if err != nil {
		return none, fmt.Errorf("%w: source: %v", ErrUntranslatableAnswer, err)
	}
	requestKey, err := psdomain.NewSourceRequestKey(decision.SourceRequestKey)
	if err != nil {
		return none, fmt.Errorf("%w: source request key: %v", ErrUntranslatableAnswer, err)
	}
	identity, err := psdomain.NewSourceIdentity(tenant, customer, source, requestKey)
	if err != nil {
		return none, fmt.Errorf("%w: source identity: %v", ErrUntranslatableAnswer, err)
	}
	return identity, nil
}

// handoffSpecFor 把基线译成 NR 的交接原料。成员与固定时间都取自基线本体而不是信封：
// 信封只带引用，基线才是「接受时固定下来的不可覆盖成员集合」。
func (adapter *RouteOnAcceptanceAdapter) handoffSpecFor(
	decision nrinbox.AcceptedDecision,
	baseline psdomain.AcceptanceBaseline,
) (nrdomain.RouteHandoffSpec, error) {
	none := nrdomain.RouteHandoffSpec{}

	// 关联带上触发家族前缀：改路复核那条链用 "intake/"，两条链共用一个关联空间会让
	// 交接登记册把不同触发的交接撞成同一份。
	correlation, err := nrdomain.NewRequestCorrelationID("acceptance/" + decision.DecisionID)
	if err != nil {
		return none, fmt.Errorf("%w: correlation: %v", ErrUntranslatableAnswer, err)
	}
	tenant, err := nrdomain.NewTenantID(decision.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	customer, err := nrdomain.NewCustomerAccountID(decision.CustomerAccountID)
	if err != nil {
		return none, fmt.Errorf("%w: customer: %v", ErrUntranslatableAnswer, err)
	}
	shipment, err := nrdomain.NewShipmentRequestID(decision.ShipmentRequestID)
	if err != nil {
		return none, fmt.Errorf("%w: shipment request: %v", ErrUntranslatableAnswer, err)
	}
	decisionRef, err := nrdomain.NewAcceptanceDecisionReference(decision.DecisionID)
	if err != nil {
		return none, fmt.Errorf("%w: acceptance decision: %v", ErrUntranslatableAnswer, err)
	}
	// 基线引用取提交版本标识，与 ReassessOnIntakeAdapter 取 Intake.Baseline() 同源：
	// 两条链必须用同一个字符串指同一份基线，否则同一个包裹的初始判断与复核会落在
	// 两个判断键上。
	baselineRef, err := nrdomain.NewAcceptanceBaselineReference(baseline.SubmissionVersionID().String())
	if err != nil {
		return none, fmt.Errorf("%w: acceptance baseline: %v", ErrUntranslatableAnswer, err)
	}

	members := baseline.DeclaredParcelIDs()
	parcels := make([]nrdomain.DeclaredParcelID, 0, len(members))
	for _, member := range members {
		parcel, err := nrdomain.NewDeclaredParcelID(member.String())
		if err != nil {
			return none, fmt.Errorf("%w: parcel: %v", ErrUntranslatableAnswer, err)
		}
		parcels = append(parcels, parcel)
	}

	return nrdomain.RouteHandoffSpec{
		Correlation:        correlation,
		TenantID:           tenant,
		CustomerAccountID:  customer,
		ShipmentRequestID:  shipment,
		AcceptanceDecision: decisionRef,
		AcceptanceBaseline: baselineRef,
		Parcels:            parcels,
		AcceptedAt:         baseline.FixedAt(),
	}, nil
}

// outcomeToConsumption 把编排结果折成消费门认得的两格：nil 是「这份投递处理完了」，
// error 是「回滚重投」。折叠规则按「重投会不会改变结果」分——不会改变的一律入账，
// 否则丢的是投递；会改变的一律回滚，否则丢的是路由义务。
func outcomeToConsumption(result nrapplication.CreateInitialRouteResult) error {
	switch result.Outcome() {
	case nrapplication.RouteHandoffProcessed:
		for _, parcel := range result.Parcels() {
			if parcel.Outcome() == nrapplication.ParcelRouteUndecided {
				return fmt.Errorf("%w: parcel %q stopped at %s",
					ErrRouteHandoffUndecided,
					parcel.Key().DeclaredParcelID,
					parcel.UndecidedReason())
			}
		}
		return nil
	case nrapplication.RouteHandoffNotApplicable:
		// 产品不要求网络路由：终局业务答案，重投一万次还是不适用。
		return nil
	case nrapplication.RouteHandoffConflict:
		// 同一交接关联已登记过另一份内容：原交接与原结果不被覆盖，重投也不会变。
		return nil
	case nrapplication.RouteHandoffUndecided:
		return fmt.Errorf("%w: %s", ErrRouteHandoffUndecided, result.UndecidedReason())
	case nrapplication.RouteHandoffNotAccepted:
		// 交接原料立不起来。原料全部由本适配器从基线译出，所以这一格是本适配器的
		// 缺陷而不是外来坏数据——响亮上抛，不要静默入账。
		return fmt.Errorf("%w: the handoff spec built from the baseline was refused",
			ErrUntranslatableAnswer)
	default:
		return fmt.Errorf("%w: unexpected route handoff outcome %q",
			ErrUntranslatableAnswer, result.Outcome())
	}
}
