package pricecardtemplate

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/spreadsheet"
)

const (
	priceCardWorkbookJSONType  = "application/json"
	priceCardWorkbookSheetType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)

var _ ports.PriceCardWorkbookProjector = Reader{}

// ProjectPriceCardWorkbook 见 ports.PriceCardWorkbookProjector。两种载体是同一份投影，只在最后一步分叉。
func (Reader) ProjectPriceCardWorkbook(
	plan domain.PricingPlanVersion,
	authorization domain.VersionReference,
	carrier ports.PriceCardWorkbookCarrier,
) (ports.PriceCardWorkbookProjection, error) {
	switch carrier {
	case ports.PriceCardWorkbookStructured, ports.PriceCardWorkbookSpreadsheet:
	default:
		return ports.PriceCardWorkbookProjection{}, errUnknownWorkbookCarrier
	}
	projected, gap := projectPriceCard(plan, authorization)
	if gap.Where != "" {
		return ports.PriceCardWorkbookProjection{Gap: ports.PriceCardWorkbookGap{Where: gap.Where, Conclusion: gap.Conclusion}}, nil
	}
	document := ports.PriceCardWorkbookDocument{
		TemplateVersion: projected.version,
		FileName:        workbookFileName(plan),
	}
	if carrier == ports.PriceCardWorkbookSpreadsheet {
		document.Body = projected.xlsx
		document.MediaType = priceCardWorkbookSheetType
	} else {
		document.Body = projected.json
		document.MediaType = priceCardWorkbookJSONType
	}
	return ports.PriceCardWorkbookProjection{Document: document}, nil
}

var errUnknownWorkbookCarrier = errors.New("pricecardtemplate: 不认的工作簿载体")

// ProjectDisclosedRateSheet 见 ports.DisclosedRateSheetProjector。
// 电子表格只留 disclosedCardField 放行的 card 格，价格表的金额行与完整投影逐格相同。
func (Reader) ProjectDisclosedRateSheet(
	plan domain.PricingPlanVersion,
	authorization domain.VersionReference,
) (ports.PriceCardWorkbookProjection, error) {
	projected, gap := projectWorkbook(plan, authorization, true)
	if gap.Where != "" {
		return ports.PriceCardWorkbookProjection{Gap: ports.PriceCardWorkbookGap{Where: gap.Where, Conclusion: gap.Conclusion}}, nil
	}
	return ports.PriceCardWorkbookProjection{Document: ports.PriceCardWorkbookDocument{
		TemplateVersion: projected.version,
		FileName:        "rate-sheet.xlsx",
		Body:            projected.xlsx,
		MediaType:       priceCardWorkbookSheetType,
	}}, nil
}

var _ ports.DisclosedRateSheetProjector = Reader{}

// projectedWorkbook 是一张卡反推出来的两种载体。json 给读口与再导入，xlsx 给下载。
// 表、列与封闭取值都用本包的 sheets、cardFields、closedVocabulary，不另写一套。
type projectedWorkbook struct {
	version string
	json    []byte
	xlsx    []byte
}

// workbookGap 是导入模板表达不了的那一处：Where 点名位置，Conclusion 是给操作者的业务结论。
// 有它就不交工作簿——交一份丢掉内容的残表会让再导入看起来成功。
type workbookGap struct {
	Where      string
	Conclusion string
}

// projectPriceCard 把一版已过构造门的价卡写成当前模板能表达的工作簿。
// 模板版本取能写下这张卡的最早一版：没声明总额上下限、按档位查表、适用服务产品与票级计价重量聚合方式的卡仍是 PPT-1，
// 进位只增不改，留空的新增格读出来与没写是同一张卡。
// 领域不保留模板节点标识，内容摘要也不含它；条件树与计算树按先序重新编号，再导入得到的是同一棵树。
func projectPriceCard(plan domain.PricingPlanVersion, authorization domain.VersionReference) (projectedWorkbook, workbookGap) {
	return projectWorkbook(plan, authorization, false)
}

// disclosedCardField 是对客价格表上 card 表留得下的格。只在这一处定义。
// 价格方向授权引用是内部引用，不进交给客户的表。其余 card 格与各张价格表都留着。
func disclosedCardField(key string) bool {
	switch key {
	case "directionAuthorizationId", "directionAuthorizationVersion":
		return false
	default:
		return true
	}
}

