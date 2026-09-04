package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// ErrUnexpectedSourceConnectorBindingOutcome 说明绑定登记册交回了结果代数以外的写入结果。
var ErrUnexpectedSourceConnectorBindingOutcome = errors.New("parcel pricing: unexpected source connector binding outcome")

// RegisterSourceConnectorBindingOutcome 是绑定登记请求的应用处理结果。
type RegisterSourceConnectorBindingOutcome uint8

const (
	RegisterSourceConnectorBindingOutcomeInvalid RegisterSourceConnectorBindingOutcome = iota
	// SourceConnectorBindingRecorded：新绑定版本已入册。
	SourceConnectorBindingRecorded
	// SourceConnectorBindingAlreadyOnRegister：同版本同声明已在册，幂等重放。
	SourceConnectorBindingAlreadyOnRegister
	// SourceConnectorBindingRegistrationConflict：同版本装了不同声明；原行不顶替，改声明请登新版本。
	SourceConnectorBindingRegistrationConflict
	// SourceConnectorBindingNotAccepted：请求不合法（零值绑定），未到达登记册。
	SourceConnectorBindingNotAccepted
	// SourceConnectorBindingUndecided：依赖故障，登记与否未知，原因随错误交回。
	SourceConnectorBindingUndecided
)

func (outcome RegisterSourceConnectorBindingOutcome) String() string {
	switch outcome {
	case SourceConnectorBindingRecorded:
		return "RECORDED"
	case SourceConnectorBindingAlreadyOnRegister:
		return "ALREADY_REGISTERED"
	case SourceConnectorBindingRegistrationConflict:
		return "CONFLICT"
	case SourceConnectorBindingNotAccepted:
		return "NOT_ACCEPTED"
	case SourceConnectorBindingUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// RegisterSourceConnectorBindingCommand 携带一版绑定；声明本体全在领域对象内，编排不拆解它。
type RegisterSourceConnectorBindingCommand struct {
	Binding domain.SourceConnectorBinding
}

type RegisterSourceConnectorBindingDeps struct {
	Bindings ports.SourceConnectorBindingRegister
}

// RegisterSourceConnectorBindingHandler 编排绑定登记：声明齐不齐在领域构造门，冲突判定在登记册，
// 这里只做受理与答案翻译。绑定是实例半边（ADR-0099 决定六），本用例不带任何默认声明。
type RegisterSourceConnectorBindingHandler struct {
	deps RegisterSourceConnectorBindingDeps
}

func NewRegisterSourceConnectorBindingHandler(deps RegisterSourceConnectorBindingDeps) *RegisterSourceConnectorBindingHandler {
	return &RegisterSourceConnectorBindingHandler{deps: deps}
}

func (handler *RegisterSourceConnectorBindingHandler) Handle(
	ctx context.Context,
	command RegisterSourceConnectorBindingCommand,
) (RegisterSourceConnectorBindingOutcome, error) {
	// 零值绑定连租户都指不出来；构造器保证非零绑定整体立得住，受理门只需分辨「根本没构造过」。
	if command.Binding.Tenant().String() == "" {
		return SourceConnectorBindingNotAccepted, nil
	}
	saved, err := handler.deps.Bindings.Register(ctx, command.Binding)
	if err != nil {
		return SourceConnectorBindingUndecided, fmt.Errorf("register source connector binding: %w", err)
	}
	switch saved {
	case ports.SourceConnectorBindingRegistered:
		return SourceConnectorBindingRecorded, nil
	case ports.SourceConnectorBindingAlreadyRegistered:
		return SourceConnectorBindingAlreadyOnRegister, nil
	case ports.SourceConnectorBindingConflict:
		return SourceConnectorBindingRegistrationConflict, nil
	default:
		return RegisterSourceConnectorBindingOutcomeInvalid, fmt.Errorf("%w: %d", ErrUnexpectedSourceConnectorBindingOutcome, saved)
	}
}
