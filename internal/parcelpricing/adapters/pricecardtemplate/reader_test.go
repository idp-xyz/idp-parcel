package pricecardtemplate

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/spreadsheet"
)

func read(t *testing.T, raw []byte) ports.PriceCardTemplateReading {
	t.Helper()
	return Reader{}.ReadPriceCardTemplate(raw)
}

func validated(t *testing.T, reading ports.PriceCardTemplateReading) domain.PricingPlanVersion {
	t.Helper()
	if !reading.Accepted || reading.Content == nil || len(reading.Problems) != 0 {
		t.Fatalf("want 已校验, got accepted=%v content=%v problems=%s", reading.Accepted, reading.Content != nil, describe(reading.Problems))
	}
	return reading.Content.Plan
}

func describe(problems []ports.PriceCardTemplateProblem) string {
	lines := make([]string, 0, len(problems))
	for _, problem := range problems {
		lines = append(lines, problem.Sheet+"!"+strconv.Itoa(problem.Row)+":"+problem.Column+" "+string(problem.Code)+" "+problem.Message)
	}
	return "\n  " + strings.Join(lines, "\n  ")
}

type at struct {
	sheet  string
	row    int
	column string
	code   ports.PriceCardTemplateProblemCode
}

func hasProblem(problems []ports.PriceCardTemplateProblem, want at) bool {
	for _, problem := range problems {
		if problem.Sheet == want.sheet && problem.Row == want.row && problem.Column == want.column && problem.Code == want.code {
			return true
		}
	}
	return false
}

// cardRow 是 card 表里某字段所在的行号：列键行是第 1 行，字段按模板行序从第 2 行起。
func cardRow(t *testing.T, key string) int {
	t.Helper()
	_, index, ok := cardField(key)
	if !ok {
		t.Fatalf("没有字段 %s", key)
	}
	return index + 2
}

// 规范第八节的验收判据：两张种子卡经模板导入，内容摘要与入库种子快照经登记门重建出的逐字节
// 相等——两条构造路径互相独立，相等即翻译没有漏格、没有改值。方向授权与方案身份一并对上。
func TestSeedCardsImportToTheSeedDigests(t *testing.T) {
	cases := map[string]filled{
		"price-card-cn-sg-cost.json": costCard(),
		"price-card-cn-sg.json":      sellCard(),
	}
	for file, card := range cases {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "scripts", "demo-seeds", "data", "pricing", file))
		if err != nil {
			t.Fatal(err)
		}
		seed, err := domain.RehydratePriceCardRegistration(raw)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		reading := read(t, card.bytes(t))
		plan := validated(t, reading)
		if plan.ContentDigest() != seed.Plan().ContentDigest() {
			t.Errorf("%s: 导入摘要 %s，种子 %s", file, plan.ContentDigest(), seed.Plan().ContentDigest())
		}
		if !reading.Plan.SameIdentity(seed.Plan().Reference()) || !reading.Content.DirectionAuthorization.SameIdentity(seed.DirectionAuthorization()) {
			t.Errorf("%s: 方案身份或方向授权对不上", file)
		}
		if reading.TemplateVersion != TemplateVersion {
			t.Errorf("%s: template version %q", file, reading.TemplateVersion)
		}
	}
}

// 模板的每一张表都用上：与领域构造函数直接立出的同一张方案逐字节同摘要。
func TestFullCardImportsToTheDirectlyConstructedPlan(t *testing.T) {
	plan := validated(t, read(t, fullCard().bytes(t)))
	if want := fullPlan(t); plan.ContentDigest() != want.ContentDigest() {
		t.Fatalf("导入摘要 %s，直接构造 %s", plan.ContentDigest(), want.ContentDigest())
	}
}

// 行序不进摘要：领域会把价表行、规则、依赖、绑定、条款与目录链接排好序（规范第四节末）。
func TestRowOrderOutsideTreesDoesNotChangeTheDigest(t *testing.T) {
	card := fullCard()
	for _, sheet := range []string{sheetRatesWeightZone, sheetFixedCharges, sheetSurcharges, sheetChargeDependencies, sheetReferenceSeries} {
		lines := card.data[sheet]
		for left, right := 0, len(lines)-1; left < right; left, right = left+1, right-1 {
			lines[left], lines[right] = lines[right], lines[left]
		}
	}
	plan := validated(t, read(t, card.bytes(t)))
	if want := fullPlan(t); plan.ContentDigest() != want.ContentDigest() {
		t.Fatalf("行序改了摘要：%s ≠ %s", plan.ContentDigest(), want.ContentDigest())
	}
}