func projectWorkbook(plan domain.PricingPlanVersion, authorization domain.VersionReference, disclosed bool) (projectedWorkbook, workbookGap) {
	p := &projector{plan: plan, authorization: authorization, disclosed: disclosed}
	if !p.screen() || !p.emit() {
		return projectedWorkbook{}, p.gap
	}
	sheetBody, err := p.xlsxBytes()
	if err != nil {
		return projectedWorkbook{}, workbookGap{Where: "工作簿", Conclusion: "电子表格写不出来"}
	}
	if disclosed {
		return projectedWorkbook{version: p.version, xlsx: sheetBody}, workbookGap{}
	}
	jsonBody, err := p.jsonBytes()
	if err != nil {
		return projectedWorkbook{}, workbookGap{Where: "工作簿", Conclusion: "结构化工作簿写不出来"}
	}
	return projectedWorkbook{version: p.version, json: jsonBody, xlsx: sheetBody}, workbookGap{}
}

type projector struct {
	plan          domain.PricingPlanVersion
	authorization domain.VersionReference
	disclosed     bool
	version       string
	gap           workbookGap
	remote        bool
	tablesByID    map[string]domain.RateTableVersion
	tableOrder    []string

	card          []map[string]string
	tableRows     []map[string]string
	weightZone    []map[string]string
	firstContinue []map[string]string
	unitPrice     []map[string]string
	rounding      []map[string]string
	fixed         []map[string]string
	surcharges    []map[string]string
	conditions    []map[string]string
	calculations  []map[string]string
	dependencies  []map[string]string
	bounds        []map[string]string
	series        []map[string]string
	exclusions    []map[string]string
	catalogues    []map[string]string
	manifest      []map[string]string

	conditionSeq   int
	calculationSeq int
}

func (p *projector) fail(where, conclusion string) bool {
	p.gap = workbookGap{Where: where, Conclusion: conclusion}
	return false
}

func referenceWhere(ref domain.VersionReference) string {
	return string(ref.Kind()) + " " + ref.ID() + "/" + ref.Version()
}

// rejectFingerprint 拦带指纹的引用。模板里的引用只有种类、标识、版本；指纹不进内容摘要，
// 但写进工作簿再导入就没了，那是丢掉登记时声明的一格，不是同一张卡。
func (p *projector) rejectFingerprint(ref domain.VersionReference) bool {
	if !ref.HasFingerprint() {
		return true
	}
	where := referenceWhere(ref)
	return p.fail(where, where+" 的版本引用带指纹，导入模板只收种类、标识与版本，交回工作簿会丢掉这枚指纹")
}

// 电子表格的时刻按 RFC3339 读，不收小数秒。带小数秒的生效区间写回去会对不齐。
func (p *projector) wholeSecond(where string, at time.Time) bool {
	if at.IsZero() || at.Nanosecond() == 0 {
		return true
	}
	return p.fail(where, where+" 是 "+at.UTC().Format(time.RFC3339Nano)+"，导入模板的时刻只收到整秒")
}

func (p *projector) builtIn(ref domain.VersionReference) bool {
	return ref.SameIdentity(p.plan.Reference()) ||
		ref.SameIdentity(p.plan.RateTable().Reference()) ||
		ref.SameIdentity(p.plan.WeightPolicy().Reference()) ||
		ref.SameIdentity(domain.NumericProfileV1Reference())
}

func (p *projector) screen() bool {
	if p.authorization.Kind() != domain.ArtifactCommercialAuthorization || p.authorization.ID() == "" {
		return p.fail("方向授权", "这张卡没有可写入模板的价格方向授权")
	}
	if !p.rejectFingerprint(p.plan.Reference()) ||
		!p.rejectFingerprint(p.authorization) ||
		!p.rejectFingerprint(p.plan.RateTable().Reference()) ||
		!p.rejectFingerprint(p.plan.WeightPolicy().Reference()) {
		return false
	}
	if !p.rememberTable(p.plan.RateTable()) {
		return false
	}
	for _, rule := range p.plan.Structures().SurchargeRules() {
		if !p.rememberCalculations(rule.Calculation()) {
			return false
		}
	}
	for _, ref := range p.plan.Manifest().References() {
		if p.builtIn(ref) {
			continue
		}
		if !p.rejectFingerprint(ref) {
			return false
		}
		if !contains(artifactKindCodes, string(ref.Kind())) {
			return p.fail(referenceWhere(ref), "版本清单里的工件种类 "+string(ref.Kind())+" 不在导入模板的封闭取值里")
		}
	}
	span := p.plan.EffectivePeriod()
	if !p.wholeSecond("方案生效起点", span.StartsAt()) || !p.wholeSecond("方案生效止点", span.EndsAt()) {
		return false
	}
	for _, id := range p.tableOrder {
		tableSpan := p.tablesByID[id].EffectivePeriod()
		where := "价表 " + id
		if !p.wholeSecond(where+" 的生效起点", tableSpan.StartsAt()) || !p.wholeSecond(where+" 的生效止点", tableSpan.EndsAt()) {
			return false
		}
	}
	p.version = templateVersionV1
	if _, bounded := p.plan.Structures().TotalChargeBounds(); bounded || p.remote {
		p.version = templateVersionV2
	}
	if len(p.plan.ServiceProducts()) > 0 {
		p.version = templateVersionV3
	}
	if _, declared := p.plan.ShipmentWeightAggregation(); declared {
		p.version = templateVersionV4
	}
	return true
}

