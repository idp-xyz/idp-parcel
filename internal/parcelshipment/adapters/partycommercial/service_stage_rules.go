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

// CancellationContentSource 取回取消授权目录（PAR-COM-17）。found=false 即未配置。
//
// 本切片不给它单独落表：按 ADR-0042，阶段内容属拥有对象（产品/合同版本）的正文，
// 存储问题属于整个阶段内容声明族（Intake/Final/Cancellation 三口一盘棋）。单独给
// 取消开表会预先拍掉族设计且碎化；日后存储成片时三口同切。
type CancellationContentSource interface {
	CancellationContentFor(
		ctx context.Context,
		identity psdomain.SourceIdentity,
	) (pcdomain.CancellationAuthorityContent, bool, error)
}

// CancellationRequesterClassSource 把消费方的请求方引用折成提供方的请求方格。
// 映射属实例半边：没有租户时谁也说不出 OPERATOR-1 是客户还是运营。
// 未配置时适配器不调用本口——首发路径必须停在「目录未配置」，不能滑成 error。
type CancellationRequesterClassSource interface {
	FormCancellationParty(
		ctx context.Context,
		identity psdomain.SourceIdentity,
		requester psdomain.CancellationRequesterReference,
	) (pcdomain.DeclaredCancellationParty, bool, error)
}

// ServiceStageRulesAdapter 把 party-commercial 的收寄资格、终局规则与取消授权目录
// 译成 parcel-shipment 的三个规则视图端口（ADR-0025 消费方侧）。声明是提供方的话语，
// 判断口径（成立/未决/不适用、满足/不满足、允许/不允许）是消费方的话语——翻译在这里，不在两边。
type ServiceStageRulesAdapter struct {
	intake       IntakeContentSource
	final        FinalContentSource
	cancellation CancellationContentSource
	requesters   CancellationRequesterClassSource
}

func NewServiceStageRulesAdapter(
	intake IntakeContentSource,
	final FinalContentSource,
	cancellation CancellationContentSource,
	requesters CancellationRequesterClassSource,
) *ServiceStageRulesAdapter {
	return &ServiceStageRulesAdapter{
		intake:       intake,
		final:        final,
		cancellation: cancellation,
		requesters:   requesters,
	}
}

var _ psports.IntakeEligibilityView = (*ServiceStageRulesAdapter)(nil)
var _ psports.FinalRuleView = (*ServiceStageRulesAdapter)(nil)
var _ psports.CancellationAuthorityView = (*ServiceStageRulesAdapter)(nil)

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

// JudgeCancellationAuthority 按目录判取消授权。目录按接受时产品/合同说话，不按
// 包裹发明不同授权（parcel 入参只为满足消费方端口）。
//
// 未配置短接在翻译之前：没有目录就不问这个 requester 是客户还是运营。已配置才折
// 请求方格；格不在词汇表内不吸收（ADR-0025）。命中带规则引用；缺行带依据拒绝。
func (adapter *ServiceStageRulesAdapter) JudgeCancellationAuthority(
	ctx context.Context,
	identity psdomain.SourceIdentity,
	requester psdomain.CancellationRequesterReference,
	_ psdomain.DeclaredParcelID,
) (psports.CancellationAuthorityJudgment, bool, error) {
	if adapter.cancellation == nil {
		return psports.CancellationAuthorityJudgment{}, false, fmt.Errorf(
			"cancellation content source is not configured")
	}
	content, configured, err := adapter.cancellation.CancellationContentFor(ctx, identity)
	if err != nil {
		return psports.CancellationAuthorityJudgment{}, false, fmt.Errorf("cancellation content: %w", err)
	}
	if !configured {
		return psports.CancellationAuthorityJudgment{}, false, nil
	}

	if adapter.requesters == nil {
		return psports.CancellationAuthorityJudgment{}, false, fmt.Errorf(
			"cancellation requester mapping is not configured")
	}
	party, formed, err := adapter.requesters.FormCancellationParty(ctx, identity, requester)
	if err != nil {
		return psports.CancellationAuthorityJudgment{}, false, fmt.Errorf("cancellation requester: %w", err)
	}
	if !formed {
		return psports.CancellationAuthorityJudgment{}, false, fmt.Errorf(
			"cancellation requester class is not formed")
	}
	if !cancellationPartyKnown(party) {
		return psports.CancellationAuthorityJudgment{}, false, fmt.Errorf(
			"%w: cancellation party %d", ErrUntranslatableAnswer, party)
	}

	rule, granted := content.RuleFor(party)
	if !granted {
		basis, err := psdomain.NewCheckReason("PARTY_NOT_AUTHORIZED_TO_CANCEL/" + party.String())
		if err != nil {
			return psports.CancellationAuthorityJudgment{}, false, fmt.Errorf("refusal basis: %w", err)
		}
		return psports.CancellationAuthorityJudgment{Granted: false, Basis: basis}, true, nil
	}
	basis, err := psdomain.NewCheckReason(rule.String())
	if err != nil {
		return psports.CancellationAuthorityJudgment{}, false, fmt.Errorf("%w: authority basis: %v", ErrUntranslatableAnswer, err)
	}
	return psports.CancellationAuthorityJudgment{Granted: true, Basis: basis}, true, nil
}

func cancellationPartyKnown(party pcdomain.DeclaredCancellationParty) bool {
	return party == pcdomain.DeclaredCustomerCancellation ||
		party == pcdomain.DeclaredOperationsCancellation
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
