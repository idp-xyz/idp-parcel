package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 预报有效时窗的登记编排（ADR-0212 决定二末项；PAR-NET-18）。在线登记写面与受控批量口调的是同一个编排（ADR-0085 决定一）：两个
// 入口的重放与重叠判据不会各自漂移。事务由进程级入口给出。

// ErrPreAdviceWindowRejected 说明命令形不成一行时窗：时长为负或不足整分、终点不晚于起点、缺依据一类。重发同样的内容不会变好，在线
// 口答坏载荷，批量口退出码 1。
var ErrPreAdviceWindowRejected = errors.New("node operations: pre-advice window registration rejected")

// RegisterPreAdviceWindowCommand 携带一次时窗登记。没有登记时刻：那是册记下的，取时钟，不由登记方说。EffectiveUntil 零值即没有终点。
type RegisterPreAdviceWindowCommand struct {
	TenantID       domain.TenantID
	Lead           time.Duration
	Grace          time.Duration
	EffectiveFrom  time.Time
	EffectiveUntil time.Time
	Basis          string
}

// PreAdviceWindowRegistrationOutcome 是登记的封闭答案，各格都是形成了的答案。
type PreAdviceWindowRegistrationOutcome uint8

const (
	PreAdviceWindowRegistrationOutcomeInvalid PreAdviceWindowRegistrationOutcome = iota
	PreAdviceWindowRegistered
	// PreAdviceWindowExisting：同一行原样再登（ADR-0085「重放答已有结果」），回显的是第一次落下的那一行。
	PreAdviceWindowExisting
	// PreAdviceWindowOverlapping：与在册行区间重叠，什么也没落，回显撞上的那一行。册只增，没有替换：要换值，登一段不重叠的。
	PreAdviceWindowOverlapping
)

func (outcome PreAdviceWindowRegistrationOutcome) String() string {
	switch outcome {
	case PreAdviceWindowRegistered:
		return "PRE_ADVICE_WINDOW_REGISTERED"
	case PreAdviceWindowExisting:
		return "EXISTING_PRE_ADVICE_WINDOW"
	case PreAdviceWindowOverlapping:
		return "PRE_ADVICE_WINDOW_OVERLAPS"
	default:
		return ""
	}
}

// PreAdviceWindowRegistrationResult 是登记的答案与它所指的那一行：新落的、已在册的，或撞上的。
type PreAdviceWindowRegistrationResult struct {
	outcome PreAdviceWindowRegistrationOutcome
	window  domain.PreAdviceValidityWindow
}

// NewPreAdviceWindowRegistrationResult 给传输层的替身造答案用；编排自己也经它出答案。
func NewPreAdviceWindowRegistrationResult(outcome PreAdviceWindowRegistrationOutcome, window domain.PreAdviceValidityWindow) PreAdviceWindowRegistrationResult {
	return PreAdviceWindowRegistrationResult{outcome: outcome, window: window}
}

func (result PreAdviceWindowRegistrationResult) Outcome() PreAdviceWindowRegistrationOutcome {
	return result.outcome
}

func (result PreAdviceWindowRegistrationResult) Window() domain.PreAdviceValidityWindow {
	return result.window
}

type RegisterPreAdviceWindowDeps struct {
	Windows ports.PreAdviceWindowRegister
	Clock   ports.Clock
}

type RegisterPreAdviceWindowHandler struct {
	deps RegisterPreAdviceWindowDeps
}

// NewRegisterPreAdviceWindowHandler 构造期拒缺件：漏装的册到第一次登记才暴露，会被当成一次依赖故障。
func NewRegisterPreAdviceWindowHandler(deps RegisterPreAdviceWindowDeps) (*RegisterPreAdviceWindowHandler, error) {
	if deps.Windows == nil || deps.Clock == nil {
		return nil, errors.New("node operations: pre-advice window registration needs a register and a clock")
	}
	return &RegisterPreAdviceWindowHandler{deps: deps}, nil
}

// Handle 形成一行时窗并登记。没落时按回显的那一行分格：内容全同是重放，否则是重叠——册上那一行不被顶替，已据它作出的范围判断仍有
// 依据。册读写不动原样交回，不形成答案。
func (handler *RegisterPreAdviceWindowHandler) Handle(
	ctx context.Context,
	command RegisterPreAdviceWindowCommand,
) (PreAdviceWindowRegistrationResult, error) {
	window, err := domain.FormPreAdviceValidityWindow(domain.PreAdviceValidityWindowSpec{
		TenantID:       command.TenantID,
		Lead:           command.Lead,
		Grace:          command.Grace,
		EffectiveFrom:  command.EffectiveFrom,
		EffectiveUntil: command.EffectiveUntil,
		Basis:          command.Basis,
		RegisteredAt:   handler.deps.Clock.Now(),
	})
	if err != nil {
		return PreAdviceWindowRegistrationResult{}, fmt.Errorf("%w: %v", ErrPreAdviceWindowRejected, err)
	}
	recorded, outcome, err := handler.deps.Windows.RegisterPreAdviceWindow(ctx, window)
	if err != nil {
		return PreAdviceWindowRegistrationResult{}, fmt.Errorf("register pre-advice window: %w", err)
	}
	switch outcome {
	case ports.DeliveryScopeAppended:
		return NewPreAdviceWindowRegistrationResult(PreAdviceWindowRegistered, recorded), nil
	case ports.DeliveryScopeAlreadyRecorded:
		if recorded.SameContent(window) {
			return NewPreAdviceWindowRegistrationResult(PreAdviceWindowExisting, recorded), nil
		}
		return NewPreAdviceWindowRegistrationResult(PreAdviceWindowOverlapping, recorded), nil
	default:
		// 时窗册只该答上面两格；答别的格是实现坏了，不是业务答案。
		return PreAdviceWindowRegistrationResult{}, fmt.Errorf("register pre-advice window: unexpected register outcome %q", outcome)
	}
}
