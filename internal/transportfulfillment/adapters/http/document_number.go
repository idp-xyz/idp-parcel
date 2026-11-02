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
