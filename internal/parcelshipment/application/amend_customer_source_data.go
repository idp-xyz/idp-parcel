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
	AmendmentUndecided
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
	case AmendmentUndecided:
		return "UNDECIDED"
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
	outcome      AmendmentOutcome
	version      domain.CustomerSourceDataVersion
	hasVersion   bool
	adoption     domain.SourceDataAdoptionJudgment
	hasAdoption  bool
	reason       JudgmentPendingReason
	continuation domain.OwnershipContinuationReference
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

// PendingReason 指名本轮为何没能形成结果。它是封闭取值而非自由文本，未决才按依赖阶段统计得
// 出来；同一个集合与决定期那几例共用，理由见 judgment_continuation.go。
func (result AmendCustomerSourceDataResult) PendingReason() JudgmentPendingReason {
	return result.reason
}

func (result AmendCustomerSourceDataResult) ContinuationReference() domain.OwnershipContinuationReference {
	return result.continuation
}

type AmendCustomerSourceDataDeps struct {
	Sources    ports.SourceSubmissionRepository
	Requests   ports.ShipmentRequestRepository
	Authorizer ports.SourceDataAmendmentAuthorizer
	Rules      ports.SourceDataRuleDeclaration
	Identities ports.SourceDataVersionIdentity
	Downstream ports.SourceDataVersionHandoff
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
		return handler.undecided(command, ShipmentRequestUnavailable), nil
	}
	if !found {
		// 命令指名了一份不存在的委托。这不是依赖答不出，而是调用方对世界的判断就是错的，
		// 因此上抛而不是形成未决。接 HTTP 时它必须映射为`统一不可见结果`，理由与出处见
		// form_acceptance_decision.go：否定结果不区分「不存在」与「属于别的租户」。
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
		// 授权服务答不出不是「这个人不能改」：后者续办补不出授权来，前者重试就好。判成未获
		// 授权会让客户以为自己越权，而真相是我们没问到。
		return handler.undecided(command, SourceDataAmendmentAuthorityUnavailable), nil
	}
	if authorization.Authority.String() == "" {
		// 未获授权是确定的业务答案，不是未决：续办也补不出授权来，重试只会得到同一个答案。
		// 它与「授权服务答不出」分属两回事，后者在上面那一格作为错误交回。
		return AmendCustomerSourceDataResult{outcome: AmendmentNotAuthorized}, nil
	}

	// 步骤 5 排在步骤 6 之前，不是可换的次序：接受基线自己就是成员集合的权威，`AT-PS-023`
	// 这一拒不需要任何已登记目录。放到矩阵查询之后，一次矩阵读不回就会把它变成未决，客户被
	// 告知「等依赖恢复」，而真相是这个请求无论矩阵怎么登记都不成立。
	if request.SourceDataScopeOutsideAcceptanceBaseline(command.Scope) {
		return AmendCustomerSourceDataResult{outcome: AmendmentDisallowed}, nil
	}

	allowance, err := handler.deps.Rules.DeclareSourceDataAmendment(ctx, ports.SourceDataAmendmentQuery{
		Identity: command.Identity,
		Scope:    command.Scope,
		Reason:   command.Reason,
	})
	if err != nil {
		// 矩阵读不回与矩阵答「没有这条」分开：后者等的是有人去 `PAR-COM-13` 登记，前者等的
		// 是依赖恢复。合成一格，续办方就不知道该催人还是该重试。
		return handler.undecided(command, SourceDataRuleUnavailable), nil
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
		return handler.undecided(command, SourceDataVersionIdentityUnavailable), nil
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
	saved, err := handler.deps.Requests.Save(ctx, command.Identity, amended)
	if err != nil {
		// 版本已形成但没落库。不交回`已记录并采用`：用例明禁「不得返回已采用或业务拒绝」，
		// 而下游按一份并不存在的版本办事会直接扑空。本轮的续办引用就是重放这次修订的凭据。
		return handler.undecided(command, AmendedRequestNotSaved), nil
	}
	if saved != ports.ShipmentRequestSaved {
		// 同上不交回`已记录并采用`。重放这次修订时来源保全会认出同一个身份，走 existing
		// 那条路——那时聚合已是新的一份，版本重新形成一次的风险由那条路的版本查找挡住。
		reason, err := saveStallReason(saved)
		if err != nil {
			return AmendCustomerSourceDataResult{}, err
		}
		return handler.undecided(command, reason), nil
	}

	adoption, derived := amended.CurrentSourceDataAdoption(command.Scope)
	if err := handler.handOff(ctx, command, version, adoption); err != nil {
		return handler.undecided(command, SourceDataVersionNotHandedOff), nil
	}

	return AmendCustomerSourceDataResult{
		outcome:     AmendmentRecorded,
		version:     version,
		hasVersion:  true,
		adoption:    adoption,
		hasAdoption: derived,
	}, nil
}

