package pricecardtemplate

import (
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/spreadsheet"
)

// decodeWorkbook 按规范第四至六节把一份工作簿读成三种读法之一。
func decodeWorkbook(book spreadsheet.Workbook) ports.PriceCardTemplateReading {
	d := newDecoder()
	present := map[string]spreadsheet.Sheet{}
	for _, sheet := range book.Sheets {
		if sheet.Name == sheetReadme {
			continue
		}
		if _, _, known := sheetByName(sheet.Name); !known {
			d.report(sheet.Name, 0, "", ports.TemplateSheetUnknown, "表 %s 不在模板的表集合里", sheet.Name)
			continue
		}
		present[sheet.Name] = sheet
	}

	card, found := present[sheetCard]
	if !found {
		return rejectedReading(ports.TemplateVersionUnsupported, "缺 card 表，读不出模板版本；本构建认 %s", TemplateVersion)
	}
	values := d.cardValues(card)
	if values == nil {
		return rejectedReading(ports.TemplateVersionUnsupported, "card 表的列键行不完整，读不出模板版本；本构建认 %s", TemplateVersion)
	}
	switch version := values["templateVersion"]; {
	case version.state != fieldPresent:
		return rejectedReading(ports.TemplateVersionUnsupported, "card 表读不出模板版本；本构建认 %s", TemplateVersion)
	case version.text != TemplateVersion:
		return rejectedReading(ports.TemplateVersionUnsupported, "模板版本是 %s，本构建只认 %s", version.text, TemplateVersion)
	}
	planID, planVersion := values["planId"], values["planVersion"]
	if planID.state != fieldPresent || planVersion.state != fieldPresent {
		return rejectedReading(ports.TemplatePlanIdentityUnreadable, "planId 与 planVersion 读不出来，草稿没有身份可落")
	}
	plan, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, planID.text, planVersion.text)
	if err != nil {
		return rejectedReading(ports.TemplatePlanIdentityUnreadable, "planId「%s」与 planVersion「%s」立不成方案版本引用：%v", planID.text, planVersion.text, err)
	}

	for _, spec := range sheets {
		if _, found := present[spec.name]; !found {
			d.report(spec.name, 0, "", ports.TemplateSheetMissing, "缺表 %s；用不到的表留空即可，不要删", spec.name)
			d.broken[spec.name] = true
		}
	}
	t := newTranslation(d, values, plan)
	t.run(present)

	reading := ports.PriceCardTemplateReading{TemplateVersion: TemplateVersion, Accepted: true, Plan: plan}
	if len(d.problems) == 0 {
		if t.plan == nil {
			// 没报任何问题却没立出方案，是本包的缺陷；如实交成一条问题，不交出一份没有内容的「已校验」。
			d.report(sheetCard, 0, "", ports.TemplateConstructionRejected, "方案没有立出来，但没有找到是哪一格")
		} else {
			reading.Content = &ports.PriceCardTemplateContent{Plan: *t.plan, DirectionAuthorization: t.authorization}
		}
	}
	reading.Problems = d.sorted()
	return reading
}

func rejectedReading(code ports.PriceCardTemplateProblemCode, format string, args ...any) ports.PriceCardTemplateReading {
	return ports.PriceCardTemplateReading{
		TemplateVersion: TemplateVersion,
		Problems:        []ports.PriceCardTemplateProblem{{Code: code, Message: fmt.Sprintf(format, args...)}},
	}
}

// cardValues 读键值表 card；列键行不完整时交 nil。
func (d *decoder) cardValues(sheet spreadsheet.Sheet) map[string]field {
	spec, _, _ := sheetByName(sheetCard)
	keys, ignored, complete := d.header(sheet, spec)
	if !complete {
		return nil
	}
	values := map[string]field{}
	firstRow := map[string]int{}
	for _, row := range sheet.Rows {
		if row.Number < 2 {
			continue
		}
		var nameCell, valueCell *spreadsheet.Cell
		for index := range row.Cells {
			cell := &row.Cells[index]
			switch keys[cell.Column] {
			case "field":
				nameCell = cell
			case "value":
				valueCell = cell
			default:
				if !ignored[cell.Column] {
					name := spreadsheet.ColumnName(cell.Column)
					d.report(sheetCard, row.Number, name, ports.TemplateCellNotApplicable, "%s 列没有列键，却填了值", name)
				}
			}
		}
		if nameCell == nil && valueCell == nil {
			continue
		}
		name := field{sheet: sheetCard, row: row.Number, column: "field"}
		if nameCell != nil {
			name = d.readCell(sheetCard, row.Number, "field", *nameCell)
		}
		if name.state != fieldPresent {
			if name.state == fieldAbsent {
				d.at(name, ports.TemplateCellRequired, "字段名为空")
			}
			continue
		}
		if _, _, known := cardField(name.text); !known {
			d.report(sheetCard, row.Number, name.text, ports.TemplateFieldUnknown, "字段 %s 不在 card 表的字段集合里", name.text)
			continue
		}
		if first, duplicated := firstRow[name.text]; duplicated {
			d.report(sheetCard, row.Number, name.text, ports.TemplateFieldDuplicated, "字段 %s 已在第 %d 行出现过", name.text, first)
			values[name.text] = field{sheet: sheetCard, row: row.Number, column: name.text, state: fieldBad}
			continue
		}
		firstRow[name.text] = row.Number
		value := field{sheet: sheetCard, row: row.Number, column: name.text}
		if valueCell != nil {
			value = d.readCell(sheetCard, row.Number, name.text, *valueCell)
		}
		values[name.text] = value
	}
	for _, expected := range cardFields {
		if _, seen := firstRow[expected.key]; !seen {
			if _, marked := values[expected.key]; !marked {
				d.report(sheetCard, 0, expected.key, ports.TemplateFieldMissing, "缺字段 %s", expected.key)
				values[expected.key] = field{sheet: sheetCard, column: expected.key, state: fieldBad}
			}
			continue
		}
		if expected.requirement == requiredAlways {
			d.required(values[expected.key])
		}
	}
	return values
}

type rateTable struct {
	header   record
	id       string
	ref      domain.VersionReference
	family   string
	currency domain.Currency
	unit     domain.WeightUnit
	period   domain.EffectivePeriod
	headerOK bool
	rowsOK   bool
	used     bool

	entries       []domain.RateEntry
	firstContinue []domain.FirstContinueRate
	unitPrice     []domain.UnitPriceRate

	version domain.RateTableVersion
	built   bool
}

type treeNode struct {
	id       string
	parent   string
	rec      record
	children []*treeNode
}

type forest struct {
	sheet  string
	usable bool
	nodes  map[string]*treeNode
	order  []*treeNode
	used   map[string]bool
}

type builtCondition struct {
	value domain.TriggerCondition
	ok    bool
}

type builtCalculation struct {
	value domain.SurchargeCalculation
	ok    bool
}

