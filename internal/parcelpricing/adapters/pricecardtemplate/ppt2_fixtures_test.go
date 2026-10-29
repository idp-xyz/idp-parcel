package pricecardtemplate

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/spreadsheet"
)

// updateGolden 重写 testdata 里的 golden 夹具：go test ./internal/parcelpricing/adapters/pricecardtemplate -run Golden -update-golden
var updateGolden = flag.Bool("update-golden", false, "重写 testdata 里的 golden 夹具")

const (
	full03XLSXGolden = "price-card-cn-sg-full-03-PPT-2.xlsx"
	full03JSONGolden = "price-card-cn-sg-full-03-workbook-PPT-2.json"
)

// full03Card 是种子 SYN-PLAN-CN-SG-FULL-03 v1 的模板写法（PPT-2）：用模板把 FULL-03 重新表达一遍，导入要得到同一份计划与
// 同一个内容摘要（见 TestFull03ReExpressedInTheTemplateImportsToTheSeedPlan）。它用到两处新结构：偏远一条规则按档位查价表
// （TABLE_LOOKUP_BY_REMOTE_TIER），以及总额下限 120.99。
func full03Card() filled {
	return filled{
		card: map[string]string{
			"templateVersion": TemplateVersion, "planId": "SYN-PLAN-CN-SG-FULL-03", "planVersion": "v1",
			"scope": "SYN-SCOPE-04", "direction": "SELL", "purpose": "CUSTOMER_CHARGE", "aggregation": "PER_PACKAGE",
			"baseChargeCode": "BASE_FREIGHT", "periodStartsAt": synStart, "rateTableId": "SYN-TABLE-CN-SG-FULL-03",
			"weightPolicyId": "SYN-WEIGHT-CN-SG-FULL-03", "weightPolicyVersion": "v1", "weightMethod": "MAX",
			"volumetricDivisor": "5000", "volumetricLengthUnit": "CM",
			"amountRoundingMode": "HALF_UP", "amountRoundingIncrement": "0.01", "amountRoundingPoints": "PER_LINE;TOTAL",
			"directionAuthorizationId": "SYN-AUTH-PRICE-DIR-01", "directionAuthorizationVersion": "v1",
		},
		data: map[string]rows{
			sheetTables: {
				{"tableId": "SYN-TABLE-CN-SG-FULL-03", "tableVersion": "v1", "family": "FIRST_CONTINUE", "currency": "CNY",
					"weightUnit": "KG", "periodStartsAt": synStart},
				{"tableId": "SYN-TABLE-CN-SG-FULL-03-REMOTE", "tableVersion": "v1", "family": "WEIGHT_ZONE", "currency": "CNY",
					"weightUnit": "KG", "periodStartsAt": synStart},
			},
			sheetRatesFirstContinue: {
				{"tableId": "SYN-TABLE-CN-SG-FULL-03", "entryId": "SYN-RATE-CN-SG-FULL3-Z1", "zone": "Z1",
					"firstWeight": "0.5", "firstAmount": "55", "step": "0.5", "stepAmount": "18"},
				{"tableId": "SYN-TABLE-CN-SG-FULL-03", "entryId": "SYN-RATE-CN-SG-FULL3-Z2", "zone": "Z2",
					"firstWeight": "0.5", "firstAmount": "48", "step": "0.5", "stepAmount": "15"},
			},
			sheetRatesWeightZone: {
				{"tableId": "SYN-TABLE-CN-SG-FULL-03-REMOTE", "entryId": "SYN-RATE-CN-SG-FULL3-REMOTE-T1", "zone": "T1", "minimum": "0", "amount": "12"},
				{"tableId": "SYN-TABLE-CN-SG-FULL-03-REMOTE", "entryId": "SYN-RATE-CN-SG-FULL3-REMOTE-T2", "zone": "T2", "minimum": "0", "amount": "28"},
			},
			sheetWeightRounding: {
				{"policy": "CHARGEABLE", "mode": "CEILING", "increment": "0.5", "unit": "KG"},
				{"policy": "VOLUMETRIC", "mode": "CEILING", "increment": "0.1", "unit": "KG"},
			},
			sheetSurcharges: {
				{"ruleId": "SYN-RULE-FULL3-FUEL", "chargeCode": "FUEL_SURCHARGE", "description": "燃油附加费（合成演示值）", "effect": "ADD",
					"conditionId": "C-FUEL", "calculationId": "K-FUEL", "exclusivity": "STANDALONE"},
				{"ruleId": "SYN-RULE-FULL3-OVERSIZE", "chargeCode": "OVERSIZE_SURCHARGE", "description": "超尺寸费（合成演示值）", "effect": "ADD",
					"conditionId": "C-OVERSIZE", "calculationId": "K-OVERSIZE", "exclusivity": "GROUPED",
					"exclusivityGroup": "SYN-GROUP-FULL3-OVERSIZE-OVERWEIGHT", "priority": "1"},
				{"ruleId": "SYN-RULE-FULL3-OVERWEIGHT", "chargeCode": "OVERWEIGHT_SURCHARGE", "description": "超重费（合成演示值）", "effect": "ADD",
					"conditionId": "C-OVERWEIGHT", "calculationId": "K-OVERWEIGHT", "exclusivity": "GROUPED",
					"exclusivityGroup": "SYN-GROUP-FULL3-OVERSIZE-OVERWEIGHT", "priority": "1"},
				{"ruleId": "SYN-RULE-FULL3-REMOTE", "chargeCode": "REMOTE_AREA_SURCHARGE", "description": "偏远地区费（合成演示值）", "effect": "ADD",
					"conditionId": "C-REMOTE", "calculationId": "K-REMOTE", "exclusivity": "STANDALONE"},
			},
			sheetConditions: {
				{"nodeId": "C-FUEL", "kind": "PREDICATE", "source": "ACTUAL_WEIGHT", "operator": "GT", "threshold": "0", "unit": "KG"},
				{"nodeId": "C-OVERSIZE", "kind": "ANY_OF"},
				{"nodeId": "C-OVERSIZE-SIDE", "parentId": "C-OVERSIZE", "kind": "PREDICATE", "source": "LONGEST_SIDE", "operator": "GT", "threshold": "100", "unit": "CM"},
				{"nodeId": "C-OVERSIZE-GIRTH", "parentId": "C-OVERSIZE", "kind": "PREDICATE", "source": "LENGTH_AND_GIRTH", "operator": "GT", "threshold": "300", "unit": "CM"},
				{"nodeId": "C-OVERWEIGHT", "kind": "PREDICATE", "source": "ACTUAL_WEIGHT", "operator": "GT", "threshold": "20", "unit": "KG"},
				{"nodeId": "C-REMOTE", "kind": "ANY_OF"},
				{"nodeId": "C-REMOTE-T1", "parentId": "C-REMOTE", "kind": "PREDICATE", "source": "ADDRESS_TYPE", "operator": "EQ", "category": "T1"},
				{"nodeId": "C-REMOTE-T2", "parentId": "C-REMOTE", "kind": "PREDICATE", "source": "ADDRESS_TYPE", "operator": "EQ", "category": "T2"},
			},
			sheetCalculations: {
				{"nodeId": "K-FUEL", "method": "PERCENT_OF_BASIS", "seriesKind": "FUEL_RATE", "seriesFactor": "1", "basisDependencyId": "SYN-DEP-FULL3-FUEL"},
				{"nodeId": "K-OVERSIZE", "method": "FIXED_AMOUNT", "amount": "35"},
				{"nodeId": "K-OVERWEIGHT", "method": "FIXED_AMOUNT", "amount": "25"},
				{"nodeId": "K-REMOTE", "method": methodTableLookupByRemoteTier, "tableId": "SYN-TABLE-CN-SG-FULL-03-REMOTE"},
			},
			sheetChargeDependencies: {
				{"dependencyId": "SYN-DEP-FULL3-FUEL", "chargeCode": "FUEL_SURCHARGE", "composition": "LISTED_CHARGES", "includes": "BASE_FREIGHT"},
			},
			sheetTotalBounds:         {{"minimum": "120.99"}},
			sheetReferenceSeries:     {{"kind": "FUEL_RATE", "seriesId": "SYN-SERIES-FUEL-FULL-01"}},
			sheetReferenceCatalogues: {{"kind": "REMOTE_TIER", "catalogueId": "SYN-CAT-REMOTE-TIER-CN-SG"}, {"kind": "ZONE", "catalogueId": "SYN-CAT-ZONE-CN-SG-FULL"}},
		},
	}
}