func (p *projector) rememberTable(table domain.RateTableVersion) bool {
	if !p.rejectFingerprint(table.Reference()) {
		return false
	}
	id := table.Reference().ID()
	if existing, found := p.tablesByID[id]; found {
		if !sameRateTable(existing, table) {
			return p.fail("价表 "+id, "价表 "+id+" 在这张卡里出现了两份不同的内容，导入模板一个标识只收一份")
		}
		return true
	}
	if p.tablesByID == nil {
		p.tablesByID = map[string]domain.RateTableVersion{}
	}
	p.tablesByID[id] = table
	p.tableOrder = append(p.tableOrder, id)
	return true
}

func (p *projector) rememberCalculations(calc domain.SurchargeCalculation) bool {
	if calc.Method() == domain.ChargeMethodTableLookup && calc.TableKey() == domain.TableKeyRemoteTier {
		p.remote = true
	}
	if table, ok := calc.LookupTable(); ok {
		if !p.rememberTable(table) {
			return false
		}
	}
	for _, operand := range calc.Operands() {
		if !p.rememberCalculations(operand) {
			return false
		}
	}
	return true
}

func (p *projector) closed(sheet, column, value, where string) bool {
	if value == "" {
		return true
	}
	for _, allowed := range vocabularyOf(sheet, column).valuesFor(p.version) {
		if allowed == value {
			return true
		}
	}
	return p.fail(where, where+" 的取值 "+value+" 不在导入模板 "+p.version+" 的封闭取值里")
}

func (p *projector) emit() bool {
	if !p.emitCard() || !p.emitTables() || !p.emitRounding() || !p.emitFixed() {
		return false
	}
	if !p.emitSurcharges() || !p.emitDependencies() || !p.emitBounds() || !p.emitSeries() {
		return false
	}
	return p.emitExclusions() && p.emitCatalogues() && p.emitManifest()
}

