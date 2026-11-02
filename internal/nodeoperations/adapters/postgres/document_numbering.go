package postgres

import (
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// errDocumentNumberingUndecided 说明交来落册的单据还没定单据号：开立编排漏了取号那一步。两格都空的新行读回来就成了「上线前开立」，
// 册拒收而不是替它落成那样（迁移 0026 头注）。
var errDocumentNumberingUndecided = errors.New("the document number of a newly issued document is not decided")

// documentNumberingColumns 把单据号这一格译成库里那两列：有号写号，单据编号时区未配置写原因。上线前开立只从读回来，开立写不出它。
func documentNumberingColumns(numbering domain.DocumentNumbering) (number, absence *string, err error) {
	if value, numbered := numbering.Number(); numbered {
		text := value.String()
		return &text, nil, nil
	}
	if reason, absent := numbering.Absence(); absent && reason == domain.NumberingTimeZoneNotConfigured {
		text := reason.String()
		return nil, &text, nil
	}
	if reason, absent := numbering.Absence(); absent {
		return nil, nil, fmt.Errorf("%w: absence %s cannot be written by an issuing path", errDocumentNumberingUndecided, reason)
	}
	return nil, nil, errDocumentNumberingUndecided
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
