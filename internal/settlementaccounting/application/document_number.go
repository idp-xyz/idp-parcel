package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// issueDocumentNumber 在开立事务里取一个单据号，译成单据上「号或无号原因」那一格（ADR-0355 决定三、七、八）。交回的
// 布尔为真是编号规则撞号：这一次不落单，调用方答撞号，开立事务必须回滚。交回错误时同样回滚。
//
// 时区未配置不是错误：单据照常开立、不带号，原因记在单据上，事后不补。对账单不走这里——它的单号是寻址键、非空，
// 落不到这一格（见 Publish）。
func issueDocumentNumber(
	ctx context.Context,
	numbers ports.DocumentNumberAssigner,
	request ports.DocumentNumberRequest,
) (domain.DocumentNumbering, bool, error) {
	assigned, err := numbers.AssignDocumentNumber(ctx, request)
	if err != nil {
		return domain.DocumentNumbering{}, false, err
	}
	switch assigned.Outcome {
	case ports.DocumentNumberAssigned:
		number, err := domain.NewDocumentNumber(assigned.Number)
		if err != nil {
			return domain.DocumentNumbering{}, false, err
		}
		numbering, err := domain.NumberedDocument(number)
		return numbering, false, err
	case ports.DocumentNumberTimeZoneUnconfigured:
		numbering, err := domain.UnnumberedDocument(domain.NumberingTimeZoneUnconfigured)
		return numbering, false, err
	case ports.DocumentNumberRuleCollision:
		return domain.DocumentNumbering{}, true, nil
	default:
		return domain.DocumentNumbering{}, false, fmt.Errorf("settlement accounting: document numbering answered an unnamed outcome %d", assigned.Outcome)
	}
}
