package domain

import (
	"errors"
	"strings"
)

// 本文件是渠道候选择优的成本单维取值与比较器（票 `label-channel/13`）。
//
// 它另立而不复用 `network-routing` 的 `SelectRouteCandidate`，依据是票 `01` 的裁决：两边的
// 合格判据本来就不同（一个看网络可达性，一个看渠道约束），且面单渠道服务已被标为不走网络
// 可达性判断，没有理由住在那个上下文里。

var (
	ErrInvalidChannelCandidateCost = errors.New("parcel shipment: invalid channel candidate cost")
	// ErrNoQualifiedChannelCandidate 说这一批里没有一个候选带着已确立的成本进来。它与
	// 「都比输了」不是一回事：一个候选都没资格参选时，没有任何东西可供比较。
	ErrNoQualifiedChannelCandidate = errors.New("parcel shipment: no qualified channel candidate")
	// ErrChannelCandidateCostTied 说成本最低这一格上不止一个候选，选不出唯一一条。
	//
	// 票 `01` 把 `PAR-NET-16` 的「不得任选」裁作**禁没有业务依据的选择**，所以这里交冲突，
	// 不学 `SelectRouteCandidate` 按候选标识升序收尾。两者结论不同不是矛盾：路由是多维准则
	// 序、产出可重算的内部计划，而渠道择优首发只按成本单维，打平是常规路径，且它选出来的
	// 是一笔真实的供应商采购。成本相同即两家渠道对运营企业的成本相同，此时选谁是商业决定
	// （承运商关系、账期、赔付条件），字母序答不出这里面任何一条。
	//
	// 裁决人与裁决规则属实例半边（`PAR-NET-16` 待提供列的「并列时的裁决规则和授权」），
	// 机制只把这一格如实交出来，不填默认裁决人。
	ErrChannelCandidateCostTied = errors.New("parcel shipment: channel candidate costs tied")
	// ErrChannelCostCurrencyMismatch 说这一批候选的成本不在同一币种上，因而没有可比性。
	//
	// 折算需要汇率，而汇率属实例半边，机制这里既没有也不该编一个。折不到同一把尺就只能
	// 拒绝：静默比大小会选出一个纯粹由币种小数位决定的赢家。
	ErrChannelCostCurrencyMismatch = errors.New("parcel shipment: channel candidate cost currency mismatch")
)

// ChannelCandidateID 是一个渠道候选的身份。
type ChannelCandidateID struct{ requiredValue }

func NewChannelCandidateID(value string) (ChannelCandidateID, error) {
	required, err := newRequiredValue("channel candidate ID", value)
	return ChannelCandidateID{required}, err
}

// ChannelCostCurrency 是成本取值的计价币种。它随取值一起进类型而不是由调用方在外面记着，
// 因为跨币种比大小是个数值上完全合法、业务上毫无意义的动作：一个日元报价在最小币单位上
// 天然比同额美元报价大两位数。
type ChannelCostCurrency struct{ requiredValue }

func NewChannelCostCurrency(value string) (ChannelCostCurrency, error) {
	required, err := newRequiredValue("channel cost currency", value)
	return ChannelCostCurrency{required}, err
}

// ChannelCostAmount 是一个候选的成本金额，按 `parcel-pricing` 交来的十进制**原样保全**。
//
// 不折成最小币单位整数，理由与 MeasurementValue 保全客户申报数字是同一条（ADR-0048）：
// 折算要知道该币种有几位小数，那是一份本上下文并不拥有、本仓今天也不存在的参照数据，
// 编一张表出来就是把一个未确认参数写死成生产默认。而择优根本不需要它——比较只发生在
// 同一币种内（跨币种已由 ErrChannelCostCurrencyMismatch 拒掉），同币种内十进制数值序
// 本身就够定谁最便宜，标度是多余的中间物。
//
// 允许零：真算出来的零价是一个价格。`PAR-NET-16` 禁的是拿零去**顶替**算不出的候选，
// 那一格由 established 挡，不靠禁止零金额来挡。
type ChannelCostAmount struct {
	raw string
}

