package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// OperationsCatalogue 实现 ports.CatalogueListRead：VE 六类目录的列表读面（票
// admin-web-page-wiring-frontier/02）。命名循 parcelpricing / partycommercial 的同名
// 适配器：同一上下文的目录查阅端点共享这一只读适配器，读的仍是 CatalogRegistrar
// 写口背后的那七张表。
//
// 租户随每次调用到达（端口签名如此）——本读面给多租户装配（`cmd/parcel-api`）消费，
// 与本包五个装载视图「租户钉在装配期」的形状刻意不同：装载视图伺候单租户编排的判断
// 输入，这里伺候运营查阅，没有一个装配期可钉的租户。
//
// 区间型目录的「版本 + 整版条目」一条语句取回，不分两次：ReadExecutor 不保证两条
// 语句同一快照，分次取会拼出从未同时存在的父子状态——尤其登记与读并发时，可能读到
// 「新版抬头 + 旧版条目集」这种库里从没存在过的组合。条目族走相关子查询 json_agg
// 各聚各的（形状循 partycommercial ListAcceptanceRulePackages 的教训注释：多族
// LEFT JOIN 会互相做笛卡尔积），这里每父只有一族条目，用子查询同样是为了第二族
// 挂上来时不必重走那个坑。
type OperationsCatalogue struct {
	db *bentopg.DB
}

func NewOperationsCatalogue(db *bentopg.DB) (*OperationsCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &OperationsCatalogue{db: db}, nil
}

var _ ports.CatalogueListRead = (*OperationsCatalogue)(nil)

// requirePositiveLimit 拒绝无意义的页大小。零与负数不是「不限量」：上不封顶的目录
// 读在租户册涨大后会把一次查阅变成全表搬运，页大小的裁决归接入面，读口只拒错值。
func requirePositiveLimit(operation string, limit int) error {
	if limit <= 0 {
		return fmt.Errorf("%s: limit must be positive, got %d", operation, limit)
	}
	return nil
}

// ListMilestoneMappings 上列里程碑映射版本连同整版条目（`PAR-VIS-01`）。新版在前
// （effective_from 降序）：查阅的第一问是「现在适用哪版」。
func (catalogue *OperationsCatalogue) ListMilestoneMappings(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.MilestoneMappingCatalogueRow, error) {
	if err := requirePositiveLimit("list milestone mappings", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list milestone mappings: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT version.mapping_version, version.effective_from, version.effective_to,
		        version.approved_by,
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'source',    entry.source_context,
		                            'factKind',  entry.source_fact_kind,
		                            'milestone', entry.milestone_ref
		                        )
		                        ORDER BY entry.source_context, entry.source_fact_kind
		                    ),
		                    '[]'::json
		                )
		           FROM visibility_exception.milestone_mapping_entry AS entry
		          WHERE entry.tenant_id       = version.tenant_id
		            AND entry.mapping_version = version.mapping_version)
		   FROM visibility_exception.milestone_mapping_version AS version
		  WHERE version.tenant_id = $1
		  ORDER BY version.effective_from DESC, version.mapping_version
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list milestone mappings: %w", err)
	}
	defer rows.Close()

	mappingRows := make([]ports.MilestoneMappingCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.MilestoneMappingCatalogueRow
		var effectiveTo *time.Time
		var entriesJSON []byte
		if err := rows.Scan(
			&row.Version, &row.EffectiveFrom, &effectiveTo, &row.ApprovedBy, &entriesJSON,
		); err != nil {
			return nil, fmt.Errorf("list milestone mappings: %w", err)
		}
		if effectiveTo != nil {
			row.EffectiveTo = *effectiveTo
			row.HasEffectiveTo = true
		}
		if err := json.Unmarshal(entriesJSON, &row.Entries); err != nil {
			return nil, fmt.Errorf("list milestone mappings: 条目集解码：%w", err)
		}
		mappingRows = append(mappingRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list milestone mappings: %w", err)
	}
	return mappingRows, nil
}