type translation struct {
	d     *decoder
	card  map[string]field
	ref   domain.VersionReference
	sheet map[string]spreadsheet.Sheet

	scope       domain.PricingScopeID
	direction   domain.PricingDirection
	purpose     domain.PricingPurpose
	aggregation string
	baseCharge  domain.ChargeCode
	period      domain.EffectivePeriod
	headerOK    bool

	tables     map[string]*rateTable
	tableOrder []*rateTable
	main       *rateTable

	weight   domain.PricingWeightPolicy
	weightOK bool

	dependencyIDs map[string]bool
	dependencies  []domain.ChargeDependency
	dependencyOK  bool

	conditions        forest
	calculations      forest
	conditionValues   map[string]builtCondition
	calculationValues map[string]builtCalculation

	rules        []domain.FixedChargeRule
	rulesOK      bool
	surcharges   []domain.SurchargeRule
	surchargesOK bool
	series       []domain.ReferenceSeriesBinding
	seriesOK     bool
	exclusions   []domain.ExclusionRule
	exclusionsOK bool
	catalogues   []domain.ReferenceCatalogueLink
	cataloguesOK bool
	manifest     []domain.VersionReference
	manifestOK   bool

	amountRounding   *domain.AmountRoundingPolicy
	amountRoundingOK bool
	authorization    domain.VersionReference
	authorizationOK  bool

	plan *domain.PricingPlanVersion
}

func newTranslation(d *decoder, card map[string]field, ref domain.VersionReference) *translation {
	return &translation{
		d: d, card: card, ref: ref,
		tables:            map[string]*rateTable{},
		dependencyIDs:     map[string]bool{},
		conditionValues:   map[string]builtCondition{},
		calculationValues: map[string]builtCalculation{},
	}
}

func (t *translation) run(present map[string]spreadsheet.Sheet) {
	t.sheet = present
	t.readHeader()
	t.readTables()
	t.resolveMainTable()
	t.readWeight()
	t.readDependencies()
	t.conditions = t.readForest(sheetConditions)
	t.calculations = t.readForest(sheetCalculations)
	t.markLookupTables()
	t.readFixedCharges()
	t.readSurcharges()
	t.readSeries()
	t.readExclusions()
	t.readCatalogues()
	t.readManifest()
	t.reportUnused()
	t.readAmountRounding()
	t.readAuthorization()
	t.buildPlan()
}

func (t *translation) records(name string) []record {
	sheet, found := t.sheet[name]
	if !found {
		return nil
	}
	spec, _, _ := sheetByName(name)
	return t.d.records(sheet, spec)
}

func all(results ...bool) bool {
	for _, result := range results {
		if !result {
			return false
		}
	}
	return true
}

// ---- card 表的方案头 ----

func (t *translation) readHeader() {
	d := t.d
	scopeOK := false
	if scope := t.card["scope"]; d.required(scope) {
		value, err := domain.NewPricingScopeID(scope.text)
		if err != nil {
			d.at(scope, ports.TemplateCellInvalid, "计价范围「%s」立不住：%v", scope.text, err)
		} else {
			t.scope, scopeOK = value, true
		}
	}
	direction, directionOK := d.code(t.card["direction"], directionCodes)
	purpose, purposeOK := d.code(t.card["purpose"], purposeCodes)
	aggregation, aggregationOK := d.code(t.card["aggregation"], aggregationCodes)
	t.direction, t.purpose, t.aggregation = domain.PricingDirection(direction), domain.PricingPurpose(purpose), aggregation
	baseOK := false
	if base := t.card["baseChargeCode"]; d.required(base) {
		value, err := domain.NewChargeCode(base.text)
		if err != nil {
			d.at(base, ports.TemplateCellInvalid, "费用代码「%s」立不住：%v", base.text, err)
		} else {
			t.baseCharge, baseOK = value, true
		}
	}
	period, periodOK := t.readPeriod(t.card["periodStartsAt"], t.card["periodEndsAt"], "方案生效区间")
	t.period = period
	t.headerOK = all(scopeOK, directionOK, purposeOK, aggregationOK, baseOK, periodOK)
}

// readPeriod 读起止两格；止点空即不设止点。
func (t *translation) readPeriod(start, end field, what string) (domain.EffectivePeriod, bool) {
	startsAt, startOK := t.d.timestamp(start)
	endOK := true
	var endsAt time.Time
	if end.state == fieldPresent {
		endsAt, endOK = t.d.timestamp(end)
	} else if end.state == fieldBad {
		endOK = false
	}
	if !startOK || !endOK {
		return domain.EffectivePeriod{}, false
	}
	period, err := domain.NewEffectivePeriod(startsAt, endsAt)
	if err != nil {
		t.d.rejected(start, what, err)
		return domain.EffectivePeriod{}, false
	}
	return period, true
}

// ---- 价表 ----

func (t *translation) readTables() {
	d := t.d
	for _, rec := range t.records(sheetTables) {
		id := rec.get("tableId")
		if id.state != fieldPresent {
			continue
		}
		if _, duplicated := t.tables[id.text]; duplicated {
			d.at(id, ports.TemplateIDDuplicated, "价表 %s 已在本表出现过", id.text)
			continue
		}
		table := &rateTable{header: rec, id: id.text, rowsOK: true}
		t.tables[id.text] = table
		t.tableOrder = append(t.tableOrder, table)

		refOK := false
		if version := rec.get("tableVersion"); d.required(version) {
			ref, err := domain.NewVersionReferenceIdentity(domain.ArtifactRateTable, id.text, version.text)
			if err != nil {
				d.at(version, ports.TemplateCellInvalid, "价表引用立不住：%v", err)
			} else {
				table.ref, refOK = ref, true
			}
		}
		family, familyOK := d.code(rec.get("family"), familyCodes)
		table.family = family
		currencyOK := false
		if currency := rec.get("currency"); d.required(currency) {
			value, err := domain.NewCurrency(currency.text)
			if err != nil {
				d.at(currency, ports.TemplateCellInvalid, "币种「%s」立不住：%v", currency.text, err)
			} else {
				table.currency, currencyOK = value, true
			}
		}
		unit, unitOK := t.weightUnit(rec.get("weightUnit"))
		table.unit = unit
		period, periodOK := t.readPeriod(rec.get("periodStartsAt"), rec.get("periodEndsAt"), "价表生效区间")
		table.period = period
		table.headerOK = all(refOK, familyOK, currencyOK, unitOK, periodOK)
	}
	t.readRates(sheetRatesWeightZone, "WEIGHT_ZONE", t.weightZoneEntry)
	t.readRates(sheetRatesFirstContinue, "FIRST_CONTINUE", t.firstContinueEntry)
	t.readRates(sheetRatesUnitPrice, "UNIT_PRICE", t.unitPriceEntry)
	for _, table := range t.tableOrder {
		t.buildTable(table)
	}
}

func (t *translation) weightUnit(f field) (domain.WeightUnit, bool) {
	code, ok := t.d.code(f, weightUnitCodes)
	if !ok {
		return "", false
	}
	unit, err := domain.NewWeightUnit(code)
	if err != nil {
		t.d.at(f, ports.TemplateCellInvalid, "重量单位「%s」立不住：%v", code, err)
		return "", false
	}
	return unit, true
}

func (t *translation) lengthUnit(f field) (domain.LengthUnit, bool) {
	code, ok := t.d.code(f, lengthUnitCodes)
	if !ok {
		return "", false
	}
	unit, err := domain.NewLengthUnit(code)
	if err != nil {
		t.d.at(f, ports.TemplateCellInvalid, "长度单位「%s」立不住：%v", code, err)
		return "", false
	}
	return unit, true
}

