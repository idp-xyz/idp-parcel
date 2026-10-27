package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// CorrectPickupAttemptResultCommand 携带一次揽收结果更正的全部输入：指名被更正的那一格（尝试提交的来源身份加对象）、更正
// 成的确定结果、所凭的后补来源与两个业务时刻。
//
// Control 与三格段声明只属更正成揽收到手：控制依据证明运输方取得了控制，失败与拒收带它就是把没有接收的到场伪装成取得
// 控制；段声明是进段的输入，与首登同形（对象级揽收登记命令的同名字段）。更正命令带段号，是因为待确认那一格从没带对象进
// 过段——这是对象的第一次揽收，不是对已有参与的重派生（ADR-0112 决定二「更正命令不带段号」说的是后一种）；缺席时不立段。
// DecidedBy 是运营决定面提交时的操作者（「发行方#sub」），只作操作证据。
type CorrectPickupAttemptResultCommand struct {
	TenantID    domain.TenantID
	SourceID    string
	Object      string
	Outcome     domain.AttemptObjectOutcome
	Basis       domain.AttemptResultBasisReference
	Source      domain.AttemptResultSourceReference
	Control     domain.TransportControlReference
	OccurredAt  time.Time
	CorrectedAt time.Time

	Segment              string
	PlannedSegment       string
	SegmentServiceAction string

	DecidedBy string
}

type CorrectPickupAttemptResultResult struct {
	outcome      AttemptResultCorrectionOutcome
	reason       AttemptResultCorrectionUndecidedReason
	record       ports.PickupResultCorrectionRecord
	hasRecord    bool
	attempt      domain.AttemptReference
	continuation string
	registration RegisterOffsitePickupResult
}

func (result CorrectPickupAttemptResultResult) Outcome() AttemptResultCorrectionOutcome {
	return result.outcome
}

func (result CorrectPickupAttemptResultResult) UndecidedReason() AttemptResultCorrectionUndecidedReason {
	return result.reason
}

// Record 交回在册的更正（成立或重放时在）。
func (result CorrectPickupAttemptResultResult) Record() (ports.PickupResultCorrectionRecord, bool) {
	return result.record, result.hasRecord
}

// Attempt 交回被更正的那一格所在的尝试身份，找到那一格时在。
func (result CorrectPickupAttemptResultResult) Attempt() domain.AttemptReference {
	return result.attempt
}

func (result CorrectPickupAttemptResultResult) ContinuationReference() string {
	return result.continuation
}

// Registration 交回更正成揽收到手时对象级揽收登记那一条路的答复（版本、交 parcel-shipment 的意图引用、进段欠账与拒绝），
// 只在新成立的更正是揽收到手时有意义。
func (result CorrectPickupAttemptResultResult) Registration() RegisterOffsitePickupResult {
	return result.registration
}

// PickupRegistrar 是更正成揽收到手时转交的对象级揽收登记。以接口而非具体处理器收下，只为测试能把「更正自己的规则」与
// 「登记的规则」分开测；生产装配交入的是同一条首登编排（带段册与判断册）。
type PickupRegistrar interface {
	Register(ctx context.Context, command RegisterOffsitePickupCommand) (RegisterOffsitePickupResult, error)
}

type CorrectPickupAttemptResultDeps struct {
	Corrections ports.PickupResultCorrections
	// Pickups 缺席时更正成揽收到手一律不受理——不会悄悄只改结果、不形成场外揽收。
	Pickups PickupRegistrar
	Clock   ports.Clock
}

type CorrectPickupAttemptResultHandler struct {
	deps CorrectPickupAttemptResultDeps
}

func NewCorrectPickupAttemptResultHandler(deps CorrectPickupAttemptResultDeps) *CorrectPickupAttemptResultHandler {
	return &CorrectPickupAttemptResultHandler{deps: deps}
}

