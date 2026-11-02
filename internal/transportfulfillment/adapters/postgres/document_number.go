package postgres

import (
	"context"
	"errors"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

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

// referencedDocumentNumberings 连读一批被引用的委托或订舱的单据号格（ADR-0355 决定十一）：引用委托、订舱的各册读回时用它给
// 引用带号，答复与查阅页以号为主、标识另列。只读不在场就不进结果——引用指向的单据不在册（总单的引用不核在册）时，读口照旧只带
// 标识，不替它编一个「上线前开立」。table 与 idColumn 只取本文件的两组常量，不拼外来串。
func referencedDocumentNumberings(
	ctx context.Context,
	querier bentopg.Querier,
	table, idColumn string,
	tenant domain.TenantID,
	ids []string,
) (map[string]domain.DocumentNumbering, error) {
	numberings := map[string]domain.DocumentNumbering{}
	if len(ids) == 0 {
		return numberings, nil
	}
	rows, err := querier.Query(ctx,
		`SELECT `+idColumn+`, document_number, document_number_absence
		   FROM transport_fulfillment.`+table+`
		  WHERE tenant_id = $1
		    AND `+idColumn+` = ANY($2)`,
		tenant.String(), ids,
	)
	if err != nil {
		return nil, fmt.Errorf("read referenced document numbers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var number, absence *string
		if err := rows.Scan(&id, &number, &absence); err != nil {
			return nil, fmt.Errorf("read referenced document numbers: %w", err)
		}
		numbering, err := documentNumberingFromColumns(number, absence)
		if err != nil {
			return nil, fmt.Errorf("read referenced document numbers: %s %q: %w", table, id, err)
		}
		numberings[id] = numbering
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read referenced document numbers: %w", err)
	}
	return numberings, nil
}

// commissionDocumentNumberings 连读一批委托的单据号格，键是委托标识。
func commissionDocumentNumberings(ctx context.Context, querier bentopg.Querier, tenant domain.TenantID, ids []string) (map[string]domain.DocumentNumbering, error) {
	return referencedDocumentNumberings(ctx, querier, "transport_commission", "commission_id", tenant, ids)
}

// bookingDocumentNumberings 连读一批订舱的单据号格，键是订舱标识。
func bookingDocumentNumberings(ctx context.Context, querier bentopg.Querier, tenant domain.TenantID, ids []string) (map[string]domain.DocumentNumbering, error) {
	return referencedDocumentNumberings(ctx, querier, "booking_request", "booking_id", tenant, ids)
}

// referencedNumbering 从连读结果里取一个引用的号格；引用为空或单据不在册答 nil。
func referencedNumbering(numberings map[string]domain.DocumentNumbering, id string) *domain.DocumentNumbering {
	numbering, found := numberings[id]
	if id == "" || !found {
		return nil
	}
	return &numbering
}
