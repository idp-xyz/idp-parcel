package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 供应商账单与审核应付两本册的详情与列表读口（UC-SA-004；票 document-numbering/11、13）。判据同本包 catalogue_read.go 头注：
// 照实转写登记册，不形成判断；缺席的引用用空串、缺席的时刻用 *time.Time。各行的已匹配金额与四个和是例外里的求和：取自领域
// 那一处定义（domain.MatchedAmountFor、domain.TallyLineDecisions），读面不另写一份求法。

// SupplierBillLinePayable 是一份通过的审核决定形成的审核应付：应付标识（就是决定标识，ADR-0357 决定三）、应付单号或无号原因
// （恰有其一，ADR-0355 决定十一）与金额币种。
type SupplierBillLinePayable struct {
	Payable               string
	DocumentNumber        string
	DocumentNumberAbsence string
	AmountMinor           int64
	Currency              string
}

// SupplierBillLineDecision 是一行主张上的一份审核决定（ADR-0357）：结果、这一行主张金额上的一段 [RangeFromMinor, RangeToMinor)、
// 依据、所解的争议（再审才有）、审的时候点名的主张版本（迁移回填的旧应付为空）、审核授权册上的审核方（AuditorRef）与点下这一次
// 的操作者（OperationEvidence，ADR-0340 决定三，为空读作不记提交方）——前者说谁有权审核，后者说这一次是谁点的。通过的那一份
// 带它形成的审核应付；读不到那份应付时 Payable 为 nil，如实交空。
type SupplierBillLineDecision struct {
	Decision          string
	Verdict           string
	RangeFromMinor    int64
	RangeToMinor      int64
	Basis             string
	ResolvesDispute   string
	ClaimVersion      string
	AuditorRef        string
	OperationEvidence string
	AuditedAt         time.Time
	Payable           *SupplierBillLinePayable
}

// SupplierBillLineEntry 是一行主张连同它的匹配与审核去向。
//
// Classification 是这一行的匹配分类（domain.MatchClassification 的 String()）；空串表示这一行在接收记录上没有匹配——经
// 接收编排进来的每一行都恰有一条匹配，空串只会出现在受控登记口或存量行上，读面如实交空，不补「未匹配」。
// ExpectedVersion 为空是「没有对应发生项」那一格的正面形状。ExpectedMinor 只在点名了预期成本时有意义。
//
// MatchedMinor 是这一行的已匹配金额（通过的上限，domain.MatchedAmountFor）；没有匹配的行是零。Decisions 是这一行上的全部审核
// 决定（不分版本，按落库先后）；PassedMinor、RejectedMinor、OpenDisputeMinor、UndecidedMinor 是它们按结果分开的四个和
// （domain.TallyLineDecisions），主张金额取这一版。
type SupplierBillLineEntry struct {
	Line             string
	FeeItem          string
	ClaimedMinor     int64
	Classification   string
	ExpectedVersion  string
	ExpectedMinor    int64
	Basis            string
	MatchedAt        *time.Time
	MatchedMinor     int64
	Decisions        []SupplierBillLineDecision
	PassedMinor      int64
	RejectedMinor    int64
	OpenDisputeMinor int64
	UndecidedMinor   int64
}

// SupplierBillLines 是一份已接收的供应商账单主张的全貌：主张身份与版本、供应商、法人、账期、币种、到达与落库时刻、被追加的
// 原主张、接收那一刻审核授权是否已配置（快照，理由见 SupplierBillReceptionCatalogueRow）、接收操作者、供应商账单号（ADR-0357
// 决定一，为空读作未记），以及按主张原序排列的各行。
type SupplierBillLines struct {
	Claim                    string
	ClaimVersion             string
	Supplier                 string
	SupplierBillNumber       string
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
//
// 一笔应付是一份通过的审核决定形成的（ADR-0357 决定三）：RangeFromMinor、RangeToMinor 是它通过的那一段，ResolvesDispute 是它解
// 的争议（再审才有）；读不到那份决定时两段都是 nil，如实交空。SupplierBillNumber 是主张在供应商账单号册上的号，为空读作未记。
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
	RangeFromMinor        *int64
	RangeToMinor          *int64
	ResolvesDispute       string
	SupplierBillNumber    string
}

// AuditedPayableFilter 是审核应付册的检索维：应付单号、主张身份、供应商，各自精确相等；空串即不按这一维筛，几维同给取交集。
// 只收这几维（票 document-numbering/11）：查阅面不收任何处置参数，理由同本包其余读口。
type AuditedPayableFilter struct {
	DocumentNumber string
	Claim          string
	Supplier       string
}

// AuditedPayableCatalogueRead 按租户上列审核应付的一页，按 AuditedPayablePages 的声明取，缺省新形成的在前（按审核时刻倒序、
// 同刻按应付标识同向）；Limit 必须为正。
//
// 一笔应付找不到它所属主张的接收记录是册上矛盾，答错误。按供应商检索时也一样：这一笔属不属于那家供应商无从判断，当它
// 不属于而滤掉，就是带不带供应商检索两种答法——不带时答错误，带了却悄悄少一笔。
type AuditedPayableCatalogueRead interface {
	ListAuditedPayables(
		ctx context.Context,
		tenant domain.TenantID,
		filter AuditedPayableFilter,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[AuditedPayableCatalogueRow], error)
}
