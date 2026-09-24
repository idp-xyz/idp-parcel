// Package spreadsheet 读写 Office Open XML 工作簿（.xlsx）的一个严格子集。
//
// 它只交出「哪张表、第几行、第几列、什么类型的格、存的是什么字面量」，不做取舍：数值格原样
// 交出存储的字面量，带公式的格标成公式，收不收、怎么报由调用方按自己的契约定（价卡导入见
// docs/design/pp-price-card-import-template-and-validation-spec.md 第五节）。通用表格库会把
// 这两类格默默换成浮点数与缓存值，那正是调用方要拒收的东西，所以这里不引库、只用标准库。
package spreadsheet

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

var (
	// ErrNotWorkbook：不是一份读得通的 .xlsx——不是压缩包（.xls、加密工作簿都落在这里）、缺必要
	// 部件，或是严格 OOXML 命名空间。
	ErrNotWorkbook = errors.New("spreadsheet: not a readable xlsx workbook")
	// ErrTooLarge：文件本身、解压后读入的部件合计或单表行数超过 Limits。
	ErrTooLarge = errors.New("spreadsheet: workbook exceeds the read limits")
)

// Limits 是防御性的技术上限，不是业务取值；零值字段表示该项不设限。
type Limits struct {
	MaxFileBytes    int64
	MaxPartBytes    int64
	MaxRowsPerSheet int
}

// CellKind 是格在文件里声明的类型。
type CellKind uint8

const (
	CellKindInvalid CellKind = iota
	// CellText：共享字符串、行内字符串，或不带公式的字符串结果。
	CellText
	// CellNumber：数值格；Text 是文件里存的字面量，不经浮点往返。
	CellNumber
	CellBoolean
	// CellDate：ISO 8601 日期格（t="d"）。电子表格常见的日期是带日期格式的数值格，那一种在这里
	// 就是 CellNumber——本包不读样式，不替调用方猜哪个数是日期。
	CellDate
	CellError
	// CellFormula：带公式的格，不论结果类型；Text 是缓存值，可能为空，也可能没重算过。
	CellFormula
)

func (kind CellKind) String() string {
	switch kind {
	case CellText:
		return "TEXT"
	case CellNumber:
		return "NUMBER"
	case CellBoolean:
		return "BOOLEAN"
	case CellDate:
		return "DATE"
	case CellError:
		return "ERROR"
	case CellFormula:
		return "FORMULA"
	default:
		return ""
	}
}

// Cell 是有内容的一格。没有内容的格（没有值也没有公式）不交出。
type Cell struct {
	// Column 从 0 起，A 列为 0。
	Column int
	Kind   CellKind
	Text   string
}

// Row 是至少有一格内容的一行，Number 与电子表格所示行号一致（从 1 起）。
type Row struct {
	Number int
	Cells  []Cell
}

// Sheet 是一张工作表，Rows 按行号升序。
type Sheet struct {
	Name string
	Rows []Row
}

// Workbook 的 Sheets 按工作簿里声明的顺序。
type Workbook struct {
	Sheets []Sheet
}

const (
	namespaceMain         = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	namespaceRelationship = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	// 严格 OOXML 的命名空间都在这个前缀下；遇到即整份不收，让人另存为普通工作簿。
	strictNamespacePrefix = "http://purl.oclc.org/ooxml/"

	relationshipOfficeDocument = namespaceRelationship + "/officeDocument"
	relationshipWorksheet      = namespaceRelationship + "/worksheet"
	relationshipSharedStrings  = namespaceRelationship + "/sharedStrings"
)

// Read 读出一份工作簿。
func Read(raw []byte, limits Limits) (Workbook, error) {
	if limits.MaxFileBytes > 0 && int64(len(raw)) > limits.MaxFileBytes {
		return Workbook{}, fmt.Errorf("%w: file is %d bytes", ErrTooLarge, len(raw))
	}
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return Workbook{}, fmt.Errorf("%w: %v", ErrNotWorkbook, err)
	}
	reader := &packageReader{parts: make(map[string]*zip.File, len(archive.File)), limits: limits}
	for _, file := range archive.File {
		// OPC 的部件名不分大小写，压缩包里的文件名分；按小写建索引。
		reader.parts[strings.ToLower(file.Name)] = file
	}
	return reader.workbook()
}

type packageReader struct {
	parts  map[string]*zip.File
	limits Limits
	read   int64
}

func (reader *packageReader) part(name string) ([]byte, error) {
	file, found := reader.parts[strings.ToLower(strings.TrimPrefix(name, "/"))]
	if !found {
		return nil, fmt.Errorf("%w: missing part %s", ErrNotWorkbook, name)
	}
	stream, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotWorkbook, err)
	}
	defer stream.Close()
	var source io.Reader = stream
	if reader.limits.MaxPartBytes > 0 {
		// 声明的解压大小可以撒谎，按实际读出的字节计。
		source = io.LimitReader(stream, reader.limits.MaxPartBytes-reader.read+1)
	}
	content, err := io.ReadAll(source)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotWorkbook, err)
	}
	reader.read += int64(len(content))
	if reader.limits.MaxPartBytes > 0 && reader.read > reader.limits.MaxPartBytes {
		return nil, fmt.Errorf("%w: uncompressed parts exceed %d bytes", ErrTooLarge, reader.limits.MaxPartBytes)
	}
	return content, nil
}

