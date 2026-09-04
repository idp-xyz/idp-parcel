package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// EvaluationSubjectKind 区分「针对已受理包裹的评价」与「包裹尚不存在时做的试算」。
// 多家供应商比价发生在客户下定之前，所以后一种不是前一种的变体——下游任何环节都不得
// 把一次试算变成钱。
type EvaluationSubjectKind string

const (
	SubjectAcceptedPackage EvaluationSubjectKind = "ACCEPTED_PACKAGE"
	SubjectEstimate        EvaluationSubjectKind = "ESTIMATE"
	// SubjectShipment（委托，票级）与 SubjectMasterDocument（承运总单，主单级）随 ADR-0111 加入（CONTEXT「评价对象」
	// 四种）：服务按票、按主单计费的项目，输入带成员清单与合计量；金额向包裹的归因归 settlement-accounting。
	SubjectShipment       EvaluationSubjectKind = "SHIPMENT"
	SubjectMasterDocument EvaluationSubjectKind = "MASTER_DOCUMENT"
)

func (kind EvaluationSubjectKind) String() string { return string(kind) }

func (kind EvaluationSubjectKind) valid() bool {
	switch kind {
	case SubjectAcceptedPackage, SubjectEstimate, SubjectShipment, SubjectMasterDocument:
		return true
	default:
		return false
	}
}

// aggregate 报出该主体是不是多个包裹的集合——那两种主体的输入带成员清单而不带尺寸。
func (kind EvaluationSubjectKind) aggregate() bool {
	return kind == SubjectShipment || kind == SubjectMasterDocument
}

// NewShipmentSubject 指名一个已受理的委托（ADR-0111 Decision 四）：身份由 parcel-shipment 拥有，本上下文不铸。
// 按 parcel-shipment 的语言，委托身份是主体，某一提交版本随快照的事实引用带——同一委托换代后成员清单可能变，
// 主体不变（票 price-card-shape-gaps/03 记的默认读法，待 PS owner 确认）。
func NewShipmentSubject(reference string) (EvaluationSubject, error) {
	if !trimmed(reference) {
		return EvaluationSubject{}, ErrPricingInputInvalid
	}
	return EvaluationSubject{kind: SubjectShipment, id: reference}, nil
}

// NewMasterDocumentSubject 指名一份承运总单（ADR-0111 Decision 四）：身份与版本由 transport-fulfillment 拥有。
// TF 今天没有承运总单登记册——这里只是形状，不造替身、不拿包裹或集运单元引用顶替；生产上形成一次主单级评价
// 要等 TF 立册。
func NewMasterDocumentSubject(reference string) (EvaluationSubject, error) {
	if !trimmed(reference) {
		return EvaluationSubject{}, ErrPricingInputInvalid
	}
	return EvaluationSubject{kind: SubjectMasterDocument, id: reference}, nil
}

// MemberManifest 是票级 / 主单级评价输入的成员清单（ADR-0111 Decision 二）：成员包裹引用、合计实重、可缺的合计
// 体积重。件数就是成员数；合计计价重量是评价内按重量策略派生的中间结果，不在这里。它进语义摘要——同一主单
// 成员变了就是另一次评价。
type MemberManifest struct {
	members         []PackageID
	totalActual     Weight
	totalVolumetric *Weight
}

// NewMemberManifest 立一份成员清单：至少一个成员、不重复；合计体积重可缺——缺了而卡按 MAX 计价时评价待判断，
// 不退回实重（与逐包裹缺尺寸同一条 CONTEXT 规则）。
func NewMemberManifest(members []PackageID, totalActual Weight, totalVolumetric *Weight) (MemberManifest, error) {
	manifest := MemberManifest{members: append([]PackageID(nil), members...), totalActual: totalActual}
	if totalVolumetric != nil {
		declared := *totalVolumetric
		manifest.totalVolumetric = &declared
	}
	sort.SliceStable(manifest.members, func(left, right int) bool {
		return manifest.members[left].String() < manifest.members[right].String()
	})
	if !manifest.valid() {
		return MemberManifest{}, ErrPricingInputInvalid
	}
	return manifest, nil
}

