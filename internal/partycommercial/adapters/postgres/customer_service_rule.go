package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 客户服务规则册的持久化面（票 party-commercial-context-gaps/05，ADR-0104，0023 迁移）。写口挂在
// CommercialPublications 上与其余正文册同笔登记；读口是独立的 CustomerServiceRuleContents——正文
// 不进整册装载（LoadForScope），理由见 ports.CustomerServiceRuleContentView。
//
// 判读纪律同六族声明表（declaration_publication.go）：**先读回既有正文再决定写不写**。正文横跨
// 父子三表，逐行 DO NOTHING 会把半份新正文并进旧正文——冲突路径必须一行不写，否则「绝不覆盖」
// 只对父行成立。

// SaveCustomerServiceRule 登记一份客户服务规则版本的正文。撞键不覆盖：同内容是重放，异内容
// （适用对象、责任方、范围、任一条期限、任一份材料清单不同）是需要商业责任方修正的冲突。
func (repository *CommercialPublications) SaveCustomerServiceRule(
	ctx context.Context,
	rule domain.CustomerServiceRuleVersion,
) (ports.CustomerServiceRuleSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CustomerServiceRuleSaveOutcomeInvalid, fmt.Errorf("save customer service rule: %w", err)
	}

	product, contract := customerServiceRuleApplicabilityColumns(rule.Applicability())
	incoming := customerServiceRuleItemsOf(rule)
	if (product == nil && contract == nil) || len(incoming.deadlines)+len(incoming.materials) == 0 {
		// 零值规则过不了 NewCustomerServiceRuleVersion，走到这里只可能是绕开构造门的零值。拦在
		// INSERT 前，否则 CHECK 会以一条技术错误报出一件领域上早该拒绝的事。
		return ports.CustomerServiceRuleSaveOutcomeInvalid,
			fmt.Errorf("save customer service rule: %w", domain.ErrInvalidCustomerServiceRuleVersion)
	}
	tenant, kind, objectID, label := ownerColumns(rule.Version())

	var existing scannedCustomerServiceRule
	present, err := scanCustomerServiceRuleParent(ctx, executor, &existing, tenant, kind, objectID, label)
	if err != nil {
		return ports.CustomerServiceRuleSaveOutcomeInvalid, fmt.Errorf("save customer service rule: %w", err)
	}
	if present {
		if !sameOptionalText(existing.product, product) ||
			!sameOptionalText(existing.contract, contract) ||
			existing.responsible != rule.ResponsibleParty().String() ||
			existing.scope != rule.Scope().String() {
			return ports.CustomerServiceRuleContentConflict, nil
		}
		existingItems, err := scanCustomerServiceRuleItems(ctx, executor, tenant, kind, objectID, label)
		if err != nil {
			return ports.CustomerServiceRuleSaveOutcomeInvalid, fmt.Errorf("save customer service rule: %w", err)
		}
		if !existingItems.equal(incoming) {
			return ports.CustomerServiceRuleContentConflict, nil
		}
		return ports.CustomerServiceRuleAlreadyRegistered, nil
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.customer_service_rule
			(tenant_id, object_kind, object_id, version_label,
			 service_product_id, customer_contract_id, responsible_party_id, scope_ref)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		tenant, kind, objectID, label,
		product, contract, rule.ResponsibleParty().String(), rule.Scope().String(),
	); err != nil {
		return ports.CustomerServiceRuleSaveOutcomeInvalid, fmt.Errorf("save customer service rule: %w", err)
	}
	for _, deadline := range rule.ClaimDeadlines() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO party_commercial.customer_service_rule_claim_deadline
				(tenant_id, object_kind, object_id, version_label,
				 deadline_kind, start_event_ref, duration_days, calendar_ref)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			tenant, kind, objectID, label,
			deadline.Kind().String(), deadline.StartEvent().String(), deadline.DurationDays(), deadline.Calendar().String(),
		); err != nil {
			return ports.CustomerServiceRuleSaveOutcomeInvalid, fmt.Errorf("save customer service rule: %w", err)
		}
	}
	for _, materials := range rule.MinimumMaterials() {
		for _, material := range materials.Materials() {
			if _, err := executor.Exec(ctx,
				`INSERT INTO party_commercial.customer_service_rule_minimum_material
					(tenant_id, object_kind, object_id, version_label, claim_kind_ref, material_ref)
				 VALUES ($1, $2, $3, $4, $5, $6)`,
				tenant, kind, objectID, label,
				materials.ClaimKind().String(), material.String(),
			); err != nil {
				return ports.CustomerServiceRuleSaveOutcomeInvalid, fmt.Errorf("save customer service rule: %w", err)
			}
		}
	}
	return ports.CustomerServiceRuleSaved, nil
}