type relationship struct {
	ID     string `xml:"Id,attr"`
	Type   string `xml:"Type,attr"`
	Target string `xml:"Target,attr"`
}

type relationships struct {
	XMLName xml.Name       `xml:"Relationships"`
	Items   []relationship `xml:"Relationship"`
}

func (reader *packageReader) relationships(source string) ([]relationship, error) {
	directory, file := path.Split(source)
	content, err := reader.part(path.Join(directory, "_rels", file+".rels"))
	if err != nil {
		return nil, err
	}
	var document relationships
	if err := xml.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotWorkbook, err)
	}
	for _, item := range document.Items {
		if strings.HasPrefix(item.Type, strictNamespacePrefix) {
			return nil, fmt.Errorf("%w: strict OOXML is not supported", ErrNotWorkbook)
		}
	}
	return document.Items, nil
}

// resolve 把关系目标换成包内部件名：绝对目标自包根算，相对目标自源部件所在目录算。
func resolve(source, target string) string {
	if strings.HasPrefix(target, "/") {
		return path.Clean(strings.TrimPrefix(target, "/"))
	}
	return path.Join(path.Dir(source), target)
}

type workbookDocument struct {
	XMLName xml.Name `xml:"workbook"`
	Sheets  []struct {
		Name string     `xml:"name,attr"`
		Attr []xml.Attr `xml:",any,attr"`
	} `xml:"sheets>sheet"`
}

func (reader *packageReader) workbook() (Workbook, error) {
	roots, err := reader.relationships("")
	if err != nil {
		return Workbook{}, err
	}
	workbookPart := ""
	for _, item := range roots {
		if item.Type == relationshipOfficeDocument {
			workbookPart = resolve("", item.Target)
		}
	}
	if workbookPart == "" {
		return Workbook{}, fmt.Errorf("%w: no office document relationship", ErrNotWorkbook)
	}
	content, err := reader.part(workbookPart)
	if err != nil {
		return Workbook{}, err
	}
	var document workbookDocument
	if err := xml.Unmarshal(content, &document); err != nil {
		return Workbook{}, fmt.Errorf("%w: %v", ErrNotWorkbook, err)
	}
	if document.XMLName.Space != namespaceMain {
		return Workbook{}, fmt.Errorf("%w: workbook namespace %q", ErrNotWorkbook, document.XMLName.Space)
	}
	links, err := reader.relationships(workbookPart)
	if err != nil {
		return Workbook{}, err
	}
	targets := make(map[string]string, len(links))
	var shared []string
	for _, item := range links {
		switch item.Type {
		case relationshipWorksheet:
			targets[item.ID] = resolve(workbookPart, item.Target)
		case relationshipSharedStrings:
			raw, err := reader.part(resolve(workbookPart, item.Target))
			if err != nil {
				return Workbook{}, err
			}
			if shared, err = readSharedStrings(raw); err != nil {
				return Workbook{}, err
			}
		}
	}

	book := Workbook{Sheets: make([]Sheet, 0, len(document.Sheets))}
	for _, declared := range document.Sheets {
		id := ""
		for _, attribute := range declared.Attr {
			if attribute.Name.Space == namespaceRelationship && attribute.Name.Local == "id" {
				id = attribute.Value
			}
		}
		target, found := targets[id]
		if !found {
			// 图表页之类不是工作表的页没有 worksheet 关系；它们不在任何契约里，照样交出一张空表，
			// 让调用方按表名决定收不收，而不是在这里悄悄吞掉。
			book.Sheets = append(book.Sheets, Sheet{Name: declared.Name})
			continue
		}
		raw, err := reader.part(target)
		if err != nil {
			return Workbook{}, err
		}
		rows, err := readRows(raw, shared, reader.limits.MaxRowsPerSheet)
		if err != nil {
			return Workbook{}, err
		}
		book.Sheets = append(book.Sheets, Sheet{Name: declared.Name, Rows: rows})
	}
	return book, nil
}

// readSharedStrings 读共享字符串表。一项的文本是它直属的 <t>，或各个 <r> 里的 <t> 依次相连；
// 注音（<rPh>）里的 <t> 不是这一格的内容，不收。
func readSharedStrings(raw []byte) ([]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	var (
		items   []string
		current strings.Builder
		stack   []string
	)
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return items, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%w: shared strings: %v", ErrNotWorkbook, err)
		}
		switch element := token.(type) {
		case xml.StartElement:
			stack = append(stack, element.Name.Local)
			if element.Name.Local == "si" {
				current.Reset()
			}
		case xml.EndElement:
			if element.Name.Local == "si" {
				items = append(items, current.String())
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if inStringText(stack, "si") {
				current.Write(element)
			}
		}
	}
}

