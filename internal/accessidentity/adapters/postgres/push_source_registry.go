package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// PushSourceRegistry 实现推送源册的登记口、装载口与重放账（access_identity 0006、ADR-0149 决定三第三条）。撞键从不覆盖：
// 同内容重放答原结果，异内容答冲突，册上已有的那一行原样不动。密钥本体不进这些表。
type PushSourceRegistry struct {
	db *bentopg.DB
}

func NewPushSourceRegistry(db *bentopg.DB) (*PushSourceRegistry, error) {
	if db == nil {
		return nil, fmt.Errorf("access identity postgres: db is nil")
	}
	return &PushSourceRegistry{db: db}, nil
}

var (
	_ accessidentity.PushSourceRegistry  = (*PushSourceRegistry)(nil)
	_ accessidentity.PushSourceRegistrar = (*PushSourceRegistry)(nil)
	_ accessidentity.PushReplayLedger    = (*PushSourceRegistry)(nil)
)

func (registry *PushSourceRegistry) RegisterPushSource(
	ctx context.Context,
	binding accessidentity.PushSourceBinding,
) (accessidentity.PushSourceRegistrationOutcome, error) {
	const operation = "register push source"
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	client := binding.Client()
	var bound bool
	if err := executor.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM access_identity.integration_client
			 WHERE issuer = $1 AND subject = $2 AND tenant_id = $3)`,
		client.Issuer(), client.Subject(), binding.TenantID(),
	).Scan(&bound); err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if !bound {
		// 重放一份已经落定的绑定，不因「先查客户端」改口：客户端在，这一支进不来。客户端不在而推送源标识已被别的租户占住，
		// 仍答跨租户，不把「标识已被占」说成「客户端未登记」。
		switch outcome, decided, err := registry.storedPushSourceOutcome(ctx, binding); {
		case err != nil:
			return "", fmt.Errorf("%s: %w", operation, err)
		case decided && outcome == accessidentity.PushSourceBoundToAnotherTenant:
			return outcome, nil
		}
		return accessidentity.PushSourceClientNotRegistered, nil
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO access_identity.push_source
			(source_id, tenant_id, issuer, subject, window_seconds, clock_skew_seconds, basis_ref)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT DO NOTHING`,
		binding.Source().String(), binding.TenantID(), client.Issuer(), client.Subject(),
		secondsOf(binding.Window()), secondsOf(binding.ClockSkew()), binding.Basis(),
	)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return accessidentity.PushSourceRegistrationRecorded, nil
	}
	outcome, decided, err := registry.storedPushSourceOutcome(ctx, binding)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if !decided {
		return "", fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	return outcome, nil
}

// storedPushSourceOutcome 读已有的推送源行并与这份绑定比。行不存在时 decided=false：调用方正在走插入，不该把「还没有」答成冲突。
func (registry *PushSourceRegistry) storedPushSourceOutcome(
	ctx context.Context,
	binding accessidentity.PushSourceBinding,
) (accessidentity.PushSourceRegistrationOutcome, bool, error) {
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return "", false, err
	}
	var (
		tenant, issuer, subject, basis string
		windowSeconds, skewSeconds     int64
	)
	err = executor.QueryRow(ctx,
		`SELECT tenant_id, issuer, subject, window_seconds, clock_skew_seconds, basis_ref
		   FROM access_identity.push_source
		  WHERE source_id = $1`,
		binding.Source().String(),
	).Scan(&tenant, &issuer, &subject, &windowSeconds, &skewSeconds, &basis)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	client := binding.Client()
	switch {
	case tenant != binding.TenantID():
		return accessidentity.PushSourceBoundToAnotherTenant, true, nil
	case issuer == client.Issuer() && subject == client.Subject() &&
		windowSeconds == secondsOf(binding.Window()) && skewSeconds == secondsOf(binding.ClockSkew()) &&
		basis == binding.Basis():
		return accessidentity.PushSourceRegistrationAlreadyRegistered, true, nil
	default:
		return accessidentity.PushSourceRegistrationContentConflict, true, nil
	}
}

