package pricecardtemplate

import (
	"errors"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/spreadsheet"
)

// 规范第三节的技术上限：防御性的，不是业务取值。
const (
	MaxFileBytes    = 10 << 20
	maxPartBytes    = 64 << 20
	maxRowsPerSheet = 100000
)

// Reader 是 ports.PriceCardTemplateReader 的实现。
type Reader struct{}

var _ ports.PriceCardTemplateReader = Reader{}

func (Reader) ReadPriceCardTemplate(raw []byte) ports.PriceCardTemplateReading {
	book, err := spreadsheet.Read(raw, spreadsheet.Limits{
		MaxFileBytes: MaxFileBytes, MaxPartBytes: maxPartBytes, MaxRowsPerSheet: maxRowsPerSheet,
	})
	switch {
	case errors.Is(err, spreadsheet.ErrTooLarge):
		return rejectedReading(ports.TemplateFileTooLarge,
			"文件超过技术上限（文件 10 MiB、解压后 64 MiB、单表 100 000 行）：%v", err)
	case err != nil:
		return rejectedReading(ports.TemplateFileNotWorkbook,
			"读不出这份文件（%v）；请在 Excel 或 WPS 里另存为「Excel 工作簿（*.xlsx）」", err)
	}
	return decodeWorkbook(book)
}