func (manifest MemberManifest) Members() []PackageID {
	return append([]PackageID(nil), manifest.members...)
}
func (manifest MemberManifest) Count() int                { return len(manifest.members) }
func (manifest MemberManifest) TotalActualWeight() Weight { return manifest.totalActual }

// TotalVolumetricWeight 只在调用方声明了合计体积重时给出。
func (manifest MemberManifest) TotalVolumetricWeight() (Weight, bool) {
	if manifest.totalVolumetric == nil {
		return Weight{}, false
	}
	return *manifest.totalVolumetric, true
}

func (manifest MemberManifest) valid() bool {
	if len(manifest.members) == 0 || !manifest.totalActual.valid() {
		return false
	}
	for index, member := range manifest.members {
		if !member.valid() || (index > 0 && manifest.members[index-1].String() >= member.String()) {
			return false
		}
	}
	return manifest.totalVolumetric == nil || (manifest.totalVolumetric.valid() && manifest.totalVolumetric.unit == manifest.totalActual.unit)
}

type EvaluationSubject struct {
	kind EvaluationSubjectKind
	id   string
}

func NewAcceptedPackageSubject(packageID PackageID) (EvaluationSubject, error) {
	if !packageID.valid() {
		return EvaluationSubject{}, ErrPricingInputInvalid
	}
	return EvaluationSubject{kind: SubjectAcceptedPackage, id: packageID.String()}, nil
}

// NewEstimateSubject 指名一个只在该次试算内存在的试算对象。引用由发起试算的一方给出；
// 本上下文不为它铸造身份。
func NewEstimateSubject(reference string) (EvaluationSubject, error) {
	if strings.TrimSpace(reference) == "" || strings.TrimSpace(reference) != reference {
		return EvaluationSubject{}, ErrPricingInputInvalid
	}
	return EvaluationSubject{kind: SubjectEstimate, id: reference}, nil
}

func (subject EvaluationSubject) Kind() EvaluationSubjectKind { return subject.kind }
func (subject EvaluationSubject) Reference() string           { return subject.id }

func (subject EvaluationSubject) valid() bool {
	return subject.kind.valid() && strings.TrimSpace(subject.id) != "" && strings.TrimSpace(subject.id) == subject.id
}

// PricingInputSnapshot 携带包裹的尺寸三边，而不是别处算好的体积重。体积重在这里由卡
// 自己版本化声明的体积系数派生，所以同一组尺寸只可能有一个体积重，而系数变化必定进入
// 版本内容摘要。
type PricingInputSnapshot struct {
	tenantID TenantID
	scope    PricingScopeID
	subject  EvaluationSubject
	// zone 是调用方直接给的分区，可缺（ADR-0109 Decision 四）：绑了目录的卡从 postal 解分区，这一格留空；
	// 没绑的卡读这一格。两者至少一个在。
	zone              string
	postal            *PostalRoute
	catalogueReadings []ResolvedCatalogueValue
	// actualWeight 在逐包裹主体上是包裹实重，在票级 / 主单级主体上是成员清单的合计实重（ADR-0111 Decision 二）。
	actualWeight Weight
	dimensions   *Dimensions
	// members 只在票级 / 主单级主体上有；包裹与试算主体不得带它，构造门按主体种类拒错配。
	members        *MemberManifest
	businessAt     time.Time
	factReferences []VersionedFactReference
	seriesValues   []ReferenceSeriesValue
	settlement     *Currency
}

// NewAggregatePricingInputSnapshot 立一份票级 / 主单级评价输入（ADR-0111 Decision 二）：主体是委托或承运总单，
// 重量来自成员清单的合计，尺寸不适用。分区走调用方给值这一格，绑了目录的卡再 WithPostalRoute 补邮编路线。
func NewAggregatePricingInputSnapshot(
	tenantID TenantID,
	scope PricingScopeID,
	subject EvaluationSubject,
	zone string,
	members MemberManifest,
	businessAt time.Time,
	factReferences ...VersionedFactReference,
) (PricingInputSnapshot, error) {
	if !subject.kind.aggregate() || !members.valid() || strings.TrimSpace(zone) == "" {
		return PricingInputSnapshot{}, ErrPricingInputInvalid
	}
	input, err := newPricingInputSnapshot(tenantID, scope, subject, zone, members.totalActual, nil, businessAt, factReferences)
	if err != nil {
		return PricingInputSnapshot{}, err
	}
	declared := members
	input.members = &declared
	return input, nil
}

