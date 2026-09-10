package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidCarrierFirstEffectivePickup = errors.New("transport fulfillment: invalid carrier first effective pickup")
	// ErrCarrierPickupNotFormed：失效只从已形成的版本长出来（ADR-0135 决定六「已形成 → 失效版本」）。待确认或
	// 已失效的链尾上没有一个「取消权已结束」的边界可撤回——它与形状错误分格，恢复动作不是改输入，而是承认
	// 这条链此刻本来就没有收寄。
	ErrCarrierPickupNotFormed = errors.New("transport fulfillment: the current carrier pickup version is not a formed pickup")
	// ErrCarrierPickupAlreadyFormed：已形成之后不回待确认（ADR-0135 决定六）。终局已据它形成、取消权已结束，
	// 另一来源的相反证据是段级实际承运商判断的来源冲突，不是收寄的更正；要撤回只能沿依据的更正关系走失效。
	ErrCarrierPickupAlreadyFormed = errors.New("transport fulfillment: a formed carrier pickup does not return to pending")
)

// CarrierFirstEffectivePickupReference 是本上下文为一条实际承运商首次有效收寄另铸的事实身份。它与载运对象
// 分开保存（ADR-0135 决定二，同 ADR-0102 决定四「所有者自己的事实身份另铸」）：一个对象至多一条链，链的
// 身份就是这个引用，各版本都挂在它下面。
type CarrierFirstEffectivePickupReference struct{ requiredValue }

func NewCarrierFirstEffectivePickupReference(value string) (CarrierFirstEffectivePickupReference, error) {
	required, err := newRequiredValue("carrier first effective pickup reference", value)
	return CarrierFirstEffectivePickupReference{required}, err
}

// CarrierFirstEffectivePickupVersion 是链上一版的身份。首登、替代、失效、待确认各签新版本，回指前版，
// 原版本一字不动；parcel-shipment 按信封所指版本取回（ADR-0135 决定七）。
type CarrierFirstEffectivePickupVersion struct{ requiredValue }

func NewCarrierFirstEffectivePickupVersion(value string) (CarrierFirstEffectivePickupVersion, error) {
	required, err := newRequiredValue("carrier first effective pickup version", value)
	return CarrierFirstEffectivePickupVersion{required}, err
}

// CarrierPickupResult 是一版的结果，封闭三格（ADR-0135 决定四、六）：已形成是收寄；待确认是判过了但不够，
// 不构成收寄、不进段、不提供；失效是「凭前版形成的收寄失去了依据」，链尾失效即该对象当前无首次有效收寄。
type CarrierPickupResult uint8

const (
	CarrierPickupResultInvalid CarrierPickupResult = iota
	CarrierPickupFormed
	CarrierPickupPending
	CarrierPickupVoided
)

func (result CarrierPickupResult) String() string {
	switch result {
	case CarrierPickupFormed:
		return "FORMED"
	case CarrierPickupPending:
		return "PENDING"
	case CarrierPickupVoided:
		return "VOIDED"
	default:
		return ""
	}
}

func (result CarrierPickupResult) valid() bool {
	return result >= CarrierPickupFormed && result <= CarrierPickupVoided
}

// ParseCarrierPickupResult 把库面的结果词认回封闭集合；词不在集合内即拒，不猜。
func ParseCarrierPickupResult(raw string) (CarrierPickupResult, error) {
	for result := CarrierPickupFormed; result <= CarrierPickupVoided; result++ {
		if result.String() == raw {
			return result, nil
		}
	}
	return CarrierPickupResultInvalid, fmt.Errorf("%w: unknown carrier pickup result %q", ErrInvalidCarrierFirstEffectivePickup, raw)
}

// PendingPickupReason 是待确认的封闭两支（ADR-0135 决定四）。「无合格证据」刻意不在其中：收寄判断由一条证据
// 触发，没有证据就没有这次判断；「证据不表达取得控制」也不在其中——那是不构成，不落版本。
type PendingPickupReason uint8

const (
	PendingPickupReasonNone PendingPickupReason = iota
	PickupSourceConflict
	PickupCarrierIdentityNotRegistered
)