// tableOf 解析一格 tableId；tables 表结构有问题时不报解析不到。
func (t *translation) tableOf(f field) (*rateTable, bool) {
	if !t.d.required(f) {
		return nil, false
	}
	table, found := t.tables[f.text]
	if !found {
		if !t.d.broken[sheetTables] {
			t.d.at(f, ports.TemplateReferenceUnresolved, "tables 表里没有价表 %s", f.text)
		}
		return nil, false
	}
	return table, true
}

func (t *translation) readRates(sheet, family string, entry func(*rateTable, record) bool) {
	for _, rec := range t.records(sheet) {
		reference := rec.get("tableId")
		table, found := t.tableOf(reference)
		if !found {
			continue
		}
		if table.family != family {
			if table.family != "" {
				t.d.at(reference, ports.TemplateReferenceUnresolved, "价表 %s 是 %s 族，它的行应放在对应的表里", table.id, table.family)
			}
			table.rowsOK = false
			continue
		}
		if !entry(table, rec) {
			table.rowsOK = false
		}
	}
}

// tableWeight 与 tableMoney 按表头的单位与币种立值；表头有问题时只核格的写法，不立值。
func (t *translation) tableWeight(table *rateTable, f field) (domain.Weight, bool) {
	value, ok := t.d.decimal(f)
	if !ok || !table.headerOK {
		return domain.Weight{}, false
	}
	weight, err := domain.NewWeight(value, table.unit)
	if err != nil {
		t.d.at(f, ports.TemplateCellInvalid, "「%s」立不成重量：%v", f.text, err)
		return domain.Weight{}, false
	}
	return weight, true
}

func (t *translation) tableMoney(table *rateTable, f field) (domain.Money, bool) {
	value, ok := t.d.decimal(f)
	if !ok || !table.headerOK {
		return domain.Money{}, false
	}
	money, err := domain.NewMoney(value, table.currency)
	if err != nil {
		t.d.at(f, ports.TemplateCellInvalid, "「%s」立不成金额：%v", f.text, err)
		return domain.Money{}, false
	}
	return money, true
}

func (t *translation) entryID(f field) (domain.RateEntryID, bool) {
	if !t.d.required(f) {
		return domain.RateEntryID{}, false
	}
	id, err := domain.NewRateEntryID(f.text)
	if err != nil {
		t.d.at(f, ports.TemplateCellInvalid, "价表行标识「%s」立不住：%v", f.text, err)
		return domain.RateEntryID{}, false
	}
	return id, true
}

func (t *translation) weightZoneEntry(table *rateTable, rec record) bool {
	id, idOK := t.entryID(rec.get("entryId"))
	zone := rec.get("zone")
	zoneOK := t.d.required(zone)
	minimum, minimumOK := t.tableWeight(table, rec.get("minimum"))
	maximumField := rec.get("maximum")
	var maximum domain.Weight
	maximumOK := maximumField.state != fieldBad
	if maximumField.state == fieldPresent {
		maximum, maximumOK = t.tableWeight(table, maximumField)
	}
	amount, amountOK := t.tableMoney(table, rec.get("amount"))
	if !all(idOK, zoneOK, minimumOK, maximumOK, amountOK) {
		return false
	}
	var entry domain.RateEntry
	var err error
	if maximumField.state == fieldPresent {
		entry, err = domain.NewRateEntry(id, zone.text, minimum, maximum, amount)
	} else {
		entry, err = domain.NewOpenEndedRateEntry(id, zone.text, minimum, amount)
	}
	if err != nil {
		t.d.rejected(rec.get("entryId"), "价表行", err)
		return false
	}
	table.entries = append(table.entries, entry)
	return true
}

func (t *translation) firstContinueEntry(table *rateTable, rec record) bool {
	id, idOK := t.entryID(rec.get("entryId"))
	zone := rec.get("zone")
	zoneOK := t.d.required(zone)
	firstWeight, firstWeightOK := t.tableWeight(table, rec.get("firstWeight"))
	firstAmount, firstAmountOK := t.tableMoney(table, rec.get("firstAmount"))
	step, stepOK := t.tableWeight(table, rec.get("step"))
	stepAmount, stepAmountOK := t.tableMoney(table, rec.get("stepAmount"))
	if !all(idOK, zoneOK, firstWeightOK, firstAmountOK, stepOK, stepAmountOK) {
		return false
	}
	rate, err := domain.NewFirstContinueRate(id, zone.text, firstWeight, firstAmount, step, stepAmount)
	if err != nil {
		t.d.rejected(rec.get("entryId"), "首续重价表行", err)
		return false
	}
	table.firstContinue = append(table.firstContinue, rate)
	return true
}

func (t *translation) unitPriceEntry(table *rateTable, rec record) bool {
	id, idOK := t.entryID(rec.get("entryId"))
	zone := rec.get("zone")
	zoneOK := t.d.required(zone)
	amount, amountOK := t.tableMoney(table, rec.get("amountPerUnit"))
	if !all(idOK, zoneOK, amountOK) {
		return false
	}
	rate, err := domain.NewUnitPriceRate(id, zone.text, amount)
	if err != nil {
		t.d.rejected(rec.get("entryId"), "单价价表行", err)
		return false
	}
	table.unitPrice = append(table.unitPrice, rate)
	return true
}

func (t *translation) buildTable(table *rateTable) {
	if !table.headerOK || !table.rowsOK {
		return
	}
	var version domain.RateTableVersion
	var err error
	switch table.family {
	case "WEIGHT_ZONE":
		version, err = domain.NewRateTableVersion(table.ref, domain.RateTableFamilyWeightZone, table.currency, table.unit, table.period, table.entries)
	case "FIRST_CONTINUE":
		version, err = domain.NewFirstContinueRateTable(table.ref, table.currency, table.unit, table.period, table.firstContinue)
	case "UNIT_PRICE":
		version, err = domain.NewUnitPriceRateTable(table.ref, table.currency, table.unit, table.period, table.unitPrice)
	}
	if err != nil {
		t.d.rejected(table.header.get("tableId"), "价表 "+table.id, err)
		return
	}
	table.version, table.built = version, true
}

func (t *translation) resolveMainTable() {
	table, found := t.tableOf(t.card["rateTableId"])
	if !found {
		return
	}
	table.used = true
	t.main = table
}

// mainMoney 与 mainWeight 按主价表的币种与单位立值：固定费用、附加费金额、金额取整与条件最低
// 计价重量都以主价表为准（构造门本来就要求一致，逐行再写一遍只多一处写错的机会）。
func (t *translation) mainMoney(f field) (domain.Money, bool) {
	value, ok := t.d.decimal(f)
	if !ok || t.main == nil || !t.main.headerOK {
		return domain.Money{}, false
	}
	money, err := domain.NewMoney(value, t.main.currency)
	if err != nil {
		t.d.at(f, ports.TemplateCellInvalid, "「%s」立不成金额：%v", f.text, err)
		return domain.Money{}, false
	}
	return money, true
}

func (t *translation) mainWeight(f field) (domain.Weight, bool) {
	value, ok := t.d.decimal(f)
	if !ok || t.main == nil || !t.main.headerOK {
		return domain.Weight{}, false
	}
	weight, err := domain.NewWeight(value, t.main.unit)
	if err != nil {
		t.d.at(f, ports.TemplateCellInvalid, "「%s」立不成重量：%v", f.text, err)
		return domain.Weight{}, false
	}
	return weight, true
}

