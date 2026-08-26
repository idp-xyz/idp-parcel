package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// CaseRegisterCatalogue 实现 ports.CaseRegisterCatalogueRead：案件配置三本册子
// （就绪判断、提交授权、关闭义务目录）的列表读面（ADR-0077，票
// admin-web-page-wiring-frontier/05）。读的就是三本登记册本表；点读视图
// （ReadinessView / SubmissionAuthorityView / ObligationInventoryView）按键单点作答，
// 这里按租户上列——两种读法各答各的问题，谁也不为对方改形状（判据同 RuleCatalogue）。
//
// 就绪与授权逐行走点读同一条重建门（构造 → 撤销追加）：坏行在读口拦下，不进查阅
// 答案；撤销行原判断与失效两段都如实读回，不折成未配置或仍有效。
//
// 关闭义务的目录行与义务项父子两表**一条语句取回**：ReadExecutor 不保证两条语句同一
// 快照，分次取会拼出从未同时存在的父子状态（登记与读并发时尤甚）。义务项走相关子
// 查询 json_agg 各聚各的，形状循 visibilityexception OperationsCatalogue 的同款教训
// 注释——多族 LEFT JOIN 会互相做笛卡尔积。
//
// 排序按登记册键升序（就绪/授权按单元，义务按案件、项内按义务名），保证分页可重复。
// limit 非正是调用方编程错误：静默答一页会把「忘了传」变成一个没人决定过的页大小。
type CaseRegisterCatalogue struct {
	db *bentopg.DB
}

func NewCaseRegisterCatalogue(db *bentopg.DB) (*CaseRegisterCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &CaseRegisterCatalogue{db: db}, nil
}

var _ ports.CaseRegisterCatalogueRead = (*CaseRegisterCatalogue)(nil)

func (catalogue *CaseRegisterCatalogue) ListReadinessJudgments(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]domain.ReadinessJudgment, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list readiness judgments: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list readiness judgments: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT unit_id, basis_ref, judged_at, revoked_by, revoked_at
		   FROM customs_compliance.readiness_judgment
		  WHERE tenant_id = $1
		  ORDER BY unit_id
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list readiness judgments: %w", err)
	}
	defer rows.Close()

	judgments := make([]domain.ReadinessJudgment, 0, limit)
	for rows.Next() {
		var (
			unitRaw, basisRaw string
			judgedAt          time.Time
			revokedBy         *string
			revokedAt         *time.Time
		)
		if err := rows.Scan(&unitRaw, &basisRaw, &judgedAt, &revokedBy, &revokedAt); err != nil {
			return nil, fmt.Errorf("list readiness judgments: %w", err)
		}
		unit, err := domain.NewDeclarationUnitID(unitRaw)
		if err != nil {
			return nil, fmt.Errorf("rebuild readiness judgment: %w", err)
		}
		basis, err := domain.NewReadinessBasisReference(basisRaw)
		if err != nil {
			return nil, fmt.Errorf("rebuild readiness judgment: %w", err)
		}
		judgment, err := domain.JudgeReady(unit, basis, judgedAt)
		if err != nil {
			return nil, fmt.Errorf("rebuild readiness judgment: %w", err)
		}
		if revokedBy != nil {
			judgment, err = judgment.Revoke(*revokedBy, *revokedAt)
			if err != nil {
				return nil, fmt.Errorf("rebuild readiness judgment: %w", err)
			}
		}
		judgments = append(judgments, judgment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list readiness judgments: %w", err)
	}
	return judgments, nil
}

func (catalogue *CaseRegisterCatalogue) ListSubmissionAuthorities(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]domain.SubmissionAuthorization, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list submission authorities: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list submission authorities: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT unit_id, authority_ref, granted_at, revoked_by, revoked_at
		   FROM customs_compliance.submission_authority
		  WHERE tenant_id = $1
		  ORDER BY unit_id
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list submission authorities: %w", err)
	}
	defer rows.Close()

	authorizations := make([]domain.SubmissionAuthorization, 0, limit)
	for rows.Next() {
		var (
			unitRaw, authorityRaw string
			grantedAt             time.Time
			revokedBy             *string
			revokedAt             *time.Time
		)
		if err := rows.Scan(&unitRaw, &authorityRaw, &grantedAt, &revokedBy, &revokedAt); err != nil {
			return nil, fmt.Errorf("list submission authorities: %w", err)
		}
		unit, err := domain.NewDeclarationUnitID(unitRaw)
		if err != nil {
			return nil, fmt.Errorf("rebuild submission authority: %w", err)
		}
		authority, err := domain.NewSubmissionAuthorityReference(authorityRaw)
		if err != nil {
			return nil, fmt.Errorf("rebuild submission authority: %w", err)
		}
		authorization, err := domain.GrantSubmissionAuthority(unit, authority, grantedAt)
		if err != nil {
			return nil, fmt.Errorf("rebuild submission authority: %w", err)
		}
		if revokedBy != nil {
			authorization, err = authorization.Revoke(*revokedBy, *revokedAt)
			if err != nil {
				return nil, fmt.Errorf("rebuild submission authority: %w", err)
			}
		}
		authorizations = append(authorizations, authorization)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list submission authorities: %w", err)
	}
	return authorizations, nil
}

