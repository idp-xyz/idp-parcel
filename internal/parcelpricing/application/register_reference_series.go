package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// ErrUnexpectedReferenceSeriesOutcome 说明序列登记册交回了结果代数以外的写入结果。
var ErrUnexpectedReferenceSeriesOutcome = errors.New("parcel pricing: unexpected reference series registration outcome")

// RegisterReferenceSeriesOutcome 是序列登记请求的应用处理结果。
type RegisterReferenceSeriesOutcome uint8

const (
	RegisterReferenceSeriesOutcomeInvalid RegisterReferenceSeriesOutcome = iota
	// ReferenceSeriesRecorded：新序列版本已入册。
	ReferenceSeriesRecorded
	// ReferenceSeriesAlreadyOnRegister：同版本同内容已在册，重复登记是幂等重放。
	ReferenceSeriesAlreadyOnRegister
	// ReferenceSeriesRegistrationConflict：同版本引用装了不同取值。取值更正必须
	// 形成新序列版本（CONTEXT），原行不被顶替，续办属治理裁决。
	ReferenceSeriesRegistrationConflict
	// ReferenceSeriesRegistrationIncomparable：同版本引用已按另一套规范化形状在册，
	// 摘要不可比——不是冲突，也不是幂等重放。
	ReferenceSeriesRegistrationIncomparable
	// ReferenceSeriesRegistrationNotAccepted：请求不合法（零值登记），未到达登记册。
	ReferenceSeriesRegistrationNotAccepted
	// ReferenceSeriesRegistrationUndecided：依赖故障，登记与否未知，原因随错误交回。
	ReferenceSeriesRegistrationUndecided
)

func (outcome RegisterReferenceSeriesOutcome) String() string {
	switch outcome {
	case ReferenceSeriesRecorded:
		return "RECORDED"
	case ReferenceSeriesAlreadyOnRegister:
		return "ALREADY_REGISTERED"
	case ReferenceSeriesRegistrationConflict:
		return "CONTENT_CONFLICT"
	case ReferenceSeriesRegistrationIncomparable:
		return "CANONICALIZATION_DIFFERS"
	case ReferenceSeriesRegistrationNotAccepted:
		return "NOT_ACCEPTED"
	case ReferenceSeriesRegistrationUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// RegisterReferenceSeriesCommand 携带一次序列登记请求。登记本体（来源标识、期次取值
// 与凭证、口径引用、登记责任方、更正关系）全部在领域对象内，编排不拆解它。
type RegisterReferenceSeriesCommand struct {
	Registration domain.ReferenceSeriesRegistration
}

type RegisterReferenceSeriesDeps struct {
	Register ports.ReferenceSeriesRegister
}

// RegisterReferenceSeriesHandler 编排序列登记用例（票 08 件③）。证据与责任判断全在
// 领域与登记册（登记门在构造器，冲突判定在登记册的结果代数），这里只做受理与答案翻译。
type RegisterReferenceSeriesHandler struct {
	deps RegisterReferenceSeriesDeps
}

func NewRegisterReferenceSeriesHandler(deps RegisterReferenceSeriesDeps) *RegisterReferenceSeriesHandler {
	return &RegisterReferenceSeriesHandler{deps: deps}
}

// Handle 把一次登记请求推进到登记册答案。依赖故障不吞：登记是治理动作，操作者必须
// 拿到失败原因才能续办。
func (handler *RegisterReferenceSeriesHandler) Handle(
	ctx context.Context,
	command RegisterReferenceSeriesCommand,
) (RegisterReferenceSeriesOutcome, error) {
	// 零值登记连租户都指不出来。构造器保证非零登记整体立得住，所以受理门只需分辨
	// 「根本没构造过」。
	if command.Registration.Tenant().String() == "" {
		return ReferenceSeriesRegistrationNotAccepted, nil
	}

	saved, err := handler.deps.Register.Register(ctx, command.Registration)
	if err != nil {
		return ReferenceSeriesRegistrationUndecided, fmt.Errorf("register reference series: %w", err)
	}
	switch saved {
	case ports.ReferenceSeriesRegistered:
		return ReferenceSeriesRecorded, nil
	case ports.ReferenceSeriesAlreadyRegistered:
		return ReferenceSeriesAlreadyOnRegister, nil
	case ports.ReferenceSeriesContentConflict:
		return ReferenceSeriesRegistrationConflict, nil
	case ports.ReferenceSeriesCanonicalizationDiffers:
		return ReferenceSeriesRegistrationIncomparable, nil
	default:
		return RegisterReferenceSeriesOutcomeInvalid, fmt.Errorf("%w: %d", ErrUnexpectedReferenceSeriesOutcome, saved)
	}
}