// inStringText 答当前位置是不是一段字符串项（共享字符串的 si、行内字符串的 is）的正文。
func inStringText(stack []string, container string) bool {
	depth := len(stack)
	if depth < 2 || stack[depth-1] != "t" {
		return false
	}
	if stack[depth-2] == container {
		return true
	}
	return depth >= 3 && stack[depth-2] == "r" && stack[depth-3] == container
}

func readRows(raw []byte, shared []string, maxRows int) ([]Row, error) {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	var (
		rows       []Row
		stack      []string
		rowCount   int
		rowNumber  int
		current    *Row
		cell       cellState
		lastColumn int
	)
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return rows, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%w: worksheet: %v", ErrNotWorkbook, err)
		}
		switch element := token.(type) {
		case xml.StartElement:
			stack = append(stack, element.Name.Local)
			switch element.Name.Local {
			case "row":
				rowCount++
				if maxRows > 0 && rowCount > maxRows {
					return nil, fmt.Errorf("%w: more than %d rows in a sheet", ErrTooLarge, maxRows)
				}
				number := rowNumber + 1
				if value := attribute(element, "r"); value != "" {
					parsed, err := strconv.Atoi(value)
					if err != nil || parsed < 1 {
						return nil, fmt.Errorf("%w: row reference %q", ErrNotWorkbook, value)
					}
					number = parsed
				}
				rowNumber = number
				current = &Row{Number: number}
				lastColumn = -1
			case "c":
				column := lastColumn + 1
				if reference := attribute(element, "r"); reference != "" {
					parsed, err := columnOf(reference)
					if err != nil {
						return nil, err
					}
					column = parsed
				}
				lastColumn = column
				cell = cellState{column: column, kind: attribute(element, "t")}
			case "f":
				cell.formula = true
			}
		case xml.EndElement:
			switch element.Name.Local {
			case "c":
				if current != nil {
					if resolved, ok, err := cell.resolve(shared); err != nil {
						return nil, err
					} else if ok {
						current.Cells = append(current.Cells, resolved)
					}
				}
				cell = cellState{}
			case "row":
				if current != nil && len(current.Cells) > 0 {
					rows = append(rows, *current)
				}
				current = nil
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			depth := len(stack)
			switch {
			case depth >= 2 && stack[depth-1] == "v" && stack[depth-2] == "c":
				cell.value.Write(element)
			case inStringText(stack, "is"):
				cell.inline.Write(element)
			}
		}
	}
}

type cellState struct {
	column  int
	kind    string
	formula bool
	value   strings.Builder
	inline  strings.Builder
}

func (cell *cellState) resolve(shared []string) (Cell, bool, error) {
	value := cell.value.String()
	if cell.formula {
		return Cell{Column: cell.column, Kind: CellFormula, Text: value}, true, nil
	}
	resolved := Cell{Column: cell.column}
	switch cell.kind {
	case "s":
		if value == "" {
			return Cell{}, false, nil
		}
		index, err := strconv.Atoi(value)
		if err != nil || index < 0 || index >= len(shared) {
			return Cell{}, false, fmt.Errorf("%w: shared string index %q", ErrNotWorkbook, value)
		}
		resolved.Kind, resolved.Text = CellText, shared[index]
	case "inlineStr":
		resolved.Kind, resolved.Text = CellText, cell.inline.String()
	case "str":
		resolved.Kind, resolved.Text = CellText, value
	case "b":
		resolved.Kind, resolved.Text = CellBoolean, value
	case "e":
		resolved.Kind, resolved.Text = CellError, value
	case "d":
		resolved.Kind, resolved.Text = CellDate, value
	case "", "n":
		resolved.Kind, resolved.Text = CellNumber, value
	default:
		return Cell{}, false, fmt.Errorf("%w: cell type %q", ErrNotWorkbook, cell.kind)
	}
	if resolved.Text == "" {
		return Cell{}, false, nil
	}
	return resolved, true, nil
}

func attribute(element xml.StartElement, name string) string {
	for _, item := range element.Attr {
		if item.Name.Space == "" && item.Name.Local == name {
			return item.Value
		}
	}
	return ""
}

// columnOf 从 A1 形式的格引用取列号（从 0 起）。
func columnOf(reference string) (int, error) {
	column := 0
	letters := 0
	for _, character := range reference {
		if character < 'A' || character > 'Z' {
			break
		}
		column = column*26 + int(character-'A'+1)
		letters++
	}
	if letters == 0 || letters > 3 {
		return 0, fmt.Errorf("%w: cell reference %q", ErrNotWorkbook, reference)
	}
	return column - 1, nil
}

// ColumnName 把列号（从 0 起）写成电子表格的列名，A 列为 0。
func ColumnName(column int) string {
	name := ""
	for column >= 0 {
		name = string(rune('A'+column%26)) + name
		column = column/26 - 1
	}
	return name
}
