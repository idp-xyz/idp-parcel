package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// CaseRegisterCatalogue 实现 ports.CaseRegisterCatalogueRead：案件配置各册
// （就绪判断、提交授权、关闭义务目录）的列表读面（ADR-0077，票
// admin-web-page-wiring-frontier/05）。读的就是这几册登记册本表；点读视图
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
// 三册按 ports 里的声明（ReadinessJudgmentPages / SubmissionAuthorityPages /
// ClosureObligationPages）取页，缺省序是登记册键升序（就绪/授权按单元，义务按案件），
// 义务项在一行之内按义务名排。limit 非正是调用方编程错误：静默答一页会把「忘了传」
// 变成一个没人决定过的页大小。
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

// revocationAbsent 是就绪与授权两册状态维 effective 的落列：撤销时刻缺席的文本形（'true' / 'false'，与词表同词）。撤销两列
// 同生同灭（约束 readiness_judgment_revocation_paired 与 submission_authority_revocation_paired），只看时刻一列，与领域
// Effective() 读的是同一格；重建门照同一行再走一遍，筛出的行与答复里的 effective 对不上的路没有。不建索引：两册按单元键
// 上列，这一维只在本租户的行上再滤一遍。
const revocationAbsent = "(revoked_at IS NULL)::text"

// readinessJudgmentPages 把 ports.ReadinessJudgmentPages 落到列上。q 在单元、就绪依据与撤销方上做字面包含。
var readinessJudgmentPages = ports.ReadinessJudgmentPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"unit": "unit_id", "judgedAt": "judged_at"},
	Identity: []string{"unit_id"},
	Filters:  map[string]string{"effective": revocationAbsent},
	Keyword:  []string{"unit_id", "basis_ref", "revoked_by"},
})

// scannedReadinessJudgment 留着单元与判断时刻两列的原值：游标位置取自 ORDER BY 用的列，不取重建后的领域对象。
type scannedReadinessJudgment struct {
	judgment domain.ReadinessJudgment
	unit     string
	judgedAt time.Time
}

func (catalogue *CaseRegisterCatalogue) ListReadinessJudgments(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[domain.ReadinessJudgment], error) {
	fail := func(err error) (cataloguepage.Result[domain.ReadinessJudgment], error) {
		return cataloguepage.Result[domain.ReadinessJudgment]{}, fmt.Errorf("list readiness judgments: %w", err)
	}
	const source = "customs_compliance.readiness_judgment"
	page, err := openPage(ctx, catalogue.db, readinessJudgmentPages, query, limit, source, "tenant_id = $1", tenant.String())
	if err != nil {
		return fail(err)
	}
	rows, err := page.rows(ctx, "unit_id, basis_ref, judged_at, revoked_by, revoked_at", source)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]scannedReadinessJudgment, 0, limit+1)
	for rows.Next() {
		var (
			row       scannedReadinessJudgment
			basisRaw  string
			revokedBy *string
			revokedAt *time.Time
		)
		if err := rows.Scan(&row.unit, &basisRaw, &row.judgedAt, &revokedBy, &revokedAt); err != nil {
			return fail(err)
		}
		unit, err := domain.NewDeclarationUnitID(row.unit)
		if err != nil {
			return fail(fmt.Errorf("rebuild readiness judgment: %w", err))
		}
		basis, err := domain.NewReadinessBasisReference(basisRaw)
		if err != nil {
			return fail(fmt.Errorf("rebuild readiness judgment: %w", err))
		}
		judgment, err := domain.JudgeReady(unit, basis, row.judgedAt)
		if err != nil {
			return fail(fmt.Errorf("rebuild readiness judgment: %w", err))
		}
		if revokedBy != nil {
			judgment, err = judgment.Revoke(*revokedBy, *revokedAt)
			if err != nil {
				return fail(fmt.Errorf("rebuild readiness judgment: %w", err))
			}
		}
		row.judgment = judgment
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	result, err := cataloguepage.Finish(query, fetched, limit, page.total,
		func(row scannedReadinessJudgment, sortField string) cataloguepage.Position {
			value := row.unit
			if sortField == "judgedAt" {
				value = cataloguepage.FormatInstant(row.judgedAt)
			}
			return cataloguepage.Position{Value: value, Identity: []string{row.unit}}
		})
	if err != nil {
		return fail(err)
	}
	return pageEntries(result, func(row scannedReadinessJudgment) domain.ReadinessJudgment { return row.judgment }), nil
}