type scannedCustomerServiceRule struct {
	product     *string
	contract    *string
	responsible string
	scope       string
}

// customerServiceRuleApplicabilityColumns 把两格封闭的适用声明摊成两列，恰一非空——列上 CHECK 是
// 这一条的镜像（判据同 creditLimitColumns）。
func customerServiceRuleApplicabilityColumns(applicability domain.CustomerServiceRuleApplicability) (*string, *string) {
	if product, applies := applicability.ServiceProduct(); applies {
		value := product.String()
		return &value, nil
	}
	if contract, applies := applicability.CustomerContract(); applies {
		value := contract.String()
		return nil, &value
	}
	return nil, nil
}

// customerServiceRuleApplicabilityFrom 把两列折回领域构造门。CHECK 保证恰一列在场，走到两空/两满
// 说明库与领域已经分叉，报错不吸收。
func customerServiceRuleApplicabilityFrom(product, contract *string) (domain.CustomerServiceRuleApplicability, error) {
	switch {
	case product != nil && contract == nil:
		id, err := domain.NewCommercialObjectID(*product)
		if err != nil {
			return domain.CustomerServiceRuleApplicability{}, err
		}
		return domain.CustomerServiceRuleAppliesToServiceProduct(id), nil
	case product == nil && contract != nil:
		id, err := domain.NewCommercialObjectID(*contract)
		if err != nil {
			return domain.CustomerServiceRuleApplicability{}, err
		}
		return domain.CustomerServiceRuleAppliesToCustomerContract(id), nil
	default:
		return domain.CustomerServiceRuleApplicability{},
			fmt.Errorf("customer service rule row applies to neither a product nor a contract exactly")
	}
}

func sameOptionalText(left, right *string) bool {
	return (left == nil && right == nil) ||
		(left != nil && right != nil && *left == *right)
}

// customerServiceRuleItems 是两项正文按键归档后的比对形：期限按种类，材料按「类型 + 条目」逐条。
// 比对按映射/集合进行，声明顺序不构成不同的内容。
type customerServiceRuleItems struct {
	deadlines map[string]scannedClaimDeadline
	materials map[string]bool
}

type scannedClaimDeadline struct {
	startEvent string
	days       int
	calendar   string
}

func (items customerServiceRuleItems) equal(other customerServiceRuleItems) bool {
	if len(items.deadlines) != len(other.deadlines) || len(items.materials) != len(other.materials) {
		return false
	}
	for kind, deadline := range items.deadlines {
		if other.deadlines[kind] != deadline {
			return false
		}
	}
	for key := range items.materials {
		if !other.materials[key] {
			return false
		}
	}
	return true
}

func customerServiceRuleItemsOf(rule domain.CustomerServiceRuleVersion) customerServiceRuleItems {
	items := customerServiceRuleItems{
		deadlines: make(map[string]scannedClaimDeadline),
		materials: make(map[string]bool),
	}
	for _, deadline := range rule.ClaimDeadlines() {
		items.deadlines[deadline.Kind().String()] = scannedClaimDeadline{
			startEvent: deadline.StartEvent().String(),
			days:       deadline.DurationDays(),
			calendar:   deadline.Calendar().String(),
		}
	}
	for _, materials := range rule.MinimumMaterials() {
		for _, material := range materials.Materials() {
			items.materials[materials.ClaimKind().String()+"\x1f"+material.String()] = true
		}
	}
	return items
}