// Members 只在票级 / 主单级主体上给出成员清单。
func (input PricingInputSnapshot) Members() (MemberManifest, bool) {
	if input.members == nil {
		return MemberManifest{}, false
	}
	return *input.members, true
}

// memberCount 是按件计收的乘数：聚合主体取成员数，单包裹主体恒为一。
func (input PricingInputSnapshot) memberCount() int {
	if input.members == nil {
		return 1
	}
	return len(input.members.members)
}

// NewPostalPricingInputSnapshot 立一份不带调用方分区、只带邮编路线的输入：分区与偏远档位由绑了目录的卡
// 从目录解出（ADR-0109 Decision 四）。给到一张没绑目录的卡，评价落待判断——没有任何一方能产出分区。
func NewPostalPricingInputSnapshot(
	tenantID TenantID,
	scope PricingScopeID,
	subject EvaluationSubject,
	route PostalRoute,
	actualWeight Weight,
	dimensions *Dimensions,
	businessAt time.Time,
	factReferences ...VersionedFactReference,
) (PricingInputSnapshot, error) {
	if !route.valid() {
		return PricingInputSnapshot{}, ErrPricingInputInvalid
	}
	input, err := newPricingInputSnapshot(tenantID, scope, subject, "", actualWeight, dimensions, businessAt, factReferences)
	if err != nil {
		return PricingInputSnapshot{}, err
	}
	declared := route
	input.postal = &declared
	return input, nil
}

// WithPostalRoute 给一份带调用方分区的输入补上邮编路线：绑了档位目录而分区仍由调用方给的卡要两者都有。
func (input PricingInputSnapshot) WithPostalRoute(route PostalRoute) (PricingInputSnapshot, error) {
	if !input.valid() || !route.valid() {
		return PricingInputSnapshot{}, ErrPricingInputInvalid
	}
	updated := copyInputSnapshot(input)
	declared := route
	updated.postal = &declared
	return updated, nil
}

// PostalRoute 报出输入携带的邮编路线；只给了分区的输入第二个返回值为假。
func (input PricingInputSnapshot) PostalRoute() (PostalRoute, bool) {
	if input.postal == nil {
		return PostalRoute{}, false
	}
	return *input.postal, true
}

// WithSettlementCurrency 记录合同的结算币种。ADR-0013 指出计价必须知道它，而
// party-commercial 已随商业依据一并提供，所以它随快照进来，不另立一个新的来源。
func (input PricingInputSnapshot) WithSettlementCurrency(settlement Currency) (PricingInputSnapshot, error) {
	if !input.valid() || !settlement.valid() {
		return PricingInputSnapshot{}, ErrPricingInputInvalid
	}
	updated := copyInputSnapshot(input)
	updated.settlement = &settlement
	return updated, nil
}

// SettlementCurrency 报出本次评价必须以哪个币种输出。
func (input PricingInputSnapshot) SettlementCurrency() (Currency, bool) {
	if input.settlement == nil {
		return Currency{}, false
	}
	return *input.settlement, true
}

