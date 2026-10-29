package pricecardtemplate

import "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"

var _ ports.PriceCardTemplateSpecSource = Reader{}

// PriceCardTemplateSpec 见 ports.PriceCardTemplateSpecSource。规格整个由 sheets、cardFields 与 closedVocabulary 生成，
// 不手写任何一张表或一列：读取模板的定义改了，规格口答的跟着改。它答最新一版（TemplateVersion）的全貌，封闭取值
// 取最新一版里收的那些；旧版文件的读法由 acceptedTemplateVersions 另报，不在规格里复述。
func (Reader) PriceCardTemplateSpec() ports.PriceCardTemplateSpec {
	spec := ports.PriceCardTemplateSpec{
		TemplateVersion:          TemplateVersion,
		AcceptedTemplateVersions: append([]string(nil), acceptedTemplateVersions...),
		Sheets:                   make([]ports.PriceCardTemplateSheetSpec, 0, len(sheets)),
	}
	for _, sheet := range sheets {
		entries := sheet.columns
		layout := "ROWS"
		if sheet.name == sheetCard {
			// card 表的列是 field / value 两列，对外要画表单的是它的字段；工作簿里照旧写成 {field, value} 行。
			entries, layout = cardFields, "FIELDS"
		}
		out := ports.PriceCardTemplateSheetSpec{
			Name: sheet.name, Title: sheet.title, Purpose: sheet.note, Layout: layout, Repeatable: !sheet.single, Since: sheet.since,
			Columns: make([]ports.PriceCardTemplateColumnSpec, 0, len(entries)),
		}
		for _, entry := range entries {
			column := ports.PriceCardTemplateColumnSpec{
				Key: entry.key, Title: entry.title, Requirement: entry.requirement.code(), Description: entry.note,
			}
			if vocab, closed := closedVocabulary[sheet.name][entry.key]; closed {
				column.AllowedValues = append([]string(nil), vocab.valuesFor(TemplateVersion)...)
				column.MultiValued = vocab.multi
			}
			out.Columns = append(out.Columns, column)
		}
		spec.Sheets = append(spec.Sheets, out)
	}
	return spec
}
