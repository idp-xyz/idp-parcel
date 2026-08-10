// Package domain 承载网络与路由的领域模型：服务区域、网络拓扑、路由候选，以及供其他
// 上下文消费的可达性判断。它不拥有客户地址、服务产品、客户合同、委托决定或履约资源。
package domain

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrBlankValue                   = errors.New("network routing: blank value")
	ErrInvalidRouteCandidate        = errors.New("network routing: invalid route candidate")
	ErrInvalidEvidenceGap           = errors.New("network routing: invalid evidence gap")
	ErrCandidateSpaceNotEstablished = errors.New("network routing: candidate space is not established")
	ErrDuplicateRouteCandidate      = errors.New("network routing: duplicate route candidate")
)

type requiredValue struct {
	value string
}

func newRequiredValue(name, value string) (requiredValue, error) {
	if strings.TrimSpace(value) == "" {
		return requiredValue{}, fmt.Errorf("%w: %s", ErrBlankValue, name)
	}
	return requiredValue{value: value}, nil
}

func (value requiredValue) String() string {
	return value.value
}

func (value requiredValue) valid() bool {
	return strings.TrimSpace(value.value) != ""
}

type CandidateID struct{ requiredValue }

func NewCandidateID(value string) (CandidateID, error) {
	required, err := newRequiredValue("candidate ID", value)
	return CandidateID{required}, err
}

// CandidateReason 是候选被淘汰或留作证据未知的稳定原因。它是引用而非自由文本，这样
// 结果可以按原因维度统计，而不是退化成检索日志字符串。
type CandidateReason struct{ requiredValue }

func NewCandidateReason(value string) (CandidateReason, error) {
	required, err := newRequiredValue("candidate reason", value)
	return CandidateReason{required}, err
}

type EvidenceGapReference struct{ requiredValue }

func NewEvidenceGapReference(value string) (EvidenceGapReference, error) {
	required, err := newRequiredValue("evidence gap reference", value)
	return EvidenceGapReference{required}, err
}

type CandidateOutcome uint8

const (
	CandidateOutcomeInvalid CandidateOutcome = iota
	CandidateQualified
	CandidateEliminated
	CandidateEvidenceUnknown
)

func (outcome CandidateOutcome) valid() bool {
	return outcome >= CandidateQualified && outcome <= CandidateEvidenceUnknown
}

func (outcome CandidateOutcome) String() string {
	switch outcome {
	case CandidateQualified:
		return "QUALIFIED"
	case CandidateEliminated:
		return "ELIMINATED"
	case CandidateEvidenceUnknown:
		return "EVIDENCE_UNKNOWN"
	default:
		return ""
	}
}

// RouteCandidate 是一次可达性评估中被考虑的一条可能路径。除合格外的结果都必须携带
// 原因：没有记录淘汰依据的淘汰，支撑不起一个`不可达`判断。
type RouteCandidate struct {
	id      CandidateID
	outcome CandidateOutcome
	reason  CandidateReason
}

func NewRouteCandidate(id CandidateID, outcome CandidateOutcome, reason CandidateReason) (RouteCandidate, error) {
	if !id.valid() || !outcome.valid() {
		return RouteCandidate{}, ErrInvalidRouteCandidate
	}
	if outcome != CandidateQualified && !reason.valid() {
		return RouteCandidate{}, ErrInvalidRouteCandidate
	}
	return RouteCandidate{id: id, outcome: outcome, reason: reason}, nil
}

func (candidate RouteCandidate) ID() CandidateID {
	return candidate.id
}

func (candidate RouteCandidate) Outcome() CandidateOutcome {
	return candidate.outcome
}

func (candidate RouteCandidate) Reason() CandidateReason {
	return candidate.reason
}

// EvidenceGapScope 区分只影响特定候选的缺口与影响整次评估的缺口。这个区分决定一条
// 已证明合格的候选是否仍然成立：别处的缺口推翻不了它，全局缺口可以。
type EvidenceGapScope uint8

const (
	EvidenceGapScopeInvalid EvidenceGapScope = iota
	CandidateScopedGap
	GlobalGap
)

func (scope EvidenceGapScope) valid() bool {
	return scope >= CandidateScopedGap && scope <= GlobalGap
}

func (scope EvidenceGapScope) String() string {
	switch scope {
	case CandidateScopedGap:
		return "CANDIDATE_SCOPED"
	case GlobalGap:
		return "GLOBAL"
	default:
		return ""
	}
}

// EvidenceGap 指名一处缺失或无法确定的业务证据。它是业务事实而非技术故障：调不通的
// 依赖属于应用层的`未形成判断`，绝不记成这里的一个缺口。
type EvidenceGap struct {
	reference EvidenceGapReference
	scope     EvidenceGapScope
	affected  []CandidateID
}

