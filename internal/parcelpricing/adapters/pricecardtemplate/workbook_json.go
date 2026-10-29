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
//
// 电子表格读口边读边数、超限即停（spreadsheet.Limits）；JSON 载体没有现成的容器替它数，所以解析是流式的、边解边数，
// 超过下面几个上限立刻停，不先把整份解成树再判。这些上限是防御性的技术上限，不是业务取值。

var _ ports.PriceCardWorkbookReader = Reader{}

// JSON 载体的结构上限，与 MaxFileBytes、maxRowsPerSheet 一起构成「再大就不是一张价卡」的边界。表数与列键数远大于模板的表数
// 与最宽那张表的列数，余量留给未知表与未知列（它们照样要被如实报出来）；列键数上限同时是行内位掩码的宽度，不能超过 64。
// 总格数取模板里最宽的价表行表按单表行数上限铺满也装得下的规模；它同时封住「少数几行带大量不同键」那一类按行数乘列数放大的输入。
const (
	maxJSONSheets  = 64
	maxJSONColumns = 64
	maxJSONCells   = 1_000_000
)

// workbookDoc 是校验过形状的工作簿 JSON。templateVersion 已去首尾空白：读取时 card 表里的版本格同样去空白，两边一个口径。
type workbookDoc struct {
	templateVersion string
	sheets          map[string]*jsonSheet
}

// jsonSheet 是一张表：列键表（本表出现过的不同列键，按首次出现的先后，至多 maxJSONColumns 个）与各行。行不用「键 → 格」的映射，
// 而是格的切片、列用列键表里的下标：每格 24 字节上下而不是一个映射条目，行内同名键用一个位掩码判，总格数打满上限时内存也留在
// 几十兆。
type jsonSheet struct {
	columns []string
	rows    [][]jsonField
}

// jsonField 是一行里的一格：列下标加格的内容。字符串与数字都按文本交出（数字取书写的十进制字面量，不经浮点），null 是空格，
// 布尔留给译码去拒。
type jsonField struct {
	text   string
	column uint8
	kind   spreadsheet.CellKind
	null   bool
}

func (sheet *jsonSheet) columnIndex(key string) int {
	for index, candidate := range sheet.columns {
		if candidate == key {
			return index
		}
	}
	return -1
}

// lookup 取一行里某个列键的格；没有这个键答假。
func (sheet *jsonSheet) lookup(row []jsonField, key string) (jsonField, bool) {
	column := sheet.columnIndex(key)
	for _, field := range row {
		if column >= 0 && int(field.column) == column {
			return field, true
		}
	}
	return jsonField{}, false
}

func malformed(format string, args ...any) error {
	return ports.PriceCardWorkbookRejection{Detail: fmt.Sprintf(format, args...), TemplateVersion: TemplateVersion}
}

func tooLarge(format string, args ...any) error {
	return ports.PriceCardWorkbookRejection{TooLarge: true, Detail: fmt.Sprintf(format, args...), TemplateVersion: TemplateVersion}
}

// workbookParser 流式解一份工作簿 JSON，同时数表、行、列键与格。
type workbookParser struct {
	decoder *json.Decoder
	cells   int
}

func (parser *workbookParser) token() (json.Token, error) {
	token, err := parser.decoder.Token()
	switch {
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return nil, malformed("工作簿 JSON 是空的或提前结束")
	case err != nil:
		return nil, malformed("读不出 JSON（%v）", err)
	}
	return token, nil
}

