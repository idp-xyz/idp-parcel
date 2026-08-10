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

// commercialVersionKey 决定两次登记是否指同一个对象版本。对象类型算在键内，因为服务
// 产品与客户合同完全可以合法地共用同一个标识而不是同一个对象。
type commercialVersionKey struct {
	kind     CommercialObjectKind
	objectID CommercialObjectID
	version  CommercialVersionLabel
}

// CommercialRegistry 是解析据以选择的受控发布集合。它只增不改：新版本与旧版本并存
// 而不是替换它们，已登记版本的内容也绝不被改写。
type CommercialRegistry struct {
	versions map[commercialVersionKey]CommercialVersion
}

func NewCommercialRegistry() *CommercialRegistry {
	return &CommercialRegistry{versions: make(map[commercialVersionKey]CommercialVersion)}
}

// Register 接纳一个已发布版本。同内容重复登记是重放；同版本号携带不同内容是需要商业
// 责任方修正的冲突，绝不是静默覆盖。
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

// sameReleasedContent 比较一次发布固定了什么。生命周期位置刻意不算在内：一个后来生效
// 或已退役的版本仍是同一次发布，重新登记它不该被读成内容冲突。
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

// ViewRevision 是范围级的权威视图修订：一个按内容单调变化的引用，用于证明某范围解析
// 当时的商业视图是否仍是同一个。它由范围内所有版本派生，而不是手工递增——手工递增总会
// 有人忘记，派生值不可能与登记册实际内容脱节。
//
// 它刻意是范围级而非对象级。同范围新增一个竞争候选时，先前采用的那个对象一个字节都
// 没变；只检查该对象，就会让一次新的重叠溜过去，而解析其实已经不再唯一。
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
