package postgres

import "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"

// 运营决定的操作证据列（ADR-0303；迁移 TF 0051）在各登记册上同一种落法：缺席落 NULL，不落空串——空串会让「没记下
// 操作者」与「记下了一位空名的操作者」在库面长成两张脸，0051 的 CHECK 也拒空白。

// optionalRegistrant 把可缺的操作证据落成可空列。
func optionalRegistrant(reference domain.RegistrantReference) *string {
	if !reference.Present() {
		return nil
	}
	value := reference.String()
	return &value
}

// registrantFromColumn 读回可空的操作证据列，走构造门：库面绕过 CHECK 写进的空白在这里响亮。
func registrantFromColumn(column *string) (domain.RegistrantReference, error) {
	if column == nil {
		return domain.RegistrantReference{}, nil
	}
	return domain.NewRegistrantReference(*column)
}