func (p *projector) emitCard() bool {
	weight := p.plan.WeightPolicy()
	divisor, lengthUnit := "", ""
	if factor, ok := weight.VolumetricFactor(); ok {
		divisor, lengthUnit = factor.Divisor().String(), factor.LengthUnit().String()
	}
	mode, increment, points := "", "", ""
	if rounding, ok := p.plan.Structures().AmountRounding(); ok {
		mode = string(rounding.Mode())
		increment = rounding.Increment().Amount().String()
		points = joinPoints(rounding.Points())
	}
	products := joinProducts(p.plan.ServiceProducts())
	shipmentAggregation, shipmentRoundingPoint := "", ""
	if method, declared := p.plan.ShipmentWeightAggregation(); declared {
		shipmentAggregation = method.String()
		if point, hasPoint := p.plan.ShipmentWeightRoundingPoint(); hasPoint {
			shipmentRoundingPoint = point.String()
		}
	}
	values := map[string]string{
		"templateVersion":                    p.version,
		"planId":                             p.plan.Reference().ID(),
		"planVersion":                        p.plan.Reference().Version(),
		"scope":                              p.plan.Scope().String(),
		"direction":                          p.plan.Direction().String(),
		"purpose":                            p.plan.Purpose().String(),
		"aggregation":                        p.plan.Aggregation().String(),
		"baseChargeCode":                     p.plan.BaseChargeCode().String(),
		"periodStartsAt":                     clock(p.plan.EffectivePeriod().StartsAt()),
		"periodEndsAt":                       clock(p.plan.EffectivePeriod().EndsAt()),
		"rateTableId":                        p.plan.RateTable().Reference().ID(),
		"weightPolicyId":                     weight.Reference().ID(),
		"weightPolicyVersion":                weight.Reference().Version(),
		"weightMethod":                       string(weight.Method()),
		"volumetricDivisor":                  divisor,
		"volumetricLengthUnit":               lengthUnit,
		"amountRoundingMode":                 mode,
		"amountRoundingIncrement":            increment,
		"amountRoundingPoints":               points,
		"directionAuthorizationId":           p.authorization.ID(),
		"directionAuthorizationVersion":      p.authorization.Version(),
		cardFieldServiceProducts:             products,
		cardFieldShipmentWeightAggregation:   shipmentAggregation,
		cardFieldShipmentWeightRoundingPoint: shipmentRoundingPoint,
	}
	for _, field := range cardFields {
		if !availableIn(cardFieldSince[field.key], p.version) {
			continue
		}
		if p.disclosed && !disclosedCardField(field.key) {
			continue
		}
		if omittableCardFields[field.key] && values[field.key] == "" {
			continue
		}
		where := "card." + field.key
		switch field.key {
		case "direction", "purpose", "aggregation", "weightMethod", "volumetricLengthUnit", "amountRoundingMode", cardFieldShipmentWeightAggregation, cardFieldShipmentWeightRoundingPoint:
			if !p.closed(sheetCard, field.key, values[field.key], where) {
				return false
			}
		case "amountRoundingPoints":
			for _, point := range strings.Split(values[field.key], ";") {
				if !p.closed(sheetCard, field.key, point, where) {
					return false
				}
			}
		}
		p.card = append(p.card, map[string]string{"field": field.key, "value": values[field.key]})
	}
	return true
}

func (p *projector) emitTables() bool {
	for _, id := range p.tableOrder {
		table := p.tablesByID[id]
		header := map[string]string{
			"tableId":        id,
			"tableVersion":   table.Reference().Version(),
			"family":         string(table.Family()),
			"currency":       table.Currency().String(),
			"weightUnit":     table.WeightUnit().String(),
			"periodStartsAt": clock(table.EffectivePeriod().StartsAt()),
			"periodEndsAt":   clock(table.EffectivePeriod().EndsAt()),
		}
		if !p.closed(sheetTables, "family", header["family"], "价表 "+id+" 的价表族") ||
			!p.closed(sheetTables, "currency", header["currency"], "价表 "+id+" 的币种") ||
			!p.closed(sheetTables, "weightUnit", header["weightUnit"], "价表 "+id+" 的重量单位") {
			return false
		}
		p.tableRows = append(p.tableRows, header)
		switch table.Family() {
		case domain.RateTableFamilyWeightZone:
			for _, entry := range table.Entries() {
				row := map[string]string{
					"tableId": id, "entryId": entry.ID().String(), "zone": entry.Zone(),
					"minimum": entry.Minimum().Value().String(), "amount": entry.Amount().Amount().String(),
				}
				if maximum, ok := entry.Maximum(); ok {
					row["maximum"] = maximum.Value().String()
				}
				p.weightZone = append(p.weightZone, row)
			}
		case domain.RateTableFamilyFirstContinue:
			for _, rate := range table.FirstContinueRates() {
				p.firstContinue = append(p.firstContinue, map[string]string{
					"tableId": id, "entryId": rate.ID().String(), "zone": rate.Zone(),
					"firstWeight": rate.FirstWeight().Value().String(), "firstAmount": rate.FirstAmount().Amount().String(),
					"step": rate.Step().Value().String(), "stepAmount": rate.StepAmount().Amount().String(),
				})
			}
		case domain.RateTableFamilyUnitPrice:
			for _, rate := range table.UnitPriceRates() {
				p.unitPrice = append(p.unitPrice, map[string]string{
					"tableId": id, "entryId": rate.ID().String(), "zone": rate.Zone(),
					"amountPerUnit": rate.AmountPerUnit().Amount().String(),
				})
			}
		default:
			return p.fail("价表 "+id, "价表 "+id+" 的价表族 "+string(table.Family())+" 不在导入模板里")
		}
	}
	return true
}

func (p *projector) emitRounding() bool {
	weight := p.plan.WeightPolicy()
	if !p.writeRounding("CHARGEABLE", weight.Rounding()) {
		return false
	}
	if factor, ok := weight.VolumetricFactor(); ok {
		return p.writeRounding("VOLUMETRIC", factor.Rounding())
	}
	return true
}

