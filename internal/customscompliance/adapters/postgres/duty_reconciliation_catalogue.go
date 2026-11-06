package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// DutyReconciliationCatalogue 一并实现 ports.DutyCollaborationCatalogueRead 与
// ports.DutyVerificationCatalogueRead：税费付款协作事项与税费付款核对两册的列表读面
// （ADR-0077，票 sa-cc/10）。两口落在一个类型上只是装配省事——两张表各自的键互不
// 相干，端点也按两个读口分别依赖它（判据同 DutyPaymentReconciliation 三口一体）。
// 资金事实引用那张表不在本读口：它今天没有查阅面要求，「待关联」是它上面的派生，
// 不是这两册的列。
//
// 点读 FindCollaboration / FindVerification 按键单点作答伺候核对编排，这里按租户
// 上列伺候查阅——两种读法各答各的问题，谁也不为对方改形状。读回经领域构造重建
// （rebuildCollaboration / rebuildVerification，与点读同一条路）：库里一行若立不起
// 领域对象，说明有人绕过写口改了它，作错误抛出而不是交回半成品；三轴集外词形同样
// 在重建处上抛，不在 SQL 里折成第四格。
//
// 两册按 ports 里的声明（DutyCollaborationPages / DutyVerificationPages）取页，缺省序是
// 各表的键序升序，核对版本再按核对时间——同三维键的多版本按时间读得出先后，但读口
// 不替读者判「哪版是当前」（迟到事实按新版本追加、不覆盖，UC-CC-009）。limit 非正是
// 调用方编程错误：静默答一页会把「忘了传」变成一个没人决定过的页大小。
type DutyReconciliationCatalogue struct {
	db *bentopg.DB
}

func NewDutyReconciliationCatalogue(db *bentopg.DB) (*DutyReconciliationCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &DutyReconciliationCatalogue{db: db}, nil
}

var (
	_ ports.DutyCollaborationCatalogueRead = (*DutyReconciliationCatalogue)(nil)
	_ ports.DutyVerificationCatalogueRead  = (*DutyReconciliationCatalogue)(nil)
)

// dutyCollaborationPages 把 ports.DutyCollaborationPages 落到列上。q 在决定范围、税费引用与付款义务人上做字面包含。
var dutyCollaborationPages = ports.DutyCollaborationPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"scope": "scope_ref", "formedAt": "formed_at"},
	Identity: []string{"scope_ref", "duty_ref"},
	Filters:  map[string]string{"kind": "kind"},
	Keyword:  []string{"scope_ref", "duty_ref", "obligor_ref"},
})

// scannedDutyCollaboration 留着键（范围、税费引用）与形成时刻的原值：游标位置取自 ORDER BY 用的列，不取重建后的领域对象——
// 无需付款格的税费引用在领域上是零值，在键上是空串，两者不是同一个写法。
type scannedDutyCollaboration struct {
	collaboration domain.DutyPaymentCollaboration
	scope, duty   string
	formedAt      time.Time
}

func (catalogue *DutyReconciliationCatalogue) ListDutyCollaborations(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[domain.DutyPaymentCollaboration], error) {
	fail := func(err error) (cataloguepage.Result[domain.DutyPaymentCollaboration], error) {
		return cataloguepage.Result[domain.DutyPaymentCollaboration]{}, fmt.Errorf("list duty collaborations: %w", err)
	}
	const source = "customs_compliance.duty_payment_collaboration"
	page, err := openPage(ctx, catalogue.db, dutyCollaborationPages, query, limit, source, "tenant_id = $1", tenant.String())
	if err != nil {
		return fail(err)
	}
	rows, err := page.rows(ctx,
		"scope_ref, duty_ref, kind, no_pay_basis, obligor_ref, requirement_ref, target_ref, formed_at", source)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]scannedDutyCollaboration, 0, limit+1)
	for rows.Next() {
		var (
			row                                            scannedDutyCollaboration
			kind, noPayBasis, obligor, requirement, target string
		)
		if err := rows.Scan(&row.scope, &row.duty, &kind, &noPayBasis,
			&obligor, &requirement, &target, &row.formedAt); err != nil {
			return fail(err)
		}
		scope, err := domain.NewDecisionScopeReference(row.scope)
		if err != nil {
			return fail(fmt.Errorf("rebuild duty collaboration catalogue: %w", err))
		}
		// 无需付款格的税费引用在键上是空串（0016 自注）：那一格的领域对象要的是零值
		// 引用，不是一个「空串」引用——后者构造期就拒。
		var duty domain.AssessedDutyReference
		if row.duty != "" {
			if duty, err = domain.NewAssessedDutyReference(row.duty); err != nil {
				return fail(fmt.Errorf("rebuild duty collaboration catalogue: %w", err))
			}
		}
		collaboration, err := rebuildCollaboration(kind, duty, noPayBasis, scope, obligor, requirement, target, row.formedAt)
		if err != nil {
			return fail(fmt.Errorf("rebuild duty collaboration catalogue: %w", err))
		}
		row.collaboration = collaboration
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	result, err := cataloguepage.Finish(query, fetched, limit, page.total,
		func(row scannedDutyCollaboration, sortField string) cataloguepage.Position {
			value := row.scope
			if sortField == "formedAt" {
				value = cataloguepage.FormatInstant(row.formedAt)
			}
			return cataloguepage.Position{Value: value, Identity: []string{row.scope, row.duty}}
		})
	if err != nil {
		return fail(err)
	}
	return pageEntries(result,
		func(row scannedDutyCollaboration) domain.DutyPaymentCollaboration { return row.collaboration }), nil
}

