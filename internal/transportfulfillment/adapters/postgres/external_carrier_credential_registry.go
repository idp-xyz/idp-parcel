package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ExternalCarrierCredentials 实现 ports.ExternalCarrierCredentialRegistry 与
// ports.ExternalCarrierCredentialResolver：凭证一个版本一行，只插不改。
//
// 一个类型担两个端口是因为解析口读的就是这本册子——「此刻指向谁」是对登记内容的一次问答，
// 不是另一份数据；装配处仍按端口各自注入。有了这个实现之后，CredentialRegistryUnconfigured
// 那一格只剩「装配处没接登记册」一种来路，本类型从不产出它。
type ExternalCarrierCredentials struct {
	db    *bentopg.DB
	clock ports.Clock
}

// NewExternalCarrierCredentials 需要时钟，因为解析口答的是「此刻」：适用范围是业务时间区间，
// 判它盖不盖住当下要有一个当下。登记册那一半不用时钟——落库时刻由记录自带。
func NewExternalCarrierCredentials(db *bentopg.DB, clock ports.Clock) (*ExternalCarrierCredentials, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &ExternalCarrierCredentials{db: db, clock: clock}, nil
}

var (
	_ ports.ExternalCarrierCredentialRegistry = (*ExternalCarrierCredentials)(nil)
	_ ports.ExternalCarrierCredentialResolver = (*ExternalCarrierCredentials)(nil)
)

const externalCarrierCredentialColumns = `tenant_id, credential_ref, version, assigner_ref,
		        identified_kind, identified_ref, effective_from, effective_until,
		        standing, changed_at, supersedes_version, replaced_by_credential, recorded_at`

// FindByKey 按（租户+凭证+版本）取回一个版本。否定结果只回 false。
func (repository *ExternalCarrierCredentials) FindByKey(
	ctx context.Context,
	key ports.ExternalCarrierCredentialKey,
) (ports.ExternalCarrierCredentialRecord, bool, error) {
	records, err := repository.query(ctx, "find external carrier credential",
		`SELECT `+externalCarrierCredentialColumns+`
		   FROM transport_fulfillment.external_carrier_credential
		  WHERE tenant_id = $1 AND credential_ref = $2 AND version = $3`,
		key.TenantID.String(), key.Credential.String(), key.Version.String())
	if err != nil || len(records) == 0 {
		return ports.ExternalCarrierCredentialRecord{}, false, err
	}
	return records[0], true, nil
}

// FindCurrent 取回一份凭证此刻未被任何版本回指的那一版。「当前」是派生问答不是可变标记：
// 表上没有 current 列，改一版就得回写的东西这里一个都没有。
func (repository *ExternalCarrierCredentials) FindCurrent(
	ctx context.Context,
	tenant domain.TenantID,
	credential domain.ExternalCarrierCredentialReference,
) (ports.ExternalCarrierCredentialRecord, bool, error) {
	records, err := repository.query(ctx, "find current external carrier credential",
		`SELECT `+externalCarrierCredentialColumns+`
		   FROM transport_fulfillment.external_carrier_credential AS current
		  WHERE tenant_id = $1 AND credential_ref = $2
		    AND NOT EXISTS (
		        SELECT 1 FROM transport_fulfillment.external_carrier_credential AS successor
		         WHERE successor.tenant_id = current.tenant_id
		           AND successor.credential_ref = current.credential_ref
		           AND successor.supersedes_version = current.version)
		  ORDER BY recorded_at DESC
		  LIMIT 1`,
		tenant.String(), credential.String())
	if err != nil || len(records) == 0 {
		return ports.ExternalCarrierCredentialRecord{}, false, err
	}
	return records[0], true, nil
}

// ListVersions 按登记先后交回一份凭证的全部版本；同一时刻落的再按版本号排，让顺序可复现。
func (repository *ExternalCarrierCredentials) ListVersions(
	ctx context.Context,
	tenant domain.TenantID,
	credential domain.ExternalCarrierCredentialReference,
) ([]ports.ExternalCarrierCredentialRecord, error) {
	return repository.query(ctx, "list external carrier credential versions",
		`SELECT `+externalCarrierCredentialColumns+`
		   FROM transport_fulfillment.external_carrier_credential
		  WHERE tenant_id = $1 AND credential_ref = $2
		  ORDER BY recorded_at, version`,
		tenant.String(), credential.String())
}