// Correct 把一格待确认的揽收结果更正成确定结果：受理 → 锁住并读回那一格 → 已有更正按内容作答（同内容重放、异内容冲突）
// → 领域更正（只回指待确认、过确定结果的构造门）→ 揽收到手时经对象级揽收登记形成场外揽收 → 追加更正。
//
// **必须在调用方的事务里跑**：加锁、登记与追加同一笔。登记写在更正之前，更正追加不出就要回滚登记——所以装配的事务壳对
// 「成立」以外的任何一格都整笔回滚，不留下半件事实（这个处理器本身只在领域正当拒绝时不写任何东西）。
func (handler *CorrectPickupAttemptResultHandler) Correct(
	ctx context.Context,
	command CorrectPickupAttemptResultCommand,
) (CorrectPickupAttemptResultResult, error) {
	notAccepted := CorrectPickupAttemptResultResult{outcome: AttemptResultCorrectionNotAccepted}
	object, decidedBy, accepted := handler.accept(command)
	if !accepted {
		return notAccepted, nil
	}

	key := ports.PickupAttemptKey{TenantID: command.TenantID, SourceID: command.SourceID}
	undecided := func(reason AttemptResultCorrectionUndecidedReason) CorrectPickupAttemptResultResult {
		return CorrectPickupAttemptResultResult{
			outcome: AttemptResultCorrectionUndecided, reason: reason,
			continuation: attemptResultCorrectionContinuation("PICKUP_ATTEMPT_RESULT_CORRECTION",
				command.TenantID.String(), command.SourceID, object.String()),
		}
	}
	target, found, err := handler.deps.Corrections.LoadForCorrection(ctx, key, object)
	if err != nil {
		return undecided(AttemptResultCorrectionStoreUnavailable), nil
	}
	if !found {
		// 指错了尝试或对象：提交自身的矛盾，改请求。
		return notAccepted, nil
	}
	attemptRef := target.Attempt.Attempt()

	if target.HasCorrection {
		return handler.answerExisting(target, command, attemptRef), nil
	}
	correction, err := domain.CorrectPendingAttemptObjectResult(
		target.Attempt, target.Result, command.Outcome, command.Basis, command.Source, command.OccurredAt, command.CorrectedAt)
	switch {
	case err == nil:
	case isNotPending(err):
		return CorrectPickupAttemptResultResult{outcome: AttemptResultNotPending, attempt: attemptRef}, nil
	default:
		return CorrectPickupAttemptResultResult{outcome: AttemptResultCorrectionNotAccepted, attempt: attemptRef}, nil
	}

	var registration RegisterOffsitePickupResult
	if command.Outcome.Succeeded() {
		// 更正成揽收到手：控制依据在场外揽收上，场外揽收经对象级揽收登记形成——与首登同一条路（进段、交 parcel-shipment 采认），
		// 同（对象，尝试）已登记的同内容重放读回在册那一份，于是此前经对象级登记口绕过的做法也能收口，且不会有两份揽收。
		if handler.deps.Pickups == nil {
			return CorrectPickupAttemptResultResult{outcome: AttemptResultCorrectionNotAccepted, attempt: attemptRef}, nil
		}
		registration, err = handler.deps.Pickups.Register(ctx, RegisterOffsitePickupCommand{
			TenantID:             command.TenantID,
			Object:               object.String(),
			Task:                 target.Attempt.Task().String(),
			Attempt:              attemptRef.String(),
			Place:                target.Attempt.Place().String(),
			Control:              command.Control.String(),
			ExecutedBy:           target.Attempt.ExecutedBy().String(),
			OccurredAt:           command.OccurredAt,
			Segment:              command.Segment,
			PlannedSegment:       command.PlannedSegment,
			SegmentServiceAction: command.SegmentServiceAction,
		})
		if err != nil {
			return CorrectPickupAttemptResultResult{}, fmt.Errorf("register the corrected pickup: %w", err)
		}
		switch registration.Outcome() {
		case PickupRegistered, PickupExistingVersion:
		case PickupRegistrationConflict:
			// 同（对象，尝试）已登记的揽收说的是另一回事：更正不顶替它，要改那份揽收走揽收更正口。
			return CorrectPickupAttemptResultResult{outcome: AttemptResultCorrectionConflict, attempt: attemptRef}, nil
		case PickupRegistrationNotAccepted:
			return CorrectPickupAttemptResultResult{outcome: AttemptResultCorrectionNotAccepted, attempt: attemptRef}, nil
		case PickupRegistrationUndecided:
			return undecided(AttemptResultCorrectionPickupRegistrationUnsettled), nil
		default:
			return CorrectPickupAttemptResultResult{}, fmt.Errorf("unexpected pickup registration outcome %q", registration.Outcome())
		}
	}

	record := ports.PickupResultCorrectionRecord{
		Key:        key,
		Correction: correction,
		Control:    command.Control,
		DecidedBy:  decidedBy,
		RecordedAt: handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Corrections.Save(ctx, record)
	if err != nil {
		return undecided(AttemptResultCorrectionStoreUnavailable), nil
	}
	if saved != ports.PickupResultCorrectionSaved {
		// 这格结果已被锁住读过一遍，追加撞键只可能是锁没守住：响亮失败，让事务整笔回滚（登记随之撤回）。
		return CorrectPickupAttemptResultResult{}, fmt.Errorf("pickup result correction was recorded under the lock by another writer: %d", saved)
	}
	return CorrectPickupAttemptResultResult{
		outcome: AttemptResultCorrected, record: record, hasRecord: true, attempt: attemptRef, registration: registration,
	}, nil
}

// accept 过更正命令自己的形状：租户与来源身份、对象、更正成的结果是个确定结果、后补来源必备、控制依据与段声明只属揽收到手、
// 操作证据给了就得是个成形的引用。一个都不查库——立不起来的是提交自身的矛盾，改请求。
func (handler *CorrectPickupAttemptResultHandler) accept(
	command CorrectPickupAttemptResultCommand,
) (domain.CarriedObjectReference, domain.RegistrantReference, bool) {
	none := domain.CarriedObjectReference{}
	if strings.TrimSpace(command.TenantID.String()) == "" || strings.TrimSpace(command.SourceID) == "" {
		return none, domain.RegistrantReference{}, false
	}
	object, err := domain.NewCarriedObjectReference(command.Object)
	if err != nil {
		return none, domain.RegistrantReference{}, false
	}
	if command.Outcome == domain.AttemptObjectOutcomeInvalid || command.Outcome.PendingConfirmation() ||
		command.Source.String() == "" {
		return none, domain.RegistrantReference{}, false
	}
	picked := command.Outcome.Succeeded()
	declaresSegment := command.Segment != "" || command.PlannedSegment != "" || command.SegmentServiceAction != ""
	if picked != (command.Control.String() != "") || (!picked && declaresSegment) {
		return none, domain.RegistrantReference{}, false
	}
	var decidedBy domain.RegistrantReference
	if command.DecidedBy != "" {
		if decidedBy, err = domain.NewRegistrantReference(command.DecidedBy); err != nil {
			return none, domain.RegistrantReference{}, false
		}
	}
	return object, decidedBy, true
}

// answerExisting 按已落下的更正作答：说的同一件事是重放，说的另一件事是冲突，在册的不被顶替。
func (handler *CorrectPickupAttemptResultHandler) answerExisting(
	target ports.PickupResultCorrectionTarget,
	command CorrectPickupAttemptResultCommand,
	attempt domain.AttemptReference,
) CorrectPickupAttemptResultResult {
	stored := target.Correction
	effective := stored.Correction.Effective()
	storedBasis, submittedBasis := "", command.Basis.String()
	if value, present := effective.Basis(); present {
		storedBasis = value.String()
	}
	same := correctionContent(effective.Outcome().String(), storedBasis, stored.Correction.Source().String(),
		stored.Control.String(), effective.OccurredAt(), stored.Correction.CorrectedAt()) ==
		correctionContent(command.Outcome.String(), submittedBasis, command.Source.String(),
			command.Control.String(), command.OccurredAt, command.CorrectedAt)
	if !same {
		return CorrectPickupAttemptResultResult{outcome: AttemptResultCorrectionConflict, attempt: attempt}
	}
	return CorrectPickupAttemptResultResult{
		outcome: AttemptResultCorrectionExisting, record: stored, hasRecord: true, attempt: attempt,
	}
}
