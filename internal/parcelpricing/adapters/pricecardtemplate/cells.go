package pricecardtemplate

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/spreadsheet"
)

type fieldState uint8

const (
	fieldAbsent fieldState = iota
	fieldPresent
	// fieldBad：格的类型已经被拒（公式、错误值……），问题已报，不要再按「空」报一遍必填。
	fieldBad
)

// field 是读出来的一格，连同它的坐标。
type field struct {
	sheet  string
	row    int
	column string
	state  fieldState
	text   string
}

// record 是一张表的一条数据行。
type record struct {
	sheet string
	row   int
	cells map[string]field
}

func (rec record) get(key string) field {
	if found, ok := rec.cells[key]; ok {
		return found
	}
	return field{sheet: rec.sheet, row: rec.row, column: key}
}

type decoder struct {
	problems []ports.PriceCardTemplateProblem
	// broken 记下结构有问题的表（缺表、缺列）：指进这些表的引用不再报解析不到，免得一处结构
	// 问题引出一串连带问题。
	broken map[string]bool
}

func newDecoder() *decoder {
	return &decoder{broken: map[string]bool{}}
}

func (d *decoder) report(sheet string, row int, column string, code ports.PriceCardTemplateProblemCode, format string, args ...any) {
	d.problems = append(d.problems, ports.PriceCardTemplateProblem{
		Sheet: sheet, Row: row, Column: column, Code: code, Message: fmt.Sprintf(format, args...),
	})
}

func (d *decoder) at(f field, code ports.PriceCardTemplateProblemCode, format string, args ...any) {
	d.report(f.sheet, f.row, f.column, code, format, args...)
}

