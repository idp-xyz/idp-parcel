package pricecardtemplate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/spreadsheet"
)

// 结构化工作簿（JSON）是模板的第二种载体，与电子表格文件是同一个模板的两种写法：
//
//	{"templateVersion":"PPT-2","sheets":{"<表名>":[{"<列键>":<字符串|数字|null>, ...}, ...], ...}}
//
// 表名与列键逐字等于模板；一行一个对象；card 表照电子表格里 card 表的写法，一行一个字段，写成 {field, value}。
//
// 本文件只做「载体 → 内部表示」这一步：把 JSON 译成 spreadsheet.Workbook，然后交给 decodeWorkbook。它不判任何一格的
// 内容，也不另立问题码——表、列、格的一切判定都在 decodeWorkbook 之后，与读文件同一条路径，所以同一份内容无论从哪个载体
// 进来，读法、摘要、问题清单（含坐标）都逐位相同。坐标的约定：数组下标 i 的那一行是第 i+2 行（第 1 行是列键行），与同一份
// 内容写成电子表格时的行号一致。

var _ ports.PriceCardWorkbookReader = Reader{}

// workbookDoc 是校验过形状的工作簿 JSON。
type workbookDoc struct {
	templateVersion string
	sheets          map[string][]map[string]jsonCell
}

// jsonCell 是一格：字符串与数字都按文本交出（数字取书写的十进制字面量，不经浮点），null 是空格，布尔留给译码去拒。
type jsonCell struct {
	kind spreadsheet.CellKind
	text string
	null bool
}

const maxJSONDepth = 8

func malformed(format string, args ...any) error {
	return ports.PriceCardWorkbookRejection{Detail: fmt.Sprintf(format, args...)}
}

func tooLarge(format string, args ...any) error {
	return ports.PriceCardWorkbookRejection{TooLarge: true, Detail: fmt.Sprintf(format, args...)}
}

// parseWorkbookJSON 严格解一份工作簿 JSON：同一对象里同名键出现两次不收（后一个静默盖掉前一个，读到的就不是提交的），
// 顶层多出的键不收，尾随的东西不收。
func parseWorkbookJSON(raw []byte) (workbookDoc, error) {
	if len(raw) > MaxFileBytes {
		return workbookDoc{}, tooLarge("超过技术上限（10 MiB）")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := readJSONValue(decoder, "工作簿", 0)
	if err != nil {
		return workbookDoc{}, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return workbookDoc{}, malformed("工作簿 JSON 后面还有多余的内容")
	}
	top, ok := value.(map[string]any)
	if !ok {
		return workbookDoc{}, malformed(`顶层必须是对象 {"templateVersion": ..., "sheets": {...}}`)
	}
	for key := range top {
		if key != "templateVersion" && key != "sheets" {
			return workbookDoc{}, malformed("顶层只认 templateVersion 与 sheets，多出了 %q", key)
		}
	}
	version, ok := top["templateVersion"].(string)
	if !ok {
		return workbookDoc{}, malformed("顶层缺 templateVersion，或它不是字符串")
	}
	sheetsValue, ok := top["sheets"].(map[string]any)
	if !ok {
		return workbookDoc{}, malformed("顶层缺 sheets，或它不是对象（表名 → 行数组）")
	}
	doc := workbookDoc{templateVersion: version, sheets: map[string][]map[string]jsonCell{}}
	for name, rowsValue := range sheetsValue {
		rows, ok := rowsValue.([]any)
		if !ok {
			return workbookDoc{}, malformed("表 %s 必须是行数组", name)
		}
		if len(rows) > maxRowsPerSheet {
			return workbookDoc{}, tooLarge("表 %s 超过单表 %d 行的技术上限", name, maxRowsPerSheet)
		}
		parsed := make([]map[string]jsonCell, 0, len(rows))
		for index, rowValue := range rows {
			row, ok := rowValue.(map[string]any)
			if !ok {
				return workbookDoc{}, malformed("表 %s 的第 %d 行（数组下标 %d）必须是对象", name, index+2, index)
			}
			cells := make(map[string]jsonCell, len(row))
			for key, cellValue := range row {
				switch typed := cellValue.(type) {
				case string:
					cells[key] = jsonCell{kind: spreadsheet.CellText, text: typed}
				case json.Number:
					cells[key] = jsonCell{kind: spreadsheet.CellText, text: typed.String()}
				case bool:
					cells[key] = jsonCell{kind: spreadsheet.CellBoolean, text: fmt.Sprint(typed)}
				case nil:
					cells[key] = jsonCell{null: true}
				default:
					return workbookDoc{}, malformed("表 %s 第 %d 行的 %s 只能是字符串、数字或 null", name, index+2, key)
				}
			}
			parsed = append(parsed, cells)
		}
		doc.sheets[name] = parsed
	}
	return doc, nil
}

func readJSONValue(decoder *json.Decoder, path string, depth int) (any, error) {
	if depth > maxJSONDepth {
		return nil, malformed("%s 嵌套过深", path)
	}
	token, err := decoder.Token()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, malformed("工作簿 JSON 是空的或提前结束")
		}
		return nil, malformed("读不出 JSON（%v）", err)
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return token, nil
	}
	switch delimiter {
	case '{':
		object := map[string]any{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, malformed("读不出 JSON（%v）", err)
			}
			key, _ := keyToken.(string)
			if _, duplicated := object[key]; duplicated {
				return nil, malformed("%s 里的键 %q 出现了不止一次", path, key)
			}
			value, err := readJSONValue(decoder, path+"."+key, depth+1)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		if _, err := decoder.Token(); err != nil {
			return nil, malformed("读不出 JSON（%v）", err)
		}
		return object, nil
	case '[':
		array := []any{}
		for decoder.More() {
			value, err := readJSONValue(decoder, path+"[]", depth+1)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, malformed("读不出 JSON（%v）", err)
		}
		return array, nil
	default:
		return nil, malformed("%s 里有多余的括号", path)
	}
}

