// Package transportfulfillment 是 network-routing 消费 transport-fulfillment 权威交接结果的适配器（ADR-0025 消费方
// 侧；只翻译不判断）。交接进入一个节点、取得节点控制，是 CONTEXT「当前可控节点」认可的两种依据之一；本包把那一次
// 交接译成复核触发，复核结论由 NR 的复核编排回答（UC-NR-003，ADR-0215）。
package transportfulfillment

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	nrparcelshipment "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	// ErrHandoverNotVisible 说明按信封的键还读不回那一版交接，或交接登记册此刻读不动。交接与它的信封在 TF 侧同一笔
	// 落库，读不着通常是可见性滞后：重投会改变结果，生产装配把它登进本路的未决哨兵。
	ErrHandoverNotVisible = errors.New(
		"network routing transportfulfillment adapter: transport handover is not yet visible")
	// ErrHandoverRecordInconsistent 说明按键取回的交接指着另一把键、业务时间为零，或裁决落在封闭三值之外。仓储不变量
	// 已破（ADR-0029），不是等谁，禁止进未决哨兵。
	ErrHandoverRecordInconsistent = errors.New(
		"network routing transportfulfillment adapter: transport handover disagrees with its key")
	// ErrUntranslatableAnswer 语义与其余消费方适配器的同名哨兵一致：一侧交出了词汇表之外的内容，或本包自己组出的
	// 触发过不了复核的门。编程错误，不折成「不复核」。
	ErrUntranslatableAnswer = errors.New(
		"network routing transportfulfillment adapter: untranslatable answer")
	// ErrHandoverReassessmentUndecided 说明这次复核停在未决（复核登记册或路由历史读不回、证据未配置、适用性看不清、
	// 取号依赖故障）。与收寄采用那一路的同类哨兵分设：两条触发等的依赖不同，运维读到失败要分得出是哪一路。
	ErrHandoverReassessmentUndecided = errors.New(
		"network routing transportfulfillment adapter: route reassessment on transport handover is undecided")
)

// TransportHandoverFinder 按交接判断键取回那一版登记。由 TF 的交接登记册满足（*tfpostgres.TransportHandovers 的同名
// 方法）；只取一法，本适配器不写 TF 的库。
type TransportHandoverFinder interface {
	FindByKey(
		ctx context.Context,
		key tfports.TransportHandoverKey,
	) (tfports.TransportHandoverRecord, bool, error)
}

// AcceptedCarriedGeoLookup 按（租户，包裹）答这件包裹是不是当前已接受包裹，并交回判断键各维与受理基线那一版的地理
// 投影。真实装配是 nrparcelshipment.AcceptedCarriedGeoSource。
type AcceptedCarriedGeoLookup interface {
	LoadAcceptedCarriedGeo(
		ctx context.Context,
		tenant nrdomain.TenantID,
		parcel nrdomain.DeclaredParcelID,
	) (nrparcelshipment.AcceptedCarriedGeo, bool, error)
}

// ReassessOnHandoverAdapter 是 nrinbox.TransportHandoverConsumer 的真实处理方：按键读回交接，只对「`已交接`且声明了
// 接收节点」的那一版形成复核触发（ADR-0215 决定一），其余入账。
//
// Purpose 是装配参数，与 ReassessOnIntakeAdapter 同一条纪律：判断键含服务目的，目的属服务产品的话语，未配置时停下
// 不猜。
type ReassessOnHandoverAdapter struct {
	handovers TransportHandoverFinder
	accepted  AcceptedCarriedGeoLookup
	reassess  *nrapplication.ReassessRouteHandler
	purpose   nrdomain.ServicePurpose
}

func NewReassessOnHandoverAdapter(
	handovers TransportHandoverFinder,
	accepted AcceptedCarriedGeoLookup,
	reassess *nrapplication.ReassessRouteHandler,
	purpose nrdomain.ServicePurpose,
) (*ReassessOnHandoverAdapter, error) {
	if handovers == nil {
		return nil, fmt.Errorf("network routing transportfulfillment adapter: handover finder is nil")
	}
	if accepted == nil {
		return nil, fmt.Errorf("network routing transportfulfillment adapter: accepted carried geo lookup is nil")
	}
	if reassess == nil {
		return nil, fmt.Errorf("network routing transportfulfillment adapter: reassessment handler is nil")
	}
	return &ReassessOnHandoverAdapter{handovers: handovers, accepted: accepted, reassess: reassess, purpose: purpose}, nil
}

