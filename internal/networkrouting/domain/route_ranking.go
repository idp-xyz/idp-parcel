package domain

import "errors"

var ErrInvalidRanking = errors.New("network routing: invalid ranking facts")

// RankingForm 是路由策略版本声明的内置排序形态（ADR-0146 决定二、七）。形态的判断逻辑归
// 产品、在这里执行；租户只选形态、填取值。
type RankingForm uint8

const (
	RankingFormUndeclared RankingForm = iota
	// CostSingleDimensionRanking 是 `PAR-NET-16` 已确认的首发形态：满足硬约束与时间可行性
	// 之后按成本单维择优。
	CostSingleDimensionRanking
)

func (form RankingForm) String() string {
	switch form {
	case CostSingleDimensionRanking:
		return "COST_SINGLE_DIMENSION"
	default:
		return ""
	}
}

// CandidateCostState 是候选成本事实的封闭三格。待判断与不可计价分开，是因为恢复动作不同：
// 前者等评价形成，后者要改价卡或候选集合。
type CandidateCostState uint8

const (
	CandidateCostStateInvalid CandidateCostState = iota
	CandidateCostPriced
	CandidateCostPending
	CandidateCostUnpriceable
)

func (state CandidateCostState) String() string {
	switch state {
	case CandidateCostPriced:
		return "PRICED"
	case CandidateCostPending:
		return "PENDING"
	case CandidateCostUnpriceable:
		return "UNPRICEABLE"
	default:
		return ""
	}
}

// CandidateCostFact 是一个候选在成本单维下的可比事实：已计价时带最小币单位金额与币种，
// 待判断与不可计价两格不带金额——没有金额就是没有，不以零表示。金额从哪里来（按各候选
// 自己的体积系数与进位算出的 BUY 评价）归取数侧，领域只比较不折算。
type CandidateCostFact struct {
	candidate   CandidateID
	state       CandidateCostState
	amountMinor int64
	currency    requiredValue
}

func NewPricedCandidateCost(candidate CandidateID, amountMinor int64, currency string) (CandidateCostFact, error) {
	code, err := newRequiredValue("candidate cost currency", currency)
	if err != nil || !candidate.valid() || amountMinor < 0 {
		return CandidateCostFact{}, ErrInvalidRanking
	}
	return CandidateCostFact{
		candidate:   candidate,
		state:       CandidateCostPriced,
		amountMinor: amountMinor,
		currency:    code,
	}, nil
}

func NewPendingCandidateCost(candidate CandidateID) (CandidateCostFact, error) {
	return unpricedCandidateCost(candidate, CandidateCostPending)
}

func NewUnpriceableCandidateCost(candidate CandidateID) (CandidateCostFact, error) {
	return unpricedCandidateCost(candidate, CandidateCostUnpriceable)
}

func unpricedCandidateCost(candidate CandidateID, state CandidateCostState) (CandidateCostFact, error) {
	if !candidate.valid() {
		return CandidateCostFact{}, ErrInvalidRanking
	}
	return CandidateCostFact{candidate: candidate, state: state}, nil
}

// RankingOutcome 是一次按形态排序的封闭结果。
type RankingOutcome uint8

const (
	RankingOutcomeInvalid RankingOutcome = iota
	RankingSelected
	// RankingNoQualifiedCandidate 是没有候选通过硬约束与时间可行性——那是`无当前有效路由`
	// 或未决的信号，由编排按候选空间收没收敛去分，不是排序能替策略回答的。
	RankingNoQualifiedCandidate
	// RankingFormNotDeclared 是有合格候选、而路由策略版本没有声明排序形态：租户还没选，
	// 不替它选一种。
	RankingFormNotDeclared
	// RankingNoPricedCandidate 是合格候选全部缺成本事实而出局：候选在，只是没有一家可比。
	RankingNoPricedCandidate
	RankingCurrenciesDiffer
	// RankingTied 是最低成本并列、选不出唯一一条：交人工裁决（`PAR-NET-16`「不得任选」）。
	RankingTied
)

func (outcome RankingOutcome) String() string {
	switch outcome {
	case RankingSelected:
		return "SELECTED"
	case RankingNoQualifiedCandidate:
		return "NO_QUALIFIED_CANDIDATE"
	case RankingFormNotDeclared:
		return "RANKING_FORM_NOT_DECLARED"
	case RankingNoPricedCandidate:
		return "NO_PRICED_CANDIDATE"
	case RankingCurrenciesDiffer:
		return "CURRENCIES_DIFFER"
	case RankingTied:
		return "TIED"
	default:
		return ""
	}
}

