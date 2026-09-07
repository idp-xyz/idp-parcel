package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件是六族声明表的写入半边（syn-wall-door-audit 票 03），挂在 CommercialPublications
// 上实现 ports.PublicationRegistry 的九个声明 Save。
//
// 判读纪律与 SaveVersion 相反：**先读回既有正文再决定写不写**。SaveVersion 靠单行
// ON CONFLICT 撞键后读回；声明正文横跨父子多行，逐行 DO NOTHING 会把半份新正文并进
// 旧正文——冲突路径必须一行不写，否则「绝不覆盖」只对单行成立。既有正文在场即只比对：
// 同内容答`已登记`（重放）、异内容答`内容冲突`。既有缺席才插入，全部用普通 INSERT——
// 并发同键首插由主键兜底，第二个事务撞唯一违反上抛 error 整项回滚，不会留下混种正文。
//
// 所有比对都按集合/映射进行，声明顺序不构成不同的内容（与登记册 sameReleasedContent
// 对区间与引用的立场一致）。

// ownerColumns 取声明拥有版本的四维身份。类别取自领域对象而不是写死：每张表的
// CHECK 已把类别钉在正确取值上，这里再写一遍数字只会多一处要同步的镜像。
func ownerColumns(version domain.CommercialVersion) (string, uint8, string, string) {
	return version.Tenant().String(), uint8(version.Kind()), version.ObjectID().String(), version.Version().String()
}

// SaveAsOfPolicies 登记一份规则包的整套时点锚声明（PAR-COM-14 的机制半边）。
// 整套是登记单位：单条判断的锚离开同套其余判断没有独立含义，半套在场的中间态
// 也不该存在（DeclareAsOfPolicies 拒绝冲突与缺件）。
func (repository *CommercialPublications) SaveAsOfPolicies(
	ctx context.Context,
	declaration domain.AsOfDeclaration,
) (ports.DeclarationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save as-of policies: %w", err)
	}
	tenant, kind, objectID, label := ownerColumns(declaration.RulePackage())
	incoming := declaration.Policies()

	existing, err := scanAsOfRows(ctx, executor, tenant, kind, objectID, label)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save as-of policies: %w", err)
	}
	if len(existing) > 0 {
		if len(existing) != len(incoming) {
			return ports.DeclarationContentConflict, nil
		}
		for _, policy := range incoming {
			found, present := existing[policy.Judgment().String()]
			if !present ||
				found.semantics != policy.Semantics().String() ||
				found.policyVersion != policy.PolicyVersion().String() {
				return ports.DeclarationContentConflict, nil
			}
		}
		return ports.DeclarationAlreadyRegistered, nil
	}

	for _, policy := range incoming {
		if _, err := executor.Exec(ctx,
			`INSERT INTO party_commercial.as_of_policy_declaration
				(tenant_id, object_kind, object_id, version_label, judgment_type, semantics_ref, policy_version)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			tenant, kind, objectID, label,
			policy.Judgment().String(), policy.Semantics().String(), policy.PolicyVersion().String(),
		); err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save as-of policies: %w", err)
		}
	}
	return ports.DeclarationSaved, nil
}

type asOfRow struct {
	semantics     string
	policyVersion string
}

func scanAsOfRows(
	ctx context.Context,
	executor bentopg.Executor,
	tenant string, kind uint8, objectID, label string,
) (map[string]asOfRow, error) {
	rows, err := executor.Query(ctx,
		`SELECT judgment_type, semantics_ref, policy_version
		   FROM party_commercial.as_of_policy_declaration
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant, kind, objectID, label)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	existing := make(map[string]asOfRow)
	for rows.Next() {
		var judgment, semantics, policyVersion string
		if err := rows.Scan(&judgment, &semantics, &policyVersion); err != nil {
			return nil, err
		}
		existing[judgment] = asOfRow{semantics: semantics, policyVersion: policyVersion}
	}
	return existing, rows.Err()
}