func (registry *PushSourceRegistry) RegisterPushSigningKey(
	ctx context.Context,
	key accessidentity.PushSigningKey,
) (accessidentity.PushSourceRegistrationOutcome, error) {
	const operation = "register push signing key"
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	var bound bool
	if err := executor.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM access_identity.push_source
			 WHERE source_id = $1 AND tenant_id = $2)`,
		key.Source().String(), key.TenantID(),
	).Scan(&bound); err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if !bound {
		return accessidentity.PushSourceNotRegistered, nil
	}

	startsAt := key.Interval().StartsAt()
	var endsAt *time.Time
	if end, bounded := key.Interval().EndsAt(); bounded {
		endsAt = &end
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO access_identity.push_signing_key
			(tenant_id, source_id, key_id, form, key_ref, effective_starts_at, effective_ends_at, basis_ref)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT DO NOTHING`,
		key.TenantID(), key.Source().String(), key.KeyID(), key.Form().String(), key.Reference().String(),
		startsAt, endsAt, key.Basis(),
	)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return accessidentity.PushSourceRegistrationRecorded, nil
	}

	var (
		storedForm, storedRef, storedBasis string
		storedStartsAt                     time.Time
		storedEndsAt                       *time.Time
	)
	err = executor.QueryRow(ctx,
		`SELECT form, key_ref, effective_starts_at, effective_ends_at, basis_ref
		   FROM access_identity.push_signing_key
		  WHERE tenant_id = $1 AND source_id = $2 AND key_id = $3`,
		key.TenantID(), key.Source().String(), key.KeyID(),
	).Scan(&storedForm, &storedRef, &storedStartsAt, &storedEndsAt, &storedBasis)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if storedForm == key.Form().String() && storedRef == key.Reference().String() &&
		sameInstant(storedStartsAt, startsAt) && sameOptionalInstant(storedEndsAt, endsAt) && storedBasis == key.Basis() {
		return accessidentity.PushSourceRegistrationAlreadyRegistered, nil
	}
	return accessidentity.PushSourceRegistrationContentConflict, nil
}

func (registry *PushSourceRegistry) RegisterPushSigningKeyRetirement(
	ctx context.Context,
	retirement accessidentity.PushSigningKeyRetirement,
) (accessidentity.PushSourceRegistrationOutcome, error) {
	const operation = "register push signing key retirement"
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	var registered bool
	if err := executor.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM access_identity.push_signing_key
			 WHERE tenant_id = $1 AND source_id = $2 AND key_id = $3)`,
		retirement.TenantID(), retirement.Source().String(), retirement.KeyID(),
	).Scan(&registered); err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if !registered {
		return accessidentity.PushSigningKeyNotRegistered, nil
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO access_identity.push_signing_key_retirement
			(tenant_id, source_id, key_id, retires_at, basis_ref)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT DO NOTHING`,
		retirement.TenantID(), retirement.Source().String(), retirement.KeyID(), retirement.RetiresAt(), retirement.Basis(),
	)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return accessidentity.PushSourceRegistrationRecorded, nil
	}

	var (
		storedRetiresAt time.Time
		storedBasis     string
	)
	err = executor.QueryRow(ctx,
		`SELECT retires_at, basis_ref
		   FROM access_identity.push_signing_key_retirement
		  WHERE tenant_id = $1 AND source_id = $2 AND key_id = $3`,
		retirement.TenantID(), retirement.Source().String(), retirement.KeyID(),
	).Scan(&storedRetiresAt, &storedBasis)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if sameInstant(storedRetiresAt, retirement.RetiresAt()) && storedBasis == retirement.Basis() {
		return accessidentity.PushSourceRegistrationAlreadyRegistered, nil
	}
	return accessidentity.PushSourceRegistrationContentConflict, nil
}