// ---- 计重策略 ----

func (t *translation) readWeight() {
	d := t.d
	refOK := false
	var ref domain.VersionReference
	id, version := t.card["weightPolicyId"], t.card["weightPolicyVersion"]
	if d.required(id) && d.required(version) {
		value, err := domain.NewVersionReferenceIdentity(domain.ArtifactWeightPolicy, id.text, version.text)
		if err != nil {
			d.at(id, ports.TemplateCellInvalid, "计重策略引用立不住：%v", err)
		} else {
			ref, refOK = value, true
		}
	}
	method, methodOK := d.code(t.card["weightMethod"], weightMethodCodes)

	divisorField, unitField := t.card["volumetricDivisor"], t.card["volumetricLengthUnit"]
	volumetricDeclared := divisorField.state == fieldPresent || unitField.state == fieldPresent
	pairOK := divisorField.state != fieldBad && unitField.state != fieldBad
	var divisor domain.Decimal
	var lengthUnit domain.LengthUnit
	if volumetricDeclared {
		var divisorOK, unitOK bool
		divisor, divisorOK = d.decimal(divisorField)
		lengthUnit, unitOK = t.lengthUnit(unitField)
		pairOK = pairOK && divisorOK && unitOK
	}

	chargeable, volumetric, roundingOK := t.readRounding(volumetricDeclared)
	if !all(refOK, methodOK, pairOK, roundingOK) {
		return
	}
	var factor *domain.VolumetricFactor
	if volumetricDeclared {
		value, err := domain.NewVolumetricFactor(divisor, lengthUnit, volumetric)
		if err != nil {
			d.rejected(divisorField, "体积重因子", err)
			return
		}
		factor = &value
	}
	policy, err := domain.NewPricingWeightPolicy(ref, domain.PricingWeightMethod(method), chargeable, factor)
	if err != nil {
		d.rejected(t.card["weightMethod"], "计重策略", err)
		return
	}
	t.weight, t.weightOK = policy, true
}

// readRounding 读 weight_rounding：同一 policy 的各行按行序组成分段。
func (t *translation) readRounding(volumetricDeclared bool) (domain.WeightRoundingPolicy, domain.WeightRoundingPolicy, bool) {
	d := t.d
	if _, found := t.sheet[sheetWeightRounding]; !found {
		return domain.WeightRoundingPolicy{}, domain.WeightRoundingPolicy{}, false
	}
	groups := map[string][]record{}
	groupsOK := map[string]bool{"CHARGEABLE": true, "VOLUMETRIC": true}
	records := t.records(sheetWeightRounding)
	if d.broken[sheetWeightRounding] {
		return domain.WeightRoundingPolicy{}, domain.WeightRoundingPolicy{}, false
	}
	for _, rec := range records {
		policy, ok := d.code(rec.get("policy"), roundingPolicyCodes)
		if !ok {
			groupsOK["CHARGEABLE"], groupsOK["VOLUMETRIC"] = false, false
			continue
		}
		groups[policy] = append(groups[policy], rec)
	}
	chargeable, chargeableOK := t.roundingPolicy(groups["CHARGEABLE"], groupsOK["CHARGEABLE"])
	if len(groups["CHARGEABLE"]) == 0 {
		d.report(sheetWeightRounding, 0, "policy", ports.TemplateCellRequired, "缺 CHARGEABLE（计费重）的进位段")
		chargeableOK = false
	}
	volumetricOK := true
	var volumetric domain.WeightRoundingPolicy
	switch {
	case volumetricDeclared && len(groups["VOLUMETRIC"]) == 0:
		d.report(sheetWeightRounding, 0, "policy", ports.TemplateCellRequired, "card 声明了体积重因子，缺 VOLUMETRIC 的进位段")
		volumetricOK = false
	case !volumetricDeclared && len(groups["VOLUMETRIC"]) > 0:
		for _, rec := range groups["VOLUMETRIC"] {
			d.at(rec.get("policy"), ports.TemplateCellNotApplicable, "card 没有体积重因子，用不到 VOLUMETRIC 的进位段")
		}
		volumetricOK = false
	case volumetricDeclared:
		volumetric, volumetricOK = t.roundingPolicy(groups["VOLUMETRIC"], groupsOK["VOLUMETRIC"])
	}
	return chargeable, volumetric, chargeableOK && volumetricOK
}

func (t *translation) roundingPolicy(rows []record, groupOK bool) (domain.WeightRoundingPolicy, bool) {
	d := t.d
	if len(rows) == 0 {
		return domain.WeightRoundingPolicy{}, false
	}
	segments := make([]domain.WeightRoundingSegment, 0, len(rows))
	ok := groupOK
	for _, rec := range rows {
		mode, modeOK := d.code(rec.get("mode"), roundingModeCodes)
		unit, unitOK := t.weightUnit(rec.get("unit"))
		increment, incrementOK := d.decimal(rec.get("increment"))
		maximumField := rec.get("maximum")
		var maximum domain.Decimal
		maximumOK := maximumField.state != fieldBad
		if maximumField.state == fieldPresent {
			maximum, maximumOK = d.decimal(maximumField)
		}
		if !all(modeOK, unitOK, incrementOK, maximumOK) {
			ok = false
			continue
		}
		incrementWeight, err := domain.NewWeight(increment, unit)
		if err != nil {
			d.at(rec.get("increment"), ports.TemplateCellInvalid, "「%s」立不成重量：%v", rec.get("increment").text, err)
			ok = false
			continue
		}
		var segment domain.WeightRoundingSegment
		if maximumField.state == fieldPresent {
			maximumWeight, err := domain.NewWeight(maximum, unit)
			if err != nil {
				d.at(maximumField, ports.TemplateCellInvalid, "「%s」立不成重量：%v", maximumField.text, err)
				ok = false
				continue
			}
			segment, err = domain.NewWeightRoundingSegment(domain.RoundingMode(mode), incrementWeight, maximumWeight)
			if err != nil {
				d.rejected(rec.get("policy"), "进位段", err)
				ok = false
				continue
			}
		} else {
			segment, err = domain.NewOpenEndedWeightRoundingSegment(domain.RoundingMode(mode), incrementWeight)
			if err != nil {
				d.rejected(rec.get("policy"), "进位段", err)
				ok = false
				continue
			}
		}
		segments = append(segments, segment)
	}
	if !ok {
		return domain.WeightRoundingPolicy{}, false
	}
	policy, err := domain.NewSegmentedWeightRoundingPolicy(segments)
	if err != nil {
		d.rejected(rows[0].get("policy"), "进位分段", err)
		return domain.WeightRoundingPolicy{}, false
	}
	return policy, true
}

// ---- 费用依赖 ----

func (t *translation) chargeCodes(f field) ([]domain.ChargeCode, bool) {
	items, ok := t.d.list(f, func(item string) error {
		_, err := domain.NewChargeCode(item)
		return err
	})
	if !ok {
		return nil, false
	}
	codes := make([]domain.ChargeCode, 0, len(items))
	for _, item := range items {
		code, _ := domain.NewChargeCode(item)
		codes = append(codes, code)
	}
	return codes, true
}

