package domain

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

type identifier struct {
	value string
}

func newIdentifier(name, value string) (identifier, error) {
	if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
		return identifier{}, fmt.Errorf("%w: %s", ErrInvalidIdentifier, name)
	}
	return identifier{value: value}, nil
}

func (value identifier) String() string {
	return value.value
}

func (value identifier) valid() bool {
	return value.value != "" && strings.TrimSpace(value.value) == value.value
}

type TenantID struct{ identifier }

func NewTenantID(value string) (TenantID, error) {
	identifier, err := newIdentifier("tenant ID", value)
	return TenantID{identifier: identifier}, err
}

type PricingScopeID struct{ identifier }

func NewPricingScopeID(value string) (PricingScopeID, error) {
	identifier, err := newIdentifier("pricing scope ID", value)
	return PricingScopeID{identifier: identifier}, err
}

func NewPricingScope(value string) (PricingScopeID, error) {
	return NewPricingScopeID(value)
}

type BusinessScopeID = PricingScopeID

func NewBusinessScopeID(value string) (BusinessScopeID, error) {
	return NewPricingScopeID(value)
}

type PackageID struct{ identifier }

func NewPackageID(value string) (PackageID, error) {
	identifier, err := newIdentifier("package ID", value)
	return PackageID{identifier: identifier}, err
}

type PricingPlanID struct{ identifier }

func NewPricingPlanID(value string) (PricingPlanID, error) {
	identifier, err := newIdentifier("pricing plan ID", value)
	return PricingPlanID{identifier: identifier}, err
}

type RateTableID struct{ identifier }

func NewRateTableID(value string) (RateTableID, error) {
	identifier, err := newIdentifier("rate table ID", value)
	return RateTableID{identifier: identifier}, err
}

type RateEntryID struct{ identifier }

func NewRateEntryID(value string) (RateEntryID, error) {
	identifier, err := newIdentifier("rate entry ID", value)
	return RateEntryID{identifier: identifier}, err
}

type WeightPolicyID struct{ identifier }

func NewWeightPolicyID(value string) (WeightPolicyID, error) {
	identifier, err := newIdentifier("weight policy ID", value)
	return WeightPolicyID{identifier: identifier}, err
}

type EvaluationID struct{ identifier }

func NewEvaluationID(value string) (EvaluationID, error) {
	identifier, err := newIdentifier("evaluation ID", value)
	return EvaluationID{identifier: identifier}, err
}

// EvaluationRequestReference 回指 settlement-accounting 的评价请求——UC-SA-002 步 2「请求评价」留在那一侧的
// 登记（票 sa-cc/11 裁决 2）。只引用它的铸造标识，不复制它的任何内容：主要范围、计算目的、合格来源引用只有
// SA 登记册一处权威。它与本包的 EvaluationRequest 不是一回事：那是 PP 内部「已成形、可交纯函数」的评价请求，
// 这是它为谁而形成的那份外部请求。
type EvaluationRequestReference struct{ identifier }

func NewEvaluationRequestReference(value string) (EvaluationRequestReference, error) {
	identifier, err := newIdentifier("evaluation request reference", value)
	return EvaluationRequestReference{identifier: identifier}, err
}

type Currency struct {
	code string
}

type CurrencyCode = Currency

func NewCurrency(code string) (Currency, error) {
	if len(code) != 3 || code != strings.ToUpper(code) || !allLetters(code) {
		return Currency{}, ErrInvalidCurrency
	}
	return Currency{code: code}, nil
}

func (currency Currency) String() string {
	return currency.code
}

func (currency Currency) valid() bool {
	return len(currency.code) == 3 && currency.code == strings.ToUpper(currency.code) && allLetters(currency.code)
}

type ChargeCode struct {
	value string
}

var chargeCodePattern = regexp.MustCompile("^[A-Z][A-Z0-9_]*$")

