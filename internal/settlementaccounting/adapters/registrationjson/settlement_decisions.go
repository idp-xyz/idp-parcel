package registrationjson

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 运营确认口与截单发布口的载荷（ADR-0339 决定一、二）。两口只经操作者渠道进来，租户取认证结果，载荷里没有 tenantId 一格：自报
// 租户按未知字段拒。

type chargeConfirmationDocument struct {
	Charge string `json:"charge"`
}

// ChargeConfirmationFromJSONForTenant 译装一次补拍确认：只点名一笔费用。确认依据、确认事实都在册上，载荷不带。
func ChargeConfirmationFromJSONForTenant(raw []byte, tenant domain.TenantID) (application.ConfirmChargeCommand, error) {
	var document chargeConfirmationDocument
	if err := decodeStrict(raw, &document); err != nil {
		return application.ConfirmChargeCommand{}, fmt.Errorf("费用确认输入不是本入口的形状：%w", err)
	}
	if _, err := domain.NewCustomerChargeID(document.Charge); err != nil {
		return application.ConfirmChargeCommand{}, fmt.Errorf("charge：%w", err)
	}
	return application.ConfirmChargeCommand{TenantID: tenant, ChargeID: document.Charge}, nil
}

type statementPublicationDocument struct {
	Number              string   `json:"number"`
	Account             string   `json:"account"`
	Period              string   `json:"period"`
	Version             string   `json:"version"`
	Currency            string   `json:"currency"`
	Charges             []string `json:"charges"`
	LastAttributionDate string   `json:"lastAttributionDate"`
	DeclaredTotal       string   `json:"declaredTotal"`
	CutOffAt            string   `json:"cutOffAt"`
}

// StatementPublicationFromJSONForTenant 译装一次截单发布。申报合计按最小币单位的整数串写，与客户费用读口的金额同一写法。
// 发布时刻不在载荷里：那是这一动作落下的时刻，由调用方的时钟给。各格的业务判据（归属日、出账、勾稽）都在截单编排里，这里
// 只管形状。
func StatementPublicationFromJSONForTenant(
	raw []byte,
	tenant domain.TenantID,
	publishedAt time.Time,
) (application.PublishStatementCommand, error) {
	var document statementPublicationDocument
	if err := decodeStrict(raw, &document); err != nil {
		return application.PublishStatementCommand{}, fmt.Errorf("截单发布输入不是本入口的形状：%w", err)
	}
	declared, err := plainMinor(document.DeclaredTotal)
	if err != nil {
		return application.PublishStatementCommand{}, fmt.Errorf("declaredTotal：%w", err)
	}
	// 各格的构造门在这里先过一遍：形状写坏的答坏报文，不落进编排去答一个不带原因的未受理。
	for name, check := range map[string]error{
		"number":              firstErr(domain.NewStatementNumber(document.Number)),
		"account":             firstErr(domain.NewSettlementAccountID(document.Account)),
		"period":              firstErr(domain.NewBillingPeriodReference(document.Period)),
		"version":             firstErr(domain.NewStatementDraftVersion(document.Version)),
		"currency":            firstErr(domain.NewCurrencyCode(document.Currency)),
		"lastAttributionDate": firstErr(domain.ParseAttributionDate(document.LastAttributionDate)),
	} {
		if check != nil {
			return application.PublishStatementCommand{}, fmt.Errorf("%s：%w", name, check)
		}
	}
	for _, charge := range document.Charges {
		if _, err := domain.NewCustomerChargeID(charge); err != nil {
			return application.PublishStatementCommand{}, fmt.Errorf("charges：%w", err)
		}
	}
	cutOffAt, err := time.Parse(time.RFC3339, document.CutOffAt)
	if err != nil {
		return application.PublishStatementCommand{}, fmt.Errorf("cutOffAt：%w", err)
	}
	if len(document.Charges) == 0 {
		return application.PublishStatementCommand{}, fmt.Errorf("charges：至少列一笔费用")
	}
	return application.PublishStatementCommand{
		TenantID:            tenant,
		Number:              document.Number,
		Account:             document.Account,
		Period:              document.Period,
		Version:             document.Version,
		Currency:            document.Currency,
		ChargeIDs:           document.Charges,
		LastAttributionDate: document.LastAttributionDate,
		DeclaredTotalMinor:  declared,
		CutOffAt:            cutOffAt.UTC(),
		PublishedAt:         publishedAt.UTC(),
	}, nil
}

// plainMinor 只收不带符号、不带前导零的十进制整数串：「+9600」「09600」与「9600」在勾稽里是同一个数，在审计里却是三种写法。
func plainMinor(value string) (int64, error) {
	if value == "" || (len(value) > 1 && value[0] == '0') || strings.TrimLeft(value, "0123456789") != "" {
		return 0, fmt.Errorf("金额要按最小币单位写不带符号与前导零的整数串")
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("金额：%w", err)
	}
	return parsed, nil
}

func firstErr[T any](_ T, err error) error { return err }
