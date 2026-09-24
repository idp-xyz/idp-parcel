package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// SupplementOutcome 是受控补充（`AT-PS-036` 第一支）的应用处理结果。没有一个取值是接受
// 判决：新版本落地后委托仍为`已提交`，重新判断由既有的判断编排按新任务推进。
//
// `基准过期`与`来源冲突`分开：前者是客户在补充一个已被换代的版本（去读当前版本再来），
// 后者是同一补充请求身份携带了不同内容（先说清到底提交的是哪一份）。`成员集合变更`是
// 确定的业务拒绝——增删拆并归关联新委托，复核也翻不了案。
type SupplementOutcome uint8

const (
	SupplementOutcomeInvalid SupplementOutcome = iota
	SupplementRecorded
	SupplementAlreadyHandled
	SupplementSourceConflict
	SupplementBasisStale
	SupplementBoundaryChanged
	SupplementDecisionAlreadyFormed
	SupplementUndecided
)

func (outcome SupplementOutcome) String() string {
	switch outcome {
	case SupplementRecorded:
		return "SUPPLEMENT_RECORDED"
	case SupplementAlreadyHandled:
		return "SUPPLEMENT_ALREADY_HANDLED"
	case SupplementSourceConflict:
		return "SUPPLEMENT_SOURCE_CONFLICT"
	case SupplementBasisStale:
		return "SUPPLEMENT_BASIS_STALE"
	case SupplementBoundaryChanged:
		return "SUPPLEMENT_BOUNDARY_CHANGED"
	case SupplementDecisionAlreadyFormed:
		return "SUPPLEMENT_DECISION_ALREADY_FORMED"
	case SupplementUndecided:
		return "SUPPLEMENT_UNDECIDED"
	default:
		return ""
	}
}

// FormNewSubmissionVersionCommand 携带一次受控补充。补充请求有自己的来源身份
// （SupplementIdentity），与定位委托的原提交身份分开——同一客户就同一份委托先提交后
// 补充，是两次来源请求（先例：资料修订与撤回）。
//
// BasisVersion 是客户认为正在补充的那个版本：基准不同即形成明确答案而不是静默换代，
// 「资料版本提交顺序由明确的领域顺序、基础版本关系裁决」（UC-PS-002 一致性）在提交版本
// 这一层同样成立。
type FormNewSubmissionVersionCommand struct {
	Identity           domain.SourceIdentity
	SupplementIdentity domain.SourceIdentity
	PayloadDigest      domain.PayloadDigest
	OccurredAt         time.Time
	ReceivedAt         time.Time
	ShipmentRequestID  domain.ShipmentRequestID
	BasisVersion       domain.SubmissionVersionID
	DeclaredParcelIDs  []domain.DeclaredParcelID
	// DeclaredProfiles 随新版本重报的成员声明画像（ADR-0048），允许缺席或部分覆盖。
	DeclaredProfiles []domain.DeclaredParcelProfile
	// DeclaredElements 随新版本重报的寄 / 收两段地址要素（pp-seams/05），同画像纪律不从旧版本继承：接受基线可以
	// 落在这一版上，读口从它自己的子段读值。与 PayloadDigest 同出接单入口的一次 CanonicalizeSubmission。
	DeclaredElements domain.DeclaredAddressElements
	// RequestedServiceProduct 随新版本重报的服务产品声明（票 psb/17），同 DeclaredElements 纪律不继承。
	RequestedServiceProduct domain.DeclaredServiceProduct
}

type FormNewSubmissionVersionResult struct {
	outcome      SupplementOutcome
	version      domain.SubmissionVersion
	hasVersion   bool
	decision     domain.AcceptanceDecision
	hasDecision  bool
	reason       JudgmentPendingReason
	continuation domain.OwnershipContinuationReference
}

func (result FormNewSubmissionVersionResult) Outcome() SupplementOutcome {
	return result.outcome
}

// Version 只在新版本已形成（或重放读回原版本）时给出。未形成一律不带：交回一个零值
// 版本，下游会按一份不存在的版本重新判断。
func (result FormNewSubmissionVersionResult) Version() (domain.SubmissionVersion, bool) {
	return result.version, result.hasVersion
}

