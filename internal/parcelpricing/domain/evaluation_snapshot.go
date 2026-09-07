package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrEvaluationSnapshotInvalid 表示快照解不出一个立得住的评价——坏写入在重建处
// 暴露，而不是变成一个看起来合法的计算事实。
var ErrEvaluationSnapshotInvalid = errors.New("parcel pricing: invalid evaluation snapshot")

// 本文件是评价的持久化快照口。序列化形状留在领域而不是适配器，是个刻意的例外：
// 评价是二十余个值对象组成的闭合计算记录，字段全部未导出且没有外部消费者需要按列
// 检索其内部结构——在适配器里复刻整张图等于为同一形状立第二个口径，领域每动一个
// 字段就悄悄漂移一次。重建后必过 evaluation.valid()：它逐层重验费用行、金额守恒与
// 语义摘要自校（当前规范化版本内），是全仓最强的读回门。

type decimalSnapshot struct {
	Coefficient string `json:"coefficient"`
	Scale       uint32 `json:"scale"`
}

type moneySnapshot struct {
	Amount   decimalSnapshot `json:"amount"`
	Currency string          `json:"currency"`
}

type weightSnapshot struct {
	Value decimalSnapshot `json:"value"`
	Unit  string          `json:"unit"`
}

// versionReferenceSnapshot 是版本引用在各快照文档里的形状：三元必在，指纹可缺（ADR-0108）。
// Digest 只为读回旧形状而留——ADR-0108 之前那一格装的是真摘要、占位或令牌，读回时一律放进
// 可选指纹；本构建写出的快照不再写它。
type versionReferenceSnapshot struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Version     string `json:"version"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Digest      string `json:"digest,omitempty"`
}

type periodSnapshot struct {
	StartsAt time.Time `json:"startsAt"`
	EndsAt   time.Time `json:"endsAt"`
}

type subjectSnapshot struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type dimensionsSnapshot struct {
	Longest  decimalSnapshot `json:"longest"`
	Second   decimalSnapshot `json:"second"`
	Shortest decimalSnapshot `json:"shortest"`
	Unit     string          `json:"unit"`
}

type seriesValueSnapshot struct {
	Kind       string                    `json:"kind"`
	Reference  versionReferenceSnapshot  `json:"reference"`
	Value      decimalSnapshot           `json:"value"`
	QuoteBasis *versionReferenceSnapshot `json:"quoteBasis,omitempty"`
	Currency   string                    `json:"currency,omitempty"`
	Absent     bool                      `json:"absent,omitempty"`
}

type postalRouteSnapshot struct {
	Origin      string `json:"origin,omitempty"`
	Destination string `json:"destination"`
}

type catalogueReadingSnapshot struct {
	Kind      string                   `json:"kind"`
	Reference versionReferenceSnapshot `json:"reference"`
	Value     string                   `json:"value,omitempty"`
	Resolved  bool                     `json:"resolved"`
}

type memberManifestSnapshot struct {
	Members         []string        `json:"members"`
	TotalActual     weightSnapshot  `json:"totalActual"`
	TotalVolumetric *weightSnapshot `json:"totalVolumetric,omitempty"`
}

type inputSnapshotDocument struct {
	TenantID          string                     `json:"tenantId"`
	Scope             string                     `json:"scope"`
	Subject           subjectSnapshot            `json:"subject"`
	Zone              string                     `json:"zone"`
	Postal            *postalRouteSnapshot       `json:"postal,omitempty"`
	CatalogueReadings []catalogueReadingSnapshot `json:"catalogueReadings,omitempty"`
	Members           *memberManifestSnapshot    `json:"members,omitempty"`
	ActualWeight      weightSnapshot             `json:"actualWeight"`
	Dimensions        *dimensionsSnapshot        `json:"dimensions,omitempty"`
	BusinessAt        time.Time                  `json:"businessAt"`
	FactReferences    []versionReferenceSnapshot `json:"factReferences"`
	SeriesValues      []seriesValueSnapshot      `json:"seriesValues"`
	Settlement        *string                    `json:"settlement,omitempty"`
}

type weightResultSnapshot struct {
	Method       string          `json:"method"`
	Actual       weightSnapshot  `json:"actual"`
	Volumetric   *weightSnapshot `json:"volumetric,omitempty"`
	Raw          weightSnapshot  `json:"raw"`
	Rounded      weightSnapshot  `json:"rounded"`
	RoundingMode string          `json:"roundingMode"`
	Increment    weightSnapshot  `json:"increment"`
	Explanation  string          `json:"explanation"`
}

type rateSelectionSnapshot struct {
	Family      string        `json:"family"`
	ID          string        `json:"id"`
	Zone        string        `json:"zone"`
	Amount      moneySnapshot `json:"amount"`
	Explanation string        `json:"explanation"`
}

type chargeLineSnapshot struct {
	ID          string        `json:"id"`
	Kind        string        `json:"kind"`
	Code        string        `json:"code"`
	Scope       string        `json:"scope"`
	Basis       string        `json:"basis"`
	Method      string        `json:"method"`
	Description string        `json:"description"`
	Effect      string        `json:"effect"`
	Amount      moneySnapshot `json:"amount"`
	Order       int           `json:"order"`
	SourceRef   string        `json:"sourceRef"`
}

type conversionSnapshot struct {
	Original  moneySnapshot            `json:"original"`
	Rate      decimalSnapshot          `json:"rate"`
	Series    versionReferenceSnapshot `json:"series"`
	Converted moneySnapshot            `json:"converted"`
}

// issueSnapshot 多两格可缺席的「涉及序列」主体（ADR-0105 Decision 二）：只进快照不进规范化文档，旧快照没有
// 这两格照样读回、主体为空。
type issueSnapshot struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	SeriesKind string `json:"seriesKind,omitempty"`
	SeriesID   string `json:"seriesId,omitempty"`
}

func issueSnapshotOf(issue EvaluationIssue) issueSnapshot {
	snapshot := issueSnapshot{Code: issue.code, Message: issue.message}
	if issue.series != nil {
		snapshot.SeriesKind = issue.series.kind.String()
		snapshot.SeriesID = issue.series.seriesID
	}
	return snapshot
}

func issueFromSnapshot(snapshot issueSnapshot) EvaluationIssue {
	issue := EvaluationIssue{code: snapshot.Code, message: snapshot.Message}
	if snapshot.SeriesKind != "" {
		issue.series = &SeriesSubject{kind: ReferenceSeriesKind(snapshot.SeriesKind), seriesID: snapshot.SeriesID}
	}
	return issue
}

// amountRoundingStepSnapshot 是一次金额取整留痕的快照形状（ADR-0107）。
type amountRoundingStepSnapshot struct {
	Point     string        `json:"point"`
	Subject   string        `json:"subject,omitempty"`
	Mode      string        `json:"mode"`
	Increment moneySnapshot `json:"increment"`
	Before    moneySnapshot `json:"before"`
	After     moneySnapshot `json:"after"`
}

func amountRoundingStepOf(step AmountRoundingStep) amountRoundingStepSnapshot {
	return amountRoundingStepSnapshot{
		Point:     step.point.String(),
		Subject:   step.subject,
		Mode:      string(step.mode),
		Increment: moneyOf(step.increment),
		Before:    moneyOf(step.before),
		After:     moneyOf(step.after),
	}
}

func amountRoundingStepFrom(snapshot amountRoundingStepSnapshot) AmountRoundingStep {
	return AmountRoundingStep{
		point:     AmountRoundingPoint(snapshot.Point),
		subject:   snapshot.Subject,
		mode:      RoundingMode(snapshot.Mode),
		increment: moneyFrom(snapshot.Increment),
		before:    moneyFrom(snapshot.Before),
		after:     moneyFrom(snapshot.After),
	}
}

type evaluationSnapshot struct {
	ID                   string                       `json:"id"`
	ReplayOf             *string                      `json:"replayOf,omitempty"`
	Status               string                       `json:"status"`
	Evidence             string                       `json:"evidence"`
	Direction            string                       `json:"direction"`
	Purpose              string                       `json:"purpose"`
	PlanReference        versionReferenceSnapshot     `json:"planReference"`
	PlanPeriod           periodSnapshot               `json:"planPeriod"`
	TablePeriod          periodSnapshot               `json:"tablePeriod"`
	PlanContentDigest    string                       `json:"planContentDigest"`
	PlanCanonicalization string                       `json:"planCanonicalization"`
	Manifest             []versionReferenceSnapshot   `json:"manifest"`
	Input                inputSnapshotDocument        `json:"input"`
	PricingWeight        *weightResultSnapshot        `json:"pricingWeight,omitempty"`
	MatchedRate          *rateSelectionSnapshot       `json:"matchedRate,omitempty"`
	ChargeLines          []chargeLineSnapshot         `json:"chargeLines"`
	Total                *moneySnapshot               `json:"total,omitempty"`
	Conversion           *conversionSnapshot          `json:"conversion,omitempty"`
	AmountRounding       []amountRoundingStepSnapshot `json:"amountRounding,omitempty"`
	Issues               []issueSnapshot              `json:"issues"`
	Explanation          []string                     `json:"explanation"`
	SemanticDigest       string                       `json:"semanticDigest"`
}

// MarshalEvaluationSnapshot 把一份评价折成持久化快照。只接受立得住的评价——写入前
// 的门与读回的门是同一道。
func MarshalEvaluationSnapshot(evaluation PricingEvaluation) ([]byte, error) {
	if !evaluation.valid() {
		return nil, ErrEvaluationSnapshotInvalid
	}
	document := evaluationSnapshot{
		ID:                   evaluation.id.String(),
		Status:               string(evaluation.status),
		Evidence:             string(evaluation.evidence),
		Direction:            string(evaluation.direction),
		Purpose:              string(evaluation.purpose),
		PlanReference:        versionReferenceOf(evaluation.planReference),
		PlanPeriod:           periodSnapshot{StartsAt: evaluation.planPeriod.startsAt, EndsAt: evaluation.planPeriod.endsAt},
		TablePeriod:          periodSnapshot{StartsAt: evaluation.tablePeriod.startsAt, EndsAt: evaluation.tablePeriod.endsAt},
		PlanContentDigest:    evaluation.planContentDigest,
		PlanCanonicalization: evaluation.planCanonicalization,
		Input:                inputDocumentOf(evaluation.input),
		SemanticDigest:       evaluation.semanticDigest,
		Explanation:          append([]string{}, evaluation.explanation...),
		Issues:               make([]issueSnapshot, 0, len(evaluation.issues)),
		ChargeLines:          make([]chargeLineSnapshot, 0, len(evaluation.chargeLines)),
		Manifest:             make([]versionReferenceSnapshot, 0, len(evaluation.manifest.references)),
	}
	if evaluation.replayOf != nil {
		replayOf := evaluation.replayOf.String()
		document.ReplayOf = &replayOf
	}
	for _, reference := range evaluation.manifest.references {
		document.Manifest = append(document.Manifest, versionReferenceOf(reference))
	}
	if evaluation.pricingWeight != nil {
		weight := weightResultOf(*evaluation.pricingWeight)
		document.PricingWeight = &weight
	}
	if evaluation.matchedRate != nil {
		rate := rateSelectionOf(*evaluation.matchedRate)
		document.MatchedRate = &rate
	}
	for _, line := range evaluation.chargeLines {
		document.ChargeLines = append(document.ChargeLines, chargeLineOf(line))
	}
	if evaluation.total != nil {
		total := moneyOf(*evaluation.total)
		document.Total = &total
	}
	if evaluation.conversion != nil {
		conversion := conversionSnapshot{
			Original:  moneyOf(evaluation.conversion.original),
			Rate:      decimalOf(evaluation.conversion.rate),
			Series:    versionReferenceOf(evaluation.conversion.series),
			Converted: moneyOf(evaluation.conversion.converted),
		}
		document.Conversion = &conversion
	}
	for _, step := range evaluation.amountRounding {
		document.AmountRounding = append(document.AmountRounding, amountRoundingStepOf(step))
	}
	for _, issue := range evaluation.issues {
		document.Issues = append(document.Issues, issueSnapshotOf(issue))
	}
	return json.Marshal(document)
}

// RehydrateEvaluationSnapshot 从快照重建评价并整图重验：费用行守恒、状态形状与语义
// 摘要自校都过 evaluation.valid() 那一道门。
func RehydrateEvaluationSnapshot(raw []byte) (PricingEvaluation, error) {
	var document evaluationSnapshot
	if err := json.Unmarshal(raw, &document); err != nil {
		return PricingEvaluation{}, fmt.Errorf("%w: %v", ErrEvaluationSnapshotInvalid, err)
	}

	evaluation := PricingEvaluation{
		id:                   EvaluationID{identifier{value: document.ID}},
		status:               EvaluationStatus(document.Status),
		evidence:             EvidenceKind(document.Evidence),
		direction:            PricingDirection(document.Direction),
		purpose:              PricingPurpose(document.Purpose),
		planReference:        versionReferenceFrom(document.PlanReference),
		planPeriod:           EffectivePeriod{startsAt: document.PlanPeriod.StartsAt, endsAt: document.PlanPeriod.EndsAt},
		tablePeriod:          EffectivePeriod{startsAt: document.TablePeriod.StartsAt, endsAt: document.TablePeriod.EndsAt},
		planContentDigest:    document.PlanContentDigest,
		planCanonicalization: document.PlanCanonicalization,
		input:                inputFrom(document.Input),
		semanticDigest:       document.SemanticDigest,
		explanation:          append([]string{}, document.Explanation...),
	}
	if document.ReplayOf != nil {
		replayOf := EvaluationID{identifier{value: *document.ReplayOf}}
		evaluation.replayOf = &replayOf
	}
	references := make([]VersionReference, 0, len(document.Manifest))
	for _, reference := range document.Manifest {
		references = append(references, versionReferenceFrom(reference))
	}
	evaluation.manifest = VersionManifest{references: references}
	if document.PricingWeight != nil {
		weight := weightResultFrom(*document.PricingWeight)
		evaluation.pricingWeight = &weight
	}
	if document.MatchedRate != nil {
		rate := rateSelectionFrom(*document.MatchedRate)
		evaluation.matchedRate = &rate
	}
	for _, line := range document.ChargeLines {
		evaluation.chargeLines = append(evaluation.chargeLines, chargeLineFrom(line))
	}
	for _, step := range document.AmountRounding {
		evaluation.amountRounding = append(evaluation.amountRounding, amountRoundingStepFrom(step))
	}
	if document.Total != nil {
		total := moneyFrom(*document.Total)
		evaluation.total = &total
	}
	if document.Conversion != nil {
		conversion := ConversionStep{
			original:  moneyFrom(document.Conversion.Original),
			rate:      decimalFrom(document.Conversion.Rate),
			series:    versionReferenceFrom(document.Conversion.Series),
			converted: moneyFrom(document.Conversion.Converted),
		}
		evaluation.conversion = &conversion
	}
	for _, issue := range document.Issues {
		evaluation.issues = append(evaluation.issues, issueFromSnapshot(issue))
	}

	if !evaluation.valid() {
		return PricingEvaluation{}, ErrEvaluationSnapshotInvalid
	}
	return evaluation, nil
}

func decimalOf(value Decimal) decimalSnapshot {
	return decimalSnapshot{Coefficient: value.coefficient, Scale: value.scale}
}

// decimalFrom 按字段原样构造，不解析也不规范化。写法是否规范由 Decimal.valid() 判（ADR-0123 Decision 一），
// 三条重建门整图重验时把不规范的写法连同整份快照一起拒掉（Decision 三）。这里不能「先规范化再交摘要自校」：
// 一个在读回时悄悄把 1.00 改成 1 的门，会把生产路径上新出现的非规范产出者藏到第一次重放才露头，而那时报
// 的是摘要不符，与真正的内容冲突长同一张脸。
func decimalFrom(snapshot decimalSnapshot) Decimal {
	return Decimal{coefficient: snapshot.Coefficient, scale: snapshot.Scale}
}

func moneyOf(value Money) moneySnapshot {
	return moneySnapshot{Amount: decimalOf(value.amount), Currency: value.currency.code}
}

func moneyFrom(snapshot moneySnapshot) Money {
	return Money{amount: decimalFrom(snapshot.Amount), currency: Currency{code: snapshot.Currency}}
}

func weightOf(value Weight) weightSnapshot {
	return weightSnapshot{Value: decimalOf(value.value), Unit: string(value.unit)}
}

func weightFrom(snapshot weightSnapshot) Weight {
	return Weight{value: decimalFrom(snapshot.Value), unit: WeightUnit(snapshot.Unit)}
}

func versionReferenceOf(reference VersionReference) versionReferenceSnapshot {
	return versionReferenceSnapshot{
		Kind:        string(reference.kind),
		ID:          reference.id,
		Version:     reference.version,
		Fingerprint: reference.fingerprint,
	}
}

// versionReferenceFrom 读回一条引用。新形状带 fingerprint；旧形状只有 digest，读回时放进
// 可选指纹（ADR-0108 Consequences 点名的那一格兼容读法）。两者都在时以新字段为准。
func versionReferenceFrom(snapshot versionReferenceSnapshot) VersionReference {
	fingerprint := snapshot.Fingerprint
	if fingerprint == "" {
		fingerprint = snapshot.Digest
	}
	return VersionReference{
		kind:        ArtifactKind(snapshot.Kind),
		id:          snapshot.ID,
		version:     snapshot.Version,
		fingerprint: fingerprint,
	}
}

// identityOnly 把一条引用快照抹去指纹，只剩三元——序列登记的内容摘要拿它当输入，指纹因此
// 不进任何摘要（ADR-0108 Decision 二）。
func (snapshot versionReferenceSnapshot) identityOnly() versionReferenceSnapshot {
	return versionReferenceSnapshot{Kind: snapshot.Kind, ID: snapshot.ID, Version: snapshot.Version}
}

func inputDocumentOf(input PricingInputSnapshot) inputSnapshotDocument {
	document := inputSnapshotDocument{
		TenantID:       input.tenantID.String(),
		Scope:          input.scope.String(),
		Subject:        subjectSnapshot{Kind: string(input.subject.kind), ID: input.subject.id},
		Zone:           input.zone,
		ActualWeight:   weightOf(input.actualWeight),
		BusinessAt:     input.businessAt,
		FactReferences: make([]versionReferenceSnapshot, 0, len(input.factReferences)),
		SeriesValues:   make([]seriesValueSnapshot, 0, len(input.seriesValues)),
	}
	if input.dimensions != nil {
		document.Dimensions = &dimensionsSnapshot{
			Longest:  decimalOf(input.dimensions.longest),
			Second:   decimalOf(input.dimensions.second),
			Shortest: decimalOf(input.dimensions.shortest),
			Unit:     string(input.dimensions.unit),
		}
	}
	for _, reference := range input.factReferences {
		document.FactReferences = append(document.FactReferences, versionReferenceOf(reference.reference))
	}
	for _, value := range input.seriesValues {
		entry := seriesValueSnapshot{
			Kind:      string(value.kind),
			Reference: versionReferenceOf(value.reference),
			Value:     decimalOf(value.value),
			Absent:    value.absent,
		}
		if value.currency != nil {
			entry.Currency = value.currency.code
		}
		if value.quoteBasis != nil {
			basis := versionReferenceOf(*value.quoteBasis)
			entry.QuoteBasis = &basis
		}
		document.SeriesValues = append(document.SeriesValues, entry)
	}
	if input.settlement != nil {
		settlement := input.settlement.code
		document.Settlement = &settlement
	}
	if input.postal != nil {
		document.Postal = &postalRouteSnapshot{Origin: input.postal.origin, Destination: input.postal.destination}
	}
	if input.members != nil {
		manifest := memberManifestSnapshot{TotalActual: weightOf(input.members.totalActual), Members: make([]string, 0, len(input.members.members))}
		for _, member := range input.members.members {
			manifest.Members = append(manifest.Members, member.String())
		}
		if input.members.totalVolumetric != nil {
			volumetric := weightOf(*input.members.totalVolumetric)
			manifest.TotalVolumetric = &volumetric
		}
		document.Members = &manifest
	}
	for _, reading := range input.catalogueReadings {
		document.CatalogueReadings = append(document.CatalogueReadings, catalogueReadingSnapshot{
			Kind:      string(reading.kind),
			Reference: versionReferenceOf(reading.reference),
			Value:     string(reading.value),
			Resolved:  reading.resolved,
		})
	}
	return document
}

func inputFrom(document inputSnapshotDocument) PricingInputSnapshot {
	input := PricingInputSnapshot{
		tenantID:     TenantID{identifier{value: document.TenantID}},
		scope:        PricingScopeID{identifier{value: document.Scope}},
		subject:      EvaluationSubject{kind: EvaluationSubjectKind(document.Subject.Kind), id: document.Subject.ID},
		zone:         document.Zone,
		actualWeight: weightFrom(document.ActualWeight),
		businessAt:   document.BusinessAt,
	}
	if document.Dimensions != nil {
		input.dimensions = &Dimensions{
			longest:  decimalFrom(document.Dimensions.Longest),
			second:   decimalFrom(document.Dimensions.Second),
			shortest: decimalFrom(document.Dimensions.Shortest),
			unit:     LengthUnit(document.Dimensions.Unit),
		}
	}
	for _, reference := range document.FactReferences {
		input.factReferences = append(input.factReferences,
			VersionedFactReference{reference: versionReferenceFrom(reference)})
	}
	for _, value := range document.SeriesValues {
		entry := ReferenceSeriesValue{
			kind:      ReferenceSeriesKind(value.Kind),
			reference: versionReferenceFrom(value.Reference),
			value:     decimalFrom(value.Value),
			absent:    value.Absent,
		}
		if value.Currency != "" {
			currency := Currency{code: value.Currency}
			entry.currency = &currency
		}
		if value.QuoteBasis != nil {
			basis := versionReferenceFrom(*value.QuoteBasis)
			entry.quoteBasis = &basis
		}
		input.seriesValues = append(input.seriesValues, entry)
	}
	if document.Settlement != nil {
		settlement := Currency{code: *document.Settlement}
		input.settlement = &settlement
	}
	if document.Postal != nil {
		input.postal = &PostalRoute{origin: document.Postal.Origin, destination: document.Postal.Destination}
	}
	if document.Members != nil {
		manifest := MemberManifest{totalActual: weightFrom(document.Members.TotalActual)}
		for _, member := range document.Members.Members {
			manifest.members = append(manifest.members, PackageID{identifier{value: member}})
		}
		if document.Members.TotalVolumetric != nil {
			volumetric := weightFrom(*document.Members.TotalVolumetric)
			manifest.totalVolumetric = &volumetric
		}
		input.members = &manifest
	}
	for _, reading := range document.CatalogueReadings {
		input.catalogueReadings = append(input.catalogueReadings, ResolvedCatalogueValue{
			kind:      CatalogueKind(reading.Kind),
			reference: versionReferenceFrom(reading.Reference),
			value:     CategoryValue(reading.Value),
			resolved:  reading.Resolved,
		})
	}
	return input
}

func weightResultOf(result PricingWeightResult) weightResultSnapshot {
	snapshot := weightResultSnapshot{
		Method:       string(result.method),
		Actual:       weightOf(result.actual),
		Raw:          weightOf(result.raw),
		Rounded:      weightOf(result.rounded),
		RoundingMode: string(result.roundingMode),
		Increment:    weightOf(result.increment),
		Explanation:  result.explanation,
	}
	if result.volumetric != nil {
		volumetric := weightOf(*result.volumetric)
		snapshot.Volumetric = &volumetric
	}
	return snapshot
}

func weightResultFrom(snapshot weightResultSnapshot) PricingWeightResult {
	result := PricingWeightResult{
		method:       PricingWeightMethod(snapshot.Method),
		actual:       weightFrom(snapshot.Actual),
		raw:          weightFrom(snapshot.Raw),
		rounded:      weightFrom(snapshot.Rounded),
		roundingMode: RoundingMode(snapshot.RoundingMode),
		increment:    weightFrom(snapshot.Increment),
		explanation:  snapshot.Explanation,
	}
	if snapshot.Volumetric != nil {
		volumetric := weightFrom(*snapshot.Volumetric)
		result.volumetric = &volumetric
	}
	return result
}

func rateSelectionOf(selection RateSelection) rateSelectionSnapshot {
	return rateSelectionSnapshot{
		Family:      string(selection.family),
		ID:          selection.id.String(),
		Zone:        selection.zone,
		Amount:      moneyOf(selection.amount),
		Explanation: selection.explanation,
	}
}

func rateSelectionFrom(snapshot rateSelectionSnapshot) RateSelection {
	return RateSelection{
		family:      RateTableFamily(snapshot.Family),
		id:          RateEntryID{identifier{value: snapshot.ID}},
		zone:        snapshot.Zone,
		amount:      moneyFrom(snapshot.Amount),
		explanation: snapshot.Explanation,
	}
}

func chargeLineOf(line ChargeLine) chargeLineSnapshot {
	return chargeLineSnapshot{
		ID:          line.id,
		Kind:        string(line.kind),
		Code:        line.chargeCode.value,
		Scope:       string(line.scope),
		Basis:       string(line.basis),
		Method:      string(line.method),
		Description: line.description,
		Effect:      string(line.effect),
		Amount:      moneyOf(line.amount),
		Order:       line.order,
		SourceRef:   line.sourceRef,
	}
}

func chargeLineFrom(snapshot chargeLineSnapshot) ChargeLine {
	return ChargeLine{
		id:          snapshot.ID,
		kind:        ChargeLineKind(snapshot.Kind),
		chargeCode:  ChargeCode{value: snapshot.Code},
		scope:       ChargeScope(snapshot.Scope),
		basis:       ChargeBasis(snapshot.Basis),
		method:      ChargeMethod(snapshot.Method),
		description: snapshot.Description,
		effect:      ChargeEffect(snapshot.Effect),
		amount:      moneyFrom(snapshot.Amount),
		order:       snapshot.Order,
		sourceRef:   snapshot.SourceRef,
	}
}
