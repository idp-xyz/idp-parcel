package domain

import (
	"errors"
	"time"
)

// ErrInvalidLoadingConcurrency 说明裁决输入不成立：计划版本空、节点空、计划节点链空，
// 或因果陈述停在零值。零值不是「因果不明」——不明必须显式声明。
var ErrInvalidLoadingConcurrency = errors.New("network routing: invalid loading concurrency")

// CausalityStatement 是这次装载或交接与这次改路之间的因果关系，由拥有该事实的上下文给出。
// 可比表示权威业务时间可以用来分先后。不明表示来源已经说明推不出因果。
type CausalityStatement uint8

const (
	CausalityStatementInvalid CausalityStatement = iota
	CausalityComparable
	CausalityUnknown
)

// LoadingFact 是一条装载或交接事实的引用面。发生时间、节点和所依计划版本都来自
// transport-fulfillment 或 node-operations。本上下文不拥有这条事实，也不收消息到达时间。
type LoadingFact struct {
	OccurredAt time.Time
	Node       string
	Plan       RoutePlanVersionID
	Causality  CausalityStatement
}

// RerouteEffectBoundary 是一次改路的生效边界：生效时刻，以及被离开的那一版计划节点链。
type RerouteEffectBoundary struct {
	EffectiveAt time.Time
	Plan        RoutePlanVersionID
	PlanNodes   []string
}

// LoadingConcurrencyOutcome 是并发裁决的封闭四格。同一时刻与因果不明都不并进前两格。
type LoadingConcurrencyOutcome uint8

const (
	LoadingConcurrencyInvalid LoadingConcurrencyOutcome = iota
	// LoadOccurredFirst 是实际装载先发生：改路从下一个可控节点生效。
	LoadOccurredFirst
	// RerouteEffectiveFirst 是改路先有效：其后的装载保留为真实事实，并形成路由偏离。
	// 偏离之后的处置不在本上下文。
	RerouteEffectiveFirst
	// SameInstant 是两个权威业务时间相等。时间推不出先后。
	SameInstant
	// CausalityUnclear 是来源标明因果不明、时间缺一，或装载所依计划不是这次改路离开的计划。
	CausalityUnclear
)

func (outcome LoadingConcurrencyOutcome) String() string {
	switch outcome {
	case LoadOccurredFirst:
		return "LOAD_OCCURRED_FIRST"
	case RerouteEffectiveFirst:
		return "REROUTE_EFFECTIVE_FIRST"
	case SameInstant:
		return "SAME_INSTANT"
	case CausalityUnclear:
		return "CAUSALITY_UNCLEAR"
	default:
		return ""
	}
}

// LoadingConcurrencyJudgment 是一次裁决。后两格不带下一节点，也不形成偏离。
type LoadingConcurrencyJudgment struct {
	outcome              LoadingConcurrencyOutcome
	nextControllableNode string
	hasNext              bool
	deviation            bool
}

func (judgment LoadingConcurrencyJudgment) Outcome() LoadingConcurrencyOutcome {
	return judgment.outcome
}

// NextControllableNode 只在装载先发生、且计划上还有更后的节点时交出该节点。
func (judgment LoadingConcurrencyJudgment) NextControllableNode() (string, bool) {
	return judgment.nextControllableNode, judgment.hasNext
}

// RouteDeviationIndicated 只在改路先有效时为真。它标明要形成偏离，不打开异常处置。
func (judgment LoadingConcurrencyJudgment) RouteDeviationIndicated() bool {
	return judgment.deviation
}

// ArbitrateLoadingConcurrency 按权威业务发生时间、改路生效边界和明确因果关系裁决。
// 没有消息到达时间这一格。同一时刻与因果不明都停住，不选装载先，也不选改路先。
func ArbitrateLoadingConcurrency(
	boundary RerouteEffectBoundary,
	load LoadingFact,
) (LoadingConcurrencyJudgment, error) {
	if !boundary.Plan.valid() || !load.Plan.valid() || load.Node == "" || len(boundary.PlanNodes) == 0 {
		return LoadingConcurrencyJudgment{}, ErrInvalidLoadingConcurrency
	}
	if load.Causality != CausalityComparable && load.Causality != CausalityUnknown {
		return LoadingConcurrencyJudgment{}, ErrInvalidLoadingConcurrency
	}
	if load.Causality == CausalityUnknown ||
		boundary.EffectiveAt.IsZero() || load.OccurredAt.IsZero() ||
		load.Plan != boundary.Plan {
		return LoadingConcurrencyJudgment{outcome: CausalityUnclear}, nil
	}
	if load.OccurredAt.Equal(boundary.EffectiveAt) {
		return LoadingConcurrencyJudgment{outcome: SameInstant}, nil
	}
	if load.OccurredAt.Before(boundary.EffectiveAt) {
		next, hasNext, err := nextControllableNode(boundary.PlanNodes, load.Node)
		if err != nil {
			return LoadingConcurrencyJudgment{}, err
		}
		return LoadingConcurrencyJudgment{
			outcome:              LoadOccurredFirst,
			nextControllableNode: next,
			hasNext:              hasNext,
		}, nil
	}
	return LoadingConcurrencyJudgment{outcome: RerouteEffectiveFirst, deviation: true}, nil
}

func nextControllableNode(planNodes []string, loadNode string) (string, bool, error) {
	index := -1
	for i, node := range planNodes {
		if node == loadNode {
			index = i
			break
		}
	}
	if index < 0 {
		return "", false, ErrInvalidLoadingConcurrency
	}
	if index+1 >= len(planNodes) {
		return "", false, nil
	}
	return planNodes[index+1], true, nil
}