var _ nrinbox.TransportHandoverHandler = (*ReassessOnHandoverAdapter)(nil)

// HandleRegisteredTransportHandover 按键读回交接，判它是不是复核触发，是则复核。
//
// 整个跑在消费门的事务里：复核登记、适用性写入与判断库读写都加入同一事务，「处理成功与消费入账同一事务」原样成立。
func (adapter *ReassessOnHandoverAdapter) HandleRegisteredTransportHandover(
	ctx context.Context,
	registered nrinbox.RegisteredTransportHandover,
) error {
	if !adapter.purpose.Valid() {
		return fmt.Errorf("%w: service purpose is not configured", ErrUntranslatableAnswer)
	}
	key, err := handoverKeyOf(registered)
	if err != nil {
		return err
	}
	record, found, err := adapter.handovers.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrHandoverNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: object %q scope %q version %q",
			ErrHandoverNotVisible, registered.Object, registered.Scope, registered.Version)
	}
	handover := record.Handover
	if record.Key != key || handover.TenantID() != key.TenantID || handover.Object() != key.Object ||
		handover.Scope() != key.Scope || handover.Version() != key.Version || handover.JudgedAt().IsZero() {
		return fmt.Errorf("%w: object %q scope %q version %q",
			ErrHandoverRecordInconsistent, registered.Object, registered.Scope, registered.Version)
	}

	// 拒收与待确认没有转移控制；`已交接`而没声明接收节点，是交给运输方之类、之后没有可控节点的交接。两者都不是
	// 「当前可控节点」的依据，入账收工——让它回滚重投会把一份合法的交接永远卡在队列里。
	switch handover.Verdict() {
	case tfdomain.ObjectHandedOver:
	case tfdomain.HandoverRefused, tfdomain.HandoverPendingConfirmation:
		return nil
	default:
		return fmt.Errorf("%w: verdict of object %q version %q",
			ErrHandoverRecordInconsistent, registered.Object, registered.Version)
	}
	receiving, declared := handover.ReceivingNode()
	if !declared {
		return nil
	}

	tenant, err := nrdomain.NewTenantID(handover.TenantID().String())
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	// 载运对象引用与 NR 的声明包裹身份是同一串字面；对象若是集运单元，反查不中，下面入账。
	parcel, err := nrdomain.NewDeclaredParcelID(handover.Object().String())
	if err != nil {
		return fmt.Errorf("%w: carried object: %v", ErrUntranslatableAnswer, err)
	}
	accepted, isAccepted, err := adapter.accepted.LoadAcceptedCarriedGeo(ctx, tenant, parcel)
	if err != nil {
		return err
	}
	if !isAccepted {
		// 对象不是当前已接受包裹：没有可复核的路由判断属于它。复核复的是一份已有判断，这里不该停在未决等它出现。
		return nil
	}

	trigger, err := adapter.triggerSpecFor(handover, receiving, tenant, parcel, accepted)
	if err != nil {
		return err
	}
	result, err := adapter.reassess.Handle(ctx, nrapplication.ReassessRouteCommand{
		Trigger: trigger,
		Carried: nrports.RequestCarriedContent{Geo: accepted.Geo},
	})
	if err != nil {
		return fmt.Errorf("reassess route on transport handover: %w", err)
	}
	return reassessmentToConsumption(result)
}

// triggerSpecFor 把交接译成复核触发：控制依据是权威运输交接，位置是接收节点，来源版本与业务时间取交接那一版
// （ADR-0215 决定一）。
func (adapter *ReassessOnHandoverAdapter) triggerSpecFor(
	handover tfdomain.TransportHandover,
	receiving tfdomain.NetworkNodeReference,
	tenant nrdomain.TenantID,
	parcel nrdomain.DeclaredParcelID,
	accepted nrparcelshipment.AcceptedCarriedGeo,
) (nrdomain.ReassessmentTriggerSpec, error) {
	none := nrdomain.ReassessmentTriggerSpec{}
	correlation, err := nrdomain.NewRequestCorrelationID(correlationOf(handover))
	if err != nil {
		return none, fmt.Errorf("%w: correlation: %v", ErrUntranslatableAnswer, err)
	}
	location, err := nrdomain.NewActualLocationReference(receiving.String())
	if err != nil {
		return none, fmt.Errorf("%w: receiving node: %v", ErrUntranslatableAnswer, err)
	}
	sourceVersion, err := nrdomain.NewSourceFactVersionReference(handover.Version().String())
	if err != nil {
		return none, fmt.Errorf("%w: handover version: %v", ErrUntranslatableAnswer, err)
	}
	return nrdomain.ReassessmentTriggerSpec{
		Correlation: correlation,
		Key: nrdomain.InitialRouteJudgmentKey{
			TenantID:           tenant,
			CustomerAccountID:  accepted.CustomerAccountID,
			ShipmentRequestID:  accepted.ShipmentRequestID,
			AcceptanceBaseline: accepted.AcceptanceBaseline,
			DeclaredParcelID:   parcel,
			ServicePurpose:     adapter.purpose,
		},
		Control:       nrdomain.TransportHandoverControl,
		Location:      location,
		SourceVersion: sourceVersion,
		OccurredAt:    handover.JudgedAt(),
	}, nil
}

