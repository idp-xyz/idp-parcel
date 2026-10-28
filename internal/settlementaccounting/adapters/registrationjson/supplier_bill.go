package registrationjson

import (
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 供应商账单接收口与审核口的载荷（ADR-0340 决定一、二）。两口只经操作者渠道进来，租户取认证结果，载荷里没有 tenantId 一格：自报
// 租户按未知字段拒。金额按最小币单位的整数串写，与客户费用读口、截单发布口同一写法。

type supplierBillLineDocument struct {
	Line    string `json:"line"`
	FeeItem string `json:"feeItem"`
	Claimed string `json:"claimed"`
}

type supplierBillDirectiveDocument struct {
	Line            string `json:"line"`
	Classification  string `json:"classification"`
	ExpectedVersion string `json:"expectedVersion"`
	Basis           string `json:"basis"`
}

type supplierBillReceptionDocument struct {
	Claim            string                          `json:"claim"`
	Version          string                          `json:"version"`
	Supplier         string                          `json:"supplier"`
	LegalEntity      string                          `json:"legalEntity"`
	Period           string                          `json:"period"`
	Currency         string                          `json:"currency"`
	Lines            []supplierBillLineDocument      `json:"lines"`
	ReceivedAt       string                          `json:"receivedAt"`
	SupplementsClaim string                          `json:"supplementsClaim"`
	Directives       []supplierBillDirectiveDocument `json:"directives"`
}

// SupplierBillReceptionFromJSONForTenant 译装一份账单与它的逐行匹配裁决。receivedAt 是账单到达的业务时刻，由录入的人照来件写；
// 逐行裁决的分类只收产品认得的几种。哪一行该有裁决、裁决指的预期成本是不是当前版，由接收编排判。
func SupplierBillReceptionFromJSONForTenant(raw []byte, tenant domain.TenantID) (application.ReceiveSupplierBillCommand, error) {
	none := application.ReceiveSupplierBillCommand{}
	var document supplierBillReceptionDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("供应商账单接收输入不是本入口的形状：%w", err)
	}
	spec := domain.SupplierBillClaimSpec{}
	var err error
	if spec.Claim, err = domain.NewBillClaimID(document.Claim); err != nil {
		return none, fmt.Errorf("claim：%w", err)
	}
	if spec.Version, err = domain.NewBillClaimVersion(document.Version); err != nil {
		return none, fmt.Errorf("version：%w", err)
	}
	if spec.Supplier, err = domain.NewSupplierPartyReference(document.Supplier); err != nil {
		return none, fmt.Errorf("supplier：%w", err)
	}
	if spec.LegalEntity, err = domain.NewLegalEntityReference(document.LegalEntity); err != nil {
		return none, fmt.Errorf("legalEntity：%w", err)
	}
	if spec.Period, err = domain.NewBillingPeriodReference(document.Period); err != nil {
		return none, fmt.Errorf("period：%w", err)
	}
	if spec.Currency, err = domain.NewCurrencyCode(document.Currency); err != nil {
		return none, fmt.Errorf("currency：%w", err)
	}
	if spec.ReceivedAt, err = time.Parse(time.RFC3339, document.ReceivedAt); err != nil {
		return none, fmt.Errorf("receivedAt：%w", err)
	}
	spec.ReceivedAt = spec.ReceivedAt.UTC()
	if document.SupplementsClaim != "" {
		if spec.SupplementsClaim, err = domain.NewBillClaimID(document.SupplementsClaim); err != nil {
			return none, fmt.Errorf("supplementsClaim：%w", err)
		}
	}
	if len(document.Lines) == 0 {
		return none, fmt.Errorf("lines：至少一行")
	}
	for _, line := range document.Lines {
		parsed := domain.BillLine{}
		if parsed.Line, err = domain.NewBillLineReference(line.Line); err != nil {
			return none, fmt.Errorf("lines.line：%w", err)
		}
		if parsed.FeeItem, err = domain.NewFeeItemReference(line.FeeItem); err != nil {
			return none, fmt.Errorf("lines.feeItem：%w", err)
		}
		if parsed.ClaimedMinor, err = plainMinor(line.Claimed); err != nil {
			return none, fmt.Errorf("lines.claimed：%w", err)
		}
		spec.Lines = append(spec.Lines, parsed)
	}
	command := application.ReceiveSupplierBillCommand{TenantID: tenant, Claim: spec}
	for _, directive := range document.Directives {
		parsed := application.LineMatchDirective{}
		if parsed.Line, err = domain.NewBillLineReference(directive.Line); err != nil {
			return none, fmt.Errorf("directives.line：%w", err)
		}
		if parsed.Classification, err = domain.MatchClassificationFromName(directive.Classification); err != nil {
			return none, fmt.Errorf("directives.classification：%w", err)
		}
		if directive.ExpectedVersion != "" {
			if parsed.ExpectedVersion, err = domain.NewSupplierCostVersionID(directive.ExpectedVersion); err != nil {
				return none, fmt.Errorf("directives.expectedVersion：%w", err)
			}
		}
		// 匹配依据只给差异与重复计费几格；完全匹配不带，带了由匹配门拒。
		if directive.Basis != "" {
			if parsed.Basis, err = domain.NewMatchBasisReference(directive.Basis); err != nil {
				return none, fmt.Errorf("directives.basis：%w", err)
			}
		}
		command.Directives = append(command.Directives, parsed)
	}
	return command, nil
}

type supplierBillAuditDocument struct {
	Claim   string `json:"claim"`
	Version string `json:"version"`
	Line    string `json:"line"`
	Payable string `json:"payable"`
}

// SupplierBillAuditFromJSONForTenant 译装一次审核：点名账单、版本、行，与要形成的应付标识。金额、上限、归集账户都在册上，载荷不带。
func SupplierBillAuditFromJSONForTenant(raw []byte, tenant domain.TenantID) (application.AuditBillLineCommand, error) {
	none := application.AuditBillLineCommand{}
	var document supplierBillAuditDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("供应商账单审核输入不是本入口的形状：%w", err)
	}
	command := application.AuditBillLineCommand{TenantID: tenant}
	var err error
	if command.Claim, err = domain.NewBillClaimID(document.Claim); err != nil {
		return none, fmt.Errorf("claim：%w", err)
	}
	if command.Version, err = domain.NewBillClaimVersion(document.Version); err != nil {
		return none, fmt.Errorf("version：%w", err)
	}
	if command.Line, err = domain.NewBillLineReference(document.Line); err != nil {
		return none, fmt.Errorf("line：%w", err)
	}
	if command.Payable, err = domain.NewPayableID(document.Payable); err != nil {
		return none, fmt.Errorf("payable：%w", err)
	}
	return command, nil
}
