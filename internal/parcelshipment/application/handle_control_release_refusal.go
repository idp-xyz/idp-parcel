package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ErrControlReleaseRefusalHandlingNotAssembled 说明处理编排少接了一个必需件。少接一个件等多久都不会长出来，所以响亮报错，不当作
// 未决，也不悄悄跳过交续办信封。
var ErrControlReleaseRefusalHandlingNotAssembled = errors.New("parcel shipment: control release refusal handling is not assembled")

// ErrControlReleaseRefusalActionUnknown 说明命令的动作不是封闭两值之一。
var ErrControlReleaseRefusalActionUnknown = errors.New("parcel shipment: control release refusal action is unknown")

// ControlReleaseRefusalAction 是处理一笔被拒收的释放的两种动作（ADR-0293 决定二）。
type ControlReleaseRefusalAction uint8

const (
	ControlReleaseRefusalActionInvalid ControlReleaseRefusalAction = iota
	// ReleaseControlReleaseRefusalAtSettlement：处理人核对后，经按原关联的释放编排再请求一次释放。提供方答已收口才算处理。
	ReleaseControlReleaseRefusalAtSettlement
	// MarkControlReleaseRefusalHandled：资金已在本产品之外处置，处理人只留痕。本方不替他判这是不是真的。
	MarkControlReleaseRefusalHandled
)

// ControlReleaseRefusalHandlingOutcome 是处理命令的封闭结果。
//
//   - `已处理`：这一次赢了，处理留痕与续办信封同笔交出。
//   - `已被处理`：这一笔先被别人处理了（查时已处理，或并发里输了），读回先到的那一份；不再问提供方、不留痕、不交第二封信封。
//   - `释放未收口`：提供方答可重试未形成（或读不回判断），什么也没写，稍后可再试。
//   - `释放再被拒收`：提供方又一次确定不受理（含本方译不动它的答复）。什么也没写；原因随结果带出。这一笔只能「标记已处理」，
//     或在结算侧把拒收的缘由解掉之后再试。
type ControlReleaseRefusalHandlingOutcome uint8

const (
	ControlReleaseRefusalHandlingOutcomeInvalid ControlReleaseRefusalHandlingOutcome = iota
	ControlReleaseRefusalHandled
	ControlReleaseRefusalAlreadyHandled
	ControlReleaseNotSettled
	ControlReleaseRefusedAgain
)

func (outcome ControlReleaseRefusalHandlingOutcome) String() string {
	switch outcome {
	case ControlReleaseRefusalHandled:
		return "HANDLED"
	case ControlReleaseRefusalAlreadyHandled:
		return "ALREADY_HANDLED"
	case ControlReleaseNotSettled:
		return "RELEASE_NOT_SETTLED"
	case ControlReleaseRefusedAgain:
		return "RELEASE_REFUSED_AGAIN"
	default:
		return ""
	}
}

// HandleControlReleaseRefusalCommand 指名册上的哪一笔、做哪一种处理，谁处理、凭什么。Identity 由接入面按认证出的租户与委托标识查出，
// 不从请求体收。
type HandleControlReleaseRefusalCommand struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	ControlResultID   domain.FinancialControlResultID
	Action            ControlReleaseRefusalAction
	Handler           domain.ControlReleaseRefusalHandlerReference
	Basis             domain.ControlReleaseRefusalHandlingBasis
}

type HandleControlReleaseRefusalResult struct {
	outcome     ControlReleaseRefusalHandlingOutcome
	handling    ports.ControlReleaseRefusalHandling
	hasHandling bool
	reason      domain.CheckReason
}

func (result HandleControlReleaseRefusalResult) Outcome() ControlReleaseRefusalHandlingOutcome {
	return result.outcome
}

// Handling 在`已处理`时是本次落下的留痕，在`已被处理`时是先到的那一份。
func (result HandleControlReleaseRefusalResult) Handling() (ports.ControlReleaseRefusalHandling, bool) {
	return result.handling, result.hasHandling
}

// Reason 只在`释放再被拒收`时在场：提供方给的原因，或本上下文译不动答复时自有的原因。
func (result HandleControlReleaseRefusalResult) Reason() domain.CheckReason {
	return result.reason
}

type HandleControlReleaseRefusalDeps struct {
	Requests  ports.ShipmentRequestRepository
	Refusals  ports.ControlReleaseRefusalRegister
	Handlings ports.ControlReleaseRefusalHandlings
	// Judgments、Release 与 ScopeLock 只有「在结算侧释放」这一支用：标记已处理不问提供方。Release 传带 savepoint 的那一只，
	// 释放口答类型化的可重试未形成而事务没中止时，提供方已落库的那一半留着（ADR-0291）。
	Judgments ports.RecordedJudgmentReader
	Release   ports.PreAcceptanceControlRelease
	ScopeLock ports.PreAcceptanceControlScopeLock
	// Handoff 交续办信封：与处理留痕同一事务（ADR-0094 决定四）。
	Handoff ports.ControlReleaseRefusalHandledHandoff
	Clock   ports.Clock
}