func (t *translation) chargeCode(f field) (domain.ChargeCode, bool) {
	if !t.d.required(f) {
		return domain.ChargeCode{}, false
	}
	code, err := domain.NewChargeCode(f.text)
	if err != nil {
		t.d.at(f, ports.TemplateCellInvalid, "费用代码「%s」立不住：%v", f.text, err)
		return domain.ChargeCode{}, false
	}
	return code, true
}

func (t *translation) readDependencies() {
	d := t.d
	t.dependencyOK = !d.broken[sheetChargeDependencies]
	for _, rec := range t.records(sheetChargeDependencies) {
		id := rec.get("dependencyId")
		if id.state != fieldPresent {
			t.dependencyOK = false
			continue
		}
		if t.dependencyIDs[id.text] {
			d.at(id, ports.TemplateIDDuplicated, "费用依赖 %s 已在本表出现过", id.text)
			t.dependencyOK = false
			continue
		}
		t.dependencyIDs[id.text] = true
		code, codeOK := t.chargeCode(rec.get("chargeCode"))
		composition, compositionOK := d.code(rec.get("composition"), compositionCodes)
		includesField, excludesField := rec.get("includes"), rec.get("excludes")
		var includes, excludes []domain.ChargeCode
		includesOK, excludesOK := true, excludesField.state != fieldBad
		if excludesField.state == fieldPresent {
			excludes, excludesOK = t.chargeCodes(excludesField)
		}
		switch composition {
		case "LISTED_CHARGES":
			includes, includesOK = t.chargeCodes(includesField)
		case "ALL_CHARGES":
			includesOK = d.mustBeBlank(includesField, "ALL_CHARGES 不列 includes")
		}
		if !all(codeOK, compositionOK, includesOK, excludesOK) {
			t.dependencyOK = false
			continue
		}
		var dependency domain.ChargeDependency
		var err error
		if composition == "ALL_CHARGES" {
			dependency, err = domain.NewAllChargesDependency(id.text, code, excludes)
		} else {
			dependency, err = domain.NewListedChargeDependency(id.text, code, includes, excludes)
		}
		if err != nil {
			d.rejected(id, "费用依赖", err)
			t.dependencyOK = false
			continue
		}
		t.dependencies = append(t.dependencies, dependency)
	}
}

// ---- 条件树与计算树 ----

func (t *translation) readForest(name string) forest {
	d := t.d
	trees := forest{sheet: name, nodes: map[string]*treeNode{}, used: map[string]bool{}}
	records := t.records(name)
	if _, found := t.sheet[name]; !found || d.broken[name] {
		return trees
	}
	trees.usable = true
	for _, rec := range records {
		id := rec.get("nodeId")
		if id.state != fieldPresent {
			continue
		}
		if _, duplicated := trees.nodes[id.text]; duplicated {
			d.at(id, ports.TemplateIDDuplicated, "节点 %s 已在本表出现过", id.text)
			continue
		}
		node := &treeNode{id: id.text, rec: rec}
		if parent := rec.get("parentId"); parent.state == fieldPresent {
			node.parent = parent.text
		}
		trees.nodes[node.id] = node
		trees.order = append(trees.order, node)
	}
	for _, node := range trees.order {
		if node.parent == "" {
			continue
		}
		parent, found := trees.nodes[node.parent]
		if !found {
			d.at(node.rec.get("parentId"), ports.TemplateTreeInvalid, "父节点 %s 不在本表", node.parent)
			continue
		}
		parent.children = append(parent.children, node)
	}
	for _, node := range trees.order {
		seen := map[string]bool{}
		for cursor := node; cursor != nil && cursor.parent != ""; cursor = trees.nodes[cursor.parent] {
			if seen[cursor.id] {
				d.at(node.rec.get("parentId"), ports.TemplateTreeInvalid, "节点 %s 的祖先链成环", node.id)
				break
			}
			seen[cursor.id] = true
		}
	}
	return trees
}

// root 解析一格指向树根的引用。树所在的表结构有问题时不报解析不到。
func (t *translation) root(trees *forest, f field) (*treeNode, bool) {
	if !t.d.required(f) || !trees.usable {
		return nil, false
	}
	node, found := trees.nodes[f.text]
	if !found {
		t.d.at(f, ports.TemplateReferenceUnresolved, "%s 表里没有节点 %s", trees.sheet, f.text)
		return nil, false
	}
	if node.parent != "" {
		t.d.at(f, ports.TemplateReferenceUnresolved, "节点 %s 不是根（它的父节点是 %s）", f.text, node.parent)
		return nil, false
	}
	trees.used[node.id] = true
	return node, true
}

func (t *translation) conditionRoot(f field) (domain.TriggerCondition, bool) {
	node, found := t.root(&t.conditions, f)
	if !found {
		return domain.TriggerCondition{}, false
	}
	return t.condition(node)
}

func (t *translation) condition(node *treeNode) (domain.TriggerCondition, bool) {
	if built, done := t.conditionValues[node.id]; done {
		return built.value, built.ok
	}
	value, ok := t.conditionOf(node)
	t.conditionValues[node.id] = builtCondition{value: value, ok: ok}
	return value, ok
}

var predicateColumns = []string{"source", "operator", "threshold", "unit", "category"}

func (t *translation) conditionOf(node *treeNode) (domain.TriggerCondition, bool) {
	d := t.d
	rec := node.rec
	kind, ok := d.code(rec.get("kind"), triggerKindCodes)
	if !ok {
		return domain.TriggerCondition{}, false
	}
	if kind == "PREDICATE" {
		if len(node.children) > 0 {
			d.at(rec.get("kind"), ports.TemplateTreeInvalid, "PREDICATE 节点不能有子节点")
			return domain.TriggerCondition{}, false
		}
		predicate, ok := t.predicate(rec)
		if !ok {
			return domain.TriggerCondition{}, false
		}
		trigger, err := domain.NewTrigger(predicate)
		if err != nil {
			d.rejected(rec.get("nodeId"), "判定条件", err)
			return domain.TriggerCondition{}, false
		}
		return trigger, true
	}
	blanks := true
	for _, key := range predicateColumns {
		if !d.mustBeBlank(rec.get(key), kind+" 节点不填判定格") {
			blanks = false
		}
	}
	if len(node.children) == 0 {
		d.at(rec.get("kind"), ports.TemplateTreeInvalid, "%s 节点至少要一个子节点", kind)
		return domain.TriggerCondition{}, false
	}
	operands := make([]domain.TriggerCondition, 0, len(node.children))
	childrenOK := true
	for _, child := range node.children {
		operand, ok := t.condition(child)
		if !ok {
			childrenOK = false
			continue
		}
		operands = append(operands, operand)
	}
	if !blanks || !childrenOK {
		return domain.TriggerCondition{}, false
	}
	var trigger domain.TriggerCondition
	var err error
	if kind == "ALL_OF" {
		trigger, err = domain.NewAllOfTrigger(operands...)
	} else {
		trigger, err = domain.NewAnyOfTrigger(operands...)
	}
	if err != nil {
		d.rejected(rec.get("nodeId"), "组合条件", err)
		return domain.TriggerCondition{}, false
	}
	return trigger, true
}

