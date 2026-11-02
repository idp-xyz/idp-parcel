package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 供应商账单与审核应付两本册的详情与列表读口（UC-SA-004；票 document-numbering/11）。判据同本包 catalogue_read.go 头注：
// 照实转写登记册，不重建领域对象、不形成判断；缺席的引用用空串、缺席的时刻用 *time.Time。

// SupplierBillLinePayable 是挂在一行主张上的审核应付：应付标识（调用方生成的幂等键）、应付单号或无号原因（恰有其一，
// ADR-0355 决定十一）、金额、审核授权册上的审核方（AuditorRef）与点下这一次审核的操作者（OperationEvidence，ADR-0340 决定三，
// 为空读作不记提交方）。两者分开：前者说谁有权审核，后者说这一次是谁点的。
type SupplierBillLinePayable struct {
	Payable               string
	DocumentNumber        string
	DocumentNumberAbsence string
	AmountMinor           int64
	Currency              string
	AuditorRef            string
	OperationEvidence     string
	AuditedAt             time.Time
}

// SupplierBillLineEntry 是一行主张连同它的匹配与审核去向。
//
// Classification 是这一行的匹配分类（domain.MatchClassification 的 String()）；空串表示这一行在接收记录上没有匹配——经
// 接收编排进来的每一行都恰有一条匹配，空串只会出现在受控登记口或存量行上，读面如实交空，不补「未匹配」。
// ExpectedVersion 为空是「没有对应发生项」那一格的正面形状。ExpectedMinor 只在点名了预期成本时有意义。
// Payable 为 nil 即这一行还没有审核应付；有差异的行不会有——审核只收已匹配的行。
type SupplierBillLineEntry struct {
	Line            string
	FeeItem         string
	ClaimedMinor    int64
	Classification  string
	ExpectedVersion string
	ExpectedMinor   int64
	Basis           string
	MatchedAt       *time.Time
	Payable         *SupplierBillLinePayable
}

// SupplierBillLines 是一份已接收的供应商账单主张的全貌：主张身份与版本、供应商、法人、账期、币种、到达与落库时刻、被追加的
// 原主张、审核授权是否已配置、接收操作者，以及按主张原序排列的各行。
type SupplierBillLines struct {
	Claim                    string
	ClaimVersion             string
	Supplier                 string
	LegalEntity              string
	Period                   string
	Currency                 string
	ReceivedAt               time.Time
	Supplements              string
	AuditAuthorityConfigured bool
	OperationEvidence        string
	RecordedAt               time.Time
	Lines                    []SupplierBillLineEntry
}

// SupplierBillLineRead 按租户、主张身份与版本读一份账单的各行；不在本租户的册上答 false。
type SupplierBillLineRead interface {
	FindSupplierBillLines(
		ctx context.Context,
		tenant domain.TenantID,
		claim string,
		version string,
	) (SupplierBillLines, bool, error)
}

// AuditedPayableCatalogueRow 是审核应付册上的一笔：应付标识、应付单号或无号原因（恰有其一）、所属主张与行、匹配所引的
// 预期成本版本、法人、应付归集账户、币种与金额、审核授权方、点审核的操作者、审核与落库时刻。
//
// Supplier 与 Period 取自这份主张的接收记录：应付行自己不带供应商与账期，回指的是主张身份与行。同一主张有几个版本时取最早
// 落库的那一版——供应商与账期是主张身份的一部分，各版本不该不同；真不同时这里只说最早那一版，不另造判断。
type AuditedPayableCatalogueRow struct {
	Payable               string
	DocumentNumber        string
	DocumentNumberAbsence string
	Claim                 string
	Line                  string
	ExpectedVersion       string
	Supplier              string
	Period                string
	LegalEntity           string
	Account               string
	Currency              string
	AmountMinor           int64
	AuditorRef            string
	OperationEvidence     string
	AuditedAt             time.Time
	RecordedAt            time.Time
}

// AuditedPayableFilter 是审核应付册的检索维：应付单号、主张身份、供应商，各自精确相等；空串即不按这一维筛，几维同给取交集。
// 只收这几维（票 document-numbering/11）：查阅面不收任何处置参数，理由同本包其余读口。
type AuditedPayableFilter struct {
	DocumentNumber string
	Claim          string
	Supplier       string
}

// AuditedPayableCatalogueRead 按租户上列审核应付，新形成的在前（按审核时刻倒序、同刻按应付标识）；Limit 必须为正。
type AuditedPayableCatalogueRead interface {
	ListAuditedPayables(
		ctx context.Context,
		tenant domain.TenantID,
		filter AuditedPayableFilter,
		limit int,
	) ([]AuditedPayableCatalogueRow, error)
}
