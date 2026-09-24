package spreadsheet_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/spreadsheet"
)

// 写出件与读取件互为验证：写出的每一格按行内字符串读回，列头在第 1 行，空串不写格。
func TestWrittenWorkbookReadsBackCellForCell(t *testing.T) {
	sheets := []spreadsheet.WriteSheet{
		{Name: "card", Columns: []spreadsheet.WriteColumn{{Header: "field", Text: true}, {Header: "value", Text: true}},
			Rows: [][]string{{"templateVersion", "PPT-1"}, {"periodEndsAt", ""}, {"note", "a<b & \"c\""}}},
		{Name: "说明", Columns: []spreadsheet.WriteColumn{{Header: "表", Width: 30}}},
	}
	var buffer bytes.Buffer
	if err := spreadsheet.Write(&buffer, sheets); err != nil {
		t.Fatal(err)
	}
	book, err := spreadsheet.Read(buffer.Bytes(), spreadsheet.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(book.Sheets) != 2 || book.Sheets[0].Name != "card" || book.Sheets[1].Name != "说明" {
		t.Fatalf("sheets = %+v", book.Sheets)
	}
	card := book.Sheets[0].Rows
	want := []spreadsheet.Row{
		{Number: 1, Cells: []spreadsheet.Cell{{Column: 0, Kind: spreadsheet.CellText, Text: "field"}, {Column: 1, Kind: spreadsheet.CellText, Text: "value"}}},
		{Number: 2, Cells: []spreadsheet.Cell{{Column: 0, Kind: spreadsheet.CellText, Text: "templateVersion"}, {Column: 1, Kind: spreadsheet.CellText, Text: "PPT-1"}}},
		{Number: 3, Cells: []spreadsheet.Cell{{Column: 0, Kind: spreadsheet.CellText, Text: "periodEndsAt"}}},
		{Number: 4, Cells: []spreadsheet.Cell{{Column: 0, Kind: spreadsheet.CellText, Text: "note"}, {Column: 1, Kind: spreadsheet.CellText, Text: "a<b & \"c\""}}},
	}
	if len(card) != len(want) {
		t.Fatalf("card rows = %+v", card)
	}
	for index := range want {
		if card[index].Number != want[index].Number || len(card[index].Cells) != len(want[index].Cells) {
			t.Fatalf("row %d = %+v, want %+v", index, card[index], want[index])
		}
		for cell := range want[index].Cells {
			if card[index].Cells[cell] != want[index].Cells[cell] {
				t.Fatalf("row %d cell %d = %+v, want %+v", index, cell, card[index].Cells[cell], want[index].Cells[cell])
			}
		}
	}
}

// 入库的模板文件靠逐字节比对防漂，所以同一份输入必须写出同一串字节。
func TestWriteIsDeterministic(t *testing.T) {
	sheets := []spreadsheet.WriteSheet{{Name: "tables", Columns: []spreadsheet.WriteColumn{{Header: "tableId", Text: true}}}}
	var first, second bytes.Buffer
	if err := spreadsheet.Write(&first, sheets); err != nil {
		t.Fatal(err)
	}
	if err := spreadsheet.Write(&second, sheets); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("two writes of the same workbook differ")
	}
}

const (
	mainNamespace = `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"`
	relNamespace  = `xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	relType       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
)

func handMadeWorkbook(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, content := range parts {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func standardParts(sheetXML string) map[string]string {
	return map[string]string{
		"_rels/.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="` + relType + `/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml": `<workbook ` + mainNamespace + ` ` + relNamespace + `><sheets><sheet name="data" sheetId="1" r:id="rId7"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId7" Type="` + relType + `/worksheet" Target="/xl/worksheets/Sheet1.xml"/>` +
			`<Relationship Id="rId8" Type="` + relType + `/sharedStrings" Target="sharedStrings.xml"/></Relationships>`,
		"xl/sharedStrings.xml": `<sst ` + mainNamespace + `>` +
			`<si><t>plain</t></si>` +
			`<si><r><rPr><b/></rPr><t>ri</t></r><r><t xml:space="preserve">ch </t></r></si>` +
			`<si><t>漢字</t><rPh sb="0" eb="2"><t>かんじ</t></rPh><phoneticPr fontId="1"/></si>` +
			`</sst>`,
		"xl/worksheets/sheet1.xml": sheetXML,
	}
}

