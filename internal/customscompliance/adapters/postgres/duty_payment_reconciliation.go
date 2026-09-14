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

// DutyPaymentReconciliation 一并实现 ports.DutyCollaborationStore、ports.ExternalFundsFactRegister
// 与 ports.DutyVerificationStore（0016 建三表，票 mechanism-executor-triage/07 CC-c）。三口
// 落在一个类型上只是装配省事——三张表各自的键与写入代数互不相干，编排也按三个端口分别
// 依赖它，不因此共享任何一行。
//
// 写入代数与其余登记册同款：一律不 UPSERT、无 UPDATE 路径——协作事项与核对都是追加的版本
// （税费更正换税费引用、迟到事实换指纹，各自另起一行），资金事实引用是来源事实的登记不是
// 余额。同键由 DO NOTHING 折成`已登记`交回，内容是否同一份由编排读回自己比。
type DutyPaymentReconciliation struct {
	db *bentopg.DB
}

func NewDutyPaymentReconciliation(db *bentopg.DB) (*DutyPaymentReconciliation, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &DutyPaymentReconciliation{db: db}, nil
}

var (
	_ ports.DutyCollaborationStore    = (*DutyPaymentReconciliation)(nil)
	_ ports.ExternalFundsFactRegister = (*DutyPaymentReconciliation)(nil)
	_ ports.DutyVerificationStore     = (*DutyPaymentReconciliation)(nil)
)

// FindCollaboration 按（租户，范围，税费引用）取回协作事项；无需付款格以空税费引用查。
func (store *DutyPaymentReconciliation) FindCollaboration(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.DecisionScopeReference,
	duty domain.AssessedDutyReference,
) (domain.DutyPaymentCollaboration, bool, error) {
	none := domain.DutyPaymentCollaboration{}
	if strings.TrimSpace(scope.String()) == "" {
		return none, false, fmt.Errorf("find duty collaboration: the scope is blank")
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("find duty collaboration: %w", err)
	}

	var (
		kind, noPayBasis, obligor, requirement, target string
		formedAt                                       time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT kind, no_pay_basis, obligor_ref, requirement_ref, target_ref, formed_at
		   FROM customs_compliance.duty_payment_collaboration
		  WHERE tenant_id = $1 AND scope_ref = $2 AND duty_ref = $3`,
		tenant.String(), scope.String(), duty.String(),
	).Scan(&kind, &noPayBasis, &obligor, &requirement, &target, &formedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("find duty collaboration: %w", err)
	}

	collaboration, err := rebuildCollaboration(kind, duty, noPayBasis, scope, obligor, requirement, target, formedAt)
	if err != nil {
		return none, false, fmt.Errorf("rebuild duty collaboration: %w", err)
	}
	return collaboration, true, nil
}

// SaveCollaboration 登记一份协作事项。无需付款格的税费引用为空串入键——两格在同一范围上
// 各占一行（0016 自注）。
func (store *DutyPaymentReconciliation) SaveCollaboration(
	ctx context.Context,
	tenant domain.TenantID,
	collaboration domain.DutyPaymentCollaboration,
) (ports.CaseConfigurationSaveOutcome, error) {
	kind := collaboration.Kind().String()
	if kind == "" {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("save duty collaboration: unknown obligation kind %d", collaboration.Kind())
	}
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("save duty collaboration: %w", err)
	}

	duty, _ := collaboration.Duty()
	noPayBasis, _ := collaboration.NoPayBasis()
	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.duty_payment_collaboration
			(tenant_id, scope_ref, duty_ref, kind, no_pay_basis, obligor_ref, requirement_ref, target_ref, formed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), collaboration.Scope().String(), duty.String(),
		kind, noPayBasis,
		collaboration.Obligor().String(), collaboration.Requirement().String(), collaboration.Target().String(),
		collaboration.FormedAt().UTC(),
	)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("save duty collaboration: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	return ports.CaseConfigurationRegistered, nil
}

// RegisterFundsFact 登记一条外部资金事实引用。received_at 取事务内库时钟——它是本上下文
// 接收这一动作的时间，与来源的业务时间 occurred_at 是两列。付款人「来源未提供」那一格落成
// payer_ref 为 NULL（0020 放宽；空串仍被 CHECK 拒）——NULL 在这一列的唯一含义就是 CONTEXT 要
// 「明确记录」的那个「未提供」，不是缺省；零值付款人（两格都不是）是调用方编程错误，写前拒。
func (store *DutyPaymentReconciliation) RegisterFundsFact(
	ctx context.Context,
	tenant domain.TenantID,
	registration ports.ExternalFundsFactRegistration,
) (ports.CaseConfigurationSaveOutcome, error) {
	if strings.TrimSpace(registration.Fact.String()) == "" || registration.OccurredAt.IsZero() {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register funds fact: the fact reference or its business instant is blank")
	}
	if !registration.Payer.Valid() {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register funds fact: the payer is neither provided nor explicitly not provided")
	}
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register funds fact: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.external_funds_fact
			(tenant_id, fact_ref, source_ref, payer_ref, currency, amount_minor, occurred_at, received_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		 ON CONFLICT DO NOTHING`,
		tenant.String(), registration.Fact.String(),
		registration.Source, payerColumn(registration.Payer), registration.Currency, registration.AmountMinor,
		registration.OccurredAt.UTC(),
	)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register funds fact: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	return ports.CaseConfigurationRegistered, nil
}

