// Package networkrouting 是 parcel-shipment 对 network-routing 的消费侧适配器——第三个
// 跨上下文适配器（ADR-0025：只有本包可以同时导入两个上下文；只翻译不判断，翻译必须是
// 全函数）。
package networkrouting

import (
	"context"
	"errors"
	"fmt"

	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ErrUntranslatableAnswer 与另两个适配器的同名哨兵同义：某一侧交出了词汇表之外的内容，
// 是编程错误不是业务答案。
var ErrUntranslatableAnswer = errors.New("parcel shipment networkrouting adapter: untranslatable answer")

// 服务目的未配置时本适配器交回的停摆原因。目的由服务产品定义（NR 的 ServicePurpose 词条），
// 没有租户就没有产品映射——那是实例半边，不冠 NR- 前缀。
const reasonPurposeNotConfigured = "REACHABILITY_PURPOSE_NOT_CONFIGURED"

// ReachabilityAdapter 把 parcel-shipment 的可达性两个端口（形成判断与提交前重校）接到
// network-routing 的应用编排上。
//
// Purpose 是装配期给定的服务目的引用（实例半边）：零值是「显式未配置」的诚实表达，届时
// 形成停在`未形成`、重校停在`无法判定`——不代拟一个目的去问另一个产品的网络资格。
type ReachabilityAdapter struct {
	assess     *nrapplication.AssessParcelReachabilityHandler
	revalidate *nrapplication.ValidateReachabilityJudgmentHandler
	purpose    nrdomain.ServicePurpose
}

type ReachabilityAdapterDeps struct {
	Assess     *nrapplication.AssessParcelReachabilityHandler
	Revalidate *nrapplication.ValidateReachabilityJudgmentHandler
	Purpose    nrdomain.ServicePurpose
}

func NewReachabilityAdapter(deps ReachabilityAdapterDeps) *ReachabilityAdapter {
	return &ReachabilityAdapter{
		assess:     deps.Assess,
		revalidate: deps.Revalidate,
		purpose:    deps.Purpose,
	}
}

var (
	_ psports.ReachabilityAssessor    = (*ReachabilityAdapter)(nil)
	_ psports.ReachabilityRevalidator = (*ReachabilityAdapter)(nil)
)

// correlationIdentity 是交给提供方的请求关联，同时充当译回本上下文的判断标识——与 SA
// 适配器的 controlRequestIdentity 同一路数：提供方按关联持有中间状态（ADR-0027）。
// 解析标识在关联里。重校回指形成时记下的那一串，不另造编号。
func correlationIdentity(
	shipmentRequestID psdomain.ShipmentRequestID,
	submissionVersion psdomain.SubmissionVersionID,
	parcelID psdomain.DeclaredParcelID,
	resolution psdomain.CommercialResolutionID,
) string {
	identity := shipmentRequestID.String() + "/" + submissionVersion.String() + "/" + parcelID.String()
	// 解析标识不进判断键。把它留在关联外面，同键重放会交回旧判断，换过的闭包不再被问到。
	if resolution.String() == "" {
		return identity
	}
	return identity + "/" + resolution.String()
}

// AssessParcelReachability 执行形成半边：关联带上这一轮的解析，判断键不带。交提供方形成
// （或找回）三值判断，再译回本上下文的判断引用。
func (adapter *ReachabilityAdapter) AssessParcelReachability(
	ctx context.Context,
	request psports.ReachabilityRequest,
) (psports.ReachabilityAssessment, error) {
	if !adapter.purpose.Valid() {
		reason, err := psdomain.NewCheckReason(reasonPurposeNotConfigured)
		if err != nil {
			return psports.ReachabilityAssessment{}, fmt.Errorf("%w: check reason: %v", ErrUntranslatableAnswer, err)
		}
		return psports.ReachabilityAssessment{Outcome: psports.ReachabilityNotFormed, Reason: reason}, nil
	}
	command, err := adapter.assessCommandFor(request)
	if err != nil {
		return psports.ReachabilityAssessment{}, err
	}
	answer, err := adapter.assess.Handle(ctx, command)
	if err != nil {
		return psports.ReachabilityAssessment{}, fmt.Errorf("assess parcel reachability: %w", err)
	}
	return assessmentFor(request, command, answer)
}

// RevalidateReachabilityJudgment 执行重校半边：有记下的判断标识就按它回指原判断，
// 否则按委托、提交版本与包裹派生。提供方按视图修订确认或判换代，结果按全函数译回。
func (adapter *ReachabilityAdapter) RevalidateReachabilityJudgment(
	ctx context.Context,
	query psports.ReachabilityRevalidationQuery,
) (psports.ReachabilityRevalidation, error) {
	if !adapter.purpose.Valid() {
		reason, err := psdomain.NewCheckReason(reasonPurposeNotConfigured)
		if err != nil {
			return psports.ReachabilityRevalidation{}, fmt.Errorf("%w: check reason: %v", ErrUntranslatableAnswer, err)
		}
		return psports.ReachabilityRevalidation{
			Outcome: psports.ReachabilityRevalidationUndetermined,
			Reason:  reason,
		}, nil
	}
	key, err := adapter.judgmentKeyFor(query.Identity, query.ShipmentRequestID, query.SubmissionVersion, query.DeclaredParcelID, query.AsOf)
	if err != nil {
		return psports.ReachabilityRevalidation{}, err
	}
	correlation := correlationFor(query.ShipmentRequestID, query.SubmissionVersion, query.DeclaredParcelID, psdomain.CommercialResolutionID{})
	if query.JudgmentID.String() != "" {
		parsed, err := nrdomain.NewRequestCorrelationID(query.JudgmentID.String())
		if err != nil {
			return psports.ReachabilityRevalidation{}, fmt.Errorf("%w: judgment ID: %v", ErrUntranslatableAnswer, err)
		}
		correlation = parsed
	}
	answer, err := adapter.revalidate.Handle(ctx, nrapplication.ValidateReachabilityJudgmentCommand{
		Correlation: correlation,
		Key:         key,
	})
	if err != nil {
		return psports.ReachabilityRevalidation{}, fmt.Errorf("revalidate reachability judgment: %w", err)
	}

	switch answer.Outcome() {
	case nrapplication.JudgmentStillCurrent:
		return psports.ReachabilityRevalidation{Outcome: psports.ReachabilityJudgmentStillCurrent}, nil
	case nrapplication.JudgmentSuperseded:
		return psports.ReachabilityRevalidation{Outcome: psports.ReachabilityJudgmentSuperseded}, nil
	case nrapplication.JudgmentNotFound:
		return psports.ReachabilityRevalidation{Outcome: psports.ReachabilityRevalidationJudgmentNotFound}, nil
	case nrapplication.ValidationNotFormed:
		reason, err := psdomain.NewCheckReason("NR-" + answer.NotFormedReason().String())
		if err != nil {
			return psports.ReachabilityRevalidation{}, fmt.Errorf("%w: check reason: %v", ErrUntranslatableAnswer, err)
		}
		return psports.ReachabilityRevalidation{
			Outcome: psports.ReachabilityRevalidationUndetermined,
			Reason:  reason,
		}, nil
	case nrapplication.ValidationNotAccepted:
		return psports.ReachabilityRevalidation{Outcome: psports.ReachabilityRevalidationInputNotAccepted}, nil
	default:
		return psports.ReachabilityRevalidation{}, fmt.Errorf("%w: validation outcome %d",
			ErrUntranslatableAnswer, answer.Outcome())
	}
}

func (adapter *ReachabilityAdapter) assessCommandFor(
	request psports.ReachabilityRequest,
) (nrapplication.AssessParcelReachabilityCommand, error) {
	key, err := adapter.judgmentKeyFor(request.Identity, request.ShipmentRequestID, request.SubmissionVersion, request.DeclaredParcelID, request.AsOf)
	if err != nil {
		return nrapplication.AssessParcelReachabilityCommand{}, err
	}
	resolution, err := resolutionReference(request.Resolution)
	if err != nil {
		return nrapplication.AssessParcelReachabilityCommand{}, fmt.Errorf("%w: commercial resolution: %v", ErrUntranslatableAnswer, err)
	}
	return nrapplication.AssessParcelReachabilityCommand{
		Correlation: correlationFor(request.ShipmentRequestID, request.SubmissionVersion, request.DeclaredParcelID, request.Resolution),
		Key:         key,
		Resolution:  resolution,
	}, nil
}

// resolutionReference 把本上下文已经采用的解析标识译成提供方的命令附加字段。
// 零值留零值：提供方把空引用答成未形成，这里不代拟一个标识。非空却译不出是词汇表之外，上抛。
func resolutionReference(id psdomain.CommercialResolutionID) (nrdomain.CommercialResolutionReference, error) {
	if id.String() == "" {
		return nrdomain.CommercialResolutionReference{}, nil
	}
	return nrdomain.NewCommercialResolutionReference(id.String())
}

// judgmentKeyFor 把消费方标识译成提供方判断键。立不起来的部分译成零值交提供方短路作答
// （`未受理`）——与另两个适配器同一条纪律。时点三件套按已回显的判断时点直译。
func (adapter *ReachabilityAdapter) judgmentKeyFor(
	identity psdomain.SourceIdentity,
	shipmentRequestID psdomain.ShipmentRequestID,
	submissionVersion psdomain.SubmissionVersionID,
	parcelID psdomain.DeclaredParcelID,
	asOf psdomain.JudgmentAsOf,
) (nrdomain.ReachabilityJudgmentKey, error) {
	key := nrdomain.ReachabilityJudgmentKey{ServicePurpose: adapter.purpose}
	if tenant, err := nrdomain.NewTenantID(identity.TenantID().String()); err == nil {
		key.TenantID = tenant
	}
	if account, err := nrdomain.NewCustomerAccountID(identity.CustomerAccountID().String()); err == nil {
		key.CustomerAccountID = account
	}
	if requestID, err := nrdomain.NewShipmentRequestID(shipmentRequestID.String()); err == nil {
		key.ShipmentRequestID = requestID
	}
	if version, err := nrdomain.NewSubmissionVersionID(submissionVersion.String()); err == nil {
		key.SubmissionVersion = version
	}
	if parcel, err := nrdomain.NewDeclaredParcelID(parcelID.String()); err == nil {
		key.DeclaredParcelID = parcel
	}

	semantic, err := nrdomain.NewAsOfSemantic(asOf.Semantics().String())
	if err != nil {
		return nrdomain.ReachabilityJudgmentKey{}, fmt.Errorf("%w: as-of semantic: %v", ErrUntranslatableAnswer, err)
	}
	strategy, err := nrdomain.NewAsOfStrategyVersion(asOf.PolicyVersion().String())
	if err != nil {
		return nrdomain.ReachabilityJudgmentKey{}, fmt.Errorf("%w: as-of strategy version: %v", ErrUntranslatableAnswer, err)
	}
	judgmentAsOf, err := nrdomain.NewJudgmentAsOf(semantic, asOf.At(), strategy)
	if err != nil {
		return nrdomain.ReachabilityJudgmentKey{}, fmt.Errorf("%w: judgment as-of: %v", ErrUntranslatableAnswer, err)
	}
	key.AsOf = judgmentAsOf
	return key, nil
}

func correlationFor(
	shipmentRequestID psdomain.ShipmentRequestID,
	submissionVersion psdomain.SubmissionVersionID,
	parcelID psdomain.DeclaredParcelID,
	resolution psdomain.CommercialResolutionID,
) nrdomain.RequestCorrelationID {
	correlation, err := nrdomain.NewRequestCorrelationID(
		correlationIdentity(shipmentRequestID, submissionVersion, parcelID, resolution))
	if err != nil {
		return nrdomain.RequestCorrelationID{}
	}
	return correlation
}

// assessmentFor 是提供方形成答复到本上下文落点的全函数。`首次形成`与`重放既有`都落
// `已判断`——两者交回的是同一份判断；`不适用`同落`已判断`，分别由判断取值与所携依据
// 带过来（端口注释的原话，这里只是执行它）。
func assessmentFor(
	request psports.ReachabilityRequest,
	command nrapplication.AssessParcelReachabilityCommand,
	answer nrapplication.AssessParcelReachabilityResult,
) (psports.ReachabilityAssessment, error) {
	switch answer.Outcome() {
	case nrapplication.JudgmentFormed, nrapplication.ExistingJudgment:
		finding, present := answer.Finding()
		if !present {
			return psports.ReachabilityAssessment{}, fmt.Errorf("%w: formed answer carries no finding",
				ErrUntranslatableAnswer)
		}
		value, err := reachabilityValueFor(finding.Value())
		if err != nil {
			return psports.ReachabilityAssessment{}, err
		}
		judgment, err := judgmentFor(request, command.Correlation.String(), value, psdomain.ReachabilityBasisReference{})
		if err != nil {
			return psports.ReachabilityAssessment{}, err
		}
		return psports.ReachabilityAssessment{Outcome: psports.ReachabilityAssessed, Judgment: judgment}, nil
	case nrapplication.NotApplicable:
		basis, err := psdomain.NewReachabilityBasisReference(answer.EligibilityBasis().String())
		if err != nil {
			return psports.ReachabilityAssessment{}, fmt.Errorf("%w: eligibility basis: %v", ErrUntranslatableAnswer, err)
		}
		judgment, err := judgmentFor(request, "", psdomain.ReachabilityNotApplicable, basis)
		if err != nil {
			return psports.ReachabilityAssessment{}, err
		}
		return psports.ReachabilityAssessment{Outcome: psports.ReachabilityAssessed, Judgment: judgment}, nil
	case nrapplication.JudgmentNotFormed:
		reason, err := psdomain.NewCheckReason("NR-" + answer.NotFormedReason().String())
		if err != nil {
			return psports.ReachabilityAssessment{}, fmt.Errorf("%w: check reason: %v", ErrUntranslatableAnswer, err)
		}
		return psports.ReachabilityAssessment{Outcome: psports.ReachabilityNotFormed, Reason: reason}, nil
	case nrapplication.RequestConflict:
		return psports.ReachabilityAssessment{Outcome: psports.ReachabilityRequestConflict}, nil
	case nrapplication.RequestNotAccepted:
		return psports.ReachabilityAssessment{Outcome: psports.ReachabilityRequestNotAccepted}, nil
	default:
		return psports.ReachabilityAssessment{}, fmt.Errorf("%w: assessment outcome %d",
			ErrUntranslatableAnswer, answer.Outcome())
	}
}

// judgmentFor 构造消费方的判断引用。判断标识取请求关联（`不适用`不带标识——那一支下
// 提供方不形成判断，PS 构造器也正是这么要求的）；时点用请求里那一份已回显的判断时点：
// 提供方按键相等担保它判的就是这一时点，不再另发一份回显。
func judgmentFor(
	request psports.ReachabilityRequest,
	judgmentID string,
	value psdomain.ReachabilityValue,
	basis psdomain.ReachabilityBasisReference,
) (psdomain.ReachabilityJudgment, error) {
	spec := psdomain.ReachabilityJudgmentSpec{
		ParcelID: request.DeclaredParcelID,
		Value:    value,
		Basis:    basis,
		AsOf:     request.AsOf,
	}
	if judgmentID != "" {
		id, err := psdomain.NewReachabilityJudgmentID(judgmentID)
		if err != nil {
			return psdomain.ReachabilityJudgment{}, fmt.Errorf("%w: judgment ID: %v", ErrUntranslatableAnswer, err)
		}
		spec.JudgmentID = id
	}
	judgment, err := psdomain.NewReachabilityJudgment(spec)
	if err != nil {
		return psdomain.ReachabilityJudgment{}, fmt.Errorf("%w: reachability judgment: %v", ErrUntranslatableAnswer, err)
	}
	return judgment, nil
}

// reachabilityValueFor 逐格翻译提供方的三值结论。两边的封闭集合逐名对应，default 报错
// 不吸收。
func reachabilityValueFor(value nrdomain.ReachabilityValue) (psdomain.ReachabilityValue, error) {
	switch value {
	case nrdomain.Reachable:
		return psdomain.ReachabilityReachable, nil
	case nrdomain.Unreachable:
		return psdomain.ReachabilityUnreachable, nil
	case nrdomain.InsufficientEvidence:
		return psdomain.ReachabilityInsufficientEvidence, nil
	default:
		return psdomain.ReachabilityValueInvalid, fmt.Errorf("%w: finding value %d", ErrUntranslatableAnswer, value)
	}
}
