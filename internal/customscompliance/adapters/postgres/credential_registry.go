package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// CredentialRegistrations 实现 ports.CredentialRegistry：监管凭证登记册的写口（0014 建表，
// 票 mechanism-executor-triage/07 CC-a）。
//
// 写入代数与其余登记册同款且更简——一律不 UPSERT，同键（租户，凭证身份）由 DO NOTHING 折成
// `已登记`交回，内容是否同一份由编排读回自己比；**没有任何 UPDATE 路径**：凭证是不可变版本，
// 期限、持有人、额度都不是可推进的状态，换任何一件是另一张凭证。额度占用/释放/核销那条
// 生命周期不在本册。
type CredentialRegistrations struct {
	db *bentopg.DB
}

func NewCredentialRegistrations(db *bentopg.DB) (*CredentialRegistrations, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &CredentialRegistrations{db: db}, nil
}

var _ ports.CredentialRegistry = (*CredentialRegistrations)(nil)

// RegisterCredential 登记一版凭证。registered_at 取事务内的库时钟：它是登记这一动作的
// 时间，不是凭证的任何业务时间——那两个在行内另有自己的列，不混用。
func (registry *CredentialRegistrations) RegisterCredential(
	ctx context.Context,
	tenant domain.TenantID,
	credential domain.RegulatoryCredential,
) (ports.CaseConfigurationSaveOutcome, error) {
	if strings.TrimSpace(credential.ID().String()) == "" {
		// 零值凭证的各维皆空，落库会撞 CHECK 而报成「依赖故障」，它明明是调用方编程错误。
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register credential: the credential is zero-valued")
	}
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register credential: %w", err)
	}

	uses, _ := credential.Uses()
	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.regulatory_credential
			(tenant_id, credential_id, issuer_ref, holder_ref, procedure_ref,
			 valid_from, valid_to, uses, registered_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		 ON CONFLICT DO NOTHING`,
		tenant.String(), credential.ID().String(),
		credential.Issuer().String(), credential.Holder().String(), credential.Procedure().String(),
		credential.ValidFrom().UTC(), credential.ValidTo().UTC(), uses,
	)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register credential: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	return ports.CaseConfigurationRegistered, nil
}

// CredentialView 实现 ports.CredentialView：按（租户，凭证身份）取回在册版本。读回经领域
// 构造重建——库里一行若立不起 RegulatoryCredential，说明有人绕过写口改了它，作错误抛出
// 而不是交回一个半成品对象。
type CredentialView struct {
	db *bentopg.DB
}

func NewCredentialView(db *bentopg.DB) (*CredentialView, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &CredentialView{db: db}, nil
}

var _ ports.CredentialView = (*CredentialView)(nil)

func (view *CredentialView) LoadCredential(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.CredentialID,
) (domain.RegulatoryCredential, bool, error) {
	none := domain.RegulatoryCredential{}
	if strings.TrimSpace(id.String()) == "" {
		// 空键是调用方编程错误，与「凭证还没登记」是两回事（判据同口岸读口）。
		return none, false, fmt.Errorf("load credential: the credential ID is blank")
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load credential: %w", err)
	}

	var (
		issuer, holder, procedure string
		validFrom, validTo        time.Time
		uses                      int
	)
	err = querier.QueryRow(ctx,
		`SELECT issuer_ref, holder_ref, procedure_ref, valid_from, valid_to, uses
		   FROM customs_compliance.regulatory_credential
		  WHERE tenant_id = $1 AND credential_id = $2`,
		tenant.String(), id.String(),
	).Scan(&issuer, &holder, &procedure, &validFrom, &validTo, &uses)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load credential: %w", err)
	}

	credential, err := rebuildCredential(id, issuer, holder, procedure, validFrom, validTo, uses)
	if err != nil {
		return none, false, fmt.Errorf("rebuild credential: %w", err)
	}
	return credential, true, nil
}

func rebuildCredential(
	id domain.CredentialID,
	issuer, holder, procedure string,
	validFrom, validTo time.Time,
	uses int,
) (domain.RegulatoryCredential, error) {
	issuerRef, err := domain.NewRegulatoryAuthorityReference(issuer)
	if err != nil {
		return domain.RegulatoryCredential{}, err
	}
	holderRef, err := domain.NewCredentialHolderReference(holder)
	if err != nil {
		return domain.RegulatoryCredential{}, err
	}
	procedureRef, err := domain.NewCustomsProcedureReference(procedure)
	if err != nil {
		return domain.RegulatoryCredential{}, err
	}
	return domain.RegisterCredential(id, issuerRef, holderRef, procedureRef, validFrom.UTC(), validTo.UTC(), uses)
}
