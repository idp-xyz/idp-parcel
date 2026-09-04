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

// ReviewReferenceCatalogueOutcome 是目录版本复核请求的应用处理结果，七格与序列复核同义。
type ReviewReferenceCatalogueOutcome uint8

const (
	ReviewReferenceCatalogueOutcomeInvalid ReviewReferenceCatalogueOutcome = iota
	CatalogueReviewRecorded
	CatalogueReviewAlreadyOnRegister
	CatalogueReviewConflict
	CatalogueReviewVersionUnknown
	// CatalogueReviewNeedsAnotherReviewer：复核责任方就是登记责任方，四眼门拒——换人，不是改字段。
	CatalogueReviewNeedsAnotherReviewer
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
	case CatalogueReviewNotAccepted:
		return "NOT_ACCEPTED"
	case CatalogueReviewUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// ReviewReferenceCatalogueCommand 按（目录标识、版本号）指名被复核的版本；复核时刻可缺，缺时取时钟当下。
type ReviewReferenceCatalogueCommand struct {
	Tenant           domain.TenantID
	CatalogueID      string
	CatalogueVersion string
	Reviewer         string
	Decision         domain.SeriesReviewDecision
	Basis            string
	ReviewedAt       time.Time
}

type ReviewReferenceCatalogueDeps struct {
	Versions ports.ReferenceCatalogueVersionLoader
	Reviews  ports.ReferenceCatalogueReviewRegister
	Clock    ports.Clock
}

type ReviewReferenceCatalogueHandler struct {
	deps ReviewReferenceCatalogueDeps
}

func NewReviewReferenceCatalogueHandler(deps ReviewReferenceCatalogueDeps) *ReviewReferenceCatalogueHandler {
	return &ReviewReferenceCatalogueHandler{deps: deps}
}

// Handle 把一次复核请求推进到复核册答案：取登记（四眼门要拿到登记责任方）→ 领域构造 → 追加。
func (handler *ReviewReferenceCatalogueHandler) Handle(
	ctx context.Context,
	command ReviewReferenceCatalogueCommand,
) (ReviewReferenceCatalogueOutcome, error) {
	if command.Tenant.String() == "" || command.CatalogueID == "" || command.CatalogueVersion == "" {
		return CatalogueReviewNotAccepted, nil
	}
	registration, found, err := handler.deps.Versions.LoadVersion(ctx, command.Tenant, command.CatalogueID, command.CatalogueVersion)
	if err != nil {
		return CatalogueReviewUndecided, fmt.Errorf("review reference catalogue: %w", err)
	}
	if !found {
		return CatalogueReviewVersionUnknown, nil
	}
	reviewedAt := command.ReviewedAt
	if reviewedAt.IsZero() {
		reviewedAt = handler.deps.Clock.Now()
	}
	review, err := domain.NewCatalogueReview(registration, command.Reviewer, reviewedAt, command.Decision, command.Basis)
	switch {
	case errors.Is(err, domain.ErrCatalogueReviewerIsRegistrant):
		return CatalogueReviewNeedsAnotherReviewer, nil
	case err != nil:
		return CatalogueReviewNotAccepted, nil
	}
	saved, err := handler.deps.Reviews.Record(ctx, review)
	if err != nil {
		return CatalogueReviewUndecided, fmt.Errorf("review reference catalogue: %w", err)
	}
	switch saved {
	case ports.ReferenceCatalogueReviewRecorded:
		return CatalogueReviewRecorded, nil
	case ports.ReferenceCatalogueReviewAlreadyRecorded:
		return CatalogueReviewAlreadyOnRegister, nil
	case ports.ReferenceCatalogueReviewConflict:
		return CatalogueReviewConflict, nil
	case ports.ReferenceCatalogueReviewVersionUnknown:
		return CatalogueReviewVersionUnknown, nil
	default:
		return ReviewReferenceCatalogueOutcomeInvalid, fmt.Errorf("%w: %d", ErrUnexpectedCatalogueReviewOutcome, saved)
	}
}
