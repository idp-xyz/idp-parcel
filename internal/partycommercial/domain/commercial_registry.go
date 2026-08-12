package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"
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

// commercialVersionKey 决定两次登记是否指同一个对象版本。租户算在键内（ADR-0040）：
// 跨租户可以合法共用同一 objectID+version。对象类型也算在键内，因为服务产品与客户合同
// 完全可以合法地共用同一个标识而不是同一个对象。
type commercialVersionKey struct {
	tenant   TenantID
	kind     CommercialObjectKind
	objectID CommercialObjectID
	version  CommercialVersionLabel
}

// CommercialRegistry 是解析据以选择的受控发布集合。它只增不改：新版本与旧版本并存
// 而不是替换它们，已登记版本的内容也绝不被改写。
//
// 商业价格政策与版本分开登记：版本回答「有没有这份价格规则对象」，政策回答「哪个方向
// 绑了哪份定价方案」。计价闭包要的是后者（ADR-0034）。
//
// 有效性更正另册保存（ADR-0038）：改的是选用区间，不是版本键下的正文。
type CommercialRegistry struct {
	versions    map[commercialVersionKey]CommercialVersion
	corrections map[commercialVersionKey]ValidityCorrection
	policies    []CommercialPricePolicy
}

func NewCommercialRegistry() *CommercialRegistry {
	return &CommercialRegistry{
		versions:    make(map[commercialVersionKey]CommercialVersion),
		corrections: make(map[commercialVersionKey]ValidityCorrection),
	}
}