func (store *DutyPaymentReconciliation) LoadFundsFact(
	ctx context.Context,
	tenant domain.TenantID,
	fact domain.ExternalFundsFactReference,
) (ports.ExternalFundsFactRegistration, bool, error) {
	none := ports.ExternalFundsFactRegistration{}
	if strings.TrimSpace(fact.String()) == "" {
		return none, false, fmt.Errorf("load funds fact: the fact reference is blank")
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load funds fact: %w", err)
	}

	registration := ports.ExternalFundsFactRegistration{Fact: fact}
	var payer *string
	err = querier.QueryRow(ctx,
		`SELECT source_ref, payer_ref, currency, amount_minor, occurred_at
		   FROM customs_compliance.external_funds_fact
		  WHERE tenant_id = $1 AND fact_ref = $2`,
		tenant.String(), fact.String(),
	).Scan(&registration.Source, &payer, &registration.Currency,
		&registration.AmountMinor, &registration.OccurredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load funds fact: %w", err)
	}
	if registration.Payer, err = payerFromColumn(payer); err != nil {
		return none, false, fmt.Errorf("rebuild funds fact: %w", err)
	}
	registration.OccurredAt = registration.OccurredAt.UTC()
	return registration, true, nil
}

// payerColumn 把付款人一格折成 payer_ref 列：「来源提供」是引用本身，「来源未提供」是 NULL。
func payerColumn(payer domain.FundsPayer) *string {
	if !payer.Provided() {
		return nil
	}
	reference := payer.Reference()
	return &reference
}

// payerFromColumn 把 payer_ref 列译回一格：NULL 即「来源未提供」；非空即「来源提供」。空白串到不了这里
// （0016 / 0020 的 CHECK 拦），真到了是库被旁路改过，经领域构造响亮拒。
func payerFromColumn(column *string) (domain.FundsPayer, error) {
	if column == nil {
		return domain.FundsPayerNotProvided(), nil
	}
	return domain.ProvidedFundsPayer(*column)
}