func (p *projector) writeRounding(policy string, rounding domain.WeightRoundingPolicy) bool {
	for _, segment := range rounding.Segments() {
		row := map[string]string{
			"policy":    policy,
			"mode":      string(segment.Mode()),
			"increment": segment.Increment().Value().String(),
			"unit":      segment.Increment().Unit().String(),
		}
		if maximum, ok := segment.Maximum(); ok {
			row["maximum"] = maximum.Value().String()
		}
		if !p.closed(sheetWeightRounding, "policy", policy, "weight_rounding.policy") ||
			!p.closed(sheetWeightRounding, "mode", row["mode"], "weight_rounding.mode") ||
			!p.closed(sheetWeightRounding, "unit", row["unit"], "weight_rounding.unit") {
			return false
		}
		p.rounding = append(p.rounding, row)
	}
	return true
}

func (p *projector) emitFixed() bool {
	for _, rule := range p.plan.Rules() {
		row := map[string]string{
			"ruleId": rule.ID(), "chargeCode": rule.Code().String(), "description": rule.Description(),
			"effect": string(rule.Effect()), "amount": rule.Amount().Amount().String(), "order": strconv.Itoa(rule.Order()),
		}
		if unit := rule.Unit().String(); unit != "" {
			row["unit"] = unit
		}
		if !p.closed(sheetFixedCharges, "effect", row["effect"], "固定费用 "+rule.ID()+" 的收费方向") ||
			!p.closed(sheetFixedCharges, "unit", row["unit"], "固定费用 "+rule.ID()+" 的计费单位") {
			return false
		}
		p.fixed = append(p.fixed, row)
	}
	return true
}

func (p *projector) emitSurcharges() bool {
	for _, rule := range p.plan.Structures().SurchargeRules() {
		where := "附加费 " + rule.ID()
		conditionID, ok := p.emitCondition(rule.Condition(), "", where+" 的触发条件")
		if !ok {
			return false
		}
		calculationID, ok := p.emitCalculation(rule.Calculation(), "", where+" 的计算方式")
		if !ok {
			return false
		}
		row := map[string]string{
			"ruleId": rule.ID(), "chargeCode": rule.Code().String(), "description": rule.Description(),
			"effect": string(rule.Effect()), "conditionId": conditionID, "calculationId": calculationID,
		}
		if stance := rule.Exclusivity().String(); stance != "" {
			row["exclusivity"] = stance
		}
		if group, grouped := rule.ExclusivityGroup(); grouped {
			row["exclusivityGroup"] = group
			row["priority"] = strconv.Itoa(rule.Priority())
		}
		if unit := rule.Unit().String(); unit != "" {
			row["unit"] = unit
		}
		if minimum, declared := rule.ConditionalMinimumWeight(); declared {
			minimumCondition, ok := p.emitCondition(minimum.Condition(), "", where+" 的条件最低计价重量")
			if !ok {
				return false
			}
			row["minimumWeightId"] = minimum.ID()
			row["minimumWeightConditionId"] = minimumCondition
			row["minimumWeight"] = minimum.Minimum().Value().String()
		}
		if !p.closed(sheetSurcharges, "effect", row["effect"], where+" 的收费方向") ||
			!p.closed(sheetSurcharges, "exclusivity", row["exclusivity"], where+" 的互斥立场") ||
			!p.closed(sheetSurcharges, "unit", row["unit"], where+" 的计费单位") {
			return false
		}
		p.surcharges = append(p.surcharges, row)
	}
	return true
}

func (p *projector) emitCondition(trigger domain.TriggerCondition, parent, where string) (string, bool) {
	p.conditionSeq++
	id := "c" + strconv.Itoa(p.conditionSeq)
	row := map[string]string{"nodeId": id, "kind": string(trigger.Kind())}
	if parent != "" {
		row["parentId"] = parent
	}
	if !p.closed(sheetConditions, "kind", row["kind"], where) {
		return "", false
	}
	if predicate, ok := trigger.Predicate(); ok {
		row["source"] = predicate.Source().String()
		row["operator"] = predicate.Operator().String()
		if !p.closed(sheetConditions, "source", row["source"], where) ||
			!p.closed(sheetConditions, "operator", row["operator"], where) {
			return "", false
		}
		if category, categorical := predicate.ExpectedCategory(); categorical {
			row["category"] = category.String()
		} else {
			row["threshold"] = predicate.ThresholdValue().String()
			row["unit"] = predicate.ThresholdUnit()
			if !p.closed(sheetConditions, "unit", row["unit"], where) {
				return "", false
			}
		}
		p.conditions = append(p.conditions, row)
		return id, true
	}
	p.conditions = append(p.conditions, row)
	for _, operand := range trigger.Operands() {
		if _, ok := p.emitCondition(operand, id, where); !ok {
			return "", false
		}
	}
	return id, true
}

