package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件编排计价参考目录的登记与复核用例（ADR-0109 Decision 二）。两个用例与序列那两个逐格同形，
// 但不合并：命令与答案代数各是各的，合并后装配点可以接错一格而编译仍绿。

var (
	// ErrUnexpectedReferenceCatalogueOutcome 说明目录登记册交回了结果代数以外的写入结果。
	ErrUnexpectedReferenceCatalogueOutcome = errors.New("parcel pricing: unexpected reference catalogue registration outcome")
	// ErrUnexpectedCatalogueReviewOutcome 说明目录复核册交回了结果代数以外的写入结果。
	ErrUnexpectedCatalogueReviewOutcome = errors.New("parcel pricing: unexpected reference catalogue review outcome")
)

// RegisterReferenceCatalogueOutcome 是目录登记请求的应用处理结果。
type RegisterReferenceCatalogueOutcome uint8

const (
	RegisterReferenceCatalogueOutcomeInvalid RegisterReferenceCatalogueOutcome = iota
	// ReferenceCatalogueRecorded：新目录版本已入册。
	ReferenceCatalogueRecorded
	// ReferenceCatalogueAlreadyOnRegister：同版本同内容已在册，幂等重放。
	ReferenceCatalogueAlreadyOnRegister
	// ReferenceCatalogueRegistrationConflict：同版本引用装了不同映射。更正必须形成新版本，原行不被顶替。
	ReferenceCatalogueRegistrationConflict
	// ReferenceCatalogueRegistrationIncomparable：同版本引用已按另一套规范化形状在册，摘要不可比。
	ReferenceCatalogueRegistrationIncomparable
	// ReferenceCatalogueRegistrationNotAccepted：请求不合法（零值登记），未到达登记册。
	ReferenceCatalogueRegistrationNotAccepted
	// ReferenceCatalogueRegistrationUndecided：依赖故障，登记与否未知。
	ReferenceCatalogueRegistrationUndecided
)