func (store *DutyPaymentReconciliation) FindVerification(
	ctx context.Context,
	key ports.DutyVerificationKey,
) (ports.DutyVerificationRecord, bool, error) {
	none := ports.DutyVerificationRecord{}
	if strings.TrimSpace(key.Digest) == "" {
		return none, false, fmt.Errorf("find duty verification: the version digest is blank")
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("find duty verification: %w", err)
	}

	var (
		coverage, delta, validity, basis string
		verifiedAt                       time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT coverage, delta, validity, basis, verified_at
		   FROM customs_compliance.duty_payment_verification
		  WHERE tenant_id = $1 AND duty_ref = $2 AND funds_ref = $3 AND scope_ref = $4 AND version_digest = $5`,
		key.TenantID.String(), key.Duty.String(), key.Funds.String(), key.Scope.String(), key.Digest,
	).Scan(&coverage, &delta, &validity, &basis, &verifiedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("find duty verification: %w", err)
	}

	verification, err := rebuildVerification(key, coverage, delta, validity, verifiedAt)
	if err != nil {
		return none, false, fmt.Errorf("rebuild duty verification: %w", err)
	}
	return ports.DutyVerificationRecord{Key: key, Verification: verification, Basis: basis}, true, nil
}

// SaveVerification 登记一版核对。键与核对对象说的必须是同一件事——键上的三维若与对象不符，
// 库里就会有一行按 A 查、内容却是 B 的核对，这是调用方编程错误，响亮拒。
func (store *DutyPaymentReconciliation) SaveVerification(
	ctx context.Context,
	record ports.DutyVerificationRecord,
) (ports.CaseConfigurationSaveOutcome, error) {
	verification := record.Verification
	if record.Key.Duty != verification.Duty() || record.Key.Funds != verification.Funds() ||
		record.Key.Scope != verification.Scope() {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("save duty verification: key disagrees with the verification it claims to index")
	}
	if strings.TrimSpace(record.Key.Digest) == "" || strings.TrimSpace(record.Basis) == "" {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("save duty verification: the version digest or the association basis is blank")
	}
	coverage, delta, validity := verification.Coverage().String(), verification.Delta().String(), verification.Validity().String()
	if coverage == "" || delta == "" || validity == "" {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("save duty verification: an axis is outside its closed set")
	}
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("save duty verification: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.duty_payment_verification
			(tenant_id, duty_ref, funds_ref, scope_ref, version_digest,
			 coverage, delta, validity, basis, verified_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(), record.Key.Duty.String(), record.Key.Funds.String(),
		record.Key.Scope.String(), record.Key.Digest,
		coverage, delta, validity, record.Basis, verification.VerifiedAt().UTC(),
	)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("save duty verification: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	return ports.CaseConfigurationRegistered, nil
}

func rebuildCollaboration(
	kind string,
	duty domain.AssessedDutyReference,
	noPayBasis string,
	scope domain.DecisionScopeReference,
	obligor, requirement, target string,
	formedAt time.Time,
) (domain.DutyPaymentCollaboration, error) {
	spec := domain.DutyCollaborationSpec{
		Duty:       duty,
		NoPayBasis: noPayBasis,
		Scope:      scope,
		FormedAt:   formedAt.UTC(),
	}
	switch kind {
	case domain.ObligationFromAssessedDuty.String():
		spec.Kind = domain.ObligationFromAssessedDuty
	case domain.ObligationExplicitlyNotRequired.String():
		spec.Kind = domain.ObligationExplicitlyNotRequired
	default:
		return domain.DutyPaymentCollaboration{}, fmt.Errorf("unknown obligation kind %q", kind)
	}
	var err error
	if spec.Obligor, err = domain.NewLegalObligorReference(obligor); err != nil {
		return domain.DutyPaymentCollaboration{}, err
	}
	if spec.Requirement, err = domain.NewPaymentRequirementSource(requirement); err != nil {
		return domain.DutyPaymentCollaboration{}, err
	}
	if spec.Target, err = domain.NewResponsibilityTargetReference(target); err != nil {
		return domain.DutyPaymentCollaboration{}, err
	}
	return domain.FormDutyCollaboration(spec)
}

func rebuildVerification(
	key ports.DutyVerificationKey,
	coverage, delta, validity string,
	verifiedAt time.Time,
) (domain.DutyPaymentVerification, error) {
	coverageAxis, err := closedWord(coverage, domain.CoverageNone, domain.CoveragePartial, domain.CoverageFull)
	if err != nil {
		return domain.DutyPaymentVerification{}, err
	}
	deltaAxis, err := closedWord(delta, domain.DeltaNone, domain.DeltaShort, domain.DeltaExcess, domain.DeltaPending)
	if err != nil {
		return domain.DutyPaymentVerification{}, err
	}
	validityAxis, err := closedWord(validity,
		domain.FundsFactValid, domain.FundsFactInvalidated, domain.FundsFactConflicting, domain.FundsFactPending)
	if err != nil {
		return domain.DutyPaymentVerification{}, err
	}
	return domain.VerifyDutyPayment(key.Duty, key.Funds, key.Scope, coverageAxis, deltaAxis, validityAxis, verifiedAt.UTC())
}

// closedWord 把库列的词形译回封闭集里的那一格；集外即库被旁路改过，作错误抛出。
func closedWord[T fmt.Stringer](raw string, members ...T) (T, error) {
	for _, member := range members {
		if member.String() == raw {
			return member, nil
		}
	}
	var zero T
	return zero, fmt.Errorf("unknown closed-set word %q", raw)
}