func (p *projector) emitCalculation(calc domain.SurchargeCalculation, parent, where string) (string, bool) {
	p.calculationSeq++
	id := "k" + strconv.Itoa(p.calculationSeq)
	row := map[string]string{"nodeId": id, "method": calculationMethod(calc)}
	if parent != "" {
		row["parentId"] = parent
	}
	if !p.closed(sheetCalculations, "method", row["method"], where) {
		return "", false
	}
	switch calc.Method() {
	case domain.ChargeMethodGreaterOf:
		operands := calc.Operands()
		if len(operands) != 2 {
			return "", p.fail(where, where+" 的取较大者不是恰好两个操作数，导入模板表达不了")
		}
		p.calculations = append(p.calculations, row)
		if _, ok := p.emitCalculation(operands[0], id, where); !ok {
			return "", false
		}
		if _, ok := p.emitCalculation(operands[1], id, where); !ok {
			return "", false
		}
		return id, true
	case domain.ChargeMethodFixedAmount:
		amount, _ := calc.FixedAmount()
		row["amount"] = amount.Amount().String()
	case domain.ChargeMethodTableLookup:
		table, _ := calc.LookupTable()
		row["tableId"] = table.Reference().ID()
	case domain.ChargeMethodPercentOfBasis:
		if percent, basis, ok := calc.PercentOfBasis(); ok {
			row["percentage"] = percent.String()
			row["basisDependencyId"] = basis
		} else if kind, factor, basis, ok := calc.SeriesRate(); ok {
			row["seriesKind"] = kind.String()
			row["seriesFactor"] = factor.String()
			row["basisDependencyId"] = basis
			if !p.closed(sheetCalculations, "seriesKind", row["seriesKind"], where) {
				return "", false
			}
		} else {
			return "", p.fail(where, where+" 的按基数百分比既不是写死的百分数也不是序列费率")
		}
	case domain.ChargeMethodSeriesAmount:
		seriesID, behaviour, _ := calc.SeriesAmount()
		row["seriesId"] = seriesID
		row["outOfWindow"] = behaviour.String()
		if !p.closed(sheetCalculations, "outOfWindow", row["outOfWindow"], where) {
			return "", false
		}
	default:
		return "", p.fail(where, where+" 的计算类型 "+string(calc.Method())+" 不在导入模板里")
	}
	p.calculations = append(p.calculations, row)
	return id, true
}

func calculationMethod(calc domain.SurchargeCalculation) string {
	if calc.Method() == domain.ChargeMethodTableLookup && calc.TableKey() == domain.TableKeyRemoteTier {
		return methodTableLookupByRemoteTier
	}
	return string(calc.Method())
}

func (p *projector) emitDependencies() bool {
	for _, dependency := range p.plan.Structures().ChargeDependencies() {
		row := map[string]string{
			"dependencyId": dependency.ID(), "chargeCode": dependency.Code().String(),
			"composition": dependency.Composition().String(),
		}
		if includes := joinCodes(dependency.Includes()); includes != "" {
			row["includes"] = includes
		}
		if excludes := joinCodes(dependency.Excludes()); excludes != "" {
			row["excludes"] = excludes
		}
		if !p.closed(sheetChargeDependencies, "composition", row["composition"], "费用依赖 "+dependency.ID()) {
			return false
		}
		p.dependencies = append(p.dependencies, row)
	}
	return true
}

func (p *projector) emitBounds() bool {
	bounds, ok := p.plan.Structures().TotalChargeBounds()
	if !ok {
		return true
	}
	row := map[string]string{}
	if minimum, declared := bounds.Minimum(); declared {
		row["minimum"] = minimum.Amount().String()
	}
	if maximum, declared := bounds.Maximum(); declared {
		row["maximum"] = maximum.Amount().String()
	}
	p.bounds = append(p.bounds, row)
	return true
}

