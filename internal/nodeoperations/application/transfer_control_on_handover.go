package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// ControlOnHandoverOutcome 是「一版权威运输交接到了 → 推进交出节点那一侧的控制」的结果代数（ADR-0214，UC-NO-005 转出）。
//
// 落了行的两格之外，其余各格都不写：拒收与待确认不转出控制；后继版仍`已交接`时控制早已随前版转出；被后继版接续了的那一版晚到时，
// 它已不是这个对象的交接结果。几格分开，因为人去查的地方不同。
type ControlOnHandoverOutcome uint8

const (
	ControlOnHandoverOutcomeInvalid ControlOnHandoverOutcome = iota
	// ControlTransferredOutOnHandover：转出了此刻唯一在身的那一份控制，转移册落一行。
	ControlTransferredOutOnHandover
	// ControlNotAdvancedOnHandover：入账不推进，原因在落下的那一行上（domain.ControlNotAdvancedReason）。
	ControlNotAdvancedOnHandover
	// ControlTransitionAlreadyRecorded：这一版在这个方向上已有一行，交回那一行。
	ControlTransitionAlreadyRecorded
	// HandoverDoesNotTransferControl：拒收或待确认，同链上也没转出过，零写入。
	HandoverDoesNotTransferControl
	// ControlAlreadyTransferredInChain：同链前版已经转出过，这一版仍`已交接`，零写入（ADR-0214 决定五）。
	ControlAlreadyTransferredInChain
	// HandoverVersionSuperseded：这一版已被同链后一版更正，零写入；后一版自己的信封来决定。
	HandoverVersionSuperseded
	// ControlOnHandoverUndecided：交接登记册此刻读不动或还读不回这一版。
	ControlOnHandoverUndecided
)

func (outcome ControlOnHandoverOutcome) String() string {
	switch outcome {
	case ControlTransferredOutOnHandover:
		return "CONTROL_TRANSFERRED_OUT"
	case ControlNotAdvancedOnHandover:
		return "CONTROL_NOT_ADVANCED"
	case ControlTransitionAlreadyRecorded:
		return "TRANSITION_ALREADY_RECORDED"
	case HandoverDoesNotTransferControl:
		return "HANDOVER_DOES_NOT_TRANSFER"
	case ControlAlreadyTransferredInChain:
		return "ALREADY_TRANSFERRED_IN_CHAIN"
	case HandoverVersionSuperseded:
		return "HANDOVER_VERSION_SUPERSEDED"
	case ControlOnHandoverUndecided:
		return "CONTROL_ON_HANDOVER_UNDECIDED"
	default:
		return ""
	}
}

// ControlOnHandoverUndecidedReason 指名停在交接登记册哪一口。读不回单列一格：交接与信封在登记那一笔同时落库，读不回只剩可见性
// 滞后，与登记册读不动的续办不同。
type ControlOnHandoverUndecidedReason uint8

const (
	ControlOnHandoverUndecidedReasonNone ControlOnHandoverUndecidedReason = iota
	ControlOnHandoverHandoverUnreadable
	ControlOnHandoverHandoverNotVisible
)

func (reason ControlOnHandoverUndecidedReason) String() string {
	switch reason {
	case ControlOnHandoverHandoverUnreadable:
		return "HANDOVER_REGISTRY_UNREADABLE"
	case ControlOnHandoverHandoverNotVisible:
		return "HANDOVER_NOT_VISIBLE"
	default:
		return ""
	}
}

// ControlOnHandoverDisposition 是这一拍的答复对那封交接信封意味着什么，判据同 ADR-0216 决定三：原样重投会不会改变结果。
type ControlOnHandoverDisposition uint8

const (
	ControlOnHandoverDispositionInvalid ControlOnHandoverDisposition = iota
	// ControlOnHandoverSettled：入账不重投。入账不推进的各格也在这里：对不上实物、没有在身控制，重投答的还是同一件事。
	ControlOnHandoverSettled
	// ControlOnHandoverRetry：等交接登记册，重投可能就过。
	ControlOnHandoverRetry
	// ControlOnHandoverDefect：结果落在封闭集合以外，不入账，让它响。
	ControlOnHandoverDefect
)

