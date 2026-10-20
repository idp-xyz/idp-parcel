package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 预报有效时窗的收口编排（票 ps-external-mark-relations/14 裁定；PAR-NET-18）。收口是一次显式的登记写，与时窗登记同一能力面：在线
// 收口写面与受控批量口调的是同一个编排（ADR-0085 决定一），两个入口的重放判据与拒法不会各自漂移。事务由进程级入口给出。

// ErrPreAdviceWindowClosureRejected 说明这份收口落不下，重发同样的内容不会变好：形不成收口（收口时刻不晚于那一行的起点、缺依据一
// 类），或那一行收不了口（登记时带终点、已按别的内容收过口、本租户没有那一行）。裁定把这几格都归坏输入，在线口答坏载荷，批量口
// 退出码 1；错误文本说的是哪一格，给批量口的操作员看。
var ErrPreAdviceWindowClosureRejected = errors.New("node operations: pre-advice window closure rejected")

// ClosePreAdviceWindowCommand 携带一次收口。WindowFrom 指名要收口的那一行（同租户起点唯一）。没有登记时刻：那是册记下的，取时钟。
type ClosePreAdviceWindowCommand struct {
	TenantID   domain.TenantID
	WindowFrom time.Time
	ClosedAt   time.Time
	Basis      string
}

// PreAdviceWindowClosureOutcome 是收口的封闭答案，各格都是形成了的答案。
type PreAdviceWindowClosureOutcome uint8

const (
	PreAdviceWindowClosureOutcomeInvalid PreAdviceWindowClosureOutcome = iota
	PreAdviceWindowClosed
	// PreAdviceWindowClosureExisting：同一份收口原样再收（ADR-0085「重放答已有结果」），回显的是第一次落下的那一份。
	PreAdviceWindowClosureExisting
)

func (outcome PreAdviceWindowClosureOutcome) String() string {
	switch outcome {
	case PreAdviceWindowClosed:
		return "PRE_ADVICE_WINDOW_CLOSED"
	case PreAdviceWindowClosureExisting:
		return "EXISTING_PRE_ADVICE_WINDOW_CLOSURE"
	default:
		return ""
	}
}

// PreAdviceWindowClosureResult 是收口的答案与收口之后的那一行。
type PreAdviceWindowClosureResult struct {
	outcome PreAdviceWindowClosureOutcome
	window  domain.PreAdviceValidityWindow
}

// NewPreAdviceWindowClosureResult 给传输层的替身造答案用；编排自己也经它出答案。
func NewPreAdviceWindowClosureResult(outcome PreAdviceWindowClosureOutcome, window domain.PreAdviceValidityWindow) PreAdviceWindowClosureResult {
	return PreAdviceWindowClosureResult{outcome: outcome, window: window}
}

func (result PreAdviceWindowClosureResult) Outcome() PreAdviceWindowClosureOutcome {
	return result.outcome
}

func (result PreAdviceWindowClosureResult) Window() domain.PreAdviceValidityWindow {
	return result.window
}

type ClosePreAdviceWindowDeps struct {
	Windows ports.PreAdviceWindowRegister
	Clock   ports.Clock
}

type ClosePreAdviceWindowHandler struct {
	deps ClosePreAdviceWindowDeps
}

// NewClosePreAdviceWindowHandler 构造期拒缺件，理由同 NewRegisterPreAdviceWindowHandler。
func NewClosePreAdviceWindowHandler(deps ClosePreAdviceWindowDeps) (*ClosePreAdviceWindowHandler, error) {
	if deps.Windows == nil || deps.Clock == nil {
		return nil, errors.New("node operations: pre-advice window closure needs a register and a clock")
	}
	return &ClosePreAdviceWindowHandler{deps: deps}, nil
}

// Handle 形成一份收口并落册。没落时按回显的那一行分格：那一行带着内容全同的收口是重放，答已有结果；带着别的收口、或登记时就带
// 终点、或本租户没有那一行，都答被拒——册上已有的终点不被顶替，已据它作出的范围判断仍有依据。册读写不动原样交回，不形成答案。
func (handler *ClosePreAdviceWindowHandler) Handle(
	ctx context.Context,
	command ClosePreAdviceWindowCommand,
) (PreAdviceWindowClosureResult, error) {
	closure, err := domain.FormPreAdviceWindowClosure(domain.PreAdviceWindowClosureSpec{
		TenantID:     command.TenantID,
		WindowFrom:   command.WindowFrom,
		ClosedAt:     command.ClosedAt,
		Basis:        command.Basis,
		RegisteredAt: handler.deps.Clock.Now(),
	})
	if err != nil {
		return PreAdviceWindowClosureResult{}, fmt.Errorf("%w: %v", ErrPreAdviceWindowClosureRejected, err)
	}
	recorded, outcome, err := handler.deps.Windows.ClosePreAdviceWindow(ctx, closure)
	if err != nil {
		return PreAdviceWindowClosureResult{}, fmt.Errorf("close pre-advice window: %w", err)
	}
	switch outcome {
	case ports.DeliveryScopeAppended:
		return NewPreAdviceWindowClosureResult(PreAdviceWindowClosed, recorded), nil
	case ports.DeliveryScopeAlreadyRecorded:
		existing, closed := recorded.Closure()
		if closed && existing.SameContent(closure) {
			return NewPreAdviceWindowClosureResult(PreAdviceWindowClosureExisting, recorded), nil
		}
		if closed {
			return PreAdviceWindowClosureResult{}, fmt.Errorf("%w: the window from %s was already closed at %s",
				ErrPreAdviceWindowClosureRejected, recorded.EffectiveFrom().Format(time.RFC3339), existing.ClosedAt().Format(time.RFC3339))
		}
		until, _ := recorded.EffectiveUntil()
		return PreAdviceWindowClosureResult{}, fmt.Errorf("%w: the window from %s was registered with an end at %s",
			ErrPreAdviceWindowClosureRejected, recorded.EffectiveFrom().Format(time.RFC3339), until.Format(time.RFC3339))
	case ports.DeliveryScopeTargetNotFound:
		return PreAdviceWindowClosureResult{}, fmt.Errorf("%w: no window of this tenant starts at %s",
			ErrPreAdviceWindowClosureRejected, closure.WindowFrom().Format(time.RFC3339))
	default:
		// 时窗册的收口写口只该答上面三格；答别的格是实现坏了，不是业务答案。
		return PreAdviceWindowClosureResult{}, fmt.Errorf("close pre-advice window: unexpected register outcome %q", outcome)
	}
}
