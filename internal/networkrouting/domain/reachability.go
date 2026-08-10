// Package domain holds the network-routing model: service areas, network
// topology, route candidates and the reachability findings other contexts
// consume. It owns no customer address, product, contract, shipment decision or
// fulfilment resource.
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

// CandidateReason is the stable reason a candidate was eliminated or left
// unknown. It is a reference rather than free text so that outcomes stay
// countable by cause instead of by log string.
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

// RouteCandidate is one possible path considered by a reachability assessment.
// Anything other than a qualified outcome must carry its reason, because an
// elimination without a recorded cause cannot support an unreachable finding.
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

// EvidenceGapScope separates a gap that only clouds particular candidates from
// one that bears on the assessment as a whole. The distinction decides whether a
// proven candidate still stands: a gap elsewhere cannot overturn it, a global
// one can.
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

// EvidenceGap names a missing or indeterminate piece of business evidence. It is
// a business fact, not a technical failure: a dependency that could not be
// called is the application's "no finding formed", never a gap recorded here.
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

// ReachabilityValue is the three-valued domain finding this context owns. There
// is deliberately no fourth value: "no finding formed" is an application
// processing result, and letting it into this set would put a technical failure
// into the same tally as a business judgement.
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

// EvidenceGaps is retained on every finding, not only on insufficient ones. A
// reachable finding keeps the other candidates' gaps because the assessment must
// stay reproducible, and a later reviewer needs to see what was still unknown
// when the proven candidate carried the conclusion.
func (finding ReachabilityFinding) EvidenceGaps() []EvidenceGap {
	return append([]EvidenceGap(nil), finding.gaps...)
}

// ConcludeReachability applies the evidence matrix to an evaluated candidate
// space. It forms only the three-valued domain finding: it never decides
// acceptance, never selects a route, and never reports a technical failure.
//
// An empty candidate space is refused rather than answered. Zero candidates
// cannot show whether nothing was generated because coverage excludes the
// destination — which belongs in an eliminated candidate — or because generation
// itself failed, which is the application's "no finding formed". Answering it
// either way would invent one of those.
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
