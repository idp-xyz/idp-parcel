package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// FormETAOutcome 是 ETA 预测与可见性缺口两个入口共用的应用处理结果。预测与缺口分格：
// 一个是版本化时间预测，一个是「预期观察届满未得」的判断，下游各走各的链。
type FormETAOutcome uint8

const (
	FormETAOutcomeInvalid FormETAOutcome = iota
	ETAFormed
	ETARefreshed
	ETAExistingResult
	GapFormed
	GapWindowNotElapsed
	GapExistingResult
	FormETAUndecided
	FormETANotAccepted
)

func (outcome FormETAOutcome) String() string {
	switch outcome {
	case ETAFormed:
		return "ETA_FORMED"
	case ETARefreshed:
		return "ETA_REFRESHED"
	case ETAExistingResult:
		return "ETA_EXISTING_RESULT"
	case GapFormed:
		return "GAP_FORMED"
	case GapWindowNotElapsed:
		return "GAP_WINDOW_NOT_ELAPSED"
	case GapExistingResult:
		return "GAP_EXISTING_RESULT"
	case FormETAUndecided:
		return "UNDECIDED"
	case FormETANotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// FormETAUndecidedReason 指名本轮停在哪一步。
type FormETAUndecidedReason uint8

const (
	FormETAUndecidedReasonNone FormETAUndecidedReason = iota
	ETAStoreUnavailable
	ETAIdentityUnavailable
	VisibilityGapStoreUnavailable
)

func (reason FormETAUndecidedReason) String() string {
	switch reason {
	case ETAStoreUnavailable:
		return "ETA_STORE_UNAVAILABLE"
	case ETAIdentityUnavailable:
		return "ETA_IDENTITY_UNAVAILABLE"
	case VisibilityGapStoreUnavailable:
		return "VISIBILITY_GAP_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// FormETACommand 携带一次预测的全部业务事实（版本标识由编排签发，预测时间取时钟）。
// 各件缺一在门口即未受理——只有计划时间凑不齐输入与模型，领域构造器与这道门共同保证
// 「不用计划填充」。租户显式随命令到达（ADR-0003）：包裹引用只在租户内唯一。
type FormETACommand struct {
	TenantID   domain.TenantID
	Parcel     domain.TrackedParcelReference
	Milestone  domain.MilestoneReference
	Source     domain.ETASourceKind
	Inputs     domain.PredictionInputsReference
	Model      domain.PredictionModelReference
	RangeFrom  time.Time
	RangeTo    time.Time
	Confidence domain.ConfidenceReference
}

// FormVisibilityGapCommand 携带一次缺口判断请求：明确预期的观察、版本化窗口规则与
// 窗口截止。届满与否由编排按时钟判断——请求方说了不算。租户显式随命令到达
// （ADR-0003）。
type FormVisibilityGapCommand struct {
	TenantID    domain.TenantID
	Parcel      domain.TrackedParcelReference
	Expectation domain.ExpectedObservationReference
	WindowRule  domain.ObservationWindowReference
	WindowEnd   time.Time
}

type FormETAResult struct {
	outcome    FormETAOutcome
	eta        domain.ETAPrediction
	hasETA     bool
	gap        domain.VisibilityGap
	hasGap     bool
	reason     FormETAUndecidedReason
	handoffRef string
}

func (result FormETAResult) Outcome() FormETAOutcome {
	return result.outcome
}

// ETA 只在预测成立（本轮或此前）时给出。
func (result FormETAResult) ETA() (domain.ETAPrediction, bool) {
	return result.eta, result.hasETA
}

// Gap 只在缺口成立（本轮或此前）时给出。窗口未届满没有缺口可给——那不是一个空缺口，
// 是根本没有缺口。
func (result FormETAResult) Gap() (domain.VisibilityGap, bool) {
	return result.gap, result.hasGap
}

func (result FormETAResult) UndecidedReason() FormETAUndecidedReason {
	return result.reason
}

// HandoffReference 非空说明结果已成立但意图还没交出去，重放会重发同一份。
func (result FormETAResult) HandoffReference() string {
	return result.handoffRef
}

type FormETADeps struct {
	Predictions ports.ETAStore
	Gaps        ports.VisibilityGapStore
	Identities  ports.ETAIdentityFactory
	ViewChain   ports.ETAHandoff
	SignalChain ports.VisibilityGapHandoff
	Clock       ports.Clock
}

type FormETAHandler struct {
	deps FormETADeps
}

func NewFormETAHandler(deps FormETADeps) *FormETAHandler {
	return &FormETAHandler{deps: deps}
}

// FormETA 把一组预测事实推进到版本化 ETA：受理（各件缺一即未受理）→ 幂等按（包裹+
// 里程碑+输入版本），同输入不重形成只重发同一份意图 → 输入变化经 Refresh 换版指回
// 前版，历史预测不覆盖 → 意图交客户视图链（ETA 新版本是视图重派生的触发之一）。
// 预测不是承诺——领域类型上没有承诺、路由计划或实际时间的字段，这里也不碰它们。
func (handler *FormETAHandler) FormETA(
	ctx context.Context,
	command FormETACommand,
) (FormETAResult, error) {
	if command.TenantID.String() == "" ||
		command.Parcel.String() == "" ||
		command.Milestone.String() == "" ||
		command.Source.String() == "" ||
		command.Inputs.String() == "" ||
		command.Model.String() == "" ||
		command.Confidence.String() == "" ||
		command.RangeFrom.IsZero() ||
		command.RangeTo.IsZero() {
		return FormETAResult{outcome: FormETANotAccepted}, nil
	}

	current, found, err := handler.deps.Predictions.FindCurrent(ctx, command.TenantID, command.Parcel, command.Milestone)
	if err != nil {
		return FormETAResult{outcome: FormETAUndecided, reason: ETAStoreUnavailable}, nil
	}
	if found && current.Inputs() == command.Inputs {
		// 同（包裹+里程碑+输入版本）重放：不重形成，把同一份意图再交一次（ADR-0043）。
		return FormETAResult{
			outcome:    ETAExistingResult,
			eta:        current,
			hasETA:     true,
			handoffRef: handler.handOffETA(ctx, command.TenantID, current),
		}, nil
	}

	version, err := handler.deps.Identities.NextETAVersionID(ctx)
	if err != nil {
		return FormETAResult{outcome: FormETAUndecided, reason: ETAIdentityUnavailable}, nil
	}
	spec := domain.ETAPredictionSpec{
		Version:     version,
		Parcel:      command.Parcel,
		Milestone:   command.Milestone,
		Source:      command.Source,
		Inputs:      command.Inputs,
		Model:       command.Model,
		RangeFrom:   command.RangeFrom,
		RangeTo:     command.RangeTo,
		Confidence:  command.Confidence,
		PredictedAt: handler.deps.Clock.Now(),
	}

	var prediction domain.ETAPrediction
	outcome := ETAFormed
	if found {
		// 输入版本变了：新预测换版本指回前版，历史预测不覆盖。
		prediction, err = current.Refresh(spec)
		outcome = ETARefreshed
	} else {
		prediction, err = domain.FormETAPrediction(spec)
	}
	if err != nil {
		return FormETAResult{}, fmt.Errorf("form eta prediction: %w", err)
	}
	if err := handler.deps.Predictions.Save(ctx, command.TenantID, prediction); err != nil {
		return FormETAResult{outcome: FormETAUndecided, reason: ETAStoreUnavailable}, nil
	}
	return FormETAResult{
		outcome:    outcome,
		eta:        prediction,
		hasETA:     true,
		handoffRef: handler.handOffETA(ctx, command.TenantID, prediction),
	}, nil
}

// FormVisibilityGap 把一次缺口判断请求推进到缺口或如实的「未成」：窗口届满才成缺口
// （领域构造器已钉，这里按时钟给出届满事实——编排不提前宣告），未届满不造缺口、不落
// 库、不发意图；已成立的缺口按身份三维幂等，重放只重发同一份意图给信号链。
func (handler *FormETAHandler) FormVisibilityGap(
	ctx context.Context,
	command FormVisibilityGapCommand,
) (FormETAResult, error) {
	if command.TenantID.String() == "" ||
		command.Parcel.String() == "" ||
		command.Expectation.String() == "" ||
		command.WindowRule.String() == "" ||
		command.WindowEnd.IsZero() {
		return FormETAResult{outcome: FormETANotAccepted}, nil
	}

	existing, found, err := handler.deps.Gaps.FindCurrent(ctx, command.TenantID, command.Parcel, command.Expectation, command.WindowRule)
	if err != nil {
		return FormETAResult{outcome: FormETAUndecided, reason: VisibilityGapStoreUnavailable}, nil
	}
	if found {
		return FormETAResult{
			outcome:    GapExistingResult,
			gap:        existing,
			hasGap:     true,
			handoffRef: handler.handOffGap(ctx, command.TenantID, existing),
		}, nil
	}

	now := handler.deps.Clock.Now()
	if !now.After(command.WindowEnd) {
		// 「等一等」与「缺口」是两回事：未届满如实答未成，不形成缺口也不留半个占位。
		return FormETAResult{outcome: GapWindowNotElapsed}, nil
	}
	gap, err := domain.FormVisibilityGap(
		command.Parcel,
		command.Expectation,
		command.WindowRule,
		command.WindowEnd,
		now,
	)
	if err != nil {
		return FormETAResult{}, fmt.Errorf("form visibility gap: %w", err)
	}
	if err := handler.deps.Gaps.Save(ctx, command.TenantID, gap); err != nil {
		return FormETAResult{outcome: FormETAUndecided, reason: VisibilityGapStoreUnavailable}, nil
	}
	return FormETAResult{
		outcome:    GapFormed,
		gap:        gap,
		hasGap:     true,
		handoffRef: handler.handOffGap(ctx, command.TenantID, gap),
	}, nil
}

// handOffETA 把新预测版本交给客户视图链，交不出去时交回发布续办引用（ADR-0043）。
func (handler *FormETAHandler) handOffETA(ctx context.Context, tenant domain.TenantID, eta domain.ETAPrediction) string {
	if err := handler.deps.ViewChain.HandOffETA(ctx, ports.ETAHandoffIntent{
		TenantID:   tenant,
		Prediction: eta,
	}); err != nil {
		return "CONT-" + shortDigest("ETA_HANDOFF", eta.Version().String())
	}
	return ""
}

// handOffGap 把已成立的缺口交给信号链，交不出去时交回发布续办引用（ADR-0043）。
// 缺口是否命中异常规则由分诊判断，这里不替它判。
func (handler *FormETAHandler) handOffGap(ctx context.Context, tenant domain.TenantID, gap domain.VisibilityGap) string {
	if err := handler.deps.SignalChain.HandOffVisibilityGap(ctx, ports.VisibilityGapHandoffIntent{
		TenantID: tenant,
		Gap:      gap,
	}); err != nil {
		return "CONT-" + shortDigest(
			"VISIBILITY_GAP_HANDOFF",
			gap.Parcel().String(),
			gap.Expectation().String(),
			gap.WindowRule().String(),
		)
	}
	return ""
}
