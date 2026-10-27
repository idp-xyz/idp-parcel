package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ErrCancellationOrderConflictDecisionNotAssembled 说明决定编排少接了一个必需件。少接一个件等多久都不会长出来，所以响亮报错，不当作未决，也不悄悄
// 跳过作废或续办信封。
var ErrCancellationOrderConflictDecisionNotAssembled = errors.New("parcel shipment: cancellation order conflict decision is not assembled")

// ErrCancellationOrderConflictDecisionKindUnknown 说明命令的去向不是封闭两值之一。
var ErrCancellationOrderConflictDecisionKindUnknown = errors.New("parcel shipment: cancellation order conflict decision kind is unknown")

// ErrCancellationOrderConflictNotVisible 是`统一不可见结果`：册上没有这份冲突、冲突属于另一个租户、被指名的几段对不上，同一个答案——可区分即可
// 枚举别人的冲突与取消（AT-PS-052 的统一不可见同理）。
var ErrCancellationOrderConflictNotVisible = errors.New("parcel shipment: cancellation order conflict is not visible")

// ErrCancellationBoundaryNotLocked 说明没取到包裹的取消边界锁。决定是人下的命令：取不到锁就整笔失败，让人稍后再试，不在锁外读写。
var ErrCancellationBoundaryNotLocked = errors.New("parcel shipment: the cancellation boundary lock was not taken")

// CancellationOrderConflictDecisionOutcome 是决定命令的封闭结果。
//
//   - `已决定`：这一次赢了，决定留痕落下；认定取消不成立时，依据它的不采用记录同笔作废，续办信封同笔交出。
//   - `已被决定`：这份冲突先被别人决定了（查时已决定，或并发里输了），读回先到的那一份；不作废、不留痕、不交第二封信封。
type CancellationOrderConflictDecisionOutcome uint8

const (
	CancellationOrderConflictDecisionOutcomeInvalid CancellationOrderConflictDecisionOutcome = iota
	CancellationOrderConflictDecided
	CancellationOrderConflictAlreadyDecidedOutcome
)

func (outcome CancellationOrderConflictDecisionOutcome) String() string {
	switch outcome {
	case CancellationOrderConflictDecided:
		return "DECIDED"
	case CancellationOrderConflictAlreadyDecidedOutcome:
		return "ALREADY_DECIDED"
	default:
		return ""
	}
}

// DecideCancellationOrderConflictCommand 指名册上的哪一份冲突、做哪一种决定，谁决定、凭什么。租户与决定人由接入面按认证结果给，不从请求体收。
type DecideCancellationOrderConflictCommand struct {
	Tenant       domain.TenantID
	Parcel       domain.DeclaredParcelID
	BoundaryKind domain.CancellationBoundaryKind
	BoundaryRef  domain.CancellationBoundaryReference
	Cancellation domain.ParcelCancellationID
	Kind         domain.CancellationOrderConflictDecisionKind
	Decider      domain.CancellationOrderConflictDeciderReference
	Basis        domain.CancellationOrderConflictDecisionBasis
}

type DecideCancellationOrderConflictResult struct {
	outcome  CancellationOrderConflictDecisionOutcome
	decision ports.CancellationOrderConflictDecision
	voided   int
}

func (result DecideCancellationOrderConflictResult) Outcome() CancellationOrderConflictDecisionOutcome {
	return result.outcome
}

// Decision 在`已决定`时是本次落下的留痕，在`已被决定`时是先到的那一份。
func (result DecideCancellationOrderConflictResult) Decision() ports.CancellationOrderConflictDecision {
	return result.decision
}

// VoidedRefusals 是本次作废的、依据这份取消的终局不采用行数；只有`已决定`且认定取消不成立时可能大于零。
func (result DecideCancellationOrderConflictResult) VoidedRefusals() int {
	return result.voided
}

type DecideCancellationOrderConflictDeps struct {
	Conflicts ports.CancellationOrderConflictRegister
	Decisions ports.CancellationOrderConflictDecisions
	// Voider 与 Handoff 只有「认定取消依据不成立」这一支用：交来源方更正不改取消，也没有被挡住的来源要重驱。
	Voider  ports.CancellationRefusedFinalVoider
	Handoff ports.CancellationNotUpheldHandoff
	// BoundaryLock 让决定与同一包裹上的取消、采用、终局判断排队：决定改的是「这件包裹有没有有效取消」，那几处正是在锁里读它。
	BoundaryLock ports.CancellationBoundaryLock
	Clock        ports.Clock
}

type DecideCancellationOrderConflictHandler struct {
	deps DecideCancellationOrderConflictDeps
}

func NewDecideCancellationOrderConflictHandler(deps DecideCancellationOrderConflictDeps) *DecideCancellationOrderConflictHandler {
	return &DecideCancellationOrderConflictHandler{deps: deps}
}

