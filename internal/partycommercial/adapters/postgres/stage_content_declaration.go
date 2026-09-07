package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// StageContentDeclarations 实现阶段内容三口：按已唯一选出的拥有规则版本取回收寄
// 资格、终局规则与取消授权目录。
//
// 只读。正文属实例半边，本适配器不提供写口，也不在读不到时代拟任何一行——无父行是
// 未配置；有父行而子行空/坏是损坏的声明，走 error。不进 CommercialRegistry /
// ViewRevision：声明改动与选择无关（ADR-0058 / D-4）。
type StageContentDeclarations struct {
	db *bentopg.DB
}

func NewStageContentDeclarations(db *bentopg.DB) (*StageContentDeclarations, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &StageContentDeclarations{db: db}, nil
}

var _ ports.IntakeQualificationView = (*StageContentDeclarations)(nil)
var _ ports.FinalRuleContentView = (*StageContentDeclarations)(nil)
var _ ports.CancellationAuthorityContentView = (*StageContentDeclarations)(nil)

// LoadIntakeQualification 取回收寄资格声明。
//
// found=false = **声明未登记**（无父行）。父行在场即走 NewIntakeQualificationContent
// 重建；零允许来源是缺件，领域拒装，本口交 error 不折成未配置。显式租户与规则包必须
// 同一身份，否则 error 且不交内容。父行与两张子表由一条语句取回，不拆成两次查询。
func (repository *StageContentDeclarations) LoadIntakeQualification(
	ctx context.Context,
	tenant domain.TenantID,
	rulePackage domain.CommercialVersion,
) (domain.IntakeQualificationContent, bool, error) {
	none := domain.IntakeQualificationContent{}
	if err := requireOwnedVersion(tenant, rulePackage, "load intake qualification"); err != nil {
		return none, false, err
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load intake qualification: %w", err)
	}

	var sourcesJSON, qualsJSON []byte
	err = querier.QueryRow(ctx,
		`SELECT COALESCE(sources.payload, '[]'::json),
		        COALESCE(quals.payload, '[]'::json)
		   FROM party_commercial.intake_qualification_content AS parent
		   LEFT JOIN LATERAL (
		       SELECT json_agg(child.source_kind ORDER BY child.source_kind) AS payload
		         FROM party_commercial.intake_allowed_source AS child
		        WHERE child.tenant_id     = parent.tenant_id
		          AND child.object_kind   = parent.object_kind
		          AND child.object_id     = parent.object_id
		          AND child.version_label = parent.version_label
		   ) AS sources ON true
		   LEFT JOIN LATERAL (
		       SELECT json_agg(child.rule_reference ORDER BY child.rule_reference) AS payload
		         FROM party_commercial.intake_qualification_ref AS child
		        WHERE child.tenant_id     = parent.tenant_id
		          AND child.object_kind   = parent.object_kind
		          AND child.object_id     = parent.object_id
		          AND child.version_label = parent.version_label
		   ) AS quals ON true
		  WHERE parent.tenant_id     = $1
		    AND parent.object_kind   = $2
		    AND parent.object_id     = $3
		    AND parent.version_label = $4`,
		tenant.String(),
		uint8(domain.AcceptanceRulePackageObject),
		rulePackage.ObjectID().String(),
		rulePackage.Version().String(),
	).Scan(&sourcesJSON, &qualsJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load intake qualification: %w", err)
	}

	sources, err := declaredIntakeSourcesFromJSON(sourcesJSON)
	if err != nil {
		return none, false, fmt.Errorf("load intake qualification: %w", err)
	}
	qualifications, err := ruleReferencesFromJSON(qualsJSON)
	if err != nil {
		return none, false, fmt.Errorf("load intake qualification: %w", err)
	}
	content, err := domain.NewIntakeQualificationContent(rulePackage, sources, qualifications)
	if err != nil {
		return none, false, fmt.Errorf("load intake qualification: %w", err)
	}
	return content, true, nil
}