// submissionAuthorityPages 把 ports.SubmissionAuthorityPages 落到列上。q 在单元、授权引用与撤销方上做字面包含。
var submissionAuthorityPages = ports.SubmissionAuthorityPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"unit": "unit_id", "grantedAt": "granted_at"},
	Identity: []string{"unit_id"},
	Filters:  map[string]string{"effective": revocationAbsent},
	Keyword:  []string{"unit_id", "authority_ref", "revoked_by"},
})

// scannedSubmissionAuthority 留着单元与授予时刻两列的原值，理由同 scannedReadinessJudgment。
type scannedSubmissionAuthority struct {
	authorization domain.SubmissionAuthorization
	unit          string
	grantedAt     time.Time
}

func (catalogue *CaseRegisterCatalogue) ListSubmissionAuthorities(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[domain.SubmissionAuthorization], error) {
	fail := func(err error) (cataloguepage.Result[domain.SubmissionAuthorization], error) {
		return cataloguepage.Result[domain.SubmissionAuthorization]{}, fmt.Errorf("list submission authorities: %w", err)
	}
	const source = "customs_compliance.submission_authority"
	page, err := openPage(ctx, catalogue.db, submissionAuthorityPages, query, limit, source, "tenant_id = $1", tenant.String())
	if err != nil {
		return fail(err)
	}
	rows, err := page.rows(ctx, "unit_id, authority_ref, granted_at, revoked_by, revoked_at", source)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]scannedSubmissionAuthority, 0, limit+1)
	for rows.Next() {
		var (
			row          scannedSubmissionAuthority
			authorityRaw string
			revokedBy    *string
			revokedAt    *time.Time
		)
		if err := rows.Scan(&row.unit, &authorityRaw, &row.grantedAt, &revokedBy, &revokedAt); err != nil {
			return fail(err)
		}
		unit, err := domain.NewDeclarationUnitID(row.unit)
		if err != nil {
			return fail(fmt.Errorf("rebuild submission authority: %w", err))
		}
		authority, err := domain.NewSubmissionAuthorityReference(authorityRaw)
		if err != nil {
			return fail(fmt.Errorf("rebuild submission authority: %w", err))
		}
		authorization, err := domain.GrantSubmissionAuthority(unit, authority, row.grantedAt)
		if err != nil {
			return fail(fmt.Errorf("rebuild submission authority: %w", err))
		}
		if revokedBy != nil {
			authorization, err = authorization.Revoke(*revokedBy, *revokedAt)
			if err != nil {
				return fail(fmt.Errorf("rebuild submission authority: %w", err))
			}
		}
		row.authorization = authorization
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	result, err := cataloguepage.Finish(query, fetched, limit, page.total,
		func(row scannedSubmissionAuthority, sortField string) cataloguepage.Position {
			value := row.unit
			if sortField == "grantedAt" {
				value = cataloguepage.FormatInstant(row.grantedAt)
			}
			return cataloguepage.Position{Value: value, Identity: []string{row.unit}}
		})
	if err != nil {
		return fail(err)
	}
	return pageEntries(result,
		func(row scannedSubmissionAuthority) domain.SubmissionAuthorization { return row.authorization }), nil
}

// closureObligationItemDocument 是义务项在 json_agg 里的临时词形。handedTo 与
// appliesUntil 用指针收 NULL：非承接项的承接对象、尚无终点的适用区间在库里就是
// NULL，指针缺席即领域侧的零值，不代填任何编造值。
type closureObligationItemDocument struct {
	Obligation string  `json:"obligation"`
	Scope      string  `json:"scope"`
	State      string  `json:"state"`
	Basis      string  `json:"basis"`
	HandedTo   *string `json:"handedTo"`
	// conflictsWith 在库里是可空数组，json_agg 里 NULL 即缺席。
	ConflictsWith []string   `json:"conflictsWith"`
	AppliesFrom   time.Time  `json:"appliesFrom"`
	AppliesUntil  *time.Time `json:"appliesUntil"`
}