// RouteRanking 是候选评估层次 4 的结果。出局名单是合格候选里因缺成本事实而不参比的那些，
// 留痕要说得出它们为什么没被比。
type RouteRanking struct {
	outcome  RankingOutcome
	selected CandidateID
	tied     []CandidateID
	excluded []CandidateID
}

func (ranking RouteRanking) Outcome() RankingOutcome {
	return ranking.outcome
}

func (ranking RouteRanking) Selected() (CandidateID, bool) {
	return ranking.selected, ranking.outcome == RankingSelected
}

// Tied 交回最低成本并列的那几家，次序照证据给的候选次序，不承载任何先后。
func (ranking RouteRanking) Tied() []CandidateID {
	return append([]CandidateID(nil), ranking.tied...)
}

func (ranking RouteRanking) Excluded() []CandidateID {
	return append([]CandidateID(nil), ranking.excluded...)
}

// RankRouteCandidates 执行候选评估层次 4：按路由策略版本声明的排序形态，在**合格**候选中
// 择优（`AT-NR-001`「按适用策略排序，只形成一个当前有效计划」）。
//
// 最低成本并列时交 RankingTied 而不收尾。此前的多准则比较器全平时按候选标识升序收尾，理由
// 是多维准则序几乎不会全维打平；成本单维恰恰最容易打平，按标识收尾等于让字母序替运营企业
// 挑线路，而 `PAR-NET-16` 明禁无业务依据的选择。幂等不靠收尾守：同一份证据两次判断仍给同
// 一个并列结果。
func RankRouteCandidates(
	form RankingForm,
	candidates []RouteCandidate,
	costs []CandidateCostFact,
) (RouteRanking, error) {
	if form != RankingFormUndeclared && form.String() == "" {
		return RouteRanking{}, ErrInvalidRanking
	}
	indexed := make(map[CandidateID]CandidateCostFact, len(costs))
	for _, cost := range costs {
		if !cost.candidate.valid() || cost.state.String() == "" {
			return RouteRanking{}, ErrInvalidRanking
		}
		if _, duplicated := indexed[cost.candidate]; duplicated {
			return RouteRanking{}, ErrInvalidRanking
		}
		indexed[cost.candidate] = cost
	}
	var qualified []RouteCandidate
	for _, candidate := range candidates {
		if candidate.outcome == CandidateQualified {
			qualified = append(qualified, candidate)
		}
	}
	if len(qualified) == 0 {
		return RouteRanking{outcome: RankingNoQualifiedCandidate}, nil
	}
	if form == RankingFormUndeclared {
		return RouteRanking{outcome: RankingFormNotDeclared}, nil
	}

	var priced []CandidateCostFact
	var excluded []CandidateID
	for _, candidate := range qualified {
		cost, stated := indexed[candidate.id]
		if !stated {
			// 待判断与不可计价都有自己的一格；什么都没说是证据装配漏了，不是其中哪一格。
			return RouteRanking{}, ErrInvalidRanking
		}
		if cost.state != CandidateCostPriced {
			excluded = append(excluded, candidate.id)
			continue
		}
		priced = append(priced, cost)
	}
	if len(priced) == 0 {
		return RouteRanking{outcome: RankingNoPricedCandidate, excluded: excluded}, nil
	}
	for _, cost := range priced[1:] {
		if cost.currency != priced[0].currency {
			return RouteRanking{outcome: RankingCurrenciesDiffer, excluded: excluded}, nil
		}
	}
	lowest := priced[0].amountMinor
	for _, cost := range priced[1:] {
		if cost.amountMinor < lowest {
			lowest = cost.amountMinor
		}
	}
	var atLowest []CandidateID
	for _, cost := range priced {
		if cost.amountMinor == lowest {
			atLowest = append(atLowest, cost.candidate)
		}
	}
	if len(atLowest) > 1 {
		return RouteRanking{outcome: RankingTied, tied: atLowest, excluded: excluded}, nil
	}
	return RouteRanking{outcome: RankingSelected, selected: atLowest[0], excluded: excluded}, nil
}