func (t *translation) predicate(rec record) (domain.FeatureCondition, bool) {
	d := t.d
	source, ok := d.code(rec.get("source"), featureSourceCodes)
	if !ok {
		return domain.FeatureCondition{}, false
	}
	feature := domain.FeatureSource(source)
	switch source {
	case "ZONE", "ADDRESS_TYPE", "SERVICE_OPTION":
		operator := rec.get("operator")
		operatorOK := operator.state != fieldBad
		if operator.state == fieldPresent && operator.text != "EQ" {
			d.at(operator, ports.TemplateCellInvalid, "类别类来源只按等于判定，operator 留空或写 EQ")
			operatorOK = false
		}
		thresholdOK := d.mustBeBlank(rec.get("threshold"), "类别类来源不填阈值")
		unitOK := d.mustBeBlank(rec.get("unit"), "类别类来源不填单位")
		category := rec.get("category")
		categoryOK := d.required(category)
		if !all(operatorOK, thresholdOK, unitOK, categoryOK) {
			return domain.FeatureCondition{}, false
		}
		condition, err := domain.NewCategoryFeatureCondition(feature, domain.CategoryValue(category.text))
		if err != nil {
			d.rejected(category, "类别判定", err)
			return domain.FeatureCondition{}, false
		}
		return condition, true
	}
	categoryOK := d.mustBeBlank(rec.get("category"), "长度、体积、重量类来源不填类别")
	operator, operatorOK := d.code(rec.get("operator"), operatorCodes)
	threshold, thresholdOK := d.decimal(rec.get("threshold"))
	comparison := domain.ComparisonOperator(operator)
	unitField := rec.get("unit")
	var condition domain.FeatureCondition
	var err error
	switch source {
	case "ACTUAL_WEIGHT", "VOLUMETRIC_WEIGHT", "CHARGEABLE_WEIGHT":
		unit, unitOK := t.weightUnit(unitField)
		if !all(categoryOK, operatorOK, thresholdOK, unitOK) {
			return domain.FeatureCondition{}, false
		}
		weight, weightErr := domain.NewWeight(threshold, unit)
		if weightErr != nil {
			d.at(rec.get("threshold"), ports.TemplateCellInvalid, "阈值立不成重量：%v", weightErr)
			return domain.FeatureCondition{}, false
		}
		condition, err = domain.NewWeightFeatureCondition(feature, comparison, weight)
	case "VOLUME":
		unit, unitOK := t.lengthUnit(unitField)
		if !all(categoryOK, operatorOK, thresholdOK, unitOK) {
			return domain.FeatureCondition{}, false
		}
		volume, volumeErr := domain.NewVolume(threshold, unit)
		if volumeErr != nil {
			d.at(rec.get("threshold"), ports.TemplateCellInvalid, "阈值立不成体积：%v", volumeErr)
			return domain.FeatureCondition{}, false
		}
		condition, err = domain.NewVolumeFeatureCondition(feature, comparison, volume)
	default:
		unit, unitOK := t.lengthUnit(unitField)
		if !all(categoryOK, operatorOK, thresholdOK, unitOK) {
			return domain.FeatureCondition{}, false
		}
		length, lengthErr := domain.NewLength(threshold, unit)
		if lengthErr != nil {
			d.at(rec.get("threshold"), ports.TemplateCellInvalid, "阈值立不成长度：%v", lengthErr)
			return domain.FeatureCondition{}, false
		}
		condition, err = domain.NewLengthFeatureCondition(feature, comparison, length)
	}
	if err != nil {
		d.rejected(rec.get("nodeId"), "特征判定", err)
		return domain.FeatureCondition{}, false
	}
	return condition, true
}

// markLookupTables 先把计算树里 TABLE_LOOKUP 行指到的表记为用到：未被引用的根也照样算，
// 否则一个没人指的计算根会让它查的表再报一遍「没人用」。
func (t *translation) markLookupTables() {
	for _, node := range t.calculations.order {
		method, lookup := node.rec.get("method"), node.rec.get("tableId")
		if method.state == fieldPresent && method.text == "TABLE_LOOKUP" && lookup.state == fieldPresent {
			if table, found := t.tables[lookup.text]; found {
				table.used = true
			}
		}
	}
}

func (t *translation) calculationRoot(f field) (domain.SurchargeCalculation, bool) {
	node, found := t.root(&t.calculations, f)
	if !found {
		return domain.SurchargeCalculation{}, false
	}
	return t.calculation(node)
}

func (t *translation) calculation(node *treeNode) (domain.SurchargeCalculation, bool) {
	if built, done := t.calculationValues[node.id]; done {
		return built.value, built.ok
	}
	value, ok := t.calculationOf(node)
	t.calculationValues[node.id] = builtCalculation{value: value, ok: ok}
	return value, ok
}

var calculationValueColumns = []string{"amount", "tableId", "percentage", "seriesKind", "seriesFactor", "basisDependencyId", "seriesId", "outOfWindow"}

var calculationUses = map[string][]string{
	"FIXED_AMOUNT":     {"amount"},
	"TABLE_LOOKUP":     {"tableId"},
	"PERCENT_OF_BASIS": {"percentage", "seriesKind", "seriesFactor", "basisDependencyId"},
	"GREATER_OF":       {},
	"SERIES_AMOUNT":    {"seriesId", "outOfWindow"},
}

func (t *translation) calculationOf(node *treeNode) (domain.SurchargeCalculation, bool) {
	d := t.d
	rec := node.rec
	method, ok := d.code(rec.get("method"), methodCodes)
	if !ok {
		return domain.SurchargeCalculation{}, false
	}
	if method != "GREATER_OF" && len(node.children) > 0 {
		d.at(rec.get("method"), ports.TemplateTreeInvalid, "只有 GREATER_OF 能当父节点")
		return domain.SurchargeCalculation{}, false
	}
	blanks := true
	for _, key := range calculationValueColumns {
		if contains(calculationUses[method], key) {
			continue
		}
		if !d.mustBeBlank(rec.get(key), method+" 不用这一格") {
			blanks = false
		}
	}
	var calculation domain.SurchargeCalculation
	var err error
	switch method {
	case "FIXED_AMOUNT":
		amount, amountOK := t.mainMoney(rec.get("amount"))
		if !blanks || !amountOK {
			return domain.SurchargeCalculation{}, false
		}
		calculation, err = domain.NewFixedAmountSurcharge(amount)
	case "TABLE_LOOKUP":
		table, found := t.tableOf(rec.get("tableId"))
		if !blanks || !found || !table.built {
			return domain.SurchargeCalculation{}, false
		}
		calculation, err = domain.NewTableLookupSurcharge(table.version)
	case "PERCENT_OF_BASIS":
		return t.percentOfBasis(rec, blanks)
	case "GREATER_OF":
		if len(node.children) != 2 {
			d.at(rec.get("method"), ports.TemplateTreeInvalid, "GREATER_OF 要恰好两个子节点，现有 %d 个", len(node.children))
			return domain.SurchargeCalculation{}, false
		}
		first, firstOK := t.calculation(node.children[0])
		second, secondOK := t.calculation(node.children[1])
		if !blanks || !firstOK || !secondOK {
			return domain.SurchargeCalculation{}, false
		}
		calculation, err = domain.NewGreaterOfSurcharge(first, second)
	case "SERIES_AMOUNT":
		series := rec.get("seriesId")
		seriesOK := d.required(series)
		behaviour, behaviourOK := d.code(rec.get("outOfWindow"), outOfWindowCodes)
		if !all(blanks, seriesOK, behaviourOK) {
			return domain.SurchargeCalculation{}, false
		}
		calculation, err = domain.NewSeriesAmountSurcharge(series.text, domain.OutOfWindowBehaviour(behaviour))
	}
	if err != nil {
		d.rejected(rec.get("nodeId"), "计算方式", err)
		return domain.SurchargeCalculation{}, false
	}
	return calculation, true
}