// sorted 按模板的表序、行号、列序排问题；同一格上的几条再按问题码与说明排，先后稳定。
func (d *decoder) sorted() []ports.PriceCardTemplateProblem {
	problems := append([]ports.PriceCardTemplateProblem(nil), d.problems...)
	sort.SliceStable(problems, func(left, right int) bool {
		a, b := problems[left], problems[right]
		if sheetRank(a.Sheet) != sheetRank(b.Sheet) {
			return sheetRank(a.Sheet) < sheetRank(b.Sheet)
		}
		if a.Sheet != b.Sheet {
			return a.Sheet < b.Sheet
		}
		if a.Row != b.Row {
			return a.Row < b.Row
		}
		if columnRank(a.Sheet, a.Column) != columnRank(b.Sheet, b.Column) {
			return columnRank(a.Sheet, a.Column) < columnRank(b.Sheet, b.Column)
		}
		if a.Column != b.Column {
			return a.Column < b.Column
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Message < b.Message
	})
	return problems
}

func sheetRank(name string) int {
	if name == "" {
		return -1
	}
	if _, index, ok := sheetByName(name); ok {
		return index
	}
	return len(sheets)
}

func columnRank(sheet, key string) int {
	if key == "" {
		return -1
	}
	if sheet == sheetCard {
		if _, index, ok := cardField(key); ok {
			return index
		}
	} else if spec, _, ok := sheetByName(sheet); ok {
		if _, index, found := spec.column(key); found {
			return index
		}
	}
	return 1 << 16
}

var numericLiteral = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

// maxExactSignificantDigits 是电子表格的显示精度：数值格存的字面量超过这么多位有效数字，
// 多出来的尾数只可能是二进制浮点的痕迹（规范第五节）。
const maxExactSignificantDigits = 15

func exactLiteral(text string) bool {
	if !numericLiteral.MatchString(text) {
		return false
	}
	digits := strings.TrimLeft(strings.ReplaceAll(strings.TrimPrefix(text, "-"), ".", ""), "0")
	return len(digits) <= maxExactSignificantDigits
}

// readCell 按规范第五节读一格。
func (d *decoder) readCell(sheet string, row int, column string, cell spreadsheet.Cell) field {
	read := field{sheet: sheet, row: row, column: column}
	switch cell.Kind {
	case spreadsheet.CellText:
		if text := strings.TrimSpace(cell.Text); text != "" {
			read.state, read.text = fieldPresent, text
		}
	case spreadsheet.CellNumber:
		if !exactLiteral(cell.Text) {
			d.at(read, ports.TemplateCellNumericNotExact,
				"数值格存的是 %s，不是它显示的那个数；请把这一列设为文本格后重填", cell.Text)
			read.state = fieldBad
			break
		}
		read.state, read.text = fieldPresent, cell.Text
	case spreadsheet.CellFormula:
		d.at(read, ports.TemplateCellFormula, "这一格是公式；公式的缓存值可能没重算，请「粘贴为值」后重填")
		read.state = fieldBad
	case spreadsheet.CellError:
		d.at(read, ports.TemplateCellErrorValue, "这一格是错误值 %s", cell.Text)
		read.state = fieldBad
	default:
		d.at(read, ports.TemplateCellNotText, "这一格是%s类型的格；请按文本填写", cellKindName(cell.Kind))
		read.state = fieldBad
	}
	return read
}

func cellKindName(kind spreadsheet.CellKind) string {
	switch kind {
	case spreadsheet.CellBoolean:
		return "布尔"
	case spreadsheet.CellDate:
		return "日期"
	default:
		return kind.String()
	}
}

// header 读列键行（第 1 行）。未知列与重复列下的数据由列键行那一条问题兜住，数据格不再逐格报。
// 缺列时整张表不再读，记为结构有问题。
func (d *decoder) header(sheet spreadsheet.Sheet, spec sheetSpec) (map[int]string, map[int]bool, bool) {
	keys := map[int]string{}
	ignored := map[int]bool{}
	seen := map[string]bool{}
	for _, row := range sheet.Rows {
		if row.Number != 1 {
			continue
		}
		for _, cell := range row.Cells {
			name := spreadsheet.ColumnName(cell.Column)
			key := strings.TrimSpace(cell.Text)
			switch {
			case cell.Kind != spreadsheet.CellText || key == "":
				d.report(spec.name, 1, name, ports.TemplateColumnUnknown, "%s 列的列头不是文本列键", name)
				ignored[cell.Column] = true
			case seen[key]:
				d.report(spec.name, 1, key, ports.TemplateColumnDuplicated, "列键 %s 出现了不止一次（%s 列）", key, name)
				ignored[cell.Column] = true
			default:
				if _, _, known := spec.column(key); !known {
					d.report(spec.name, 1, key, ports.TemplateColumnUnknown, "列键 %s 不在本表的列集合里（%s 列）", key, name)
					ignored[cell.Column] = true
					continue
				}
				seen[key] = true
				keys[cell.Column] = key
			}
		}
	}
	complete := true
	for _, expected := range spec.columns {
		if !seen[expected.key] {
			d.report(spec.name, 1, expected.key, ports.TemplateColumnMissing, "缺列键 %s", expected.key)
			complete = false
		}
	}
	if !complete {
		d.broken[spec.name] = true
	}
	return keys, ignored, complete
}

// records 读一张普通表的数据行；整行空白的行跳过。
func (d *decoder) records(sheet spreadsheet.Sheet, spec sheetSpec) []record {
	keys, ignored, complete := d.header(sheet, spec)
	if !complete {
		return nil
	}
	var records []record
	for _, row := range sheet.Rows {
		if row.Number < 2 {
			continue
		}
		rec := record{sheet: spec.name, row: row.Number, cells: map[string]field{}}
		stray := false
		for _, cell := range row.Cells {
			key, known := keys[cell.Column]
			if !known {
				if !ignored[cell.Column] {
					name := spreadsheet.ColumnName(cell.Column)
					d.report(spec.name, row.Number, name, ports.TemplateCellNotApplicable, "%s 列没有列键，却填了值", name)
					stray = true
				}
				continue
			}
			if read := d.readCell(spec.name, row.Number, key, cell); read.state != fieldAbsent {
				rec.cells[key] = read
			}
		}
		if len(rec.cells) == 0 && !stray {
			continue
		}
		for _, expected := range spec.columns {
			if expected.requirement == requiredAlways {
				d.required(rec.get(expected.key))
			}
		}
		records = append(records, rec)
	}
	return records
}

// required 答这一格有没有值；空的报必填格为空，已经被拒的格不再报。
func (d *decoder) required(f field) bool {
	switch f.state {
	case fieldPresent:
		return true
	case fieldAbsent:
		if !d.alreadyReported(f, ports.TemplateCellRequired) {
			d.at(f, ports.TemplateCellRequired, "必填格为空")
		}
	}
	return false
}

func (d *decoder) alreadyReported(f field, code ports.PriceCardTemplateProblemCode) bool {
	for _, problem := range d.problems {
		if problem.Sheet == f.sheet && problem.Row == f.row && problem.Column == f.column && problem.Code == code {
			return true
		}
	}
	return false
}

// mustBeBlank 答这一格是不是空的；填了值的报不适用。
func (d *decoder) mustBeBlank(f field, why string) bool {
	if f.state == fieldPresent {
		d.at(f, ports.TemplateCellNotApplicable, "%s", why)
		return false
	}
	return f.state == fieldAbsent
}

func (d *decoder) code(f field, allowed []string) (string, bool) {
	if !d.required(f) {
		return "", false
	}
	for _, candidate := range allowed {
		if f.text == candidate {
			return f.text, true
		}
	}
	d.at(f, ports.TemplateCellInvalid, "「%s」不是可取值；可取：%s", f.text, strings.Join(allowed, "、"))
	return "", false
}

func (d *decoder) decimal(f field) (domain.Decimal, bool) {
	if !d.required(f) {
		return domain.Decimal{}, false
	}
	value, err := domain.ParseDecimal(f.text)
	if err != nil {
		d.at(f, ports.TemplateCellInvalid, "「%s」不是十进制数（不收科学计数法）", f.text)
		return domain.Decimal{}, false
	}
	return value, true
}

func (d *decoder) timestamp(f field) (time.Time, bool) {
	if !d.required(f) {
		return time.Time{}, false
	}
	value, err := time.Parse(time.RFC3339, f.text)
	if err != nil {
		d.at(f, ports.TemplateCellInvalid, "「%s」不是带时区的时刻，请写成 2026-01-01T00:00:00Z 这样", f.text)
		return time.Time{}, false
	}
	return value, true
}

func (d *decoder) integer(f field) (int, bool) {
	if !d.required(f) {
		return 0, false
	}
	value, err := strconv.Atoi(f.text)
	if err != nil {
		d.at(f, ports.TemplateCellInvalid, "「%s」不是整数", f.text)
		return 0, false
	}
	return value, true
}

// list 读分号分隔的列表：逐个去首尾空白，空元素与重复元素不收，每个元素交 check。
func (d *decoder) list(f field, check func(string) error) ([]string, bool) {
	if !d.required(f) {
		return nil, false
	}
	var items []string
	seen := map[string]bool{}
	for _, part := range strings.Split(f.text, ";") {
		item := strings.TrimSpace(part)
		if item == "" {
			d.at(f, ports.TemplateCellInvalid, "「%s」里有空元素", f.text)
			return nil, false
		}
		if seen[item] {
			d.at(f, ports.TemplateCellInvalid, "「%s」里 %s 重复", f.text, item)
			return nil, false
		}
		if err := check(item); err != nil {
			d.at(f, ports.TemplateCellInvalid, "「%s」里的 %s 立不住：%v", f.text, item, err)
			return nil, false
		}
		seen[item] = true
		items = append(items, item)
	}
	return items, true
}

func (d *decoder) rejected(f field, what string, err error) {
	d.at(f, ports.TemplateConstructionRejected, "%s立不住：%v", what, err)
}

func oneOf(allowed []string) func(string) error {
	return func(item string) error {
		for _, candidate := range allowed {
			if item == candidate {
				return nil
			}
		}
		return fmt.Errorf("可取：%s", strings.Join(allowed, "、"))
	}
}