// ListTriageRules 上列分诊规则版本连同整版条目（`PAR-VIS-05`）。
func (catalogue *OperationsCatalogue) ListTriageRules(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.TriageRuleCatalogueRow, error) {
	if err := requirePositiveLimit("list triage rules", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list triage rules: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT version.rule_version, version.effective_from, version.effective_to,
		        version.approved_by,
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'signalKind', entry.signal_kind,
		                            'confidence', entry.confidence_ref,
		                            'outcome',    entry.outcome
		                        )
		                        ORDER BY entry.signal_kind, entry.confidence_ref
		                    ),
		                    '[]'::json
		                )
		           FROM visibility_exception.triage_rule_entry AS entry
		          WHERE entry.tenant_id    = version.tenant_id
		            AND entry.rule_version = version.rule_version)
		   FROM visibility_exception.triage_rule_version AS version
		  WHERE version.tenant_id = $1
		  ORDER BY version.effective_from DESC, version.rule_version
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list triage rules: %w", err)
	}
	defer rows.Close()

	ruleRows := make([]ports.TriageRuleCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.TriageRuleCatalogueRow
		var effectiveTo *time.Time
		var entriesJSON []byte
		if err := rows.Scan(
			&row.Version, &row.EffectiveFrom, &effectiveTo, &row.ApprovedBy, &entriesJSON,
		); err != nil {
			return nil, fmt.Errorf("list triage rules: %w", err)
		}
		if effectiveTo != nil {
			row.EffectiveTo = *effectiveTo
			row.HasEffectiveTo = true
		}
		if err := json.Unmarshal(entriesJSON, &row.Entries); err != nil {
			return nil, fmt.Errorf("list triage rules: 条目集解码：%w", err)
		}
		ruleRows = append(ruleRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list triage rules: %w", err)
	}
	return ruleRows, nil
}