// WithReferenceSeries 返回一份携带本次评价已解析序列取值的副本。它与构造函数分开，
// 因为取值是按计价基准时点解析的，那时快照的其余部分已经固定。
func (input PricingInputSnapshot) WithReferenceSeries(values ...ReferenceSeriesValue) (PricingInputSnapshot, error) {
	if !input.valid() {
		return PricingInputSnapshot{}, ErrPricingInputInvalid
	}
	seen := make(map[string]struct{}, len(values))
	copyOfValues := make([]ReferenceSeriesValue, 0, len(values))
	for _, value := range values {
		if !value.valid() {
			return PricingInputSnapshot{}, ErrInvalidReferenceSeries
		}
		// 同一序列有两期取值，会把选哪一个交给遍历顺序决定。
		if _, exists := seen[value.seriesReadingKey()]; exists {
			return PricingInputSnapshot{}, fmt.Errorf("%w: duplicate reading for %s", ErrInvalidReferenceSeries, value.seriesReadingKey())
		}
		seen[value.seriesReadingKey()] = struct{}{}
		copyOfValues = append(copyOfValues, value)
	}
	sort.SliceStable(copyOfValues, func(left, right int) bool {
		return copyOfValues[left].seriesReadingKey() < copyOfValues[right].seriesReadingKey()
	})
	updated := copyInputSnapshot(input)
	updated.seriesValues = copyOfValues
	return updated, nil
}

// ReferenceSeriesValues 报出冻结进本快照的序列取值。
func (input PricingInputSnapshot) ReferenceSeriesValues() []ReferenceSeriesValue {
	return append([]ReferenceSeriesValue(nil), input.seriesValues...)
}

func NewPricingInputSnapshot(
	tenantID TenantID,
	scope PricingScopeID,
	subject EvaluationSubject,
	zone string,
	actualWeight Weight,
	dimensions *Dimensions,
	businessAt time.Time,
	factReferences ...VersionedFactReference,
) (PricingInputSnapshot, error) {
	// 调用方给分区的路径：分区必备。不带分区的输入走 NewPostalPricingInputSnapshot。
	if strings.TrimSpace(zone) == "" {
		return PricingInputSnapshot{}, ErrPricingInputInvalid
	}
	return newPricingInputSnapshot(tenantID, scope, subject, zone, actualWeight, dimensions, businessAt, factReferences)
}

func newPricingInputSnapshot(
	tenantID TenantID,
	scope PricingScopeID,
	subject EvaluationSubject,
	zone string,
	actualWeight Weight,
	dimensions *Dimensions,
	businessAt time.Time,
	factReferences []VersionedFactReference,
) (PricingInputSnapshot, error) {
	if !tenantID.valid() || !scope.valid() || !subject.valid() || strings.TrimSpace(zone) != zone || !actualWeight.valid() || !validBusinessTime(businessAt) {
		return PricingInputSnapshot{}, ErrPricingInputInvalid
	}
	if dimensions != nil && !dimensions.valid() {
		return PricingInputSnapshot{}, ErrPricingInputInvalid
	}
	copyOfReferences := append([]VersionedFactReference(nil), factReferences...)
	for _, reference := range copyOfReferences {
		if !reference.valid() {
			return PricingInputSnapshot{}, ErrPricingInputInvalid
		}
	}
	var dimensionsCopy *Dimensions
	if dimensions != nil {
		copy := *dimensions
		dimensionsCopy = &copy
	}
	return PricingInputSnapshot{
		tenantID:       tenantID,
		scope:          scope,
		subject:        subject,
		zone:           zone,
		actualWeight:   actualWeight,
		dimensions:     dimensionsCopy,
		businessAt:     businessAt,
		factReferences: copyOfReferences,
	}, nil
}

func (input PricingInputSnapshot) TenantID() TenantID    { return input.tenantID }
func (input PricingInputSnapshot) Scope() PricingScopeID { return input.scope }
func (input PricingInputSnapshot) Zone() string          { return input.zone }

func (input PricingInputSnapshot) Subject() (EvaluationSubject, bool) {
	return input.subject, input.subject.valid()
}

// PackageID 报出本次评价针对的已受理包裹。试算对象没有包裹，所以要把评价变成钱的
// 调用方必须检查第二个返回值，不能假定它一定存在。
func (input PricingInputSnapshot) PackageID() (PackageID, bool) {
	if input.subject.kind != SubjectAcceptedPackage {
		return PackageID{}, false
	}
	packageID, err := NewPackageID(input.subject.id)
	if err != nil {
		return PackageID{}, false
	}
	return packageID, true
}
func (input PricingInputSnapshot) ActualWeight() Weight  { return input.actualWeight }
func (input PricingInputSnapshot) BusinessAt() time.Time { return input.businessAt }
func (input PricingInputSnapshot) FactReferences() []VersionedFactReference {
	return append([]VersionedFactReference(nil), input.factReferences...)
}

