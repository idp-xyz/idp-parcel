package tfhttp

import "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"

// documentNumberKeys 是委托、订舱线格式上单据号那一格的两键（ADR-0355 决定十一）。有号只出 `documentNumber`；没有号只出
// `documentNumberAbsence`，取 `ISSUED_BEFORE_NUMBERING`（本规则生效前开立）或 `NUMBERING_TIME_ZONE_NOT_CONFIGURED`（开立时
// 单据编号时区未配置）——不答空串，也不拿内部标识顶，页面据原因分别注明。
type documentNumberKeys struct {
	DocumentNumber        string `json:"documentNumber,omitempty"`
	DocumentNumberAbsence string `json:"documentNumberAbsence,omitempty"`
}

func documentNumberKeysOf(numbering domain.DocumentNumbering) documentNumberKeys {
	if number, numbered := numbering.Number(); numbered {
		return documentNumberKeys{DocumentNumber: number.String()}
	}
	absence, _ := numbering.Absence()
	return documentNumberKeys{DocumentNumberAbsence: absence.String()}
}

// commissionNumberKeys 是答复与读口里一个委托引用旁边的单据号两键（ADR-0355 决定十一），命名取「所属单据 + DocumentNumber」：
// 引用的委托有号只出 `commissionDocumentNumber`，没号只出 `commissionDocumentNumberAbsence`；引用的委托没读到时两键都不出，
// 读的人照旧拿 `commission` 那一格的标识。新增字段，原有的引用格不动。
type commissionNumberKeys struct {
	CommissionDocumentNumber        string `json:"commissionDocumentNumber,omitempty"`
	CommissionDocumentNumberAbsence string `json:"commissionDocumentNumberAbsence,omitempty"`
}

func commissionNumberKeysOf(numbering *domain.DocumentNumbering) commissionNumberKeys {
	if numbering == nil {
		return commissionNumberKeys{}
	}
	keys := documentNumberKeysOf(*numbering)
	return commissionNumberKeys{CommissionDocumentNumber: keys.DocumentNumber, CommissionDocumentNumberAbsence: keys.DocumentNumberAbsence}
}

// bookingNumberKeys 是一个订舱引用旁边的单据号两键，读法同 commissionNumberKeys。
type bookingNumberKeys struct {
	BookingDocumentNumber        string `json:"bookingDocumentNumber,omitempty"`
	BookingDocumentNumberAbsence string `json:"bookingDocumentNumberAbsence,omitempty"`
}

func bookingNumberKeysOf(numbering *domain.DocumentNumbering) bookingNumberKeys {
	if numbering == nil {
		return bookingNumberKeys{}
	}
	keys := documentNumberKeysOf(*numbering)
	return bookingNumberKeys{BookingDocumentNumber: keys.DocumentNumber, BookingDocumentNumberAbsence: keys.DocumentNumberAbsence}
}