func (reason PendingPickupReason) String() string {
	switch reason {
	case PickupSourceConflict:
		return "SOURCE_CONFLICT"
	case PickupCarrierIdentityNotRegistered:
		return "IDENTITY_NOT_REGISTERED"
	default:
		return ""
	}
}

func (reason PendingPickupReason) valid() bool {
	return reason == PickupSourceConflict || reason == PickupCarrierIdentityNotRegistered
}

// ParsePendingPickupReason 把库面的原因词认回封闭集合。
func ParsePendingPickupReason(raw string) (PendingPickupReason, error) {
	for reason := PickupSourceConflict; reason <= PickupCarrierIdentityNotRegistered; reason++ {
		if reason.String() == raw {
			return reason, nil
		}
	}
	return PendingPickupReasonNone, fmt.Errorf("%w: unknown pending pickup reason %q", ErrInvalidCarrierFirstEffectivePickup, raw)
}

// CarrierPickupBasis 是一条依据：来源种类（封闭四格，同实际承运商判断）、来源事实引用与**来源事实的版本**
// （ADR-0135 决定二）。版本必备——依据是「F 的第 v1 代」而不是「F」，更正才有办法回指到被更正的那一代。
type CarrierPickupBasis struct {
	source        CarrierEvidenceSource
	reference     CarrierEvidenceReference
	sourceVersion requiredValue
}

func NewCarrierPickupBasis(source CarrierEvidenceSource, reference, sourceVersion string) (CarrierPickupBasis, error) {
	if !source.valid() {
		return CarrierPickupBasis{}, fmt.Errorf("%w: basis source", ErrInvalidCarrierFirstEffectivePickup)
	}
	referenceValue, err := NewCarrierEvidenceReference(reference)
	if err != nil {
		return CarrierPickupBasis{}, fmt.Errorf("%w: %v", ErrInvalidCarrierFirstEffectivePickup, err)
	}
	version, err := newRequiredValue("carrier pickup basis source version", sourceVersion)
	if err != nil {
		return CarrierPickupBasis{}, fmt.Errorf("%w: %v", ErrInvalidCarrierFirstEffectivePickup, err)
	}
	return CarrierPickupBasis{source: source, reference: referenceValue, sourceVersion: version}, nil
}

func (basis CarrierPickupBasis) Source() CarrierEvidenceSource       { return basis.source }
func (basis CarrierPickupBasis) Reference() CarrierEvidenceReference { return basis.reference }
func (basis CarrierPickupBasis) SourceVersion() string               { return basis.sourceVersion.String() }

func (basis CarrierPickupBasis) valid() bool {
	return basis.source.valid() && basis.reference.valid() && basis.sourceVersion.valid()
}

// CarrierFirstEffectivePickupSpec 是形成一条**已形成**首登版本所需的全部输入（ADR-0135 决定二的五件加依据）。
type CarrierFirstEffectivePickupSpec struct {
	TenantID   TenantID
	Object     CarriedObjectReference
	Fact       CarrierFirstEffectivePickupReference
	Version    CarrierFirstEffectivePickupVersion
	Carrier    CarrierSubject
	OccurredAt time.Time
	JudgedAt   time.Time
	Bases      []CarrierPickupBasis
}

// PendingCarrierFirstEffectivePickupSpec 是形成一条**待确认**首登版本所需的输入：没有承运主体、没有业务时间，
// 有原因与依据。
type PendingCarrierFirstEffectivePickupSpec struct {
	TenantID TenantID
	Object   CarriedObjectReference
	Fact     CarrierFirstEffectivePickupReference
	Version  CarrierFirstEffectivePickupVersion
	Reason   PendingPickupReason
	JudgedAt time.Time
	Bases    []CarrierPickupBasis
}

