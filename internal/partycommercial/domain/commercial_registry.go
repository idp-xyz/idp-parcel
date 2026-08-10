package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
)

var (
	ErrCommercialVersionNotPublished = errors.New("party commercial: commercial version is not published")
	ErrCommercialVersionConflict     = errors.New("party commercial: commercial version conflicts with the registered content")
)

type RegistrationOutcome uint8

const (
	RegistrationOutcomeInvalid RegistrationOutcome = iota
	RegistrationCreated
	RegistrationReplay
	RegistrationConflict
)

func (outcome RegistrationOutcome) String() string {
	switch outcome {
	case RegistrationCreated:
		return "CREATED"
	case RegistrationReplay:
		return "REPLAY"
	case RegistrationConflict:
		return "CONFLICT"
	default:
		return ""
	}
}

// commercialVersionKey is what makes two registrations the same object version.
// Kind is part of it because a service product and a customer contract may
// legitimately carry the same identifier without being the same object.
type commercialVersionKey struct {
	kind     CommercialObjectKind
	objectID CommercialObjectID
	version  CommercialVersionLabel
}

// CommercialRegistry is the set of controlled releases resolution selects from.
// It only ever adds: a new version joins its predecessors rather than replacing
// them, and a registered version's content is never rewritten.
type CommercialRegistry struct {
	versions map[commercialVersionKey]CommercialVersion
}

func NewCommercialRegistry() *CommercialRegistry {
	return &CommercialRegistry{versions: make(map[commercialVersionKey]CommercialVersion)}
}

// Register admits a released version. Re-registering identical content is a
// replay; the same version label carrying different content is a conflict for
// the commercial owner to resolve, never a silent overwrite.
func (registry *CommercialRegistry) Register(version CommercialVersion) (RegistrationOutcome, error) {
	if version.status == CommercialVersionStatusInvalid || version.status == CommercialVersionDraft {
		return RegistrationOutcomeInvalid, ErrCommercialVersionNotPublished
	}

	key := commercialVersionKey{kind: version.kind, objectID: version.objectID, version: version.version}
	existing, found := registry.versions[key]
	if !found {
		registry.versions[key] = version
		return RegistrationCreated, nil
	}
	if sameReleasedContent(existing, version) {
		return RegistrationReplay, nil
	}
	return RegistrationConflict, ErrCommercialVersionConflict
}

// sameReleasedContent compares what a release fixes. Lifecycle position is
// excluded on purpose: a version that has since taken effect or been retired is
// still the same release, so re-registering it must not read as a conflict.
func sameReleasedContent(left, right CommercialVersion) bool {
	leftEnd, leftBounded := left.effective.EndsAt()
	rightEnd, rightBounded := right.effective.EndsAt()
	return left.contentDigest == right.contentDigest &&
		left.scope == right.scope &&
		left.effective.StartsAt().Equal(right.effective.StartsAt()) &&
		leftBounded == rightBounded &&
		(!leftBounded || leftEnd.Equal(rightEnd)) &&
		left.approval.reference == right.approval.reference &&
		left.approval.source == right.approval.source &&
		left.approval.approvedAt.Equal(right.approval.approvedAt)
}

func (registry *CommercialRegistry) Lookup(
	kind CommercialObjectKind,
	objectID CommercialObjectID,
	version CommercialVersionLabel,
) (CommercialVersion, bool) {
	found, exists := registry.versions[commercialVersionKey{kind: kind, objectID: objectID, version: version}]
	return found, exists
}

func (registry *CommercialRegistry) Count() int {
	return len(registry.versions)
}

// ViewRevision is the scope-level authority view revision: a monotonic-by-content
// reference proving whether the commercial view a scope was resolved under is
// still the same one. It is derived from every version in the scope rather than
// bumped by hand, so it cannot drift from what the registry actually holds.
//
// It is deliberately scope-level and not per-object. Adding a rival candidate to
// a scope leaves the previously adopted object untouched, so checking only that
// object would let a new overlap slip past a resolution that has since become
// ambiguous.
func (registry *CommercialRegistry) ViewRevision(scope CommercialScopeReference) AuthorityViewRevision {
	parts := make([]string, 0, len(registry.versions))
	for key, version := range registry.versions {
		if version.scope != scope {
			continue
		}
		parts = append(parts, strings.Join([]string{
			key.kind.String(),
			key.objectID.String(),
			key.version.String(),
			version.contentDigest.String(),
			version.status.String(),
		}, "\x1f"))
	}
	sort.Strings(parts)

	digest := sha256.Sum256([]byte(strings.Join(parts, "\x1e")))
	return AuthorityViewRevision{requiredValue{value: "VIEW-" + hex.EncodeToString(digest[:8])}}
}