type HandleControlReleaseRefusalHandler struct {
	deps HandleControlReleaseRefusalDeps
}

func NewHandleControlReleaseRefusalHandler(deps HandleControlReleaseRefusalDeps) *HandleControlReleaseRefusalHandler {
	return &HandleControlReleaseRefusalHandler{deps: deps}
}

// Handle 处理释放未受理册上一笔被拒收的占用，并同笔让接受链重驱那份委托的当前版本（ADR-0293 决定二、四）。
//
// 它不保存委托，不持委托行锁：锁序是 S（占用所在作用域的锁，本编排在释放前先取，提供方释放里重入）→ 处理留痕 → 续办信封
// （ports.PreAcceptanceControlScopeLock 头注）。调用方把整个 Handle 放进一笔事务：留痕写、信封写与提供方账本上的释放同生共死，
// 返回错误即整笔回滚。
//
// 「在结算侧释放」复用按原关联放占用的那一处定义（occupationRelease），不另立人工释放入口：释放认哪一版、哪一笔、哪一份作用域，
// 与终局各路、接受链放的是同一笔，幂等归提供方账本（ADR-0122 决定三）。没存作用域的存量行交零值，提供方照旧拒收，处理人只剩
// 「标记已处理」一条路。
func (handler *HandleControlReleaseRefusalHandler) Handle(
	ctx context.Context,
	command HandleControlReleaseRefusalCommand,
) (HandleControlReleaseRefusalResult, error) {
	if err := handler.assembled(command.Action); err != nil {
		return HandleControlReleaseRefusalResult{}, err
	}
	kind, err := handlingKindOf(command.Action)
	if err != nil {
		return HandleControlReleaseRefusalResult{}, err
	}

	key := ports.ControlReleaseRefusalKey{
		TenantID:          command.Identity.TenantID(),
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
		ControlResultID:   command.ControlResultID,
	}
	if err := handler.requireRegistered(ctx, key); err != nil {
		return HandleControlReleaseRefusalResult{}, err
	}

	earlier, handled, err := handler.deps.Handlings.FindControlReleaseRefusalHandling(ctx, key)
	if err != nil {
		return HandleControlReleaseRefusalResult{}, fmt.Errorf("handle control release refusal: %w", err)
	}
	if handled {
		return alreadyHandled(earlier), nil
	}

	if command.Action == ReleaseControlReleaseRefusalAtSettlement {
		unsettled, err := handler.releaseAtSettlement(ctx, command)
		if err != nil {
			return HandleControlReleaseRefusalResult{}, err
		}
		if unsettled != nil {
			return *unsettled, nil
		}
	}

	saved, winner, err := handler.deps.Handlings.HandleControlReleaseRefusal(ctx, ports.ControlReleaseRefusalHandling{
		Key:       key,
		Kind:      kind,
		Handler:   command.Handler,
		Basis:     command.Basis,
		HandledAt: handler.deps.Clock.Now().UTC(),
	})
	if err != nil {
		return HandleControlReleaseRefusalResult{}, fmt.Errorf("handle control release refusal: %w", err)
	}
	switch saved {
	case ports.ControlReleaseRefusalHandlingRecorded:
	case ports.ControlReleaseRefusalAlreadyHandled:
		return alreadyHandled(winner), nil
	case ports.ControlReleaseRefusalNotRegistered:
		return HandleControlReleaseRefusalResult{}, fmt.Errorf("handle control release refusal: %w", ErrShipmentRequestNotVisible)
	default:
		return HandleControlReleaseRefusalResult{}, fmt.Errorf("handle control release refusal: handling save outcome %d is not a known outcome", saved)
	}

	request, found, err := handler.deps.Requests.FindBySourceIdentity(ctx, command.Identity)
	if err != nil {
		return HandleControlReleaseRefusalResult{}, fmt.Errorf("handle control release refusal: %w", err)
	}
	if !found || request.ShipmentRequestID() != command.ShipmentRequestID {
		// 册上有这一笔、委托却读不到或对不上：数据不一致，整笔失败让留痕回滚，不交一封指向别的委托的信封。
		return HandleControlReleaseRefusalResult{}, fmt.Errorf(
			"handle control release refusal: request %s of the refusal is not readable by its source identity", command.ShipmentRequestID)
	}
	if err := handler.deps.Handoff.HandOffControlReleaseRefusalHandled(ctx, ports.ControlReleaseRefusalHandledHandoffIntent{
		Identity: command.Identity,
		Request:  request,
		Handling: winner,
	}); err != nil {
		return HandleControlReleaseRefusalResult{}, fmt.Errorf("handle control release refusal: hand off resume: %w", err)
	}
	return HandleControlReleaseRefusalResult{
		outcome:     ControlReleaseRefusalHandled,
		handling:    winner,
		hasHandling: true,
	}, nil
}