// 存的字面量恰是显示的那个数的数值格照收，结果与文本格相同；带浮点尾数的不收。
func TestNumericCellsAreReadByTheirStoredLiteral(t *testing.T) {
	book := costCard().book(t)
	setCellIn(&book, sheetRatesWeightZone, 2, "amount", spreadsheet.Cell{Kind: spreadsheet.CellNumber, Text: "30"})
	plan := validated(t, decodeWorkbook(book))
	want := validated(t, read(t, costCard().bytes(t)))
	if plan.ContentDigest() != want.ContentDigest() {
		t.Fatal("精确的数值格与文本格读出了不同的方案")
	}

	book = costCard().book(t)
	setCellIn(&book, sheetRatesWeightZone, 2, "amount", spreadsheet.Cell{Kind: spreadsheet.CellNumber, Text: "2.2999999999999998"})
	reading := decodeWorkbook(book)
	if reading.Content != nil || !hasProblem(reading.Problems, at{sheetRatesWeightZone, 2, "amount", ports.TemplateCellNumericNotExact}) {
		t.Fatalf("problems = %s", describe(reading.Problems))
	}
}

// 空白模板的结构与读取的封闭集合严丝合缝：只填方案身份，读出来没有任何表、列、字段层面的问题。
func TestBlankTemplateStructureMatchesTheReader(t *testing.T) {
	var buffer bytes.Buffer
	if err := WriteTemplate(&buffer); err != nil {
		t.Fatal(err)
	}
	book, err := spreadsheet.Read(buffer.Bytes(), spreadsheet.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	for _, sheet := range book.Sheets {
		if sheet.Name == sheetCard {
			for index := range sheet.Rows {
				if len(sheet.Rows[index].Cells) == 0 {
					continue
				}
				switch sheet.Rows[index].Cells[0].Text {
				case "planId":
					sheet.Rows[index].Cells = append(sheet.Rows[index].Cells, spreadsheet.Cell{Column: 1, Kind: spreadsheet.CellText, Text: "SYN-PLAN"})
				case "planVersion":
					sheet.Rows[index].Cells = append(sheet.Rows[index].Cells, spreadsheet.Cell{Column: 1, Kind: spreadsheet.CellText, Text: "v1"})
				}
			}
		}
	}
	reading := decodeWorkbook(book)
	if !reading.Accepted {
		t.Fatalf("blank template rejected: %s", describe(reading.Problems))
	}
	for _, problem := range reading.Problems {
		switch problem.Code {
		case ports.TemplateSheetMissing, ports.TemplateSheetUnknown, ports.TemplateColumnMissing, ports.TemplateColumnUnknown,
			ports.TemplateColumnDuplicated, ports.TemplateFieldMissing, ports.TemplateFieldUnknown, ports.TemplateFieldDuplicated:
			t.Errorf("结构问题：%s", describe([]ports.PriceCardTemplateProblem{problem}))
		}
	}
	if !hasProblem(reading.Problems, at{sheetCard, cardRow(t, "scope"), "scope", ports.TemplateCellRequired}) {
		t.Errorf("空白模板应报必填格为空：%s", describe(reading.Problems))
	}
}

func TestTemplateFileIsDeterministic(t *testing.T) {
	var first, second bytes.Buffer
	if err := WriteTemplate(&first); err != nil {
		t.Fatal(err)
	}
	if err := WriteTemplate(&second); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("两次写出的模板不同")
	}
}

