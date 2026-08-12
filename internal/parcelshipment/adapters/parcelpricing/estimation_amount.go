// Package parcelpricing 是 parcel-shipment 消费 parcel-pricing 纯评价的适配器
// （ADR-0025 消费方侧；ADR-0048 第三增量的金额缝装配器）。
//
// 它只翻译不判断：声明画像译计价输入、评价合计译控制金额；价卡、计价范围、区域与
// 结算币种是租户实例参数，经 EstimationBasisSource 进来，未配置停在未形成。
package parcelpricing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ErrUntranslatableAnswer 语义与其他消费方适配器的同名哨兵一致：某一侧交出了词汇表
// 之外的内容，是编程错误不是业务答案。
var ErrUntranslatableAnswer = errors.New("parcel shipment parcelpricing adapter: untranslatable answer")

// EstimationBasis 是一次接受前估价的实例参数包：按哪份价卡、哪个计价范围与区域、以哪个
// 结算币种（含最小单位位数）出结果。全部属租户实例半边（SET-01/PAR-SET-*）：真实价卡
// 不可得时本包不虚构任何一项。
type EstimationBasis struct {
	Plan        ppdomain.PricingPlanVersion
	Scope       ppdomain.PricingScopeID
	Zone        string
	Currency    ppdomain.Currency
	MinorDigits int
	Evidence    ppdomain.EvidenceKind
}

func (basis EstimationBasis) valid() bool {
	return strings.TrimSpace(basis.Zone) != "" && basis.MinorDigits >= 0 && basis.MinorDigits <= 8
}

// EstimationBasisSource 为一次控制请求交回估价基。第二个返回值为 false 即「显式未配置」。
type EstimationBasisSource interface {
	FindEstimationBasis(
		ctx context.Context,
		request psports.FinancialControlRequest,
	) (EstimationBasis, bool, error)
}

// EstimationAmountSource 从声明画像与合成/真实价卡形成控制金额。它结构性实现
// settlementaccounting 适配器的 ControlAmountSource（同签名），装配处直接接上。
//
// 金额 = 当前提交版本逐成员估价合计：控制作用在整份委托上（FinancialControlRequest 的
// 版本级语义），少一个成员的估价就少占一份资金。
type EstimationAmountSource struct {
	requests psports.ShipmentRequestRepository
	basis    EstimationBasisSource
}

// NewEstimationAmountSource 装配两半。basis 允许为 nil——实例未配置的诚实表达。
func NewEstimationAmountSource(
	requests psports.ShipmentRequestRepository,
	basis EstimationBasisSource,
) *EstimationAmountSource {
	return &EstimationAmountSource{requests: requests, basis: basis}
}

// FormControlAmount 逐段停在「未形成」而不发明数字：估价基未配置、委托或版本对不上、
// 成员缺画像、单位读不出、评价没给合计（待判断/不可计价/失败），都交 false——一个在
// 这些缺口上凑出来的金额，占的是客户的真金白银。依赖读不回才是错误。
func (source *EstimationAmountSource) FormControlAmount(
	ctx context.Context,
	request psports.FinancialControlRequest,
) (int64, bool, error) {
	if source.requests == nil || source.basis == nil {
		return 0, false, nil
	}
	basis, found, err := source.basis.FindEstimationBasis(ctx, request)
	if err != nil {
		return 0, false, fmt.Errorf("find estimation basis: %w", err)
	}
	if !found {
		return 0, false, nil
	}
	if !basis.valid() {
		return 0, false, fmt.Errorf("%w: estimation basis is incomplete", ErrUntranslatableAnswer)
	}

	shipment, found, err := source.requests.FindBySourceIdentity(ctx, request.Identity)
	if err != nil {
		return 0, false, fmt.Errorf("find shipment request: %w", err)
	}
	if !found || shipment.ShipmentRequestID() != request.ShipmentRequestID {
		return 0, false, nil
	}
	version := shipment.CurrentSubmissionVersion()
	if version.VersionID() != request.SubmissionVersion {
		// 版本已换代：为一个不再待判断的版本占资金，释放都无从按当前版本认领。
		return 0, false, nil
	}

	total := int64(0)
	for _, member := range version.DeclaredParcelIDs() {
		profile, declared := version.ProfileFor(member)
		if !declared {
			// 客户没报测量，估价缺输入。停下等资料补齐（UC-PS-002 的路），不是拿零凑。
			return 0, false, nil
		}
		amount, formed, err := source.estimateMember(request, basis, member, profile)
		if err != nil || !formed {
			return 0, false, err
		}
		total += amount
	}
	if total <= 0 {
		// 合计非正形不成资金占用：SA 在受理期拒绝非正数金额，这里交出去只会变成
		// `未受理`。停在未形成，让人去看这份价卡为什么估出零。
		return 0, false, nil
	}
	return total, true, nil
}