func alreadyHandled(earlier ports.ControlReleaseRefusalHandling) HandleControlReleaseRefusalResult {
	return HandleControlReleaseRefusalResult{
		outcome:     ControlReleaseRefusalAlreadyHandled,
		handling:    earlier,
		hasHandling: true,
	}
}

func handlingKindOf(action ControlReleaseRefusalAction) (ports.ControlReleaseRefusalHandlingKind, error) {
	switch action {
	case ReleaseControlReleaseRefusalAtSettlement:
		return ports.ControlReleaseRefusalReleasedAtSettlement, nil
	case MarkControlReleaseRefusalHandled:
		return ports.ControlReleaseRefusalMarkedHandled, nil
	default:
		return ports.ControlReleaseRefusalHandlingKindInvalid, ErrControlReleaseRefusalActionUnknown
	}
}

func (handler *HandleControlReleaseRefusalHandler) assembled(action ControlReleaseRefusalAction) error {
	deps := handler.deps
	if deps.Requests == nil || deps.Refusals == nil || deps.Handlings == nil || deps.Handoff == nil || deps.Clock == nil {
		return ErrControlReleaseRefusalHandlingNotAssembled
	}
	if action == ReleaseControlReleaseRefusalAtSettlement &&
		(deps.Judgments == nil || deps.Release == nil || deps.ScopeLock == nil) {
		return ErrControlReleaseRefusalHandlingNotAssembled
	}
	return nil
}

// requireRegistered 核对册上有这一笔：册读口按租户切，别的租户的行、不存在的行与不同版本的行同样读不到，一律答「统一不可见」。
func (handler *HandleControlReleaseRefusalHandler) requireRegistered(ctx context.Context, key ports.ControlReleaseRefusalKey) error {
	refusals, err := handler.deps.Refusals.ListByShipmentRequest(ctx, key.TenantID, key.ShipmentRequestID)
	if err != nil {
		return fmt.Errorf("handle control release refusal: %w", err)
	}
	for _, refusal := range refusals {
		if ports.KeyOfRefusal(refusal) == key {
			return nil
		}
	}
	return fmt.Errorf("handle control release refusal: %w", ErrShipmentRequestNotVisible)
}

// releaseAtSettlement 按原关联再请求一次释放。交回 nil 即已收口；否则是要原样答给处理人的结果（什么也没写）。读不回判断、取不到锁
// 与判断对不上册行是错误，不折成释放没收口。
func (handler *HandleControlReleaseRefusalHandler) releaseAtSettlement(
	ctx context.Context,
	command HandleControlReleaseRefusalCommand,
) (*HandleControlReleaseRefusalResult, error) {
	recorded, err := handler.deps.Judgments.LoadRecordedJudgments(ctx, command.Identity.TenantID(), command.ShipmentRequestID, command.SubmissionVersion)
	if err != nil {
		return nil, fmt.Errorf("handle control release refusal: load recorded judgments: %w", err)
	}
	control := recorded.FinancialControl
	if control.ResultID() != command.ControlResultID {
		return nil, fmt.Errorf("handle control release refusal: version %s recorded control result %s, the refusal names %s",
			command.SubmissionVersion, control.ResultID(), command.ControlResultID)
	}

	release := occupationRelease{
		judgments: handler.deps.Judgments,
		release:   handler.deps.Release,
		identity:  command.Identity,
		request:   command.ShipmentRequestID,
		scopeLock: handler.deps.ScopeLock,
	}
	if err := release.lockControlScopes(ctx, []domain.FinancialControlResult{control}); err != nil {
		return nil, fmt.Errorf("handle control release refusal: lock control scope: %w", err)
	}
	outcome := release.releaseFormed(ctx, command.SubmissionVersion, control)
	switch {
	case len(outcome.refusals) > 0:
		return &HandleControlReleaseRefusalResult{outcome: ControlReleaseRefusedAgain, reason: outcome.refusals[0].reason}, nil
	case outcome.pending.open():
		return &HandleControlReleaseRefusalResult{outcome: ControlReleaseNotSettled}, nil
	default:
		return nil, nil
	}
}
