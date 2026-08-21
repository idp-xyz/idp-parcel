package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// ErrUnexpectedPriceCardOutcome 说明价卡仓储交回了结果代数以外的写入结果。
var ErrUnexpectedPriceCardOutcome = errors.New("parcel pricing: unexpected price card registration outcome")

// RegisterPriceCardOutcome 是价卡登记请求的应用处理结果。
type RegisterPriceCardOutcome uint8

const (
	RegisterPriceCardOutcomeInvalid RegisterPriceCardOutcome = iota
	// PriceCardRecorded：新版本已入册。
	PriceCardRecorded
	// PriceCardAlreadyOnRegister：同版本同内容已在册，重复登记是幂等重放。
	PriceCardAlreadyOnRegister
	// PriceCardRegistrationConflict：版本内容冲突（同版本引用、同规范化版本、异
	// 内容摘要）。原行不被顶替，续办属治理裁决。
	PriceCardRegistrationConflict
	// PriceCardRegistrationIncomparable：同版本引用已按另一套规范化版本在册，摘要
	// 不可比（ADR-0014）——不是冲突，也不是幂等重放。
	PriceCardRegistrationIncomparable
	// PriceCardRegistrationNotAccepted：请求不合法（零值登记），未到达仓储。
	PriceCardRegistrationNotAccepted
	// PriceCardRegistrationUndecided：依赖故障，登记与否未知，原因随错误交回。
	PriceCardRegistrationUndecided
)

func (outcome RegisterPriceCardOutcome) String() string {
	switch outcome {
	case PriceCardRecorded:
		return "RECORDED"
	case PriceCardAlreadyOnRegister:
		return "ALREADY_REGISTERED"
	case PriceCardRegistrationConflict:
		return "CONTENT_CONFLICT"
	case PriceCardRegistrationIncomparable:
		return "CANONICALIZATION_DIFFERS"
	case PriceCardRegistrationNotAccepted:
		return "NOT_ACCEPTED"
	case PriceCardRegistrationUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// RegisterPriceCardCommand 携带一次价卡登记请求。登记本体（方案版本、源文件身份、
// 方向授权引用、发布批准责任方）全部在领域对象内，编排不拆解它。
type RegisterPriceCardCommand struct {
	Registration domain.PriceCardRegistration
}

type RegisterPriceCardDeps struct {
	Catalog ports.PriceCardCatalog
}

// RegisterPriceCardHandler 编排价卡登记用例（票 07 件③）。治理判断全在领域与仓储
// （登记门在构造器，冲突判定在仓储的结果代数），这里只做受理与答案翻译。
type RegisterPriceCardHandler struct {
	deps RegisterPriceCardDeps
}

func NewRegisterPriceCardHandler(deps RegisterPriceCardDeps) *RegisterPriceCardHandler {
	return &RegisterPriceCardHandler{deps: deps}
}

// Handle 把一次登记请求推进到登记册答案。与评价用例不同，依赖故障不吞：登记是治理
// 动作，操作者必须拿到失败原因才能续办，答一个没有成因的`未决`等于没答。
func (handler *RegisterPriceCardHandler) Handle(
	ctx context.Context,
	command RegisterPriceCardCommand,
) (RegisterPriceCardOutcome, error) {
	// 零值登记连租户都指不出来。构造器保证非零登记整体立得住，所以受理门只需分辨
	// 「根本没构造过」。
	if command.Registration.Tenant().String() == "" {
		return PriceCardRegistrationNotAccepted, nil
	}

	saved, err := handler.deps.Catalog.Register(ctx, command.Registration)
	if err != nil {
		return PriceCardRegistrationUndecided, fmt.Errorf("register price card: %w", err)
	}
	switch saved {
	case ports.PriceCardRegistered:
		return PriceCardRecorded, nil
	case ports.PriceCardAlreadyRegistered:
		return PriceCardAlreadyOnRegister, nil
	case ports.PriceCardContentConflict:
		return PriceCardRegistrationConflict, nil
	case ports.PriceCardCanonicalizationDiffers:
		return PriceCardRegistrationIncomparable, nil
	default:
		return RegisterPriceCardOutcomeInvalid, fmt.Errorf("%w: %d", ErrUnexpectedPriceCardOutcome, saved)
	}
}