// CarrierFirstEffectivePickup 是本上下文就一个载运对象首次进入某实际承运商运输控制所形成的独立控制事实
// （CONTEXT「实际承运商首次有效收寄」词条；ADR-0135）。值语义：每次转换交回新版本，原版本一字不动——
// 类型上没有任何改写既有版本的方法。
//
// 它与场外揽收、权威运输交接并列为控制事实，已形成即该对象进入实际履约段的参与起点（第三格
// EnteredByCarrierFirstEffectivePickup）；它不是外部承运轨迹事实的一列或一个版本，也不是实际承运商判断。
type CarrierFirstEffectivePickup struct {
	tenantID   TenantID
	object     CarriedObjectReference
	fact       CarrierFirstEffectivePickupReference
	version    CarrierFirstEffectivePickupVersion
	result     CarrierPickupResult
	carrier    CarrierSubject
	occurredAt time.Time
	judgedAt   time.Time
	reason     PendingPickupReason
	bases      []CarrierPickupBasis
	supersedes CarrierFirstEffectivePickupVersion
}

// FormCarrierFirstEffectivePickup 是已形成首登版本的构造门：承运主体必须是在册身份引用（本上下文不铸身份），
// 业务发生时间与判断形成时间分列在场，依据至少一条且同一（引用，版本）不重复。
func FormCarrierFirstEffectivePickup(spec CarrierFirstEffectivePickupSpec) (CarrierFirstEffectivePickup, error) {
	pickup := CarrierFirstEffectivePickup{
		tenantID:   spec.TenantID,
		object:     spec.Object,
		fact:       spec.Fact,
		version:    spec.Version,
		result:     CarrierPickupFormed,
		carrier:    spec.Carrier,
		occurredAt: spec.OccurredAt.UTC(),
		judgedAt:   spec.JudgedAt.UTC(),
		bases:      append([]CarrierPickupBasis(nil), spec.Bases...),
	}
	if !pickup.valid() {
		return CarrierFirstEffectivePickup{}, ErrInvalidCarrierFirstEffectivePickup
	}
	return pickup, nil
}

// HoldCarrierFirstEffectivePickupPending 是待确认首登版本的构造门：原因在封闭两支内、依据至少一条。
func HoldCarrierFirstEffectivePickupPending(spec PendingCarrierFirstEffectivePickupSpec) (CarrierFirstEffectivePickup, error) {
	pickup := CarrierFirstEffectivePickup{
		tenantID: spec.TenantID,
		object:   spec.Object,
		fact:     spec.Fact,
		version:  spec.Version,
		result:   CarrierPickupPending,
		judgedAt: spec.JudgedAt.UTC(),
		reason:   spec.Reason,
		bases:    append([]CarrierPickupBasis(nil), spec.Bases...),
	}
	if !pickup.valid() {
		return CarrierFirstEffectivePickup{}, ErrInvalidCarrierFirstEffectivePickup
	}
	return pickup, nil
}

func (pickup CarrierFirstEffectivePickup) TenantID() TenantID             { return pickup.tenantID }
func (pickup CarrierFirstEffectivePickup) Object() CarriedObjectReference { return pickup.object }
func (pickup CarrierFirstEffectivePickup) Fact() CarrierFirstEffectivePickupReference {
	return pickup.fact
}
func (pickup CarrierFirstEffectivePickup) Version() CarrierFirstEffectivePickupVersion {
	return pickup.version
}
func (pickup CarrierFirstEffectivePickup) Result() CarrierPickupResult { return pickup.result }
func (pickup CarrierFirstEffectivePickup) JudgedAt() time.Time         { return pickup.judgedAt }
func (pickup CarrierFirstEffectivePickup) Bases() []CarrierPickupBasis {
	return append([]CarrierPickupBasis(nil), pickup.bases...)
}
func (pickup CarrierFirstEffectivePickup) Formed() bool { return pickup.result == CarrierPickupFormed }
func (pickup CarrierFirstEffectivePickup) Voided() bool { return pickup.result == CarrierPickupVoided }

// Carrier 只在已形成的版本上交回在册承运主体。
func (pickup CarrierFirstEffectivePickup) Carrier() (CarrierSubject, bool) {
	return pickup.carrier, pickup.carrier.valid()
}

// OccurredAt 是业务发生时间——取自依据，依据是外部承运轨迹事实时取它已判断的有效时间（ADR-0135 决定三）；
// 只在已形成的版本上给出。
func (pickup CarrierFirstEffectivePickup) OccurredAt() (time.Time, bool) {
	if pickup.occurredAt.IsZero() {
		return time.Time{}, false
	}
	return pickup.occurredAt, true
}

