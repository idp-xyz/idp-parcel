package application

import (
	"bytes"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/spreadsheet"
)

// customerRateSheet 在对客价格表前加封面。封面只有报价号、相对方、责任法人、有效期与条款。
// 价格表本身由计价投影交来，这里不改其中的金额格。
func customerRateSheet(send domain.QuotationSend, price []byte) ([]byte, string, error) {
	book, err := spreadsheet.Read(price, spreadsheet.Limits{
		MaxFileBytes: 10 << 20, MaxPartBytes: 64 << 20, MaxRowsPerSheet: 100000,
	})
	if err != nil {
		return nil, "", fmt.Errorf("party commercial: read disclosed rate sheet: %w", err)
	}
	sheets := make([]spreadsheet.WriteSheet, 0, 1+len(book.Sheets))
	sheets = append(sheets, quotationCover(send))
	for _, sheet := range book.Sheets {
		converted, convErr := disclosedSheet(sheet)
		if convErr != nil {
			return nil, "", convErr
		}
		sheets = append(sheets, converted)
	}
	var buffer bytes.Buffer
	if err := spreadsheet.Write(&buffer, sheets); err != nil {
		return nil, "", fmt.Errorf("party commercial: write customer rate sheet: %w", err)
	}
	return buffer.Bytes(), rateSheetFileName(send.ID().String()), nil
}

func quotationCover(send domain.QuotationSend) spreadsheet.WriteSheet {
	terms := send.Terms().Text()
	if terms == "" {
		terms = "没有另加条款"
	}
	rows := [][]string{
		{"报价号", send.ID().String()},
		{"相对方", send.CounterpartyName().String()},
		{"责任法人", send.LegalEntityName().String()},
		{"有效期起点（含）", send.Validity().StartsAt().Format(time.RFC3339)},
		{"有效期终点（不含）", send.Validity().EndsAt().Format(time.RFC3339)},
		{"条款", terms},
	}
	return spreadsheet.WriteSheet{
		Name: "封面",
		Columns: []spreadsheet.WriteColumn{
			{Header: "项目", Text: true, Width: 24},
			{Header: "内容", Text: true, Width: 48},
		},
		Rows: rows,
	}
}

func disclosedSheet(sheet spreadsheet.Sheet) (spreadsheet.WriteSheet, error) {
	if len(sheet.Rows) == 0 {
		return spreadsheet.WriteSheet{Name: sheet.Name}, nil
	}
	width := 0
	for _, row := range sheet.Rows {
		for _, cell := range row.Cells {
			if cell.Column+1 > width {
				width = cell.Column + 1
			}
		}
	}
	if sheet.Rows[0].Number != 1 {
		return spreadsheet.WriteSheet{}, fmt.Errorf("party commercial: rate sheet %s has no header row", sheet.Name)
	}
	headers := rowText(sheet.Rows[0], width)
	columns := make([]spreadsheet.WriteColumn, len(headers))
	for index, header := range headers {
		columns[index] = spreadsheet.WriteColumn{Header: header, Text: true}
	}
	rows := make([][]string, 0, len(sheet.Rows)-1)
	for _, row := range sheet.Rows[1:] {
		rows = append(rows, rowText(row, width))
	}
	return spreadsheet.WriteSheet{Name: sheet.Name, Columns: columns, Rows: rows}, nil
}

func rowText(row spreadsheet.Row, width int) []string {
	values := make([]string, width)
	for _, cell := range row.Cells {
		if cell.Column >= 0 && cell.Column < width {
			values[cell.Column] = cell.Text
		}
	}
	return values
}

func rateSheetFileName(id string) string {
	for _, character := range id {
		switch {
		case character >= 'A' && character <= 'Z', character >= 'a' && character <= 'z', character >= '0' && character <= '9':
		case character == '.' || character == '_' || character == '-':
		default:
			return "rate-sheet.xlsx"
		}
	}
	if id == "" {
		return "rate-sheet.xlsx"
	}
	return "rate-sheet-" + id + ".xlsx"
}