// asPPT1 把一份卡写成 PPT-1 形状的电子表格字节：不带 total_bounds 表，card 表的版本写 PPT-1。其余一字不动——这正是
// PPT-1 时代的文件长的样子（PPT-2 只增不改），用它证旧版文件在新构建里读出同样的东西。
func (card filled) asPPT1(t *testing.T) []byte {
	t.Helper()
	var kept []spreadsheet.WriteSheet
	for _, sheet := range card.sheets(t) {
		if sheet.Name == sheetTotalBounds {
			continue
		}
		if sheet.Name == sheetCard {
			for index, row := range sheet.Rows {
				if row[0] == "templateVersion" {
					sheet.Rows[index] = []string{"templateVersion", templateVersionV1}
				}
			}
		}
		kept = append(kept, sheet)
	}
	return writeSheets(t, kept)
}

// workbookJSON 把一份卡写成结构化工作簿（JSON）：与 bytes 写出的电子表格逐行对应——card 表每个字段一行（空值写 null），
// 其余各表一行一个对象、空值不写；没有数据行的表不写（缺省即空表）。数据行的数组下标加 2 就是电子表格里的行号。
func (card filled) workbookJSON(t *testing.T) []byte {
	t.Helper()
	sheetsOut := map[string]any{}
	cardRows := make([]any, 0, len(cardFields))
	for _, entry := range cardFields {
		var value any
		if text := card.card[entry.key]; text != "" {
			value = text
		}
		cardRows = append(cardRows, map[string]any{"field": entry.key, "value": value})
	}
	sheetsOut[sheetCard] = cardRows
	for _, spec := range sheets {
		if spec.name == sheetCard || len(card.data[spec.name]) == 0 {
			continue
		}
		out := make([]any, 0, len(card.data[spec.name]))
		for _, line := range card.data[spec.name] {
			row := map[string]any{}
			for key, text := range line {
				if text != "" {
					row[key] = text
				}
			}
			out = append(out, row)
		}
		sheetsOut[spec.name] = out
	}
	version := card.card["templateVersion"]
	if version == "" {
		version = TemplateVersion
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(map[string]any{"templateVersion": version, "sheets": sheetsOut}); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// compactJSON 去掉缩进，给要逐字节比的用例用。
func compactJSON(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, raw); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// seedFull03 读 FULL-03 的种子快照。
func seedFull03(t *testing.T) (raw []byte, registration domain.PriceCardRegistration) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "scripts", "demo-seeds", "data", "pricing", "price-card-cn-sg-full-03.json"))
	if err != nil {
		t.Fatal(err)
	}
	registration, err = domain.RehydratePriceCardRegistration(raw)
	if err != nil {
		t.Fatal(err)
	}
	return raw, registration
}

func mustReadBook(t *testing.T, raw []byte) spreadsheet.Workbook {
	t.Helper()
	book, err := spreadsheet.Read(raw, spreadsheet.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return book
}

// templateReader 是被测的读口；`if` 头里不能直接写 Reader{} 复合字面量，测试里统一用它。
var templateReader Reader
