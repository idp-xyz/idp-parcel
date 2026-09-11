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

// CredentialGateRegistrations 实现 ports.CredentialGateRegistry：凭证门禁判断登记册的写口（0018 建表，
// 票 sa-cc/04）。
//
// 写入代数与其余登记册同款且更简——一律不 UPSERT，同键（三维 + 内容指纹）由 DO NOTHING 折成`已登记`
// 交回；**没有任何 UPDATE 路径**：判断是不可覆盖的版本，换内容换指纹另起一行。来源变化让既有就绪
// 判断失效那件事在 readiness_judgment 的 revoked_* 两列上，不在本册。
type CredentialGateRegistrations struct {
	db *bentopg.DB
}

func NewCredentialGateRegistrations(db *bentopg.DB) (*CredentialGateRegistrations, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &CredentialGateRegistrations{db: db}, nil
}

var _ ports.CredentialGateRegistry = (*CredentialGateRegistrations)(nil)

// RegisterCredentialGate 登记一版判断。键与判断对象说的必须是同一件事——三维不符或指纹不是这条
// 判断的指纹，库里就会有一行按 A 查、内容却是 B 的判断，这是调用方编程错误，响亮拒（判据同
// SaveVerification）。
func (registry *CredentialGateRegistrations) RegisterCredentialGate(
	ctx context.Context,
	record ports.CredentialGateRecord,
) (ports.CaseConfigurationSaveOutcome, error) {
	judgment := record.Judgment
	if strings.TrimSpace(judgment.Credential().String()) == "" {
		// 零值判断的各维皆空，落库会撞 CHECK 而报成「依赖故障」，它明明是调用方编程错误。
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register credential gate: the judgment is zero-valued")
	}
	if record.Key.Unit != judgment.Unit() || record.Key.Credential != judgment.Credential() ||
		record.Key.Digest != ports.CredentialGateDigest(judgment) {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register credential gate: key disagrees with the judgment it claims to index")
	}
	conclusion := judgment.Conclusion().String()
	if conclusion == "" {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register credential gate: the conclusion is outside its closed set")
	}
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register credential gate: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.credential_gate_judgment
			(tenant_id, unit_id, credential_id, version_digest,
			 procedure_ref, holder_ref, as_of, conclusion, basis_ref, role_ref, judged_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(), record.Key.Unit.String(), record.Key.Credential.String(), record.Key.Digest,
		judgment.Procedure().String(), judgment.Holder().String(), judgment.AsOf().UTC(),
		conclusion, judgment.Basis().String(), judgment.Role().String(), judgment.JudgedAt().UTC(),
	)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register credential gate: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	return ports.CaseConfigurationRegistered, nil
}

// CredentialGateView 实现 ports.CredentialGateView：按幂等键取回一版判断。读回经领域构造重建——
// 库里一行若立不起 CredentialGateJudgment，说明有人绕过写口改了它，作错误抛出而不是交回一个
// 半成品对象。
type CredentialGateView struct {
	db *bentopg.DB
}

func NewCredentialGateView(db *bentopg.DB) (*CredentialGateView, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &CredentialGateView{db: db}, nil
}

var _ ports.CredentialGateView = (*CredentialGateView)(nil)

func (view *CredentialGateView) LoadCredentialGate(
	ctx context.Context,
	key ports.CredentialGateKey,
) (ports.CredentialGateRecord, bool, error) {
	none := ports.CredentialGateRecord{}
	if strings.TrimSpace(key.Digest) == "" {
		// 空键是调用方编程错误，与「那一版还没登记」是两回事（判据同凭证读口）。
		return none, false, fmt.Errorf("load credential gate: the version digest is blank")
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load credential gate: %w", err)
	}

	var (
		procedure, holder, conclusion, basis, role string
		asOf, judgedAt                             time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT procedure_ref, holder_ref, as_of, conclusion, basis_ref, role_ref, judged_at
		   FROM customs_compliance.credential_gate_judgment
		  WHERE tenant_id = $1 AND unit_id = $2 AND credential_id = $3 AND version_digest = $4`,
		key.TenantID.String(), key.Unit.String(), key.Credential.String(), key.Digest,
	).Scan(&procedure, &holder, &asOf, &conclusion, &basis, &role, &judgedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load credential gate: %w", err)
	}

	judgment, err := rebuildCredentialGate(key, procedure, holder, asOf, conclusion, basis, role, judgedAt)
	if err != nil {
		return none, false, fmt.Errorf("rebuild credential gate: %w", err)
	}
	return ports.CredentialGateRecord{Key: key, Judgment: judgment}, true, nil
}

func rebuildCredentialGate(
	key ports.CredentialGateKey,
	procedure, holder string,
	asOf time.Time,
	conclusion, basis, role string,
	judgedAt time.Time,
) (domain.CredentialGateJudgment, error) {
	spec := domain.CredentialGateSpec{
		Unit:       key.Unit,
		Credential: key.Credential,
		AsOf:       asOf.UTC(),
		JudgedAt:   judgedAt.UTC(),
	}
	var err error
	if spec.Procedure, err = domain.NewCustomsProcedureReference(procedure); err != nil {
		return domain.CredentialGateJudgment{}, err
	}
	if spec.Holder, err = domain.NewCredentialHolderReference(holder); err != nil {
		return domain.CredentialGateJudgment{}, err
	}
	if spec.Conclusion, err = closedWord(conclusion,
		domain.CredentialGateApplicable, domain.CredentialGateNotApplicable,
		domain.CredentialGateCredentialNotRegistered, domain.CredentialGateUndecided); err != nil {
		return domain.CredentialGateJudgment{}, err
	}
	if spec.Basis, err = domain.NewCredentialGateBasisReference(basis); err != nil {
		return domain.CredentialGateJudgment{}, err
	}
	if spec.Role, err = domain.NewResponsibleRoleReference(role); err != nil {
		return domain.CredentialGateJudgment{}, err
	}
	return domain.RecordCredentialGate(spec)
}