func (input PricingInputSnapshot) Dimensions() (Dimensions, bool) {
	if input.dimensions == nil {
		return Dimensions{}, false
	}
	return *input.dimensions, true
}

// Features 把可判定量一次性派生出来，使同一次评价里的每条规则读到相同的值，
// 而不是各自重新派生一遍。
func (input PricingInputSnapshot) Features() (PackageFeatures, error) {
	if !input.valid() {
		return PackageFeatures{}, ErrPricingInputInvalid
	}
	// 聚合主体的特征只有重量与类别：几何量在多个包裹的集合上没有定义，读它们的条件报特征不可用。
	if input.members != nil {
		return PackageFeatures{actualWeight: input.actualWeight}, nil
	}
	sides, declared := input.Dimensions()
	if !declared {
		return PackageFeatures{}, ErrMissingDimensions
	}
	return NewPackageFeatures(sides, input.actualWeight)
}

func (input PricingInputSnapshot) valid() bool {
	if !input.tenantID.valid() || !input.scope.valid() || !input.subject.valid() || strings.TrimSpace(input.zone) != input.zone || !input.actualWeight.valid() || !validBusinessTime(input.businessAt) {
		return false
	}
	// 分区与邮编路线至少一个在：两个都没有，没有任何一条路径能得出分区。
	if input.zone == "" && input.postal == nil {
		return false
	}
	if input.postal != nil && !input.postal.valid() {
		return false
	}
	for _, reading := range input.catalogueReadings {
		if !reading.valid() {
			return false
		}
	}
	if input.dimensions != nil && !input.dimensions.valid() {
		return false
	}
	// 成员清单与主体种类成对：聚合主体必有且合计实重就是快照的实重，单包裹主体必无。
	if input.subject.kind.aggregate() != (input.members != nil) {
		return false
	}
	if input.members != nil && (!input.members.valid() || input.dimensions != nil || !input.members.totalActual.Equal(input.actualWeight)) {
		return false
	}
	for _, reference := range input.factReferences {
		if !reference.valid() {
			return false
		}
	}
	return true
}

type PricingWeightResult struct {
	method       PricingWeightMethod
	actual       Weight
	volumetric   *Weight
	raw          Weight
	rounded      Weight
	roundingMode RoundingMode
	increment    Weight
	explanation  string
}