// SaveAcceptanceRuleContent 登记一份规则包的接受内容声明（ADR-0042）：适用校验组与
// 人工复核指令同属一行，组集合在子表逐行。
func (repository *CommercialPublications) SaveAcceptanceRuleContent(
	ctx context.Context,
	content domain.AcceptanceRuleContent,
) (ports.DeclarationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save acceptance rule content: %w", err)
	}
	tenant, kind, objectID, label := ownerColumns(content.RulePackage())

	var existingReview string
	present, err := scanSingleText(ctx, executor,
		`SELECT manual_review
		   FROM party_commercial.acceptance_rule_content
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		&existingReview, tenant, kind, objectID, label)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save acceptance rule content: %w", err)
	}
	if present {
		existingGroups, err := scanTextSet(ctx, executor,
			`SELECT check_group
			   FROM party_commercial.acceptance_rule_check_group
			  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
			tenant, kind, objectID, label)
		if err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save acceptance rule content: %w", err)
		}
		incoming := content.ApplicableGroups()
		if existingReview != content.ManualReview().String() || len(existingGroups) != len(incoming) {
			return ports.DeclarationContentConflict, nil
		}
		for _, group := range incoming {
			if !existingGroups[group.String()] {
				return ports.DeclarationContentConflict, nil
			}
		}
		return ports.DeclarationAlreadyRegistered, nil
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.acceptance_rule_content
			(tenant_id, object_kind, object_id, version_label, manual_review)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenant, kind, objectID, label, content.ManualReview().String(),
	); err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save acceptance rule content: %w", err)
	}
	for _, group := range content.ApplicableGroups() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO party_commercial.acceptance_rule_check_group
				(tenant_id, object_kind, object_id, version_label, check_group)
			 VALUES ($1, $2, $3, $4, $5)`,
			tenant, kind, objectID, label, group.String(),
		); err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save acceptance rule content: %w", err)
		}
	}
	return ports.DeclarationSaved, nil
}

// SavePendingRoutingPermission 登记一份服务产品的待路由许可（ADR-0042）。许可必须
// 携带依据，由领域构造门与库内 NOT NULL 双重保证。
func (repository *CommercialPublications) SavePendingRoutingPermission(
	ctx context.Context,
	permission domain.PendingRoutingPermission,
) (ports.DeclarationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save pending routing permission: %w", err)
	}
	tenant, kind, objectID, label := ownerColumns(permission.Product())

	var existingBasis string
	present, err := scanSingleText(ctx, executor,
		`SELECT basis_ref
		   FROM party_commercial.pending_routing_permission
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		&existingBasis, tenant, kind, objectID, label)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save pending routing permission: %w", err)
	}
	if present {
		if existingBasis != permission.Basis().String() {
			return ports.DeclarationContentConflict, nil
		}
		return ports.DeclarationAlreadyRegistered, nil
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.pending_routing_permission
			(tenant_id, object_kind, object_id, version_label, basis_ref)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenant, kind, objectID, label, permission.Basis().String(),
	); err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save pending routing permission: %w", err)
	}
	return ports.DeclarationSaved, nil
}

