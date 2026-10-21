// Package parcelshipment 是 network-routing 消费 parcel-shipment 有效网络收寄结果的
// 适配器（ADR-0025 消费方侧）。它只翻译不判断：采用记录译成复核触发，复核结论由 NR 的
// 复核编排回答（UC-PS-003 步骤 8「network-routing 消费有效网络收寄及原物理位置/控制
// 引用，通过 UC-NR-003 重新校验路由」）。
package parcelshipment

import (
	"context"
	"errors"
	"fmt"
	"time"

	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ErrUntranslatableAnswer 语义与其他消费方适配器的同名哨兵一致。
var ErrUntranslatableAnswer = errors.New("network routing parcelshipment adapter: untranslatable answer")

// ErrRefusalDoesNotTrigger 说明这份采用记录是`不采用`：没有责任起点成立，路由没有要
// 复核的收寄事实。它与翻译坏了分开——不采用是提供方的业务答案，不是本适配器的故障。
var ErrRefusalDoesNotTrigger = errors.New("network routing parcelshipment adapter: a refused adoption does not trigger reassessment")

// ReassessOnIntakeAdapter 把有效网络收寄的采用记录交给 NR 的复核编排。
//
// Purpose 是装配参数：初始路由判断键含服务目的，而目的属服务产品的话语——与可达性
// 适配器的同名参数一条纪律，未配置时停下不猜。
type ReassessOnIntakeAdapter struct {
	reassess *nrapplication.ReassessRouteHandler
	purpose  nrdomain.ServicePurpose
}

func NewReassessOnIntakeAdapter(
	reassess *nrapplication.ReassessRouteHandler,
	purpose nrdomain.ServicePurpose,
) *ReassessOnIntakeAdapter {
	return &ReassessOnIntakeAdapter{reassess: reassess, purpose: purpose}
}

// TriggerReassessment 翻译并转交一份采用记录，连同采用信封携带的内容（复核按它取证据，ADR-0075）与这件委托接受的时刻
// （复核的业务时间下限，ADR-0279；由调用方从 parcel-shipment 读来）。
// 控制依据按来源类型逐格译：节点收寄是节点控制，建立当前可控节点；场外揽收译成揽收那一格，照复核却不建立可控节点，
// 揽收地点是客户场所、原样作复核位置（ADR-0249）。普通扫描在提供方的采用判断里就进不来。
func (adapter *ReassessOnIntakeAdapter) TriggerReassessment(
	ctx context.Context,
	record psports.IntakeAdoptionRecord,
	acceptedAt time.Time,
	carried nrports.RequestCarriedContent,
) (nrapplication.ReassessRouteResult, error) {
	if !adapter.purpose.Valid() {
		return nrapplication.ReassessRouteResult{}, fmt.Errorf(
			"%w: service purpose is not configured", ErrUntranslatableAnswer)
	}
	if !record.Adopted {
		return nrapplication.ReassessRouteResult{}, ErrRefusalDoesNotTrigger
	}
	trigger, err := adapter.triggerSpecFor(record)
	if err != nil {
		return nrapplication.ReassessRouteResult{}, err
	}
	trigger.AcceptedAt = acceptedAt
	return adapter.reassess.Handle(ctx, nrapplication.ReassessRouteCommand{Trigger: trigger, Carried: carried})
}

func (adapter *ReassessOnIntakeAdapter) triggerSpecFor(
	record psports.IntakeAdoptionRecord,
) (nrdomain.ReassessmentTriggerSpec, error) {
	none := nrdomain.ReassessmentTriggerSpec{}
	source := record.Intake.Source()

	correlation, err := nrdomain.NewRequestCorrelationID(
		"intake/" + record.Key.TenantID.String() + "/" + record.Key.Parcel.String() +
			"/" + record.Key.Kind.String() + "/" + record.Key.Version.String())
	if err != nil {
		return none, fmt.Errorf("%w: correlation: %v", ErrUntranslatableAnswer, err)
	}
	tenant, err := nrdomain.NewTenantID(record.Key.TenantID.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	customer, err := nrdomain.NewCustomerAccountID(record.CustomerAccountID.String())
	if err != nil {
		return none, fmt.Errorf("%w: customer: %v", ErrUntranslatableAnswer, err)
	}
	shipment, err := nrdomain.NewShipmentRequestID(record.ShipmentRequestID.String())
	if err != nil {
		return none, fmt.Errorf("%w: shipment request: %v", ErrUntranslatableAnswer, err)
	}
	baseline, err := nrdomain.NewAcceptanceBaselineReference(record.Intake.Baseline().String())
	if err != nil {
		return none, fmt.Errorf("%w: acceptance baseline: %v", ErrUntranslatableAnswer, err)
	}
	parcel, err := nrdomain.NewDeclaredParcelID(record.Key.Parcel.String())
	if err != nil {
		return none, fmt.Errorf("%w: parcel: %v", ErrUntranslatableAnswer, err)
	}
	location, err := nrdomain.NewActualLocationReference(source.Place().String())
	if err != nil {
		return none, fmt.Errorf("%w: intake place: %v", ErrUntranslatableAnswer, err)
	}
	sourceVersion, err := nrdomain.NewSourceFactVersionReference(source.Version().String())
	if err != nil {
		return none, fmt.Errorf("%w: source version: %v", ErrUntranslatableAnswer, err)
	}
	control, err := controlKindFor(source.Kind())
	if err != nil {
		return none, err
	}

	return nrdomain.ReassessmentTriggerSpec{
		Correlation: correlation,
		Key: nrdomain.InitialRouteJudgmentKey{
			TenantID:           tenant,
			CustomerAccountID:  customer,
			ShipmentRequestID:  shipment,
			AcceptanceBaseline: baseline,
			DeclaredParcelID:   parcel,
			ServicePurpose:     adapter.purpose,
		},
		Control:       control,
		Location:      location,
		SourceVersion: sourceVersion,
		OccurredAt:    source.OccurredAt(),
	}, nil
}

// controlKindFor 逐格翻译两边的封闭集合，default 报错不吸收（ADR-0025）。
func controlKindFor(kind psdomain.IntakeSourceKind) (nrdomain.ControlEvidenceKind, error) {
	switch kind {
	case psdomain.NodeIntakeSource:
		return nrdomain.NodeIntakeControl, nil
	case psdomain.OffsitePickupSource:
		// 不译成权威运输交接：那一格是交进声明的接收节点，位置是节点码；揽收的位置是客户场所，译过去复核就会拿它
		// 比计划节点、把揽收当可控节点（ADR-0249）。
		return nrdomain.OffsitePickupControl, nil
	default:
		return nrdomain.ControlEvidenceKindInvalid, fmt.Errorf(
			"%w: intake source kind %d", ErrUntranslatableAnswer, kind)
	}
}
