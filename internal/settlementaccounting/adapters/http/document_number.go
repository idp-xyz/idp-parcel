package settlementhttp

import "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"

// numberingWords 把单据上「号或无号原因」那一格写成答复的两格：有号时只出号，没号时只出原因（ADR-0355 决定十一：
// 没有单据号的不答空串、不拿内部标识顶）。
func numberingWords(numbering domain.DocumentNumbering) (string, string) {
	if number, numbered := numbering.Number(); numbered {
		return number.String(), ""
	}
	absence, _ := numbering.Absence()
	return "", absence.String()
}