func (disposition ControlOnHandoverDisposition) String() string {
	switch disposition {
	case ControlOnHandoverSettled:
		return "SETTLED"
	case ControlOnHandoverRetry:
		return "RETRY"
	case ControlOnHandoverDefect:
		return "DEFECT"
	default:
		return ""
	}
}

// TransferControlOnHandoverCommand 是交接信封上的判断键。版本是键的一维：更正是新版本新登记，各有自己的信封。
type TransferControlOnHandoverCommand struct {
	TenantID domain.TenantID
	Handover domain.AuthoritativeHandoverReference
}

type TransferControlOnHandoverResult struct {
	outcome    ControlOnHandoverOutcome
	reason     ControlOnHandoverUndecidedReason
	transition ports.ControlTransitionRecord
}

func (result TransferControlOnHandoverResult) Outcome() ControlOnHandoverOutcome {
	return result.outcome
}

func (result TransferControlOnHandoverResult) UndecidedReason() ControlOnHandoverUndecidedReason {
	return result.reason
}

// Transition 只在落了行或已在册时给出那一行。
func (result TransferControlOnHandoverResult) Transition() (ports.ControlTransitionRecord, bool) {
	switch result.outcome {
	case ControlTransferredOutOnHandover, ControlNotAdvancedOnHandover, ControlTransitionAlreadyRecorded:
		return result.transition, true
	default:
		return ports.ControlTransitionRecord{}, false
	}
}

// Disposition 把结果折成对信封的处置。各格逐一列出、不留默认入账：日后多一格时落进 Defect 响起来，而不是悄悄入账。
func (result TransferControlOnHandoverResult) Disposition() ControlOnHandoverDisposition {
	switch result.outcome {
	case ControlTransferredOutOnHandover, ControlNotAdvancedOnHandover, ControlTransitionAlreadyRecorded,
		HandoverDoesNotTransferControl, ControlAlreadyTransferredInChain, HandoverVersionSuperseded:
		return ControlOnHandoverSettled
	case ControlOnHandoverUndecided:
		return ControlOnHandoverRetry
	default:
		return ControlOnHandoverDefect
	}
}

// ErrUnexpectedControlTransitionSave 说明转移册答出封闭集合以外的落册结果，或撞了主键却读不回那一行。
var ErrUnexpectedControlTransitionSave = errors.New("node operations: unexpected control transition save outcome")

// TransferControlOnHandoverDeps 全部必备。Clock 只给转移册的记录时刻；转出时刻取交接的业务时间。
type TransferControlOnHandoverDeps struct {
	Handovers ports.AuthoritativeHandoverSource
	Objects   ports.HandoverObjectResolver
	Controls  ports.PhysicalControlRegister
	Clock     ports.Clock
}

// TransferControlOnHandoverHandler 是 UC-NO-005 的转出一半（ADR-0214 决定二至五）：按信封的键读回那一版权威运输交接，交接对象按
// 正式包裹身份经本上下文的当前关联对上作业实物，`已交接`即转出它此刻唯一在身的控制。要在消费门那一笔事务里调：实物锁、转移行与
// inbox 的已处理同生共死。
type TransferControlOnHandoverHandler struct {
	deps TransferControlOnHandoverDeps
}

func NewTransferControlOnHandoverHandler(deps TransferControlOnHandoverDeps) (*TransferControlOnHandoverHandler, error) {
	if deps.Handovers == nil || deps.Objects == nil || deps.Controls == nil || deps.Clock == nil {
		return nil, errors.New("node operations: transfer control on handover needs handovers, objects, controls and a clock")
	}
	return &TransferControlOnHandoverHandler{deps: deps}, nil
}