func (outcome RegisterReferenceCatalogueOutcome) String() string {
	switch outcome {
	case ReferenceCatalogueRecorded:
		return "RECORDED"
	case ReferenceCatalogueAlreadyOnRegister:
		return "ALREADY_REGISTERED"
	case ReferenceCatalogueRegistrationConflict:
		return "CONTENT_CONFLICT"
	case ReferenceCatalogueRegistrationIncomparable:
		return "CANONICALIZATION_DIFFERS"
	case ReferenceCatalogueRegistrationNotAccepted:
		return "NOT_ACCEPTED"
	case ReferenceCatalogueRegistrationUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// RegisterReferenceCatalogueCommand 携带一次目录登记请求，登记本体全在领域对象内。
type RegisterReferenceCatalogueCommand struct {
	Registration domain.ReferenceCatalogueRegistration
}

type RegisterReferenceCatalogueDeps struct {
	Register ports.ReferenceCatalogueRegister
}

type RegisterReferenceCatalogueHandler struct {
	deps RegisterReferenceCatalogueDeps
}

func NewRegisterReferenceCatalogueHandler(deps RegisterReferenceCatalogueDeps) *RegisterReferenceCatalogueHandler {
	return &RegisterReferenceCatalogueHandler{deps: deps}
}

// Handle 把一次登记请求推进到登记册答案。依赖故障不吞：登记是治理动作，操作者要拿到原因才能续办。
func (handler *RegisterReferenceCatalogueHandler) Handle(
	ctx context.Context,
	command RegisterReferenceCatalogueCommand,
) (RegisterReferenceCatalogueOutcome, error) {
	if command.Registration.Tenant().String() == "" {
		return ReferenceCatalogueRegistrationNotAccepted, nil
	}
	saved, err := handler.deps.Register.Register(ctx, command.Registration)
	if err != nil {
		return ReferenceCatalogueRegistrationUndecided, fmt.Errorf("register reference catalogue: %w", err)
	}
	switch saved {
	case ports.ReferenceCatalogueRegistered:
		return ReferenceCatalogueRecorded, nil
	case ports.ReferenceCatalogueAlreadyRegistered:
		return ReferenceCatalogueAlreadyOnRegister, nil
	case ports.ReferenceCatalogueContentConflict:
		return ReferenceCatalogueRegistrationConflict, nil
	case ports.ReferenceCatalogueCanonicalizationDiffers:
		return ReferenceCatalogueRegistrationIncomparable, nil
	default:
		return RegisterReferenceCatalogueOutcomeInvalid, fmt.Errorf("%w: %d", ErrUnexpectedReferenceCatalogueOutcome, saved)
	}
}

// ReviewReferenceCatalogueOutcome 是目录版本复核请求的应用处理结果，各格与序列复核同义。
type ReviewReferenceCatalogueOutcome uint8

const (
	ReviewReferenceCatalogueOutcomeInvalid ReviewReferenceCatalogueOutcome = iota
	CatalogueReviewRecorded
	CatalogueReviewAlreadyOnRegister
	CatalogueReviewConflict
	CatalogueReviewVersionUnknown
	// CatalogueReviewNeedsAnotherReviewer：租户的参考复核职责规则要求不同主体，而复核责任方就是登记责任方——换人，不是改字段。
	CatalogueReviewNeedsAnotherReviewer
	// CatalogueReviewNotConfigured：租户没有登记参考复核职责规则，复核不放行——租户先登规则。
	CatalogueReviewNotConfigured
	// CatalogueReviewReviewerNotQualified：规则要求复核者持某一格授予而他没有；受控批量口带不出授予集，一律落这一格。
	CatalogueReviewReviewerNotQualified
	CatalogueReviewNotAccepted
	CatalogueReviewUndecided
)

func (outcome ReviewReferenceCatalogueOutcome) String() string {
	switch outcome {
	case CatalogueReviewRecorded:
		return "RECORDED"
	case CatalogueReviewAlreadyOnRegister:
		return "ALREADY_RECORDED"
	case CatalogueReviewConflict:
		return "CONFLICT"
	case CatalogueReviewVersionUnknown:
		return "VERSION_UNKNOWN"
	case CatalogueReviewNeedsAnotherReviewer:
		return "NEEDS_ANOTHER_REVIEWER"
	case CatalogueReviewNotConfigured:
		return "NOT_CONFIGURED"
	case CatalogueReviewReviewerNotQualified:
		return "REVIEWER_NOT_QUALIFIED"
	case CatalogueReviewNotAccepted:
		return "NOT_ACCEPTED"
	case CatalogueReviewUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// ReviewReferenceCatalogueCommand 按（目录标识、版本号）指名被复核的版本；复核时刻可缺，缺时取时钟当下。ReviewerGrants 同序列
// 复核那一格：操作者面取自信封，受控批量口留空。目录没有来源喂价，也就没有系统代写那一格。
type ReviewReferenceCatalogueCommand struct {
	Tenant           domain.TenantID
	CatalogueID      string
	CatalogueVersion string
	Reviewer         string
	ReviewerGrants   []domain.OperatorGrant
	Decision         domain.SeriesReviewDecision
	Basis            string
	ReviewedAt       time.Time
}

// ReviewReferenceCatalogueDeps 是目录复核的依赖。Registered 与 PricedScopes 在一版目录通过复核时按绑了它的卡交「参数已登记」，
// 理由同序列复核；DutyRules 是与序列复核共用的那一本参考复核职责规则。
type ReviewReferenceCatalogueDeps struct {
	Versions     ports.ReferenceCatalogueVersionLoader
	Reviews      ports.ReferenceCatalogueReviewRegister
	DutyRules    ports.ReferenceReviewDutyRuleView
	Clock        ports.Clock
	Registered   ports.PricingParameterRegisteredHandoff
	PricedScopes ports.PricingParameterBindingView
}

// ErrCatalogueReviewHandoffMissing 说明装配没给目录复核接「参数已登记」交接口或绑定读口。
var ErrCatalogueReviewHandoffMissing = errors.New("parcel pricing: reference catalogue review has no pricing parameter registered handoff")

type ReviewReferenceCatalogueHandler struct {
	deps ReviewReferenceCatalogueDeps
}

func NewReviewReferenceCatalogueHandler(deps ReviewReferenceCatalogueDeps) *ReviewReferenceCatalogueHandler {
	return &ReviewReferenceCatalogueHandler{deps: deps}
}

// Handle 把一次复核请求推进到复核册答案：取登记（规则要比登记责任方）→ 读租户的参考复核职责规则 → 领域构造 → 追加。规则
// 未登记即`未配置`、不落复核行（ADR-0262 决定一）。
func (handler *ReviewReferenceCatalogueHandler) Handle(
	ctx context.Context,
	command ReviewReferenceCatalogueCommand,
) (ReviewReferenceCatalogueOutcome, error) {
	if command.Tenant.String() == "" || command.CatalogueID == "" || command.CatalogueVersion == "" {
		return CatalogueReviewNotAccepted, nil
	}
	if handler.deps.Registered == nil || handler.deps.PricedScopes == nil {
		return CatalogueReviewUndecided, ErrCatalogueReviewHandoffMissing
	}
	if handler.deps.DutyRules == nil {
		return CatalogueReviewUndecided, ErrReferenceReviewDutyRuleViewMissing
	}
	registration, found, err := handler.deps.Versions.LoadVersion(ctx, command.Tenant, command.CatalogueID, command.CatalogueVersion)
	if err != nil {
		return CatalogueReviewUndecided, fmt.Errorf("review reference catalogue: %w", err)
	}
	if !found {
		return CatalogueReviewVersionUnknown, nil
	}
	rule, configured, err := handler.deps.DutyRules.LoadReferenceReviewDutyRule(ctx, command.Tenant)
	if err != nil {
		return CatalogueReviewUndecided, fmt.Errorf("review reference catalogue: %w", err)
	}
	if !configured {
		return CatalogueReviewNotConfigured, nil
	}
	reviewer, err := domain.NewOperatorSubject(command.Reviewer, command.ReviewerGrants)
	if err != nil {
		return CatalogueReviewNotAccepted, nil
	}
	reviewedAt := command.ReviewedAt
	if reviewedAt.IsZero() {
		reviewedAt = handler.deps.Clock.Now()
	}
	review, err := domain.NewCatalogueReview(registration, reviewer, rule, reviewedAt, command.Decision, command.Basis)
	switch {
	case errors.Is(err, domain.ErrCatalogueReviewerIsRegistrant):
		return CatalogueReviewNeedsAnotherReviewer, nil
	case errors.Is(err, domain.ErrReferenceReviewerLacksRequiredGrant):
		return CatalogueReviewReviewerNotQualified, nil
	case errors.Is(err, domain.ErrInvalidCatalogueReview):
		return CatalogueReviewNotAccepted, nil
	case err != nil:
		// 规则不是本租户的、或读回的规则立不住：读口或装配的错，不是业务答案。
		return CatalogueReviewUndecided, fmt.Errorf("review reference catalogue: %w", err)
	}
	saved, err := handler.deps.Reviews.Record(ctx, review)
	if err != nil {
		return CatalogueReviewUndecided, fmt.Errorf("review reference catalogue: %w", err)
	}
	switch saved {
	case ports.ReferenceCatalogueReviewRecorded:
		return handler.announce(ctx, review, CatalogueReviewRecorded)
	case ports.ReferenceCatalogueReviewAlreadyRecorded:
		return handler.announce(ctx, review, CatalogueReviewAlreadyOnRegister)
	case ports.ReferenceCatalogueReviewConflict:
		return CatalogueReviewConflict, nil
	case ports.ReferenceCatalogueReviewVersionUnknown:
		return CatalogueReviewVersionUnknown, nil
	default:
		return ReviewReferenceCatalogueOutcomeInvalid, fmt.Errorf("%w: %d", ErrUnexpectedCatalogueReviewOutcome, saved)
	}
}

// announce 只在复核通过时交「参数已登记」，理由同 ReviewReferenceSeriesHandler.announce。
func (handler *ReviewReferenceCatalogueHandler) announce(
	ctx context.Context,
	review domain.CatalogueReview,
	outcome ReviewReferenceCatalogueOutcome,
) (ReviewReferenceCatalogueOutcome, error) {
	if review.Decision() != domain.SeriesReviewApproved {
		return outcome, nil
	}
	if err := announceBoundPricingParameter(ctx, handler.deps.PricedScopes, handler.deps.Registered,
		review.Tenant(), ports.PricingParameterReferenceCatalogue, review.Reference()); err != nil {
		return CatalogueReviewUndecided, fmt.Errorf("review reference catalogue: %w", err)
	}
	return outcome, nil
}