// ListNotificationPolicies 上列通知策略（`PAR-VIS-07`）。deadline_after 以 interval
// 的文本词形转写（如 "72:00:00"）——相对量照库的口径交出，不折回 Go Duration（月与
// 日的进位语义在库那套里，折算就是改写）。
func (catalogue *OperationsCatalogue) ListNotificationPolicies(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.NotificationPolicyCatalogueRow, error) {
	if err := requirePositiveLimit("list notification policies", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list notification policies: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT policy.disclosure_policy_ref, policy.channel_ref,
		        policy.deadline_after::text, policy.obligation_ref, policy.approved_by
		   FROM visibility_exception.notification_policy AS policy
		  WHERE policy.tenant_id = $1
		  ORDER BY policy.disclosure_policy_ref
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list notification policies: %w", err)
	}
	defer rows.Close()

	policyRows := make([]ports.NotificationPolicyCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.NotificationPolicyCatalogueRow
		if err := rows.Scan(
			&row.Policy, &row.Channel, &row.DeadlineAfter, &row.Obligation, &row.ApprovedBy,
		); err != nil {
			return nil, fmt.Errorf("list notification policies: %w", err)
		}
		policyRows = append(policyRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notification policies: %w", err)
	}
	return policyRows, nil
}

// ListClaimEligibilities 上列索赔资格声明连同承担的索赔类型（`PAR-VIS-08` 合同覆盖角）。
func (catalogue *OperationsCatalogue) ListClaimEligibilities(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.ClaimEligibilityCatalogueRow, error) {
	if err := requirePositiveLimit("list claim eligibilities", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list claim eligibilities: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT declaration.contract_scope_ref, declaration.rule_version,
		        declaration.approved_by,
		        (SELECT COALESCE(json_agg(kind.claim_kind_ref ORDER BY kind.claim_kind_ref), '[]'::json)
		           FROM visibility_exception.claim_covered_kind AS kind
		          WHERE kind.tenant_id          = declaration.tenant_id
		            AND kind.contract_scope_ref = declaration.contract_scope_ref)
		   FROM visibility_exception.claim_contract_scope AS declaration
		  WHERE declaration.tenant_id = $1
		  ORDER BY declaration.contract_scope_ref
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list claim eligibilities: %w", err)
	}
	defer rows.Close()

	declarationRows := make([]ports.ClaimEligibilityCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.ClaimEligibilityCatalogueRow
		var kindsJSON []byte
		if err := rows.Scan(&row.Contract, &row.Version, &row.ApprovedBy, &kindsJSON); err != nil {
			return nil, fmt.Errorf("list claim eligibilities: %w", err)
		}
		if err := json.Unmarshal(kindsJSON, &row.CoveredKinds); err != nil {
			return nil, fmt.Errorf("list claim eligibilities: 覆盖集解码：%w", err)
		}
		declarationRows = append(declarationRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list claim eligibilities: %w", err)
	}
	return declarationRows, nil
}

// ListClaimAuthorizations 上列申请人授权目录连同名单（`PAR-VIS-08` 申请人授权角）。
// 空名单如实交回空集合——「目录在场、当前不授权任何人」是一次已作出的授权决定
// （0018），不是没登记。
func (catalogue *OperationsCatalogue) ListClaimAuthorizations(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.ClaimAuthorizationCatalogueRow, error) {
	if err := requirePositiveLimit("list claim authorizations", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list claim authorizations: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT catalogue.customer_ref, catalogue.rule_version, catalogue.approved_by,
		        (SELECT COALESCE(json_agg(member.applicant_ref ORDER BY member.applicant_ref), '[]'::json)
		           FROM visibility_exception.claim_authorized_applicant AS member
		          WHERE member.tenant_id    = catalogue.tenant_id
		            AND member.customer_ref = catalogue.customer_ref)
		   FROM visibility_exception.claim_authorization_catalogue AS catalogue
		  WHERE catalogue.tenant_id = $1
		  ORDER BY catalogue.customer_ref
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list claim authorizations: %w", err)
	}
	defer rows.Close()

	catalogueRows := make([]ports.ClaimAuthorizationCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.ClaimAuthorizationCatalogueRow
		var applicantsJSON []byte
		if err := rows.Scan(&row.Customer, &row.Version, &row.ApprovedBy, &applicantsJSON); err != nil {
			return nil, fmt.Errorf("list claim authorizations: %w", err)
		}
		if err := json.Unmarshal(applicantsJSON, &row.Applicants); err != nil {
			return nil, fmt.Errorf("list claim authorizations: 名单解码：%w", err)
		}
		catalogueRows = append(catalogueRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list claim authorizations: %w", err)
	}
	return catalogueRows, nil
}

// disclosureCellDocument 是披露条目一维在 json_agg 里的临时词形。content 用指针收
// NULL：非展示维的内容列是 NULL（0012 的 shape 约束），译回空串——空串在读口不歧义，
// 端口注释记明「Content 只在 SHOWN 时非空」。
type disclosureCellDocument struct {
	State   string  `json:"state"`
	Content *string `json:"content"`
}

func (document disclosureCellDocument) cell() ports.DisclosureDimensionCell {
	cell := ports.DisclosureDimensionCell{State: document.State}
	if document.Content != nil {
		cell.Content = *document.Content
	}
	return cell
}

type disclosureEntryDocument struct {
	Customer   string                 `json:"customer"`
	Milestones disclosureCellDocument `json:"milestones"`
	ETA        disclosureCellDocument `json:"eta"`
	Final      disclosureCellDocument `json:"final"`
	Note       disclosureCellDocument `json:"note"`
}

// ListDisclosurePolicies 上列披露策略版本连同整版条目（`PAR-VIS-09`）。
func (catalogue *OperationsCatalogue) ListDisclosurePolicies(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.DisclosurePolicyCatalogueRow, error) {
	if err := requirePositiveLimit("list disclosure policies", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list disclosure policies: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT version.policy_version, version.effective_from, version.effective_to,
		        version.approved_by,
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'customer', entry.customer_account_ref,
		                            'milestones', json_build_object(
		                                'state', entry.milestones_state, 'content', entry.milestones_content),
		                            'eta', json_build_object(
		                                'state', entry.eta_state, 'content', entry.eta_content),
		                            'final', json_build_object(
		                                'state', entry.final_state, 'content', entry.final_content),
		                            'note', json_build_object(
		                                'state', entry.note_state, 'content', entry.note_content)
		                        )
		                        ORDER BY entry.customer_account_ref
		                    ),
		                    '[]'::json
		                )
		           FROM visibility_exception.disclosure_policy_entry AS entry
		          WHERE entry.tenant_id      = version.tenant_id
		            AND entry.policy_version = version.policy_version)
		   FROM visibility_exception.disclosure_policy_version AS version
		  WHERE version.tenant_id = $1
		  ORDER BY version.effective_from DESC, version.policy_version
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list disclosure policies: %w", err)
	}
	defer rows.Close()

	policyRows := make([]ports.DisclosurePolicyCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.DisclosurePolicyCatalogueRow
		var effectiveTo *time.Time
		var entriesJSON []byte
		if err := rows.Scan(
			&row.Version, &row.EffectiveFrom, &effectiveTo, &row.ApprovedBy, &entriesJSON,
		); err != nil {
			return nil, fmt.Errorf("list disclosure policies: %w", err)
		}
		if effectiveTo != nil {
			row.EffectiveTo = *effectiveTo
			row.HasEffectiveTo = true
		}
		var documents []disclosureEntryDocument
		if err := json.Unmarshal(entriesJSON, &documents); err != nil {
			return nil, fmt.Errorf("list disclosure policies: 条目集解码：%w", err)
		}
		row.Entries = make([]ports.DisclosurePolicyEntryRow, 0, len(documents))
		for _, document := range documents {
			row.Entries = append(row.Entries, ports.DisclosurePolicyEntryRow{
				Customer:   document.Customer,
				Milestones: document.Milestones.cell(),
				ETA:        document.ETA.cell(),
				Final:      document.Final.cell(),
				Note:       document.Note.cell(),
			})
		}
		policyRows = append(policyRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list disclosure policies: %w", err)
	}
	return policyRows, nil
}