// estimateMember 为一个成员执行一次纯评价并译回最小币单位金额。
func (source *EstimationAmountSource) estimateMember(
	request psports.FinancialControlRequest,
	basis EstimationBasis,
	member psdomain.DeclaredParcelID,
	profile psdomain.DeclaredParcelProfile,
) (int64, bool, error) {
	input, formed, err := source.inputFor(request, basis, member, profile)
	if err != nil || !formed {
		return 0, false, err
	}

	evaluationID, err := ppdomain.NewEvaluationID(estimationIdentity(request, member))
	if err != nil {
		return 0, false, fmt.Errorf("%w: evaluation ID: %v", ErrUntranslatableAnswer, err)
	}
	evaluationRequest, err := ppdomain.NewEvaluationRequest(evaluationID, basis.Plan, input, basis.Evidence)
	if err != nil {
		return 0, false, fmt.Errorf("%w: evaluation request: %v", ErrUntranslatableAnswer, err)
	}
	evaluation := ppdomain.EvaluatePricing(evaluationRequest)
	moneyTotal, completed := evaluation.Total()
	if !completed {
		// 待判断、不可计价、冲突与失败都没有合计。控制金额不替评价补一个数——四种结果
		// 互相顶替正是提供方 CONTEXT 明禁的。
		return 0, false, nil
	}
	if moneyTotal.Currency() != basis.Currency {
		return 0, false, fmt.Errorf("%w: evaluation currency %s differs from settlement %s",
			ErrUntranslatableAnswer, moneyTotal.Currency(), basis.Currency)
	}
	minor, err := minorUnitsOf(moneyTotal.Amount().String(), basis.MinorDigits)
	if err != nil {
		return 0, false, fmt.Errorf("%w: total %s: %v", ErrUntranslatableAnswer, moneyTotal.Amount(), err)
	}
	return minor, true, nil
}

// inputFor 把一张声明画像译成计价输入。单位读不出交 false 不猜——猜错单位占错三个量级
// 的资金。
func (source *EstimationAmountSource) inputFor(
	request psports.FinancialControlRequest,
	basis EstimationBasis,
	member psdomain.DeclaredParcelID,
	profile psdomain.DeclaredParcelProfile,
) (ppdomain.PricingInputSnapshot, bool, error) {
	// 单位符号折大写后进提供方的封闭词表（kg 与 KG 是同一个单位的两种写法，折叠是翻译
	// 不是判断）；词表外的单位仍拒绝——猜错单位占错三个量级的资金。
	weightUnit, err := ppdomain.NewWeightUnit(strings.ToUpper(profile.Measurement().Weight().Unit().String()))
	if err != nil {
		return ppdomain.PricingInputSnapshot{}, false, nil
	}
	weight, err := ppdomain.NewWeightFromString(profile.Measurement().Weight().Value().String(), weightUnit)
	if err != nil {
		return ppdomain.PricingInputSnapshot{}, false, nil
	}

	var dimensions *ppdomain.Dimensions
	if declared, present := profile.Measurement().Dimensions(); present {
		lengthUnit, err := ppdomain.NewLengthUnit(strings.ToUpper(declared.Unit().String()))
		if err != nil {
			return ppdomain.PricingInputSnapshot{}, false, nil
		}
		length, lengthErr := ppdomain.ParseDecimal(declared.Length().String())
		width, widthErr := ppdomain.ParseDecimal(declared.Width().String())
		height, heightErr := ppdomain.ParseDecimal(declared.Height().String())
		if lengthErr != nil || widthErr != nil || heightErr != nil {
			return ppdomain.PricingInputSnapshot{}, false, nil
		}
		sides, err := ppdomain.NewDimensions(length, width, height, lengthUnit)
		if err != nil {
			return ppdomain.PricingInputSnapshot{}, false, nil
		}
		dimensions = &sides
	}

	tenant, err := ppdomain.NewTenantID(request.Identity.TenantID().String())
	if err != nil {
		return ppdomain.PricingInputSnapshot{}, false, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	subject, err := ppdomain.NewEstimateSubject(estimationIdentity(request, member))
	if err != nil {
		return ppdomain.PricingInputSnapshot{}, false, fmt.Errorf("%w: estimate subject: %v", ErrUntranslatableAnswer, err)
	}
	input, err := ppdomain.NewPricingInputSnapshot(
		tenant,
		basis.Scope,
		subject,
		basis.Zone,
		weight,
		dimensions,
		request.AsOf.At(),
	)
	if err != nil {
		return ppdomain.PricingInputSnapshot{}, false, fmt.Errorf("%w: pricing input: %v", ErrUntranslatableAnswer, err)
	}
	input, err = input.WithSettlementCurrency(basis.Currency)
	if err != nil {
		return ppdomain.PricingInputSnapshot{}, false, fmt.Errorf("%w: settlement currency: %v", ErrUntranslatableAnswer, err)
	}
	return input, true, nil
}

// estimationIdentity 由请求键与成员确定性派生：同一请求重放得到同一试算对象与评价标识，
// 纯评价因此可比对可复算。
func estimationIdentity(request psports.FinancialControlRequest, member psdomain.DeclaredParcelID) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		request.Identity.TenantID().String(),
		request.ShipmentRequestID.String(),
		request.SubmissionVersion.String(),
		member.String(),
	}, "\x00")))
	return "EST-" + hex.EncodeToString(digest[:8])
}

// minorUnitsOf 把规范十进制金额换成最小币单位整数。字符串算术不走浮点；小数位超过币种
// 声明的位数按装配错误拒绝——静默截断是在改一个要占用的金额。
func minorUnitsOf(canonical string, digits int) (int64, error) {
	if canonical == "" || strings.HasPrefix(canonical, "-") {
		return 0, errors.New("amount is empty or negative")
	}
	whole, fraction, _ := strings.Cut(canonical, ".")
	if len(fraction) > digits {
		return 0, fmt.Errorf("amount %s exceeds %d minor digits", canonical, digits)
	}
	fraction += strings.Repeat("0", digits-len(fraction))
	combined := strings.TrimLeft(whole+fraction, "0")
	if combined == "" {
		return 0, nil
	}
	var minor int64
	for _, character := range combined {
		if character < '0' || character > '9' {
			return 0, fmt.Errorf("amount %s is not canonical", canonical)
		}
		digit := int64(character - '0')
		if minor > (1<<62)/10 {
			return 0, fmt.Errorf("amount %s overflows", canonical)
		}
		minor = minor*10 + digit
	}
	return minor, nil
}
