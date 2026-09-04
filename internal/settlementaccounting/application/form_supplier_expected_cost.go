// form_supplier_expected_cost.go 编排 UC-SA-002 步 5 的 BUY 方向：把一份已完成的 BUY
// `PricingEvaluation` 与 TF 的运输收费发生项、费用项目和供应商协议一起采用为供应商预期成本
// 的首版。SELL 方向（客户费用）另有 confirm_charge.go 与 record_charge_adjustment.go；预期成本
// 的计价纠错（步 8）走领域的 AppendCorrection，不在本形成口。
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrUnexpectedExpectedCostSave 说明预期成本登记面交回了封闭集合以外的写入结果。
var ErrUnexpectedExpectedCostSave = errors.New("settlement accounting: unexpected expected cost save outcome")

// ExpectedCostOutcome 是一次预期成本形成的应用处理结果。提供方的四种非完成结果各有去处
// （ADR-0029 按恢复动作分格）：不可计价是终局（不重试、不落零额，AT-SA-175），冲突等裁决，
// 待判断与未形成归未决并指名等谁。`已形成过首版`是专格：同一发生项、费用项目和规则版本的
// 第二个版本身份不是重放也不是冲突——恢复动作是走纠错（AppendCorrection），不是改单。
type ExpectedCostOutcome uint8

const (
	ExpectedCostOutcomeInvalid ExpectedCostOutcome = iota
	ExpectedCostFormed
	ExpectedCostExistingResult
	ExpectedCostAlreadyFormed
	ExpectedCostConflict
	ExpectedCostUnratable
	EvaluationConflict
	ExpectedCostNotAccepted
	ExpectedCostUndecided
)

func (outcome ExpectedCostOutcome) String() string {
	switch outcome {
	case ExpectedCostFormed:
		return "EXPECTED_COST_FORMED"
	case ExpectedCostExistingResult:
		return "EXISTING_EXPECTED_COST"
	case ExpectedCostAlreadyFormed:
		return "EXPECTED_COST_ALREADY_FORMED"
	case ExpectedCostConflict:
		return "EXPECTED_COST_CONFLICT"
	case ExpectedCostUnratable:
		return "PRICING_EXCLUDED"
	case EvaluationConflict:
		return "EVALUATION_CONFLICT"
	case ExpectedCostNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case ExpectedCostUndecided:
		return "EXPECTED_COST_UNDECIDED"
	default:
		return ""
	}
}

// ExpectedCostUndecidedReason 指名形成停在哪一步等谁。评价待判断等提供方以新评价补齐事实
// 或汇率序列；评价未形成等重试；换算步骤缺席等提供方在评价内换算（AT-SA-177，不自行补算）。
type ExpectedCostUndecidedReason uint8

const (
	ExpectedCostUndecidedReasonNone ExpectedCostUndecidedReason = iota
	EvaluationViewUnavailable
	EvaluationPending
	EvaluationNotFormed
	ConversionStepMissing
	ExpectedCostRegistryUnavailable
)

func (reason ExpectedCostUndecidedReason) String() string {
	switch reason {
	case EvaluationViewUnavailable:
		return "EVALUATION_VIEW_UNAVAILABLE"
	case EvaluationPending:
		return "EVALUATION_PENDING"
	case EvaluationNotFormed:
		return "EVALUATION_NOT_FORMED"
	case ConversionStepMissing:
		return "CONVERSION_STEP_MISSING"
	case ExpectedCostRegistryUnavailable:
		return "EXPECTED_COST_REGISTRY_UNAVAILABLE"
	default:
		return ""
	}
}

// FormSupplierExpectedCostCommand 携带形成一份预期成本首版所需、而评价里没有的那几件：
// 发生项（TF 拥有的只读引用）、费用项目、供应商协议快照引用，加上要采用的评价引用与本版
// 身份。金额、币种、换算步骤与规则版本不在命令上——它们整组出自评价（CONTEXT「一份预期成本
// 版本的全部计价结果，整组出自它引用的那一个 BUY PricingEvaluation」），命令带得了它们就有了
// 第二套数字。
type FormSupplierExpectedCostCommand struct {
	TenantID   domain.TenantID
	Version    domain.SupplierCostVersionID
	Occurrence domain.TransportChargeOccurrence
	FeeItem    domain.FeeItemReference
	Agreement  domain.SupplierAgreementReference
	Evaluation domain.BuyEvaluationReference
}

type ExpectedCostResult struct {
	outcome      ExpectedCostOutcome
	reason       ExpectedCostUndecidedReason
	cost         domain.SupplierExpectedCost
	hasCost      bool
	continuation string
}

func (result ExpectedCostResult) Outcome() ExpectedCostOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result ExpectedCostResult) UndecidedReason() ExpectedCostUndecidedReason {
	return result.reason
}