// SavePreAcceptanceControl 登记一份客户合同版本的接受前财务控制声明（PAR-COM-15）。
// `不适用`必带依据、`要求控制`必不带——两头由领域构造门保证，库内 CHECK 镜像。
func (repository *CommercialPublications) SavePreAcceptanceControl(
	ctx context.Context,
	declaration domain.PreAcceptanceControlDeclaration,
) (ports.DeclarationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save pre-acceptance control: %w", err)
	}
	tenant, kind, objectID, label := ownerColumns(declaration.Contract())

	var incomingBasis *string
	if basis, inapplicable := declaration.NotApplicableBasis(); inapplicable {
		value := basis.String()
		incomingBasis = &value
	}

	rows, err := executor.Query(ctx,
		`SELECT requirement, not_applicable_basis
		   FROM party_commercial.pre_acceptance_control_declaration
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant, kind, objectID, label)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save pre-acceptance control: %w", err)
	}
	var existingRequirement string
	var existingBasis *string
	present := false
	for rows.Next() {
		if err := rows.Scan(&existingRequirement, &existingBasis); err != nil {
			rows.Close()
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save pre-acceptance control: %w", err)
		}
		present = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save pre-acceptance control: %w", err)
	}

	if present {
		sameBasis := (existingBasis == nil && incomingBasis == nil) ||
			(existingBasis != nil && incomingBasis != nil && *existingBasis == *incomingBasis)
		if existingRequirement != declaration.Requirement().String() || !sameBasis {
			return ports.DeclarationContentConflict, nil
		}
		return ports.DeclarationAlreadyRegistered, nil
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.pre_acceptance_control_declaration
			(tenant_id, object_kind, object_id, version_label, requirement, not_applicable_basis)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		tenant, kind, objectID, label, declaration.Requirement().String(), incomingBasis,
	); err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save pre-acceptance control: %w", err)
	}
	return ports.DeclarationSaved, nil
}

// SaveCustomerContractContent 登记一份客户合同版本的正文：规则包引用 + 按费用范围的
// 财务控制约定。零约定是合法的显式空（父行在场零子行），与未登记分得开。
func (repository *CommercialPublications) SaveCustomerContractContent(
	ctx context.Context,
	contract domain.CustomerContract,
) (ports.DeclarationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save customer contract content: %w", err)
	}
	tenant, kind, objectID, label := ownerColumns(contract.Version())

	var existingRulePackage string
	present, err := scanSingleText(ctx, executor,
		`SELECT rule_package_id
		   FROM party_commercial.customer_contract_content
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		&existingRulePackage, tenant, kind, objectID, label)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save customer contract content: %w", err)
	}
	incoming := contract.Bindings()
	if present {
		existing, err := scanContractBindings(ctx, executor, tenant, kind, objectID, label)
		if err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save customer contract content: %w", err)
		}
		if existingRulePackage != contract.AcceptanceRulePackage().String() || len(existing) != len(incoming) {
			return ports.DeclarationContentConflict, nil
		}
		for _, binding := range incoming {
			found, bound := existing[binding.Scope().String()]
			if !bound || !sameControlBinding(found, binding) {
				return ports.DeclarationContentConflict, nil
			}
		}
		return ports.DeclarationAlreadyRegistered, nil
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.customer_contract_content
			(tenant_id, object_kind, object_id, version_label, rule_package_id)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenant, kind, objectID, label, contract.AcceptanceRulePackage().String(),
	); err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save customer contract content: %w", err)
	}
	for _, binding := range incoming {
		var policyID, basis *string
		if policy, applies := binding.Policy(); applies {
			value := policy.String()
			policyID = &value
		} else {
			value := binding.InapplicabilityBasis().String()
			basis = &value
		}
		if _, err := executor.Exec(ctx,
			`INSERT INTO party_commercial.customer_contract_control_binding
				(tenant_id, object_kind, object_id, version_label, charge_scope_ref, policy_id, inapplicability_basis)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			tenant, kind, objectID, label, binding.Scope().String(), policyID, basis,
		); err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save customer contract content: %w", err)
		}
	}
	return ports.DeclarationSaved, nil
}

type contractBindingRow struct {
	policyID *string
	basis    *string
}

func scanContractBindings(
	ctx context.Context,
	executor bentopg.Executor,
	tenant string, kind uint8, objectID, label string,
) (map[string]contractBindingRow, error) {
	rows, err := executor.Query(ctx,
		`SELECT charge_scope_ref, policy_id, inapplicability_basis
		   FROM party_commercial.customer_contract_control_binding
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant, kind, objectID, label)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	existing := make(map[string]contractBindingRow)
	for rows.Next() {
		var scope string
		var row contractBindingRow
		if err := rows.Scan(&scope, &row.policyID, &row.basis); err != nil {
			return nil, err
		}
		existing[scope] = row
	}
	return existing, rows.Err()
}

func sameControlBinding(existing contractBindingRow, incoming domain.FinancialControlBinding) bool {
	if policy, applies := incoming.Policy(); applies {
		return existing.policyID != nil && *existing.policyID == policy.String() && existing.basis == nil
	}
	return existing.basis != nil && *existing.basis == incoming.InapplicabilityBasis().String() &&
		existing.policyID == nil
}