func NewChannelCostAmount(raw string) (ChannelCostAmount, error) {
	trimmed := strings.TrimSpace(raw)
	// 形状校验与申报测量共用 decimalShape：负号、指数、多点一律不是金额。负成本没有
	// 业务读法，而它在比较里会赢。
	if !decimalShape(trimmed) {
		return ChannelCostAmount{}, ErrInvalidChannelCandidateCost
	}
	return ChannelCostAmount{raw: trimmed}, nil
}

func (amount ChannelCostAmount) String() string {
	return amount.raw
}

func (amount ChannelCostAmount) valid() bool {
	return amount.raw != ""
}

// Cmp 按十进制**数值**比大小，交回 -1/0/1。不用字符串序：字典序上 "10.00" 排在 "9.50"
// 之前，而金额原样保全意味着同一批里出现不同小数位是常态，不是例外。
func (amount ChannelCostAmount) Cmp(other ChannelCostAmount) int {
	leftWhole, leftFraction, _ := strings.Cut(amount.raw, ".")
	rightWhole, rightFraction, _ := strings.Cut(other.raw, ".")
	if order := compareWholeDigits(leftWhole, rightWhole); order != 0 {
		return order
	}
	return compareFractionDigits(leftFraction, rightFraction)
}

// compareWholeDigits 比两个无符号整数数字串：去掉前导零之后先比位数，再逐位比。
func compareWholeDigits(left, right string) int {
	left, right = strings.TrimLeft(left, "0"), strings.TrimLeft(right, "0")
	if len(left) != len(right) {
		if len(left) < len(right) {
			return -1
		}
		return 1
	}
	return strings.Compare(left, right)
}

// compareFractionDigits 右侧补零到等长再比。小数部分是定点的，"5" 是五分之一而 "05" 是
// 二十分之一，不补零就会把它们读成同一个数。
func compareFractionDigits(left, right string) int {
	for len(left) < len(right) {
		left += "0"
	}
	for len(right) < len(left) {
		right += "0"
	}
	return strings.Compare(left, right)
}

// ChannelCostUnavailability 说这个候选的成本为什么没能确立。
//
// 它单独成格而不是让成本取一个特殊数值，是因为「算不出」在数值域里没有一个安全的表示：
// 零在成本单维上是最优，负数在比较里同样能赢。见 ChannelCandidateCost 的注释。
//
// 四格与 `parcel-pricing` 的四种非完成结果一一对应，不合并。合并的诱惑很大——择优对这四
// 格的即时处置都是「出局」——但提供方 CONTEXT 明写「四者不得互相替代」，且它们的续办完全
// 不同：`待判断`去补事实，`不可计价`换渠道，`冲突`等治理裁决，`未形成`重试。票 `14` 的
// 落选留痕要答的正是这个，压成一格就答不出。
type ChannelCostUnavailability uint8

const (
	ChannelCostUnavailabilityInvalid ChannelCostUnavailability = iota
	// ChannelCostPendingEvidence 对应提供方的`待判断`：补齐事实就能得出价格，
	// 续办是去补那份事实，不是换一个候选。
	ChannelCostPendingEvidence
	// ChannelCostRatecardExclusion 对应提供方的`不可计价`：价卡明确排除了这一票，
	// 是确定结论。补事实与重试都不会改变它，续办是换渠道。
	ChannelCostRatecardExclusion
	// ChannelCostConflict 对应提供方的`冲突`：存在互斥的规则、区间或版本候选，
	// 要治理责任方裁决，不是本上下文能推进的。
	ChannelCostConflict
	// ChannelCostNotFormed 对应提供方的`未形成`：请求不合法或计算没能完成。它是技术
	// 或结构失败，不是业务结论，续办是重试。
	ChannelCostNotFormed
)

func (grade ChannelCostUnavailability) String() string {
	switch grade {
	case ChannelCostPendingEvidence:
		return "PENDING_EVIDENCE"
	case ChannelCostRatecardExclusion:
		return "RATECARD_EXCLUSION"
	case ChannelCostConflict:
		return "CONFLICT"
	case ChannelCostNotFormed:
		return "NOT_FORMED"
	default:
		return ""
	}
}