func NewChargeCode(value string) (ChargeCode, error) {
	if !chargeCodePattern.MatchString(value) {
		return ChargeCode{}, ErrInvalidChargeCode
	}
	return ChargeCode{value: value}, nil
}

func (code ChargeCode) String() string { return code.value }

func (code ChargeCode) valid() bool {
	return chargeCodePattern.MatchString(code.value)
}

type WeightUnit string

const (
	WeightUnitGram     WeightUnit = "G"
	WeightUnitKilogram WeightUnit = "KG"
	WeightUnitOunce    WeightUnit = "OZ"
	WeightUnitPound    WeightUnit = "LB"
)

func NewWeightUnit(value string) (WeightUnit, error) {
	unit := WeightUnit(value)
	if !unit.valid() {
		return "", ErrInvalidWeightUnit
	}
	return unit, nil
}

func (unit WeightUnit) String() string {
	return string(unit)
}

func (unit WeightUnit) valid() bool {
	switch unit {
	case WeightUnitGram, WeightUnitKilogram, WeightUnitOunce, WeightUnitPound:
		return true
	default:
		return false
	}
}

type Weight struct {
	value Decimal
	unit  WeightUnit
}

func NewWeight(value Decimal, unit WeightUnit) (Weight, error) {
	if !value.valid() || value.IsNegative() || !unit.valid() {
		return Weight{}, ErrInvalidWeight
	}
	return Weight{value: value, unit: unit}, nil
}

func NewWeightFromString(value string, unit WeightUnit) (Weight, error) {
	decimal, err := ParseDecimal(value)
	if err != nil {
		return Weight{}, err
	}
	return NewWeight(decimal, unit)
}

func (weight Weight) Value() Decimal {
	return weight.value
}

func (weight Weight) Unit() WeightUnit {
	return weight.unit
}

func (weight Weight) valid() bool {
	return weight.value.valid() && !weight.value.IsNegative() && weight.unit.valid()
}

func (weight Weight) Compare(other Weight) (int, error) {
	if !weight.valid() || !other.valid() {
		return 0, ErrInvalidWeight
	}
	if weight.unit != other.unit {
		return 0, ErrWeightUnitMismatch
	}
	return weight.value.Cmp(other.value), nil
}

func (weight Weight) Equal(other Weight) bool {
	return weight.valid() && other.valid() && weight.unit == other.unit && weight.value.Equal(other.value)
}

type PricingDirection string

const (
	PricingDirectionBuy      PricingDirection = "BUY"
	PricingDirectionSell     PricingDirection = "SELL"
	PricingDirectionInternal PricingDirection = "INTERNAL"
)

func NewPricingDirection(value string) (PricingDirection, error) {
	direction := PricingDirection(value)
	if !direction.valid() {
		return "", ErrInvalidDirection
	}
	return direction, nil
}

func (direction PricingDirection) valid() bool {
	switch direction {
	case PricingDirectionBuy, PricingDirectionSell, PricingDirectionInternal:
		return true
	default:
		return false
	}
}

func (direction PricingDirection) String() string {
	return string(direction)
}

type PricingPurpose string

const (
	PricingPurposeCustomerCharge PricingPurpose = "CUSTOMER_CHARGE"
	PricingPurposeSupplierCost   PricingPurpose = "SUPPLIER_COST"
	PricingPurposeInternalPrice  PricingPurpose = "INTERNAL_PRICE"
)

func NewPricingPurpose(value string) (PricingPurpose, error) {
	purpose := PricingPurpose(value)
	if !purpose.valid() {
		return "", ErrInvalidPurpose
	}
	return purpose, nil
}

func (purpose PricingPurpose) valid() bool {
	return purpose.pairedDirection().valid()
}

// pairedDirection 是本计算目的唯一可以搭配声明的价格方向。首发让两个轴一一对应，因此
// 计算目的尚未携带价格方向之外的信息；该轴保留下来，是为了将来真实参数证明需要区分同
// 方向评价时可以用上。拓宽这个轴意味着重新确认这套配对，而不是静默把它去掉。
func (purpose PricingPurpose) pairedDirection() PricingDirection {
	switch purpose {
	case PricingPurposeCustomerCharge:
		return PricingDirectionSell
	case PricingPurposeSupplierCost:
		return PricingDirectionBuy
	case PricingPurposeInternalPrice:
		return PricingDirectionInternal
	default:
		return ""
	}
}