func scanCustomerServiceRuleParent(
	ctx context.Context,
	executor bentopg.Executor,
	target *scannedCustomerServiceRule,
	tenant string, kind uint8, objectID, label string,
) (bool, error) {
	rows, err := executor.Query(ctx,
		`SELECT service_product_id, customer_contract_id, responsible_party_id, scope_ref
		   FROM party_commercial.customer_service_rule
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant, kind, objectID, label)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	present := false
	for rows.Next() {
		if err := rows.Scan(&target.product, &target.contract, &target.responsible, &target.scope); err != nil {
			return false, err
		}
		present = true
	}
	return present, rows.Err()
}

func scanCustomerServiceRuleItems(
	ctx context.Context,
	executor bentopg.Executor,
	tenant string, kind uint8, objectID, label string,
) (customerServiceRuleItems, error) {
	items := customerServiceRuleItems{
		deadlines: make(map[string]scannedClaimDeadline),
		materials: make(map[string]bool),
	}
	rows, err := executor.Query(ctx,
		`SELECT deadline_kind, start_event_ref, duration_days, calendar_ref
		   FROM party_commercial.customer_service_rule_claim_deadline
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant, kind, objectID, label)
	if err != nil {
		return items, err
	}
	for rows.Next() {
		var deadlineKind string
		var deadline scannedClaimDeadline
		if err := rows.Scan(&deadlineKind, &deadline.startEvent, &deadline.days, &deadline.calendar); err != nil {
			rows.Close()
			return items, err
		}
		items.deadlines[deadlineKind] = deadline
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return items, err
	}
	items.materials, err = scanTextSet(ctx, executor,
		`SELECT claim_kind_ref || '`+"\x1f"+`' || material_ref
		   FROM party_commercial.customer_service_rule_minimum_material
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant, kind, objectID, label)
	return items, err
}

// CustomerServiceRuleContents 实现 ports.CustomerServiceRuleContentView：按已唯一选出的客户服务
// 规则版本取回正文。
//
// 只读。正文属实例半边，本适配器不提供写口（写口在 CommercialPublications 上随发布同笔），也不在
// 读不到时代拟任何期限或材料——缺规则就是未登记，visibility-exception 据以停在指名到维的未决。
type CustomerServiceRuleContents struct {
	db *bentopg.DB
}

func NewCustomerServiceRuleContents(db *bentopg.DB) (*CustomerServiceRuleContents, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &CustomerServiceRuleContents{db: db}, nil
}

var _ ports.CustomerServiceRuleContentView = (*CustomerServiceRuleContents)(nil)

// LoadCustomerServiceRule 取回客户服务规则正文。
//
// found=false = 正文未登记（无父行）。显式租户与版本必须同一身份，否则 error 且不交内容
// （ADR-0003/0040，租户是身份不是过滤器）。父行与两张子表由一条语句取回：ReadExecutor 不保证
// 多条语句同一快照，分几次会拼出从未同时存在的父子状态。读回的正文过 NewCustomerServiceRuleVersion
// 重建——有父无子在那里被拒，上抛不折成未登记；再过 ConsistentCustomerServiceRuleApplicability
// 核壳与正文所挂对象一致（ADR-0104 Decision 四），分歧是坏数据不是「没配规则」。
func (repository *CustomerServiceRuleContents) LoadCustomerServiceRule(
	ctx context.Context,
	tenant domain.TenantID,
	rule domain.CommercialVersion,
) (domain.CustomerServiceRuleVersion, bool, error) {
	none := domain.CustomerServiceRuleVersion{}
	if tenant.String() == "" ||
		rule.ObjectID().String() == "" || rule.Version().String() == "" {
		return none, false, fmt.Errorf("load customer service rule: tenant and rule identity are required")
	}
	if tenant != rule.Tenant() {
		return none, false, fmt.Errorf("load customer service rule: tenant does not own this rule")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load customer service rule: %w", err)
	}

	var row scannedCustomerServiceRule
	var deadlinesJSON, materialsJSON []byte
	err = querier.QueryRow(ctx,
		`SELECT parent.service_product_id, parent.customer_contract_id,
		        parent.responsible_party_id, parent.scope_ref,
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'kind',       deadline.deadline_kind,
		                            'startEvent', deadline.start_event_ref,
		                            'days',       deadline.duration_days,
		                            'calendar',   deadline.calendar_ref
		                        )
		                        ORDER BY deadline.deadline_kind
		                    ),
		                    '[]'::json
		                )
		           FROM party_commercial.customer_service_rule_claim_deadline AS deadline
		          WHERE deadline.tenant_id     = parent.tenant_id
		            AND deadline.object_kind   = parent.object_kind
		            AND deadline.object_id     = parent.object_id
		            AND deadline.version_label = parent.version_label),
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'claimKind', material.claim_kind_ref,
		                            'material',  material.material_ref
		                        )
		                        ORDER BY material.claim_kind_ref, material.material_ref
		                    ),
		                    '[]'::json
		                )
		           FROM party_commercial.customer_service_rule_minimum_material AS material
		          WHERE material.tenant_id     = parent.tenant_id
		            AND material.object_kind   = parent.object_kind
		            AND material.object_id     = parent.object_id
		            AND material.version_label = parent.version_label)
		   FROM party_commercial.customer_service_rule AS parent
		  WHERE parent.tenant_id     = $1
		    AND parent.object_kind   = $2
		    AND parent.object_id     = $3
		    AND parent.version_label = $4`,
		tenant.String(),
		uint8(domain.CustomerServiceRuleObject),
		rule.ObjectID().String(),
		rule.Version().String(),
	).Scan(&row.product, &row.contract, &row.responsible, &row.scope, &deadlinesJSON, &materialsJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load customer service rule: %w", err)
	}

	content, err := customerServiceRuleFrom(rule, row, deadlinesJSON, materialsJSON)
	if err != nil {
		return none, false, fmt.Errorf("load customer service rule: %w", err)
	}
	return content, true, nil
}

func customerServiceRuleFrom(
	version domain.CommercialVersion,
	row scannedCustomerServiceRule,
	deadlinesJSON, materialsJSON []byte,
) (domain.CustomerServiceRuleVersion, error) {
	none := domain.CustomerServiceRuleVersion{}
	applicability, err := customerServiceRuleApplicabilityFrom(row.product, row.contract)
	if err != nil {
		return none, err
	}
	if err := domain.ConsistentCustomerServiceRuleApplicability(version, applicability); err != nil {
		return none, err
	}
	responsible, err := domain.NewPartyID(row.responsible)
	if err != nil {
		return none, err
	}
	scope, err := domain.NewCommercialScopeReference(row.scope)
	if err != nil {
		return none, err
	}
	deadlines, err := claimDeadlinesFromJSON(deadlinesJSON)
	if err != nil {
		return none, err
	}
	materials, err := minimumMaterialsFromJSON(materialsJSON)
	if err != nil {
		return none, err
	}
	return domain.NewCustomerServiceRuleVersion(version, applicability, responsible, scope, deadlines, materials)
}

type claimDeadlineDocument struct {
	Kind       string `json:"kind"`
	StartEvent string `json:"startEvent"`
	Days       int    `json:"days"`
	Calendar   string `json:"calendar"`
}

func claimDeadlinesFromJSON(raw []byte) ([]domain.ClaimDeadlineRule, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var documents []claimDeadlineDocument
	if err := json.Unmarshal(raw, &documents); err != nil {
		return nil, fmt.Errorf("claim deadlines are not this adapter's shape: %w", err)
	}
	deadlines := make([]domain.ClaimDeadlineRule, 0, len(documents))
	for _, document := range documents {
		kind, err := claimDeadlineKindFrom(document.Kind)
		if err != nil {
			return nil, err
		}
		startEvent, err := domain.NewDeadlineStartEventReference(document.StartEvent)
		if err != nil {
			return nil, err
		}
		calendar, err := domain.NewBusinessCalendarReference(document.Calendar)
		if err != nil {
			return nil, err
		}
		deadline, err := domain.NewClaimDeadlineRule(kind, startEvent, document.Days, calendar)
		if err != nil {
			return nil, err
		}
		deadlines = append(deadlines, deadline)
	}
	return deadlines, nil
}

type minimumMaterialDocument struct {
	ClaimKind string `json:"claimKind"`
	Material  string `json:"material"`
}

// minimumMaterialsFromJSON 把逐条成行的材料条目按索赔类型归回清单。行已按类型、条目排序取回，
// 同一类型的条目因此连续，归组按顺序扫一遍即可；清单本身再经 NewMinimumMaterialsRule 重建。
func minimumMaterialsFromJSON(raw []byte) ([]domain.MinimumMaterialsRule, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var documents []minimumMaterialDocument
	if err := json.Unmarshal(raw, &documents); err != nil {
		return nil, fmt.Errorf("minimum materials are not this adapter's shape: %w", err)
	}
	var rules []domain.MinimumMaterialsRule
	var currentKind string
	var current []domain.MaterialRequirementReference
	flush := func() error {
		if len(current) == 0 {
			return nil
		}
		claimKind, err := domain.NewClaimKindReference(currentKind)
		if err != nil {
			return err
		}
		rule, err := domain.NewMinimumMaterialsRule(claimKind, current)
		if err != nil {
			return err
		}
		rules = append(rules, rule)
		current = nil
		return nil
	}
	for _, document := range documents {
		if document.ClaimKind != currentKind {
			if err := flush(); err != nil {
				return nil, err
			}
			currentKind = document.ClaimKind
		}
		material, err := domain.NewMaterialRequirementReference(document.Material)
		if err != nil {
			return nil, err
		}
		current = append(current, material)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return rules, nil
}

// ListCustomerServiceRules 上列客户服务规则版本壳，正文（0023）左连接。
//
// 装载方向与 ListAuthorizationRules 同派：版本侧驱动，正文左连接——壳可先入册、正文随发布登记，
// 只列正文行会让未登正文的已发布规则版本从目录上消失，而那个状态正是 visibility-exception 点读答
// 未登记的状态，目录必须让它可见。两张子表各由一个子查询聚成 json 数组，与父行同一条语句取回。
//
// 目录不重建领域对象、不形成判断：有父行而两张子表都空是坏数据（领域要求至少一项），拦它归内容
// 读口 LoadCustomerServiceRule；这里照 ListAcceptanceRulePackages 的先例如实交回空集合。
func (catalogue *OperationsCatalogue) ListCustomerServiceRules(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.CustomerServiceRuleRow, error) {
	if err := requirePositiveLimit("list customer service rules", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list customer service rules: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT version.object_id, version.version_label, version.scope_ref, version.status,
		        version.effective_starts_at, version.effective_ends_at, version.published_at,
		        content.service_product_id, content.customer_contract_id,
		        content.responsible_party_id, content.scope_ref, content.registered_at,
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'kind',       deadline.deadline_kind,
		                            'startEvent', deadline.start_event_ref,
		                            'days',       deadline.duration_days,
		                            'calendar',   deadline.calendar_ref
		                        )
		                        ORDER BY deadline.deadline_kind
		                    ),
		                    '[]'::json
		                )
		           FROM party_commercial.customer_service_rule_claim_deadline AS deadline
		          WHERE deadline.tenant_id     = version.tenant_id
		            AND deadline.object_kind   = version.object_kind
		            AND deadline.object_id     = version.object_id
		            AND deadline.version_label = version.version_label),
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'claimKind', material.claim_kind_ref,
		                            'material',  material.material_ref
		                        )
		                        ORDER BY material.claim_kind_ref, material.material_ref
		                    ),
		                    '[]'::json
		                )
		           FROM party_commercial.customer_service_rule_minimum_material AS material
		          WHERE material.tenant_id     = version.tenant_id
		            AND material.object_kind   = version.object_kind
		            AND material.object_id     = version.object_id
		            AND material.version_label = version.version_label)
		   FROM party_commercial.commercial_version AS version
		   LEFT JOIN party_commercial.customer_service_rule AS content
		          ON content.tenant_id     = version.tenant_id
		         AND content.object_kind   = version.object_kind
		         AND content.object_id     = version.object_id
		         AND content.version_label = version.version_label
		  WHERE version.tenant_id   = $1
		    AND version.object_kind = $2
		  ORDER BY version.published_at DESC, version.object_id, version.version_label
		  LIMIT $3`,
		tenant.String(),
		uint8(domain.CustomerServiceRuleObject),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list customer service rules: %w", err)
	}
	defer rows.Close()

	ruleRows := make([]ports.CustomerServiceRuleRow, 0, limit)
	for rows.Next() {
		var row ports.CustomerServiceRuleRow
		var status int16
		var endsAt, registeredAt *time.Time
		var product, contract, responsible, ruleScope *string
		var deadlinesJSON, materialsJSON []byte
		if err := rows.Scan(
			&row.ObjectID, &row.VersionLabel, &row.Scope, &status,
			&row.EffectiveStartsAt, &endsAt, &row.PublishedAt,
			&product, &contract, &responsible, &ruleScope, &registeredAt,
			&deadlinesJSON, &materialsJSON,
		); err != nil {
			return nil, fmt.Errorf("list customer service rules: %w", err)
		}
		statusWord := domain.CommercialVersionStatus(status).String()
		if statusWord == "" {
			return nil, fmt.Errorf("list customer service rules: 版本状态 %d 不在封闭集内", status)
		}
		row.Status = statusWord
		if endsAt != nil {
			row.EffectiveEndsAt = *endsAt
			row.HasEffectiveEnd = true
		}
		// registered_at 在正文父行上 NOT NULL，它的在场即正文的在场。
		if registeredAt != nil {
			row.HasContent = true
			row.RegisteredAt = *registeredAt
			switch {
			case product != nil && contract == nil:
				row.ServiceProduct = *product
			case product == nil && contract != nil:
				row.CustomerContract = *contract
			default:
				return nil, fmt.Errorf("list customer service rules: %s/%s 的适用声明既不是产品也不是合同",
					row.ObjectID, row.VersionLabel)
			}
			row.ResponsibleParty = *responsible
			row.RuleScope = *ruleScope
		}
		if row.ClaimDeadlines, err = claimDeadlineRowsFromJSON(deadlinesJSON); err != nil {
			return nil, fmt.Errorf("list customer service rules: %w", err)
		}
		if row.MinimumMaterials, err = minimumMaterialsRowsFromJSON(materialsJSON); err != nil {
			return nil, fmt.Errorf("list customer service rules: %w", err)
		}
		ruleRows = append(ruleRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list customer service rules: %w", err)
	}
	return ruleRows, nil
}

