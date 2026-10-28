package domain

import (
	"errors"
	"fmt"
	"time"
)

// 本文件是待人工处理项（PS CONTEXT「待人工处理项」）：收寄后服务处置决定向对象所有者提出的范围化请求里，系统今天不替所有者执行的那几项，成为运营人员在
// 系统里逐项登记处理结果的一行。哪一项由系统执行、哪一项由人工，是随产品发布的内置策略（ExecutedBySystem），不是租户取值。登记即这一项的承接结果：
// 已处理或无法处理，附说明、操作者与处理时刻；它是 PS 保全的运营事实，不改写所有者的对象，也不形成包裹终局。

// ErrInvalidManualItemResult 说明一份处理结果立不起来：结果不在封闭集、说明或操作者缺席，或所指不是封闭表里的请求项。
var ErrInvalidManualItemResult = errors.New("parcel shipment: invalid post-intake manual item result")

// ParseDispositionOwner 按字面认所有者，词不在封闭集内即拒。
func ParseDispositionOwner(raw string) (DispositionOwner, error) {
	for _, owner := range []DispositionOwner{DispositionOwnerNetworkRouting, DispositionOwnerNodeOperations, DispositionOwnerTransportFulfillment} {
		if owner.String() == raw {
			return owner, nil
		}
	}
	return DispositionOwnerInvalid, fmt.Errorf("disposition owner %q is not an owner", raw)
}

// ParseDispositionOwnerAction 按字面认请求动作，词不在封闭集内即拒。
func ParseDispositionOwnerAction(raw string) (DispositionOwnerAction, error) {
	for _, action := range []DispositionOwnerAction{
		DispositionActionHoldAtControl, DispositionActionHoldOnwardMovement, DispositionActionReplanToAlternateDestination,
		DispositionActionStartAlternateJourney, DispositionActionPlanReturnRoute, DispositionActionLoadForReturn,
		DispositionActionStartReturnJourney, DispositionActionReleaseUnconsumedPlan,
	} {
		if action.String() == raw {
			return action, nil
		}
	}
	return DispositionOwnerActionInvalid, fmt.Errorf("disposition owner action %q is not an action", raw)
}

// ExecutedBySystem 问这一项请求是否由系统里的所有者消费者自动执行。今天只有运输履约的两个「另起旅程」动作（UC-TF-007）：其余各项所有者在系统里没有对应
// 模型，由运营人员在系统外处理后登记结果。这是产品策略：某个所有者日后有了执行器，把它的动作加进这里，下一份决定起它不再成为待人工处理项；已经形成的
// 待人工处理项不受影响。
func (action DispositionOwnerAction) ExecutedBySystem() bool {
	return action == DispositionActionStartAlternateJourney || action == DispositionActionStartReturnJourney
}

// ManualOwnerRequests 是这个去向的请求表里需要人工处理的那几项：封闭表里系统不执行的部分，次序同请求表。
func (direction PostIntakeDispositionDirection) ManualOwnerRequests() []DispositionOwnerRequest {
	var manual []DispositionOwnerRequest
	for _, request := range direction.OwnerRequests() {
		if !request.Action.ExecutedBySystem() {
			manual = append(manual, request)
		}
	}
	return manual
}

// ManualItemOutcome 是一项人工处理的结果，封闭两值。
type ManualItemOutcome uint8

const (
	ManualItemOutcomeInvalid ManualItemOutcome = iota
	// ManualItemHandled：运营人员已经把这一项办完。
	ManualItemHandled
	// ManualItemUnable：这一项办不了；原因写在说明里。无法处理也是承接结果，当前控制责任不因此消失。
	ManualItemUnable
)

func (outcome ManualItemOutcome) String() string {
	switch outcome {
	case ManualItemHandled:
		return "HANDLED"
	case ManualItemUnable:
		return "UNABLE"
	default:
		return ""
	}
}

// ParseManualItemOutcome 按字面认处理结果。
func ParseManualItemOutcome(raw string) (ManualItemOutcome, error) {
	for _, outcome := range []ManualItemOutcome{ManualItemHandled, ManualItemUnable} {
		if outcome.String() == raw {
			return outcome, nil
		}
	}
	return ManualItemOutcomeInvalid, fmt.Errorf("manual item outcome %q is not an outcome", raw)
}

// ManualItemNote 是处理说明：做了什么，或为什么办不了。本上下文只要求它在场。
type ManualItemNote struct{ requiredValue }

func NewManualItemNote(value string) (ManualItemNote, error) {
	required, err := newRequiredValue("manual item note", value)
	return ManualItemNote{required}, err
}

// ManualItemHandlerReference 是登记处理结果的操作者引用，取（发行方，sub）这一对。
type ManualItemHandlerReference struct{ requiredValue }

func NewManualItemHandlerReference(value string) (ManualItemHandlerReference, error) {
	required, err := newRequiredValue("manual item handler reference", value)
	return ManualItemHandlerReference{required}, err
}

// ManualItemKey 指名一项待人工处理项：哪一份决定链上的哪一版向哪个所有者提出的哪个动作。
type ManualItemKey struct {
	Tenant          TenantID
	Parcel          DeclaredParcelID
	DecisionVersion int
	Owner           DispositionOwner
	Action          DispositionOwnerAction
}

// ManualItemKeyError 核对键的搭配：决定版本从 1 起，所有者与动作必须是封闭表里的一项，且这一项是需要人工处理的。
func ManualItemKeyError(key ManualItemKey) error {
	if !key.Tenant.valid() || !key.Parcel.valid() || key.DecisionVersion < 1 {
		return fmt.Errorf("%w: tenant, parcel and a decision version from 1 are required", ErrInvalidManualItemResult)
	}
	if key.Owner.String() == "" || key.Action.String() == "" {
		return fmt.Errorf("%w: owner and action must be named", ErrInvalidManualItemResult)
	}
	if key.Action.ExecutedBySystem() {
		return fmt.Errorf("%w: %s is executed by the system and has no manual item", ErrInvalidManualItemResult, key.Action)
	}
	return nil
}

// ManualItemResult 是一项待人工处理项的处理结果：结果、说明、操作者与处理时刻。登记后不改不删；要改口，是运营在系统外另行处理并在说明里留痕，不是改写这一行。
type ManualItemResult struct {
	Outcome   ManualItemOutcome
	Note      ManualItemNote
	Handler   ManualItemHandlerReference
	HandledAt time.Time
}

// FormManualItemResult 形成一份处理结果，缺件即拒；时刻截到 UTC。
func FormManualItemResult(outcome ManualItemOutcome, note ManualItemNote, handler ManualItemHandlerReference, handledAt time.Time) (ManualItemResult, error) {
	if outcome.String() == "" || !note.valid() || !handler.valid() || handledAt.IsZero() {
		return ManualItemResult{}, fmt.Errorf("%w: outcome, note, handler and time are required", ErrInvalidManualItemResult)
	}
	return ManualItemResult{Outcome: outcome, Note: note, Handler: handler, HandledAt: handledAt.UTC()}, nil
}

// SameContentAs 问两份处理结果说的是不是同一件事：结果、说明与操作者都相同。同一个人带着同样内容重发要读回先前那一份；内容不同就是冲突。
func (result ManualItemResult) SameContentAs(other ManualItemResult) bool {
	return result.Outcome == other.Outcome && result.Note == other.Note && result.Handler == other.Handler
}
