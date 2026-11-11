package pricecardtemplate

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件证 PPT-3：card 表的 serviceProducts 字段把售价卡的适用服务产品带进方案（ADR-0378 决定一、四）。两种载体读出同一张卡；
// 不写这一行与写空值都是不声明；旧版文件里填了它说清是版本问题；不是 SELL 的卡填了它报不适用。

func namedSellCard(products string) filled {
	card := sellCard()
	card.card["templateVersion"] = templateVersionV3
	card.card[cardFieldServiceProducts] = products
	return card
}

func TestAPPT3SellCardCarriesItsServiceProductsThroughBothCarriers(t *testing.T) {
	card := namedSellCard("SYN-PROD-FBA;SYN-PROD-EXPRESS")
	fromFile := read(t, card.bytes(t))
	plan := validated(t, fromFile)
	products := plan.ServiceProducts()
	if len(products) != 2 || products[0].String() != "SYN-PROD-EXPRESS" || products[1].String() != "SYN-PROD-FBA" {
		t.Fatalf("适用产品 = %v，要排序后的两个", products)
	}
	if plan.CanonicalizationVersion() != "PPC-7" || fromFile.TemplateVersion != templateVersionV3 {
		t.Fatalf("规范化版本 %s、模板版本 %s，要 PPC-7、PPT-3", plan.CanonicalizationVersion(), fromFile.TemplateVersion)
	}
	fromForm := readWorkbookJSON(t, card.workbookJSON(t))
	if readingKey(t, fromForm) != readingKey(t, fromFile) {
		t.Fatalf("表单与文件读出的不同：\n%s\n%s", readingKey(t, fromForm), readingKey(t, fromFile))
	}
	unnamed := validated(t, read(t, sellCard().bytes(t)))
	if unnamed.ContentDigest() == plan.ContentDigest() {
		t.Fatal("声明了产品的卡与不声明的卡摘要相同")
	}
}

func TestAPPT3WorkbookWithoutTheServiceProductsRowReadsAsAnUnnamedCard(t *testing.T) {
	plain := sellCard()
	plain.card["templateVersion"] = templateVersionV2
	reference := validated(t, read(t, plain.bytes(t)))

	// 管理台只在 SELL 卡选了产品时才写这一行，其余照最新版本号发：整行缺席要读通，与写空值同答。
	var withoutRow []map[string]any
	for _, entry := range cardFields {
		if entry.key == cardFieldServiceProducts {
			continue
		}
		var value any
		if text := plain.card[entry.key]; text != "" {
			value = text
		}
		if entry.key == "templateVersion" {
			value = templateVersionV3
		}
		withoutRow = append(withoutRow, map[string]any{"field": entry.key, "value": value})
	}
	raw := replaceCardRows(t, plain.workbookJSON(t), withoutRow)
	for name, reading := range map[string]ports.PriceCardTemplateReading{
		"整行缺席":  readWorkbookJSON(t, raw),
		"写了空值":  read(t, namedSellCard("").bytes(t)),
		"空值表单":  readWorkbookJSON(t, namedSellCard("").workbookJSON(t)),
		"PPT-2": read(t, plain.bytes(t)),
	} {
		plan := validated(t, reading)
		if len(plan.ServiceProducts()) != 0 || plan.ContentDigest() != reference.ContentDigest() || plan.CanonicalizationVersion() != "PPC-5" {
			t.Fatalf("%s：产品 %v、摘要 %s（%s），要不声明、与 PPT-2 读出的同摘要 PPC-5", name, plan.ServiceProducts(), plan.ContentDigest(), plan.CanonicalizationVersion())
		}
	}
}

func TestServiceProductsInAnOlderTemplateAreAVersionProblem(t *testing.T) {
	card := namedSellCard("SYN-PROD-FBA")
	card.card["templateVersion"] = templateVersionV2
	var rows []map[string]any
	for _, entry := range cardFields {
		var value any
		if text := card.card[entry.key]; text != "" {
			value = text
		}
		rows = append(rows, map[string]any{"field": entry.key, "value": value})
	}
	reading := readWorkbookJSON(t, replaceCardRows(t, card.workbookJSON(t), rows))
	found := false
	for _, problem := range reading.Problems {
		if problem.Column == cardFieldServiceProducts && problem.Code == ports.TemplateFieldUnknown &&
			strings.Contains(problem.Message, "自模板 PPT-3 起才可用") && strings.Contains(problem.Message, "换用 PPT-3 模板") {
			found = true
		}
	}
	if !found || reading.Content != nil {
		t.Fatalf("PPT-2 文件填了适用产品，要点名「自模板 PPT-3 起才可用」且不出方案：%s", describe(reading.Problems))
	}
}

