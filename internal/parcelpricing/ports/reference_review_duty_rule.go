package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 参考复核职责规则的端口（ADR-0262 决定一）。规则是租户治理参数（实例半边）：机制半边立读口、写口与表。写口给受控 CLI
// （parcel-pricing-register 的规则登记）、装配与测试用——有意不照价卡发布审批职责规则那一套只有读口可用的先例：复核门在
// 规则未登记时不放行，而演示租户要由种子在复核之前登一条，得有一个受控登记口。在线登记面随治理写面那一族另裁。

// ReferenceReviewDutyRuleSaveOutcome 是规则登记落点的封闭代数（ADR-0031）。
type ReferenceReviewDutyRuleSaveOutcome uint8

const (
	ReferenceReviewDutyRuleSaveOutcomeInvalid ReferenceReviewDutyRuleSaveOutcome = iota
	ReferenceReviewDutyRuleSaved
	// ReferenceReviewDutyRuleAlreadyRegistered：同租户同内容已在册，重放。
	ReferenceReviewDutyRuleAlreadyRegistered
	// ReferenceReviewDutyRuleContentConflict：同租户已登了另一条，原行不被顶替。
	ReferenceReviewDutyRuleContentConflict
)

func (outcome ReferenceReviewDutyRuleSaveOutcome) String() string {
	switch outcome {
	case ReferenceReviewDutyRuleSaved:
		return "SAVED"
	case ReferenceReviewDutyRuleAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case ReferenceReviewDutyRuleContentConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}

// ReferenceReviewDutyRuleView 按租户读规则。found=false 就是未登记——人工复核据此答`未配置`、不放行，不以任何默认代替。
type ReferenceReviewDutyRuleView interface {
	LoadReferenceReviewDutyRule(ctx context.Context, tenant domain.TenantID) (domain.ReferenceReviewDutyRule, bool, error)
}

// ReferenceReviewDutyRuleRegistry 在读口之上加登记写口，一租户一条：同内容重放，异内容答冲突、原行不被顶替。
type ReferenceReviewDutyRuleRegistry interface {
	ReferenceReviewDutyRuleView
	SaveReferenceReviewDutyRule(ctx context.Context, rule domain.ReferenceReviewDutyRule) (ReferenceReviewDutyRuleSaveOutcome, error)
}