// PendingReason 只在待确认的版本上给出。
func (pickup CarrierFirstEffectivePickup) PendingReason() (PendingPickupReason, bool) {
	return pickup.reason, pickup.reason.valid()
}

// Supersedes 交回本版本回指的前版；首登第二个返回值为 false。
func (pickup CarrierFirstEffectivePickup) Supersedes() (CarrierFirstEffectivePickupVersion, bool) {
	return pickup.supersedes, pickup.supersedes.valid()
}

// BasedOn 答本版本的依据里有没有恰为（引用，来源版本）的那一条——更正只替代「被更正的那一代恰是当前依据」
// 的版本（ADR-0135 决定六，判据同 ADR-0112 决定二）。
func (pickup CarrierFirstEffectivePickup) BasedOn(reference CarrierEvidenceReference, sourceVersion string) bool {
	for _, basis := range pickup.bases {
		if basis.reference == reference && basis.sourceVersion.String() == sourceVersion {
			return true
		}
	}
	return false
}

// CarrierPickupSupersession 携带一次替代所需的内容：新版本号、更正后（或此刻识别出）的承运主体、业务时间、
// 判断时间与依据。从待确认到已形成、从已形成到已形成、从失效再次形成，走的都是这一道门。
type CarrierPickupSupersession struct {
	Version    CarrierFirstEffectivePickupVersion
	Carrier    CarrierSubject
	OccurredAt time.Time
	JudgedAt   time.Time
	Bases      []CarrierPickupBasis
}

// Supersede 形成一条已形成的新版本回指本版本：沿用事实身份与对象，原版本一字不动（值语义）。沿用原版本号
// 即覆盖，构造期拒绝（同 OffsitePickup.Correct）。
func (pickup CarrierFirstEffectivePickup) Supersede(supersession CarrierPickupSupersession) (CarrierFirstEffectivePickup, error) {
	if !pickup.valid() || !supersession.Version.valid() || supersession.Version == pickup.version {
		return CarrierFirstEffectivePickup{}, ErrInvalidCarrierFirstEffectivePickup
	}
	next, err := FormCarrierFirstEffectivePickup(CarrierFirstEffectivePickupSpec{
		TenantID:   pickup.tenantID,
		Object:     pickup.object,
		Fact:       pickup.fact,
		Version:    supersession.Version,
		Carrier:    supersession.Carrier,
		OccurredAt: supersession.OccurredAt,
		JudgedAt:   supersession.JudgedAt,
		Bases:      supersession.Bases,
	})
	if err != nil {
		return CarrierFirstEffectivePickup{}, err
	}
	next.supersedes = pickup.version
	return next, nil
}

// CarrierPickupPendingSupersession 携带一次「待确认 → 待确认」所需的内容：通常是来源冲突把新依据并进来。
type CarrierPickupPendingSupersession struct {
	Version  CarrierFirstEffectivePickupVersion
	Reason   PendingPickupReason
	JudgedAt time.Time
	Bases    []CarrierPickupBasis
}

// HoldPending 在待确认的链尾上再长一版待确认（全部依据保留，原因可换）。已形成不回待确认（ADR-0135 决定六）；
// 失效版本上也不长待确认——链尾失效即当前无收寄，再来的证据要么形成、要么什么都不留。
func (pickup CarrierFirstEffectivePickup) HoldPending(supersession CarrierPickupPendingSupersession) (CarrierFirstEffectivePickup, error) {
	if pickup.result == CarrierPickupFormed {
		return CarrierFirstEffectivePickup{}, ErrCarrierPickupAlreadyFormed
	}
	if !pickup.valid() || pickup.result != CarrierPickupPending ||
		!supersession.Version.valid() || supersession.Version == pickup.version {
		return CarrierFirstEffectivePickup{}, ErrInvalidCarrierFirstEffectivePickup
	}
	next, err := HoldCarrierFirstEffectivePickupPending(PendingCarrierFirstEffectivePickupSpec{
		TenantID: pickup.tenantID,
		Object:   pickup.object,
		Fact:     pickup.fact,
		Version:  supersession.Version,
		Reason:   supersession.Reason,
		JudgedAt: supersession.JudgedAt,
		Bases:    supersession.Bases,
	})
	if err != nil {
		return CarrierFirstEffectivePickup{}, err
	}
	next.supersedes = pickup.version
	return next, nil
}