// PairsWithDirection 报出本计算目的是否与该价格方向成对声明——方案构造门用的就是这一张配对表。导出它是让
// 收（方向、目的）成对输入的入口（按评价请求形成评价）在解析价卡之前就拒掉配错的一对：配错的一对若进了
// 解析口会零命中，被读成「未配置」，让人去登记一张本就不该存在的卡。
func (purpose PricingPurpose) PairsWithDirection(direction PricingDirection) bool {
	return purpose.valid() && purpose.pairedDirection() == direction
}

func (purpose PricingPurpose) String() string {
	return string(purpose)
}

// AggregationMode 是定价方案的聚合方式：评价在什么主体上形成一次（ADR-0111 Decision 一）。逐委托（票级）
// 与逐主单（主单级）随 ADR-0111 加入；主单级的形状已定而身份来源缺——承运总单登记册归 transport-fulfillment，
// 今天不存在。
type AggregationMode string

const (
	AggregationPerPackage        AggregationMode = "PER_PACKAGE"
	AggregationPerShipment       AggregationMode = "PER_SHIPMENT"
	AggregationPerMasterDocument AggregationMode = "PER_MASTER_DOCUMENT"
)

func (mode AggregationMode) String() string { return string(mode) }

func (mode AggregationMode) valid() bool {
	switch mode {
	case AggregationPerPackage, AggregationPerShipment, AggregationPerMasterDocument:
		return true
	default:
		return false
	}
}

// aggregate 报出该聚合方式的评价主体是不是多个包裹的集合。
func (mode AggregationMode) aggregate() bool {
	return mode != AggregationPerPackage
}

// subjectScope 是这种聚合方式下「每主体一次」的费用行所标的聚合单位。
func (mode AggregationMode) subjectScope() ChargeScope {
	switch mode {
	case AggregationPerShipment:
		return ChargeScopeShipment
	case AggregationPerMasterDocument:
		return ChargeScopeMasterDocument
	default:
		return ChargeScopePackage
	}
}

// admits 判该聚合方式接不接受这种评价主体（ADR-0111 Alternatives「主体不对，标签救不回来」）。
func (mode AggregationMode) admits(kind EvaluationSubjectKind) bool {
	switch mode {
	case AggregationPerPackage:
		return kind == SubjectAcceptedPackage || kind == SubjectEstimate
	case AggregationPerShipment:
		return kind == SubjectShipment
	case AggregationPerMasterDocument:
		return kind == SubjectMasterDocument
	default:
		return false
	}
}

// ChargeUnit 是一条固定规则或附加费规则在聚合主体上的计收单位（ADR-0111 Decision 二「按件的行按件数乘定额，
// 按票 / 按主单的行取定额一次」）。默认每主体一次；按件只在聚合方式为逐委托 / 逐主单的卡上有意义，逐包裹的卡
// 上两者是同一件事，构造门拒。
type ChargeUnit string

const (
	ChargeUnitPerSubject ChargeUnit = ""
	ChargeUnitPerPiece   ChargeUnit = "PER_PIECE"
)

func (unit ChargeUnit) String() string { return string(unit) }

func (unit ChargeUnit) valid() bool {
	return unit == ChargeUnitPerSubject || unit == ChargeUnitPerPiece
}

type PricingWeightMethod string

const (
	PricingWeightActualOnly PricingWeightMethod = "ACTUAL_ONLY"
	PricingWeightMax        PricingWeightMethod = "MAX"
)

func (method PricingWeightMethod) valid() bool {
	return method == PricingWeightActualOnly || method == PricingWeightMax
}

