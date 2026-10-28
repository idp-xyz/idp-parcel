package registrationjson

import (
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

type statementCycleDocument struct {
	TenantID  string `json:"tenantId"`
	AccountID string `json:"accountId"`
	Kind      string `json:"kind"`
	Weekday   *int   `json:"closingWeekday"`
	MonthDay  *int   `json:"closingMonthDay"`
	Since     string `json:"since"`
}

// StatementCycleFromJSON 译装一户的对账周期（ADR-0339 决定三）。周截的星期几按 0 = 星期日写，月截 0 = 月末；不填就不替租户选
// 一个，周期构造门当场拒。
func StatementCycleFromJSON(raw []byte) (domain.TenantID, domain.StatementCycle, error) {
	var document statementCycleDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.StatementCycle{}, fmt.Errorf("对账周期登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.StatementCycle{}, fmt.Errorf("tenantId：%w", err)
	}
	account, err := domain.NewSettlementAccountID(document.AccountID)
	if err != nil {
		return domain.TenantID{}, domain.StatementCycle{}, fmt.Errorf("accountId：%w", err)
	}
	kind, err := domain.StatementCycleKindFromName(document.Kind)
	if err != nil {
		return domain.TenantID{}, domain.StatementCycle{}, fmt.Errorf("kind：%w", err)
	}
	since, err := domain.ParseAttributionDate(document.Since)
	if err != nil {
		return domain.TenantID{}, domain.StatementCycle{}, fmt.Errorf("since：%w", err)
	}
	var weekday *time.Weekday
	if document.Weekday != nil {
		value := time.Weekday(*document.Weekday)
		weekday = &value
	}
	cycle, err := domain.NewStatementCycle(account, kind, weekday, document.MonthDay, since)
	if err != nil {
		return domain.TenantID{}, domain.StatementCycle{}, err
	}
	return tenant, cycle, nil
}
