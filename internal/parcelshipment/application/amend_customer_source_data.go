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
	AmendmentAlreadyHandled
	AmendmentSourceConflict
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
	case AmendmentAlreadyHandled:
		return "ALREADY_HANDLED"
	case AmendmentSourceConflict:
		return "SOURCE_CONFLICT"
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
	// 先看这个来源身份是不是已经到过。放到业务判断之后再查就晚了：那时授权与规则已经重跑
	// 过一遍，而重跑正是重放要挡住的事。
	preserved, arrived, err := handler.deps.Sources.FindPreserved(ctx, command.AmendmentIdentity)
	if err != nil {
		return AmendCustomerSourceDataResult{}, fmt.Errorf("find preserved amendment source: %w", err)
	}
	if arrived {
		return handler.resolvePreserved(ctx, command, preserved, incoming)
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

// resolvePreserved 回答一个来源身份已经到过的修订请求。
//
// 同身份同内容是重复到达：追加一次观察，再读回它当初形成的那份版本，绝不重跑授权与规则。重跑
// 不只是浪费——授权与登记矩阵在两次之间都可能变，同一个请求身份会因此读出两种回执，而用例要求
// 重试「返回已有结果」，且「不能创建第二个资料版本」。
//
// 同身份不同内容是请求冲突：原请求与原版本都不被覆盖，也不形成第二份版本。它与重复到达分成两
// 个结果，因为客户要做的事不同——一个是这次请求已经办过了，一个是先说清到底提交的是哪一份。
// `occurredAt`/`receivedAt` 的差异不落在这里：它们不进内容摘要，因此只会被判成重复到达。
func (handler *AmendCustomerSourceDataHandler) resolvePreserved(
	ctx context.Context,
	command AmendCustomerSourceDataCommand,
	preserved domain.SourceSubmissionFingerprint,
	incoming domain.SourceSubmissionFingerprint,
) (AmendCustomerSourceDataResult, error) {
	classification, err := domain.ClassifySourceSubmission(preserved, incoming)
	if err != nil {
		return AmendCustomerSourceDataResult{}, fmt.Errorf("classify amendment source: %w", err)
	}
	if classification == domain.SourceConflict {
		return AmendCustomerSourceDataResult{outcome: AmendmentSourceConflict}, nil
	}

	if err := handler.deps.Sources.AppendObservation(ctx, incoming); err != nil {
		return AmendCustomerSourceDataResult{}, fmt.Errorf("append amendment source observation: %w", err)
	}

	request, found, err := handler.deps.Requests.FindBySourceIdentity(ctx, command.Identity)
	if err != nil {
		return AmendCustomerSourceDataResult{}, fmt.Errorf("find shipment request: %w", err)
	}
	if !found {
		return AmendCustomerSourceDataResult{}, fmt.Errorf("amend customer source data: %w", domain.ErrInvalidShipmentRequest)
	}
	return handler.existing(request, command.AmendmentIdentity), nil
}

// existing 读回这次修订请求当初形成的那份版本。版本自带形成它的那次请求指纹，所以不必另存一张
// 请求到版本的映射——留痕本身就是索引，而多存一张映射就多一处会与版本对不上的地方。
//
// 找不到版本时照样交回`已有结果`，只是不带版本：上一轮可能停在`待复核`或业务拒绝，那些同样是
// 已经形成的结果。这里不重跑一遍去补个版本出来，重跑正是本路径要挡住的事。
func (handler *AmendCustomerSourceDataHandler) existing(
	request domain.ShipmentRequest,
	amendment domain.SourceIdentity,
) AmendCustomerSourceDataResult {
	for _, version := range request.CustomerSourceDataVersions() {
		if version.Request().Identity() != amendment {
			continue
		}
		// 采用判断按版本自己的范围读，不按本次命令的范围：交回的是那份版本，说的就该是它
		// 所在范围此刻的采用判断。
		adoption, derived := request.CurrentSourceDataAdoption(version.Scope())
		return AmendCustomerSourceDataResult{
			outcome:     AmendmentAlreadyHandled,
			version:     version,
			hasVersion:  true,
			adoption:    adoption,
			hasAdoption: derived,
		}
	}
	return AmendCustomerSourceDataResult{outcome: AmendmentAlreadyHandled}
}