// RoundingMode 是取整模式的封闭集，重量取整与金额取整共用。HALF_UP（半数远离零）随 ADR-0107 加入：
// 商业取整至少要它；再扩几个由首份真实价卡的条款定，不预填。
type RoundingMode string

const (
	RoundingNone    RoundingMode = "NONE"
	RoundingCeiling RoundingMode = "CEILING"
	RoundingHalfUp  RoundingMode = "HALF_UP"
)

func (mode RoundingMode) valid() bool {
	switch mode {
	case RoundingNone, RoundingCeiling, RoundingHalfUp:
		return true
	default:
		return false
	}
}

type ChargeEffect string

const (
	ChargeEffectAdd    ChargeEffect = "ADD"
	ChargeEffectDeduct ChargeEffect = "DEDUCT"
)

func (effect ChargeEffect) valid() bool {
	return effect == ChargeEffectAdd || effect == ChargeEffectDeduct
}

// ChargeScope 是费用行上标的聚合单位（ADR-0111 Decision 一「费用行上标聚合单位，读面据此知道一行金额是每包裹
// 还是每票、每主单」）。按件计收的行在聚合主体上仍标 PACKAGE——金额是件数乘定额，解释里写着乘法。
type ChargeScope string

const (
	ChargeScopePackage        ChargeScope = "PACKAGE"
	ChargeScopeShipment       ChargeScope = "SHIPMENT"
	ChargeScopeMasterDocument ChargeScope = "MASTER_DOCUMENT"
)

func (scope ChargeScope) String() string { return string(scope) }

func (scope ChargeScope) valid() bool {
	switch scope {
	case ChargeScopePackage, ChargeScopeShipment, ChargeScopeMasterDocument:
		return true
	default:
		return false
	}
}

type ChargeBasis string

const (
	ChargeBasisRateEntry   ChargeBasis = "RATE_ENTRY"
	ChargeBasisFixedAmount ChargeBasis = "FIXED_AMOUNT"
)

func (basis ChargeBasis) valid() bool {
	return basis == ChargeBasisRateEntry || basis == ChargeBasisFixedAmount
}

// ChargeMethod 是一条评价费用行的金额如何产生。CONTEXT 把取值集合闭合为四种，且作用于
// 任何费用行，因此声明它的规则与它产生的费用行称呼同一个计算方法，而不是各留一套私有
// 词汇。扩充这个集合属于版本内容变化。
type ChargeMethod string

const (
	ChargeMethodFixedAmount    ChargeMethod = "FIXED_AMOUNT"
	ChargeMethodTableLookup    ChargeMethod = "TABLE_LOOKUP"
	ChargeMethodPercentOfBasis ChargeMethod = "PERCENT_OF_BASIS"
	ChargeMethodGreaterOf      ChargeMethod = "GREATER_OF"
	// ChargeMethodSeriesAmount 是「取当期序列定额」（ADR-0110 Decision 二）：按评价基准时点解出金额序列的那一期
	// 金额作附加费金额。并列一种新计算而不是让定额的金额可来自序列——同一种计算两种来源要在规范化形状里多一个
	// 判别字段（ADR-0110 Alternatives）。它是定额不是百分比，不带基数依赖。
	ChargeMethodSeriesAmount ChargeMethod = "SERIES_AMOUNT"
)

func (method ChargeMethod) String() string { return string(method) }

func (method ChargeMethod) valid() bool {
	switch method {
	case ChargeMethodFixedAmount, ChargeMethodTableLookup, ChargeMethodPercentOfBasis, ChargeMethodGreaterOf, ChargeMethodSeriesAmount:
		return true
	default:
		return false
	}
}

type EvidenceKind string

const (
	EvidenceSynthetic  EvidenceKind = "S"
	EvidenceReplay     EvidenceKind = "R"
	EvidenceProduction EvidenceKind = "P"
)

func (kind EvidenceKind) valid() bool {
	switch kind {
	case EvidenceSynthetic, EvidenceReplay, EvidenceProduction:
		return true
	default:
		return false
	}
}

