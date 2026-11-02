package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 费用争议与审核应付的单据号落在各自表上的两列（迁移 settlement_accounting/0053）：document_number 与
// document_number_absence。有号时只有前一列；开立时单据编号时区未配置，只有后一列；两列都空是本决定之前开立的
// 旧行，读作上线前开立——旧行不回填，所以「上线前开立」不写进库，只由两列都空表达。

// numberingColumns 把单据上那一格拆成两列的写入值。新写的行不会是上线前开立：取号那一步只给出有号或时区未配置，
// 交来上线前开立说明编排漏接了取号，拒写而不是悄悄落成一张旧单。
func numberingColumns(numbering domain.DocumentNumbering) (*string, *string, error) {
	if number, numbered := numbering.Number(); numbered {
		value := number.String()
		return &value, nil, nil
	}
	if absence, absent := numbering.Absence(); absent && absence == domain.NumberingTimeZoneUnconfigured {
		value := absence.String()
		return nil, &value, nil
	}
	return nil, nil, fmt.Errorf("a newly issued document must carry a document number or the reason it has none")
}

// numberingFrom 把两列读回那一格。
func numberingFrom(number, absence *string) (domain.DocumentNumbering, error) {
	switch {
	case number != nil && absence == nil:
		value, err := domain.NewDocumentNumber(*number)
		if err != nil {
			return domain.DocumentNumbering{}, err
		}
		return domain.NumberedDocument(value)
	case number == nil && absence == nil:
		return domain.UnnumberedDocument(domain.IssuedBeforeNumbering)
	case number == nil && *absence == domain.NumberingTimeZoneUnconfigured.String():
		return domain.UnnumberedDocument(domain.NumberingTimeZoneUnconfigured)
	default:
		return domain.DocumentNumbering{}, fmt.Errorf("document number columns disagree")
	}
}

// documentNumberUsed 答一个单据号是否已在本租户的这张表上。table 只由本包的常量传入。取号编排只在开立事务里调它（事务外先被
// 编号组件拒），读执行器随 ctx 落在那笔事务上，看得见本事务里已落下的单。
func documentNumberUsed(ctx context.Context, db *bentopg.DB, table string, tenant domain.TenantID, number string) (bool, error) {
	executor, err := db.ReadExecutor(ctx)
	if err != nil {
		return false, err
	}
	var used bool
	if err := executor.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM `+table+` WHERE tenant_id = $1 AND document_number = $2)`,
		tenant.String(), number).Scan(&used); err != nil {
		return false, err
	}
	return used, nil
}
