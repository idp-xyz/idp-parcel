package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// catalogFamilyExceptionDisclosureRules 是异常披露规则目录在写入侧咨询锁里的族号，
// 与 catalog_registration.go 里那三族同一把锁的不同键。
const catalogFamilyExceptionDisclosureRules int32 = 4

// ExceptionDisclosureRules 实现 ports.ExceptionDisclosureRuleView：按当前适用的异常
// 披露规则回答「对这个客户与这类信号，披露与否、能否自动发布、内容从哪来」。
//
// 目录内容属实例半边（`PAR-VIS-07` 待提供），实现不属：空目录时如实交回「未配置」，
// 由编排停在未决——没有规则时既不能说披露也不能说不披露。依赖调不通才作为 error
// 返回。租户在装配期固定，理由同本包其余只读视图：RuleForSignal 的签名里没有租户，
// 而客户账户引用只在租户内唯一（ADR-0003）。
type ExceptionDisclosureRules struct {
	db     *bentopg.DB
	tenant domain.TenantID
}

func NewExceptionDisclosureRules(db *bentopg.DB, tenant domain.TenantID) (*ExceptionDisclosureRules, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &ExceptionDisclosureRules{db: db, tenant: tenant}, nil
}

var _ ports.ExceptionDisclosureRuleView = (*ExceptionDisclosureRules)(nil)

// RuleForSignal 取当前适用版本下（客户 + 类型 + 可信度）那一条。
//
// 适用版本取当前仍然有效的那一份（同分诊规则）：披露决定判的是手上这个活信号，库上的
// 部分唯一索引担保未闭区间至多一份。目录已发布但这一键没有条目仍交回未配置——与 0012
// 披露策略同一条理由：没有默认披露值可发明，「还没写到这个账户」不能读成「这个账户
// 什么都不披露」。规则引用取版本号：决定带着它走，通知策略按它取渠道与时限。
func (view *ExceptionDisclosureRules) RuleForSignal(
	ctx context.Context,
	customer domain.CustomerAccountReference,
	kind domain.ExceptionSignalKindReference,
	confidence domain.ConfidenceReference,
) (ports.ExceptionDisclosureRule, bool, error) {
	if view.tenant.String() == "" ||
		customer.String() == "" ||
		kind.String() == "" ||
		confidence.String() == "" {
		return ports.ExceptionDisclosureRule{}, false, nil
	}

	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ExceptionDisclosureRule{}, false, fmt.Errorf("exception disclosure rule: %w", err)
	}

	var ruleVersion string
	err = querier.QueryRow(ctx,
		`SELECT rule_version
		   FROM visibility_exception.exception_disclosure_rule_version
		  WHERE tenant_id = $1
		    AND effective_to IS NULL
		    AND effective_from <= now()`,
		view.tenant.String(),
	).Scan(&ruleVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ExceptionDisclosureRule{}, false, nil
	}
	if err != nil {
		return ports.ExceptionDisclosureRule{}, false, fmt.Errorf("exception disclosure rule: %w", err)
	}

	var (
		disclosable, autoRelease bool
		contentRaw               *string
	)
	err = querier.QueryRow(ctx,
		`SELECT disclosable, auto_release, content_ref
		   FROM visibility_exception.exception_disclosure_rule_entry
		  WHERE tenant_id = $1
		    AND rule_version = $2
		    AND customer_account_ref = $3
		    AND signal_kind = $4
		    AND confidence_ref = $5`,
		view.tenant.String(), ruleVersion, customer.String(), kind.String(), confidence.String(),
	).Scan(&disclosable, &autoRelease, &contentRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ExceptionDisclosureRule{}, false, nil
	}
	if err != nil {
		return ports.ExceptionDisclosureRule{}, false, fmt.Errorf("exception disclosure rule: %w", err)
	}

	rule := ports.ExceptionDisclosureRule{Disclosable: disclosable, AutoRelease: autoRelease}
	if rule.Policy, err = domain.NewDisclosurePolicyReference(ruleVersion); err != nil {
		return ports.ExceptionDisclosureRule{}, false, fmt.Errorf("exception disclosure rule: %w", err)
	}
	// 内容与披露成对、自动发布不越过披露——库上两条 CHECK 守着，这里再过一遍，挡的是
	// CHECK 被后续迁移放宽而 Go 侧没跟上；那时上抛，不把一条自相矛盾的规则答给编排。
	if disclosable != (contentRaw != nil) || (!disclosable && autoRelease) {
		return ports.ExceptionDisclosureRule{}, false, fmt.Errorf(
			"exception disclosure rule: 条目 %s/%s/%s 的披露、内容与自动发布不成对", customer, kind, confidence)
	}
	if contentRaw != nil {
		if rule.Content, err = domain.NewDisclosureContentReference(*contentRaw); err != nil {
			return ports.ExceptionDisclosureRule{}, false, fmt.Errorf("exception disclosure rule: %w", err)
		}
	}
	return rule, true, nil
}

var _ ports.ExceptionDisclosureRuleRegistry = (*CatalogRegistrar)(nil)

// RegisterExceptionDisclosureRules 登记一版异常披露规则（`PAR-VIS-07` 的披露与自动发布
// 范围半边）。准入、接续闭合与整版同一提交的纪律全部沿用 admitVersion——它是第四份
// 区间型目录，形状与前三份逐字相同。条目的成对纪律（内容随披露、自动发布不越过披露）
// 交给 0023 的两条 CHECK：不成对就整版不落。
func (registrar *CatalogRegistrar) RegisterExceptionDisclosureRules(
	ctx context.Context,
	tenant domain.TenantID,
	registration ports.ExceptionDisclosureRuleRegistration,
) (ports.CatalogRegistrationOutcome, error) {
	executor, err := registrar.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("register exception disclosure rules: %w", err)
	}
	outcome, err := registrar.admitVersion(ctx, executor, versionAdmission{
		family:     catalogFamilyExceptionDisclosureRules,
		table:      "visibility_exception.exception_disclosure_rule_version",
		versionCol: "rule_version",
		tenant:     tenant,
		header:     registration.Header,
	})
	if outcome != ports.CatalogVersionRegistered || err != nil {
		return outcome, wrapRegister("register exception disclosure rules", err)
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.exception_disclosure_rule_version
			(tenant_id, rule_version, effective_from, effective_to, approved_by)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenant.String(), registration.Header.Version,
		registration.Header.EffectiveFrom.UTC(), effectiveTo(registration.Header),
		registration.Header.ApprovedBy,
	); err != nil {
		return ports.CatalogRegistrationOutcomeInvalid, fmt.Errorf("register exception disclosure rules: %w", err)
	}
	for _, entry := range registration.Entries {
		// 内容用指针而不是空串：0023 的成对约束正是靠 NULL 分辨「这一条不带内容」与
		// 「登记了一个空内容引用」。
		if _, err := executor.Exec(ctx,
			`INSERT INTO visibility_exception.exception_disclosure_rule_entry
				(tenant_id, rule_version, customer_account_ref, signal_kind, confidence_ref,
				 disclosable, auto_release, content_ref)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			tenant.String(), registration.Header.Version,
			entry.Customer.String(), entry.Kind.String(), entry.Confidence.String(),
			entry.Disclosable, entry.AutoRelease, nullIfBlank(entry.Content.String()),
		); err != nil {
			return ports.CatalogRegistrationOutcomeInvalid,
				fmt.Errorf("register exception disclosure rules: 条目 %s/%s/%s：%w",
					entry.Customer, entry.Kind, entry.Confidence, err)
		}
	}
	return ports.CatalogVersionRegistered, nil
}