func (t *translation) percentOfBasis(rec record, blanks bool) (domain.SurchargeCalculation, bool) {
	d := t.d
	basis := rec.get("basisDependencyId")
	basisOK := d.required(basis)
	if basisOK && !t.dependencyIDs[basis.text] {
		if !d.broken[sheetChargeDependencies] {
			d.at(basis, ports.TemplateReferenceUnresolved, "charge_dependencies 表里没有费用依赖 %s", basis.text)
		}
		basisOK = false
	}
	percentage, kind, factor := rec.get("percentage"), rec.get("seriesKind"), rec.get("seriesFactor")
	seriesDeclared := kind.state != fieldAbsent || factor.state != fieldAbsent
	var calculation domain.SurchargeCalculation
	var err error
	switch {
	case percentage.state != fieldAbsent && seriesDeclared:
		d.mustBeBlank(kind, "已填 percentage，不再填序列费率")
		d.mustBeBlank(factor, "已填 percentage，不再填序列费率")
		return domain.SurchargeCalculation{}, false
	case percentage.state != fieldAbsent:
		value, ok := d.decimal(percentage)
		if !all(blanks, basisOK, ok) {
			return domain.SurchargeCalculation{}, false
		}
		calculation, err = domain.NewPercentOfBasisSurcharge(value, basis.text)
	case seriesDeclared:
		code, kindOK := d.code(kind, seriesKindCodes)
		value, factorOK := d.decimal(factor)
		if !all(blanks, basisOK, kindOK, factorOK) {
			return domain.SurchargeCalculation{}, false
		}
		calculation, err = domain.NewSeriesRateSurcharge(domain.ReferenceSeriesKind(code), value, basis.text)
	default:
		d.at(percentage, ports.TemplateCellRequired, "PERCENT_OF_BASIS 要 percentage，或 seriesKind 与 seriesFactor，二选一")
		return domain.SurchargeCalculation{}, false
	}
	if err != nil {
		d.rejected(rec.get("nodeId"), "按基数百分比", err)
		return domain.SurchargeCalculation{}, false
	}
	return calculation, true
}

func contains(items []string, item string) bool {
	for _, candidate := range items {
		if candidate == item {
			return true
		}
	}
	return false
}

// ---- 固定费用与附加费 ----

func (t *translation) chargeUnit(f field) (bool, bool) {
	switch f.state {
	case fieldAbsent:
		return false, true
	case fieldBad:
		return false, false
	}
	code, ok := t.d.code(f, chargeUnitCodes)
	return code == "PER_PIECE", ok
}

func (t *translation) readFixedCharges() {
	d := t.d
	t.rulesOK = !d.broken[sheetFixedCharges]
	for _, rec := range t.records(sheetFixedCharges) {
		id, description := rec.get("ruleId"), rec.get("description")
		idOK, descriptionOK := d.required(id), d.required(description)
		code, codeOK := t.chargeCode(rec.get("chargeCode"))
		effect, effectOK := d.code(rec.get("effect"), effectCodes)
		amount, amountOK := t.mainMoney(rec.get("amount"))
		order, orderOK := d.integer(rec.get("order"))
		perPiece, unitOK := t.chargeUnit(rec.get("unit"))
		if !all(idOK, descriptionOK, codeOK, effectOK, amountOK, orderOK, unitOK) {
			t.rulesOK = false
			continue
		}
		rule, err := domain.NewFixedChargeRule(id.text, code, description.text, domain.ChargeEffect(effect), amount, order)
		if err == nil && perPiece {
			rule, err = rule.PerPiece()
		}
		if err != nil {
			d.rejected(id, "固定费用", err)
			t.rulesOK = false
			continue
		}
		t.rules = append(t.rules, rule)
	}
}

func (t *translation) readSurcharges() {
	d := t.d
	t.surchargesOK = !d.broken[sheetSurcharges]
	for _, rec := range t.records(sheetSurcharges) {
		// 引用先解析：即使本行别的格有问题，指到的根也算用到，免得再报一遍「没人用」。
		condition, conditionOK := t.conditionRoot(rec.get("conditionId"))
		calculation, calculationOK := t.calculationRoot(rec.get("calculationId"))
		minimumID, minimumCondition, minimumWeight := rec.get("minimumWeightId"), rec.get("minimumWeightConditionId"), rec.get("minimumWeight")
		minimumDeclared := minimumID.state != fieldAbsent || minimumCondition.state != fieldAbsent || minimumWeight.state != fieldAbsent
		var minimumValue domain.TriggerCondition
		var minimum domain.Weight
		minimumOK := true
		if minimumDeclared {
			var conditionFound, weightOK bool
			minimumValue, conditionFound = t.conditionRoot(minimumCondition)
			minimum, weightOK = t.mainWeight(minimumWeight)
			minimumOK = all(d.required(minimumID), conditionFound, weightOK)
		}

		id, description := rec.get("ruleId"), rec.get("description")
		idOK, descriptionOK := d.required(id), d.required(description)
		code, codeOK := t.chargeCode(rec.get("chargeCode"))
		effect, effectOK := d.code(rec.get("effect"), effectCodes)
		perPiece, unitOK := t.chargeUnit(rec.get("unit"))

		exclusivity := rec.get("exclusivity")
		group, priorityField := rec.get("exclusivityGroup"), rec.get("priority")
		stance, stanceOK := "", exclusivity.state != fieldBad
		var priority int
		if exclusivity.state == fieldPresent {
			stance, stanceOK = d.code(exclusivity, exclusivityCodes)
		}
		switch stance {
		case "GROUPED":
			var priorityOK bool
			groupOK := d.required(group)
			priority, priorityOK = d.integer(priorityField)
			stanceOK = stanceOK && groupOK && priorityOK
		default:
			reason := "未声明互斥立场时不填"
			if stance == "STANDALONE" {
				reason = "STANDALONE 不填"
			}
			groupBlank := d.mustBeBlank(group, reason)
			priorityBlank := d.mustBeBlank(priorityField, reason)
			stanceOK = stanceOK && groupBlank && priorityBlank
		}
		if !all(conditionOK, calculationOK, minimumOK, idOK, descriptionOK, codeOK, effectOK, unitOK, stanceOK) {
			t.surchargesOK = false
			continue
		}
		rule, err := domain.NewSurchargeRule(id.text, code, description.text, domain.ChargeEffect(effect), condition, calculation)
		if err == nil && perPiece {
			rule, err = rule.PerPiece()
		}
		if err == nil {
			switch stance {
			case "STANDALONE":
				rule, err = rule.Standalone()
			case "GROUPED":
				rule, err = rule.InExclusivityGroup(group.text, priority)
			}
		}
		if err == nil && minimumDeclared {
			var conditional domain.ConditionalMinimumWeight
			conditional, err = domain.NewConditionalMinimumWeight(minimumID.text, minimumValue, minimum)
			if err == nil {
				rule, err = rule.WithConditionalMinimumWeight(conditional)
			}
		}
		if err != nil {
			d.rejected(id, "附加费", err)
			t.surchargesOK = false
			continue
		}
		t.surcharges = append(t.surcharges, rule)
	}
}