// handOff 执行步骤 9：把版本引用交给适用下游。
//
// 意图由版本标识认领，因此同一份版本无论交几次都是同一份意图，而不是第二份——`AT-PS-031`
// 「版本只形成一次；仅重试同一发布意图」正是这个意思。本上下文不记意图完没完成：那份状态要与
// 版本同一事务落库才算数，而事务与 outbox 仍阻断于 ADR-0017 的 Bento 闸门。在那之前重放一律
// 重发同一意图，由下游按版本标识认领，比记一份证明不了原子性的完成标志诚实。
// 版本与范围都取自那一份版本自己，不取本次命令：重放路径上交的是读回来的那一份，两处各取一边
// 会让意图指着一个范围、带着另一个范围的版本。
func (handler *AmendCustomerSourceDataHandler) handOff(
	ctx context.Context,
	command AmendCustomerSourceDataCommand,
	version domain.CustomerSourceDataVersion,
	adoption domain.SourceDataAdoptionJudgment,
) error {
	return handler.deps.Downstream.HandOffSourceDataVersion(ctx, ports.SourceDataVersionHandoffIntent{
		Identity: command.Identity,
		Version:  version.VersionID(),
		Scope:    version.Scope(),
		Adoption: adoption,
	})
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
		return handler.undecided(command, ShipmentRequestUnavailable), nil
	}
	if !found {
		// 同上一处：查不到是调用方的错，接 HTTP 时同样映射为`统一不可见结果`。
		return AmendCustomerSourceDataResult{}, fmt.Errorf("amend customer source data: %w", domain.ErrInvalidShipmentRequest)
	}
	return handler.existing(ctx, command, request), nil
}

// undecided 交回本轮的未决结果：封闭原因加一个续办引用，而不是一个 error——用例要求未决同时
// 给出原因与安全续办入口，两样 error 都给不出。
//
// 它不往接受判断任务上记处理尝试，这一点与决定期那几例不同：资料修订发生在委托`已接受`之后，
// 那件任务已经完成，把修订的未决追加上去会让一件办完的任务重新看起来卡着。
func (handler *AmendCustomerSourceDataHandler) undecided(
	command AmendCustomerSourceDataCommand,
	reason JudgmentPendingReason,
) AmendCustomerSourceDataResult {
	return AmendCustomerSourceDataResult{
		outcome:      AmendmentUndecided,
		reason:       reason,
		continuation: handler.continuationReference(command, reason),
	}
}

// continuationReference 由停下原因与本次修订的范围共同派生，因此同一次修订因同一原因停下时
// 拿到的引用始终相同，续办方据以查回原次尝试而不必靠猜。
//
// 范围取修订请求自己的身份加资料范围，不取委托加提交版本：同一份已接受委托上可以并行有多处
// 修订，按委托派生会让它们全部收敛到同一个引用，续办方就分不出该重放哪一次。
func (handler *AmendCustomerSourceDataHandler) continuationReference(
	command AmendCustomerSourceDataCommand,
	reason JudgmentPendingReason,
) domain.OwnershipContinuationReference {
	parcelID, _ := command.Scope.DeclaredParcelID()
	return judgmentContinuation(
		reason,
		command.AmendmentIdentity.TenantID().String(),
		command.AmendmentIdentity.CustomerAccountID().String(),
		command.AmendmentIdentity.RequestKey().String(),
		command.Scope.ShipmentRequestID().String(),
		parcelID.String(),
		command.Scope.DataGroup().String(),
	)
}

// existing 读回这次修订请求当初形成的那份版本。版本自带形成它的那次请求指纹，所以不必另存一张
// 请求到版本的映射——留痕本身就是索引，而多存一张映射就多一处会与版本对不上的地方。
//
// 找不到版本时照样交回`已有结果`，只是不带版本：上一轮可能停在`待复核`或业务拒绝，那些同样是
// 已经形成的结果，也没有版本可交给下游。这里不重跑一遍去补个版本出来，重跑正是本路径要挡住的事。
//
// 读回版本时重发同一份意图。上一轮可能正停在交接失败上，而这条路上没有别的东西会去补发——只答
// `已有结果`就收工，那份版本会永远停在「本上下文已形成、下游从不知道」。重发的是同一份而不是
// 第二份：意图由版本标识认领，而版本这一路只形成过一次。
func (handler *AmendCustomerSourceDataHandler) existing(
	ctx context.Context,
	command AmendCustomerSourceDataCommand,
	request domain.ShipmentRequest,
) AmendCustomerSourceDataResult {
	for _, version := range request.CustomerSourceDataVersions() {
		if version.Request().Identity() != command.AmendmentIdentity {
			continue
		}
		// 采用判断按版本自己的范围读，不按本次命令的范围：交回的是那份版本，说的就该是它
		// 所在范围此刻的采用判断。
		adoption, derived := request.CurrentSourceDataAdoption(version.Scope())
		if err := handler.handOff(ctx, command, version, adoption); err != nil {
			return handler.undecided(command, SourceDataVersionNotHandedOff)
		}
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