// dutyVerificationPages 把 ports.DutyVerificationPages 落到列上。三轴各落在同名列上；q 在税费、资金事实与决定范围
// 的引用上做字面包含。
var dutyVerificationPages = ports.DutyVerificationPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"duty": "duty_ref", "verifiedAt": "verified_at"},
	Identity: []string{"duty_ref", "funds_ref", "scope_ref", "verified_at", "version_digest"},
	Filters:  map[string]string{"coverage": "coverage", "delta": "delta", "validity": "validity"},
	Keyword:  []string{"duty_ref", "funds_ref", "scope_ref"},
})

// scannedDutyVerification 留着标识各列的原值，理由同 scannedDutyCollaboration。
type scannedDutyVerification struct {
	record                     ports.DutyVerificationRecord
	duty, funds, scope, digest string
	verifiedAt                 time.Time
}

func (catalogue *DutyReconciliationCatalogue) ListDutyVerifications(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.DutyVerificationRecord], error) {
	fail := func(err error) (cataloguepage.Result[ports.DutyVerificationRecord], error) {
		return cataloguepage.Result[ports.DutyVerificationRecord]{}, fmt.Errorf("list duty verifications: %w", err)
	}
	const source = "customs_compliance.duty_payment_verification"
	page, err := openPage(ctx, catalogue.db, dutyVerificationPages, query, limit, source, "tenant_id = $1", tenant.String())
	if err != nil {
		return fail(err)
	}
	rows, err := page.rows(ctx,
		`duty_ref, funds_ref, scope_ref, version_digest,
		        funds_version, procedure_ref, coverage, delta, validity, basis, verified_at`,
		source)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]scannedDutyVerification, 0, limit+1)
	for rows.Next() {
		var (
			row                                                       scannedDutyVerification
			fundsVersion, procedure, coverage, delta, validity, basis string
		)
		if err := rows.Scan(&row.duty, &row.funds, &row.scope, &row.digest,
			&fundsVersion, &procedure, &coverage, &delta, &validity, &basis, &row.verifiedAt); err != nil {
			return fail(err)
		}
		key := ports.DutyVerificationKey{TenantID: tenant, Digest: row.digest}
		if key.Duty, err = domain.NewAssessedDutyReference(row.duty); err != nil {
			return fail(fmt.Errorf("rebuild duty verification catalogue: %w", err))
		}
		if key.Funds, err = domain.NewExternalFundsFactReference(row.funds); err != nil {
			return fail(fmt.Errorf("rebuild duty verification catalogue: %w", err))
		}
		if key.Scope, err = domain.NewDecisionScopeReference(row.scope); err != nil {
			return fail(fmt.Errorf("rebuild duty verification catalogue: %w", err))
		}
		verification, err := rebuildVerification(key, fundsVersion, procedure, coverage, delta, validity, row.verifiedAt)
		if err != nil {
			return fail(fmt.Errorf("rebuild duty verification catalogue: %w", err))
		}
		row.record = ports.DutyVerificationRecord{Key: key, Verification: verification, Basis: basis}
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	result, err := cataloguepage.Finish(query, fetched, limit, page.total,
		func(row scannedDutyVerification, sortField string) cataloguepage.Position {
			verifiedAt := cataloguepage.FormatInstant(row.verifiedAt)
			value := row.duty
			if sortField == "verifiedAt" {
				value = verifiedAt
			}
			return cataloguepage.Position{
				Value:    value,
				Identity: []string{row.duty, row.funds, row.scope, verifiedAt, row.digest},
			}
		})
	if err != nil {
		return fail(err)
	}
	return pageEntries(result, func(row scannedDutyVerification) ports.DutyVerificationRecord { return row.record }), nil
}