// SaveIntakeQualification 登记一份规则包的收寄资格声明（PAR-COM-16）：允许来源至少
// 一格、硬资格可为显式空清单。
func (repository *CommercialPublications) SaveIntakeQualification(
	ctx context.Context,
	content domain.IntakeQualificationContent,
) (ports.DeclarationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save intake qualification: %w", err)
	}
	tenant, kind, objectID, label := ownerColumns(content.Owner())

	present, err := parentRowPresent(ctx, executor,
		`SELECT 1
		   FROM party_commercial.intake_qualification_content
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant, kind, objectID, label)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save intake qualification: %w", err)
	}
	if present {
		existingSources, err := scanTextSet(ctx, executor,
			`SELECT source_kind
			   FROM party_commercial.intake_allowed_source
			  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
			tenant, kind, objectID, label)
		if err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save intake qualification: %w", err)
		}
		existingRefs, err := scanTextSet(ctx, executor,
			`SELECT rule_reference
			   FROM party_commercial.intake_qualification_ref
			  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
			tenant, kind, objectID, label)
		if err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save intake qualification: %w", err)
		}
		sources := content.AllowedSources()
		qualifications := content.Qualifications()
		if len(existingSources) != len(sources) || len(existingRefs) != len(qualifications) {
			return ports.DeclarationContentConflict, nil
		}
		for _, source := range sources {
			if !existingSources[source.String()] {
				return ports.DeclarationContentConflict, nil
			}
		}
		for _, qualification := range qualifications {
			if !existingRefs[qualification.String()] {
				return ports.DeclarationContentConflict, nil
			}
		}
		return ports.DeclarationAlreadyRegistered, nil
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.intake_qualification_content
			(tenant_id, object_kind, object_id, version_label)
		 VALUES ($1, $2, $3, $4)`,
		tenant, kind, objectID, label,
	); err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save intake qualification: %w", err)
	}
	for _, source := range content.AllowedSources() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO party_commercial.intake_allowed_source
				(tenant_id, object_kind, object_id, version_label, source_kind)
			 VALUES ($1, $2, $3, $4, $5)`,
			tenant, kind, objectID, label, source.String(),
		); err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save intake qualification: %w", err)
		}
	}
	for _, qualification := range content.Qualifications() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO party_commercial.intake_qualification_ref
				(tenant_id, object_kind, object_id, version_label, rule_reference)
			 VALUES ($1, $2, $3, $4, $5)`,
			tenant, kind, objectID, label, qualification.String(),
		); err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save intake qualification: %w", err)
		}
	}
	return ports.DeclarationSaved, nil
}