// ResolveCredential 答「这份凭证此刻指向哪个载运对象」。四种情形同落`未知`（理由在端口注释）：
// 从未登记、当前版已不适用、当前版的适用范围盖不住此刻、当前版标识的不是载运对象。
//
// 适用状态与适用范围两道都看，缺一格都不答`已解析`：状态是分配方明说的（作废、失效、替代），
// 范围是登记时给的终点——首版可以带一个预先声明的终点，到期而没人来登失效版本时，范围那一
// 道仍然拦得住。相反，一份「自某个将来时刻起作废」的凭证虽然范围此刻仍盖得住，状态已经说了
// 分配方收回了它，也不再用它认领事实。两道都朝保守方向，因为`未知`的续办只是去查登记册，而
// 把事实认到一份已被收回的凭证所指的对象上，事后要撤的就不止一条。
func (repository *ExternalCarrierCredentials) ResolveCredential(
	ctx context.Context,
	tenant domain.TenantID,
	credential domain.ExternalCarrierCredentialReference,
) (domain.CarriedObjectReference, ports.CredentialResolution, error) {
	current, found, err := repository.FindCurrent(ctx, tenant, credential)
	if err != nil {
		return domain.CarriedObjectReference{}, ports.CredentialResolutionInvalid, fmt.Errorf("resolve external carrier credential: %w", err)
	}
	if !found || !current.Credential.Applicable() || !current.Credential.Applicability().Covers(repository.clock.Now()) {
		return domain.CarriedObjectReference{}, ports.CredentialUnknown, nil
	}
	object, identifiesCarriedObject := current.Credential.Identifies().CarriedObject()
	if !identifiesCarriedObject {
		return domain.CarriedObjectReference{}, ports.CredentialUnknown, nil
	}
	return object, ports.CredentialResolved, nil
}

func (repository *ExternalCarrierCredentials) query(
	ctx context.Context,
	verb string,
	sql string,
	args ...any,
) ([]ports.ExternalCarrierCredentialRecord, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", verb, err)
	}
	rows, err := querier.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", verb, err)
	}
	defer rows.Close()

	var records []ports.ExternalCarrierCredentialRecord
	for rows.Next() {
		record, err := scanExternalCarrierCredential(rows)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", verb, err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%s: %w", verb, err)
	}
	return records, nil
}

