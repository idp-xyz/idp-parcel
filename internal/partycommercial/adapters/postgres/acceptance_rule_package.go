package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// AcceptanceRulePackages 实现 ports.AcceptanceRulePackageContentView：按已唯一选出的
// 接单规则包版本取回正文。
//
// 只读。正文属实例半边，本适配器不提供写口，也不在读不到时代拟任何规则——无父行
// 是未配置，有父行零子行是坏数据（空包等于无条件接受）。不进 CommercialRegistry /
// ViewRevision：五维照存但不参与选择（ADR-0059）。
type AcceptanceRulePackages struct {
	db *bentopg.DB
}

func NewAcceptanceRulePackages(db *bentopg.DB) (*AcceptanceRulePackages, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &AcceptanceRulePackages{db: db}, nil
}

var _ ports.AcceptanceRulePackageContentView = (*AcceptanceRulePackages)(nil)

// LoadAcceptanceRulePackage 取回规则包正文。
//
// found=false = **正文未登记**（无父行）。父行在场即走 NewAcceptanceRulePackage 重建，
// 零子行被领域拒绝，本口把那次失败上抛，不折成未配置。显式租户与规则包对象必须同一
// 身份，否则 error 且不交内容。父行与规则由一条左连接取回，不拆成两次查询。
func (repository *AcceptanceRulePackages) LoadAcceptanceRulePackage(
	ctx context.Context,
	tenant domain.TenantID,
	rulePackage domain.CommercialVersion,
) (domain.AcceptanceRulePackage, bool, error) {
	none := domain.AcceptanceRulePackage{}
	if tenant.String() == "" ||
		rulePackage.ObjectID().String() == "" || rulePackage.Version().String() == "" {
		return none, false, fmt.Errorf("load acceptance rule package: tenant and rule package identity are required")
	}
	// 显式租户与规则包对象必须是同一个身份：按租户查库、按规则包重建，两处各写各的就会
	// 把 A 的行装进 B 的规则包（ADR-0003/0040，租户是身份不是过滤器）。
	if tenant != rulePackage.Tenant() {
		return none, false, fmt.Errorf("load acceptance rule package: tenant does not own this rule package")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load acceptance rule package: %w", err)
	}

	// 父 LEFT JOIN 子一次取回：ReadExecutor 不保证两条语句同一快照，分两次会拼出从未
	// 同时存在的父子状态。无父行 = 零行（found=false）；有父零子 = 一行空 json 数组。
	var productID, contractID, legalEntityRef, scopeRef string
	var startsAt time.Time
	var endsAt *time.Time
	var rulesJSON []byte
	err = querier.QueryRow(ctx,
		`SELECT parent.service_product_id,
		        parent.contract_id,
		        parent.legal_entity_ref,
		        parent.scope_ref,
		        parent.effective_starts_at,
		        parent.effective_ends_at,
		        COALESCE(
		            json_agg(
		                json_build_object(
		                    'category',  child.rule_category,
		                    'reference', child.rule_reference
		                )
		                ORDER BY child.rule_category, child.rule_reference
		            ) FILTER (WHERE child.rule_reference IS NOT NULL),
		            '[]'::json
		        )
		   FROM party_commercial.acceptance_rule_package AS parent
		   LEFT JOIN party_commercial.acceptance_rule_package_rule AS child
		          ON child.tenant_id     = parent.tenant_id
		         AND child.object_kind   = parent.object_kind
		         AND child.object_id     = parent.object_id
		         AND child.version_label = parent.version_label
		  WHERE parent.tenant_id     = $1
		    AND parent.object_kind   = $2
		    AND parent.object_id     = $3
		    AND parent.version_label = $4
		  GROUP BY parent.tenant_id, parent.object_kind, parent.object_id,
		           parent.version_label, parent.service_product_id, parent.contract_id,
		           parent.legal_entity_ref, parent.scope_ref, parent.effective_starts_at,
		           parent.effective_ends_at`,
		tenant.String(),
		uint8(domain.AcceptanceRulePackageObject),
		rulePackage.ObjectID().String(),
		rulePackage.Version().String(),
	).Scan(&productID, &contractID, &legalEntityRef, &scopeRef, &startsAt, &endsAt, &rulesJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load acceptance rule package: %w", err)
	}

	applicability, err := rulePackageApplicabilityFrom(productID, contractID, legalEntityRef, scopeRef, startsAt, endsAt)
	if err != nil {
		return none, false, fmt.Errorf("load acceptance rule package: %w", err)
	}
	rules, err := assembledRulesFromJSON(rulesJSON)
	if err != nil {
		return none, false, fmt.Errorf("load acceptance rule package: %w", err)
	}

	content, err := domain.NewAcceptanceRulePackage(rulePackage, applicability, rules)
	if err != nil {
		return none, false, fmt.Errorf("load acceptance rule package: %w", err)
	}
	return content, true, nil
}

func rulePackageApplicabilityFrom(
	productID, contractID, legalEntityRef, scopeRef string,
	startsAt time.Time,
	endsAt *time.Time,
) (domain.RulePackageApplicability, error) {
	product, err := domain.NewCommercialObjectID(productID)
	if err != nil {
		return domain.RulePackageApplicability{}, err
	}
	contract, err := domain.NewCommercialObjectID(contractID)
	if err != nil {
		return domain.RulePackageApplicability{}, err
	}
	legalEntity, err := domain.NewLegalEntityReference(legalEntityRef)
	if err != nil {
		return domain.RulePackageApplicability{}, err
	}
	scope, err := domain.NewCommercialScopeReference(scopeRef)
	if err != nil {
		return domain.RulePackageApplicability{}, err
	}
	end := time.Time{}
	if endsAt != nil {
		end = *endsAt
	}
	interval, err := domain.NewEffectiveInterval(startsAt, end)
	if err != nil {
		return domain.RulePackageApplicability{}, err
	}
	return domain.NewRulePackageApplicability(product, contract, legalEntity, scope, interval)
}

type assembledRuleDocument struct {
	Category  string `json:"category"`
	Reference string `json:"reference"`
}

func assembledRulesFromJSON(raw []byte) ([]domain.AssembledRule, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var documents []assembledRuleDocument
	if err := json.Unmarshal(raw, &documents); err != nil {
		return nil, fmt.Errorf("assembled rules are not this adapter's shape: %w", err)
	}
	rules := make([]domain.AssembledRule, 0, len(documents))
	for _, document := range documents {
		category, err := ruleCategoryFrom(document.Category)
		if err != nil {
			return nil, err
		}
		reference, err := domain.NewRuleReference(document.Reference)
		if err != nil {
			return nil, err
		}
		rule, err := domain.NewAssembledRule(category, reference)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func ruleCategoryFrom(raw string) (domain.RuleCategory, error) {
	switch raw {
	case domain.MinimumIngressIdentityRules.String():
		return domain.MinimumIngressIdentityRules, nil
	case domain.ShipmentInvariantRules.String():
		return domain.ShipmentInvariantRules, nil
	case domain.ProductAndContractDocumentRules.String():
		return domain.ProductAndContractDocumentRules, nil
	case domain.RegulatorySourceDocumentRules.String():
		return domain.RegulatorySourceDocumentRules, nil
	case domain.CrossFieldConditionRules.String():
		return domain.CrossFieldConditionRules, nil
	default:
		return domain.RuleCategoryInvalid, fmt.Errorf("unknown rule category %q", raw)
	}
}