func TestServiceProductsAreRefusedOnANonSellCardAndMalformedListsAreNamed(t *testing.T) {
	cost := costCard()
	cost.card["templateVersion"] = templateVersionV3
	cost.card[cardFieldServiceProducts] = "SYN-PROD-FBA"
	reading := read(t, cost.bytes(t))
	if reading.Content != nil || !hasProblem(reading.Problems, at{sheetCard, cardRow(t, cardFieldServiceProducts), cardFieldServiceProducts, ports.TemplateCellNotApplicable}) {
		t.Fatalf("BUY 卡填了适用产品：%s", describe(reading.Problems))
	}

	for name, products := range map[string]string{
		"重复":  "SYN-PROD-FBA;SYN-PROD-FBA",
		"空元素": "SYN-PROD-FBA;;SYN-PROD-EXPRESS",
	} {
		reading := read(t, namedSellCard(products).bytes(t))
		if reading.Content != nil || !hasProblem(reading.Problems, at{sheetCard, cardRow(t, cardFieldServiceProducts), cardFieldServiceProducts, ports.TemplateCellInvalid}) {
			t.Fatalf("%s：%s", name, describe(reading.Problems))
		}
	}
}

func TestTheBlankPPT3TemplateListsTheServiceProductsField(t *testing.T) {
	var buffer bytes.Buffer
	if err := WriteTemplate(&buffer); err != nil {
		t.Fatal(err)
	}
	book := mustReadBook(t, buffer.Bytes())
	var cardKeys, readme []string
	for _, sheet := range book.Sheets {
		for _, row := range sheet.Rows {
			if len(row.Cells) == 0 {
				continue
			}
			switch sheet.Name {
			case sheetCard:
				cardKeys = append(cardKeys, row.Cells[0].Text)
			case sheetReadme:
				for _, cell := range row.Cells {
					readme = append(readme, cell.Text)
				}
			}
		}
	}
	if !strings.Contains(strings.Join(cardKeys, ","), cardFieldServiceProducts) {
		t.Fatalf("空白模板的 card 表没有 %s：%v", cardFieldServiceProducts, cardKeys)
	}
	if text := strings.Join(readme, "\n"); !strings.Contains(text, "（PPT-3 起）") || !strings.Contains(text, "价卡导入模板 PPT-3") {
		t.Fatal("说明表没点出 serviceProducts 自 PPT-3 起")
	}
}

// replaceCardRows 把工作簿 JSON 的 card 表换成给定的行，外层版本跟着 card 表里写的版本走（两处不一致整份不收），其余原样。
func replaceCardRows(t *testing.T, raw []byte, rows []map[string]any) []byte {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	document["sheets"].(map[string]any)[sheetCard] = rows
	for _, row := range rows {
		if row["field"] == "templateVersion" {
			document["templateVersion"] = row["value"]
		}
	}
	out, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// 管理台按规格把 card 字段逐行列全、空的写 null：PPT-3 的 BUY 卡带一行空的 serviceProducts，照常读通、不声明。
func TestAPPT3BuyCardWithANullServiceProductsRowReadsFine(t *testing.T) {
	cost := costCard()
	cost.card["templateVersion"] = templateVersionV3
	for name, reading := range map[string]ports.PriceCardTemplateReading{
		"表单": readWorkbookJSON(t, cost.workbookJSON(t)),
		"文件": read(t, cost.bytes(t)),
	} {
		plan := validated(t, reading)
		if len(plan.ServiceProducts()) != 0 || plan.CanonicalizationVersion() != "PPC-5" || reading.TemplateVersion != templateVersionV3 {
			t.Fatalf("%s：产品 %v、%s、%s", name, plan.ServiceProducts(), plan.CanonicalizationVersion(), reading.TemplateVersion)
		}
	}
	if !bytes.Contains(cost.workbookJSON(t), []byte(`"field": "serviceProducts"`)) {
		t.Fatal("夹具没写出 serviceProducts 那一行，这一格没证到")
	}
}
