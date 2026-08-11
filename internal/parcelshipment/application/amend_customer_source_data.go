package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// AmendmentOutcome 是修订这一步的应用处理结果。
type AmendmentOutcome uint8

const (
	AmendmentOutcomeInvalid AmendmentOutcome = iota
	AmendmentRecorded
	AmendmentAwaitingReview
	AmendmentNotAuthorized
	AmendmentDisallowed
)

func (outcome AmendmentOutcome) String() string {
	switch outcome {
	case AmendmentRecorded:
		return "RECORDED"
	case AmendmentAwaitingReview:
		return "AWAITING_REVIEW"
	case AmendmentNotAuthorized:
		return "NOT_AUTHORIZED"
	case AmendmentDisallowed:
		return "DISALLOWED"
	default:
		return ""
	}
}

type AmendCustomerSourceDataCommand struct {
	// Identity 是产生委托的那一次提交的来源身份，用来找到目标委托。
	Identity domain.SourceIdentity
	// AmendmentIdentity 是修订请求自己的来源身份，与 Identity 分开的理由同撤回：合用一个
	// 会让修订被判成原提交的重放。
	AmendmentIdentity domain.SourceIdentity
	PayloadDigest     domain.PayloadDigest
	OccurredAt        time.Time
	ReceivedAt        time.Time
	Scope             domain.SourceDataScope
	Basis             domain.SourceDataBasis
	Reason            domain.AmendmentReasonReference
	Requester         domain.RequesterReference
	// EffectiveAt 是客户声明的资料适用时间，可以缺失。缺失时保持零值，绝不用 occurredAt
	// 或 receivedAt 顶替——`UC-PS-002` 明禁那种补齐。
	EffectiveAt time.Time
}

type AmendCustomerSourceDataResult struct {
	outcome     AmendmentOutcome
	version     domain.CustomerSourceDataVersion
	hasVersion  bool
	adoption    domain.SourceDataAdoptionJudgment
	hasAdoption bool
}

func (result AmendCustomerSourceDataResult) Outcome() AmendmentOutcome {
	return result.outcome
}

func (result AmendCustomerSourceDataResult) Version() (domain.CustomerSourceDataVersion, bool) {
	return result.version, result.hasVersion
}

// Adoption 是本次修订之后该范围上的当前采用判断。它与版本分开交回：版本是刚形成的这一份，
// 采用判断说的是下游此刻该消费哪一份，两者在分叉时并不是同一个。
func (result AmendCustomerSourceDataResult) Adoption() (domain.SourceDataAdoptionJudgment, bool) {
	return result.adoption, result.hasAdoption
}

type AmendCustomerSourceDataDeps struct {
	Sources    ports.SourceSubmissionRepository
	Requests   ports.ShipmentRequestRepository
	Authorizer ports.SourceDataAmendmentAuthorizer
	Rules      ports.SourceDataRuleDeclaration
	Identities ports.SourceDataVersionIdentity
	Clock      ports.Clock
}

type AmendCustomerSourceDataHandler struct {
	deps AmendCustomerSourceDataDeps
}

func NewAmendCustomerSourceDataHandler(deps AmendCustomerSourceDataDeps) *AmendCustomerSourceDataHandler {
	return &AmendCustomerSourceDataHandler{deps: deps}
}