func (p *projector) emitSeries() bool {
	for _, binding := range p.plan.Structures().ReferenceSeries() {
		row := map[string]string{"kind": binding.Kind().String(), "seriesId": binding.SeriesID()}
		if !p.closed(sheetReferenceSeries, "kind", row["kind"], "参考序列 "+binding.SeriesID()) {
			return false
		}
		p.series = append(p.series, row)
	}
	return true
}

func (p *projector) emitExclusions() bool {
	for _, rule := range p.plan.Structures().ExclusionRules() {
		conditionID, ok := p.emitCondition(rule.Condition(), "", "排除条款 "+rule.ID())
		if !ok {
			return false
		}
		p.exclusions = append(p.exclusions, map[string]string{
			"exclusionId": rule.ID(), "clause": rule.Clause(), "conditionId": conditionID,
		})
	}
	return true
}

func (p *projector) emitCatalogues() bool {
	for _, link := range p.plan.Structures().ReferenceCatalogues() {
		row := map[string]string{"kind": link.Kind().String(), "catalogueId": link.CatalogueID()}
		if !p.closed(sheetReferenceCatalogues, "kind", row["kind"], "参考目录 "+link.CatalogueID()) {
			return false
		}
		p.catalogues = append(p.catalogues, row)
	}
	return true
}

// 方案、主价表、计重策略与内置数值口径由构造门自己写进版本清单，模板的 manifest_dependencies 只收额外依赖。
func (p *projector) emitManifest() bool {
	for _, ref := range p.plan.Manifest().References() {
		if p.builtIn(ref) {
			continue
		}
		p.manifest = append(p.manifest, map[string]string{
			"kind": string(ref.Kind()), "id": ref.ID(), "version": ref.Version(),
		})
	}
	return true
}

func (p *projector) jsonBytes() ([]byte, error) {
	sheets := map[string][]map[string]any{sheetCard: cardObjects(p.card)}
	put := func(name string, rows []map[string]string) {
		if len(rows) == 0 {
			return
		}
		sheets[name] = rowObjects(rows)
	}
	put(sheetTables, p.tableRows)
	put(sheetRatesWeightZone, p.weightZone)
	put(sheetRatesFirstContinue, p.firstContinue)
	put(sheetRatesUnitPrice, p.unitPrice)
	put(sheetWeightRounding, p.rounding)
	put(sheetFixedCharges, p.fixed)
	put(sheetSurcharges, p.surcharges)
	put(sheetConditions, p.conditions)
	put(sheetCalculations, p.calculations)
	put(sheetChargeDependencies, p.dependencies)
	put(sheetTotalBounds, p.bounds)
	put(sheetReferenceSeries, p.series)
	put(sheetExclusions, p.exclusions)
	put(sheetReferenceCatalogues, p.catalogues)
	put(sheetManifestDependencies, p.manifest)
	return json.Marshal(struct {
		TemplateVersion string                      `json:"templateVersion"`
		Sheets          map[string][]map[string]any `json:"sheets"`
	}{TemplateVersion: p.version, Sheets: sheets})
}

