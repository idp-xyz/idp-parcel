package registrationjson

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

type chargeAttributionDocument struct {
	TenantID string `json:"tenantId"`
	FeeItem  string `json:"feeItem"`
	Form     string `json:"form"`
}

// ChargeAttributionFromJSON 译装一条费用项目的归属日形态。这一行只有形态：带时区或截单时刻的旧写法按未知字段拒，
// 那两格登在结算账户的对账安排上（ADR-0269 决定二），收下它们就又有了第二份。
func ChargeAttributionFromJSON(raw []byte) (domain.TenantID, domain.ChargeAttributionRegistration, error) {
	var document chargeAttributionDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.ChargeAttributionRegistration{}, fmt.Errorf("归属日形态登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.ChargeAttributionRegistration{}, fmt.Errorf("tenantId：%w", err)
	}
	feeItem, err := domain.NewFeeItemReference(document.FeeItem)
	if err != nil {
		return domain.TenantID{}, domain.ChargeAttributionRegistration{}, fmt.Errorf("feeItem：%w", err)
	}
	form, err := domain.ChargeAttributionFormFromName(document.Form)
	if err != nil {
		return domain.TenantID{}, domain.ChargeAttributionRegistration{}, fmt.Errorf("form：%w", err)
	}
	registration, err := domain.NewChargeAttributionRegistration(feeItem, form)
	if err != nil {
		return domain.TenantID{}, domain.ChargeAttributionRegistration{}, err
	}
	return tenant, registration, nil
}

type statementArrangementDocument struct {
	TenantID     string `json:"tenantId"`
	AccountID    string `json:"accountId"`
	TimeZone     string `json:"timeZone"`
	CutoffMinute *int   `json:"cutoffMinute"`
}

// StatementArrangementFromJSON 译装一版对账安排。不填默认时区或截单时刻：cutoffMinute 缺席与 0 是两回事，0 是
// 午夜这一份登记（ADR-0168 决定二的写法），所以缺席当场拒。
func StatementArrangementFromJSON(raw []byte) (domain.TenantID, domain.StatementArrangement, error) {
	var document statementArrangementDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.StatementArrangement{}, fmt.Errorf("对账安排登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.StatementArrangement{}, fmt.Errorf("tenantId：%w", err)
	}
	account, err := domain.NewSettlementAccountID(document.AccountID)
	if err != nil {
		return domain.TenantID{}, domain.StatementArrangement{}, fmt.Errorf("accountId：%w", err)
	}
	if document.CutoffMinute == nil {
		return domain.TenantID{}, domain.StatementArrangement{}, fmt.Errorf("cutoffMinute：缺席")
	}
	arrangement, err := domain.NewStatementArrangement(account, document.TimeZone, *document.CutoffMinute)
	if err != nil {
		return domain.TenantID{}, domain.StatementArrangement{}, err
	}
	return tenant, arrangement, nil
}
