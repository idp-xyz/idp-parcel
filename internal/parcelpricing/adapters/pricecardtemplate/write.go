package pricecardtemplate

import (
	"io"

	"go.idp.xyz/idp-parcel/internal/platform/spreadsheet"
)

// TemplateFileName 是空白模板的文件名，带模板版本：版本进位时文件名跟着换，旧文件不会冒充新模板。
const TemplateFileName = "price-card-import-template-" + TemplateVersion + ".xlsx"

// WriteTemplate 写出空白模板：每张表一行列键，所有列预设为文本格；card 表预先列好全部字段名，
// 只有 templateVersion 预填（那是模板自己的版本，不是业务值），其余值留空；末尾是说明表。
// 不预填任何业务值，同 ADR-0089「模板属机制半边，模板里的取值属实例半边」。
func WriteTemplate(w io.Writer) error {
	out := make([]spreadsheet.WriteSheet, 0, len(sheets)+1)
	for _, spec := range sheets {
		sheet := spreadsheet.WriteSheet{Name: spec.name, Columns: textColumns(spec.columns)}
		if spec.name == sheetCard {
			for _, entry := range cardFields {
				value := ""
				if entry.key == "templateVersion" {
					value = TemplateVersion
				}
				sheet.Rows = append(sheet.Rows, []string{entry.key, value})
			}
		}
		out = append(out, sheet)
	}
	out = append(out, readme())
	return spreadsheet.Write(w, out)
}

func textColumns(columns []column) []spreadsheet.WriteColumn {
	written := make([]spreadsheet.WriteColumn, len(columns))
	for index, entry := range columns {
		written[index] = spreadsheet.WriteColumn{Header: entry.key, Text: true, Width: 24}
	}
	return written
}

// readme 由同一张列定义生成：列键、必填性与说明都不另写一份。
func readme() spreadsheet.WriteSheet {
	rows := [][]string{
		{"", "", "", "价卡导入模板 " + TemplateVersion + "。本表解析时跳过，可以不动。"},
		{"", "", "", "每张表第 1 行是列键，不要改名、不要删表；用不到的表留空即可。整行空白的行会被跳过。"},
		{"", "", "", "所有格按文本填写，不要用公式。金额、小数、重量写十进制数，不用科学计数法；单位与币种由列或表头给出。"},
		{"", "", "", "时刻写成带时区的形式，如 2026-01-01T00:00:00Z 或 2026-01-01T08:00:00+08:00。"},
		{"", "", "", "列表格以英文分号 ; 分隔。封闭代码照下面列出的原样书写。"},
	}
	for _, spec := range sheets {
		rows = append(rows, []string{spec.name, "", "", spec.note})
		entries := spec.columns
		if spec.name == sheetCard {
			entries = cardFields
		}
		for _, entry := range entries {
			rows = append(rows, []string{spec.name, entry.key, entry.requirement.String(), entry.note})
		}
	}
	return spreadsheet.WriteSheet{
		Name: sheetReadme,
		Columns: []spreadsheet.WriteColumn{
			{Header: "表", Width: 24}, {Header: "列或字段", Width: 30}, {Header: "必填", Width: 10}, {Header: "说明", Width: 100},
		},
		Rows: rows,
	}
}
