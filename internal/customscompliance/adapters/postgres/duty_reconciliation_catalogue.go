package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
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
// 排序按各表的键序升序，核对版本再按核对时间——同三维键的多版本按时间读得出先后，
// 但读口不替读者判「哪版是当前」（迟到事实按新版本追加、不覆盖，UC-CC-009）。limit
// 非正是调用方编程错误：静默答一页会把「忘了传」变成一个没人决定过的页大小。
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

func (catalogue *DutyReconciliationCatalogue) ListDutyCollaborations(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]domain.DutyPaymentCollaboration, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list duty collaborations: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list duty collaborations: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT scope_ref, duty_ref, kind, no_pay_basis, obligor_ref, requirement_ref, target_ref, formed_at
		   FROM customs_compliance.duty_payment_collaboration
		  WHERE tenant_id = $1
		  ORDER BY scope_ref, duty_ref
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list duty collaborations: %w", err)
	}
	defer rows.Close()

	entries := make([]domain.DutyPaymentCollaboration, 0, limit)
	for rows.Next() {
		var (
			scopeRaw, dutyRaw, kind, noPayBasis, obligor, requirement, target string
			formedAt                                                          time.Time
		)
		if err := rows.Scan(&scopeRaw, &dutyRaw, &kind, &noPayBasis,
			&obligor, &requirement, &target, &formedAt); err != nil {
			return nil, fmt.Errorf("list duty collaborations: %w", err)
		}
		scope, err := domain.NewDecisionScopeReference(scopeRaw)
		if err != nil {
			return nil, fmt.Errorf("rebuild duty collaboration catalogue: %w", err)
		}
		// 无需付款格的税费引用在键上是空串（0016 自注）：那一格的领域对象要的是零值
		// 引用，不是一个「空串」引用——后者构造期就拒。
		var duty domain.AssessedDutyReference
		if dutyRaw != "" {
			if duty, err = domain.NewAssessedDutyReference(dutyRaw); err != nil {
				return nil, fmt.Errorf("rebuild duty collaboration catalogue: %w", err)
			}
		}
		collaboration, err := rebuildCollaboration(kind, duty, noPayBasis, scope, obligor, requirement, target, formedAt)
		if err != nil {
			return nil, fmt.Errorf("rebuild duty collaboration catalogue: %w", err)
		}
		entries = append(entries, collaboration)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list duty collaborations: %w", err)
	}
	return entries, nil
}

func (catalogue *DutyReconciliationCatalogue) ListDutyVerifications(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.DutyVerificationRecord, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list duty verifications: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list duty verifications: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT duty_ref, funds_ref, scope_ref, version_digest,
		        procedure_ref, coverage, delta, validity, basis, verified_at
		   FROM customs_compliance.duty_payment_verification
		  WHERE tenant_id = $1
		  ORDER BY duty_ref, funds_ref, scope_ref, verified_at, version_digest
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list duty verifications: %w", err)
	}
	defer rows.Close()

	records := make([]ports.DutyVerificationRecord, 0, limit)
	for rows.Next() {
		var (
			dutyRaw, fundsRaw, scopeRaw, digest         string
			procedure, coverage, delta, validity, basis string
			verifiedAt                                  time.Time
		)
		if err := rows.Scan(&dutyRaw, &fundsRaw, &scopeRaw, &digest,
			&procedure, &coverage, &delta, &validity, &basis, &verifiedAt); err != nil {
			return nil, fmt.Errorf("list duty verifications: %w", err)
		}
		key := ports.DutyVerificationKey{TenantID: tenant, Digest: digest}
		if key.Duty, err = domain.NewAssessedDutyReference(dutyRaw); err != nil {
			return nil, fmt.Errorf("rebuild duty verification catalogue: %w", err)
		}
		if key.Funds, err = domain.NewExternalFundsFactReference(fundsRaw); err != nil {
			return nil, fmt.Errorf("rebuild duty verification catalogue: %w", err)
		}
		if key.Scope, err = domain.NewDecisionScopeReference(scopeRaw); err != nil {
			return nil, fmt.Errorf("rebuild duty verification catalogue: %w", err)
		}
		verification, err := rebuildVerification(key, procedure, coverage, delta, validity, verifiedAt)
		if err != nil {
			return nil, fmt.Errorf("rebuild duty verification catalogue: %w", err)
		}
		records = append(records, ports.DutyVerificationRecord{Key: key, Verification: verification, Basis: basis})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list duty verifications: %w", err)
	}
	return records, nil
}