// 整份不收的四格按先后查，撞到第一格就只答这一格（规范第六节）。
func TestWholeFileRejectionsAnswerExactlyOneProblem(t *testing.T) {
	withCard := func(key, text string) []byte {
		card := costCard()
		card.card[key] = text
		return card.bytes(t)
	}
	withoutCard := func() []byte {
		sheets := costCard().sheets(t)
		return writeSheets(t, sheets[1:])
	}
	cases := map[string]struct {
		raw  []byte
		code ports.PriceCardTemplateProblemCode
	}{
		"不是工作簿":    {[]byte("plain text, not a workbook"), ports.TemplateFileNotWorkbook},
		"超过文件上限":   {append(costCard().bytes(t), make([]byte, MaxFileBytes)...), ports.TemplateFileTooLarge},
		"缺 card 表": {withoutCard(), ports.TemplateVersionUnsupported},
		"模板版本不认":   {withCard("templateVersion", "PPT-0"), ports.TemplateVersionUnsupported},
		"模板版本空":    {withCard("templateVersion", ""), ports.TemplateVersionUnsupported},
		"方案身份空":    {withCard("planId", ""), ports.TemplatePlanIdentityUnreadable},
		"方案版本空":    {withCard("planVersion", ""), ports.TemplatePlanIdentityUnreadable},
	}
	for name, testCase := range cases {
		reading := read(t, testCase.raw)
		if reading.Accepted || reading.Content != nil || len(reading.Problems) != 1 || reading.Problems[0].Code != testCase.code {
			t.Errorf("%s: accepted=%v problems=%s", name, reading.Accepted, describe(reading.Problems))
		}
	}
}

// 读得出方案身份之后，任何一格有问题都进草稿带问题：逐条带坐标，不交方案。
func TestProblemsCarryTheirCoordinates(t *testing.T) {
	type mutation func(*filled)
	cases := map[string]struct {
		mutate mutation
		want   at
	}{
		"未知代码": {func(card *filled) { card.card["direction"] = "BUYY" },
			at{sheetCard, 0, "direction", ports.TemplateCellInvalid}},
		"时刻不带时区": {func(card *filled) { card.card["periodStartsAt"] = "2026-01-01" },
			at{sheetCard, 0, "periodStartsAt", ports.TemplateCellInvalid}},
		"科学计数法": {func(card *filled) { card.data[sheetRatesWeightZone][0]["minimum"] = "1e3" },
			at{sheetRatesWeightZone, 2, "minimum", ports.TemplateCellInvalid}},
		"必填格为空": {func(card *filled) { delete(card.data[sheetRatesWeightZone][1], "zone") },
			at{sheetRatesWeightZone, 3, "zone", ports.TemplateCellRequired}},
		"主价表指空": {func(card *filled) { card.card["rateTableId"] = "SYN-NO-SUCH-TABLE" },
			at{sheetCard, 0, "rateTableId", ports.TemplateReferenceUnresolved}},
		"价表没人用": {func(card *filled) {
			card.data[sheetTables] = append(card.data[sheetTables], map[string]string{"tableId": "SYN-SPARE", "tableVersion": "v1",
				"family": "UNIT_PRICE", "currency": "CNY", "weightUnit": "KG", "periodStartsAt": synStart})
			card.data[sheetRatesUnitPrice] = rows{{"tableId": "SYN-SPARE", "entryId": "SYN-SPARE-Z1", "zone": "Z1", "amountPerUnit": "1"}}
		}, at{sheetTables, 3, "tableId", ports.TemplateReferenceUnused}},
		"行放错族": {func(card *filled) {
			card.data[sheetRatesUnitPrice] = rows{{"tableId": "SYN-TABLE-CN-SG-COST-01", "entryId": "SYN-X", "zone": "Z1", "amountPerUnit": "1"}}
		}, at{sheetRatesUnitPrice, 2, "tableId", ports.TemplateReferenceUnresolved}},
		"区间重叠": {func(card *filled) { card.data[sheetRatesWeightZone][1]["minimum"] = "0.5" },
			at{sheetTables, 2, "tableId", ports.TemplateConstructionRejected}},
		"方向与目的不配": {func(card *filled) { card.card["purpose"] = "CUSTOMER_CHARGE" },
			at{sheetCard, 0, "", ports.TemplateConstructionRejected}},
		"缺计费重进位段": {func(card *filled) { card.data[sheetWeightRounding] = nil },
			at{sheetWeightRounding, 0, "policy", ports.TemplateCellRequired}},
		"体积重进位段用不到": {func(card *filled) {
			card.data[sheetWeightRounding] = append(card.data[sheetWeightRounding],
				map[string]string{"policy": "VOLUMETRIC", "mode": "CEILING", "increment": "0.1", "unit": "KG"})
		}, at{sheetWeightRounding, 3, "policy", ports.TemplateCellNotApplicable}},
		"列表重复": {func(card *filled) {
			card.card["amountRoundingMode"], card.card["amountRoundingIncrement"], card.card["amountRoundingPoints"] = "HALF_UP", "0.01", "TOTAL;TOTAL"
		}, at{sheetCard, 0, "amountRoundingPoints", ports.TemplateCellInvalid}},
		"三格缺一": {func(card *filled) { card.card["amountRoundingMode"] = "HALF_UP" },
			at{sheetCard, 0, "amountRoundingIncrement", ports.TemplateCellRequired}},
	}
	for name, testCase := range cases {
		card := costCard()
		testCase.mutate(&card)
		want := testCase.want
		if want.sheet == sheetCard && want.row == 0 && want.column != "" && want.code != ports.TemplateFieldMissing {
			want.row = cardRow(t, want.column)
		}
		reading := read(t, card.bytes(t))
		if !reading.Accepted || reading.Content != nil || !hasProblem(reading.Problems, want) {
			t.Errorf("%s: want %+v, got accepted=%v content=%v problems=%s", name, want, reading.Accepted, reading.Content != nil, describe(reading.Problems))
		}
	}
}