// FindPushSource 用一条查询取绑定与名下全部密钥（连同停用）。此刻生不生效不在这里判。
func (registry *PushSourceRegistry) FindPushSource(
	ctx context.Context,
	source accessidentity.PushSourceID,
) (accessidentity.PushSourceStanding, bool, error) {
	const operation = "find push source"
	querier, err := registry.db.ReadExecutor(ctx)
	if err != nil {
		return accessidentity.PushSourceStanding{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := querier.Query(ctx,
		`SELECT s.tenant_id, s.issuer, s.subject, s.window_seconds, s.clock_skew_seconds, s.basis_ref,
		        k.key_id, k.form, k.key_ref, k.effective_starts_at, k.effective_ends_at, k.basis_ref,
		        r.retires_at, r.basis_ref
		   FROM access_identity.push_source s
		   LEFT JOIN access_identity.push_signing_key k
		     ON k.source_id = s.source_id AND k.tenant_id = s.tenant_id
		   LEFT JOIN access_identity.push_signing_key_retirement r
		     ON r.tenant_id = k.tenant_id AND r.source_id = k.source_id AND r.key_id = k.key_id
		  WHERE s.source_id = $1
		  ORDER BY k.effective_starts_at, k.key_id`,
		source.String(),
	)
	if err != nil {
		return accessidentity.PushSourceStanding{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()

	var (
		found    bool
		binding  accessidentity.PushSourceBinding
		recorded []accessidentity.RecordedPushSigningKey
	)
	for rows.Next() {
		var (
			tenant, issuer, subject, basis string
			windowSeconds, skewSeconds     int64
			keyID, form, keyRef, keyBasis  *string
			startsAt, endsAt, retiresAt    *time.Time
			retirementBasis                *string
		)
		if err := rows.Scan(&tenant, &issuer, &subject, &windowSeconds, &skewSeconds, &basis,
			&keyID, &form, &keyRef, &startsAt, &endsAt, &keyBasis,
			&retiresAt, &retirementBasis,
		); err != nil {
			return accessidentity.PushSourceStanding{}, false, fmt.Errorf("%s: %w", operation, err)
		}
		if !found {
			binding, err = pushBindingFromRow(source, tenant, issuer, subject, windowSeconds, skewSeconds, basis)
			if err != nil {
				return accessidentity.PushSourceStanding{}, false, fmt.Errorf("%s: %w", operation, err)
			}
			found = true
		}
		if keyID == nil {
			continue
		}
		entry, err := recordedPushKeyFromRow(tenant, source, *keyID, *form, *keyRef, *startsAt, endsAt, *keyBasis, retiresAt, retirementBasis)
		if err != nil {
			return accessidentity.PushSourceStanding{}, false, fmt.Errorf("%s: 密钥 %s：%w", operation, *keyID, err)
		}
		recorded = append(recorded, entry)
	}
	if err := rows.Err(); err != nil {
		return accessidentity.PushSourceStanding{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	if !found {
		return accessidentity.PushSourceStanding{}, false, nil
	}
	standing, err := accessidentity.NewPushSourceStanding(binding, recorded)
	if err != nil {
		return accessidentity.PushSourceStanding{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return standing, true, nil
}

func (registry *PushSourceRegistry) RecordPushReceipt(ctx context.Context, receipt accessidentity.PushReceipt) (bool, error) {
	const operation = "record push receipt"
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO access_identity.push_receipt
			(source_id, digest, tenant_id, push_id, signed_at, retain_until)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT DO NOTHING`,
		receipt.Source().String(), receipt.Digest(), receipt.TenantID(), receipt.PushID(),
		receipt.SignedAt(), receipt.RetainUntil(),
	)
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	return tag.RowsAffected() > 0, nil
}

func pushBindingFromRow(
	source accessidentity.PushSourceID,
	tenant, issuer, subject string,
	windowSeconds, skewSeconds int64,
	basis string,
) (accessidentity.PushSourceBinding, error) {
	client, err := accessidentity.NewIntegrationClientSubject(issuer, subject)
	if err != nil {
		return accessidentity.PushSourceBinding{}, err
	}
	window, err := durationFromSeconds(windowSeconds)
	if err != nil {
		return accessidentity.PushSourceBinding{}, err
	}
	skew, err := durationFromSeconds(skewSeconds)
	if err != nil {
		return accessidentity.PushSourceBinding{}, err
	}
	return accessidentity.NewPushSourceBinding(source, client, tenant, window, skew, basis)
}

func recordedPushKeyFromRow(
	tenant string,
	source accessidentity.PushSourceID,
	keyID, rawForm, rawRef string,
	startsAt time.Time,
	endsAt *time.Time,
	basis string,
	retiresAt *time.Time,
	retirementBasis *string,
) (accessidentity.RecordedPushSigningKey, error) {
	form, err := accessidentity.ParsePushSignatureForm(rawForm)
	if err != nil {
		return accessidentity.RecordedPushSigningKey{}, err
	}
	reference, err := accessidentity.NewCredentialReference(rawRef)
	if err != nil {
		return accessidentity.RecordedPushSigningKey{}, err
	}
	end := time.Time{}
	if endsAt != nil {
		end = *endsAt
	}
	interval, err := accessidentity.NewEffectiveInterval(startsAt, end)
	if err != nil {
		return accessidentity.RecordedPushSigningKey{}, err
	}
	key, err := accessidentity.NewPushSigningKey(tenant, source, keyID, form, reference, interval, basis)
	if err != nil {
		return accessidentity.RecordedPushSigningKey{}, err
	}
	if retiresAt == nil {
		return accessidentity.NewRecordedPushSigningKey(key, nil)
	}
	referenceBasis := ""
	if retirementBasis != nil {
		referenceBasis = *retirementBasis
	}
	retirement, err := accessidentity.NewPushSigningKeyRetirement(tenant, source, keyID, *retiresAt, referenceBasis)
	if err != nil {
		return accessidentity.RecordedPushSigningKey{}, err
	}
	return accessidentity.NewRecordedPushSigningKey(key, &retirement)
}

func secondsOf(d time.Duration) int64 {
	return int64(d / time.Second)
}

func durationFromSeconds(seconds int64) (time.Duration, error) {
	if seconds < 0 {
		return 0, fmt.Errorf("access identity postgres: negative duration")
	}
	d := time.Duration(seconds) * time.Second
	if int64(d/time.Second) != seconds {
		return 0, fmt.Errorf("access identity postgres: duration overflows")
	}
	return d, nil
}
