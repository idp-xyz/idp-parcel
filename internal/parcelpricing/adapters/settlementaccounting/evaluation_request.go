// Package settlementaccounting 是 parcel-pricing 消费 settlement-accounting 评价请求的适配器（ADR-0025 消费方侧；
// 票 sa-cc/11 裁决 5，口径照 sa-cc/03「裁决」：走 SA 的只读口、取信封所指那一份、不读写侧登记面）。
//
// 它只翻译不判断：把 SA 登记册里的一份评价请求译成本上下文「按评价请求形成评价」的命令——主要范围、计算目的、
// 合格来源引用与计价基准时点，每一格都是引用或时点，一个数字都没有（ADR-0107 / ADR-0013）。评价上留的只有回指；
// SA 的内容一格都不复制进评价。
package settlementaccounting

import (
	"context"
	"errors"
	"fmt"

	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var (
	// ErrUntranslatableReference 表示信封带来的引用在 SA 词汇里构造不出来（空白租户 / 请求 ID）。那是引用坏了，不是
	// 等谁——消费者译码已经拒过两维缺席，这里只可能是两侧词汇表出了分歧。
	ErrUntranslatableReference = errors.New(
		"parcel pricing settlementaccounting adapter: untranslatable evaluation request reference")
	// ErrUntranslatableAnswer 语义与其他消费方适配器的同名哨兵一致：某一侧交出了词汇表之外的内容——SA 记录译不成
	// PP 命令（目的词表外、范围空），或译出的命令被入口答未受理。重投不会变好，原样上抛让失败码落 publish_failed。
	ErrUntranslatableAnswer = errors.New("parcel pricing settlementaccounting adapter: untranslatable answer")
)

// EvaluationRequestSource 是 SA 评价请求登记册的只读半边在本适配器眼里的读口。SA 的 postgres `EvaluationRequests`
// 直接满足它；这里只声明用到的那一口，拿不到 Save（sa-cc/03 裁决「不读写侧登记面」在类型上成立）。
type EvaluationRequestSource interface {
	FindByID(ctx context.Context, key saports.EvaluationRequestKey) (saports.EvaluationRequestRecord, bool, error)
}

// TranslateEvaluationRequest 把 SA 的一份评价请求记录译成本上下文的形成命令（裁决 1 让入口收的那几样）。
//
//   - 租户与回指取登记册的键：回指是 SA 铸造的请求 ID 的字面，不派生、不改写。
//   - 主要范围按字面译成 PricingScopeID——SA 的 PrimaryScopeReference 与 PP 的计价范围是否同一标识空间归两侧 owner
//     裁（票面完成记录的判断项）；这里不猜任何映射。
//   - 计算目的按 SA 封闭词表一格对一格：BUY_SUPPLIER_COST → BUY + SUPPLIER_COST。词表外的值是 SA 词汇变了，拒。
//   - 计价基准时点取发生项的业务时间：它是 TF 在该版本上钉死的属性、随引用带出供形成用（SA 头注）；RequestedAt /
//     RecordedAt 是请求何时被提出 / 登记，不是被计价的那件事何时发生。
//   - 三件来源引用按字面带过去当钥匙，不读它们的内容。
//   - 证据层级由装配方声明、不给默认（入口头注同一取舍）：它是来源数据的属性，本适配器不知道来源是什么。
func TranslateEvaluationRequest(
	record saports.EvaluationRequestRecord,
	evidence ppdomain.EvidenceKind,
) (ppapplication.FormEvaluationFromRequestCommand, error) {
	none := ppapplication.FormEvaluationFromRequestCommand{}
	if !evidence.Declared() {
		return none, fmt.Errorf("%w: evidence kind %q is not one of S / R / P", ErrUntranslatableAnswer, evidence)
	}
	tenant, err := ppdomain.NewTenantID(record.Key.TenantID.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	reference, err := ppdomain.NewEvaluationRequestReference(record.Key.Request.String())
	if err != nil {
		return none, fmt.Errorf("%w: request reference: %v", ErrUntranslatableAnswer, err)
	}
	request := record.Request
	scope, err := ppdomain.NewPricingScopeID(request.Scope().String())
	if err != nil {
		return none, fmt.Errorf("%w: primary scope: %v", ErrUntranslatableAnswer, err)
	}
	direction, purpose, err := translatePurpose(request.Purpose())
	if err != nil {
		return none, err
	}
	sources := request.Sources()
	return ppapplication.FormEvaluationFromRequestCommand{
		Tenant:    tenant,
		Request:   reference,
		Scope:     scope,
		Direction: direction,
		Purpose:   purpose,
		BasisAt:   sources.Occurrence.OccurredAt(),
		Sources: ppports.EligibleSourceReferences{
			Occurrence:        sources.Occurrence.ID().String(),
			OccurrenceVersion: sources.Occurrence.Version().String(),
			FeeItem:           sources.FeeItem.String(),
			SupplierAgreement: sources.Agreement.String(),
		},
		Evidence: evidence,
	}, nil
}

// translatePurpose 把 SA 的计算目的译成 PP 的（方向、目的）一对。SA 的词表封闭且与 PP 的配对一一对应（SA 头注
// 「目的与价格方向在提供方那边成对声明」）；这里逐格写出而不按字面拆串，是让 SA 加一格时这里编译期就得表态。
func translatePurpose(purpose sadomain.CalculationPurpose) (ppdomain.PricingDirection, ppdomain.PricingPurpose, error) {
	switch purpose {
	case sadomain.BuySupplierCost:
		return ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost, nil
	default:
		return "", "", fmt.Errorf("%w: calculation purpose %q", ErrUntranslatableAnswer, purpose.String())
	}
}