// CarrierPickupVoiding 携带一次失效所需的内容：新版本号、判断时间与依据（更正后不再表达收寄的那一代）。
type CarrierPickupVoiding struct {
	Version  CarrierFirstEffectivePickupVersion
	JudgedAt time.Time
	Bases    []CarrierPickupBasis
}

// Void 从已形成的版本长出失效版本：回指本版本、标失效、不带承运主体与业务时间（它记的是「凭前版形成的收寄
// 失去了依据」，不是一个新的控制起点）。只有已形成能失效（ErrCarrierPickupNotFormed）。
func (pickup CarrierFirstEffectivePickup) Void(voiding CarrierPickupVoiding) (CarrierFirstEffectivePickup, error) {
	if pickup.result != CarrierPickupFormed {
		return CarrierFirstEffectivePickup{}, ErrCarrierPickupNotFormed
	}
	if !pickup.valid() || !voiding.Version.valid() || voiding.Version == pickup.version {
		return CarrierFirstEffectivePickup{}, ErrInvalidCarrierFirstEffectivePickup
	}
	next := CarrierFirstEffectivePickup{
		tenantID:   pickup.tenantID,
		object:     pickup.object,
		fact:       pickup.fact,
		version:    voiding.Version,
		result:     CarrierPickupVoided,
		judgedAt:   voiding.JudgedAt.UTC(),
		bases:      append([]CarrierPickupBasis(nil), voiding.Bases...),
		supersedes: pickup.version,
	}
	if !next.valid() {
		return CarrierFirstEffectivePickup{}, ErrInvalidCarrierFirstEffectivePickup
	}
	return next, nil
}

// valid 是三种结果共用的形状判据：身份四件与判断时间在场、依据至少一条且不重复；已形成恰带承运主体与业务时间、
// 不带原因；待确认恰带原因、不带承运主体与业务时间；失效三者都不带且必回指前版（首登不能失效）。
func (pickup CarrierFirstEffectivePickup) valid() bool {
	if !pickup.tenantID.valid() || !pickup.object.valid() || !pickup.fact.valid() || !pickup.version.valid() ||
		pickup.judgedAt.IsZero() || !pickup.result.valid() || len(pickup.bases) == 0 {
		return false
	}
	seen := make(map[string]bool, len(pickup.bases))
	for _, basis := range pickup.bases {
		if !basis.valid() {
			return false
		}
		key := basis.reference.String() + "\x00" + basis.sourceVersion.String()
		if seen[key] {
			return false
		}
		seen[key] = true
	}
	if pickup.supersedes.valid() && pickup.supersedes == pickup.version {
		return false
	}
	switch pickup.result {
	case CarrierPickupFormed:
		return pickup.carrier.valid() && !pickup.occurredAt.IsZero() && !pickup.reason.valid()
	case CarrierPickupPending:
		return !pickup.carrier.valid() && pickup.occurredAt.IsZero() && pickup.reason.valid()
	case CarrierPickupVoided:
		return !pickup.carrier.valid() && pickup.occurredAt.IsZero() && !pickup.reason.valid() && pickup.supersedes.valid()
	default:
		return false
	}
}

// carrierPickupBasisReference 是参与关系入场依据引用的拼法：`CARRIER-FIRST-EFFECTIVE-PICKUP/<版本>`，
// 与 `OFFSITE-PICKUP/<版本>`、`TRANSPORT-HANDOVER/<版本>` 同一条纪律——分隔符归拼接处所有。
func carrierPickupBasisReference(version CarrierFirstEffectivePickupVersion) (ParticipationBasisReference, error) {
	return NewParticipationBasisReference("CARRIER-FIRST-EFFECTIVE-PICKUP/" + strings.TrimSpace(version.String()))
}
