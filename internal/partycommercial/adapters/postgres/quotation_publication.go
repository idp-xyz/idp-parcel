package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// QuotationPublication 读发布门要的发出、答复、租户规则，以及合同是否已经指名计价范围。
type QuotationPublication struct {
	db *bentopg.DB
}

func NewQuotationPublication(db *bentopg.DB) (*QuotationPublication, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &QuotationPublication{db: db}, nil
}

var _ ports.QuotationPublicationView = (*QuotationPublication)(nil)

func (view *QuotationPublication) LoadQuotationPublicationRequirement(ctx context.Context, tenant domain.TenantID) (bool, bool, error) {
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return false, false, err
	}
	var requires bool
	err = querier.QueryRow(ctx,
		`SELECT requires FROM party_commercial.quotation_publication_requirement WHERE tenant_id = $1`,
		tenant.String(),
	).Scan(&requires)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("load quotation publication requirement: %w", err)
	}
	return requires, true, nil
}

func (view *QuotationPublication) SaveQuotationPublicationRequirement(ctx context.Context, requirement domain.QuotationPublicationRequirement) (bool, bool, error) {
	executor, err := view.db.RequireExecutor(ctx)
	if err != nil {
		return false, false, err
	}
	// 撞键不报错再读回，与签署证据要求同一写法。唯一约束冲突若让语句失败，事务就中止，后面的比对读不出来。
	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.quotation_publication_requirement (tenant_id, requires) VALUES ($1, $2)
		 ON CONFLICT DO NOTHING`,
		requirement.Tenant().String(), requirement.Requires(),
	)
	if err != nil {
		return false, false, fmt.Errorf("save quotation publication requirement: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return false, true, nil
	}
	var existingRequires bool
	err = executor.QueryRow(ctx,
		`SELECT requires FROM party_commercial.quotation_publication_requirement WHERE tenant_id = $1`,
		requirement.Tenant().String(),
	).Scan(&existingRequires)
	if err != nil {
		return false, false, fmt.Errorf("save quotation publication requirement: %w", err)
	}
	if existingRequires == requirement.Requires() {
		return true, false, nil
	}
	return false, false, nil
}

func (view *QuotationPublication) ListQuotationSendsForPlan(ctx context.Context, tenant domain.TenantID, planID, planVersion string) ([]ports.QuotationSendRef, error) {
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT quotation_id, content_digest
		   FROM party_commercial.quotation_send
		  WHERE tenant_id = $1 AND plan_id = $2 AND plan_version = $3`,
		tenant.String(), planID, planVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("list quotation sends for plan: %w", err)
	}
	defer rows.Close()
	var sends []ports.QuotationSendRef
	for rows.Next() {
		var ref ports.QuotationSendRef
		if err := rows.Scan(&ref.ID, &ref.Digest); err != nil {
			return nil, err
		}
		sends = append(sends, ref)
	}
	return sends, rows.Err()
}

func (view *QuotationPublication) AcceptedQuotationReply(ctx context.Context, tenant domain.TenantID, quotationID string) (string, bool, bool, error) {
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return "", false, false, err
	}
	var digest, decision string
	err = querier.QueryRow(ctx,
		`SELECT send.content_digest, reply.decision
		   FROM party_commercial.quotation_send send
		   JOIN party_commercial.quotation_reply reply
		     ON reply.tenant_id = send.tenant_id AND reply.quotation_id = send.quotation_id
		  WHERE send.tenant_id = $1 AND send.quotation_id = $2`,
		tenant.String(), quotationID,
	).Scan(&digest, &decision)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, false, nil
	}
	if err != nil {
		return "", false, false, fmt.Errorf("accepted quotation reply: %w", err)
	}
	return digest, decision == domain.QuotationReplyAccepted.String(), true, nil
}

// ContractNamesPricingScope 按解析选用的同一判法回答：折进退役之后状态仍是已生效，且选用区间含 at。
// 状态列随发布写一次，退役与截止另册；只看状态列会把已经截止、退役或走出区间的版本仍当成此刻适用。
// 发布时边界未开的版本停在已发布，解析也不选它（applicable 要求已生效），这里同样不选。
// 账户是否已停用不在合同解析里，而在身份生命周期：这一口从相对方名下的账户找合同，最新修订已停用的账户不代表这位客户此刻还用着那份合同。
func (view *QuotationPublication) ContractNamesPricingScope(ctx context.Context, tenant domain.TenantID, partyID, scope string, at time.Time) (bool, error) {
	if at.IsZero() || tenant.String() == "" || partyID == "" || scope == "" {
		return false, fmt.Errorf("contract names pricing scope: tenant, party, scope and at are required")
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return false, err
	}
	rows, err := querier.Query(ctx,
		`SELECT version.snapshot, account.deactivated_at, `+versionClosureColumns+`
		   FROM party_commercial.customer_account_registration account
		   JOIN party_commercial.customer_contract_content contract
		     ON contract.tenant_id = account.tenant_id
		    AND contract.customer_account_id = account.account_id
		   JOIN party_commercial.commercial_version version
		     ON version.tenant_id = contract.tenant_id
		    AND version.object_kind = contract.object_kind
		    AND version.object_id = contract.object_id
		    AND version.version_label = contract.version_label
		   JOIN party_commercial.customer_contract_sell_price_source source
		     ON source.tenant_id = contract.tenant_id
		    AND source.object_kind = contract.object_kind
		    AND source.object_id = contract.object_id
		    AND source.version_label = contract.version_label
		    AND source.pricing_scope_ref = $3
		   `+versionClosureJoins("version")+`
		  WHERE account.tenant_id = $1
		    AND account.customer_party_id = $2
		    AND account.revision = (
		         SELECT max(latest.revision)
		           FROM party_commercial.customer_account_registration latest
		          WHERE latest.tenant_id = account.tenant_id
		            AND latest.account_id = account.account_id)`,
		tenant.String(), partyID, scope,
	)
	if err != nil {
		return false, fmt.Errorf("contract names pricing scope: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var deactivatedAt *time.Time
		var closure scannedVersionClosure
		if err := rows.Scan(append([]any{&raw, &deactivatedAt}, closure.targets()...)...); err != nil {
			return false, fmt.Errorf("contract names pricing scope: %w", err)
		}
		if deactivatedAt != nil && !at.Before(*deactivatedAt) {
			continue
		}
		loaded, selection, err := foldCitedVersion(raw, closure)
		if err != nil {
			return false, fmt.Errorf("contract names pricing scope: %w", err)
		}
		if loaded.Status() == domain.CommercialVersionEffective && selection.Contains(at) {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("contract names pricing scope: %w", err)
	}
	return false, nil
}
