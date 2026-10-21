package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// ErrUnexpectedSeriesReviewOutcome 说明复核册交回了结果代数以外的写入结果。
var ErrUnexpectedSeriesReviewOutcome = errors.New("parcel pricing: unexpected reference series review outcome")

// ReviewReferenceSeriesOutcome 是序列版本复核请求的应用处理结果。
type ReviewReferenceSeriesOutcome uint8

const (
	ReviewReferenceSeriesOutcomeInvalid ReviewReferenceSeriesOutcome = iota
	// SeriesReviewRecorded：复核已追加；结论为通过时该版本自复核时刻起在用。
	SeriesReviewRecorded
	// SeriesReviewAlreadyOnRegister：同（版本、时刻、复核责任方）同内容已在册，幂等重放。
	SeriesReviewAlreadyOnRegister
	// SeriesReviewConflict：同键在册而结论或依据不同；原行不顶替，改主意另追加一条。
	SeriesReviewConflict
	// SeriesReviewVersionUnknown：被复核的版本不在册——先登记。
	SeriesReviewVersionUnknown
	// SeriesReviewNeedsAnotherReviewer：复核责任方就是登记责任方，四眼门拒。恢复动作是
	// 换一个人来，不是改字段，所以与 NotAccepted 分格。
	SeriesReviewNeedsAnotherReviewer
	// SeriesReviewNotAccepted：请求不合法（缺复核责任方、结论不在封闭集、缺依据等），
	// 未到达复核册。
	SeriesReviewNotAccepted
	// SeriesReviewUndecided：依赖故障，记录与否未知，原因随错误交回。
	SeriesReviewUndecided
)

