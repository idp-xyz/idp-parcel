// resume_stalled_evaluation.go 编排 ADR-0229 决定二、四的续办：一件等着的东西（一份停下的请求，或一拍停在发起之前的
// 计费时点 / 发生项），凭一个依据（一次参数登记或一次运营续办）续办一次。停下的请求另起下一代，问题原样、指名前一代；
// 发起停办按记下的输入原样重跑发起编排。旧请求与旧评价一个字都不改。本编排只办一件、只跑在调用方的事务里——逐件
// 各开一笔事务、按页取队列的是调用方。
package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var (
	// ErrStalledEvaluationVanished 说明队列交出的那一件，按键回读却不在册上。四本册都只增不删，回读不到只可能是两侧
	// 说法矛盾，不是可见性滞后。
	ErrStalledEvaluationVanished = errors.New("settlement accounting: the stalled evaluation the queue named is not in its register")
	// ErrUnexpectedResumedInitiationOutcome 说明重跑发起编排交回了封闭集合以外的结果。
	ErrUnexpectedResumedInitiationOutcome = errors.New("settlement accounting: unexpected outcome when rerunning an initiation stall")
)

// resumptionRequester 是续办出来的 BUY 请求上登的请求方。续办没有业务上的请求人：推动它的是一次登记或一次运营续办，
// 凭的是哪一次记在依据上；请求方是这条续办路自己，是产品里的一个固定名，不是租户取值。
const resumptionRequester = "settlement-accounting/evaluation-resumption"

// ResumeStalledEvaluationOutcome 是续办一件的答案。
type ResumeStalledEvaluationOutcome uint8

const (
	ResumeStalledEvaluationOutcomeInvalid ResumeStalledEvaluationOutcome = iota
	// StalledEvaluationResumed：发出了一份新请求——停下的请求的下一代，或发起停办重跑之后的首代。
	StalledEvaluationResumed
	// StalledEvaluationStillStalled：发起停办重跑之后又停在发起之前，册上换上了这一次的停因与依据。
	StalledEvaluationStillStalled
	// StalledEvaluationAlreadyResumed：早已续办过——停下的请求已有下一代，或这一拍已有请求。重放落在这里。
	StalledEvaluationAlreadyResumed
	// StalledEvaluationNotWaiting：它不在等这个依据——没有停因、停因不在这次续办的名单里、凭的正是这个依据续办过，或
	// 重跑之后这一户采用的是别的计费时点（那一拍本就不计费）。
	StalledEvaluationNotWaiting
	// StalledEvaluationNotAccepted：命令立不住，或记下的输入重跑时被发起编排拒收。
	StalledEvaluationNotAccepted
	// StalledEvaluationUndecided：某个依赖答不出，整笔回滚等重投。
	StalledEvaluationUndecided
)