// SaveFinalRule 登记一份规则包的终局规则声明（PAR-COM-17）：哪些责任结果形成终局、
// 形成哪种，以及父行上那一格可缺的面单有效期（ADR-0119）。缺行是真话（不形成终局），
// 由读侧如实交回；有效期缺席同样是真话（不失效），重放 / 冲突判据把它算进去——同行集合而
// 有效期不同即冲突。
func (repository *CommercialPublications) SaveFinalRule(
	ctx context.Context,
	content domain.FinalRuleContent,
) (ports.DeclarationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save final rule: %w", err)
	}
	tenant, kind, objectID, label := ownerColumns(content.Owner())

	rows, err := executor.Query(ctx,
		`SELECT validity_anchor, validity_duration
		   FROM party_commercial.final_rule_content
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant, kind, objectID, label)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save final rule: %w", err)
	}
	var existingAnchor *string
	var existingDuration pgtype.Interval
	present := false
	for rows.Next() {
		if err := rows.Scan(&existingAnchor, &existingDuration); err != nil {
			rows.Close()
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save final rule: %w", err)
		}
		present = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save final rule: %w", err)
	}
	incoming := content.Declarations()
	if present {
		sameValidity, err := sameLabelValidity(existingAnchor, existingDuration, content)
		if err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save final rule: %w", err)
		}
		if !sameValidity {
			return ports.DeclarationContentConflict, nil
		}
		existing, err := scanTextPairs(ctx, executor,
			`SELECT outcome, final_kind
			   FROM party_commercial.final_rule_declaration
			  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
			tenant, kind, objectID, label)
		if err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save final rule: %w", err)
		}
		if len(existing) != len(incoming) {
			return ports.DeclarationContentConflict, nil
		}
		for _, declaration := range incoming {
			if existing[declaration.Outcome.String()] != declaration.FinalKind.String() {
				return ports.DeclarationContentConflict, nil
			}
		}
		return ports.DeclarationAlreadyRegistered, nil
	}

	validityAnchor, validityDuration := labelValidityColumns(content)
	if _, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.final_rule_content
			(tenant_id, object_kind, object_id, version_label, validity_anchor, validity_duration)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		tenant, kind, objectID, label, validityAnchor, validityDuration,
	); err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save final rule: %w", err)
	}
	for _, declaration := range incoming {
		if _, err := executor.Exec(ctx,
			`INSERT INTO party_commercial.final_rule_declaration
				(tenant_id, object_kind, object_id, version_label, outcome, final_kind)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			tenant, kind, objectID, label, declaration.Outcome.String(), declaration.FinalKind.String(),
		); err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save final rule: %w", err)
		}
	}
	return ports.DeclarationSaved, nil
}

// SaveCancellationAuthority 登记一份授权规则的取消授权目录（PAR-COM-17）。缺行是
// 真话（该请求方不许取消），零行才是缺件——后者由领域构造门拒在门外。
func (repository *CommercialPublications) SaveCancellationAuthority(
	ctx context.Context,
	content domain.CancellationAuthorityContent,
) (ports.DeclarationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save cancellation authority: %w", err)
	}
	tenant, kind, objectID, label := ownerColumns(content.Owner())

	present, err := parentRowPresent(ctx, executor,
		`SELECT 1
		   FROM party_commercial.cancellation_authority_content
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant, kind, objectID, label)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save cancellation authority: %w", err)
	}
	incoming := content.Declarations()
	if present {
		existing, err := scanTextPairs(ctx, executor,
			`SELECT party, rule_reference
			   FROM party_commercial.cancellation_authority_declaration
			  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
			tenant, kind, objectID, label)
		if err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save cancellation authority: %w", err)
		}
		if len(existing) != len(incoming) {
			return ports.DeclarationContentConflict, nil
		}
		for _, declaration := range incoming {
			if existing[declaration.Party.String()] != declaration.Rule.String() {
				return ports.DeclarationContentConflict, nil
			}
		}
		return ports.DeclarationAlreadyRegistered, nil
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.cancellation_authority_content
			(tenant_id, object_kind, object_id, version_label)
		 VALUES ($1, $2, $3, $4)`,
		tenant, kind, objectID, label,
	); err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save cancellation authority: %w", err)
	}
	for _, declaration := range incoming {
		if _, err := executor.Exec(ctx,
			`INSERT INTO party_commercial.cancellation_authority_declaration
				(tenant_id, object_kind, object_id, version_label, party, rule_reference)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			tenant, kind, objectID, label, declaration.Party.String(), declaration.Rule.String(),
		); err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save cancellation authority: %w", err)
		}
	}
	return ports.DeclarationSaved, nil
}

// SaveAcceptanceRulePackage 登记一份规则包版本的正文（open-decisions D-3）：五维
// 适用性照存但不参与选择，规则引用按分类归档。
func (repository *CommercialPublications) SaveAcceptanceRulePackage(
	ctx context.Context,
	pack domain.AcceptanceRulePackage,
) (ports.DeclarationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save acceptance rule package: %w", err)
	}
	tenant, kind, objectID, label := ownerColumns(pack.Version())
	applicability := pack.Applicability()

	incoming := make(map[string]string)
	for _, category := range []domain.RuleCategory{
		domain.MinimumIngressIdentityRules,
		domain.ShipmentInvariantRules,
		domain.ProductAndContractDocumentRules,
		domain.RegulatorySourceDocumentRules,
		domain.CrossFieldConditionRules,
	} {
		for _, rule := range pack.RulesIn(category) {
			incoming[category.String()+"\x1f"+rule.Reference().String()] = ""
		}
	}

	rows, err := executor.Query(ctx,
		`SELECT service_product_id, contract_id, legal_entity_ref, scope_ref,
		        effective_starts_at, effective_ends_at
		   FROM party_commercial.acceptance_rule_package
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant, kind, objectID, label)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save acceptance rule package: %w", err)
	}
	var existingProduct, existingContract, existingLegal, existingScope string
	var existingStarts time.Time
	var existingEnds *time.Time
	present := false
	for rows.Next() {
		if err := rows.Scan(&existingProduct, &existingContract, &existingLegal, &existingScope,
			&existingStarts, &existingEnds); err != nil {
			rows.Close()
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save acceptance rule package: %w", err)
		}
		present = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save acceptance rule package: %w", err)
	}

	incomingEnds, incomingBounded := applicability.Effective().EndsAt()
	if present {
		sameEnds := (existingEnds == nil && !incomingBounded) ||
			(existingEnds != nil && incomingBounded && existingEnds.Equal(incomingEnds))
		if existingProduct != applicability.ServiceProduct().String() ||
			existingContract != applicability.Contract().String() ||
			existingLegal != applicability.LegalEntity().String() ||
			existingScope != applicability.Scope().String() ||
			!existingStarts.Equal(applicability.Effective().StartsAt()) ||
			!sameEnds {
			return ports.DeclarationContentConflict, nil
		}
		// 同分类多条是常态，比对按（分类+引用）逐条集合进行，不得按分类聚键压掉后条。
		existingSet, err := scanTextSet(ctx, executor,
			`SELECT rule_category || '`+"\x1f"+`' || rule_reference
			   FROM party_commercial.acceptance_rule_package_rule
			  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
			tenant, kind, objectID, label)
		if err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save acceptance rule package: %w", err)
		}
		if len(existingSet) != len(incoming) {
			return ports.DeclarationContentConflict, nil
		}
		for key := range incoming {
			if !existingSet[key] {
				return ports.DeclarationContentConflict, nil
			}
		}
		return ports.DeclarationAlreadyRegistered, nil
	}

	var endsColumn *time.Time
	if incomingBounded {
		utc := incomingEnds.UTC()
		endsColumn = &utc
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.acceptance_rule_package
			(tenant_id, object_kind, object_id, version_label,
			 service_product_id, contract_id, legal_entity_ref, scope_ref,
			 effective_starts_at, effective_ends_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		tenant, kind, objectID, label,
		applicability.ServiceProduct().String(),
		applicability.Contract().String(),
		applicability.LegalEntity().String(),
		applicability.Scope().String(),
		applicability.Effective().StartsAt().UTC(),
		endsColumn,
	); err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save acceptance rule package: %w", err)
	}
	for key := range incoming {
		category, reference, _ := strings.Cut(key, "\x1f")
		if _, err := executor.Exec(ctx,
			`INSERT INTO party_commercial.acceptance_rule_package_rule
				(tenant_id, object_kind, object_id, version_label, rule_category, rule_reference)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			tenant, kind, objectID, label, category, reference,
		); err != nil {
			return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save acceptance rule package: %w", err)
		}
	}
	return ports.DeclarationSaved, nil
}

// ---- 行扫描小件：每个都自己收 rows，避免在同一事务连接上带着未收的游标继续写 ----

func parentRowPresent(
	ctx context.Context,
	executor bentopg.Executor,
	sql string,
	args ...any,
) (bool, error) {
	rows, err := executor.Query(ctx, sql, args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	present := rows.Next()
	return present, rows.Err()
}

func scanSingleText(
	ctx context.Context,
	executor bentopg.Executor,
	sql string,
	target *string,
	args ...any,
) (bool, error) {
	rows, err := executor.Query(ctx, sql, args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	present := false
	for rows.Next() {
		if err := rows.Scan(target); err != nil {
			return false, err
		}
		present = true
	}
	return present, rows.Err()
}

func scanTextSet(
	ctx context.Context,
	executor bentopg.Executor,
	sql string,
	args ...any,
) (map[string]bool, error) {
	rows, err := executor.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make(map[string]bool)
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values[value] = true
	}
	return values, rows.Err()
}

func scanTextPairs(
	ctx context.Context,
	executor bentopg.Executor,
	sql string,
	args ...any,
) (map[string]string, error) {
	rows, err := executor.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pairs := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		pairs[key] = value
	}
	return pairs, rows.Err()
}