// 条件树、计算树与它们的引用：以全结构卡为底逐项改坏。
func TestTreeAndReferenceProblems(t *testing.T) {
	find := func(card filled, sheet, id string) map[string]string {
		for _, line := range card.data[sheet] {
			if line["nodeId"] == id {
				return line
			}
		}
		t.Fatalf("夹具里没有节点 %s", id)
		return nil
	}
	rowOf := func(card filled, sheet, id string) int {
		for index, line := range card.data[sheet] {
			if line["nodeId"] == id || line["ruleId"] == id {
				return index + 2
			}
		}
		t.Fatalf("夹具里没有 %s", id)
		return 0
	}
	cases := map[string]struct {
		mutate func(filled)
		want   func(filled) at
	}{
		"指到不存在的节点": {func(card filled) { card.data[sheetSurcharges][1]["conditionId"] = "C-NOWHERE" },
			func(card filled) at {
				return at{sheetSurcharges, 3, "conditionId", ports.TemplateReferenceUnresolved}
			}},
		"指到非根": {func(card filled) { card.data[sheetSurcharges][1]["conditionId"] = "C-OVERSIZE-LEN" },
			func(card filled) at {
				return at{sheetSurcharges, 3, "conditionId", ports.TemplateReferenceUnresolved}
			}},
		"根没人指": {func(card filled) { card.data[sheetExclusions] = nil },
			func(card filled) at {
				return at{sheetConditions, rowOf(card, sheetConditions, "C-EXCL"), "nodeId", ports.TemplateReferenceUnused}
			}},
		"父节点不存在": {func(card filled) { find(card, sheetConditions, "C-OVERSIZE-WT")["parentId"] = "C-GHOST" },
			func(card filled) at {
				return at{sheetConditions, rowOf(card, sheetConditions, "C-OVERSIZE-WT"), "parentId", ports.TemplateTreeInvalid}
			}},
		"成环": {func(card filled) { find(card, sheetConditions, "C-OVERSIZE")["parentId"] = "C-OVERSIZE-LEN" },
			func(card filled) at {
				return at{sheetConditions, rowOf(card, sheetConditions, "C-OVERSIZE"), "parentId", ports.TemplateTreeInvalid}
			}},
		"叶子带子节点": {func(card filled) { find(card, sheetConditions, "C-OVERSIZE-VOL")["parentId"] = "C-OVERSIZE-LEN" },
			func(card filled) at {
				return at{sheetConditions, rowOf(card, sheetConditions, "C-OVERSIZE-LEN"), "kind", ports.TemplateTreeInvalid}
			}},
		"取较大值只有一个子节点": {func(card filled) { find(card, sheetCalculations, "K-OVERSIZE-FIXED")["parentId"] = "" },
			func(card filled) at {
				return at{sheetCalculations, rowOf(card, sheetCalculations, "K-OVERSIZE"), "method", ports.TemplateTreeInvalid}
			}},
		"多填的格": {func(card filled) { find(card, sheetCalculations, "K-OVERSIZE-FIXED")["percentage"] = "3" },
			func(card filled) at {
				return at{sheetCalculations, rowOf(card, sheetCalculations, "K-OVERSIZE-FIXED"), "percentage", ports.TemplateCellNotApplicable}
			}},
		"基数依赖不存在": {func(card filled) { find(card, sheetCalculations, "K-FUEL")["basisDependencyId"] = "SYN-NO-DEP" },
			func(card filled) at {
				return at{sheetCalculations, rowOf(card, sheetCalculations, "K-FUEL"), "basisDependencyId", ports.TemplateReferenceUnresolved}
			}},
		"分组缺优先级": {func(card filled) { delete(card.data[sheetSurcharges][0], "priority") },
			func(card filled) at {
				return at{sheetSurcharges, rowOf(card, sheetSurcharges, "SYN-PPT-SUR-OVERSIZE"), "priority", ports.TemplateCellRequired}
			}},
		"类别判定不填阈值": {func(card filled) { find(card, sheetConditions, "C-REMOTE")["threshold"] = "1" },
			func(card filled) at {
				return at{sheetConditions, rowOf(card, sheetConditions, "C-REMOTE"), "threshold", ports.TemplateCellNotApplicable}
			}},
		"节点重复": {func(card filled) {
			card.data[sheetConditions] = append(card.data[sheetConditions], map[string]string{"nodeId": "C-FUEL", "kind": "PREDICATE",
				"source": "ZONE", "category": "Z9"})
		}, func(card filled) at {
			return at{sheetConditions, len(card.data[sheetConditions]) + 1, "nodeId", ports.TemplateIDDuplicated}
		}},
	}
	for name, testCase := range cases {
		card := fullCard().clone()
		testCase.mutate(card)
		want := testCase.want(card)
		reading := read(t, card.bytes(t))
		if !reading.Accepted || reading.Content != nil || !hasProblem(reading.Problems, want) {
			t.Errorf("%s: want %+v, got problems=%s", name, want, describe(reading.Problems))
		}
	}
}

