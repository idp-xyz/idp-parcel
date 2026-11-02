package postgres

import (
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// errDocumentNumberNotRecordable 说明要记号的那一份单据不在册，或已经记过号或无号原因。两者都是编排缺陷：号只在单据刚
// 落库的同一事务里记一次。
var errDocumentNumberNotRecordable = errors.New("transport fulfillment postgres: the document is missing or already carries its number")

// errUnknownDocumentNumberAbsence 说明库里的无号原因不是本上下文认得的取值，或与单据号同时在场——迁移的 CHECK 本该挡住。
var errUnknownDocumentNumberAbsence = errors.New("transport fulfillment postgres: unknown document number absence")

// documentNumberingColumns 把单据上那一格写成两列：有号写号、原因列空；时区未配置写原因、号列空。「本规则生效前开立」
// 不经这里写——它就是两列都空的旧行，新开立的单据不会落到这一格。
func documentNumberingColumns(numbering domain.DocumentNumbering) (*string, *string, error) {
	if number, numbered := numbering.Number(); numbered {
		value := number.String()
		return &value, nil, nil
	}
	absence, _ := numbering.Absence()
	if absence != domain.DocumentNumberingTimeZoneNotConfigured {
		return nil, nil, fmt.Errorf("%w: %s cannot be recorded on a newly issued document", errUnknownDocumentNumberAbsence, absence)
	}
	value := absence.String()
	return nil, &value, nil
}

// documentNumberingFromColumns 读回单据上那一格。两列都空即本规则生效前开立。
func documentNumberingFromColumns(number, absence *string) (domain.DocumentNumbering, error) {
	switch {
	case number != nil && absence == nil:
		value, err := domain.NewDocumentNumber(*number)
		if err != nil {
			return domain.DocumentNumbering{}, err
		}
		return domain.NumberedDocument(value), nil
	case number == nil && absence == nil:
		return domain.UnnumberedDocument(domain.IssuedBeforeDocumentNumbering), nil
	case number == nil && *absence == domain.DocumentNumberingTimeZoneNotConfigured.String():
		return domain.UnnumberedDocument(domain.DocumentNumberingTimeZoneNotConfigured), nil
	default:
		return domain.DocumentNumbering{}, errUnknownDocumentNumberAbsence
	}
}