// correlationOf 是交接触发的关联：以 "handover/" 起头，与 "acceptance/"、"intake/" 两族错开，同一个交接登记册里不会
// 撞成同一份。四维逐维写成「字节长度:原串」：对象、范围与版本都是登记方给的串，可能含斜杠，直拼会让两次不同的交接
// 拼成同一个关联，第二次就被当成重放。
//
// 一版交接一个关联：更正是新版本，仍`已交接`时另成一次复核，原复核不被覆盖（UC-NR-003「来源更正……形成新复核」）。
func correlationOf(handover tfdomain.TransportHandover) string {
	correlation := "handover"
	for _, part := range []string{
		handover.TenantID().String(),
		handover.Object().String(),
		handover.Scope().String(),
		handover.Version().String(),
	} {
		correlation += "/" + strconv.Itoa(len(part)) + ":" + part
	}
	return correlation
}

func handoverKeyOf(registered nrinbox.RegisteredTransportHandover) (tfports.TransportHandoverKey, error) {
	none := tfports.TransportHandoverKey{}
	tenant, err := tfdomain.NewTenantID(registered.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	object, err := tfdomain.NewCarriedObjectReference(registered.Object)
	if err != nil {
		return none, fmt.Errorf("%w: carried object: %v", ErrUntranslatableAnswer, err)
	}
	scope, err := tfdomain.NewHandoverScopeReference(registered.Scope)
	if err != nil {
		return none, fmt.Errorf("%w: handover scope: %v", ErrUntranslatableAnswer, err)
	}
	version, err := tfdomain.NewHandoverResultVersion(registered.Version)
	if err != nil {
		return none, fmt.Errorf("%w: handover version: %v", ErrUntranslatableAnswer, err)
	}
	return tfports.TransportHandoverKey{TenantID: tenant, Object: object, Scope: scope, Version: version}, nil
}

// reassessmentToConsumption 把复核结论折成消费门认得的两格：nil 是「这份投递处理完了」，error 是「回滚重投」。按
// 「重投会不会改变结果」分，与收寄采用那一路的同名折法同一判据。
func reassessmentToConsumption(result nrapplication.ReassessRouteResult) error {
	switch result.Outcome() {
	case nrapplication.ReassessedStillApplicable,
		nrapplication.ReassessedPlanLapsed,
		nrapplication.ReassessedFirstPlanFormed,
		nrapplication.ReassessedRerouted:
		// 四种结论都已越过提交边界落了库，是终局答案。失效与改路不因「听起来像坏消息」就重投：复核复的就是这件事。
		return nil
	case nrapplication.ReassessExistingResult:
		// 同一版交接已复核过，读回原结果：幂等。
		return nil
	case nrapplication.ReassessTriggerConflict:
		// 同一关联登记过另一份内容（例如委托换了受理基线之后重投），原触发与原结果不被覆盖，重投也不会变。
		return nil
	case nrapplication.ReassessUndecided:
		return fmt.Errorf("%w: %s", ErrHandoverReassessmentUndecided, result.UndecidedReason())
	case nrapplication.ReassessTriggerNotAccepted:
		// 触发原料全部由本包从交接与已接受委托译出，过不了门是本包的缺陷，不是外来坏数据。
		return fmt.Errorf("%w: the trigger built from the handover was refused", ErrUntranslatableAnswer)
	default:
		return fmt.Errorf("%w: unexpected reassessment outcome %q", ErrUntranslatableAnswer, result.Outcome())
	}
}