func (outcome ResumeStalledEvaluationOutcome) String() string {
	switch outcome {
	case StalledEvaluationResumed:
		return "RESUMED"
	case StalledEvaluationStillStalled:
		return "STILL_STALLED"
	case StalledEvaluationAlreadyResumed:
		return "ALREADY_RESUMED"
	case StalledEvaluationNotWaiting:
		return "NOT_WAITING"
	case StalledEvaluationNotAccepted:
		return "NOT_ACCEPTED"
	case StalledEvaluationUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// ResumeStalledEvaluationCommand 是续办一件：哪一件（队列交出的键）、凭什么、这个依据解得开哪几种停因。Causes 由调用方
// 按依据的来源给——参数登记交 domain.EvaluationStopCausesResumedBy，运营续办交全集——本编排只核这一件的停因在不在里面。
type ResumeStalledEvaluationCommand struct {
	TenantID domain.TenantID
	Item     ports.StalledEvaluation
	Basis    domain.ResumeBasis
	Causes   []domain.EvaluationStopCause
}

// ResumeStalledEvaluationResult 带答案；`已续办`时带发出的那份请求的标识，`未决`时带停在哪一步与依赖原样答出的错误。
type ResumeStalledEvaluationResult struct {
	outcome     ResumeStalledEvaluationOutcome
	sellRequest domain.SellEvaluationRequestID
	buyRequest  domain.EvaluationRequestID
	stopCause   domain.EvaluationStopCause
	stage       string
	cause       error
}

func (result ResumeStalledEvaluationResult) Outcome() ResumeStalledEvaluationOutcome {
	return result.outcome
}

// SellRequest 交回发出或早已发出的 SELL 请求；不是 SELL 那一路或没有请求时为零值。
func (result ResumeStalledEvaluationResult) SellRequest() domain.SellEvaluationRequestID {
	return result.sellRequest
}

// BuyRequest 交回发出或早已发出的 BUY 请求；不是 BUY 那一路或没有请求时为零值。
func (result ResumeStalledEvaluationResult) BuyRequest() domain.EvaluationRequestID {
	return result.buyRequest
}

// StopCause 在`仍停着`时交回这一次的停因。
func (result ResumeStalledEvaluationResult) StopCause() (domain.EvaluationStopCause, bool) {
	return result.stopCause, result.stopCause != domain.EvaluationStopCauseInvalid
}

// UndecidedStage 与 UndecidedCause 只在`未决`时有值。
func (result ResumeStalledEvaluationResult) UndecidedStage() string { return result.stage }

func (result ResumeStalledEvaluationResult) UndecidedCause() error { return result.cause }

// ResumeStalledEvaluationDeps 是续办的依赖：两本发起停办册与两只发起编排（重跑发起停办），两本请求册各一套读、找下一代、
// 登记、铸造与交信封（起下一代），一只时钟。缺一件，四种等待里就有一种续办不了。
type ResumeStalledEvaluationDeps struct {
	SellStalls     ports.SellEvaluationInitiationStallRegister
	BuyStalls      ports.BuyEvaluationInitiationStallRegister
	SellMoments    *RequestSellEvaluationOnParcelMomentHandler
	BuyOccurrences *TriggerBuyEvaluationOnOccurrenceFormedHandler
	SellRequests   ports.SellEvaluationRequestView
	SellSuccessors ports.SellEvaluationRequestSuccessorView
	SellRegistry   ports.SellEvaluationRequestRegistry
	SellIdentity   ports.SellEvaluationRequestIdentityFactory
	SellHandoff    ports.SellEvaluationRequestHandoff
	BuyRequests    ports.EvaluationRequestView
	BuySuccessors  ports.EvaluationRequestSuccessorView
	BuyRegistry    ports.EvaluationRequestRegistry
	BuyIdentity    ports.EvaluationRequestIdentityFactory
	BuyHandoff     ports.EvaluationRequestHandoff
	Clock          ports.Clock
}

type ResumeStalledEvaluationHandler struct {
	deps ResumeStalledEvaluationDeps
}

func NewResumeStalledEvaluationHandler(deps ResumeStalledEvaluationDeps) (*ResumeStalledEvaluationHandler, error) {
	for _, dependency := range []struct {
		name    string
		missing bool
	}{
		{"sell evaluation initiation stall register", deps.SellStalls == nil},
		{"buy evaluation initiation stall register", deps.BuyStalls == nil},
		{"request sell evaluation on parcel moment handler", deps.SellMoments == nil},
		{"trigger buy evaluation on occurrence formed handler", deps.BuyOccurrences == nil},
		{"sell evaluation request view", deps.SellRequests == nil},
		{"sell evaluation request successor view", deps.SellSuccessors == nil},
		{"sell evaluation request registry", deps.SellRegistry == nil},
		{"sell evaluation request identity factory", deps.SellIdentity == nil},
		{"sell evaluation request handoff", deps.SellHandoff == nil},
		{"buy evaluation request view", deps.BuyRequests == nil},
		{"buy evaluation request successor view", deps.BuySuccessors == nil},
		{"buy evaluation request registry", deps.BuyRegistry == nil},
		{"buy evaluation request identity factory", deps.BuyIdentity == nil},
		{"buy evaluation request handoff", deps.BuyHandoff == nil},
		{"clock", deps.Clock == nil},
	} {
		if dependency.missing {
			return nil, fmt.Errorf("%w: %s", ErrNilDependency, dependency.name)
		}
	}
	return &ResumeStalledEvaluationHandler{deps: deps}, nil
}

// Handle 按这一件在哪本册上分派。各路都在自己的事务里先按键回读、再判它还在不在等这个依据：队列只是一份名单，取名单
// 与办这一件之间可能已有别的续办先到。
func (handler *ResumeStalledEvaluationHandler) Handle(
	ctx context.Context,
	command ResumeStalledEvaluationCommand,
) (ResumeStalledEvaluationResult, error) {
	if command.TenantID.String() == "" || !command.Basis.Present() || len(command.Causes) == 0 {
		return ResumeStalledEvaluationResult{outcome: StalledEvaluationNotAccepted}, nil
	}
	switch command.Item.Kind {
	case ports.StalledSellInitiation:
		return handler.rerunSellInitiation(ctx, command)
	case ports.StalledBuyInitiation:
		return handler.rerunBuyInitiation(ctx, command)
	case ports.StalledSellRequest:
		return handler.resumeSellRequest(ctx, command)
	case ports.StalledBuyRequest:
		return handler.resumeBuyRequest(ctx, command)
	default:
		return ResumeStalledEvaluationResult{outcome: StalledEvaluationNotAccepted}, nil
	}
}

// waitsFor 答一个停因与它上次续办凭的依据，在不在等这一次续办。凭同一个依据续办过的不再推：同一封信重投、同一条运营
// 命令重跑，都不该把续办出来又停下的那一件再推一代。
func waitsFor(command ResumeStalledEvaluationCommand, cause domain.EvaluationStopCause, lastBasis domain.ResumeBasis) bool {
	if lastBasis == command.Basis {
		return false
	}
	for _, allowed := range command.Causes {
		if allowed == cause {
			return true
		}
	}
	return false
}

func (handler *ResumeStalledEvaluationHandler) rerunSellInitiation(
	ctx context.Context,
	command ResumeStalledEvaluationCommand,
) (ResumeStalledEvaluationResult, error) {
	stall, found, err := handler.deps.SellStalls.FindSellInitiationStall(ctx, command.TenantID, command.Item.SellMoment)
	if err != nil {
		return undecidedResumption("SELL_INITIATION_STALL_REGISTER", err), nil
	}
	if !found {
		return ResumeStalledEvaluationResult{}, fmt.Errorf("%w: sell initiation %s/%s",
			ErrStalledEvaluationVanished, command.Item.SellMoment.Parcel, command.Item.SellMoment.Moment)
	}
	if !waitsFor(command, stall.Cause(), stall.Basis()) {
		return ResumeStalledEvaluationResult{outcome: StalledEvaluationNotWaiting}, nil
	}
	key := stall.NaturalKey()
	result, err := handler.deps.SellMoments.Handle(ctx, RequestSellEvaluationOnParcelMomentCommand{
		TenantID:       command.TenantID,
		Parcel:         key.Parcel,
		Moment:         key.Moment,
		Source:         stall.Source(),
		PricingBasisAt: stall.PricingBasisAt(),
		ResumeBasis:    command.Basis,
	})
	if err != nil {
		return ResumeStalledEvaluationResult{}, err
	}
	switch result.Outcome() {
	case SellEvaluationRequested, SellEvaluationRequestExisting:
		record, _ := result.Request()
		outcome := StalledEvaluationResumed
		if result.Outcome() == SellEvaluationRequestExisting {
			outcome = StalledEvaluationAlreadyResumed
		}
		return ResumeStalledEvaluationResult{outcome: outcome, sellRequest: record.Key.Request}, nil
	case SellEvaluationRequestUnconfigured, SellEvaluationRequestParcelUnresolved:
		if cause, stalled := result.StopCause(); stalled {
			return ResumeStalledEvaluationResult{outcome: StalledEvaluationStillStalled, stopCause: cause}, nil
		}
		return ResumeStalledEvaluationResult{outcome: StalledEvaluationNotWaiting}, nil
	case SellEvaluationRequestUndecided:
		return undecidedResumption(result.UndecidedReason().String(), nil), nil
	case SellEvaluationRequestNotAccepted:
		return ResumeStalledEvaluationResult{outcome: StalledEvaluationNotAccepted}, nil
	default:
		return ResumeStalledEvaluationResult{}, fmt.Errorf("%w: %s", ErrUnexpectedResumedInitiationOutcome, result.Outcome())
	}
}

func (handler *ResumeStalledEvaluationHandler) rerunBuyInitiation(
	ctx context.Context,
	command ResumeStalledEvaluationCommand,
) (ResumeStalledEvaluationResult, error) {
	stall, found, err := handler.deps.BuyStalls.FindBuyInitiationStall(
		ctx, command.TenantID, command.Item.Occurrence, command.Item.OccurrenceVersion)
	if err != nil {
		return undecidedResumption("BUY_INITIATION_STALL_REGISTER", err), nil
	}
	if !found {
		return ResumeStalledEvaluationResult{}, fmt.Errorf("%w: buy initiation %s@%s",
			ErrStalledEvaluationVanished, command.Item.Occurrence, command.Item.OccurrenceVersion)
	}
	if !waitsFor(command, stall.Cause(), stall.Basis()) {
		return ResumeStalledEvaluationResult{outcome: StalledEvaluationNotWaiting}, nil
	}
	result, err := handler.deps.BuyOccurrences.Trigger(ctx, TriggerBuyEvaluationOnOccurrenceFormedCommand{
		TenantID:    command.TenantID,
		Occurrence:  stall.Occurrence(),
		Agreement:   stall.Agreement(),
		ResumeBasis: command.Basis,
	})
	if err != nil {
		return ResumeStalledEvaluationResult{}, err
	}
	switch result.Outcome() {
	case BuyEvaluationTriggerFired:
		requested, _ := result.Request()
		record, _ := requested.Request()
		outcome := StalledEvaluationResumed
		if requested.Outcome() == EvaluationRequestExisting {
			outcome = StalledEvaluationAlreadyResumed
		}
		return ResumeStalledEvaluationResult{outcome: outcome, buyRequest: record.Key.Request}, nil
	case BuyEvaluationTriggerUnconfigured:
		return ResumeStalledEvaluationResult{
			outcome:   StalledEvaluationStillStalled,
			stopCause: buyInitiationStopCauseOf(result.UnconfiguredReason()),
		}, nil
	case BuyEvaluationTriggerUndecided:
		return undecidedResumption(result.UndecidedReason().String(), nil), nil
	case BuyEvaluationTriggerNotAccepted:
		return ResumeStalledEvaluationResult{outcome: StalledEvaluationNotAccepted}, nil
	default:
		return ResumeStalledEvaluationResult{}, fmt.Errorf("%w: %s", ErrUnexpectedResumedInitiationOutcome, result.Outcome())
	}
}

func buyInitiationStopCauseOf(reason BuyEvaluationTriggerUnconfiguredReason) domain.EvaluationStopCause {
	switch reason {
	case BuyEvaluationTriggerReasonNotRegistered:
		return domain.StopOccurrenceReasonNotRegistered
	case BuyEvaluationTriggerFeeItemNotRegistered:
		return domain.StopTriggerFeeItemNotRegistered
	default:
		return domain.EvaluationStopCauseInvalid
	}
}

func (handler *ResumeStalledEvaluationHandler) resumeSellRequest(
	ctx context.Context,
	command ResumeStalledEvaluationCommand,
) (ResumeStalledEvaluationResult, error) {
	key := ports.SellEvaluationRequestKey{TenantID: command.TenantID, Request: command.Item.SellRequest}
	record, found, err := handler.deps.SellRequests.FindByID(ctx, key)
	if err != nil {
		return undecidedResumption("SELL_EVALUATION_REQUEST_VIEW", err), nil
	}
	if !found {
		return ResumeStalledEvaluationResult{}, fmt.Errorf("%w: sell request %s", ErrStalledEvaluationVanished, key.Request)
	}
	if record.StopCause == domain.EvaluationStopCauseInvalid || !waitsFor(command, record.StopCause, record.Request.ResumeBasis()) {
		return ResumeStalledEvaluationResult{outcome: StalledEvaluationNotWaiting}, nil
	}
	if successor, has, err := handler.deps.SellSuccessors.FindSuccessor(ctx, key); err != nil {
		return undecidedResumption("SELL_EVALUATION_REQUEST_SUCCESSOR_VIEW", err), nil
	} else if has {
		return ResumeStalledEvaluationResult{outcome: StalledEvaluationAlreadyResumed, sellRequest: successor.Key.Request}, nil
	}

	id, err := handler.deps.SellIdentity.MintSellEvaluationRequestID(ctx)
	if err != nil {
		return undecidedResumption("SELL_EVALUATION_REQUEST_IDENTITY", err), nil
	}
	next, err := record.Request.Resume(id, command.Basis)
	if err != nil {
		return ResumeStalledEvaluationResult{}, fmt.Errorf("resume sell request %s: %w", key.Request, err)
	}
	nextRecord := ports.SellEvaluationRequestRecord{
		Key:        ports.SellEvaluationRequestKey{TenantID: command.TenantID, Request: id},
		Request:    next,
		RecordedAt: handler.deps.Clock.Now(),
		State:      domain.SellRequestSubmitted,
	}
	saved, err := handler.deps.SellRegistry.Save(ctx, nextRecord)
	if err != nil {
		return undecidedResumption("SELL_EVALUATION_REQUEST_REGISTRY", err), nil
	}
	switch saved {
	case ports.EvaluationRequestSaved:
	case ports.EvaluationRequestAlreadyRequested:
		successor, has, err := handler.deps.SellSuccessors.FindSuccessor(ctx, key)
		if err != nil || !has {
			return undecidedResumption("SELL_EVALUATION_REQUEST_SUCCESSOR_VIEW", err), nil
		}
		return ResumeStalledEvaluationResult{outcome: StalledEvaluationAlreadyResumed, sellRequest: successor.Key.Request}, nil
	default:
		return ResumeStalledEvaluationResult{}, fmt.Errorf("%w: %d", ErrUnexpectedSellEvaluationRequestSave, saved)
	}
	err = handler.deps.SellHandoff.HandOffSellEvaluationRequest(ctx, ports.SellEvaluationRequestIntent{Record: nextRecord})
	if errors.Is(err, ports.ErrSellEvaluationRequestHandoffRejected) {
		return ResumeStalledEvaluationResult{}, fmt.Errorf("resume sell request %s: %w", key.Request, err)
	}
	if err != nil {
		return undecidedResumption("SELL_EVALUATION_REQUEST_HANDOFF", err), nil
	}
	return ResumeStalledEvaluationResult{outcome: StalledEvaluationResumed, sellRequest: id}, nil
}

func (handler *ResumeStalledEvaluationHandler) resumeBuyRequest(
	ctx context.Context,
	command ResumeStalledEvaluationCommand,
) (ResumeStalledEvaluationResult, error) {
	key := ports.EvaluationRequestKey{TenantID: command.TenantID, Request: command.Item.BuyRequest}
	record, found, err := handler.deps.BuyRequests.FindByID(ctx, key)
	if err != nil {
		return undecidedResumption("BUY_EVALUATION_REQUEST_VIEW", err), nil
	}
	if !found {
		return ResumeStalledEvaluationResult{}, fmt.Errorf("%w: buy request %s", ErrStalledEvaluationVanished, key.Request)
	}
	if record.StopCause == domain.EvaluationStopCauseInvalid || !waitsFor(command, record.StopCause, record.Request.ResumeBasis()) {
		return ResumeStalledEvaluationResult{outcome: StalledEvaluationNotWaiting}, nil
	}
	if successor, has, err := handler.deps.BuySuccessors.FindSuccessor(ctx, key); err != nil {
		return undecidedResumption("BUY_EVALUATION_REQUEST_SUCCESSOR_VIEW", err), nil
	} else if has {
		return ResumeStalledEvaluationResult{outcome: StalledEvaluationAlreadyResumed, buyRequest: successor.Key.Request}, nil
	}

	id, err := handler.deps.BuyIdentity.MintEvaluationRequestID(ctx)
	if err != nil {
		return undecidedResumption("BUY_EVALUATION_REQUEST_IDENTITY", err), nil
	}
	requester, err := domain.NewRequesterReference(resumptionRequester)
	if err != nil {
		return ResumeStalledEvaluationResult{}, fmt.Errorf("resume buy request %s: requester: %w", key.Request, err)
	}
	now := handler.deps.Clock.Now()
	next, err := record.Request.Resume(id, command.Basis, now, requester)
	if err != nil {
		return ResumeStalledEvaluationResult{}, fmt.Errorf("resume buy request %s: %w", key.Request, err)
	}
	nextRecord := ports.EvaluationRequestRecord{
		Key:        ports.EvaluationRequestKey{TenantID: command.TenantID, Request: id},
		Request:    next,
		RecordedAt: now,
	}
	saved, err := handler.deps.BuyRegistry.Save(ctx, nextRecord)
	if err != nil {
		return undecidedResumption("BUY_EVALUATION_REQUEST_REGISTRY", err), nil
	}
	switch saved {
	case ports.EvaluationRequestSaved:
	case ports.EvaluationRequestAlreadyRequested:
		successor, has, err := handler.deps.BuySuccessors.FindSuccessor(ctx, key)
		if err != nil || !has {
			return undecidedResumption("BUY_EVALUATION_REQUEST_SUCCESSOR_VIEW", err), nil
		}
		return ResumeStalledEvaluationResult{outcome: StalledEvaluationAlreadyResumed, buyRequest: successor.Key.Request}, nil
	default:
		return ResumeStalledEvaluationResult{}, fmt.Errorf("%w: %d", ErrUnexpectedEvaluationRequestSave, saved)
	}
	// 交信封失败在这里答未决、整笔回滚：续办跑在调用方的事务里，留续办引用照答已续办会让下一代提交而信封缺席，
	// 理由同发生项已形成那一路（TriggerBuyEvaluationOnOccurrenceFormedHandler.Trigger 头注）。
	if err := handler.deps.BuyHandoff.HandOffEvaluationRequest(ctx, ports.EvaluationRequestIntent{Record: nextRecord}); err != nil {
		return undecidedResumption("BUY_EVALUATION_REQUEST_HANDOFF", err), nil
	}
	return ResumeStalledEvaluationResult{outcome: StalledEvaluationResumed, buyRequest: id}, nil
}

func undecidedResumption(stage string, cause error) ResumeStalledEvaluationResult {
	return ResumeStalledEvaluationResult{outcome: StalledEvaluationUndecided, stage: stage, cause: cause}
}