// canonical 把形状校验过的工作簿写成规范化字节：键排序、无多余空白、不转义 < > &，数值字面量原样；行数组保持原序
// （行序在条件树与计算树里有意义）。
func (doc workbookDoc) canonical() ([]byte, error) {
	sheetsOut := make(map[string]any, len(doc.sheets))
	for name, rows := range doc.sheets {
		rowsOut := make([]any, 0, len(rows))
		for _, row := range rows {
			cells := make(map[string]any, len(row))
			for key, cell := range row {
				switch {
				case cell.null:
					cells[key] = nil
				case cell.kind == spreadsheet.CellBoolean:
					cells[key] = cell.text == "true"
				default:
					cells[key] = cell.text
				}
			}
			rowsOut = append(rowsOut, cells)
		}
		sheetsOut[name] = rowsOut
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(map[string]any{"templateVersion": doc.templateVersion, "sheets": sheetsOut}); err != nil {
		return nil, malformed("写不出规范化字节（%v）", err)
	}
	return bytes.TrimRight(buffer.Bytes(), "\n"), nil
}

// CanonicalizePriceCardWorkbook 见 ports.PriceCardWorkbookReader。数字格在规范化时落成字符串：数字与同样字面量的字符串
// 本来就读成同一格，规范化后它们得同一个身份。
func (Reader) CanonicalizePriceCardWorkbook(raw []byte) ([]byte, error) {
	doc, err := parseWorkbookJSON(raw)
	if err != nil {
		return nil, err
	}
	return doc.canonical()
}

// ReadPriceCardWorkbook 见 ports.PriceCardWorkbookReader。
func (Reader) ReadPriceCardWorkbook(canonical []byte) ports.PriceCardTemplateReading {
	doc, err := parseWorkbookJSON(canonical)
	if err != nil {
		return workbookRejectedReading(err)
	}
	book, err := doc.workbook()
	if err != nil {
		version := TemplateVersion
		if acceptedVersion(doc.templateVersion) {
			version = doc.templateVersion
		}
		return rejectedReading(version, ports.TemplateVersionUnsupported, "%v", err)
	}
	return decodeWorkbook(book)
}

// workbookRejectedReading 把形状没过交成整份不收的读法，话取 Detail 本身。
func workbookRejectedReading(err error) ports.PriceCardTemplateReading {
	code, detail := ports.TemplateFileNotWorkbook, err.Error()
	var rejection ports.PriceCardWorkbookRejection
	if errors.As(err, &rejection) {
		detail = rejection.Detail
		if rejection.TooLarge {
			code = ports.TemplateFileTooLarge
		}
	}
	return rejectedReading(TemplateVersion, code, "读不出这份工作簿：%s", detail)
}

// workbook 把工作簿 JSON 译成与读电子表格同一个内部表示。两处载体特有的约定在这里落定，之后没有任何一步分得出载体：
//
//   - 没有出现的表按只有列键行的空表算。电子表格里删掉一张表是要拦的失误（缺表问题），JSON 里没有「删」这回事，要求
//     调用方把十六张空表逐一写出来只是噪声。
//   - 外层的 templateVersion 补进 card 表：card 表没写这个字段（或写成空）就用外层的，写了而不同则不收——两处各说各的，
//     读哪一个都是替提交的人猜。
func (doc workbookDoc) workbook() (spreadsheet.Workbook, error) {
	declared := doc.sheets
	cardRows := append([]map[string]jsonCell(nil), declared[sheetCard]...)
	filled := false
	for index, row := range cardRows {
		if field, ok := row["field"]; !ok || field.null || strings.TrimSpace(field.text) != "templateVersion" {
			continue
		}
		value := row["value"]
		switch text := strings.TrimSpace(value.text); {
		case value.null || text == "":
			copied := map[string]jsonCell{}
			for key, cell := range row {
				copied[key] = cell
			}
			copied["value"] = jsonCell{kind: spreadsheet.CellText, text: doc.templateVersion}
			cardRows[index] = copied
		case text != strings.TrimSpace(doc.templateVersion):
			return spreadsheet.Workbook{}, fmt.Errorf("工作簿外层声明的模板版本是 %s，card 表里却写 %s；两处须一致，或 card 表不写这一行", doc.templateVersion, text)
		}
		filled = true
		break
	}
	if !filled {
		cardRows = append(cardRows, map[string]jsonCell{
			"field": {kind: spreadsheet.CellText, text: "templateVersion"},
			"value": {kind: spreadsheet.CellText, text: doc.templateVersion},
		})
	}

	var book spreadsheet.Workbook
	listed := map[string]bool{}
	for _, spec := range sheets {
		if !availableIn(spec.since, doc.templateVersion) {
			continue
		}
		listed[spec.name] = true
		rows := declared[spec.name]
		if spec.name == sheetCard {
			rows = cardRows
		}
		book.Sheets = append(book.Sheets, workbookSheet(spec.name, specKeys(spec), rows))
	}
	var rest []string
	for name := range declared {
		if !listed[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	for _, name := range rest {
		// 不在模板里的表（或旧版里还没有的表）原样交给译码，由它按未知表报；列键取各行出现过的键。
		known := []string(nil)
		if spec, _, found := sheetByName(name); found {
			known = specKeys(spec)
		}
		book.Sheets = append(book.Sheets, workbookSheet(name, known, declared[name]))
	}
	return book, nil
}

func specKeys(spec sheetSpec) []string {
	keys := make([]string, 0, len(spec.columns))
	for _, entry := range spec.columns {
		keys = append(keys, entry.key)
	}
	return keys
}

// workbookSheet 造一张表：第 1 行是列键行（模板的列，再接各行里多出来的键，按字母序——多出来的由译码报未知列），
// 数据行从第 2 行起，第 i 个数组元素就是第 i+2 行。null 与缺键都是没有内容的格，不交出。
func workbookSheet(name string, known []string, rows []map[string]jsonCell) spreadsheet.Sheet {
	columns := append([]string(nil), known...)
	index := map[string]int{}
	for position, key := range columns {
		index[key] = position
	}
	var extra []string
	seen := map[string]bool{}
	for _, row := range rows {
		for key := range row {
			if _, ok := index[key]; !ok && !seen[key] {
				seen[key] = true
				extra = append(extra, key)
			}
		}
	}
	sort.Strings(extra)
	for _, key := range extra {
		index[key] = len(columns)
		columns = append(columns, key)
	}

	sheet := spreadsheet.Sheet{Name: name}
	header := spreadsheet.Row{Number: 1}
	for position, key := range columns {
		header.Cells = append(header.Cells, spreadsheet.Cell{Column: position, Kind: spreadsheet.CellText, Text: key})
	}
	if len(columns) > 0 {
		sheet.Rows = append(sheet.Rows, header)
	}
	for position, row := range rows {
		data := spreadsheet.Row{Number: position + 2}
		for _, key := range columns {
			cell, ok := row[key]
			if !ok || cell.null {
				continue
			}
			data.Cells = append(data.Cells, spreadsheet.Cell{Column: index[key], Kind: cell.kind, Text: cell.text})
		}
		sheet.Rows = append(sheet.Rows, data)
	}
	return sheet
}
