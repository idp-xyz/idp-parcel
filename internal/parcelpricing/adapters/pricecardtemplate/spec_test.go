package pricecardtemplate

import (
	"sort"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// Covers: `AT-PP-047`，ADR-0349 决定五——模板规格逐项对应读取模板的列定义：表清单与顺序、每张表的列（card 表是字段）、中文名、
// 必填性、说明都取自 sheets 与 cardFields，没有手写的副本；增删一列规格自动跟着变。
func TestSpecMirrorsTheSheetAndColumnDefinitions(t *testing.T) {
	spec := Reader{}.PriceCardTemplateSpec()
	if spec.TemplateVersion != TemplateVersion || strings.Join(spec.AcceptedTemplateVersions, ",") != "PPT-1,PPT-2" {
		t.Fatalf("version = %s accepted = %v", spec.TemplateVersion, spec.AcceptedTemplateVersions)
	}
	if len(spec.Sheets) != len(sheets) {
		t.Fatalf("表数 %d，定义里 %d", len(spec.Sheets), len(sheets))
	}
	for index, definition := range sheets {
		got := spec.Sheets[index]
		entries := definition.columns
		layout := "ROWS"
		if definition.name == sheetCard {
			entries, layout = cardFields, "FIELDS"
		}
		if got.Name != definition.name || got.Title != definition.title || got.Purpose != definition.note ||
			got.Layout != layout || got.Repeatable == definition.single || got.Since != definition.since {
			t.Errorf("表 %d：%+v 对不上定义 %+v", index, got, definition.name)
		}
		if got.Title == "" || got.Purpose == "" {
			t.Errorf("表 %s 缺中文名或用途", definition.name)
		}
		if len(got.Columns) != len(entries) {
			t.Errorf("表 %s：列数 %d，定义里 %d", definition.name, len(got.Columns), len(entries))
			continue
		}
		for position, entry := range entries {
			column := got.Columns[position]
			if column.Key != entry.key || column.Title != entry.title || column.Description != entry.note || column.Requirement != entry.requirement.code() {
				t.Errorf("表 %s 第 %d 列：%+v 对不上 %+v", definition.name, position, column, entry)
			}
			if column.Title == "" || column.Description == "" || (column.Requirement != "always" && column.Requirement != "conditional" && column.Requirement != "optional") {
				t.Errorf("表 %s 的 %s 缺中文名、说明或必填性写法不对：%+v", definition.name, entry.key, column)
			}
		}
	}
	bounds := spec.Sheets[indexOfSheet(t, spec, sheetTotalBounds)]
	if bounds.Repeatable || bounds.Since != TemplateVersion || len(bounds.Columns) != 2 || bounds.Columns[0].Key != "minimum" || bounds.Columns[1].Key != "maximum" {
		t.Errorf("total_bounds = %+v", bounds)
	}
	if card := spec.Sheets[indexOfSheet(t, spec, sheetCard)]; card.Repeatable || card.Layout != "FIELDS" || len(card.Columns) != len(cardFields) {
		t.Errorf("card = %+v", card)
	}
}

func indexOfSheet(t *testing.T, spec ports.PriceCardTemplateSpec, name string) int {
	t.Helper()
	for index, sheet := range spec.Sheets {
		if sheet.Name == name {
			return index
		}
	}
	t.Fatalf("规格里没有表 %s", name)
	return -1
}

// Covers: `AT-PP-047`——封闭取值只有一处定义：规格里的 allowedValues 恰是登记处 closedVocabulary 的取值，登记处的每一项都对到
// 一个真实存在的列，规格里带取值的列也都登记过；新增的计算类型出现在规格里。
func TestSpecAllowedValuesAreTheClosedVocabulary(t *testing.T) {
	spec := Reader{}.PriceCardTemplateSpec()
	seen := map[string]bool{}
	for _, sheet := range spec.Sheets {
		for _, column := range sheet.Columns {
			vocab, closed := closedVocabulary[sheet.Name][column.Key]
			if !closed {
				if len(column.AllowedValues) != 0 {
					t.Errorf("%s.%s 带了取值却没登记", sheet.Name, column.Key)
				}
				continue
			}
			seen[sheet.Name+"."+column.Key] = true
			want := vocab.valuesFor(TemplateVersion)
			if strings.Join(column.AllowedValues, "|") != strings.Join(want, "|") || column.MultiValued != vocab.multi {
				t.Errorf("%s.%s：规格 %v，登记处 %v", sheet.Name, column.Key, column.AllowedValues, want)
			}
		}
	}
	for sheet, columns := range closedVocabulary {
		for key := range columns {
			if !seen[sheet+"."+key] {
				t.Errorf("登记处的 %s.%s 在规格里没有对应的列", sheet, key)
			}
		}
	}
	methods := spec.Sheets[indexOfSheet(t, spec, sheetCalculations)]
	for _, column := range methods.Columns {
		if column.Key == "method" && !contains(column.AllowedValues, methodTableLookupByRemoteTier) {
			t.Errorf("计算类型里该有 %s：%v", methodTableLookupByRemoteTier, column.AllowedValues)
		}
	}
	for _, column := range spec.Sheets[indexOfSheet(t, spec, sheetCard)].Columns {
		if column.Key == "amountRoundingPoints" && (!column.MultiValued || len(column.AllowedValues) != len(roundingPointCodes)) {
			t.Errorf("amountRoundingPoints = %+v", column)
		}
	}
}

// Covers: `AT-PP-047`——读取实际收的取值就是登记处里的那些：对每个封闭取值列塞一个词表之外的词，译码答「不是可取值」并列出的恰是登记处的
// 取值（旧版文件不列自后来版本才有的取值）。登记处里的每一列都要在下表里出现，新加封闭列忘了这里就红。
func TestDecoderReadsClosedColumnsThroughTheRegistry(t *testing.T) {
	withSeries := func() filled {
		card := fullCard().clone()
		card.data[sheetCalculations] = append(card.data[sheetCalculations],
			map[string]string{"nodeId": "K-SERIES", "method": "SERIES_AMOUNT", "seriesId": "S", "outOfWindow": "PENDING"})
		card.data[sheetSurcharges][2]["calculationId"] = "K-SERIES"
		return card
	}
	type target struct {
		sheet, column string
		fixture       func() filled
		row           int
	}
	targets := []target{
		{sheetCard, "direction", costCard, 0}, {sheetCard, "purpose", costCard, 0}, {sheetCard, "aggregation", costCard, 0},
		{sheetCard, "weightMethod", costCard, 0}, {sheetCard, "volumetricLengthUnit", sellCard, 0},
		{sheetCard, "amountRoundingMode", costCard, 0}, {sheetCard, "amountRoundingPoints", costCard, 0},
		{sheetTables, "family", costCard, 0}, {sheetTables, "weightUnit", costCard, 0},
		{sheetWeightRounding, "policy", costCard, 0}, {sheetWeightRounding, "mode", costCard, 0}, {sheetWeightRounding, "unit", costCard, 0},
		{sheetFixedCharges, "effect", sellCard, 0}, {sheetFixedCharges, "unit", sellCard, 0},
		{sheetSurcharges, "effect", fullCard, 0}, {sheetSurcharges, "exclusivity", fullCard, 0}, {sheetSurcharges, "unit", fullCard, 0},
		{sheetConditions, "kind", fullCard, 1}, {sheetConditions, "source", fullCard, 1}, {sheetConditions, "operator", fullCard, 1},
		{sheetConditions, "unit", fullCard, 1},
		{sheetCalculations, "method", fullCard, 1}, {sheetCalculations, "seriesKind", fullCard, 4},
		{sheetCalculations, "outOfWindow", withSeries, 5},
		{sheetChargeDependencies, "composition", fullCard, 0},
		{sheetReferenceSeries, "kind", fullCard, 0}, {sheetReferenceCatalogues, "kind", fullCard, 0},
		{sheetManifestDependencies, "kind", fullCard, 0},
	}
	covered := map[string]bool{}
	for _, target := range targets {
		covered[target.sheet+"."+target.column] = true
		card := target.fixture().clone()
		if target.sheet == sheetCard {
			card.card[target.column] = "ZZZ"
		} else {
			card.data[target.sheet][target.row][target.column] = "ZZZ"
		}
		reading := read(t, card.bytes(t))
		var message string
		for _, problem := range reading.Problems {
			if problem.Sheet == target.sheet && problem.Column == target.column && strings.Contains(problem.Message, "ZZZ") {
				message = problem.Message
			}
		}
		if message == "" {
			t.Errorf("%s.%s：塞词表之外的词却没有问题落在这一格：%s", target.sheet, target.column, describe(reading.Problems))
			continue
		}
		vocab := vocabularyOf(target.sheet, target.column)
		allowed := vocab.valuesFor(TemplateVersion)
		switch {
		case target.sheet == sheetConditions && target.column == "unit":
			// 并集：译码按来源挑重量单位或长度单位，话里列的是其中一类。
			if !strings.Contains(message, strings.Join(weightUnitCodes, "、")) && !strings.Contains(message, strings.Join(lengthUnitCodes, "、")) {
				t.Errorf("%s.%s：%s", target.sheet, target.column, message)
			}
		case !strings.Contains(message, strings.Join(allowed, "、")):
			t.Errorf("%s.%s：话里列的不是登记处的取值 %v：%s", target.sheet, target.column, allowed, message)
		}
	}
	var missing []string
	for sheet, columns := range closedVocabulary {
		for key := range columns {
			if !covered[sheet+"."+key] {
				missing = append(missing, sheet+"."+key)
			}
		}
	}
	sort.Strings(missing)
	if len(missing) != 0 {
		t.Fatalf("登记处里这些封闭列没有在本用例里验过：%v", missing)
	}
}