// CalculatePricingWeight 先派生重量，再抬高到方案的条件最低计价重量所施加的下限，
// 最后才进位。CONTEXT 固定了这个顺序——派生、抬高、再进位——而且它是可观察的：整千克
// 向上进位下的 1.2 KG 下限得出 2 KG，而先进位后抬高会得出 1.2，那根本不是一个整进位
// 单位。
func CalculatePricingWeight(input PricingInputSnapshot, policy PricingWeightPolicy, expectedUnit WeightUnit, floors ...Weight) (PricingWeightResult, error) {
	if !input.valid() || !policy.valid() {
		return PricingWeightResult{}, ErrPricingInputInvalid
	}
	if input.actualWeight.unit != expectedUnit {
		return PricingWeightResult{}, ErrWeightUnitMismatch
	}
	if policy.rounding.unit() != expectedUnit {
		return PricingWeightResult{}, ErrWeightUnitMismatch
	}
	var raw Weight
	var volumetric Weight
	hasVolumetric := false
	switch policy.method {
	case PricingWeightActualOnly:
		raw = input.actualWeight
	case PricingWeightMax:
		derived, err := deriveVolumetricWeight(input, policy)
		if err != nil {
			return PricingWeightResult{}, err
		}
		volumetric, hasVolumetric = derived, true
		if volumetric.unit != expectedUnit {
			return PricingWeightResult{}, ErrWeightUnitMismatch
		}
		comparison := input.actualWeight.value.Cmp(volumetric.value)
		if comparison >= 0 {
			raw = input.actualWeight
		} else {
			raw = volumetric
		}
	default:
		return PricingWeightResult{}, ErrPricingInputInvalid
	}
	derived := raw
	raised := false
	for _, floor := range floors {
		if !floor.valid() || floor.unit != raw.unit {
			return PricingWeightResult{}, ErrWeightUnitMismatch
		}
		if floor.value.Cmp(raw.value) > 0 {
			raw, raised = floor, true
		}
	}
	rounded, segment, err := policy.rounding.Apply(raw)
	if err != nil {
		return PricingWeightResult{}, err
	}
	// CONTEXT 要求解释里同时呈现抬高前后两个值，读的人才能看出最终的计价重量并非
	// 包裹自身的重量。
	raiseText := ""
	if raised {
		raiseText = fmt.Sprintf("; raised from %s %s to %s %s by a conditional minimum", derived.value.String(), derived.unit, raw.value.String(), raw.unit)
	}
	explanation := fmt.Sprintf("pricing weight uses %s; raw=%s %s%s; rounding=%s increment=%s %s%s; rounded=%s %s", policy.method, derived.value.String(), derived.unit, raiseText, segment.mode, segment.increment.value.String(), segment.increment.unit, roundingSegmentScope(segment), rounded.value.String(), rounded.unit)
	return PricingWeightResult{
		method:       policy.method,
		actual:       input.actualWeight,
		volumetric:   optionalWeightCopy(volumetric, hasVolumetric),
		raw:          raw,
		rounded:      rounded,
		roundingMode: segment.mode,
		increment:    segment.increment,
		explanation:  explanation,
	}, nil
}

func (result PricingWeightResult) Method() PricingWeightMethod { return result.method }
func (result PricingWeightResult) ActualWeight() Weight        { return result.actual }
func (result PricingWeightResult) RawWeight() Weight           { return result.raw }
func (result PricingWeightResult) RoundedWeight() Weight       { return result.rounded }
func (result PricingWeightResult) RoundingMode() RoundingMode  { return result.roundingMode }
func (result PricingWeightResult) RoundingIncrement() Weight   { return result.increment }
func (result PricingWeightResult) Explanation() string         { return result.explanation }

func (result PricingWeightResult) VolumetricWeight() (Weight, bool) {
	if result.volumetric == nil {
		return Weight{}, false
	}
	return *result.volumetric, true
}

func (result PricingWeightResult) valid() bool {
	if !result.method.valid() || !result.actual.valid() || !result.raw.valid() || !result.rounded.valid() || !result.roundingMode.valid() || !result.increment.valid() || result.increment.value.Sign() <= 0 || result.increment.unit != result.actual.unit || strings.TrimSpace(result.explanation) == "" {
		return false
	}
	if result.volumetric != nil && (!result.volumetric.valid() || result.volumetric.unit != result.actual.unit) {
		return false
	}
	return true
}

// deriveVolumetricWeight 把卡声明的体积系数施加到包裹尺寸上。尺寸没到是一个之后还可能
// 补上的事实缺失，所以调用方让评价保持`待判断`，而不是退回实重、悄悄给另一件包裹计价。
func deriveVolumetricWeight(input PricingInputSnapshot, policy PricingWeightPolicy) (Weight, error) {
	factor, declared := policy.VolumetricFactor()
	if !declared {
		return Weight{}, ErrInvalidRoundingPolicy
	}
	// 聚合主体没有一组尺寸可算：合计体积重由调用方随成员清单声明（ADR-0111 Decision 二），缺了就是缺事实。
	if input.members != nil {
		total, declared := input.members.TotalVolumetricWeight()
		if !declared {
			return Weight{}, ErrMissingDimensions
		}
		return total, nil
	}
	sides, measured := input.Dimensions()
	if !measured {
		return Weight{}, ErrMissingDimensions
	}
	volume, err := sides.Volume()
	if err != nil {
		return Weight{}, err
	}
	return factor.Apply(volume)
}

func optionalWeightCopy(value Weight, present bool) *Weight {
	if !present {
		return nil
	}
	copy := value
	return &copy
}