type ArtifactKind string

const (
	ArtifactPricingPlan     ArtifactKind = "pricing-plan"
	ArtifactRateTable       ArtifactKind = "rate-table"
	ArtifactWeightPolicy    ArtifactKind = "weight-policy"
	ArtifactReferenceSeries ArtifactKind = "reference-series"
	// 计价参考目录的版本（ADR-0109）：与序列同族，评价清单里装的是它的某一版。
	ArtifactReferenceCatalogue ArtifactKind = "reference-catalogue"
	// 汇率的口径依据由商业价格政策版本声明。计价引用它，所有权在 party-commercial。
	ArtifactCommercialPolicy ArtifactKind = "commercial-policy"
	// 价格方向授权由 party-commercial 签发。价卡登记引用它作为 BUY/SELL 方向的
	// 授权依据，这里只登引用不解析其内容——解析属授权工件的所有者。
	ArtifactCommercialAuthorization ArtifactKind = "commercial-authorization"
	ArtifactNumericProfile          ArtifactKind = "numeric-profile"
)

// NumericProfileV1Reference 是评价清单里内置的数值口径引用。只带三元（ADR-0108 Decision 五）：
// 它不是从哪份工件算出来的，此前那个 `builtin:` 常量装在 digest 槽里只是占位。
func NumericProfileV1Reference() VersionReference {
	return VersionReference{
		kind:    ArtifactNumericProfile,
		id:      "decimal-bigint",
		version: "v1",
	}
}

// referenceIdentity 是版本引用的身份：（种类，标识，版本）三元（ADR-0108 Decision 一）。
// 相等、清单去重、排序与规范化文档一律只看它——此前 NewVersionManifest 的去重键就是这三元，
// 而类型自己却多比一格 digest，两处判据不一致；现在把去重键提成类型的定义。
type referenceIdentity struct {
	kind    ArtifactKind
	id      string
	version string
}

// VersionReference 引用一份版本化工件：身份三元加一枚可选的「声明时附带的指纹」（ADR-0108
// Decision 二）。指纹记的是引用方声明这条引用时手上有什么——有真摘要就带，没有就空；它随快照
// 原样读回，**不进规范化文档，也不参与相等与排序**，同一份引用带不带指纹、带哪一种，内容摘要
// 与语义摘要都不变。想按指纹校验被引对象没变，是另一条尚未立的读法，这里不预设。
type VersionReference struct {
	kind        ArtifactKind
	id          string
	version     string
	fingerprint string
}

// EffectivePeriod 采用 [startsAt, endsAt) 区间。endsAt 为零值表示没有上界。
type EffectivePeriod struct {
	startsAt time.Time
	endsAt   time.Time
}

func NewEffectivePeriod(startsAt, endsAt time.Time) (EffectivePeriod, error) {
	if startsAt.IsZero() || (!endsAt.IsZero() && !endsAt.After(startsAt)) {
		return EffectivePeriod{}, ErrInvalidEffectivePeriod
	}
	return EffectivePeriod{startsAt: startsAt.UTC(), endsAt: endsAt.UTC()}, nil
}

func NewUnboundedEffectivePeriod(startsAt time.Time) (EffectivePeriod, error) {
	return NewEffectivePeriod(startsAt, time.Time{})
}

func (period EffectivePeriod) StartsAt() time.Time { return period.startsAt }
func (period EffectivePeriod) EndsAt() time.Time   { return period.endsAt }

func (period EffectivePeriod) Contains(value time.Time) bool {
	if !period.valid() || value.IsZero() {
		return false
	}
	value = value.UTC()
	if value.Before(period.startsAt) {
		return false
	}
	return period.endsAt.IsZero() || value.Before(period.endsAt)
}

func (period EffectivePeriod) Within(outer EffectivePeriod) bool {
	if !period.valid() || !outer.valid() || period.startsAt.Before(outer.startsAt) {
		return false
	}
	if outer.endsAt.IsZero() {
		return true
	}
	return !period.endsAt.IsZero() && !period.endsAt.After(outer.endsAt)
}