// LoadFinalRule 取回终局规则声明。无父行 = 未配置；父行零子行过不了 NewFinalRuleContent，
// 走 error。父 LEFT JOIN 子一次取回。
func (repository *StageContentDeclarations) LoadFinalRule(
	ctx context.Context,
	tenant domain.TenantID,
	rulePackage domain.CommercialVersion,
) (domain.FinalRuleContent, bool, error) {
	none := domain.FinalRuleContent{}
	if err := requireOwnedVersion(tenant, rulePackage, "load final rule"); err != nil {
		return none, false, err
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load final rule: %w", err)
	}

	var declarationsJSON []byte
	var validityAnchor *string
	var validityDuration pgtype.Interval
	err = querier.QueryRow(ctx,
		`SELECT COALESCE(
		            json_agg(
		                json_build_object(
		                    'outcome', child.outcome,
		                    'kind',    child.final_kind
		                )
		                ORDER BY child.outcome
		            ) FILTER (WHERE child.outcome IS NOT NULL),
		            '[]'::json
		        ),
		        parent.validity_anchor,
		        parent.validity_duration
		   FROM party_commercial.final_rule_content AS parent
		   LEFT JOIN party_commercial.final_rule_declaration AS child
		          ON child.tenant_id     = parent.tenant_id
		         AND child.object_kind   = parent.object_kind
		         AND child.object_id     = parent.object_id
		         AND child.version_label = parent.version_label
		  WHERE parent.tenant_id     = $1
		    AND parent.object_kind   = $2
		    AND parent.object_id     = $3
		    AND parent.version_label = $4
		  GROUP BY parent.tenant_id, parent.object_kind, parent.object_id,
		           parent.version_label, parent.validity_anchor, parent.validity_duration`,
		tenant.String(),
		uint8(domain.AcceptanceRulePackageObject),
		rulePackage.ObjectID().String(),
		rulePackage.Version().String(),
	).Scan(&declarationsJSON, &validityAnchor, &validityDuration)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load final rule: %w", err)
	}

	declarations, err := finalizationDeclarationsFromJSON(declarationsJSON)
	if err != nil {
		return none, false, fmt.Errorf("load final rule: %w", err)
	}
	// 有效期那一格按在不在场选构造门（ADR-0119 Decision 三）：两列皆 NULL 走不带有效期的那条，
	// 缺席在这里就与坏声明分开——半缺与集外取值都在 labelValidityFromColumns 报错。
	validity, declared, err := labelValidityFromColumns(validityAnchor, validityDuration)
	if err != nil {
		return none, false, fmt.Errorf("load final rule: %w", err)
	}
	var content domain.FinalRuleContent
	if declared {
		content, err = domain.NewFinalRuleContentWithValidity(rulePackage, declarations, validity)
	} else {
		content, err = domain.NewFinalRuleContent(rulePackage, declarations)
	}
	if err != nil {
		return none, false, fmt.Errorf("load final rule: %w", err)
	}
	return content, true, nil
}

// LoadCancellationAuthority 取回取消授权目录。拥有对象是授权规则。无父行 = 未配置；
// 父行零子行走 error。父 LEFT JOIN 子一次取回。
func (repository *StageContentDeclarations) LoadCancellationAuthority(
	ctx context.Context,
	tenant domain.TenantID,
	authorizationRule domain.CommercialVersion,
) (domain.CancellationAuthorityContent, bool, error) {
	none := domain.CancellationAuthorityContent{}
	if err := requireOwnedVersion(tenant, authorizationRule, "load cancellation authority"); err != nil {
		return none, false, err
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load cancellation authority: %w", err)
	}

	var declarationsJSON []byte
	err = querier.QueryRow(ctx,
		`SELECT COALESCE(
		            json_agg(
		                json_build_object(
		                    'party', child.party,
		                    'rule',  child.rule_reference
		                )
		                ORDER BY child.party
		            ) FILTER (WHERE child.party IS NOT NULL),
		            '[]'::json
		        )
		   FROM party_commercial.cancellation_authority_content AS parent
		   LEFT JOIN party_commercial.cancellation_authority_declaration AS child
		          ON child.tenant_id     = parent.tenant_id
		         AND child.object_kind   = parent.object_kind
		         AND child.object_id     = parent.object_id
		         AND child.version_label = parent.version_label
		  WHERE parent.tenant_id     = $1
		    AND parent.object_kind   = $2
		    AND parent.object_id     = $3
		    AND parent.version_label = $4
		  GROUP BY parent.tenant_id, parent.object_kind, parent.object_id,
		           parent.version_label`,
		tenant.String(),
		uint8(domain.AuthorizationRuleObject),
		authorizationRule.ObjectID().String(),
		authorizationRule.Version().String(),
	).Scan(&declarationsJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load cancellation authority: %w", err)
	}

	declarations, err := cancellationDeclarationsFromJSON(declarationsJSON)
	if err != nil {
		return none, false, fmt.Errorf("load cancellation authority: %w", err)
	}
	content, err := domain.NewCancellationAuthorityContent(authorizationRule, declarations)
	if err != nil {
		return none, false, fmt.Errorf("load cancellation authority: %w", err)
	}
	return content, true, nil
}