// 表、列、字段层面的问题，以及格类型的问题：改工作簿本身。
func TestStructureAndCellKindProblems(t *testing.T) {
	cases := map[string]struct {
		edit func(*spreadsheet.Workbook)
		want at
	}{
		"缺表": {func(book *spreadsheet.Workbook) { book.Sheets = removeSheet(book.Sheets, sheetExclusions) },
			at{sheetExclusions, 0, "", ports.TemplateSheetMissing}},
		"多出未知表": {func(book *spreadsheet.Workbook) { book.Sheets = append(book.Sheets, spreadsheet.Sheet{Name: "notes"}) },
			at{"notes", 0, "", ports.TemplateSheetUnknown}},
		"缺列": {func(book *spreadsheet.Workbook) { dropColumn(book, sheetTables, "currency") },
			at{sheetTables, 1, "currency", ports.TemplateColumnMissing}},
		"未知列": {func(book *spreadsheet.Workbook) { addHeader(book, sheetRatesWeightZone, "notes") },
			at{sheetRatesWeightZone, 1, "notes", ports.TemplateColumnUnknown}},
		"缺字段": {func(book *spreadsheet.Workbook) { dropCardField(book, "scope") },
			at{sheetCard, 0, "scope", ports.TemplateFieldMissing}},
		"公式格": {func(book *spreadsheet.Workbook) {
			setCellIn(book, sheetRatesWeightZone, 2, "amount", spreadsheet.Cell{Kind: spreadsheet.CellFormula, Text: "30"})
		}, at{sheetRatesWeightZone, 2, "amount", ports.TemplateCellFormula}},
		"错误值": {func(book *spreadsheet.Workbook) {
			setCellIn(book, sheetRatesWeightZone, 2, "amount", spreadsheet.Cell{Kind: spreadsheet.CellError, Text: "#N/A"})
		}, at{sheetRatesWeightZone, 2, "amount", ports.TemplateCellErrorValue}},
		"日期类型": {func(book *spreadsheet.Workbook) {
			setCellIn(book, sheetTables, 2, "periodStartsAt", spreadsheet.Cell{Kind: spreadsheet.CellDate, Text: "2026-01-01T00:00:00"})
		}, at{sheetTables, 2, "periodStartsAt", ports.TemplateCellNotText}},
	}
	for name, testCase := range cases {
		book := costCard().book(t)
		testCase.edit(&book)
		reading := decodeWorkbook(book)
		if !reading.Accepted || reading.Content != nil || !hasProblem(reading.Problems, testCase.want) {
			t.Errorf("%s: want %+v, got problems=%s", name, testCase.want, describe(reading.Problems))
		}
	}

	// 缺列的表记为结构有问题：指进它的引用不再报解析不到，免得连带问题。
	book := costCard().book(t)
	dropColumn(&book, sheetTables, "currency")
	for _, problem := range decodeWorkbook(book).Problems {
		if problem.Code == ports.TemplateReferenceUnresolved {
			t.Errorf("连带问题：%s", describe([]ports.PriceCardTemplateProblem{problem}))
		}
	}
	// 说明表整张跳过。
	book = costCard().book(t)
	book.Sheets = append(book.Sheets, spreadsheet.Sheet{Name: sheetReadme, Rows: []spreadsheet.Row{{Number: 1, Cells: []spreadsheet.Cell{{Kind: spreadsheet.CellText, Text: "随便写"}}}}})
	validated(t, decodeWorkbook(book))
}

