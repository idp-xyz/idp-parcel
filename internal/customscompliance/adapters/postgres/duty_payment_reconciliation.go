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

// RegisterFundsFact 登记一条外部资金事实的一个版本：身份行（0016 的 external_funds_fact，0021 起只留身份）先
// DO NOTHING 落一次，再落版本子表一行（0021 的 external_funds_fact_version，键（租户、事实、版本））；同键由
// DO NOTHING 折成`已登记`交回，内容是否同一份由编排读回自己比。两处 received_at 都取事务内库时钟——它是本上下文
// 接收这一动作的时间，与来源的业务时间 occurred_at 是两列；身份行上的那一个是首次接收。付款人「来源未提供」
// 那一格落成 payer_ref 为 NULL（0020 起；空串仍被 CHECK 拒）——NULL 在这一列的唯一含义就是 CONTEXT 要「明确记录」
// 的那个「未提供」，不是缺省；零值付款人（两格都不是）、版本空白、回指自己都是调用方编程错误，写前拒。
// 回指的前版是否已到不校验：版本链的权威在提供方，迟到的前版按自己的版本进（票 sa-cc/13 裁决 1）。
func (store *DutyPaymentReconciliation) RegisterFundsFact(
	ctx context.Context,
	tenant domain.TenantID,
	registration ports.ExternalFundsFactRegistration,
) (ports.CaseConfigurationSaveOutcome, error) {
	if strings.TrimSpace(registration.Fact.String()) == "" || registration.OccurredAt.IsZero() {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register funds fact: the fact reference or its business instant is blank")
	}
	if strings.TrimSpace(registration.Version.String()) == "" {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register funds fact: the version is blank")
	}
	if registration.Corrects == registration.Version {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register funds fact: a version cannot correct itself")
	}
	if !registration.Payer.Valid() {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register funds fact: the payer is neither provided nor explicitly not provided")
	}
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register funds fact: %w", err)
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.external_funds_fact (tenant_id, fact_ref, received_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT DO NOTHING`,
		tenant.String(), registration.Fact.String(),
	); err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register funds fact: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.external_funds_fact_version
			(tenant_id, fact_ref, version, corrects_version,
			 source_ref, payer_ref, currency, amount_minor, occurred_at, received_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		 ON CONFLICT DO NOTHING`,
		tenant.String(), registration.Fact.String(), registration.Version.String(), correctsColumn(registration.Corrects),
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

// fundsFactVersionColumns 是版本子表读回一版所需的列，两个读口共用一份、同一只扫描器译回。
const fundsFactVersionColumns = `version, corrects_version, source_ref, payer_ref, currency, amount_minor, occurred_at`

// LoadFundsFactVersion 按（租户、事实、版本）点读一版（端口头注：核对按命令所指的那一版读前置与付款人维，
// 「最近接收」读口自票 sa-cc/19 起退役）。版本只是键的一维，不比大小——版本链的权威在提供方。
func (store *DutyPaymentReconciliation) LoadFundsFactVersion(
	ctx context.Context,
	tenant domain.TenantID,
	fact domain.ExternalFundsFactReference,
	version domain.FundsFactVersion,
) (ports.ExternalFundsFactRegistration, bool, error) {
	none := ports.ExternalFundsFactRegistration{}
	if strings.TrimSpace(fact.String()) == "" || strings.TrimSpace(version.String()) == "" {
		return none, false, fmt.Errorf("load funds fact version: the fact reference or the version is blank")
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load funds fact version: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT `+fundsFactVersionColumns+`
		   FROM customs_compliance.external_funds_fact_version
		  WHERE tenant_id = $1 AND fact_ref = $2 AND version = $3`,
		tenant.String(), fact.String(), version.String(),
	)
	if err != nil {
		return none, false, fmt.Errorf("load funds fact version: %w", err)
	}
	versions, err := scanFundsFactVersions(rows, fact)
	if err != nil {
		return none, false, fmt.Errorf("load funds fact version: %w", err)
	}
	if len(versions) == 0 {
		return none, false, nil
	}
	return versions[0], true, nil
}

// ListFundsFactVersions 按接收先后列一条事实的全部版本、每版带回指前版——「登记册看得见新版本与回指」
// （票 sa-cc/13 裁决 2）就是这一问。
func (store *DutyPaymentReconciliation) ListFundsFactVersions(
	ctx context.Context,
	tenant domain.TenantID,
	fact domain.ExternalFundsFactReference,
) ([]ports.ExternalFundsFactRegistration, error) {
	if strings.TrimSpace(fact.String()) == "" {
		return nil, fmt.Errorf("list funds fact versions: the fact reference is blank")
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list funds fact versions: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT `+fundsFactVersionColumns+`
		   FROM customs_compliance.external_funds_fact_version
		  WHERE tenant_id = $1 AND fact_ref = $2
		  ORDER BY received_at ASC, version ASC`,
		tenant.String(), fact.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list funds fact versions: %w", err)
	}
	versions, err := scanFundsFactVersions(rows, fact)
	if err != nil {
		return nil, fmt.Errorf("list funds fact versions: %w", err)
	}
	return versions, nil
}

// scanFundsFactVersions 把版本子表的行译回登记：版本与回指经领域构造回来（空白到不了这里，CHECK 拦；真到了
// 是库被旁路改过，响亮拒），付款人 NULL 译「来源未提供」。
func scanFundsFactVersions(rows pgx.Rows, fact domain.ExternalFundsFactReference) ([]ports.ExternalFundsFactRegistration, error) {
	defer rows.Close()
	var versions []ports.ExternalFundsFactRegistration
	for rows.Next() {
		var (
			versionRaw      string
			corrects, payer *string
		)
		registration := ports.ExternalFundsFactRegistration{Fact: fact}
		if err := rows.Scan(&versionRaw, &corrects, &registration.Source, &payer, &registration.Currency,
			&registration.AmountMinor, &registration.OccurredAt); err != nil {
			return nil, err
		}
		var err error
		if registration.Version, err = domain.NewFundsFactVersion(versionRaw); err != nil {
			return nil, fmt.Errorf("rebuild funds fact version: %w", err)
		}
		if corrects != nil {
			if registration.Corrects, err = domain.NewFundsFactVersion(*corrects); err != nil {
				return nil, fmt.Errorf("rebuild funds fact version: corrects: %w", err)
			}
		}
		if registration.Payer, err = payerFromColumn(payer); err != nil {
			return nil, fmt.Errorf("rebuild funds fact version: %w", err)
		}
		registration.OccurredAt = registration.OccurredAt.UTC()
		versions = append(versions, registration)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return versions, nil
}

// correctsColumn 把回指前版折成 corrects_version 列：首版是 NULL。
func correctsColumn(corrects domain.FundsFactVersion) *string {
	if corrects.String() == "" {
		return nil
	}
	value := corrects.String()
	return &value
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
		fundsVersion, procedure, coverage, delta, validity, basis string
		verifiedAt                                                time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT funds_version, procedure_ref, coverage, delta, validity, basis, verified_at
		   FROM customs_compliance.duty_payment_verification
		  WHERE tenant_id = $1 AND duty_ref = $2 AND funds_ref = $3 AND scope_ref = $4 AND version_digest = $5`,
		key.TenantID.String(), key.Duty.String(), key.Funds.String(), key.Scope.String(), key.Digest,
	).Scan(&fundsVersion, &procedure, &coverage, &delta, &validity, &basis, &verifiedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("find duty verification: %w", err)
	}

	verification, err := rebuildVerification(key, fundsVersion, procedure, coverage, delta, validity, verifiedAt)
	if err != nil {
		return none, false, fmt.Errorf("rebuild duty verification: %w", err)
	}
	return ports.DutyVerificationRecord{Key: key, Verification: verification, Basis: basis}, true, nil
}

// ListVerificationsByFundsFact 按（租户、资金事实）列全部核对版本，核对时刻升序、同一时刻按指纹字典序（端口
// 头注的口径；与 LoadCurrentDutyVerification 取「当前」的序同一把尺，编排按它取各谱系最近一版时两口答案一致）。
// 读回经 rebuildVerification 整门重验。
func (store *DutyPaymentReconciliation) ListVerificationsByFundsFact(
	ctx context.Context,
	tenant domain.TenantID,
	funds domain.ExternalFundsFactReference,
) ([]ports.DutyVerificationRecord, error) {
	if strings.TrimSpace(funds.String()) == "" {
		return nil, fmt.Errorf("list duty verifications by funds fact: the fact reference is blank")
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list duty verifications by funds fact: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT duty_ref, scope_ref, version_digest,
		        funds_version, procedure_ref, coverage, delta, validity, basis, verified_at
		   FROM customs_compliance.duty_payment_verification
		  WHERE tenant_id = $1 AND funds_ref = $2
		  ORDER BY verified_at ASC, version_digest ASC`,
		tenant.String(), funds.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list duty verifications by funds fact: %w", err)
	}
	defer rows.Close()

	var records []ports.DutyVerificationRecord
	for rows.Next() {
		var (
			dutyRaw, scopeRaw, digest                                 string
			fundsVersion, procedure, coverage, delta, validity, basis string
			verifiedAt                                                time.Time
		)
		if err := rows.Scan(&dutyRaw, &scopeRaw, &digest,
			&fundsVersion, &procedure, &coverage, &delta, &validity, &basis, &verifiedAt); err != nil {
			return nil, fmt.Errorf("list duty verifications by funds fact: %w", err)
		}
		key := ports.DutyVerificationKey{TenantID: tenant, Funds: funds, Digest: digest}
		if key.Duty, err = domain.NewAssessedDutyReference(dutyRaw); err != nil {
			return nil, fmt.Errorf("rebuild duty verification: %w", err)
		}
		if key.Scope, err = domain.NewDecisionScopeReference(scopeRaw); err != nil {
			return nil, fmt.Errorf("rebuild duty verification: %w", err)
		}
		verification, err := rebuildVerification(key, fundsVersion, procedure, coverage, delta, validity, verifiedAt)
		if err != nil {
			return nil, fmt.Errorf("rebuild duty verification: %w", err)
		}
		records = append(records, ports.DutyVerificationRecord{Key: key, Verification: verification, Basis: basis})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list duty verifications by funds fact: %w", err)
	}
	return records, nil
}

// SaveVerification 登记一版核对。键与核对对象说的必须是同一件事——键上的三维若与对象不符，
// 库里就会有一行按 A 查、内容却是 B 的核对，这是调用方编程错误，响亮拒。程序与资金版本随核对对象
// 落成 `procedure_ref`（0022）/ `funds_version`（0023），都不在键上、只在指纹里，所以这里不与键比；
// 资金版本对不上已接收的版本由 0023 的外键拦。
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
			 funds_version, procedure_ref, coverage, delta, validity, basis, verified_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(), record.Key.Duty.String(), record.Key.Funds.String(),
		record.Key.Scope.String(), record.Key.Digest,
		verification.FundsVersion().String(), verification.Procedure().String(),
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

// rebuildVerification 把一行译回核对对象、整门重验：资金版本与程序经领域构造回来（空白到不了这里，0023 / 0022
// 的 CHECK 拦；真到了是库被旁路改过，响亮拒），三轴按封闭词译回。
func rebuildVerification(
	key ports.DutyVerificationKey,
	fundsVersion, procedure, coverage, delta, validity string,
	verifiedAt time.Time,
) (domain.DutyPaymentVerification, error) {
	version, err := domain.NewFundsFactVersion(fundsVersion)
	if err != nil {
		return domain.DutyPaymentVerification{}, err
	}
	procedureRef, err := domain.NewCustomsProcedureReference(procedure)
	if err != nil {
		return domain.DutyPaymentVerification{}, err
	}
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
	return domain.VerifyDutyPayment(key.Duty, key.Funds, version, key.Scope, procedureRef, coverageAxis, deltaAxis, validityAxis, verifiedAt.UTC())
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