// Handle 对册上一份取消先后冲突形成一份明确的业务决定（ADR-0295 决定一至四）。
//
// 锁序：包裹取消边界锁（ports.CancellationBoundaryLock，P）→ 决定留痕 → 作废不采用行 → 续办信封。本编排不保存委托，不取接受链的作用域锁（S）与
// 委托行锁（R），一笔事务至多持一个 P 键，所以与取消、采用、终局判断各路之间、与持 S、R 的各路之间都不成环。调用方把整个 Handle 放进一笔事务：
// 留痕、作废与信封同生共死，返回错误即整笔回滚。
//
// 认定取消依据不成立：取消记录原样保留，从此不是有效取消（有效取消读排除它）；依据它的终局不采用行作废，被它挡住的来源由续办信封重驱。交来源方
// 更正：不改取消，也不改任何事实，只留痕——这一代边界事实在收寄采用与面单终局判断处不停在冲突上，新的一代按 ADR-0254 决定二重判。
func (handler *DecideCancellationOrderConflictHandler) Handle(
	ctx context.Context,
	command DecideCancellationOrderConflictCommand,
) (DecideCancellationOrderConflictResult, error) {
	if err := handler.assembled(command.Kind); err != nil {
		return DecideCancellationOrderConflictResult{}, err
	}
	if command.Kind.String() == "" {
		return DecideCancellationOrderConflictResult{}, ErrCancellationOrderConflictDecisionKindUnknown
	}
	if !lockCancellationBoundary(ctx, handler.deps.BoundaryLock, command.Tenant, command.Parcel) {
		return DecideCancellationOrderConflictResult{}, ErrCancellationBoundaryNotLocked
	}

	key := ports.CancellationOrderConflictKey{
		TenantID:     command.Tenant,
		Parcel:       command.Parcel,
		BoundaryKind: command.BoundaryKind,
		BoundaryRef:  command.BoundaryRef,
		Cancellation: command.Cancellation,
	}
	if err := handler.requireRegistered(ctx, key); err != nil {
		return DecideCancellationOrderConflictResult{}, err
	}
	earlier, decided, err := handler.deps.Decisions.FindConflictDecision(ctx, key)
	if err != nil {
		return DecideCancellationOrderConflictResult{}, fmt.Errorf("decide cancellation order conflict: %w", err)
	}
	if decided {
		return alreadyDecidedConflict(earlier), nil
	}

	// 决定时刻截到库的精度（微秒）：首次决定的答复带的是这里的值，之后读回的是库里存的值，两者要是同一个值。
	saved, winner, err := handler.deps.Decisions.DecideCancellationOrderConflict(ctx, ports.CancellationOrderConflictDecision{
		Key:       key,
		Kind:      command.Kind,
		Decider:   command.Decider,
		Basis:     command.Basis,
		DecidedAt: handler.deps.Clock.Now().UTC().Truncate(time.Microsecond),
	})
	if err != nil {
		return DecideCancellationOrderConflictResult{}, fmt.Errorf("decide cancellation order conflict: %w", err)
	}
	switch saved {
	case ports.CancellationOrderConflictDecisionRecorded:
	case ports.CancellationOrderConflictAlreadyDecided:
		return alreadyDecidedConflict(winner), nil
	case ports.CancellationOrderConflictNotRegistered:
		return DecideCancellationOrderConflictResult{}, fmt.Errorf("decide cancellation order conflict: %w", ErrCancellationOrderConflictNotVisible)
	default:
		return DecideCancellationOrderConflictResult{}, fmt.Errorf("decide cancellation order conflict: decision save outcome %d is not a known outcome", saved)
	}

	result := DecideCancellationOrderConflictResult{outcome: CancellationOrderConflictDecided, decision: winner}
	if command.Kind != domain.CancellationNotUpheld {
		return result, nil
	}
	voided, err := handler.deps.Voider.VoidCancellationRefusedFinals(ctx, command.Tenant, command.Parcel, command.Cancellation)
	if err != nil {
		return DecideCancellationOrderConflictResult{}, fmt.Errorf("decide cancellation order conflict: void refused finals: %w", err)
	}
	if err := handler.deps.Handoff.HandOffCancellationNotUpheld(ctx, ports.CancellationNotUpheldHandoffIntent{Decision: winner}); err != nil {
		return DecideCancellationOrderConflictResult{}, fmt.Errorf("decide cancellation order conflict: hand off resume: %w", err)
	}
	result.voided = voided
	return result, nil
}

func alreadyDecidedConflict(earlier ports.CancellationOrderConflictDecision) DecideCancellationOrderConflictResult {
	return DecideCancellationOrderConflictResult{outcome: CancellationOrderConflictAlreadyDecidedOutcome, decision: earlier}
}

func (handler *DecideCancellationOrderConflictHandler) assembled(kind domain.CancellationOrderConflictDecisionKind) error {
	deps := handler.deps
	if deps.Conflicts == nil || deps.Decisions == nil || deps.BoundaryLock == nil || deps.Clock == nil {
		return ErrCancellationOrderConflictDecisionNotAssembled
	}
	if kind == domain.CancellationNotUpheld && (deps.Voider == nil || deps.Handoff == nil) {
		return ErrCancellationOrderConflictDecisionNotAssembled
	}
	return nil
}

// requireRegistered 核对册上有这一份冲突：册读口按租户切，别的租户的行、不存在的行与几段对不上的行同样读不到，一律答「统一不可见」。
func (handler *DecideCancellationOrderConflictHandler) requireRegistered(
	ctx context.Context,
	key ports.CancellationOrderConflictKey,
) error {
	entries, err := handler.deps.Conflicts.ListByParcel(ctx, key.TenantID, key.Parcel)
	if err != nil {
		return fmt.Errorf("decide cancellation order conflict: %w", err)
	}
	for _, entry := range entries {
		if ports.KeyOfConflictEntry(entry) == key {
			return nil
		}
	}
	return fmt.Errorf("decide cancellation order conflict: %w", ErrCancellationOrderConflictNotVisible)
}
