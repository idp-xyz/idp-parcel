package domain

// 本文件是「一份输入对多份价卡逐份评价」的批量口（票 `label-channel/13` 缝一）。
//
// 它存在的理由是结构性的，不是省事：`PAR-NET-16` 要求各候选按**自己**方案的体积系数与
// 进位算出自己的计费重，然后才比总价。把这一步交给调用方循环，就等于把这条要求降格成
// 一条纪律——调用方完全可以先算一次计费重再套不同费率，而那个写法在跨候选比价上必错，
// 且它错得悄无声息：每张卡单看都算对了，只有横过来比才看得出计费重被共用了。

// PlanEvaluationTarget 是批量里的一个待评价项：一份价卡版本，以及这次对它评价的标识。
//
// 标识由调用方铸，因为每次评价都要独立可追、可重放；批量口不代铸，也就不会出现「这一批
// 的第三个」这种只在批内成立、出了批就指不回去的引用。
type PlanEvaluationTarget struct {
	id   EvaluationID
	plan PricingPlanVersion
}

func NewPlanEvaluationTarget(id EvaluationID, plan PricingPlanVersion) (PlanEvaluationTarget, error) {
	if !id.valid() || !plan.valid() {
		return PlanEvaluationTarget{}, ErrInvalidPlanEvaluationTarget
	}
	return PlanEvaluationTarget{id: id, plan: plan}, nil
}

func (target PlanEvaluationTarget) ID() EvaluationID { return target.id }

func (target PlanEvaluationTarget) Plan() PricingPlanVersion { return target.plan }

// EvaluatePricingAcrossPlans 用同一份输入逐份评价每一份价卡，按入参次序交回等长的结果。
//
// 签名收成「一份输入 + N 份价卡」而不是「N 份请求」，是为了让「同一批包裹尺寸重量」由
// 结构保证：调用方没有地方可以给某个候选换一份输入，也没有地方可以塞一个自己算好的
// 计费重——计费重只能由 EvaluatePricing 从各自的 plan.weight 里算出来。
//
// 不返回 error，一个候选评不出来只坏它自己那一格。这不是宽容，是必须：批量口的调用方
// 拿它来横向比价，一个不可计价的候选若能中断整批，它就会把其余候选**已经算出来的**价格
// 一并掩掉，而调用方看到的是「这批没有价格」，与「这批都算不出」分不开。逐格结果里的
// EvaluationPending / EvaluationUnratable 各自的含义由 EvaluationStatus 定义，此处不重述。
func EvaluatePricingAcrossPlans(
	input PricingInputSnapshot,
	evidence EvidenceKind,
	targets []PlanEvaluationTarget,
) []PricingEvaluation {
	evaluations := make([]PricingEvaluation, 0, len(targets))
	for _, target := range targets {
		// 直接成形请求而不走 NewEvaluationRequest：那条路对不合法的目标返回 error，
		// 在这里只能被译成中断或被吞掉。交给 EvaluatePricing 自己判，它对不合法请求
		// 已有 INVALID_REQUEST 这一格，于是坏目标也只坏自己那一格。
		evaluations = append(evaluations, EvaluatePricing(EvaluationRequest{
			id:       target.id,
			plan:     target.plan,
			input:    copyInputSnapshot(input),
			evidence: evidence,
		}))
	}
	return evaluations
}
