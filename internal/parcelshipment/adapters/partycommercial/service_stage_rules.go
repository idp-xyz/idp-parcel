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
// 目录的拥有对象是授权规则版本，不是产品或合同——后两者是采用方（ADR-0058）。
// 从哪个授权规则版本读、怎么缓存属装配；SourceIdentity 不含该版本时，生产装配停在
// 诚实未配置，不造默认目录。
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
	evidence     psports.IntakeQualificationEvidenceView
}

func NewServiceStageRulesAdapter(
	intake IntakeContentSource,
	final FinalContentSource,
	cancellation CancellationContentSource,
	requesters CancellationRequesterClassSource,
	evidence psports.IntakeQualificationEvidenceView,
) *ServiceStageRulesAdapter {
	return &ServiceStageRulesAdapter{
		intake:       intake,
		final:        final,
		cancellation: cancellation,
		requesters:   requesters,
		evidence:     evidence,
	}
}

var _ psports.IntakeEligibilityView = (*ServiceStageRulesAdapter)(nil)
var _ psports.FinalRuleView = (*ServiceStageRulesAdapter)(nil)
var _ psports.CancellationAuthorityView = (*ServiceStageRulesAdapter)(nil)

// JudgeIntakeEligibility 按声明判收寄资格：来源不在允许集合即不适用（带来源依据——
// 服务形态不承担这种收寄，不是资格没过）；空清单是显式无硬资格即成立；清单非空则
// 逐项问证据口（ADR-0063）——未证明点名该项，依赖失败上抛，不折成目录未配置；声明
// 未配置即 found=false。
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
	// 声明列出了硬资格。证明走消费侧窄口（ADR-0063），不在这里默认成立，也不把
	// nil 证据口读成已证明。未证明保持既有依据形状，点名第一项缺口（AT-PS-047）。
	if adapter.evidence == nil {
		return psports.IntakeEligibility{}, false, fmt.Errorf(
			"parcel shipment partycommercial adapter: intake qualification evidence view is nil")
	}
	for _, qualification := range qualifications {
		rule, err := psdomain.NewQualificationRuleReference(qualification.String())
		if err != nil {
			return psports.IntakeEligibility{}, false, fmt.Errorf("%w: qualification reference: %v",
				ErrUntranslatableAnswer, err)
		}
		proof, err := adapter.evidence.ProveIntakeQualification(
			ctx, identity, source, rule, source.OccurredAt(),
		)
		if err != nil {
			return psports.IntakeEligibility{}, false, fmt.Errorf("intake qualification evidence: %w", err)
		}
		switch proof {
		case psports.IntakeQualificationProven:
			continue
		case psports.IntakeQualificationUnproven:
			basis, err := psdomain.NewCheckReason("INTAKE_QUALIFICATION_UNPROVEN/" + qualification.String())
			if err != nil {
				return psports.IntakeEligibility{}, false, fmt.Errorf("unproven basis: %w", err)
			}
			return psports.IntakeEligibility{
				Outcome: psports.IntakeEligibilityNotEstablished,
				Basis:   basis,
			}, true, nil
		default:
			return psports.IntakeEligibility{}, false, fmt.Errorf(
				"%w: intake qualification proof %d", ErrUntranslatableAnswer, proof)
		}
	}
	return psports.IntakeEligibility{Outcome: psports.IntakeEligibilityEstablished}, true, nil
}

// JudgeFinalOutcome 按声明判终局：责任结果有声明行即满足并带声明的终局类型；缺行
// 即不满足带依据（此规则包下这种结果不形成终局——那是声明的真话，编排保持未决等其他
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

// JudgeCancellationAuthority 按目录判取消授权。目录按接受时采用的授权规则说话，
// 不按包裹发明不同授权（parcel 入参只为满足消费方端口）。
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

// declaredOutcomeFor 逐格翻译责任结果：网络服务各格与面单渠道服务两格（PS 的「非取消终局结果 / 终局失败结果」
// 对提供方词汇表的 LABEL_SERVICE_COMPLETED / LABEL_SERVICE_FAILED）。两边同一个词根不是同一个词，翻译只在这里。
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
	case psdomain.LabelServiceOutcome:
		return pcdomain.DeclaredLabelServiceCompleted, nil
	case psdomain.LabelServiceFailure:
		return pcdomain.DeclaredLabelServiceFailed, nil
	default:
		return pcdomain.DeclaredResponsibilityOutcomeInvalid, fmt.Errorf(
			"%w: responsibility outcome kind %d", ErrUntranslatableAnswer, kind)
	}
}
