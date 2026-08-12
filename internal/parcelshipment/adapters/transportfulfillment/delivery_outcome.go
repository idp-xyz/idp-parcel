package transportfulfillment

import (
	"context"
	"fmt"

	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// DeliveryOutcomeAdapter 把 transport-fulfillment 的有效交付结果交给 parcel-shipment
// 的终局采用编排（UC-PS-004 的 TF-DELIVERY 来源行）。它只翻译不判断：交付是否满足此
// 产品的终局规则由 PS 侧的规则视图回答——「有效交付不在所有产品中自动等于终局」的
// 分工正在于此。
type DeliveryOutcomeAdapter struct {
	form *psapplication.FormParcelFinalHandler
}

func NewDeliveryOutcomeAdapter(form *psapplication.FormParcelFinalHandler) *DeliveryOutcomeAdapter {
	return &DeliveryOutcomeAdapter{form: form}
}

// AdoptFromEffectiveDelivery 翻译并转交一份有效交付。只有 EffectiveDelivery 能进来
// ——拒收、无人签收、地址错误在 TF 的领域上就形不成这个类型，本适配器天然只见妥投。
func (adapter *DeliveryOutcomeAdapter) AdoptFromEffectiveDelivery(
	ctx context.Context,
	delivery tfdomain.EffectiveDelivery,
	target TargetShipment,
) (psapplication.FormParcelFinalResult, error) {
	spec, err := responsibilityOutcomeFor(delivery)
	if err != nil {
		return psapplication.FormParcelFinalResult{}, err
	}
	return adapter.form.Handle(ctx, psapplication.FormParcelFinalCommand{
		Identity:          target.Identity,
		ShipmentRequestID: target.ShipmentRequestID,
		Outcome:           spec,
	})
}

// responsibilityOutcomeFor 逐维翻译：载运对象→包裹、交付判断（对象+尝试）→决定引用、
// POD 证明→执行事实引用、结果版本与实际交付时间原样带过。更正版本自然换新版本号——
// PS 侧同源新版本走重派生，POD 失效重派生的链路（AT-PS-063）由此接通。
func responsibilityOutcomeFor(delivery tfdomain.EffectiveDelivery) (psdomain.ResponsibilityOutcomeSpec, error) {
	none := psdomain.ResponsibilityOutcomeSpec{}
	parcel, err := psdomain.NewDeclaredParcelID(delivery.Object().String())
	if err != nil {
		return none, fmt.Errorf("%w: carried object: %v", ErrUntranslatableAnswer, err)
	}
	decision, err := psdomain.NewResponsibilityDecisionReference(
		"DELIVERY-JUDGMENT/" + delivery.Object().String() + "/" + delivery.Attempt().String())
	if err != nil {
		return none, fmt.Errorf("%w: delivery judgment: %v", ErrUntranslatableAnswer, err)
	}
	execution, err := psdomain.NewExecutionEvidenceReference("POD/" + delivery.Proof().String())
	if err != nil {
		return none, fmt.Errorf("%w: delivery proof: %v", ErrUntranslatableAnswer, err)
	}
	version, err := psdomain.NewResponsibilityOutcomeVersion(delivery.Version().String())
	if err != nil {
		return none, fmt.Errorf("%w: result version: %v", ErrUntranslatableAnswer, err)
	}
	return psdomain.ResponsibilityOutcomeSpec{
		Kind:       psdomain.EffectiveDeliveryOutcome,
		Parcel:     parcel,
		Decision:   decision,
		Execution:  execution,
		Version:    version,
		OccurredAt: delivery.OccurredAt(),
	}, nil
}