// AcceptanceDecision 只在决定已越过提交边界时给出：补充来晚了，调用方要读的是那份决定，
// 再按其方向走资料修订或关联新委托。
func (result FormNewSubmissionVersionResult) AcceptanceDecision() (domain.AcceptanceDecision, bool) {
	return result.decision, result.hasDecision
}

func (result FormNewSubmissionVersionResult) PendingReason() JudgmentPendingReason {
	return result.reason
}

func (result FormNewSubmissionVersionResult) ContinuationReference() domain.OwnershipContinuationReference {
	return result.continuation
}

// FormNewSubmissionVersionDeps 收拢协作方。不设第四个授权端口：受控补充走客户接入渠道，
// 渠道认证属 `PAR-INT-01`（闸门），没有硬句要求为它再立业务授权；操作员人工纠错另有硬句
// （原因+操作审计），那条路实现时再立自己的端口。
type FormNewSubmissionVersionDeps struct {
	Sources    ports.SourceSubmissionRepository
	Requests   ports.ShipmentRequestRepository
	Identities ports.SubmissionIdentityFactory
	Clock      ports.Clock
}

type FormNewSubmissionVersionHandler struct {
	deps FormNewSubmissionVersionDeps
}

func NewFormNewSubmissionVersionHandler(deps FormNewSubmissionVersionDeps) *FormNewSubmissionVersionHandler {
	return &FormNewSubmissionVersionHandler{deps: deps}
}

// Handle 让客户在`已提交`委托上以受控补充形成同一委托的新提交版本（ADR-0045 的编排半边）。
func (handler *FormNewSubmissionVersionHandler) Handle(
	ctx context.Context,
	command FormNewSubmissionVersionCommand,
) (FormNewSubmissionVersionResult, error) {
	incoming, err := domain.NewSourceSubmissionFingerprint(
		command.SupplementIdentity,
		command.PayloadDigest,
		command.OccurredAt,
		command.ReceivedAt,
	)
	if err != nil {
		return FormNewSubmissionVersionResult{}, fmt.Errorf("preserve supplement source: %w", err)
	}
	preserved, arrived, err := handler.deps.Sources.FindPreserved(ctx, command.SupplementIdentity)
	if err != nil {
		return FormNewSubmissionVersionResult{}, fmt.Errorf("find preserved supplement source: %w", err)
	}
	if arrived {
		if preserved.Digest() != incoming.Digest() {
			// 同一补充身份携带不同内容是请求冲突：原请求不被覆盖，也不形成第二个版本。
			return FormNewSubmissionVersionResult{outcome: SupplementSourceConflict}, nil
		}
		if result, resolved := handler.resolveReplay(ctx, command); resolved {
			return result, nil
		}
		// 保全过但版本没办完：接着办，不再保全一遍。
	} else if err := handler.deps.Sources.Preserve(ctx, incoming); err != nil {
		return FormNewSubmissionVersionResult{}, fmt.Errorf("preserve supplement source: %w", err)
	}

	request, found, err := handler.deps.Requests.FindBySourceIdentity(ctx, command.Identity)
	if err != nil {
		return handler.undecided(command, ShipmentRequestUnavailable), nil
	}
	if !found {
		// 与形成决定、撤回、修订同一判断：指名一份查不到的委托是调用方的错。接 HTTP 时
		// 映射`统一不可见结果`，理由见 form_acceptance_decision.go。
		return FormNewSubmissionVersionResult{}, fmt.Errorf("form new submission version: %w", domain.ErrInvalidShipmentRequest)
	}

	// 决定已越过提交边界时先短路：一份已决定的委托不该再消耗版本与任务身份（先例：撤回的
	// 同一短路）。补充来晚了要读的是那份决定，再按方向走资料修订或关联新委托。
	if decision, formed := request.AcceptanceDecision(); formed || request.State() != domain.ShipmentRequestSubmitted {
		return FormNewSubmissionVersionResult{
			outcome:     SupplementDecisionAlreadyFormed,
			decision:    decision,
			hasDecision: formed,
		}, nil
	}

	// 基准过期是明确答案不是静默换代：客户补充的是一个已被换代的版本，先去读当前版本。
	if command.BasisVersion != request.CurrentSubmissionVersion().VersionID() {
		return FormNewSubmissionVersionResult{outcome: SupplementBasisStale}, nil
	}

	versionID, err := handler.deps.Identities.NextSubmissionVersionID(ctx)
	if err != nil {
		return handler.undecided(command, SubmissionIdentityUnavailable), nil
	}
	taskID, err := handler.deps.Identities.NextAcceptanceDecisionTaskID(ctx)
	if err != nil {
		return handler.undecided(command, SubmissionIdentityUnavailable), nil
	}

	superseded, err := request.FormNewSubmissionVersion(domain.NewSubmissionVersionSpec{
		VersionID:         versionID,
		TaskID:            taskID,
		SourceSubmission:  incoming,
		DeclaredParcelIDs: command.DeclaredParcelIDs,
		Profiles:          command.DeclaredProfiles,
		Elements:          command.DeclaredElements,
		RequestedProduct:  command.RequestedServiceProduct,
		EstablishedAt:     handler.deps.Clock.Now(),
	})
	if err != nil {
		if errors.Is(err, domain.ErrSubmissionBoundaryChanged) {
			// 确定的业务拒绝：增删拆并归关联新委托，复核也翻不了案。
			return FormNewSubmissionVersionResult{outcome: SupplementBoundaryChanged}, nil
		}
		if errors.Is(err, domain.ErrDecisionAlreadyFormed) {
			decision, formed := request.AcceptanceDecision()
			return FormNewSubmissionVersionResult{
				outcome:     SupplementDecisionAlreadyFormed,
				decision:    decision,
				hasDecision: formed,
			}, nil
		}
		return FormNewSubmissionVersionResult{}, fmt.Errorf("form new submission version: %w", err)
	}

	saved, err := handler.deps.Requests.Save(ctx, command.Identity, superseded)
	if err != nil {
		// 新版本没落库就不算形成：交回`已记录`，判断编排会去推进一个查不回来的任务。
		return handler.undecided(command, SupplementedRequestNotSaved), nil
	}
	if saved != ports.ShipmentRequestSaved {
		reason, err := saveStallReason(saved)
		if err != nil {
			return FormNewSubmissionVersionResult{}, err
		}
		return handler.undecided(command, reason), nil
	}

	return FormNewSubmissionVersionResult{
		outcome:    SupplementRecorded,
		version:    superseded.CurrentSubmissionVersion(),
		hasVersion: true,
	}, nil
}