func NewEvidenceGap(reference EvidenceGapReference, scope EvidenceGapScope, affected []CandidateID) (EvidenceGap, error) {
	if !reference.valid() || !scope.valid() {
		return EvidenceGap{}, ErrInvalidEvidenceGap
	}
	if scope == CandidateScopedGap && len(affected) == 0 {
		return EvidenceGap{}, ErrInvalidEvidenceGap
	}
	for _, id := range affected {
		if !id.valid() {
			return EvidenceGap{}, ErrInvalidEvidenceGap
		}
	}
	return EvidenceGap{
		reference: reference,
		scope:     scope,
		affected:  append([]CandidateID(nil), affected...),
	}, nil
}

func (gap EvidenceGap) Reference() EvidenceGapReference {
	return gap.reference
}

func (gap EvidenceGap) Scope() EvidenceGapScope {
	return gap.scope
}

func (gap EvidenceGap) AffectedCandidates() []CandidateID {
	return append([]CandidateID(nil), gap.affected...)
}

// ReachabilityValue 是本上下文拥有的三值领域判断。刻意没有第四个取值：`未形成判断`
// 是应用处理结果，放进这个集合等于把技术故障和业务判断计入同一套统计。
type ReachabilityValue uint8

const (
	ReachabilityValueInvalid ReachabilityValue = iota
	Reachable
	Unreachable
	InsufficientEvidence
)

func (value ReachabilityValue) String() string {
	switch value {
	case Reachable:
		return "REACHABLE"
	case Unreachable:
		return "UNREACHABLE"
	case InsufficientEvidence:
		return "INSUFFICIENT_EVIDENCE"
	default:
		return ""
	}
}

type ReachabilityFinding struct {
	value      ReachabilityValue
	candidates []RouteCandidate
	gaps       []EvidenceGap
}

func (finding ReachabilityFinding) Value() ReachabilityValue {
	return finding.value
}

func (finding ReachabilityFinding) Candidates() []RouteCandidate {
	return append([]RouteCandidate(nil), finding.candidates...)
}

func (finding ReachabilityFinding) QualifiedCandidates() []RouteCandidate {
	return finding.candidatesWith(CandidateQualified)
}

func (finding ReachabilityFinding) EliminatedCandidates() []RouteCandidate {
	return finding.candidatesWith(CandidateEliminated)
}

func (finding ReachabilityFinding) UnknownCandidates() []RouteCandidate {
	return finding.candidatesWith(CandidateEvidenceUnknown)
}

func (finding ReachabilityFinding) candidatesWith(outcome CandidateOutcome) []RouteCandidate {
	matches := make([]RouteCandidate, 0, len(finding.candidates))
	for _, candidate := range finding.candidates {
		if candidate.outcome == outcome {
			matches = append(matches, candidate)
		}
	}
	return matches
}

// EvidenceGaps 在每种判断上都保留，不只在`资料不足`时。`可达`判断同样保留其他候选的
// 缺口，因为判断必须可复算：复核者需要看到结论形成时还有什么是未知的。
func (finding ReachabilityFinding) EvidenceGaps() []EvidenceGap {
	return append([]EvidenceGap(nil), finding.gaps...)
}

// ConcludeReachability 把证据判定矩阵应用到一个已评估的候选空间。它只形成三值领域
// 判断：不决定接受、不选择路线、也不报告技术故障。
//
// 空的候选空间被拒绝而不是给出结论。零个候选分不清是覆盖范围排除了目的地——那本该记成
// 一个带淘汰依据的候选——还是候选生成本身失败，而后者属应用层的`未形成判断`。给出任何
// 一种结论都是在凭空发明其中之一。
func ConcludeReachability(candidates []RouteCandidate, gaps []EvidenceGap) (ReachabilityFinding, error) {
	if len(candidates) == 0 {
		return ReachabilityFinding{}, ErrCandidateSpaceNotEstablished
	}

	seen := make(map[CandidateID]struct{}, len(candidates))
	qualified, unknown := 0, 0
	for _, candidate := range candidates {
		if !candidate.id.valid() || !candidate.outcome.valid() {
			return ReachabilityFinding{}, ErrInvalidRouteCandidate
		}
		if _, exists := seen[candidate.id]; exists {
			return ReachabilityFinding{}, ErrDuplicateRouteCandidate
		}
		seen[candidate.id] = struct{}{}

		switch candidate.outcome {
		case CandidateQualified:
			qualified++
		case CandidateEvidenceUnknown:
			unknown++
		}
	}

	globalGap := false
	for _, gap := range gaps {
		if !gap.reference.valid() || !gap.scope.valid() {
			return ReachabilityFinding{}, ErrInvalidEvidenceGap
		}
		if gap.scope == GlobalGap {
			globalGap = true
		}
	}

	finding := ReachabilityFinding{
		candidates: append([]RouteCandidate(nil), candidates...),
		gaps:       append([]EvidenceGap(nil), gaps...),
	}
	switch {
	case qualified > 0 && !globalGap:
		finding.value = Reachable
	case unknown == 0 && qualified == 0 && !globalGap:
		finding.value = Unreachable
	default:
		finding.value = InsufficientEvidence
	}
	return finding, nil
}
