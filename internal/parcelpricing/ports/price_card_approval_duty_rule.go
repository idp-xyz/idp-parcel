package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 价卡发布审批职责规则的端口（ADR-0101 决定六；spec 自决第 2 格；票 price-card-import/04）。规则是租户治理参数（实例
// 半边，参数登记册「价卡发布审批职责规则」）：机制半边只立读口与表，写口今天只给装配与测试用，登记面归治理写面那一族
// 另裁——同 party-commercial 那一条（ADR-0126 决定五）。

// PriceCardApprovalDutyRuleSaveOutcome 是规则登记落点的封闭代数（ADR-0031）。
type PriceCardApprovalDutyRuleSaveOutcome uint8

const (
	PriceCardApprovalDutyRuleSaveOutcomeInvalid PriceCardApprovalDutyRuleSaveOutcome = iota
	PriceCardApprovalDutyRuleSaved
	// PriceCardApprovalDutyRuleAlreadyRegistered：同租户同内容已在册，重放。
	PriceCardApprovalDutyRuleAlreadyRegistered
	// PriceCardApprovalDutyRuleContentConflict：同租户已登了另一条，原行不被顶替。
	PriceCardApprovalDutyRuleContentConflict
)

func (outcome PriceCardApprovalDutyRuleSaveOutcome) String() string {
	switch outcome {
	case PriceCardApprovalDutyRuleSaved:
		return "SAVED"
	case PriceCardApprovalDutyRuleAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case PriceCardApprovalDutyRuleContentConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}

// PriceCardApprovalDutyRuleView 按租户读规则。found=false 就是未登记——批准门据此答`未配置`、不放行，不以任何默认代替。
type PriceCardApprovalDutyRuleView interface {
	LoadPriceCardApprovalDutyRule(ctx context.Context, tenant domain.TenantID) (domain.PriceCardApprovalDutyRule, bool, error)
}

// PriceCardApprovalDutyRuleRegistry 在读口之上加登记写口，一租户一条。今天没有治理写面调它。
type PriceCardApprovalDutyRuleRegistry interface {
	PriceCardApprovalDutyRuleView
	SavePriceCardApprovalDutyRule(ctx context.Context, rule domain.PriceCardApprovalDutyRule) (PriceCardApprovalDutyRuleSaveOutcome, error)
}