func (period EffectivePeriod) valid() bool {
	return !period.startsAt.IsZero() && (period.endsAt.IsZero() || period.endsAt.After(period.startsAt))
}

func (period EffectivePeriod) canonicalString() string {
	if !period.valid() {
		return ""
	}
	end := "+INF"
	if !period.endsAt.IsZero() {
		end = period.endsAt.Format(time.RFC3339Nano)
	}
	return period.startsAt.Format(time.RFC3339Nano) + "|" + end
}

// NewVersionReferenceIdentity 以三元构造一条不带指纹的引用。三元一格不许空、不许带首尾空白
// （ADR-0108 Decision 三保留的那道校验）；指纹缺席不是缺陷，是「声明时手上没有摘要」的如实表达。
func NewVersionReferenceIdentity(kind ArtifactKind, id, version string) (VersionReference, error) {
	return NewVersionReferenceWithFingerprint(kind, id, version, "")
}

// NewVersionReferenceWithFingerprint 以三元加一枚可选指纹构造引用。指纹可空；带值时不许首尾空白
// ——一枚带空白的指纹既比不上真摘要也比不上空，只会让「有没有指纹」这个问题多出第三种答案。
func NewVersionReferenceWithFingerprint(kind ArtifactKind, id, version, fingerprint string) (VersionReference, error) {
	if kind == "" || strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id ||
		strings.TrimSpace(version) == "" || strings.TrimSpace(version) != version ||
		strings.TrimSpace(fingerprint) != fingerprint {
		return VersionReference{}, ErrInvalidVersionReference
	}
	return VersionReference{kind: kind, id: id, version: version, fingerprint: fingerprint}, nil
}

// NewVersionReference 是三步法留下的旧签名：第四参此前叫 digest 且要求非空，如今按 ADR-0108
// 进可选指纹、允许为空。保留它只为让按今天 main 形状新增的调用点（pricing/06 的转录器）在
// rebase 时不红；等 pricing/06 入 main 后收缩，调用点一律改用上面两条构造。
func NewVersionReference(kind ArtifactKind, id, version, digest string) (VersionReference, error) {
	return NewVersionReferenceWithFingerprint(kind, id, version, digest)
}

func (reference VersionReference) Kind() ArtifactKind { return reference.kind }
func (reference VersionReference) ID() string         { return reference.id }
func (reference VersionReference) Version() string    { return reference.version }

// Fingerprint 交回声明时附带的指纹，空串即缺席。
func (reference VersionReference) Fingerprint() string { return reference.fingerprint }

// HasFingerprint 说明这条引用声明时手上有没有摘要。缺席不影响任何摘要、相等与排序。
func (reference VersionReference) HasFingerprint() bool { return reference.fingerprint != "" }

// Digest 是三步法留下的旧访问器，等价于 Fingerprint；等 pricing/06 入 main 后收缩。
func (reference VersionReference) Digest() string { return reference.fingerprint }

// SameIdentity 按三元比较两条引用（ADR-0108 Decision 一）。别用 `==`：结构体相等会把指纹也
// 比进去，而两条只差指纹的引用指的是同一份工件。
func (reference VersionReference) SameIdentity(other VersionReference) bool {
	return reference.identity() == other.identity()
}

func (reference VersionReference) identity() referenceIdentity {
	return referenceIdentity{kind: reference.kind, id: reference.id, version: reference.version}
}

func (reference VersionReference) valid() bool {
	return strings.TrimSpace(string(reference.kind)) != "" && strings.TrimSpace(reference.id) != "" && strings.TrimSpace(reference.id) == reference.id &&
		strings.TrimSpace(reference.version) != "" && strings.TrimSpace(reference.version) == reference.version &&
		strings.TrimSpace(reference.fingerprint) == reference.fingerprint
}

type VersionManifest struct {
	references []VersionReference
}

