package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// ClosureResolutionKey 一次请求多项必需商业依据。合同并不独立成立：它引用接单
// 规则包、结算政策以及形成接受判断所需的其他对象，而用例把不完整的引用闭包视为
// 整体失败，不是部分成功。
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

// MinimumIdentityEstablished 让应用层在读取权威视图之前就能判断该不该查。用例明写最小
// 身份不成立时「不得查询候选」——先查再拒，这次查询本身就已经泄露了该范围里有没有对象。
func (key ClosureResolutionKey) MinimumIdentityEstablished() bool {
	return key.minimumIdentityEstablished()
}

// fingerprint 在单依据指纹之外补上必需依据集合。少了它，同一范围下要一份合同与要合同
// 加结算政策会得到同一个指纹，于是两次不同的解析共用续办引用，调用方续办时接回的是另
// 一次尝试。集合先排序，因为声明顺序不构成不同的请求。
func (key ClosureResolutionKey) fingerprint() string {
	bases := make([]string, 0, len(key.RequiredBases))
	for _, kind := range key.RequiredBases {
		bases = append(bases, kind.String())
	}
	sort.Strings(bases)
	return strings.Join(append([]string{
		key.singleBasisKey(CommercialObjectKindInvalid).fingerprint(),
	}, bases...), "\x00")
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

// AdoptedBasis 把一项必需依据与其采用的版本配成一对。闭包保存这样的成对结构而不是
// 具名字段，是为了让并非由商业版本支撑的依据——比如参与方关系——能够加入，而不必
// 改造闭包的形状。
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

// CommercialClosure 是解析引用闭包的全有或全无结果。只要不是唯一解析成功，它就
// 一项都不采用：把已经解出的成员交回去，等于引诱调用方在用例判定为不成立的依据上
// 继续往下走。
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

// UnresolvedBases 与 ConflictingBases 两者都报出，即便结果只由其中一个决定——
// 去修`适用冲突`的商业依据所有方，同样需要知道还缺了什么。
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

// ResolveCommercialClosure 在同一个商业选择锚点和同一份权威视图下解析每一项必需
// 依据。只有全部唯一解出才算成功。
//
// 两者同时发生时，`适用冲突` 压过 `无适用依据`。它们要求的动作不同：冲突是商业依据
// 所有方必须更正的区间重叠，而缺依据只是说这个范围里没有这类对象。报出较轻的那个，
// 会让真正需要修的问题看起来无需处理。
func ResolveCommercialClosure(registry *CommercialRegistry, key ClosureResolutionKey) CommercialClosure {
	if !key.minimumIdentityEstablished() {
		return CommercialClosure{outcome: InputNotAccepted}
	}
	// 锚点策略缺失与权威读不到都是未决，但要采取的行动不同：前者等实例参数落地，后者等
	// 重试。原因压平，调用方就只能靠猜。
	if !key.Anchor.valid() {
		return closurePending(key, AnchorPolicyNotConfigured, SelectionAnchor{}, AuthorityViewRevision{})
	}
	if registry == nil {
		return closurePending(key, AuthorityUnreadable, key.Anchor, AuthorityViewRevision{})
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
			// 成员自己已经指名了停在哪一步，闭包照搬而不另起一个原因：整体未决的根据
			// 就是那一项未决。
			return closurePending(key, result.Reason(), key.Anchor, closure.viewRevision)
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

// closurePending 构造闭包所有未决答案共用的那一种形状。理由与单依据侧的 pending 相同：
// 一个结果能不能续办，不该取决于它由哪条路径产生。
func closurePending(
	key ClosureResolutionKey,
	reason ResolutionReason,
	anchor SelectionAnchor,
	view AuthorityViewRevision,
) CommercialClosure {
	return CommercialClosure{
		outcome:      ResolutionPending,
		anchor:       anchor,
		viewRevision: view,
		reason:       reason,
		continuation: continuationFor(key.fingerprint(), ResolutionID{}, reason),
	}
}

// closureIdentity 覆盖解析键、权威视图和每一个采用版本，因此同一请求在同一视图下
// 得到同一个标识，而任何一个成员发生变化都会产生不同的标识。
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
		key.fingerprint(),
		view.String(),
	}, parts...), "\x00")))
	return ResolutionID{requiredValue{value: "CLO-" + hex.EncodeToString(digest[:8])}}
}