// Cost 在已形成、重放与`已形成过首版`三格交回那份成本。
func (result ExpectedCostResult) Cost() (domain.SupplierExpectedCost, bool) {
	return result.cost, result.hasCost
}

func (result ExpectedCostResult) ContinuationReference() string {
	return result.continuation
}

// FormSupplierExpectedCostDeps 是形成编排的依赖。Costs 与 Registry 是同一张表的读取面与
// 登记面（生产里同一个适配器），分两个接口是为了让只读的调用方拿不到写口。这里刻意没有
// 发布意图：预期成本的消费方（UC-SA-004 逐行匹配、UC-SA-006 经营口径）按版本读它，今天没有
// 任何消费者靠信封驱动；等有了再开，不预开空口。
type FormSupplierExpectedCostDeps struct {
	Evaluations ports.BuyEvaluationView
	Costs       ports.ExpectedCostView
	Registry    ports.ExpectedCostRegistry
	Clock       ports.Clock
}

type FormSupplierExpectedCostHandler struct {
	deps FormSupplierExpectedCostDeps
}

func NewFormSupplierExpectedCostHandler(deps FormSupplierExpectedCostDeps) *FormSupplierExpectedCostHandler {
	return &FormSupplierExpectedCostHandler{deps: deps}
}

// Handle 形成一份预期成本首版：受理 → 取评价采用快照（不存在是提交矛盾）→ 按提供方结果
// 分格（只有已完成往下走）→ FormSupplierExpectedCost（跨币种无换算步骤 → 待判断，不补算）
// → 登记（同版本重放按内容比对分重放/冲突；同三维已有首版交回先到者——纠错走 AppendCorrection）。
func (handler *FormSupplierExpectedCostHandler) Handle(
	ctx context.Context,
	command FormSupplierExpectedCostCommand,
) (ExpectedCostResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" ||
		command.Version.String() == "" || command.Evaluation.String() == "" {
		return ExpectedCostResult{outcome: ExpectedCostNotAccepted}, nil
	}

	adoption, found, err := handler.deps.Evaluations.LoadBuyEvaluation(ctx, command.TenantID, command.Evaluation)
	if err != nil {
		return expectedCostUndecided(EvaluationViewUnavailable, command.Evaluation.String()), nil
	}
	if !found {
		// 指名了不存在的评价：提交矛盾，改单重来——评价由提供方形成，本用例等不来它。
		return ExpectedCostResult{outcome: ExpectedCostNotAccepted}, nil
	}
	switch adoption.Outcome {
	case ports.BuyEvaluationCompleted:
	case ports.BuyEvaluationPending:
		return expectedCostUndecided(EvaluationPending, command.Evaluation.String()), nil
	case ports.BuyEvaluationNotFormed:
		return expectedCostUndecided(EvaluationNotFormed, command.Evaluation.String()), nil
	case ports.BuyEvaluationUnratable:
		// 按该价卡不存在可成立的价格：保存排除依据（评价引用就是它），不形成任何成本行，
		// 不转为待判断反复重试（AT-SA-175）。
		return ExpectedCostResult{outcome: ExpectedCostUnratable,
			continuation: costContinuation("PRICING_EXCLUDED", command.Evaluation.String())}, nil
	case ports.BuyEvaluationConflict:
		return ExpectedCostResult{outcome: EvaluationConflict,
			continuation: costContinuation("EVALUATION_CONFLICT", command.Evaluation.String())}, nil
	default:
		// 提供方多出一种结果而本上下文没接：不静默落进某一格，响亮报错（ADR-0031 同款纪律）。
		return ExpectedCostResult{}, fmt.Errorf("settlement accounting: untranslated buy evaluation outcome %d", adoption.Outcome)
	}

	cost, err := domain.FormSupplierExpectedCost(domain.SupplierExpectedCostSpec{
		Version:            command.Version,
		Occurrence:         command.Occurrence,
		FeeItem:            command.FeeItem,
		RuleVersion:        adoption.RuleVersion,
		Agreement:          command.Agreement,
		Evaluation:         command.Evaluation,
		OriginalCurrency:   adoption.OriginalCurrency,
		OriginalMinor:      adoption.OriginalMinor,
		SettlementCurrency: adoption.SettlementCurrency,
		SettlementMinor:    adoption.SettlementMinor,
		Conversion:         adoption.Conversion,
	})
	if errors.Is(err, domain.ErrConversionStepMissing) {
		// 原币与合同结算币不同而评价未携带换算步骤：保持待判断并记录缺口，不自行取汇率
		// 补算，也不以原币金额充当结算币金额（AT-SA-177）。
		return expectedCostUndecided(ConversionStepMissing, command.Evaluation.String()), nil
	}
	if err != nil {
		return ExpectedCostResult{outcome: ExpectedCostNotAccepted}, nil
	}

	// 同版本身份重放先按内容比对作答，不再提交一次（AT-SA-173「幂等返回既有费用采用，不在
	// 结算中重复算价」的登记面）；这一查与登记之间的窄窗留给 Save 的`已登记`分支兜底。
	existing, found, err := handler.deps.Costs.LoadExpectedCost(ctx, command.TenantID, command.Version)
	if err != nil {
		return expectedCostUndecided(ExpectedCostRegistryUnavailable, command.Version.String()), nil
	}
	if found {
		if expectedCostDigest(existing) != expectedCostDigest(cost) {
			return ExpectedCostResult{outcome: ExpectedCostConflict}, nil
		}
		return ExpectedCostResult{outcome: ExpectedCostExistingResult, cost: existing, hasCost: true}, nil
	}

	saved, err := handler.deps.Registry.Save(ctx, command.TenantID, cost, handler.deps.Clock.Now())
	if err != nil {
		return expectedCostUndecided(ExpectedCostRegistryUnavailable, command.Version.String()), nil
	}
	switch saved {
	case ports.ExpectedCostSaved:
		return ExpectedCostResult{outcome: ExpectedCostFormed, cost: cost, hasCost: true}, nil
	case ports.ExpectedCostAlreadyRecorded:
		return handler.existingCost(ctx, command, cost)
	default:
		return ExpectedCostResult{}, fmt.Errorf("%w: %d", ErrUnexpectedExpectedCostSave, saved)
	}
}