// 电子表格缺表会被读成失误，JSON 没出现的表按空表算。下载件把该版的表都带上（空表只有列键），
// 结构化工作簿只写有行的表。说明表解析时整张跳过，下载件不附，免得把空白模板的版本说明抄进某一版卡。
func (p *projector) xlsxBytes() ([]byte, error) {
	out := make([]spreadsheet.WriteSheet, 0, len(sheets))
	for _, spec := range sheets {
		if !availableIn(spec.since, p.version) {
			continue
		}
		sheet := spreadsheet.WriteSheet{Name: spec.name, Columns: textColumns(spec.columns)}
		if spec.name == sheetCard {
			for _, row := range p.card {
				sheet.Rows = append(sheet.Rows, []string{row["field"], row["value"]})
			}
		} else {
			for _, row := range p.rows(spec.name) {
				sheet.Rows = append(sheet.Rows, aligned(spec.columns, row))
			}
		}
		out = append(out, sheet)
	}
	var buffer bytes.Buffer
	if err := spreadsheet.Write(&buffer, out); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (p *projector) rows(name string) []map[string]string {
	switch name {
	case sheetTables:
		return p.tableRows
	case sheetRatesWeightZone:
		return p.weightZone
	case sheetRatesFirstContinue:
		return p.firstContinue
	case sheetRatesUnitPrice:
		return p.unitPrice
	case sheetWeightRounding:
		return p.rounding
	case sheetFixedCharges:
		return p.fixed
	case sheetSurcharges:
		return p.surcharges
	case sheetConditions:
		return p.conditions
	case sheetCalculations:
		return p.calculations
	case sheetChargeDependencies:
		return p.dependencies
	case sheetTotalBounds:
		return p.bounds
	case sheetReferenceSeries:
		return p.series
	case sheetExclusions:
		return p.exclusions
	case sheetReferenceCatalogues:
		return p.catalogues
	case sheetManifestDependencies:
		return p.manifest
	default:
		return nil
	}
}

func cardObjects(rows []map[string]string) []map[string]any {
	out := make([]map[string]any, len(rows))
	for index, row := range rows {
		var value any
		if row["value"] != "" {
			value = row["value"]
		}
		out[index] = map[string]any{"field": row["field"], "value": value}
	}
	return out
}

func rowObjects(rows []map[string]string) []map[string]any {
	out := make([]map[string]any, len(rows))
	for index, row := range rows {
		object := make(map[string]any, len(row))
		for key, value := range row {
			if value != "" {
				object[key] = value
			}
		}
		out[index] = object
	}
	return out
}

func aligned(columns []column, row map[string]string) []string {
	out := make([]string, len(columns))
	for index, column := range columns {
		out[index] = row[column.key]
	}
	return out
}

func workbookFileName(plan domain.PricingPlanVersion) string {
	id, version := plan.Reference().ID(), plan.Reference().Version()
	if !fileToken(id) || !fileToken(version) {
		return "price-card.xlsx"
	}
	return "price-card-" + id + "-" + version + ".xlsx"
}

func fileToken(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		switch {
		case character >= 'A' && character <= 'Z', character >= 'a' && character <= 'z', character >= '0' && character <= '9':
		case character == '.' || character == '_' || character == '-':
		default:
			return false
		}
	}
	return true
}

func clock(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format(time.RFC3339)
}

func joinCodes(codes []domain.ChargeCode) string {
	parts := make([]string, len(codes))
	for index, code := range codes {
		parts[index] = code.String()
	}
	return strings.Join(parts, ";")
}

func joinProducts(products []domain.ServiceProductReference) string {
	parts := make([]string, len(products))
	for index, product := range products {
		parts[index] = product.String()
	}
	return strings.Join(parts, ";")
}

func joinPoints(points []domain.AmountRoundingPoint) string {
	parts := make([]string, len(points))
	for index, point := range points {
		parts[index] = point.String()
	}
	return strings.Join(parts, ";")
}

func sameRateTable(left, right domain.RateTableVersion) bool {
	if !left.Reference().SameIdentity(right.Reference()) || string(left.Family()) != string(right.Family()) ||
		left.Currency().String() != right.Currency().String() || left.WeightUnit().String() != right.WeightUnit().String() ||
		!samePeriod(left.EffectivePeriod(), right.EffectivePeriod()) {
		return false
	}
	return sameEntries(left.Entries(), right.Entries()) &&
		sameFirst(left.FirstContinueRates(), right.FirstContinueRates()) &&
		sameUnit(left.UnitPriceRates(), right.UnitPriceRates())
}

func samePeriod(left, right domain.EffectivePeriod) bool {
	return left.StartsAt().Equal(right.StartsAt()) && left.EndsAt().Equal(right.EndsAt())
}

func sameEntries(left, right []domain.RateEntry) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].ID().String() != right[index].ID().String() || left[index].Zone() != right[index].Zone() ||
			!left[index].Minimum().Equal(right[index].Minimum()) || !left[index].Amount().Equal(right[index].Amount()) {
			return false
		}
		leftMax, leftOK := left[index].Maximum()
		rightMax, rightOK := right[index].Maximum()
		if leftOK != rightOK || (leftOK && !leftMax.Equal(rightMax)) {
			return false
		}
	}
	return true
}

func sameFirst(left, right []domain.FirstContinueRate) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].ID().String() != right[index].ID().String() || left[index].Zone() != right[index].Zone() ||
			!left[index].FirstWeight().Equal(right[index].FirstWeight()) || !left[index].FirstAmount().Equal(right[index].FirstAmount()) ||
			!left[index].Step().Equal(right[index].Step()) || !left[index].StepAmount().Equal(right[index].StepAmount()) {
			return false
		}
	}
	return true
}

func sameUnit(left, right []domain.UnitPriceRate) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].ID().String() != right[index].ID().String() || left[index].Zone() != right[index].Zone() ||
			!left[index].AmountPerUnit().Equal(right[index].AmountPerUnit()) {
			return false
		}
	}
	return true
}