// closureObligationItemDocument 是义务项在 json_agg 里的临时词形。handedTo 与
// appliesUntil 用指针收 NULL：非承接项的承接对象、尚无终点的适用区间在库里就是
// NULL，指针缺席即领域侧的零值，不代填任何编造值。
type closureObligationItemDocument struct {
	Obligation   string     `json:"obligation"`
	Scope        string     `json:"scope"`
	State        string     `json:"state"`
	Basis        string     `json:"basis"`
	HandedTo     *string    `json:"handedTo"`
	AppliesFrom  time.Time  `json:"appliesFrom"`
	AppliesUntil *time.Time `json:"appliesUntil"`
}

func (document closureObligationItemDocument) registration() (ports.ObligationRegistration, error) {
	// 状态封闭三值经同一张译表（obligationItemStateOf）：集外取值上抛，判据同点读
	// ——静默吸收会让旁路写入的坏数据穿进查阅答案。
	state, err := obligationItemStateOf(document.State)
	if err != nil {
		return ports.ObligationRegistration{}, err
	}
	item := domain.ClosureObligationItem{
		Obligation: document.Obligation,
		Scope:      document.Scope,
		State:      state,
		Basis:      document.Basis,
	}
	if document.HandedTo != nil {
		item.HandedTo = *document.HandedTo
	}
	registration := ports.ObligationRegistration{
		Item:        item,
		AppliesFrom: document.AppliesFrom.UTC(),
	}
	if document.AppliesUntil != nil {
		registration.AppliesUntil = document.AppliesUntil.UTC()
	}
	return registration, nil
}

func (catalogue *CaseRegisterCatalogue) ListClosureObligations(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.ClosureObligationCatalogueEntry, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list closure obligations: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list closure obligations: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT catalog.case_ref, catalog.registered_at,
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'obligation',   item.obligation,
		                            'scope',        item.scope,
		                            'state',        item.item_state,
		                            'basis',        item.basis,
		                            'handedTo',     item.handed_to,
		                            'appliesFrom',  item.applies_from,
		                            'appliesUntil', item.applies_until
		                        )
		                        ORDER BY item.obligation
		                    ),
		                    '[]'::json
		                )
		           FROM customs_compliance.closure_obligation_item AS item
		          WHERE item.tenant_id = catalog.tenant_id
		            AND item.case_ref  = catalog.case_ref)
		   FROM customs_compliance.closure_obligation_catalog AS catalog
		  WHERE catalog.tenant_id = $1
		  ORDER BY catalog.case_ref
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list closure obligations: %w", err)
	}
	defer rows.Close()

	entries := make([]ports.ClosureObligationCatalogueEntry, 0, limit)
	for rows.Next() {
		var (
			caseRaw      string
			registeredAt time.Time
			itemsJSON    []byte
		)
		if err := rows.Scan(&caseRaw, &registeredAt, &itemsJSON); err != nil {
			return nil, fmt.Errorf("list closure obligations: %w", err)
		}
		caseRef, err := domain.NewCustomsCaseID(caseRaw)
		if err != nil {
			return nil, fmt.Errorf("rebuild closure obligation catalogue: %w", err)
		}
		var documents []closureObligationItemDocument
		if err := json.Unmarshal(itemsJSON, &documents); err != nil {
			return nil, fmt.Errorf("list closure obligations: 义务项集解码：%w", err)
		}
		items := make([]ports.ObligationRegistration, 0, len(documents))
		for _, document := range documents {
			registration, err := document.registration()
			if err != nil {
				return nil, fmt.Errorf("rebuild closure obligation catalogue: %w", err)
			}
			items = append(items, registration)
		}
		entries = append(entries, ports.ClosureObligationCatalogueEntry{
			Case:         caseRef,
			RegisteredAt: registeredAt.UTC(),
			Items:        items,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list closure obligations: %w", err)
	}
	return entries, nil
}