// parseWorkbookJSON 严格解一份工作簿 JSON：同一对象里同名键出现两次不收（后一个静默盖掉前一个，读到的就不是提交的），
// 顶层多出的键不收，尾随的东西不收，超过结构上限即停。byteLimit 大于 0 时先按原始字节数判：这道只在入口判一次，
// 以提交上来的原始请求体为准（读规范化字节时不再判——规范化会把数字落成字符串，字节数可以比原始的多）。
func parseWorkbookJSON(raw []byte, byteLimit int) (workbookDoc, error) {
	if byteLimit > 0 && len(raw) > byteLimit {
		return workbookDoc{}, tooLarge("超过技术上限（10 MiB）")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	parser := &workbookParser{decoder: decoder}
	doc, err := parser.workbook()
	if err != nil {
		return workbookDoc{}, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return workbookDoc{}, malformed("工作簿 JSON 后面还有多余的内容")
	}
	return doc, nil
}

func (parser *workbookParser) workbook() (workbookDoc, error) {
	opening, err := parser.token()
	if err != nil {
		return workbookDoc{}, err
	}
	if opening != json.Delim('{') {
		return workbookDoc{}, malformed(`顶层必须是对象 {"templateVersion": ..., "sheets": {...}}`)
	}
	var doc workbookDoc
	var haveVersion, haveSheets bool
	for parser.decoder.More() {
		keyToken, err := parser.token()
		if err != nil {
			return workbookDoc{}, err
		}
		switch key, _ := keyToken.(string); key {
		case "templateVersion":
			if haveVersion {
				return workbookDoc{}, malformed("工作簿 里的键 %q 出现了不止一次", key)
			}
			valueToken, err := parser.token()
			if err != nil {
				return workbookDoc{}, err
			}
			version, ok := valueToken.(string)
			if !ok {
				return workbookDoc{}, malformed("顶层缺 templateVersion，或它不是字符串")
			}
			doc.templateVersion, haveVersion = strings.TrimSpace(version), true
		case "sheets":
			if haveSheets {
				return workbookDoc{}, malformed("工作簿 里的键 %q 出现了不止一次", key)
			}
			sheets, err := parser.sheets()
			if err != nil {
				return workbookDoc{}, err
			}
			doc.sheets, haveSheets = sheets, true
		default:
			return workbookDoc{}, malformed("顶层只认 templateVersion 与 sheets，多出了 %q", key)
		}
	}
	if _, err := parser.token(); err != nil { // 顶层的 }
		return workbookDoc{}, err
	}
	switch {
	case !haveVersion:
		return workbookDoc{}, malformed("顶层缺 templateVersion，或它不是字符串")
	case !haveSheets:
		return workbookDoc{}, malformed("顶层缺 sheets，或它不是对象（表名 → 行数组）")
	}
	return doc, nil
}

func (parser *workbookParser) sheets() (map[string]*jsonSheet, error) {
	opening, err := parser.token()
	if err != nil {
		return nil, err
	}
	if opening != json.Delim('{') {
		return nil, malformed("顶层缺 sheets，或它不是对象（表名 → 行数组）")
	}
	sheets := map[string]*jsonSheet{}
	for parser.decoder.More() {
		nameToken, err := parser.token()
		if err != nil {
			return nil, err
		}
		name, _ := nameToken.(string)
		if _, duplicated := sheets[name]; duplicated {
			return nil, malformed("sheets 里的键 %q 出现了不止一次", name)
		}
		if len(sheets) >= maxJSONSheets {
			return nil, tooLarge("表超过 %d 张的技术上限", maxJSONSheets)
		}
		sheet, err := parser.rows(name)
		if err != nil {
			return nil, err
		}
		sheets[name] = sheet
	}
	if _, err := parser.token(); err != nil { // sheets 的 }
		return nil, err
	}
	return sheets, nil
}

func (parser *workbookParser) rows(name string) (*jsonSheet, error) {
	opening, err := parser.token()
	if err != nil {
		return nil, err
	}
	if opening != json.Delim('[') {
		return nil, malformed("表 %s 必须是行数组", name)
	}
	sheet := &jsonSheet{}
	index := map[string]int{}
	for parser.decoder.More() {
		if len(sheet.rows) >= maxRowsPerSheet {
			return nil, tooLarge("表 %s 超过单表 %d 行的技术上限", name, maxRowsPerSheet)
		}
		number := len(sheet.rows) + 2
		rowOpening, err := parser.token()
		if err != nil {
			return nil, err
		}
		if rowOpening != json.Delim('{') {
			return nil, malformed("表 %s 的第 %d 行（数组下标 %d）必须是对象", name, number, number-2)
		}
		row, err := parser.row(name, number, sheet, index)
		if err != nil {
			return nil, err
		}
		sheet.rows = append(sheet.rows, row)
	}
	if _, err := parser.token(); err != nil { // 行数组的 ]
		return nil, err
	}
	return sheet, nil
}

// row 读一行的各格，起点在行对象的 { 之后。本表不同列键数有上限（maxJSONColumns）：把「少数几行带大量不同键」在解析里就挡掉，
// 之后按行遍历各行自己的格即可，不必按行数乘列数扫整表。上限恰是位掩码的宽度，行内同名键用它判。
func (parser *workbookParser) row(sheet string, number int, columns *jsonSheet, index map[string]int) ([]jsonField, error) {
	var fields []jsonField
	var seen uint64
	for parser.decoder.More() {
		keyToken, err := parser.token()
		if err != nil {
			return nil, err
		}
		key, _ := keyToken.(string)
		column, known := index[key]
		if !known {
			if len(columns.columns) >= maxJSONColumns {
				return nil, tooLarge("表 %s 出现了超过 %d 个不同的列键的技术上限", sheet, maxJSONColumns)
			}
			column = len(columns.columns)
			columns.columns = append(columns.columns, key)
			index[key] = column
		}
		if seen&(1<<uint(column)) != 0 {
			return nil, malformed("表 %s 第 %d 行里的键 %q 出现了不止一次", sheet, number, key)
		}
		seen |= 1 << uint(column)
		if parser.cells++; parser.cells > maxJSONCells {
			return nil, tooLarge("格数超过 %d 个的技术上限", maxJSONCells)
		}
		valueToken, err := parser.token()
		if err != nil {
			return nil, err
		}
		field := jsonField{column: uint8(column)}
		switch typed := valueToken.(type) {
		case string:
			field.kind, field.text = spreadsheet.CellText, typed
		case json.Number:
			field.kind, field.text = spreadsheet.CellText, typed.String()
		case bool:
			field.kind, field.text = spreadsheet.CellBoolean, fmt.Sprint(typed)
		case nil:
			field.null = true
		default:
			return nil, malformed("表 %s 第 %d 行的 %s 只能是字符串、数字或 null", sheet, number, key)
		}
		fields = append(fields, field)
	}
	if _, err := parser.token(); err != nil { // 行对象的 }
		return nil, err
	}
	return fields, nil
}

// canonical 把形状校验过的工作簿写成规范化字节：键排序、无多余空白，数字落成字符串，行数组保持原序（行序在条件树与计算树里
// 有意义）。直接写字节，不先造一棵树：总格数有上限，但再造一份树就是把内存翻倍。就地给各行的格按列键排序，所以 doc 用完即弃。
func (doc workbookDoc) canonical() []byte {
	var out bytes.Buffer
	names := make([]string, 0, len(doc.sheets))
	for name := range doc.sheets {
		names = append(names, name)
	}
	sort.Strings(names)
	out.WriteString(`{"sheets":{`)
	for index, name := range names {
		if index > 0 {
			out.WriteByte(',')
		}
		sheet := doc.sheets[name]
		rank := columnRanks(sheet.columns)
		writeJSONString(&out, name)
		out.WriteString(":[")
		for position, row := range sheet.rows {
			if position > 0 {
				out.WriteByte(',')
			}
			for at := 1; at < len(row); at++ { // 插入排序：行至多 maxJSONColumns 格
				for back := at; back > 0 && rank[row[back].column] < rank[row[back-1].column]; back-- {
					row[back], row[back-1] = row[back-1], row[back]
				}
			}
			out.WriteByte('{')
			for at, field := range row {
				if at > 0 {
					out.WriteByte(',')
				}
				writeJSONString(&out, sheet.columns[field.column])
				out.WriteByte(':')
				switch {
				case field.null:
					out.WriteString("null")
				case field.kind == spreadsheet.CellBoolean:
					out.WriteString(field.text)
				default:
					writeJSONString(&out, field.text)
				}
			}
			out.WriteByte('}')
		}
		out.WriteByte(']')
	}
	out.WriteString(`},"templateVersion":`)
	writeJSONString(&out, doc.templateVersion)
	out.WriteByte('}')
	return out.Bytes()
}

// columnRanks 给列键表里每个下标一个按字母序的名次。
func columnRanks(columns []string) []int {
	order := make([]int, len(columns))
	for index := range order {
		order[index] = index
	}
	sort.Slice(order, func(left, right int) bool { return columns[order[left]] < columns[order[right]] })
	rank := make([]int, len(columns))
	for position, index := range order {
		rank[index] = position
	}
	return rank
}

func writeJSONString(out *bytes.Buffer, text string) {
	encoded, _ := json.Marshal(text) // 字符串编码不会失败
	out.Write(encoded)
}

// CanonicalizePriceCardWorkbook 见 ports.PriceCardWorkbookReader。数字格在规范化时落成字符串：数字与同样字面量的字符串
// 本来就读成同一格，规范化后它们得同一个身份。体积上限只在这里按原始请求体判一次。
func (Reader) CanonicalizePriceCardWorkbook(raw []byte) ([]byte, error) {
	doc, err := parseWorkbookJSON(raw, MaxFileBytes)
	if err != nil {
		return nil, err
	}
	return doc.canonical(), nil
}

// ReadPriceCardWorkbook 见 ports.PriceCardWorkbookReader。读的是规范化字节，不再判体积（见 parseWorkbookJSON），结构上限仍判。
func (Reader) ReadPriceCardWorkbook(canonical []byte) ports.PriceCardTemplateReading {
	doc, err := parseWorkbookJSON(canonical, 0)
	if err != nil {
		return ports.WorkbookRejectionReading(err)
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

// workbook 把工作簿 JSON 译成与读电子表格同一个内部表示。两处载体特有的约定在这里落定，之后没有任何一步分得出载体：
//
//   - 没有出现的表按只有列键行的空表算。电子表格里删掉一张表是要拦的失误（缺表问题），JSON 里没有「删」这回事，要求
//     调用方把每张空表逐一写出来只是噪声。
//   - 外层的 templateVersion 补进 card 表：card 表没写这个字段（或写成空）就用外层的，写了而不同则不收——两处各说各的，
//     读哪一个都是替提交的人猜。
func (doc workbookDoc) workbook() (spreadsheet.Workbook, error) {
	declared := doc.sheets
	card, err := doc.cardWithVersion()
	if err != nil {
		return spreadsheet.Workbook{}, err
	}

	var book spreadsheet.Workbook
	listed := map[string]bool{}
	for _, spec := range sheets {
		if !availableIn(spec.since, doc.templateVersion) {
			continue
		}
		listed[spec.name] = true
		sheet := declared[spec.name]
		if spec.name == sheetCard {
			sheet = card
		}
		book.Sheets = append(book.Sheets, workbookSheet(spec.name, specKeys(spec), sheet))
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

// cardWithVersion 交回补好版本字段的 card 表（不改 doc 里那一张）：有 templateVersion 这一行就核对或补值，没有就在末尾补一行。
func (doc workbookDoc) cardWithVersion() (*jsonSheet, error) {
	card := &jsonSheet{}
	if declared := doc.sheets[sheetCard]; declared != nil {
		card.columns = append([]string(nil), declared.columns...)
		card.rows = append([][]jsonField(nil), declared.rows...)
	}
	for _, key := range []string{"field", "value"} {
		if card.columnIndex(key) < 0 {
			card.columns = append(card.columns, key)
		}
	}
	fieldColumn, valueColumn := card.columnIndex("field"), card.columnIndex("value")
	versionField := func(text string) []jsonField {
		return []jsonField{
			{column: uint8(fieldColumn), kind: spreadsheet.CellText, text: "templateVersion"},
			{column: uint8(valueColumn), kind: spreadsheet.CellText, text: text},
		}
	}
	for position, row := range card.rows {
		field, ok := card.lookup(row, "field")
		if !ok || field.null || strings.TrimSpace(field.text) != "templateVersion" {
			continue
		}
		value, _ := card.lookup(row, "value")
		switch text := strings.TrimSpace(value.text); {
		case value.null || text == "":
			filled := make([]jsonField, 0, len(row)+1)
			for _, existing := range row {
				if int(existing.column) != valueColumn {
					filled = append(filled, existing)
				}
			}
			card.rows[position] = append(filled, jsonField{column: uint8(valueColumn), kind: spreadsheet.CellText, text: doc.templateVersion})
		case text != doc.templateVersion:
			return nil, fmt.Errorf("工作簿外层声明的模板版本是 %s，card 表里却写 %s；两处须一致，或 card 表不写这一行", doc.templateVersion, text)
		}
		return card, nil
	}
	card.rows = append(card.rows, versionField(doc.templateVersion))
	return card, nil
}

func specKeys(spec sheetSpec) []string {
	keys := make([]string, 0, len(spec.columns))
	for _, entry := range spec.columns {
		keys = append(keys, entry.key)
	}
	return keys
}

// workbookSheet 造一张表：第 1 行是列键行（模板的列，再接各行里多出来的键，按字母序——多出来的由译码报未知列），
// 数据行从第 2 行起，第 i 个数组元素就是第 i+2 行。null 与缺键都是没有内容的格，不交出；整行没有内容的行也不交出（译码对
// 整行空白的行本就跳过，行号取自 Row.Number，不取自它在切片里的位置）。数据行只遍历各行自己的格：代价是总格数，
// 不是行数乘列数。sheet 为空表示这张表没有出现，造成只有列键行的空表。
func workbookSheet(name string, known []string, sheet *jsonSheet) spreadsheet.Sheet {
	columns := append([]string(nil), known...)
	index := make(map[string]int, len(known))
	for position, key := range columns {
		index[key] = position
	}
	var position []int // 本表列键表的下标 → 列号
	if sheet != nil {
		var extra []string
		for _, key := range sheet.columns {
			if _, ok := index[key]; !ok {
				extra = append(extra, key)
			}
		}
		sort.Strings(extra)
		for _, key := range extra {
			index[key] = len(columns)
			columns = append(columns, key)
		}
		position = make([]int, len(sheet.columns))
		for at, key := range sheet.columns {
			position[at] = index[key]
		}
	}

	out := spreadsheet.Sheet{Name: name}
	header := spreadsheet.Row{Number: 1}
	for at, key := range columns {
		header.Cells = append(header.Cells, spreadsheet.Cell{Column: at, Kind: spreadsheet.CellText, Text: key})
	}
	if len(columns) > 0 {
		out.Rows = append(out.Rows, header)
	}
	if sheet == nil {
		return out
	}
	for at, row := range sheet.rows {
		var cells []spreadsheet.Cell
		for _, field := range row {
			if !field.null {
				cells = append(cells, spreadsheet.Cell{Column: position[field.column], Kind: field.kind, Text: field.text})
			}
		}
		if len(cells) == 0 {
			continue
		}
		for next := 1; next < len(cells); next++ { // 插入排序：行至多 maxJSONColumns 格
			for back := next; back > 0 && cells[back].Column < cells[back-1].Column; back-- {
				cells[back], cells[back-1] = cells[back-1], cells[back]
			}
		}
		out.Rows = append(out.Rows, spreadsheet.Row{Number: at + 2, Cells: cells})
	}
	return out
}