// existingCost 分辨`已登记`的两种成因：同版本身份已有行（重放或请求冲突，按内容比对），
// 或同幂等三维已有首版（另一个版本身份——交回先到的首版，纠错另走）。
func (handler *FormSupplierExpectedCostHandler) existingCost(
	ctx context.Context,
	command FormSupplierExpectedCostCommand,
	formed domain.SupplierExpectedCost,
) (ExpectedCostResult, error) {
	byVersion, found, err := handler.deps.Costs.LoadExpectedCost(ctx, command.TenantID, command.Version)
	if err != nil {
		return expectedCostUndecided(ExpectedCostRegistryUnavailable, command.Version.String()), nil
	}
	if found {
		if expectedCostDigest(byVersion) != expectedCostDigest(formed) {
			// 同一版本身份携带不同内容：请求冲突，原结果不被覆盖（AT-SA-051）。
			return ExpectedCostResult{outcome: ExpectedCostConflict}, nil
		}
		return ExpectedCostResult{outcome: ExpectedCostExistingResult, cost: byVersion, hasCost: true}, nil
	}
	first, found, err := handler.deps.Registry.LoadFirstVersion(
		ctx, command.TenantID, command.Occurrence.ID(), command.FeeItem, formed.RuleVersion())
	if err != nil || !found {
		return expectedCostUndecided(ExpectedCostRegistryUnavailable, command.Version.String()), nil
	}
	return ExpectedCostResult{outcome: ExpectedCostAlreadyFormed, cost: first, hasCost: true,
		continuation: costContinuation("EXPECTED_COST_ALREADY_FORMED", first.Version().String())}, nil
}

func expectedCostUndecided(reason ExpectedCostUndecidedReason, subject string) ExpectedCostResult {
	return ExpectedCostResult{
		outcome:      ExpectedCostUndecided,
		reason:       reason,
		continuation: costContinuation(reason.String(), subject),
	}
}

func costContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

// expectedCostDigest 是同一版本身份的内容比对锚：整组计价结果与三个来源引用任一不同即是另一
// 份内容。经访问器取值而不比结构体——时间字段的单调读数与地点会让两份等价的成本比不相等。
func expectedCostDigest(cost domain.SupplierExpectedCost) string {
	originalCurrency, originalMinor := cost.OriginalAmount()
	settlementCurrency, settlementMinor := cost.SettlementAmount()
	conversion, _ := cost.Conversion()
	occurrence := cost.Occurrence()
	digest := sha256.Sum256([]byte(strings.Join([]string{
		cost.Version().String(),
		occurrence.ID().String(),
		occurrence.Reason().String(),
		occurrence.Version().String(),
		occurrence.OccurredAt().UTC().Format(time.RFC3339Nano),
		cost.FeeItem().String(),
		cost.RuleVersion().String(),
		cost.Agreement().String(),
		cost.Evaluation().String(),
		originalCurrency.String(),
		fmt.Sprintf("%d", originalMinor),
		settlementCurrency.String(),
		fmt.Sprintf("%d", settlementMinor),
		conversion.String(),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}