// Handle 让客户或其授权代表在已接受委托上形成一份客户原始资料新版本。
func (handler *AmendCustomerSourceDataHandler) Handle(
	ctx context.Context,
	command AmendCustomerSourceDataCommand,
) (AmendCustomerSourceDataResult, error) {
	incoming, err := domain.NewSourceSubmissionFingerprint(
		command.AmendmentIdentity,
		command.PayloadDigest,
		command.OccurredAt,
		command.ReceivedAt,
	)
	if err != nil {
		return AmendCustomerSourceDataResult{}, fmt.Errorf("preserve amendment source: %w", err)
	}
	if err := handler.deps.Sources.Preserve(ctx, incoming); err != nil {
		return AmendCustomerSourceDataResult{}, fmt.Errorf("preserve amendment source: %w", err)
	}

	request, found, err := handler.deps.Requests.FindBySourceIdentity(ctx, command.Identity)
	if err != nil {
		return AmendCustomerSourceDataResult{}, fmt.Errorf("find shipment request: %w", err)
	}
	if !found {
		return AmendCustomerSourceDataResult{}, fmt.Errorf("amend customer source data: %w", domain.ErrInvalidShipmentRequest)
	}

	authorization, err := handler.deps.Authorizer.AuthorizeSourceDataAmendment(
		ctx,
		ports.SourceDataAmendmentAuthorizationQuery{
			Identity:  command.Identity,
			Scope:     command.Scope,
			Requester: command.Requester,
			Reason:    command.Reason,
		},
	)
	if err != nil {
		return AmendCustomerSourceDataResult{}, fmt.Errorf("authorize source data amendment: %w", err)
	}
	if authorization.Authority.String() == "" {
		// 未获授权是确定的业务答案，不是未决：续办也补不出授权来，重试只会得到同一个答案。
		// 它与「授权服务答不出」分属两回事，后者在上面那一格作为错误交回。
		return AmendCustomerSourceDataResult{outcome: AmendmentNotAuthorized}, nil
	}

	allowance, err := handler.deps.Rules.DeclareSourceDataAmendment(ctx, ports.SourceDataAmendmentQuery{
		Identity: command.Identity,
		Scope:    command.Scope,
		Reason:   command.Reason,
	})
	if err != nil {
		return AmendCustomerSourceDataResult{}, fmt.Errorf("declare source data amendment: %w", err)
	}
	if allowance != ports.SourceDataAmendmentAllowed {
		// 闸门对齐到`允许`这一格：`UC-PS-002`「未登记时只能形成未决或业务拒绝，不能以系统
		// 便利推断允许」，所以只有明确登记为允许才继续，其余一律不形成版本也不签发标识。
		//
		// 停下之后两种答案要分开。已登记为不允许是确定的业务拒绝，复核也翻不了案；未登记是
		// 「还没人说这处资料能不能改」，停在`待复核`等矩阵登记——判成拒绝会让客户以为自己
		// 请求有错，而错的是我们还没登记规则。请求到达过这件事由上面的来源保全承担。
		outcome := AmendmentAwaitingReview
		if allowance == ports.SourceDataAmendmentDisallowed {
			outcome = AmendmentDisallowed
		}
		return AmendCustomerSourceDataResult{outcome: outcome}, nil
	}

	versionID, err := handler.deps.Identities.NextSourceDataVersionID(ctx)
	if err != nil {
		return AmendCustomerSourceDataResult{}, fmt.Errorf("next source data version ID: %w", err)
	}

	version, err := domain.FormCustomerSourceDataVersion(domain.CustomerSourceDataVersionSpec{
		VersionID:   versionID,
		Scope:       command.Scope,
		Basis:       command.Basis,
		Request:     incoming,
		Reason:      command.Reason,
		Requester:   command.Requester,
		Decider:     authorization.Decider,
		Authority:   authorization.Authority,
		EffectiveAt: command.EffectiveAt,
		FormedAt:    handler.deps.Clock.Now(),
	})
	if err != nil {
		return AmendCustomerSourceDataResult{}, fmt.Errorf("form customer source data version: %w", err)
	}

	amended, err := request.AmendCustomerSourceData(version)
	if err != nil {
		return AmendCustomerSourceDataResult{}, fmt.Errorf("amend customer source data: %w", err)
	}
	if err := handler.deps.Requests.Save(ctx, command.Identity, amended); err != nil {
		return AmendCustomerSourceDataResult{}, fmt.Errorf("save amended shipment request: %w", err)
	}

	adoption, derived := amended.CurrentSourceDataAdoption(command.Scope)
	return AmendCustomerSourceDataResult{
		outcome:     AmendmentRecorded,
		version:     version,
		hasVersion:  true,
		adoption:    adoption,
		hasAdoption: derived,
	}, nil
}