// Register 接纳一个已发布版本。同内容重复登记是重放；同版本号携带不同内容是需要商业
// 责任方修正的冲突，绝不是静默覆盖。
func (registry *CommercialRegistry) Register(version CommercialVersion) (RegistrationOutcome, error) {
	if version.status == CommercialVersionStatusInvalid || version.status == CommercialVersionDraft {
		return RegistrationOutcomeInvalid, ErrCommercialVersionNotPublished
	}
	if !version.tenant.valid() {
		return RegistrationOutcomeInvalid, ErrInvalidCommercialVersion
	}

	key := commercialVersionKey{
		tenant:   version.tenant,
		kind:     version.kind,
		objectID: version.objectID,
		version:  version.version,
	}
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

// RegisterPricePolicy 接纳一份已构造的商业价格政策。它不代替 Register：政策引用的版本
// 仍须按版本通道进入登记册；这里只把「方向 + 方案绑定」放进计价选用集合。
func (registry *CommercialRegistry) RegisterPricePolicy(policy CommercialPricePolicy) {
	registry.policies = append(registry.policies, policy)
}

// PricePolicies 交回当前已登记的政策切片副本，供解析与测试观察。
func (registry *CommercialRegistry) PricePolicies() []CommercialPricePolicy {
	return append([]CommercialPricePolicy(nil), registry.policies...)
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
		sameDeclaredReferences(left, right) &&
		left.approval.reference == right.approval.reference &&
		left.approval.source == right.approval.source &&
		left.approval.approvedAt.Equal(right.approval.approvedAt)
}

// sameDeclaredReferences 让指名引用参与「是不是同一次发布」的判定。少了它，同一版本号改
// 挂另一个规则包会被读成重放而静默通过，而那正是一次需要商业责任方修正的内容冲突。
func sameDeclaredReferences(left, right CommercialVersion) bool {
	if len(left.references) != len(right.references) {
		return false
	}
	for index, reference := range left.references {
		if reference != right.references[index] {
			return false
		}
	}
	return true
}

func (registry *CommercialRegistry) Lookup(
	tenant TenantID,
	kind CommercialObjectKind,
	objectID CommercialObjectID,
	version CommercialVersionLabel,
) (CommercialVersion, bool) {
	found, exists := registry.versions[commercialVersionKey{
		tenant: tenant, kind: kind, objectID: objectID, version: version,
	}]
	return found, exists
}

func (registry *CommercialRegistry) Count() int {
	return len(registry.versions)
}

// RegisterValidityCorrection 接纳一条区间更正。原版本必须已在册；原键下正文与原区间
// 不被改写。同更正重放不推进视图；新更正写入后 ViewRevision 必变（ADR-0038）。
func (registry *CommercialRegistry) RegisterValidityCorrection(
	correction ValidityCorrection,
) (AuthorityViewRevision, error) {
	if registry == nil {
		return AuthorityViewRevision{}, ErrValidityCorrectionInvalid
	}
	key := commercialVersionKey{
		tenant:   correction.tenant,
		kind:     correction.kind,
		objectID: correction.objectID,
		version:  correction.version,
	}
	existing, found := registry.versions[key]
	if !found {
		return AuthorityViewRevision{}, ErrCommercialVersionNotPublished
	}
	if prior, ok := registry.corrections[key]; ok && sameValidityCorrection(prior, correction) {
		return registry.ViewRevision(existing.tenant, existing.scope), nil
	}
	registry.corrections[key] = correction
	return registry.ViewRevision(existing.tenant, existing.scope), nil
}

// ValidityCorrectionOf 取回指向某对象版本的区间更正（若有）。
func (registry *CommercialRegistry) ValidityCorrectionOf(
	tenant TenantID,
	kind CommercialObjectKind,
	objectID CommercialObjectID,
	version CommercialVersionLabel,
) (ValidityCorrection, bool) {
	if registry == nil {
		return ValidityCorrection{}, false
	}
	found, ok := registry.corrections[commercialVersionKey{
		tenant: tenant, kind: kind, objectID: objectID, version: version,
	}]
	return found, ok
}

// selectionInterval 是解析选用时看到的有效区间：有更正则用更正后的，否则用版本原区间。
func (registry *CommercialRegistry) selectionInterval(version CommercialVersion) EffectiveInterval {
	if registry == nil {
		return version.effective
	}
	key := commercialVersionKey{
		tenant: version.tenant, kind: version.kind, objectID: version.objectID, version: version.version,
	}
	if correction, ok := registry.corrections[key]; ok {
		return correction.corrected
	}
	return version.effective
}

// ViewRevision 是租户+范围级的权威视图修订：一个按内容单调变化的引用，用于证明某范围
// 解析当时的商业视图是否仍是同一个。它由该租户该范围内所有版本派生，而不是手工递增——
// 手工递增总会有人忘记，派生值不可能与登记册实际内容脱节。
//
// 它刻意是范围级而非对象级。同范围新增一个竞争候选时，先前采用的那个对象一个字节都
// 没变；只检查该对象，就会让一次新的重叠溜过去，而解析其实已经不再唯一。租户轴保证
// 另一租户的写入推不动本租户的修订（ADR-0040）。
func (registry *CommercialRegistry) ViewRevision(tenant TenantID, scope CommercialScopeReference) AuthorityViewRevision {
	parts := make([]string, 0, len(registry.versions)+len(registry.policies)+len(registry.corrections))
	for key, version := range registry.versions {
		if key.tenant != tenant || version.scope != scope {
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
	for key, correction := range registry.corrections {
		version, ok := registry.versions[key]
		if !ok || key.tenant != tenant || version.scope != scope {
			continue
		}
		end, bounded := correction.corrected.EndsAt()
		endPart := ""
		if bounded {
			endPart = end.UTC().Format(time.RFC3339Nano)
		}
		parts = append(parts, strings.Join([]string{
			"VALIDITY_CORRECTION",
			key.kind.String(),
			key.objectID.String(),
			key.version.String(),
			correction.reference.String(),
			correction.corrected.StartsAt().UTC().Format(time.RFC3339Nano),
			endPart,
		}, "\x1f"))
	}
	for _, policy := range registry.policies {
		if policy.scope != scope {
			continue
		}
		// 政策参与视图修订：只改绑定、不动版本正文时，解析身份仍须跟着变。
		parts = append(parts, strings.Join([]string{
			"PRICE_POLICY",
			policy.version.objectID.String(),
			policy.version.version.String(),
			policy.direction.String(),
			policy.plan.String(),
		}, "\x1f"))
	}
	sort.Strings(parts)

	digest := sha256.Sum256([]byte(strings.Join(parts, "\x1e")))
	return AuthorityViewRevision{requiredValue{value: "VIEW-" + hex.EncodeToString(digest[:8])}}
}