func (grade ChannelCostUnavailability) valid() bool {
	return grade.String() != ""
}

// ChannelCandidateCost 是一个渠道候选在成本单维上的取值，两格：**已确立**与**未确立**。
//
// **零值读作未确立，这一条是本类型存在的全部理由。** `PAR-NET-16` 明禁「以零金额顶替」不可
// 计价的候选，而把成本做成一个裸金额时，那条禁令就只能靠每个调用方记住——漏一处，不可
// 计价的候选就以成本 0 每次都赢，赢下来的是一笔真实的供应商采购。做进类型之后，一个没经过
// PricedChannelCandidate 的值根本交不出成本取值。
type ChannelCandidateCost struct {
	candidate   ChannelCandidateID
	established bool
	amount      ChannelCostAmount
	currency    ChannelCostCurrency
	unavailable ChannelCostUnavailability
	// evaluation 是这份取值译自哪一份 `BUY` 评价，可缺席（没登记价卡的候选没经过评价）。它不参与
	// 比较，只随取值带给决定记录（票 `14`）——留痕要能指回评价，而金额本身指不回去。
	evaluation ChannelCostEvaluationReference
}

// PricedChannelCandidate 造一个成本已确立的候选。
func PricedChannelCandidate(
	candidate ChannelCandidateID,
	amount ChannelCostAmount,
	currency ChannelCostCurrency,
) (ChannelCandidateCost, error) {
	if !candidate.valid() || !amount.valid() || !currency.valid() {
		return ChannelCandidateCost{}, ErrInvalidChannelCandidateCost
	}
	return ChannelCandidateCost{
		candidate:   candidate,
		established: true,
		amount:      amount,
		currency:    currency,
	}, nil
}

// UnpriceableChannelCandidate 造一个成本未确立的候选。它要求指名是哪一格：把「算不出」
// 收成一个无因由的布尔，留痕（票 `14`）就答不出这个候选当初为什么出局。
func UnpriceableChannelCandidate(
	candidate ChannelCandidateID,
	grade ChannelCostUnavailability,
) (ChannelCandidateCost, error) {
	if !candidate.valid() || !grade.valid() {
		return ChannelCandidateCost{}, ErrInvalidChannelCandidateCost
	}
	return ChannelCandidateCost{candidate: candidate, unavailable: grade}, nil
}

func (cost ChannelCandidateCost) Candidate() ChannelCandidateID {
	return cost.candidate
}

// Amount 交出成本取值，第二个返回值为 false 即「未确立」。调用方读不到取值时不得自行
// 补零——那正是本类型要挡的那一步。
func (cost ChannelCandidateCost) Amount() (ChannelCostAmount, bool) {
	if !cost.established {
		return ChannelCostAmount{}, false
	}
	return cost.amount, true
}

// Currency 交出计价币种，第二个返回值与 Amount 同义：没有取值就没有币种。
func (cost ChannelCandidateCost) Currency() (ChannelCostCurrency, bool) {
	if !cost.established {
		return ChannelCostCurrency{}, false
	}
	return cost.currency, true
}

// Unavailability 交出出局格，第二个返回值为 false 即「这个候选的成本是确立的」。
func (cost ChannelCandidateCost) Unavailability() (ChannelCostUnavailability, bool) {
	if cost.established {
		return ChannelCostUnavailabilityInvalid, false
	}
	return cost.unavailable, true
}

// WithEvaluation 给取值带上它译自的那份 `BUY` 评价引用。已确立与未确立两格都可以带——出局
// 的候选多半也经过了评价（价卡排除、冲突、未形成都是评价的结果），留痕同样要指得回去。
func (cost ChannelCandidateCost) WithEvaluation(
	evaluation ChannelCostEvaluationReference,
) (ChannelCandidateCost, error) {
	if !cost.candidate.valid() || !evaluation.valid() {
		return ChannelCandidateCost{}, ErrInvalidChannelCandidateCost
	}
	cost.evaluation = evaluation
	return cost, nil
}