// claimDeadlineRowsFromJSON 只转写，不校验种类是否在封闭三值内——判据同 cancellationAuthorityRowsFromJSON：
// 目录不重建领域对象，拦坏数据归内容读口。
func claimDeadlineRowsFromJSON(raw []byte) ([]ports.ClaimDeadlineRow, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var documents []claimDeadlineDocument
	if err := json.Unmarshal(raw, &documents); err != nil {
		return nil, fmt.Errorf("claim deadlines are not this adapter's shape: %w", err)
	}
	deadlines := make([]ports.ClaimDeadlineRow, 0, len(documents))
	for _, document := range documents {
		deadlines = append(deadlines, ports.ClaimDeadlineRow{
			Kind:         document.Kind,
			StartEvent:   document.StartEvent,
			DurationDays: document.Days,
			Calendar:     document.Calendar,
		})
	}
	return deadlines, nil
}

// minimumMaterialsRowsFromJSON 把逐条成行的材料条目按索赔类型归回清单（行已按类型、条目排序取回，
// 同类型连续）。只转写不校验，判据同上。
func minimumMaterialsRowsFromJSON(raw []byte) ([]ports.MinimumMaterialsRow, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var documents []minimumMaterialDocument
	if err := json.Unmarshal(raw, &documents); err != nil {
		return nil, fmt.Errorf("minimum materials are not this adapter's shape: %w", err)
	}
	rows := make([]ports.MinimumMaterialsRow, 0)
	for _, document := range documents {
		if len(rows) == 0 || rows[len(rows)-1].ClaimKind != document.ClaimKind {
			rows = append(rows, ports.MinimumMaterialsRow{ClaimKind: document.ClaimKind})
		}
		last := &rows[len(rows)-1]
		last.Materials = append(last.Materials, document.Material)
	}
	return rows, nil
}

// claimDeadlineKindFrom 只认三个取值，default 报错不吸收——库上 CHECK 已经钉死，读回集外取值即库与
// 领域分叉。
func claimDeadlineKindFrom(raw string) (domain.ClaimDeadlineKind, error) {
	for _, kind := range []domain.ClaimDeadlineKind{
		domain.FirstClaimDeadline,
		domain.MaterialSupplementDeadline,
		domain.ConclusionReviewDeadline,
	} {
		if kind.String() == raw {
			return kind, nil
		}
	}
	return domain.ClaimDeadlineKindInvalid, fmt.Errorf("unknown claim deadline kind %q", strings.TrimSpace(raw))
}
