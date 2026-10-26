package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// ErrUnexpectedReferenceReviewDutyRuleOutcome 说明规则册交回了结果代数以外的写入结果。
var ErrUnexpectedReferenceReviewDutyRuleOutcome = errors.New("parcel pricing: unexpected reference review duty rule registration outcome")

// RegisterReferenceReviewDutyRuleOutcome 是参考复核职责规则登记的应用处理结果。按恢复动作分格：入册与重放都已落定；冲突是
// 租户已登了另一条——规则不在册上原地改，要改的人去走治理；不受理是请求本身立不住；未决是依赖故障。
type RegisterReferenceReviewDutyRuleOutcome uint8

const (
	RegisterReferenceReviewDutyRuleOutcomeInvalid RegisterReferenceReviewDutyRuleOutcome = iota
	ReferenceReviewDutyRuleRecorded
	ReferenceReviewDutyRuleAlreadyOnRegister
	ReferenceReviewDutyRuleConflict
	ReferenceReviewDutyRuleNotAccepted
	ReferenceReviewDutyRuleUndecided
)

func (outcome RegisterReferenceReviewDutyRuleOutcome) String() string {
	switch outcome {
	case ReferenceReviewDutyRuleRecorded:
		return "RECORDED"
	case ReferenceReviewDutyRuleAlreadyOnRegister:
		return "ALREADY_REGISTERED"
	case ReferenceReviewDutyRuleConflict:
		return "CONTENT_CONFLICT"
	case ReferenceReviewDutyRuleNotAccepted:
		return "NOT_ACCEPTED"
	case ReferenceReviewDutyRuleUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// RegisterReferenceReviewDutyRuleCommand 携带一条规则，租户与两格都在领域对象内。
type RegisterReferenceReviewDutyRuleCommand struct {
	Rule domain.ReferenceReviewDutyRule
}

// RegisterReferenceReviewDutyRuleHandler 编排参考复核职责规则的登记（ADR-0262 决定一）：受控登记口经它把一租户一条落进规则册。
// 在线登记面随治理写面那一族另裁，不在这里。
type RegisterReferenceReviewDutyRuleHandler struct {
	rules ports.ReferenceReviewDutyRuleRegistry
}

func NewRegisterReferenceReviewDutyRuleHandler(rules ports.ReferenceReviewDutyRuleRegistry) (*RegisterReferenceReviewDutyRuleHandler, error) {
	if rules == nil {
		return nil, errors.New("parcel pricing: reference review duty rule registration needs a rule registry")
	}
	return &RegisterReferenceReviewDutyRuleHandler{rules: rules}, nil
}

// Handle 把一条规则推进到规则册答案。依赖故障不吞：登记是治理动作，操作者要拿到原因才能续办。
func (handler *RegisterReferenceReviewDutyRuleHandler) Handle(
	ctx context.Context,
	command RegisterReferenceReviewDutyRuleCommand,
) (RegisterReferenceReviewDutyRuleOutcome, error) {
	if command.Rule.Tenant().String() == "" {
		return ReferenceReviewDutyRuleNotAccepted, nil
	}
	saved, err := handler.rules.SaveReferenceReviewDutyRule(ctx, command.Rule)
	if err != nil {
		return ReferenceReviewDutyRuleUndecided, fmt.Errorf("register reference review duty rule: %w", err)
	}
	switch saved {
	case ports.ReferenceReviewDutyRuleSaved:
		return ReferenceReviewDutyRuleRecorded, nil
	case ports.ReferenceReviewDutyRuleAlreadyRegistered:
		return ReferenceReviewDutyRuleAlreadyOnRegister, nil
	case ports.ReferenceReviewDutyRuleContentConflict:
		return ReferenceReviewDutyRuleConflict, nil
	default:
		return RegisterReferenceReviewDutyRuleOutcomeInvalid, fmt.Errorf("%w: %d", ErrUnexpectedReferenceReviewDutyRuleOutcome, saved)
	}
}