func requireOwnedVersion(tenant domain.TenantID, owner domain.CommercialVersion, op string) error {
	if tenant.String() == "" ||
		owner.ObjectID().String() == "" || owner.Version().String() == "" {
		return fmt.Errorf("%s: tenant and owner identity are required", op)
	}
	// 显式租户与拥有对象必须是同一个身份：按租户查库、按对象重建，两处各写各的就会
	// 把 A 的行装进 B 的规则版本（ADR-0003/0040，租户是身份不是过滤器）。
	if tenant != owner.Tenant() {
		return fmt.Errorf("%s: tenant does not own this version", op)
	}
	return nil
}

func declaredIntakeSourcesFromJSON(raw []byte) ([]domain.DeclaredIntakeSource, error) {
	names, err := stringListFromJSON(raw)
	if err != nil {
		return nil, err
	}
	sources := make([]domain.DeclaredIntakeSource, 0, len(names))
	for _, name := range names {
		source, err := declaredIntakeSourceFrom(name)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, nil
}

func ruleReferencesFromJSON(raw []byte) ([]domain.RuleReference, error) {
	names, err := stringListFromJSON(raw)
	if err != nil {
		return nil, err
	}
	refs := make([]domain.RuleReference, 0, len(names))
	for _, name := range names {
		ref, err := domain.NewRuleReference(name)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func stringListFromJSON(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var names []string
	if err := json.Unmarshal(raw, &names); err != nil {
		return nil, fmt.Errorf("string list is not this adapter's shape: %w", err)
	}
	return names, nil
}

type finalizationDocument struct {
	Outcome string `json:"outcome"`
	Kind    string `json:"kind"`
}

func finalizationDeclarationsFromJSON(raw []byte) ([]domain.FinalizationDeclaration, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var documents []finalizationDocument
	if err := json.Unmarshal(raw, &documents); err != nil {
		return nil, fmt.Errorf("finalization rows are not this adapter's shape: %w", err)
	}
	declarations := make([]domain.FinalizationDeclaration, 0, len(documents))
	for _, document := range documents {
		outcome, err := declaredResponsibilityOutcomeFrom(document.Outcome)
		if err != nil {
			return nil, err
		}
		kind, err := domain.NewRuleReference(document.Kind)
		if err != nil {
			return nil, err
		}
		declarations = append(declarations, domain.FinalizationDeclaration{
			Outcome:   outcome,
			FinalKind: kind,
		})
	}
	return declarations, nil
}

type cancellationDocument struct {
	Party string `json:"party"`
	Rule  string `json:"rule"`
}

func cancellationDeclarationsFromJSON(raw []byte) ([]domain.CancellationAuthorityDeclaration, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var documents []cancellationDocument
	if err := json.Unmarshal(raw, &documents); err != nil {
		return nil, fmt.Errorf("cancellation rows are not this adapter's shape: %w", err)
	}
	declarations := make([]domain.CancellationAuthorityDeclaration, 0, len(documents))
	for _, document := range documents {
		party, err := declaredCancellationPartyFrom(document.Party)
		if err != nil {
			return nil, err
		}
		rule, err := domain.NewRuleReference(document.Rule)
		if err != nil {
			return nil, err
		}
		declarations = append(declarations, domain.CancellationAuthorityDeclaration{
			Party: party,
			Rule:  rule,
		})
	}
	return declarations, nil
}

func declaredIntakeSourceFrom(raw string) (domain.DeclaredIntakeSource, error) {
	for _, source := range []domain.DeclaredIntakeSource{
		domain.DeclaredNodeIntake,
		domain.DeclaredOffsitePickup,
	} {
		if source.String() == raw {
			return source, nil
		}
	}
	return domain.DeclaredIntakeSourceInvalid, fmt.Errorf("unknown intake source %q", raw)
}

func declaredResponsibilityOutcomeFrom(raw string) (domain.DeclaredResponsibilityOutcome, error) {
	for _, outcome := range []domain.DeclaredResponsibilityOutcome{
		domain.DeclaredEffectiveDelivery,
		domain.DeclaredReturnCompleted,
		domain.DeclaredServiceTerminated,
		domain.DeclaredRegulatoryDisposition,
	} {
		if outcome.String() == raw {
			return outcome, nil
		}
	}
	return domain.DeclaredResponsibilityOutcomeInvalid, fmt.Errorf("unknown responsibility outcome %q", raw)
}

func declaredCancellationPartyFrom(raw string) (domain.DeclaredCancellationParty, error) {
	for _, party := range []domain.DeclaredCancellationParty{
		domain.DeclaredCustomerCancellation,
		domain.DeclaredOperationsCancellation,
	} {
		if party.String() == raw {
			return party, nil
		}
	}
	return domain.DeclaredCancellationPartyInvalid, fmt.Errorf("unknown cancellation party %q", raw)
}