// scanExternalCarrierCredential 把一行装回一个版本。逐列走各自的构造门，再交给重建门核
// 状态、终点、前版、替代者四者的配合——不按列直接拼结构体（ADR-0028）。
func scanExternalCarrierCredential(rows pgx.Rows) (ports.ExternalCarrierCredentialRecord, error) {
	var tenantID, credentialRef, version, assignerRef, identifiedKind, identifiedRef, standing string
	var effectiveUntil, changedAt *time.Time
	var effectiveFrom, recordedAt time.Time
	var supersedes, replacedBy *string
	if err := rows.Scan(
		&tenantID, &credentialRef, &version, &assignerRef,
		&identifiedKind, &identifiedRef, &effectiveFrom, &effectiveUntil,
		&standing, &changedAt, &supersedes, &replacedBy, &recordedAt,
	); err != nil {
		return ports.ExternalCarrierCredentialRecord{}, err
	}

	spec := domain.RehydrateExternalCarrierCredentialSpec{EffectiveFrom: effectiveFrom.UTC()}
	var err error
	if spec.TenantID, err = domain.NewTenantID(tenantID); err != nil {
		return ports.ExternalCarrierCredentialRecord{}, err
	}
	if spec.Credential, err = domain.NewExternalCarrierCredentialReference(credentialRef); err != nil {
		return ports.ExternalCarrierCredentialRecord{}, err
	}
	if spec.Version, err = domain.NewExternalCarrierCredentialVersion(version); err != nil {
		return ports.ExternalCarrierCredentialRecord{}, err
	}
	if spec.Assigner, err = domain.NewCredentialAssignerReference(assignerRef); err != nil {
		return ports.ExternalCarrierCredentialRecord{}, err
	}
	kind, err := domain.ParseIdentifiedObjectKind(identifiedKind)
	if err != nil {
		return ports.ExternalCarrierCredentialRecord{}, err
	}
	if spec.Identifies, err = domain.NewIdentifiedObject(kind, identifiedRef); err != nil {
		return ports.ExternalCarrierCredentialRecord{}, err
	}
	if spec.Standing, err = domain.ParseCredentialStanding(standing); err != nil {
		return ports.ExternalCarrierCredentialRecord{}, err
	}
	if effectiveUntil != nil {
		spec.EffectiveUntil = effectiveUntil.UTC()
	}
	if changedAt != nil {
		spec.ChangedAt = changedAt.UTC()
	}
	if supersedes != nil {
		if spec.Supersedes, err = domain.NewExternalCarrierCredentialVersion(*supersedes); err != nil {
			return ports.ExternalCarrierCredentialRecord{}, err
		}
	}
	if replacedBy != nil {
		if spec.ReplacedBy, err = domain.NewExternalCarrierCredentialReference(*replacedBy); err != nil {
			return ports.ExternalCarrierCredentialRecord{}, err
		}
	}

	credential, err := domain.RehydrateExternalCarrierCredential(spec)
	if err != nil {
		return ports.ExternalCarrierCredentialRecord{}, err
	}
	return ports.ExternalCarrierCredentialRecord{
		Key:        ports.ExternalCarrierCredentialKey{TenantID: spec.TenantID, Credential: spec.Credential, Version: spec.Version},
		Credential: credential,
		RecordedAt: recordedAt.UTC(),
	}, nil
}

// Save 登记一个版本。撞键答`已登记`（ADR-0031）：撞的是主键（租户+凭证+版本），是重放还是
// 改内容由调用方读回既有版本比对——写口只答「这一键已经有了」。
func (repository *ExternalCarrierCredentials) Save(
	ctx context.Context,
	record ports.ExternalCarrierCredentialRecord,
) (ports.ExternalCarrierCredentialSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ExternalCarrierCredentialSaveOutcomeInvalid, fmt.Errorf("save external carrier credential: %w", err)
	}
	if err := assertExternalCarrierCredentialKeyAgrees(record); err != nil {
		return ports.ExternalCarrierCredentialSaveOutcomeInvalid, fmt.Errorf("save external carrier credential: %w", err)
	}

	credential := record.Credential
	var supersedes, replacedBy *string
	if prior, has := credential.Supersedes(); has {
		text := prior.String()
		supersedes = &text
	}
	if replacement, has := credential.ReplacedBy(); has {
		text := replacement.String()
		replacedBy = &text
	}
	until, closed := credential.Applicability().Until()
	changedAt, changed := credential.ChangedAt()

	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.external_carrier_credential
		     (`+externalCarrierCredentialColumns+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Credential.String(),
		record.Key.Version.String(),
		credential.Assigner().String(),
		credential.Identifies().Kind().String(),
		credential.Identifies().Reference(),
		credential.Applicability().From().UTC(),
		nullableTime(until, closed),
		credential.Standing().String(),
		nullableTime(changedAt, changed),
		supersedes,
		replacedBy,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.ExternalCarrierCredentialSaveOutcomeInvalid, fmt.Errorf("save external carrier credential: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ExternalCarrierCredentialAlreadyRegistered, nil
	}
	return ports.ExternalCarrierCredentialSaved, nil
}

// assertExternalCarrierCredentialKeyAgrees 挡住「键说的是一个版本、聚合说的是另一个」那种写入。
func assertExternalCarrierCredentialKeyAgrees(record ports.ExternalCarrierCredentialRecord) error {
	if record.Key.TenantID != record.Credential.TenantID() ||
		record.Key.Credential != record.Credential.Credential() ||
		record.Key.Version != record.Credential.Version() {
		return fmt.Errorf("external carrier credential key disagrees with the aggregate")
	}
	return nil
}