func (outcome ReviewReferenceSeriesOutcome) String() string {
	switch outcome {
	case SeriesReviewRecorded:
		return "RECORDED"
	case SeriesReviewAlreadyOnRegister:
		return "ALREADY_RECORDED"
	case SeriesReviewConflict:
		return "CONFLICT"
	case SeriesReviewVersionUnknown:
		return "VERSION_UNKNOWN"
	case SeriesReviewNeedsAnotherReviewer:
		return "NEEDS_ANOTHER_REVIEWER"
	case SeriesReviewNotAccepted:
		return "NOT_ACCEPTED"
	case SeriesReviewUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// ReviewReferenceSeriesCommand 携带一次复核请求。被复核的版本按（序列标识、版本号）指名，
// 不带摘要——复核责任方手上只有这两样，摘要在登记里，读回之后由领域把复核钉到真引用上。
// 复核时刻可缺：缺时取时钟当下——复核就是复核责任方此刻作出的确认；受控批量口补录历史复核
// 时才显式给时刻。
type ReviewReferenceSeriesCommand struct {
	Tenant        domain.TenantID
	SeriesID      string
	SeriesVersion string
	Reviewer      string
	Decision      domain.SeriesReviewDecision
	Basis         string
	ReviewedAt    time.Time
}

// ReviewReferenceSeriesDeps 是序列复核的依赖。Registered 在一版序列通过复核（自复核时刻起在用）时交「参数已登记」：
// 结算那一侧停在序列读不出、缺换算步骤或笼统待判断上的请求由它续办（ADR-0229 决定五）。PricedScopes 答这条序列绑在哪些
// 卡上：信封按那几张卡的计价范围与方向各交一面，结算只续办那几个范围上的。两件都必备，理由同价卡登记。
type ReviewReferenceSeriesDeps struct {
	Versions     ports.ReferenceSeriesVersionLoader
	Reviews      ports.ReferenceSeriesReviewRegister
	Clock        ports.Clock
	Registered   ports.PricingParameterRegisteredHandoff
	PricedScopes ports.PricingParameterBindingView
}

// ErrSeriesReviewHandoffMissing 说明装配没给序列复核接「参数已登记」交接口或绑定读口。
var ErrSeriesReviewHandoffMissing = errors.New("parcel pricing: reference series review has no pricing parameter registered handoff")

// ReviewReferenceSeriesHandler 编排序列版本复核用例（ADR-0099 决定二）。四眼门与四件
// 齐备的判断在领域构造器（以被复核的登记为入参），冲突判定在复核册的结果代数，这里只
// 做受理、取登记与答案翻译。
type ReviewReferenceSeriesHandler struct {
	deps ReviewReferenceSeriesDeps
}

func NewReviewReferenceSeriesHandler(deps ReviewReferenceSeriesDeps) *ReviewReferenceSeriesHandler {
	return &ReviewReferenceSeriesHandler{deps: deps}
}

// Handle 把一次复核请求推进到复核册答案。依赖故障不吞：复核是治理动作，操作者必须拿到
// 失败原因才能续办。
func (handler *ReviewReferenceSeriesHandler) Handle(
	ctx context.Context,
	command ReviewReferenceSeriesCommand,
) (ReviewReferenceSeriesOutcome, error) {
	if command.Tenant.String() == "" || command.SeriesID == "" || command.SeriesVersion == "" {
		return SeriesReviewNotAccepted, nil
	}
	if handler.deps.Registered == nil || handler.deps.PricedScopes == nil {
		return SeriesReviewUndecided, ErrSeriesReviewHandoffMissing
	}

	registration, found, err := handler.deps.Versions.LoadVersion(ctx, command.Tenant, command.SeriesID, command.SeriesVersion)
	if err != nil {
		return SeriesReviewUndecided, fmt.Errorf("review reference series: %w", err)
	}
	if !found {
		return SeriesReviewVersionUnknown, nil
	}

	reviewedAt := command.ReviewedAt
	if reviewedAt.IsZero() {
		reviewedAt = handler.deps.Clock.Now()
	}
	review, err := domain.NewSeriesReview(registration, command.Reviewer, reviewedAt, command.Decision, command.Basis)
	switch {
	case errors.Is(err, domain.ErrSeriesReviewerIsRegistrant):
		return SeriesReviewNeedsAnotherReviewer, nil
	case err != nil:
		return SeriesReviewNotAccepted, nil
	}

	saved, err := handler.deps.Reviews.Record(ctx, review)
	if err != nil {
		return SeriesReviewUndecided, fmt.Errorf("review reference series: %w", err)
	}
	switch saved {
	case ports.ReferenceSeriesReviewRecorded:
		return handler.announce(ctx, review, SeriesReviewRecorded)
	case ports.ReferenceSeriesReviewAlreadyRecorded:
		return handler.announce(ctx, review, SeriesReviewAlreadyOnRegister)
	case ports.ReferenceSeriesReviewConflict:
		return SeriesReviewConflict, nil
	case ports.ReferenceSeriesReviewVersionUnknown:
		// 读回时还在、写时说不在——版本行只增不改，这一格在正常运行里到不了；照实翻译，
		// 不把它折成成功。
		return SeriesReviewVersionUnknown, nil
	default:
		return ReviewReferenceSeriesOutcomeInvalid, fmt.Errorf("%w: %d", ErrUnexpectedSeriesReviewOutcome, saved)
	}
}

// announce 只在复核通过时交「参数已登记」：退回的那一版不在用，续办再问一次得到的还是同一个答案。重放重发同一封，由
// 交接口按同一个标识去重。交接失败上抛，信封与复核行同生共死。
func (handler *ReviewReferenceSeriesHandler) announce(
	ctx context.Context,
	review domain.SeriesReview,
	outcome ReviewReferenceSeriesOutcome,
) (ReviewReferenceSeriesOutcome, error) {
	if review.Decision() != domain.SeriesReviewApproved {
		return outcome, nil
	}
	if err := announceBoundPricingParameter(ctx, handler.deps.PricedScopes, handler.deps.Registered,
		review.Tenant(), ports.PricingParameterReferenceSeries, review.Reference()); err != nil {
		return SeriesReviewUndecided, fmt.Errorf("review reference series: %w", err)
	}
	return outcome, nil
}