// 问题按表序、行号、列序排，与发现的先后无关。
func TestProblemsAreSortedByTemplateOrder(t *testing.T) {
	card := costCard()
	card.data[sheetWeightRounding][0]["mode"] = "UP"
	card.data[sheetRatesWeightZone][2]["amount"] = "x"
	card.data[sheetRatesWeightZone][0]["amount"] = "y"
	card.card["scope"] = " "
	problems := read(t, card.bytes(t)).Problems
	var order []string
	for _, problem := range problems {
		order = append(order, problem.Sheet+"!"+strconv.Itoa(problem.Row)+":"+problem.Column)
	}
	want := []string{
		sheetCard + "!" + strconv.Itoa(cardRow(t, "scope")) + ":scope",
		sheetRatesWeightZone + "!2:amount",
		sheetRatesWeightZone + "!4:amount",
		sheetWeightRounding + "!2:mode",
	}
	if strings.Join(order, " ") != strings.Join(want, " ") {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func removeSheet(sheets []spreadsheet.Sheet, name string) []spreadsheet.Sheet {
	kept := sheets[:0]
	for _, sheet := range sheets {
		if sheet.Name != name {
			kept = append(kept, sheet)
		}
	}
	return kept
}

func sheetIn(book *spreadsheet.Workbook, name string) *spreadsheet.Sheet {
	for index := range book.Sheets {
		if book.Sheets[index].Name == name {
			return &book.Sheets[index]
		}
	}
	return nil
}

func dropColumn(book *spreadsheet.Workbook, sheet, key string) {
	target := sheetIn(book, sheet)
	column := -1
	for _, cell := range target.Rows[0].Cells {
		if cell.Text == key {
			column = cell.Column
		}
	}
	for index := range target.Rows {
		kept := target.Rows[index].Cells[:0]
		for _, cell := range target.Rows[index].Cells {
			if cell.Column != column {
				kept = append(kept, cell)
			}
		}
		target.Rows[index].Cells = kept
	}
}

func addHeader(book *spreadsheet.Workbook, sheet, key string) {
	target := sheetIn(book, sheet)
	last := target.Rows[0].Cells[len(target.Rows[0].Cells)-1].Column
	target.Rows[0].Cells = append(target.Rows[0].Cells, spreadsheet.Cell{Column: last + 1, Kind: spreadsheet.CellText, Text: key})
}

func dropCardField(book *spreadsheet.Workbook, key string) {
	target := sheetIn(book, sheetCard)
	kept := target.Rows[:0]
	for _, row := range target.Rows {
		if len(row.Cells) > 0 && row.Cells[0].Text == key {
			continue
		}
		kept = append(kept, row)
	}
	target.Rows = kept
}

func setCellIn(book *spreadsheet.Workbook, sheet string, row int, key string, cell spreadsheet.Cell) {
	target := sheetIn(book, sheet)
	for _, header := range target.Rows[0].Cells {
		if header.Text == key {
			cell.Column = header.Column
		}
	}
	for rowIndex := range target.Rows {
		if target.Rows[rowIndex].Number != row {
			continue
		}
		for cellIndex := range target.Rows[rowIndex].Cells {
			if target.Rows[rowIndex].Cells[cellIndex].Column == cell.Column {
				target.Rows[rowIndex].Cells[cellIndex] = cell
			}
		}
	}
}
