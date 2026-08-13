package partycommercial

import (
	"context"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// IntakeContentSource 是适配器内部协作者：按委托的接受时依据取回收寄资格声明。
// 声明从哪个规则包版本读、怎么缓存属装配；found=false 即「PAR-COM-16 未声明」。
type IntakeContentSource interface {
	IntakeContentFor(
		ctx context.Context,
		identity psdomain.SourceIdentity,
		shipmentRequestID psdomain.ShipmentRequestID,
	) (pcdomain.IntakeQualificationContent, bool, error)
}

// FinalContentSource 同上，取回终局规则声明（PAR-COM-17）。
type FinalContentSource interface {
	FinalContentFor(
		ctx context.Context,
		identity psdomain.SourceIdentity,
	) (pcdomain.FinalRuleContent, bool, error)
}

// ServiceStageRulesAdapter 把 party-commercial 的收寄资格与终局规则声明译成
// parcel-shipment 的两个规则视图端口（ADR-0025 消费方侧）。声明是提供方的话语，
// 判断口径（成立/未决/不适用、满足/不满足）是消费方的话语——翻译在这里，不在两边。
type ServiceStageRulesAdapter struct {
	intake IntakeContentSource
	final  FinalContentSource
}

func NewServiceStageRulesAdapter(
	intake IntakeContentSource,
	final FinalContentSource,
) *ServiceStageRulesAdapter {
	return &ServiceStageRulesAdapter{intake: intake, final: final}
}

var _ psports.IntakeEligibilityView = (*ServiceStageRulesAdapter)(nil)
var _ psports.FinalRuleView = (*ServiceStageRulesAdapter)(nil)

// JudgeIntakeEligibility 按声明判收寄资格：来源不在允许集合即不适用（带来源依据——
// 服务形态不承担这种收寄，不是资格没过）；声明的硬资格清单非空时，逐项核对属实例
// 取证缝，机制上如实答「未成立」带清单——首发无租户时资格项证据不可能取得，这一格
// 是唯一走得到的真实分支；声明未配置即 found=false。
func (adapter *ServiceStageRulesAdapter) JudgeIntakeEligibility(
	ctx context.Context,
	identity psdomain.SourceIdentity,
	shipmentRequestID psdomain.ShipmentRequestID,
	source psdomain.IntakeSource,
) (psports.IntakeEligibility, bool, error) {
	content, configured, err := adapter.intake.IntakeContentFor(ctx, identity, shipmentRequestID)
	if err != nil {
		return psports.IntakeEligibility{}, false, fmt.Errorf("intake content: %w", err)
	}
	if !configured {
		return psports.IntakeEligibility{}, false, nil
	}

	declared, err := declaredSourceFor(source.Kind())
	if err != nil {
		return psports.IntakeEligibility{}, false, err
	}
	if !content.Allows(declared) {
		basis, err := psdomain.NewCheckReason("SOURCE_NOT_IN_SERVICE_SHAPE/" + declared.String())
		if err != nil {
			return psports.IntakeEligibility{}, false, fmt.Errorf("not-applicable basis: %w", err)
		}
		return psports.IntakeEligibility{
			Outcome: psports.IntakeServiceNotApplicable,
			Basis:   basis,
		}, true, nil
	}

	qualifications := content.Qualifications()
	if len(qualifications) == 0 {
		return psports.IntakeEligibility{Outcome: psports.IntakeEligibilityEstablished}, true, nil
	}
	// 声明列出了硬资格，而资格证据的取证缝是实例半边——机制上如实答未成立并点名
	// 头一项缺口，编排据以保持未决（AT-PS-047 的续办路），不默认通过。
	basis, err := psdomain.NewCheckReason("INTAKE_QUALIFICATION_UNPROVEN/" + qualifications[0].String())
	if err != nil {
		return psports.IntakeEligibility{}, false, fmt.Errorf("unproven basis: %w", err)
	}
	return psports.IntakeEligibility{
		Outcome: psports.IntakeEligibilityNotEstablished,
		Basis:   basis,
	}, true, nil
}

// JudgeFinalOutcome 按声明判终局：责任结果有声明行即满足并带声明的终局类型；缺行
// 即不满足带依据（此产品下这种结果不形成终局——那是声明的真话，编排保持未决等其他
// 责任结果）；声明未配置即 found=false。
func (adapter *ServiceStageRulesAdapter) JudgeFinalOutcome(
	ctx context.Context,
	identity psdomain.SourceIdentity,
	outcome psdomain.ResponsibilityOutcome,
) (psports.FinalRuleJudgment, bool, error) {
	content, configured, err := adapter.final.FinalContentFor(ctx, identity)
	if err != nil {
		return psports.FinalRuleJudgment{}, false, fmt.Errorf("final content: %w", err)
	}
	if !configured {
		return psports.FinalRuleJudgment{}, false, nil
	}

	declared, err := declaredOutcomeFor(outcome.Kind())
	if err != nil {
		return psports.FinalRuleJudgment{}, false, err
	}
	kind, declares := content.FinalKindFor(declared)
	if !declares {
		basis, err := psdomain.NewCheckReason("OUTCOME_NOT_FINAL_FOR_PRODUCT/" + declared.String())
		if err != nil {
			return psports.FinalRuleJudgment{}, false, fmt.Errorf("not-final basis: %w", err)
		}
		return psports.FinalRuleJudgment{Satisfied: false, Basis: basis}, true, nil
	}

	finalKind, err := psdomain.NewFinalKindReference(kind.String())
	if err != nil {
		return psports.FinalRuleJudgment{}, false, fmt.Errorf("%w: final kind: %v", ErrUntranslatableAnswer, err)
	}
	ruleVersion, err := psdomain.NewFinalRuleVersionReference("PAR-COM-17/" + kind.String())
	if err != nil {
		return psports.FinalRuleJudgment{}, false, fmt.Errorf("%w: rule version: %v", ErrUntranslatableAnswer, err)
	}
	return psports.FinalRuleJudgment{
		Satisfied:   true,
		Kind:        finalKind,
		RuleVersion: ruleVersion,
	}, true, nil
}

// declaredSourceFor 逐格翻译两边的封闭集合，default 报错不吸收（ADR-0025）。
func declaredSourceFor(kind psdomain.IntakeSourceKind) (pcdomain.DeclaredIntakeSource, error) {
	switch kind {
	case psdomain.NodeIntakeSource:
		return pcdomain.DeclaredNodeIntake, nil
	case psdomain.OffsitePickupSource:
		return pcdomain.DeclaredOffsitePickup, nil
	default:
		return pcdomain.DeclaredIntakeSourceInvalid, fmt.Errorf(
			"%w: intake source kind %d", ErrUntranslatableAnswer, kind)
	}
}

func declaredOutcomeFor(kind psdomain.ResponsibilityOutcomeKind) (pcdomain.DeclaredResponsibilityOutcome, error) {
	switch kind {
	case psdomain.EffectiveDeliveryOutcome:
		return pcdomain.DeclaredEffectiveDelivery, nil
	case psdomain.ReturnCompletedOutcome:
		return pcdomain.DeclaredReturnCompleted, nil
	case psdomain.ServiceTerminatedOutcome:
		return pcdomain.DeclaredServiceTerminated, nil
	case psdomain.RegulatoryDispositionExecuted:
		return pcdomain.DeclaredRegulatoryDisposition, nil
	default:
		return pcdomain.DeclaredResponsibilityOutcomeInvalid, fmt.Errorf(
			"%w: responsibility outcome kind %d", ErrUntranslatableAnswer, kind)
	}
}