// ---- 其余四张 ----

func (t *translation) readSeries() {
	d := t.d
	t.seriesOK = !d.broken[sheetReferenceSeries]
	for _, rec := range t.records(sheetReferenceSeries) {
		kind, kindOK := d.code(rec.get("kind"), seriesKindCodes)
		series := rec.get("seriesId")
		if !all(kindOK, d.required(series)) {
			t.seriesOK = false
			continue
		}
		binding, err := domain.NewReferenceSeriesBinding(domain.ReferenceSeriesKind(kind), series.text)
		if err != nil {
			d.rejected(series, "参考序列绑定", err)
			t.seriesOK = false
			continue
		}
		t.series = append(t.series, binding)
	}
}

func (t *translation) readExclusions() {
	d := t.d
	t.exclusionsOK = !d.broken[sheetExclusions]
	for _, rec := range t.records(sheetExclusions) {
		condition, conditionOK := t.conditionRoot(rec.get("conditionId"))
		id, clause := rec.get("exclusionId"), rec.get("clause")
		if !all(conditionOK, d.required(id), d.required(clause)) {
			t.exclusionsOK = false
			continue
		}
		rule, err := domain.NewExclusionRule(id.text, clause.text, condition)
		if err != nil {
			d.rejected(id, "排除条款", err)
			t.exclusionsOK = false
			continue
		}
		t.exclusions = append(t.exclusions, rule)
	}
}

func (t *translation) readCatalogues() {
	d := t.d
	t.cataloguesOK = !d.broken[sheetReferenceCatalogues]
	for _, rec := range t.records(sheetReferenceCatalogues) {
		kind, kindOK := d.code(rec.get("kind"), catalogueKindCodes)
		catalogue := rec.get("catalogueId")
		if !all(kindOK, d.required(catalogue)) {
			t.cataloguesOK = false
			continue
		}
		link, err := domain.NewReferenceCatalogueLink(domain.CatalogueKind(kind), catalogue.text)
		if err != nil {
			d.rejected(catalogue, "参考目录绑定", err)
			t.cataloguesOK = false
			continue
		}
		t.catalogues = append(t.catalogues, link)
	}
}

func (t *translation) readManifest() {
	d := t.d
	t.manifestOK = !d.broken[sheetManifestDependencies]
	for _, rec := range t.records(sheetManifestDependencies) {
		kind, kindOK := d.code(rec.get("kind"), artifactKindCodes)
		id, version := rec.get("id"), rec.get("version")
		if !all(kindOK, d.required(id), d.required(version)) {
			t.manifestOK = false
			continue
		}
		reference, err := domain.NewVersionReferenceIdentity(domain.ArtifactKind(kind), id.text, version.text)
		if err != nil {
			d.at(id, ports.TemplateCellInvalid, "版本引用立不住：%v", err)
			t.manifestOK = false
			continue
		}
		t.manifest = append(t.manifest, reference)
	}
}

func (t *translation) reportUnused() {
	for _, table := range t.tableOrder {
		if !table.used {
			t.d.at(table.header.get("tableId"), ports.TemplateReferenceUnused, "价表 %s 既不是主价表，也没有被 TABLE_LOOKUP 指到", table.id)
		}
	}
	for _, node := range t.conditions.order {
		if node.parent == "" && !t.conditions.used[node.id] {
			t.d.at(node.rec.get("nodeId"), ports.TemplateReferenceUnused, "根节点 %s 没有被附加费、条件最低计价重量或排除条款指到", node.id)
		}
	}
	for _, node := range t.calculations.order {
		if node.parent == "" && !t.calculations.used[node.id] {
			t.d.at(node.rec.get("nodeId"), ports.TemplateReferenceUnused, "根节点 %s 没有被附加费指到", node.id)
		}
	}
}

// ---- card 表的登记件与金额取整 ----

func (t *translation) readAmountRounding() {
	d := t.d
	mode, increment, points := t.card["amountRoundingMode"], t.card["amountRoundingIncrement"], t.card["amountRoundingPoints"]
	if mode.state == fieldAbsent && increment.state == fieldAbsent && points.state == fieldAbsent {
		t.amountRoundingOK = true
		return
	}
	code, modeOK := d.code(mode, roundingModeCodes)
	money, incrementOK := t.mainMoney(increment)
	items, pointsOK := d.list(points, oneOf(roundingPointCodes))
	if !all(modeOK, incrementOK, pointsOK) {
		return
	}
	applied := make([]domain.AmountRoundingPoint, 0, len(items))
	for _, item := range items {
		applied = append(applied, domain.AmountRoundingPoint(item))
	}
	policy, err := domain.NewAmountRoundingPolicy(domain.RoundingMode(code), money, applied)
	if err != nil {
		d.rejected(mode, "金额取整", err)
		return
	}
	t.amountRounding, t.amountRoundingOK = &policy, true
}

func (t *translation) readAuthorization() {
	id, version := t.card["directionAuthorizationId"], t.card["directionAuthorizationVersion"]
	if !t.d.required(id) || !t.d.required(version) {
		return
	}
	reference, err := domain.NewVersionReferenceIdentity(domain.ArtifactCommercialAuthorization, id.text, version.text)
	if err != nil {
		t.d.at(id, ports.TemplateCellInvalid, "方向授权引用立不住：%v", err)
		return
	}
	t.authorization, t.authorizationOK = reference, true
}

// ---- 方案 ----

func (t *translation) buildPlan() {
	if t.main == nil || !t.main.built {
		return
	}
	if !all(t.headerOK, t.weightOK, t.dependencyOK, t.rulesOK, t.surchargesOK, t.seriesOK, t.exclusionsOK, t.cataloguesOK, t.manifestOK, t.amountRoundingOK, t.authorizationOK) {
		return
	}
	planLevel := field{sheet: sheetCard}
	structures, err := domain.NewPricingPlanStructures(t.surcharges, t.dependencies, t.series)
	if err == nil && len(t.exclusions) > 0 {
		structures, err = structures.WithExclusionRules(t.exclusions...)
	}
	if err == nil && t.amountRounding != nil {
		structures, err = structures.WithAmountRounding(*t.amountRounding)
	}
	if err == nil && len(t.catalogues) > 0 {
		structures, err = structures.WithReferenceCatalogues(t.catalogues...)
	}
	if err != nil {
		t.d.rejected(planLevel, "方案结构件", err)
		return
	}
	var plan domain.PricingPlanVersion
	if t.aggregation == "PER_PACKAGE" {
		plan, err = domain.NewPricingPlanVersion(t.ref, t.scope, t.direction, t.purpose, t.baseCharge, t.period,
			t.main.version, t.weight, t.rules, structures, t.manifest...)
	} else {
		plan, err = domain.NewAggregatePricingPlanVersion(domain.AggregationMode(t.aggregation), t.ref, t.scope, t.direction,
			t.purpose, t.baseCharge, t.period, t.main.version, t.weight, t.rules, structures, t.manifest...)
	}
	if err != nil {
		t.d.rejected(planLevel, "定价方案", err)
		return
	}
	t.plan = &plan
}