// resolveReplay 回答一个已经到过的补充请求。版本若已挂在委托的任何一代上就交回它——
// 「同一逻辑请求以相同内容重试返回原结果，不创建第二个版本」；哪一代都找不到说明上一轮
// 在版本落库前停了，交回未解决让主路径接着办。
func (handler *FormNewSubmissionVersionHandler) resolveReplay(
	ctx context.Context,
	command FormNewSubmissionVersionCommand,
) (FormNewSubmissionVersionResult, bool) {
	request, found, err := handler.deps.Requests.FindBySourceIdentity(ctx, command.Identity)
	if err != nil || !found {
		return FormNewSubmissionVersionResult{}, false
	}
	if request.CurrentSubmissionVersion().SourceSubmission().Identity() == command.SupplementIdentity {
		return FormNewSubmissionVersionResult{
			outcome:    SupplementAlreadyHandled,
			version:    request.CurrentSubmissionVersion(),
			hasVersion: true,
		}, true
	}
	for _, prior := range request.PriorSubmissionVersions() {
		if prior.SourceSubmission().Identity() == command.SupplementIdentity {
			return FormNewSubmissionVersionResult{
				outcome:    SupplementAlreadyHandled,
				version:    prior,
				hasVersion: true,
			}, true
		}
	}
	return FormNewSubmissionVersionResult{}, false
}

func (handler *FormNewSubmissionVersionHandler) undecided(
	command FormNewSubmissionVersionCommand,
	reason JudgmentPendingReason,
) FormNewSubmissionVersionResult {
	return FormNewSubmissionVersionResult{
		outcome: SupplementUndecided,
		reason:  reason,
		continuation: judgmentContinuation(
			reason,
			command.Identity.TenantID().String(),
			command.Identity.CustomerAccountID().String(),
			command.ShipmentRequestID.String(),
			command.BasisVersion.String(),
			command.SupplementIdentity.RequestKey().String(),
		),
	}
}