func (document closureObligationItemDocument) registration() (ports.ObligationRegistration, error) {
	// 状态封闭四值经同一张译表（obligationItemStateOf）：集外取值上抛，判据同点读
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
	item.ConflictsWith = document.ConflictsWith
	registration := ports.ObligationRegistration{
		Item:        item,
		AppliesFrom: document.AppliesFrom.UTC(),
	}
	if document.AppliesUntil != nil {
		registration.AppliesUntil = document.AppliesUntil.UTC()
	}
	return registration, nil
}

// closureObligationPages 把 ports.ClosureObligationPages 落到目录表的列上。q 只在案件引用上做字面包含：义务项是另一张表里
// 的子集，进检索就得为每行另做一次子查询，本册的行是「一个案件的义务目录」，按案件找就够。
var closureObligationPages = ports.ClosureObligationPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"case": "catalog.case_ref", "registeredAt": "catalog.registered_at"},
	Identity: []string{"catalog.case_ref"},
	Keyword:  []string{"catalog.case_ref"},
})

// scannedClosureObligation 留着案件与登记时刻两列的原值，理由同 scannedReadinessJudgment。
type scannedClosureObligation struct {
	entry        ports.ClosureObligationCatalogueEntry
	caseRef      string
	registeredAt time.Time
}

func (catalogue *CaseRegisterCatalogue) ListClosureObligations(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ClosureObligationCatalogueEntry], error) {
	fail := func(err error) (cataloguepage.Result[ports.ClosureObligationCatalogueEntry], error) {
		return cataloguepage.Result[ports.ClosureObligationCatalogueEntry]{},
			fmt.Errorf("list closure obligations: %w", err)
	}
	const source = "customs_compliance.closure_obligation_catalog AS catalog"
	page, err := openPage(ctx, catalogue.db, closureObligationPages, query, limit, source,
		"catalog.tenant_id = $1", tenant.String())
	if err != nil {
		return fail(err)
	}
	rows, err := page.rows(ctx,
		`catalog.case_ref, catalog.registered_at,
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'obligation',   item.obligation,
		                            'scope',        item.scope,
		                            'state',        item.item_state,
		                            'basis',        item.basis,
		                            'handedTo',     item.handed_to,
		                            'conflictsWith', item.conflicts_with,
		                            'appliesFrom',  item.applies_from,
		                            'appliesUntil', item.applies_until
		                        )
		                        ORDER BY item.obligation
		                    ),
		                    '[]'::json
		                )
		           FROM customs_compliance.closure_obligation_item AS item
		          WHERE item.tenant_id = catalog.tenant_id
		            AND item.case_ref  = catalog.case_ref)`,
		source)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]scannedClosureObligation, 0, limit+1)
	for rows.Next() {
		var (
			row       scannedClosureObligation
			itemsJSON []byte
		)
		if err := rows.Scan(&row.caseRef, &row.registeredAt, &itemsJSON); err != nil {
			return fail(err)
		}
		caseRef, err := domain.NewCustomsCaseID(row.caseRef)
		if err != nil {
			return fail(fmt.Errorf("rebuild closure obligation catalogue: %w", err))
		}
		var documents []closureObligationItemDocument
		if err := json.Unmarshal(itemsJSON, &documents); err != nil {
			return fail(fmt.Errorf("义务项集解码：%w", err))
		}
		items := make([]ports.ObligationRegistration, 0, len(documents))
		for _, document := range documents {
			registration, err := document.registration()
			if err != nil {
				return fail(fmt.Errorf("rebuild closure obligation catalogue: %w", err))
			}
			items = append(items, registration)
		}
		row.entry = ports.ClosureObligationCatalogueEntry{
			Case:         caseRef,
			RegisteredAt: row.registeredAt.UTC(),
			Items:        items,
		}
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	result, err := cataloguepage.Finish(query, fetched, limit, page.total,
		func(row scannedClosureObligation, sortField string) cataloguepage.Position {
			value := row.caseRef
			if sortField == "registeredAt" {
				value = cataloguepage.FormatInstant(row.registeredAt)
			}
			return cataloguepage.Position{Value: value, Identity: []string{row.caseRef}}
		})
	if err != nil {
		return fail(err)
	}
	return pageEntries(result,
		func(row scannedClosureObligation) ports.ClosureObligationCatalogueEntry { return row.entry }), nil
}
