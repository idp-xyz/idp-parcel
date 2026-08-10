package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// ClosureResolutionKey asks for several required bases at once. A contract does
// not stand alone: it references an acceptance rule package, a settlement policy
// and the other objects an acceptance decision needs, and the use case treats an
// incomplete reference closure as a failure of the whole rather than a partial
// success.
type ClosureResolutionKey struct {
	TenantID             TenantID
	CustomerAccountID    CustomerAccountID
	LegalEntityCandidate LegalEntityReference
	Scope                CommercialScopeReference
	Purpose              ResolutionPurpose
	PriceDirection       PriceDirection
	Anchor               SelectionAnchor
	RequiredBases        []CommercialObjectKind
}

func (key ClosureResolutionKey) minimumIdentityEstablished() bool {
	if !key.TenantID.valid() ||
		!key.CustomerAccountID.valid() ||
		!key.LegalEntityCandidate.valid() ||
		!key.Scope.valid() ||
		!key.Purpose.valid() ||
		len(key.RequiredBases) == 0 {
		return false
	}
	if key.Purpose == PricingPurpose {
		if !key.PriceDirection.valid() {
			return false
		}
	} else if key.PriceDirection.valid() {
		return false
	}

	seen := make(map[CommercialObjectKind]struct{}, len(key.RequiredBases))
	for _, kind := range key.RequiredBases {
		if !kind.valid() {
			return false
		}
		if _, exists := seen[kind]; exists {
			return false
		}
		seen[kind] = struct{}{}
	}
	return true
}

func (key ClosureResolutionKey) singleBasisKey(kind CommercialObjectKind) ResolutionKey {
	return ResolutionKey{
		TenantID:             key.TenantID,
		CustomerAccountID:    key.CustomerAccountID,
		LegalEntityCandidate: key.LegalEntityCandidate,
		Scope:                key.Scope,
		RequiredBasis:        kind,
		Purpose:              key.Purpose,
		PriceDirection:       key.PriceDirection,
		Anchor:               key.Anchor,
	}
}

// AdoptedBasis pairs a required basis with the version adopted for it. The
// closure holds these pairs rather than typed fields so that a basis backed by
// something other than a commercial version — a party relationship, for
// instance — can join without reshaping the closure.
type AdoptedBasis struct {
	kind    CommercialObjectKind
	version CommercialVersion
}

func (adopted AdoptedBasis) Kind() CommercialObjectKind {
	return adopted.kind
}

func (adopted AdoptedBasis) Version() CommercialVersion {
	return adopted.version
}

// CommercialClosure is the all-or-nothing result of resolving a reference
// closure. On anything but a unique outcome it adopts nothing at all: handing
// back the members that did resolve would invite a caller to proceed on a basis
// the use case says is not established.
type CommercialClosure struct {
	outcome      ResolutionOutcome
	resolutionID ResolutionID
	anchor       SelectionAnchor
	viewRevision AuthorityViewRevision
	adopted      []AdoptedBasis
	unresolved   []CommercialObjectKind
	conflicting  []CommercialObjectKind
	continuation ContinuationReference
	reason       ResolutionReason
}

func (closure CommercialClosure) Outcome() ResolutionOutcome {
	return closure.outcome
}

func (closure CommercialClosure) ResolutionID() ResolutionID {
	return closure.resolutionID
}

func (closure CommercialClosure) Anchor() SelectionAnchor {
	return closure.anchor
}

func (closure CommercialClosure) ViewRevision() (AuthorityViewRevision, bool) {
	if !closure.viewRevision.valid() {
		return AuthorityViewRevision{}, false
	}
	return closure.viewRevision, true
}

func (closure CommercialClosure) Adopted() []AdoptedBasis {
	return append([]AdoptedBasis(nil), closure.adopted...)
}

func (closure CommercialClosure) AdoptedFor(kind CommercialObjectKind) (AdoptedBasis, bool) {
	for _, adopted := range closure.adopted {
		if adopted.kind == kind {
			return adopted, true
		}
	}
	return AdoptedBasis{}, false
}

// UnresolvedBases and ConflictingBases are both reported even when only one of
// them decides the outcome, because the commercial owner fixing a conflict also
// needs to know what else is still missing.
func (closure CommercialClosure) UnresolvedBases() []CommercialObjectKind {
	return append([]CommercialObjectKind(nil), closure.unresolved...)
}

func (closure CommercialClosure) ConflictingBases() []CommercialObjectKind {
	return append([]CommercialObjectKind(nil), closure.conflicting...)
}

func (closure CommercialClosure) ContinuationReference() ContinuationReference {
	return closure.continuation
}

func (closure CommercialClosure) Reason() ResolutionReason {
	return closure.reason
}

// ResolveCommercialClosure resolves every required basis under one anchor and
// one authority view. It succeeds only when all of them resolve uniquely.
//
// A conflict outranks a missing basis when both occur. They call for different
// action: a conflict is an overlap the commercial owner must correct, while a
// missing basis only says the scope holds no such object. Reporting the softer
// answer would make the one that needs fixing look like it needs nothing.
func ResolveCommercialClosure(registry *CommercialRegistry, key ClosureResolutionKey) CommercialClosure {
	if !key.minimumIdentityEstablished() {
		return CommercialClosure{outcome: InputNotAccepted}
	}
	if !key.Anchor.valid() {
		return CommercialClosure{outcome: ResolutionPending}
	}
	if registry == nil {
		return CommercialClosure{outcome: ResolutionPending, anchor: key.Anchor}
	}

	closure := CommercialClosure{
		anchor:       key.Anchor,
		viewRevision: registry.ViewRevision(key.Scope),
	}
	adopted := make([]AdoptedBasis, 0, len(key.RequiredBases))
	for _, kind := range key.RequiredBases {
		result := ResolveCommercialBasis(registry, key.singleBasisKey(kind))
		switch result.Outcome() {
		case UniquelyResolved:
			version, _ := result.AdoptedVersion()
			adopted = append(adopted, AdoptedBasis{kind: kind, version: version})
		case ApplicabilityConflict:
			closure.conflicting = append(closure.conflicting, kind)
		case NoApplicableBasis:
			closure.unresolved = append(closure.unresolved, kind)
		default:
			return CommercialClosure{
				outcome:      ResolutionPending,
				anchor:       key.Anchor,
				viewRevision: closure.viewRevision,
			}
		}
	}

	switch {
	case len(closure.conflicting) > 0:
		closure.outcome = ApplicabilityConflict
	case len(closure.unresolved) > 0:
		closure.outcome = NoApplicableBasis
	default:
		closure.outcome = UniquelyResolved
		closure.adopted = adopted
		closure.resolutionID = closureIdentity(key, closure.viewRevision, adopted)
	}
	return closure
}

// closureIdentity covers the key, the authority view and every adopted version,
// so the same request against the same view answers with the same identity and
// any change to any member produces a different one.
func closureIdentity(key ClosureResolutionKey, view AuthorityViewRevision, adopted []AdoptedBasis) ResolutionID {
	parts := make([]string, 0, len(adopted))
	for _, basis := range adopted {
		parts = append(parts, strings.Join([]string{
			basis.kind.String(),
			basis.version.objectID.String(),
			basis.version.version.String(),
			basis.version.contentDigest.String(),
		}, "\x1f"))
	}
	sort.Strings(parts)

	digest := sha256.Sum256([]byte(strings.Join(append([]string{
		key.singleBasisKey(CommercialObjectKindInvalid).fingerprint(),
		view.String(),
	}, parts...), "\x00")))
	return ResolutionID{requiredValue{value: "CLO-" + hex.EncodeToString(digest[:8])}}
}
