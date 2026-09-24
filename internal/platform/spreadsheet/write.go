package spreadsheet

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"time"
)

// WriteSheet 是要写出的一张表：第一行是列头（加粗、冻结），其下是数据行。所有格都写成行内
// 字符串，列可以预设为文本格——填表的人在文本格里敲的数字存成字符串，不经二进制浮点。
type WriteSheet struct {
	Name    string
	Columns []WriteColumn
	Rows    [][]string
}

// WriteColumn 是一列的列头、列宽（零值取默认宽度）与是否预设为文本格。
type WriteColumn struct {
	Header string
	Width  float64
	Text   bool
}

// fixedModified 让同一份输入每次写出逐字节相同的压缩包：入库的模板文件靠逐字节比对防漂。
var fixedModified = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

const defaultColumnWidth = 20

// 样式表里 cellXfs 的下标：0 常规，1 文本格，2 文本格加粗（列头）。
const (
	styleText       = 1
	styleTextHeader = 2
)

// Write 写出一份工作簿。压缩包用存储方式（不压缩）：压缩算法的输出随 Go 版本可能变，而写出
// 结果要逐字节确定。
func Write(w io.Writer, sheets []WriteSheet) error {
	if len(sheets) == 0 {
		return fmt.Errorf("spreadsheet: a workbook needs at least one sheet")
	}
	archive := zip.NewWriter(w)
	parts := []struct {
		name    string
		content []byte
	}{
		{"[Content_Types].xml", contentTypes(len(sheets))},
		{"_rels/.rels", []byte(xml.Header + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="` + relationshipOfficeDocument + `" Target="xl/workbook.xml"/></Relationships>`)},
		{"xl/workbook.xml", workbookXML(sheets)},
		{"xl/_rels/workbook.xml.rels", workbookRelationships(len(sheets))},
		{"xl/styles.xml", []byte(stylesXML)},
	}
	for index, sheet := range sheets {
		content, err := worksheetXML(sheet)
		if err != nil {
			return err
		}
		parts = append(parts, struct {
			name    string
			content []byte
		}{fmt.Sprintf("xl/worksheets/sheet%d.xml", index+1), content})
	}
	for _, part := range parts {
		entry, err := archive.CreateHeader(&zip.FileHeader{Name: part.name, Method: zip.Store, Modified: fixedModified})
		if err != nil {
			return err
		}
		if _, err := entry.Write(part.content); err != nil {
			return err
		}
	}
	return archive.Close()
}

func contentTypes(sheets int) []byte {
	var buffer bytes.Buffer
	buffer.WriteString(xml.Header)
	buffer.WriteString(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	buffer.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`)
	buffer.WriteString(`<Default Extension="xml" ContentType="application/xml"/>`)
	buffer.WriteString(`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`)
	buffer.WriteString(`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>`)
	for index := 1; index <= sheets; index++ {
		fmt.Fprintf(&buffer, `<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, index)
	}
	buffer.WriteString(`</Types>`)
	return buffer.Bytes()
}

func workbookXML(sheets []WriteSheet) []byte {
	var buffer bytes.Buffer
	buffer.WriteString(xml.Header)
	buffer.WriteString(`<workbook xmlns="` + namespaceMain + `" xmlns:r="` + namespaceRelationship + `"><sheets>`)
	for index, sheet := range sheets {
		fmt.Fprintf(&buffer, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, escape(sheet.Name), index+1, index+1)
	}
	buffer.WriteString(`</sheets></workbook>`)
	return buffer.Bytes()
}

func workbookRelationships(sheets int) []byte {
	var buffer bytes.Buffer
	buffer.WriteString(xml.Header)
	buffer.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for index := 1; index <= sheets; index++ {
		fmt.Fprintf(&buffer, `<Relationship Id="rId%d" Type="%s" Target="worksheets/sheet%d.xml"/>`, index, relationshipWorksheet, index)
	}
	fmt.Fprintf(&buffer, `<Relationship Id="rId%d" Type="%s/styles" Target="styles.xml"/>`, sheets+1, namespaceRelationship)
	buffer.WriteString(`</Relationships>`)
	return buffer.Bytes()
}

// stylesXML 是能被常见电子表格软件打开的最小样式表，外加文本格（数字格式 49，即「@」）与
// 加粗文本格两种格式。
const stylesXML = xml.Header + `<styleSheet xmlns="` + namespaceMain + `">` +
	`<fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts>` +
	`<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>` +
	`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
	`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
	`<cellXfs count="3">` +
	`<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>` +
	`<xf numFmtId="49" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` +
	`<xf numFmtId="49" fontId="1" fillId="0" borderId="0" xfId="0" applyNumberFormat="1" applyFont="1"/>` +
	`</cellXfs>` +
	`<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>` +
	`</styleSheet>`

func worksheetXML(sheet WriteSheet) ([]byte, error) {
	if len(sheet.Columns) == 0 {
		return nil, fmt.Errorf("spreadsheet: sheet %q has no columns", sheet.Name)
	}
	var buffer bytes.Buffer
	buffer.WriteString(xml.Header)
	buffer.WriteString(`<worksheet xmlns="` + namespaceMain + `" xmlns:r="` + namespaceRelationship + `">`)
	buffer.WriteString(`<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>`)
	buffer.WriteString(`<cols>`)
	for index, column := range sheet.Columns {
		width := column.Width
		if width <= 0 {
			width = defaultColumnWidth
		}
		style := ""
		if column.Text {
			style = ` style="` + strconv.Itoa(styleText) + `"`
		}
		fmt.Fprintf(&buffer, `<col min="%d" max="%d" width="%s" customWidth="1"%s/>`,
			index+1, index+1, strconv.FormatFloat(width, 'f', -1, 64), style)
	}
	buffer.WriteString(`</cols><sheetData>`)
	header := make([]string, len(sheet.Columns))
	for index, column := range sheet.Columns {
		header[index] = column.Header
	}
	writeRow(&buffer, 1, header, func(int) int { return styleTextHeader })
	for offset, row := range sheet.Rows {
		if len(row) > len(sheet.Columns) {
			return nil, fmt.Errorf("spreadsheet: sheet %q row %d has more cells than columns", sheet.Name, offset+2)
		}
		writeRow(&buffer, offset+2, row, func(index int) int {
			if sheet.Columns[index].Text {
				return styleText
			}
			return 0
		})
	}
	buffer.WriteString(`</sheetData></worksheet>`)
	return buffer.Bytes(), nil
}

func writeRow(buffer *bytes.Buffer, number int, cells []string, style func(int) int) {
	fmt.Fprintf(buffer, `<row r="%d">`, number)
	for index, text := range cells {
		if text == "" {
			continue
		}
		reference := ColumnName(index) + strconv.Itoa(number)
		styleAttribute := ""
		if applied := style(index); applied != 0 {
			styleAttribute = ` s="` + strconv.Itoa(applied) + `"`
		}
		fmt.Fprintf(buffer, `<c r="%s"%s t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, reference, styleAttribute, escape(text))
	}
	buffer.WriteString(`</row>`)
}

func escape(text string) string {
	var buffer bytes.Buffer
	_ = xml.EscapeText(&buffer, []byte(text))
	return buffer.String()
}
