package domain

import "errors"

var (
	ErrInvalidRanking       = errors.New("network routing: invalid ranking facts")
	ErrNoQualifiedCandidate = errors.New("network routing: no qualified candidate to select")
)

// RankingCriterion 是策略排序的准则引用（时效、可靠性、成本、容量风险、已承诺偏好……）。
// 刻意是引用不是封闭枚举：准则集合由路由策略版本声明（PAR-NET-14），在这里列一份就成了
// 第二处定义。
type RankingCriterion struct{ requiredValue }

func NewRankingCriterion(value string) (RankingCriterion, error) {
	required, err := newRequiredValue("ranking criterion", value)
	return RankingCriterion{required}, err
}

// CriterionScore 是一个候选在一个准则上的可比取值：越小越优。分值的量纲与折算由策略
// 版本拥有（时效折小时、成本折最小币单位都发生在事实形成处），领域只比较不折算——
// 「应用层不得自行发明固定权重或行业默认阈值」，领域同样不发明。
type CriterionScore struct {
	criterion RankingCriterion
	value     int64
}

func NewCriterionScore(criterion RankingCriterion, value int64) (CriterionScore, error) {
	if !criterion.valid() || value < 0 {
		return CriterionScore{}, ErrInvalidRanking
	}
	return CriterionScore{criterion: criterion, value: value}, nil
}

func (score CriterionScore) Criterion() RankingCriterion {
	return score.criterion
}

func (score CriterionScore) Value() int64 {
	return score.value
}

// CandidateScores 是一个合格候选的全部准则取值。
type CandidateScores struct {
	candidate CandidateID
	scores    map[RankingCriterion]int64
}

func NewCandidateScores(candidate CandidateID, scores []CriterionScore) (CandidateScores, error) {
	if !candidate.valid() || len(scores) == 0 {
		return CandidateScores{}, ErrInvalidRanking
	}
	indexed := make(map[RankingCriterion]int64, len(scores))
	for _, score := range scores {
		if !score.criterion.valid() {
			return CandidateScores{}, ErrInvalidRanking
		}
		if _, duplicated := indexed[score.criterion]; duplicated {
			// 同一准则两个取值互相矛盾，取哪个都是掷硬币。
			return CandidateScores{}, ErrInvalidRanking
		}
		indexed[score.criterion] = score.value
	}
	return CandidateScores{candidate: candidate, scores: indexed}, nil
}

func (scores CandidateScores) Candidate() CandidateID {
	return scores.candidate
}

// SelectRouteCandidate 执行候选评估层次 4：按策略声明的准则优先级序做字典序比较，在
// **合格**候选中选出排序最高者（`AT-NR-001`「按适用策略排序，只形成一个当前有效计划」）。
//
//   - priority 是策略版本声明的准则序（第一准则分高下，平则看第二准则……），序本身是
//     版本化事实输入，领域不排它也不补它；
//   - 全部声明准则仍打平时按候选标识升序收尾——排序必须全序，两个「一样好」的候选靠
//     掷硬币选会让同一份输入两次判断给出两个计划，幂等就死了；
//   - 合格候选缺某个声明准则的取值是事实装配错误：少一个值的比较不是比较；
//   - 没有合格候选交 ErrNoQualifiedCandidate——那是走`无当前有效路由`或未决的信号，
//     不是这里能替策略回答的。
func SelectRouteCandidate(
	candidates []RouteCandidate,
	scores []CandidateScores,
	priority []RankingCriterion,
) (CandidateID, error) {
	if len(priority) == 0 {
		return CandidateID{}, ErrInvalidRanking
	}
	for _, criterion := range priority {
		if !criterion.valid() {
			return CandidateID{}, ErrInvalidRanking
		}
	}
	indexed := make(map[CandidateID]CandidateScores, len(scores))
	for _, entry := range scores {
		if !entry.candidate.valid() {
			return CandidateID{}, ErrInvalidRanking
		}
		if _, duplicated := indexed[entry.candidate]; duplicated {
			return CandidateID{}, ErrInvalidRanking
		}
		indexed[entry.candidate] = entry
	}

	var best CandidateID
	var bestScores CandidateScores
	found := false
	for _, candidate := range candidates {
		if candidate.outcome != CandidateQualified {
			continue
		}
		entry, scored := indexed[candidate.id]
		if !scored {
			return CandidateID{}, ErrInvalidRanking
		}
		for _, criterion := range priority {
			if _, present := entry.scores[criterion]; !present {
				return CandidateID{}, ErrInvalidRanking
			}
		}
		if !found || ranksBefore(entry, bestScores, priority) {
			best, bestScores, found = candidate.id, entry, true
		}
	}
	if !found {
		return CandidateID{}, ErrNoQualifiedCandidate
	}
	return best, nil
}

// ranksBefore 按声明序逐准则比较，全平时按候选标识收尾。
func ranksBefore(left, right CandidateScores, priority []RankingCriterion) bool {
	for _, criterion := range priority {
		leftValue, rightValue := left.scores[criterion], right.scores[criterion]
		if leftValue != rightValue {
			return leftValue < rightValue
		}
	}
	return left.candidate.String() < right.candidate.String()
}
