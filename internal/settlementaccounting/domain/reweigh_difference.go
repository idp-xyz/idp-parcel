package domain

// ReweighEvaluation 是新评价交来的两格金额与换算依据，照评价原样。币种与换算依据保持原始字符串：形态归形成门
// （FormChargeAdjustment）判，这里只比较，不替它校验——校验在这里先拒一遍，就等于把同一条判据写成了两份。
type ReweighEvaluation struct {
	OriginalCurrency   string
	OriginalMinor      int64
	SettlementCurrency string
	SettlementMinor    int64
	Conversion         string
}

// ReweighDifference 是新评价相对一笔已确认费用算出的复磅差额：要不要记一笔计价纠错、记哪个方向、记多少。
// 金额都是绝对值，方向另给。Needed 为假时其余各格没有意义。
type ReweighDifference struct {
	// Needed 为假：结算币差额为零，不记。
	Needed    bool
	Direction AdjustmentDirection
	// SettlementMinor 是结算币差额的绝对值，恒不为零。
	SettlementMinor int64
	// CarriesOriginalDifference 为真：原币差额连同它的币种与换算依据一并记；为假：按结算币记，OriginalCurrency
	// 取新评价的结算币种、OriginalMinor 等于 SettlementMinor、Conversion 为空，正是形成门里「同币种两额相等」那一格。
	CarriesOriginalDifference bool
	OriginalCurrency          string
	OriginalMinor             int64
	Conversion                string
}

// SettleReweighDifference 算新评价相对这笔费用的复磅差额。差额按结算币判：新评价的结算币金额减去这笔费用按计价该收的
// 净额，即费用加上已有的计价纠错；为零不记，不管原币那一格差多少——头部承运商的复核调整都按账单币种开。商业让利不在
// 这份净额里：它是另凭授权让掉的，不是哪一份评价算出来的，算进去，下一次重估就会把让掉的钱记成借项收回来。调整里
// 凡不是计价纠错的一概不计，所以 adjustments 可以原样给这笔费用上的全部调整。
//
// 结算币差额不为零时，原币可比（新评价与每一笔既有计价纠错的原币种都是费用的原币种）、原币差额不为零且与结算币差额
// 同号，才连原币差额一起记，换算依据取新评价的；其余一律按结算币记。形成门的两道限制（同币种两额必须相等；跨币种必带
// 换算依据）先在这里对一遍，对不上也按结算币记：放过去只会被形成门拒收，调用方把拒收当错误上抛，这一代从此一直重投。
//
// 贷项会不会把费用让成负数看的是全部调整（含商业让利），不在这里判。
func SettleReweighDifference(
	charge CustomerCharge,
	adjustments []ChargeAdjustment,
	evaluation ReweighEvaluation,
) ReweighDifference {
	_, chargeSettlementMinor := charge.SettlementAmount()
	chargeOriginalCurrency, chargeOriginalMinor := charge.OriginalAmount()
	netSettlement, netOriginal := chargeSettlementMinor, chargeOriginalMinor
	// 原币金额只在同一种币上才能相加相减：新评价与既有的每一笔计价纠错，原币种都得是费用的原币种。
	originalComparable := evaluation.OriginalCurrency == chargeOriginalCurrency.String()
	for _, adjustment := range adjustments {
		if adjustment.Kind() != PricingCorrection {
			continue
		}
		_, settlementMinor := adjustment.SettlementAmount()
		netSettlement += adjustment.Direction().Signed(settlementMinor)
		adjustmentOriginalCurrency, originalMinor := adjustment.OriginalAmount()
		if adjustmentOriginalCurrency != chargeOriginalCurrency {
			originalComparable = false
		}
		netOriginal += adjustment.Direction().Signed(originalMinor)
	}
	settlementDelta := evaluation.SettlementMinor - netSettlement
	if settlementDelta == 0 {
		return ReweighDifference{}
	}
	difference := ReweighDifference{
		Needed:          true,
		Direction:       AdjustmentDebit,
		SettlementMinor: settlementDelta,
	}
	if settlementDelta < 0 {
		difference.Direction = AdjustmentCredit
		difference.SettlementMinor = -settlementDelta
	}
	difference.OriginalCurrency = evaluation.SettlementCurrency
	difference.OriginalMinor = difference.SettlementMinor
	if !originalComparable {
		return difference
	}
	originalDelta := evaluation.OriginalMinor - netOriginal
	sameSign := originalDelta != 0 && (originalDelta > 0) == (settlementDelta > 0)
	originalAmount := originalDelta
	if originalAmount < 0 {
		originalAmount = -originalAmount
	}
	sameCurrency := chargeOriginalCurrency.String() == evaluation.SettlementCurrency
	fitsGate := (sameCurrency && originalAmount == difference.SettlementMinor) ||
		(!sameCurrency && evaluation.Conversion != "")
	if sameSign && fitsGate {
		difference.CarriesOriginalDifference = true
		difference.OriginalCurrency = chargeOriginalCurrency.String()
		difference.OriginalMinor = originalAmount
		difference.Conversion = evaluation.Conversion
	}
	return difference
}