// Transfer 跑一版交接。各步的次序有讲究：
//
//  1. 读回那一版；读不动或读不回答未决。被同链后一版更正了就停：晚到的前版说的已不是这个对象此刻的结果。
//  2. 这一版在转出方向上已有一行就交回它：重投与人工补拍都答同一行。
//  3. 不是`已交接`：同链前版转出过就入账「控制来源链待重算」，否则零写入。不取实物锁——这一格不改变任何控制。
//  4. `已交接`：同链前版转出过就零写入；否则认交接对象，对不上一件作业实物就入账。
//  5. 对上一件：取实物锁、读它此刻在身的各份控制，锁下再认一次更正链（前版可能刚在锁外转出），然后按领域判转出或入账。
func (handler *TransferControlOnHandoverHandler) Transfer(
	ctx context.Context,
	command TransferControlOnHandoverCommand,
) (TransferControlOnHandoverResult, error) {
	tenant, reference := command.TenantID, command.Handover
	handover, found, err := handler.deps.Handovers.FindAuthoritativeHandover(ctx, tenant, reference)
	if errors.Is(err, ports.ErrAuthoritativeHandoverUnreadable) {
		return undecidedControlOnHandover(ControlOnHandoverHandoverUnreadable), nil
	}
	if err != nil {
		return TransferControlOnHandoverResult{}, err
	}
	if !found {
		return undecidedControlOnHandover(ControlOnHandoverHandoverNotVisible), nil
	}
	if handover.Reference != reference {
		return TransferControlOnHandoverResult{}, fmt.Errorf(
			"node operations: handover source answered version %q for %q", handover.Reference.Version(), reference.Version())
	}
	if handover.Superseded {
		return TransferControlOnHandoverResult{outcome: HandoverVersionSuperseded}, nil
	}

	existing, recorded, err := handler.deps.Controls.FindTransition(ctx, tenant, reference, domain.ControlTransitionOut)
	if err != nil {
		return TransferControlOnHandoverResult{}, fmt.Errorf("find the control transition of this handover: %w", err)
	}
	if recorded {
		return TransferControlOnHandoverResult{outcome: ControlTransitionAlreadyRecorded, transition: existing}, nil
	}

	earlier, chained, err := handler.deps.Controls.FindAdvancedInHandoverChain(ctx, tenant, reference, domain.ControlTransitionOut)
	if err != nil {
		return TransferControlOnHandoverResult{}, fmt.Errorf("find an earlier transfer in the handover chain: %w", err)
	}
	if !handover.HandedOver {
		if !chained {
			return TransferControlOnHandoverResult{outcome: HandoverDoesNotTransferControl}, nil
		}
		pending, err := domain.PendingControlSourceRecompute(earlier.Transition, reference, handover.JudgedAt)
		if err != nil {
			return TransferControlOnHandoverResult{}, fmt.Errorf("record the control source chain pending recompute: %w", err)
		}
		return handler.record(ctx, pending)
	}
	if chained {
		return TransferControlOnHandoverResult{outcome: ControlAlreadyTransferredInChain}, nil
	}

	unit, unresolved, err := handler.resolve(ctx, tenant, reference)
	if err != nil {
		return TransferControlOnHandoverResult{}, err
	}
	if unresolved != domain.ControlNotAdvancedReasonAbsent {
		transition, err := domain.ObjectNotResolved(tenant, reference, domain.ControlTransitionOut, unresolved, handover.JudgedAt)
		if err != nil {
			return TransferControlOnHandoverResult{}, fmt.Errorf("record the unresolved handover object: %w", err)
		}
		return handler.record(ctx, transition)
	}

	active, err := handler.deps.Controls.FindActiveControls(ctx, tenant, unit)
	if err != nil {
		return TransferControlOnHandoverResult{}, fmt.Errorf("find the active controls of the handed over unit: %w", err)
	}
	if _, chained, err := handler.deps.Controls.FindAdvancedInHandoverChain(
		ctx, tenant, reference, domain.ControlTransitionOut); err != nil {
		return TransferControlOnHandoverResult{}, fmt.Errorf("find an earlier transfer in the handover chain under the unit lock: %w", err)
	} else if chained {
		return TransferControlOnHandoverResult{outcome: ControlAlreadyTransferredInChain}, nil
	}
	transition, err := domain.JudgeTransferOut(tenant, unit, active, reference, handover.TransferOut, handover.JudgedAt)
	if err != nil {
		return TransferControlOnHandoverResult{}, fmt.Errorf("judge the transfer out: %w", err)
	}
	return handler.record(ctx, transition)
}

