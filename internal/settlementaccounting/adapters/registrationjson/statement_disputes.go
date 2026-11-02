package registrationjson

import (
	"fmt"
	"time"
	"unicode/utf8"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 登记争议口与处理争议口的载荷（ADR-0353 决定一、二）。两口只经操作者渠道进来，租户取认证结果，载荷里没有 tenantId 一格：自报
// 租户按未知字段拒。争议标识由调用方给：同一次登记重发要落回同一项，标识就得在第一次发出之前定下来。

// maxDisputeTextRunes 是理由与处理依据的字数上限。这两格是人写的话（客户怎么说、核实了什么），不是附件；写不下就该附证据另存，
// 不把整封邮件塞进一格。
const maxDisputeTextRunes = 1000

type statementDisputeDocument struct {
	Dispute        string `json:"dispute"`
	Statement      string `json:"statement"`
	Charge         string `json:"charge"`
	DisputedAmount string `json:"disputedAmount"`
	Reason         string `json:"reason"`
	OpenedAt       string `json:"openedAt"`
}

// StatementDisputeFromJSONForTenant 译装一次代登记：点名单、费用、争议金额（最小币单位的整数串，与对账单读口同一写法）、理由，
// 以及客户提出异议的时刻。这一格是业务时刻（客户来电那一刻），不是录入时刻。单在不在、费用在不在单上、金额越没越行、单作没作废，
// 都在编排里判，这里只管形状。
func StatementDisputeFromJSONForTenant(raw []byte, tenant domain.TenantID) (application.OpenDisputeCommand, error) {
	var document statementDisputeDocument
	if err := decodeStrict(raw, &document); err != nil {
		return application.OpenDisputeCommand{}, fmt.Errorf("争议登记输入不是本入口的形状：%w", err)
	}
	for name, check := range map[string]error{
		"dispute":   firstErr(domain.NewDisputeID(document.Dispute)),
		"statement": firstErr(domain.NewStatementNumber(document.Statement)),
		"charge":    firstErr(domain.NewCustomerChargeID(document.Charge)),
		"reason":    disputeText(document.Reason),
	} {
		if check != nil {
			return application.OpenDisputeCommand{}, fmt.Errorf("%s：%w", name, check)
		}
	}
	amount, err := plainMinor(document.DisputedAmount)
	if err != nil {
		return application.OpenDisputeCommand{}, fmt.Errorf("disputedAmount：%w", err)
	}
	if amount == 0 {
		return application.OpenDisputeCommand{}, fmt.Errorf("disputedAmount：争议金额要大于零")
	}
	openedAt, err := time.Parse(time.RFC3339, document.OpenedAt)
	if err != nil {
		return application.OpenDisputeCommand{}, fmt.Errorf("openedAt：%w", err)
	}
	return application.OpenDisputeCommand{
		TenantID:      tenant,
		Dispute:       document.Dispute,
		Number:        document.Statement,
		ChargeID:      document.Charge,
		DisputedMinor: amount,
		Reason:        document.Reason,
		OpenedAt:      openedAt.UTC(),
	}, nil
}

type disputeResolutionDocument struct {
	Dispute    string `json:"dispute"`
	Resolution string `json:"resolution"`
	Basis      string `json:"basis"`
	ResolvedAt string `json:"resolvedAt"`
}

// DisputeResolutionFromJSONForTenant 译装一次处理：点名争议、走向（接受、部分接受、拒绝、待复核）、依据与形成处理的时刻。处理
// 不带金额：接受与部分接受只产生请金额所有者复核的依据，金额调整由各自的唯一创建用例形成（UC-SA-003「争议与既有调整纳入」）。
func DisputeResolutionFromJSONForTenant(raw []byte, tenant domain.TenantID) (application.ResolveDisputeCommand, error) {
	var document disputeResolutionDocument
	if err := decodeStrict(raw, &document); err != nil {
		return application.ResolveDisputeCommand{}, fmt.Errorf("争议处理输入不是本入口的形状：%w", err)
	}
	if _, err := domain.NewDisputeID(document.Dispute); err != nil {
		return application.ResolveDisputeCommand{}, fmt.Errorf("dispute：%w", err)
	}
	kind, err := domain.DisputeResolutionKindFromName(document.Resolution)
	if err != nil {
		return application.ResolveDisputeCommand{}, fmt.Errorf("resolution：%w", err)
	}
	if err := disputeText(document.Basis); err != nil {
		return application.ResolveDisputeCommand{}, fmt.Errorf("basis：%w", err)
	}
	resolvedAt, err := time.Parse(time.RFC3339, document.ResolvedAt)
	if err != nil {
		return application.ResolveDisputeCommand{}, fmt.Errorf("resolvedAt：%w", err)
	}
	return application.ResolveDisputeCommand{
		TenantID:   tenant,
		Dispute:    document.Dispute,
		Kind:       kind,
		Basis:      document.Basis,
		ResolvedAt: resolvedAt.UTC(),
	}, nil
}

func disputeText(value string) error {
	if _, err := domain.NewDisputeBasisReference(value); err != nil {
		return err
	}
	if utf8.RuneCountInString(value) > maxDisputeTextRunes {
		return fmt.Errorf("超过 %d 字", maxDisputeTextRunes)
	}
	return nil
}