// Evaluation 交出所用评价引用，第二个返回值为 false 即缺席。
func (cost ChannelCandidateCost) Evaluation() (ChannelCostEvaluationReference, bool) {
	return cost.evaluation, cost.evaluation.valid()
}

// SelectChannelCandidateByCost 按成本单维择优。`PAR-NET-16` 首发只按成本单维，其余维度
// 保持未配置。四种出口：选出唯一一条、无人有资格参选（ErrNoQualifiedChannelCandidate）、
// 最低价并列（ErrChannelCandidateCostTied）、币种不齐（ErrChannelCostCurrencyMismatch）。
func SelectChannelCandidateByCost(costs []ChannelCandidateCost) (ChannelCandidateID, error) {
	ranking, err := rankChannelCandidatesByCost(costs)
	if err != nil {
		return ChannelCandidateID{}, err
	}
	if !ranking.qualified {
		return ChannelCandidateID{}, ErrNoQualifiedChannelCandidate
	}
	if ranking.tied {
		return ChannelCandidateID{}, ErrChannelCandidateCostTied
	}
	return ranking.best.Candidate(), nil
}

// channelCostRanking 是一次排序的全部结论：最优者、有没有人参选、最低价上是否并列。它是
// 择优（SelectChannelCandidateByCost）与决定记录（FormChannelSelectionDecision）共用的那一份
// 口径——两处若各排一遍，迟早在某个边界上各说各话，而那种分叉在两边各自的测试下全绿。
type channelCostRanking struct {
	best      ChannelCandidateCost
	qualified bool
	tied      bool
}

// rankChannelCandidatesByCost 在同一币种内找成本最低者，并记下最低价那一格上是否并列。
func rankChannelCandidatesByCost(costs []ChannelCandidateCost) (channelCostRanking, error) {
	var (
		ranking   channelCostRanking
		scale     ChannelCostCurrency
		scaleSeen bool
	)
	for _, cost := range costs {
		value, established := cost.Amount()
		if !established {
			continue
		}
		// 币种取第一个有资格参选者的，而不是取当前最优者的：后者会随比较过程漂移，
		// 让「这一批齐不齐」变成一个取决于入参顺序的问题。出局候选不带成本，也就不带
		// 币种，不该参与这道校验。
		switch {
		case !scaleSeen:
			scale, scaleSeen = cost.currency, true
		case cost.currency != scale:
			return channelCostRanking{}, ErrChannelCostCurrencyMismatch
		}
		bestValue, chosen := ranking.best.Amount()
		switch {
		case !chosen || value.Cmp(bestValue) < 0:
			// 更便宜的候选出现即清掉并列：并列只在**最低价那一格**上才是冲突，
			// 输家之间打平与择优无关。
			ranking.best, ranking.tied, ranking.qualified = cost, false, true
		case value.Cmp(bestValue) == 0:
			ranking.tied = true
		}
	}
	return ranking, nil
}

// outcomeOf 把一个**已确立**成本的候选归到四格里的一格：等于最低价即选中（或并列时的 TIED），
// 其余是落选。出局那一格不经这里——它由取值自己的未确立格决定，与排序无关。
func (ranking channelCostRanking) outcomeOf(cost ChannelCandidateCost) ChannelCandidateOutcome {
	value, established := cost.Amount()
	bestValue, chosen := ranking.best.Amount()
	if !established || !chosen || value.Cmp(bestValue) != 0 {
		return ChannelCandidateNotSelected
	}
	if ranking.tied {
		return ChannelCandidateTied
	}
	return ChannelCandidateSelected
}

// conclusion 把排序折成整条决定的结论，与 SelectChannelCandidateByCost 的三种非错误出口同序。
func (ranking channelCostRanking) conclusion() ChannelSelectionConclusion {
	switch {
	case !ranking.qualified:
		return ChannelSelectionConcludedNoneQualified
	case ranking.tied:
		return ChannelSelectionConcludedTied
	default:
		return ChannelSelectionConcludedSelected
	}
}