// resolve 认交接对象（ADR-0214 决定二）：集运单元、按包裹对上零件或多件，都交回入账原因；恰一件交回那一件。
func (handler *TransferControlOnHandoverHandler) resolve(
	ctx context.Context,
	tenant domain.TenantID,
	reference domain.AuthoritativeHandoverReference,
) (domain.HandlingUnitID, domain.ControlNotAdvancedReason, error) {
	resolution, err := handler.deps.Objects.ResolveHandoverObject(ctx, tenant, reference)
	if err != nil {
		return domain.HandlingUnitID{}, domain.ControlNotAdvancedReasonAbsent, fmt.Errorf("resolve the handover object: %w", err)
	}
	switch resolution.Kind {
	case ports.HandoverObjectConsolidationUnit:
		return domain.HandlingUnitID{}, domain.ConsolidationUnitPendingRule, nil
	case ports.HandoverObjectParcel:
		switch len(resolution.Units) {
		case 0:
			return domain.HandlingUnitID{}, domain.NoAssociatedUnit, nil
		case 1:
			return resolution.Units[0], domain.ControlNotAdvancedReasonAbsent, nil
		default:
			return domain.HandlingUnitID{}, domain.SeveralAssociatedUnits, nil
		}
	default:
		return domain.HandlingUnitID{}, domain.ControlNotAdvancedReasonAbsent, fmt.Errorf(
			"node operations: handover object resolved to %q", resolution.Kind)
	}
}

// record 落一行并作答。撞主键（并发里同一版先落了）就读回赢家作答，与收寄编排的 commit 同一个口径。
func (handler *TransferControlOnHandoverHandler) record(
	ctx context.Context,
	transition domain.ControlTransition,
) (TransferControlOnHandoverResult, error) {
	record := ports.ControlTransitionRecord{Transition: transition, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Controls.Save(ctx, record)
	if err != nil {
		return TransferControlOnHandoverResult{}, fmt.Errorf("record the control transition: %w", err)
	}
	switch saved {
	case ports.ControlTransitionSaved:
		outcome := ControlNotAdvancedOnHandover
		if transition.Outcome() == domain.ControlAdvanced {
			outcome = ControlTransferredOutOnHandover
		}
		return TransferControlOnHandoverResult{outcome: outcome, transition: record}, nil
	case ports.ControlTransitionAlreadyRecorded:
		winner, found, err := handler.deps.Controls.FindTransition(ctx, transition.TenantID(), transition.Handover(), transition.Direction())
		if err != nil {
			return TransferControlOnHandoverResult{}, fmt.Errorf("read back the control transition winner: %w", err)
		}
		if !found {
			return TransferControlOnHandoverResult{}, fmt.Errorf("%w: already recorded but not readable", ErrUnexpectedControlTransitionSave)
		}
		return TransferControlOnHandoverResult{outcome: ControlTransitionAlreadyRecorded, transition: winner}, nil
	default:
		return TransferControlOnHandoverResult{}, fmt.Errorf("%w: %d", ErrUnexpectedControlTransitionSave, saved)
	}
}

func undecidedControlOnHandover(reason ControlOnHandoverUndecidedReason) TransferControlOnHandoverResult {
	return TransferControlOnHandoverResult{outcome: ControlOnHandoverUndecided, reason: reason}
}