func NewVersionManifest(references []VersionReference) (VersionManifest, error) {
	if len(references) == 0 {
		return VersionManifest{}, ErrInvalidVersionReference
	}
	copyOfReferences := append([]VersionReference(nil), references...)
	seen := make(map[referenceIdentity]struct{}, len(copyOfReferences))
	for _, reference := range copyOfReferences {
		if !reference.valid() {
			return VersionManifest{}, ErrInvalidVersionReference
		}
		key := reference.identity()
		if _, exists := seen[key]; exists {
			return VersionManifest{}, ErrDuplicateVersionReference
		}
		seen[key] = struct{}{}
	}
	sort.Slice(copyOfReferences, func(left, right int) bool {
		return compareVersionReferences(copyOfReferences[left], copyOfReferences[right]) < 0
	})
	return VersionManifest{references: copyOfReferences}, nil
}

// compareVersionReferences 只比三元（ADR-0108 Decision 一）：排序与去重、相等同一套判据。
func compareVersionReferences(left, right VersionReference) int {
	for _, pair := range [][2]string{
		{string(left.kind), string(right.kind)},
		{left.id, right.id},
		{left.version, right.version},
	} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}

func (manifest VersionManifest) References() []VersionReference {
	return append([]VersionReference(nil), manifest.references...)
}

func (manifest VersionManifest) valid() bool {
	if len(manifest.references) == 0 {
		return false
	}
	normalized, err := NewVersionManifest(manifest.references)
	return err == nil && manifest.Equal(normalized)
}

// Equal 按三元逐项比较两份清单（ADR-0108 Decision 一）：重放时组合出来的清单指纹可能与记录
// 时不同（当时手上有没有摘要是声明方的事），指的却是同一批工件。
func (manifest VersionManifest) Equal(other VersionManifest) bool {
	if len(manifest.references) != len(other.references) {
		return false
	}
	for index, reference := range manifest.references {
		if !reference.SameIdentity(other.references[index]) {
			return false
		}
	}
	return true
}

type Money struct {
	amount   Decimal
	currency Currency
}

func NewMoney(amount Decimal, currency Currency) (Money, error) {
	if !amount.valid() || amount.IsNegative() || !currency.valid() {
		return Money{}, ErrInvalidMoney
	}
	return Money{amount: amount, currency: currency}, nil
}

func NewMoneyFromString(amount string, currency Currency) (Money, error) {
	decimal, err := ParseDecimal(amount)
	if err != nil {
		return Money{}, err
	}
	return NewMoney(decimal, currency)
}

func (money Money) Amount() Decimal    { return money.amount }
func (money Money) Currency() Currency { return money.currency }

func (money Money) valid() bool {
	return money.amount.valid() && !money.amount.IsNegative() && money.currency.valid()
}

func (money Money) Add(other Money) (Money, error) {
	if !money.valid() || !other.valid() {
		return Money{}, ErrInvalidMoney
	}
	if money.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}
	amount, err := money.amount.Add(other.amount)
	if err != nil {
		return Money{}, err
	}
	return NewMoney(amount, money.currency)
}

func (money Money) Equal(other Money) bool {
	return money.valid() && other.valid() && money.currency == other.currency && money.amount.Equal(other.amount)
}

type VersionedFactReference struct {
	reference VersionReference
}

func NewVersionedFactReference(reference VersionReference) (VersionedFactReference, error) {
	if !reference.valid() {
		return VersionedFactReference{}, ErrInvalidVersionReference
	}
	return VersionedFactReference{reference: reference}, nil
}

func (reference VersionedFactReference) Reference() VersionReference {
	return reference.reference
}

func (reference VersionedFactReference) valid() bool {
	return reference.reference.valid()
}

func validBusinessTime(value time.Time) bool {
	return !value.IsZero()
}

func allLetters(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if (character < 'A' || character > 'Z') && (character < 'a' || character > 'z') {
			return false
		}
	}
	return true
}