// 各类格照文件里声明的类型交出，数值格给存储的字面量而不是浮点往返后的数，带公式的格一律
// 标成公式；注音不算格的内容；缺行号、缺格引用时按前一个顺延；空格不交出。
func TestReadReportsEachCellKindWithItsStoredLiteral(t *testing.T) {
	sheet := `<worksheet ` + mainNamespace + `><sheetData>` +
		`<row r="2"><c r="A2" t="s"><v>0</v></c><c r="C2" t="s"><v>1</v></c><c t="s"><v>2</v></c></row>` +
		`<row><c r="A3"><v>5000</v></c><c r="B3" t="n"><v>2.2999999999999998</v></c><c r="C3" s="1"/></row>` +
		`<row r="7"><c r="A7"><f>1+1</f><v>2</v></c><c r="B7" t="str"><f t="shared" si="0"/><v>x</v></c><c r="C7"><f>NOW()</f></c></row>` +
		`<row r="8"><c r="A8" t="b"><v>1</v></c><c r="B8" t="e"><v>#N/A</v></c><c r="C8" t="d"><v>2026-01-01T00:00:00</v></c><c r="AA8" t="inlineStr"><is><r><t>in</t></r><r><t>line</t></r></is></c></row>` +
		`<row r="9"><c r="A9" s="3"/></row>` +
		`</sheetData></worksheet>`
	book, err := spreadsheet.Read(handMadeWorkbook(t, standardParts(sheet)), spreadsheet.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	rows := book.Sheets[0].Rows
	want := []spreadsheet.Row{
		{Number: 2, Cells: []spreadsheet.Cell{{0, spreadsheet.CellText, "plain"}, {2, spreadsheet.CellText, "rich "}, {3, spreadsheet.CellText, "漢字"}}},
		{Number: 3, Cells: []spreadsheet.Cell{{0, spreadsheet.CellNumber, "5000"}, {1, spreadsheet.CellNumber, "2.2999999999999998"}}},
		{Number: 7, Cells: []spreadsheet.Cell{{0, spreadsheet.CellFormula, "2"}, {1, spreadsheet.CellFormula, "x"}, {2, spreadsheet.CellFormula, ""}}},
		{Number: 8, Cells: []spreadsheet.Cell{{0, spreadsheet.CellBoolean, "1"}, {1, spreadsheet.CellError, "#N/A"}, {2, spreadsheet.CellDate, "2026-01-01T00:00:00"}, {26, spreadsheet.CellText, "inline"}}},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v", rows)
	}
	for index := range want {
		if rows[index].Number != want[index].Number || len(rows[index].Cells) != len(want[index].Cells) {
			t.Fatalf("row %d = %+v, want %+v", index, rows[index], want[index])
		}
		for cell := range want[index].Cells {
			if rows[index].Cells[cell] != want[index].Cells[cell] {
				t.Fatalf("row %d cell %d = %+v, want %+v", index, cell, rows[index].Cells[cell], want[index].Cells[cell])
			}
		}
	}
}

func TestReadRejectsWhatIsNotAPlainWorkbook(t *testing.T) {
	strict := standardParts(`<worksheet/>`)
	strict["_rels/.rels"] = `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://purl.oclc.org/ooxml/officeDocument/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`
	missing := standardParts(`<worksheet/>`)
	delete(missing, "xl/worksheets/sheet1.xml")
	cases := map[string][]byte{
		"not a zip (xls, encrypted)": []byte("\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1 compound file"),
		"strict OOXML":               handMadeWorkbook(t, strict),
		"missing worksheet part":     handMadeWorkbook(t, missing),
	}
	for name, raw := range cases {
		if _, err := spreadsheet.Read(raw, spreadsheet.Limits{}); !errors.Is(err, spreadsheet.ErrNotWorkbook) {
			t.Errorf("%s: err = %v, want ErrNotWorkbook", name, err)
		}
	}
}

func TestReadEnforcesItsLimits(t *testing.T) {
	sheet := `<worksheet ` + mainNamespace + `><sheetData><row r="1"><c t="s"><v>0</v></c></row><row r="2"><c t="s"><v>0</v></c></row></sheetData></worksheet>`
	raw := handMadeWorkbook(t, standardParts(sheet))
	cases := map[string]spreadsheet.Limits{
		"file bytes":     {MaxFileBytes: int64(len(raw)) - 1},
		"part bytes":     {MaxPartBytes: 64},
		"rows per sheet": {MaxRowsPerSheet: 1},
	}
	for name, limits := range cases {
		if _, err := spreadsheet.Read(raw, limits); !errors.Is(err, spreadsheet.ErrTooLarge) {
			t.Errorf("%s: err = %v, want ErrTooLarge", name, err)
		}
	}
	if _, err := spreadsheet.Read(raw, spreadsheet.Limits{MaxFileBytes: int64(len(raw)), MaxRowsPerSheet: 2}); err != nil {
		t.Fatalf("at the limits: %v", err)
	}
}

func TestColumnNames(t *testing.T) {
	for column, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 51: "AZ", 52: "BA", 701: "ZZ", 702: "AAA"} {
		if got := spreadsheet.ColumnName(column); got != want {
			t.Errorf("ColumnName(%d) = %s, want %s", column, got, want)
		}
	}
}
