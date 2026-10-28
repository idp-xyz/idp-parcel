package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ErrPostIntakeManualItemNotAssembled 说明登记编排少接了一个必需件，响亮报错。
var ErrPostIntakeManualItemNotAssembled = errors.New("parcel shipment: post-intake manual item registration is not assembled")

// ErrPostIntakeManualItemNotVisible 是`统一不可见结果`：命令指名的项查无、属于另一个租户、不属这件包裹或这一版决定，同一个答案。可区分即可枚举别人的包裹。
var ErrPostIntakeManualItemNotVisible = errors.New("parcel shipment: post-intake manual item is not visible")

// PostIntakeManualRegistrationOutcome 是登记命令的封闭结果。
//
//   - `已登记`：这一次赢了，结果落册。
//   - `已登记过`：同一项已有结果，且内容（结果、说明、操作者）与这条命令相同——重放，读回先前那一份。
//   - `结果冲突`：同一项已有结果，内容与这条命令不同；不覆盖，读回先前那一份。
type PostIntakeManualRegistrationOutcome uint8

const (
	PostIntakeManualOutcomeInvalid PostIntakeManualRegistrationOutcome = iota
	PostIntakeManualOutcomeRegistered
	PostIntakeManualOutcomeAlreadyRegistered
	PostIntakeManualOutcomeConflict
)

func (outcome PostIntakeManualRegistrationOutcome) String() string {
	switch outcome {
	case PostIntakeManualOutcomeRegistered:
		return "REGISTERED"
	case PostIntakeManualOutcomeAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case PostIntakeManualOutcomeConflict:
		return "RESULT_CONFLICT"
	default:
		return ""
	}
}

// RegisterPostIntakeManualItemResultCommand 指名哪一项、登记什么结果。租户与操作者由接入面按认证结果给，不从请求体收；处理时刻由编排取钟。
type RegisterPostIntakeManualItemResultCommand struct {
	Key     domain.ManualItemKey
	Outcome domain.ManualItemOutcome
	Note    domain.ManualItemNote
	Handler domain.ManualItemHandlerReference
}

type RegisterPostIntakeManualItemResultResult struct {
	outcome PostIntakeManualRegistrationOutcome
	item    ports.PostIntakeManualItem
}

func (result RegisterPostIntakeManualItemResultResult) Outcome() PostIntakeManualRegistrationOutcome {
	return result.outcome
}

// Item 是登记后（或先前登记的）这一项，连同它的结果。
func (result RegisterPostIntakeManualItemResultResult) Item() ports.PostIntakeManualItem {
	return result.item
}

type RegisterPostIntakeManualItemResultDeps struct {
	ManualItems ports.PostIntakeManualItems
	Clock       ports.Clock
}

type RegisterPostIntakeManualItemResultHandler struct {
	deps RegisterPostIntakeManualItemResultDeps
}

func NewRegisterPostIntakeManualItemResultHandler(deps RegisterPostIntakeManualItemResultDeps) *RegisterPostIntakeManualItemResultHandler {
	return &RegisterPostIntakeManualItemResultHandler{deps: deps}
}

// Handle 登记一项待人工处理项的处理结果（ADR-0340 决定二）。调用方把它放进一笔事务；返回错误即整笔回滚。
//
// 每项至多一份结果：重放答`已登记过`，同项异内容答`结果冲突`，都不覆盖。登记不要求这一项所属的决定还是链尾——包裹实物上的扣住或装载不因后来的决定而自行消失，
// 运营人员办完了就该有地方记；清单与读回面各自给出链尾版本，办之前看链尾说了什么是操作者的事。
func (handler *RegisterPostIntakeManualItemResultHandler) Handle(
	ctx context.Context,
	command RegisterPostIntakeManualItemResultCommand,
) (RegisterPostIntakeManualItemResultResult, error) {
	if handler.deps.ManualItems == nil || handler.deps.Clock == nil {
		return RegisterPostIntakeManualItemResultResult{}, ErrPostIntakeManualItemNotAssembled
	}
	if err := domain.ManualItemKeyError(command.Key); err != nil {
		return RegisterPostIntakeManualItemResultResult{}, err
	}
	existing, found, err := handler.deps.ManualItems.FindPostIntakeManualItem(ctx, command.Key)
	if err != nil {
		return RegisterPostIntakeManualItemResultResult{}, fmt.Errorf("register post-intake manual item result: %w", err)
	}
	if !found {
		return RegisterPostIntakeManualItemResultResult{}, fmt.Errorf("register post-intake manual item result: %w", ErrPostIntakeManualItemNotVisible)
	}
	if existing.HasResult {
		return answerRegistered(existing, command), nil
	}
	// 处理时刻截到库的精度（微秒）：首次登记的答复带的是这里的值，之后读回的是库里存的值，两者要是同一个值。
	result, err := domain.FormManualItemResult(command.Outcome, command.Note, command.Handler, handler.deps.Clock.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		return RegisterPostIntakeManualItemResultResult{}, fmt.Errorf("register post-intake manual item result: %w", err)
	}
	registered, item, present, err := handler.deps.ManualItems.RegisterPostIntakeManualItemResult(ctx, command.Key, result)
	if err != nil {
		return RegisterPostIntakeManualItemResultResult{}, fmt.Errorf("register post-intake manual item result: %w", err)
	}
	if !present {
		return RegisterPostIntakeManualItemResultResult{}, fmt.Errorf("register post-intake manual item result: %w", ErrPostIntakeManualItemNotVisible)
	}
	switch registered {
	case ports.PostIntakeManualRegistered:
		return RegisterPostIntakeManualItemResultResult{outcome: PostIntakeManualOutcomeRegistered, item: item}, nil
	case ports.PostIntakeManualAlreadyRegistered:
		return answerRegistered(item, command), nil
	default:
		return RegisterPostIntakeManualItemResultResult{}, fmt.Errorf("register post-intake manual item result: outcome %d is not a known outcome", registered)
	}
}

// answerRegistered 在这一项已有结果时作答：内容相同是重放，不同是冲突。
func answerRegistered(item ports.PostIntakeManualItem, command RegisterPostIntakeManualItemResultCommand) RegisterPostIntakeManualItemResultResult {
	if item.Result.SameContentAs(domain.ManualItemResult{Outcome: command.Outcome, Note: command.Note, Handler: command.Handler}) {
		return RegisterPostIntakeManualItemResultResult{outcome: PostIntakeManualOutcomeAlreadyRegistered, item: item}
	}
	return RegisterPostIntakeManualItemResultResult{outcome: PostIntakeManualOutcomeConflict, item: item}
}
