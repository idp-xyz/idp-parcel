package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// RegistrationNumberTypes 实现 ports.RegistrationNumberTypeRegistry 与 ports.RegistrationNumberTypeLookup：
// 注册号类型目录的只增登记、最新修订装载与按国家 / 地区取整份目录（0033 迁移）。撞键不覆盖：
// 同内容重放，异内容冲突（ADR-0031，判据与 PartyIdentityRegistrations 同款）。
type RegistrationNumberTypes struct {
	db *bentopg.DB
}

func NewRegistrationNumberTypes(db *bentopg.DB) (*RegistrationNumberTypes, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &RegistrationNumberTypes{db: db}, nil
}

var (
	_ ports.RegistrationNumberTypeRegistry = (*RegistrationNumberTypes)(nil)
	_ ports.RegistrationNumberTypeLookup   = (*RegistrationNumberTypes)(nil)
)

// registrationNumberTypeDocument 是登记快照的形状。生命周期两件随快照走，所以停用修订与它前一笔
// 登记修订的内容摘要不同——停用落在下一个修订号上，不会被当成对前一笔的重放。
type registrationNumberTypeDocument struct {
	Tenant            string     `json:"tenant"`
	Country           string     `json:"country"`
	TypeCode          string     `json:"typeCode"`
	Revision          int        `json:"revision"`
	TypeName          string     `json:"typeName"`
	Layer             string     `json:"layer"`
	FormatPattern     string     `json:"formatPattern"`
	Basis             string     `json:"basis"`
	EffectiveFrom     time.Time  `json:"effectiveFrom"`
	DeactivatedAt     *time.Time `json:"deactivatedAt,omitempty"`
	DeactivationBasis string     `json:"deactivationBasis,omitempty"`
}

func documentOfRegistrationNumberType(
	registration domain.RegistrationNumberTypeRegistration,
) registrationNumberTypeDocument {
	lifecycle := registration.Lifecycle()
	document := registrationNumberTypeDocument{
		Tenant:        registration.Tenant().String(),
		Country:       registration.Country().String(),
		TypeCode:      registration.Code().String(),
		Revision:      registration.Revision(),
		TypeName:      registration.Name().String(),
		Layer:         registration.Layer().String(),
		FormatPattern: registration.Format().Pattern(),
		Basis:         registration.Basis().String(),
		EffectiveFrom: lifecycle.EffectiveFrom().UTC(),
	}
	if basis, at, has := lifecycle.Deactivation(); has {
		utc := at.UTC()
		document.DeactivatedAt = &utc
		document.DeactivationBasis = basis.String()
	}
	return document
}

func (repository *RegistrationNumberTypes) SaveRegistrationNumberType(
	ctx context.Context,
	registration domain.RegistrationNumberTypeRegistration,
) (ports.RegistrationNumberTypeSaveOutcome, error) {
	const operation = "save registration number type"
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.RegistrationNumberTypeSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}

	document := documentOfRegistrationNumberType(registration)
	raw, err := json.Marshal(document)
	if err != nil {
		return ports.RegistrationNumberTypeSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	digest := sha256.Sum256(raw)
	contentDigest := hex.EncodeToString(digest[:])

	var deactivationBasis *string
	if document.DeactivatedAt != nil {
		deactivationBasis = &document.DeactivationBasis
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.registration_number_type_registration
			(tenant_id, country_code, type_code, revision,
			 type_name, layer, format_pattern, basis_ref,
			 effective_from, deactivated_at, deactivation_basis, content_digest, snapshot)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT DO NOTHING`,
		document.Tenant, document.Country, document.TypeCode, document.Revision,
		document.TypeName, document.Layer, document.FormatPattern, document.Basis,
		document.EffectiveFrom, document.DeactivatedAt, deactivationBasis, contentDigest, raw,
	)
	if err != nil {
		return ports.RegistrationNumberTypeSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return ports.RegistrationNumberTypeSaved, nil
	}

	var existingDigest string
	err = executor.QueryRow(ctx,
		`SELECT content_digest
		   FROM party_commercial.registration_number_type_registration
		  WHERE tenant_id = $1 AND country_code = $2 AND type_code = $3 AND revision = $4`,
		document.Tenant, document.Country, document.TypeCode, document.Revision,
	).Scan(&existingDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.RegistrationNumberTypeSaveOutcomeInvalid, fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return ports.RegistrationNumberTypeSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if existingDigest == contentDigest {
		return ports.RegistrationNumberTypeAlreadyRegistered, nil
	}
	return ports.RegistrationNumberTypeContentConflict, nil
}

func (repository *RegistrationNumberTypes) LoadLatestRegistrationNumberType(
	ctx context.Context,
	tenant domain.TenantID,
	country domain.RegistrationCountryCode,
	code domain.RegistrationNumberTypeCode,
) (domain.RegistrationNumberTypeRegistration, bool, error) {
	const operation = "load latest registration number type"
	if tenant.String() == "" || country.String() == "" || code.String() == "" {
		return domain.RegistrationNumberTypeRegistration{}, false,
			fmt.Errorf("%s: tenant, country and type code are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.RegistrationNumberTypeRegistration{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	var raw []byte
	err = querier.QueryRow(ctx,
		`SELECT snapshot
		   FROM party_commercial.registration_number_type_registration
		  WHERE tenant_id = $1 AND country_code = $2 AND type_code = $3
		  ORDER BY revision DESC
		  LIMIT 1`,
		tenant.String(), country.String(), code.String(),
	).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RegistrationNumberTypeRegistration{}, false, nil
	}
	if err != nil {
		return domain.RegistrationNumberTypeRegistration{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	registration, err := registrationNumberTypeFromSnapshot(raw)
	if err != nil {
		return domain.RegistrationNumberTypeRegistration{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return registration, true, nil
}

// LoadRegistrationNumberTypeCatalogue 取一个国家 / 地区下各类型的最新修订，折成领域目录。
// 已停用与未到生效的类型照样装进目录：判号时它们答`类型不在生效期`而不是`类型未登记`——两格
// 的续办不同，读侧先筛掉就把前者说成了后者。
func (repository *RegistrationNumberTypes) LoadRegistrationNumberTypeCatalogue(
	ctx context.Context,
	tenant domain.TenantID,
	country domain.RegistrationCountryCode,
) (domain.RegistrationNumberTypeCatalogue, error) {
	const operation = "load registration number type catalogue"
	if tenant.String() == "" || country.String() == "" {
		return domain.RegistrationNumberTypeCatalogue{},
			fmt.Errorf("%s: tenant and country are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.RegistrationNumberTypeCatalogue{}, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := querier.Query(ctx,
		`SELECT DISTINCT ON (type_code) snapshot
		   FROM party_commercial.registration_number_type_registration
		  WHERE tenant_id = $1 AND country_code = $2
		  ORDER BY type_code, revision DESC`,
		tenant.String(), country.String(),
	)
	if err != nil {
		return domain.RegistrationNumberTypeCatalogue{}, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()

	var latest []domain.RegistrationNumberTypeRegistration
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return domain.RegistrationNumberTypeCatalogue{}, fmt.Errorf("%s: %w", operation, err)
		}
		registration, err := registrationNumberTypeFromSnapshot(raw)
		if err != nil {
			return domain.RegistrationNumberTypeCatalogue{}, fmt.Errorf("%s: %w", operation, err)
		}
		latest = append(latest, registration)
	}
	if err := rows.Err(); err != nil {
		return domain.RegistrationNumberTypeCatalogue{}, fmt.Errorf("%s: %w", operation, err)
	}
	catalogue, err := domain.NewRegistrationNumberTypeCatalogue(tenant, country, latest)
	if err != nil {
		return domain.RegistrationNumberTypeCatalogue{}, fmt.Errorf("%s: %w", operation, err)
	}
	return catalogue, nil
}

// registrationNumberTypeFromSnapshot 经真构造门重建一笔修订：格式正文重新编译、层按封闭集反查、
// 停用经领域转换挂回。任何一格过不了门即坏数据，上抛不吞。
func registrationNumberTypeFromSnapshot(raw []byte) (domain.RegistrationNumberTypeRegistration, error) {
	var document registrationNumberTypeDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return domain.RegistrationNumberTypeRegistration{}, fmt.Errorf("快照不是本适配器写下的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.Tenant)
	if err != nil {
		return domain.RegistrationNumberTypeRegistration{}, err
	}
	country, err := domain.NewRegistrationCountryCode(document.Country)
	if err != nil {
		return domain.RegistrationNumberTypeRegistration{}, err
	}
	code, err := domain.NewRegistrationNumberTypeCode(document.TypeCode)
	if err != nil {
		return domain.RegistrationNumberTypeRegistration{}, err
	}
	name, err := domain.NewRegistrationNumberTypeName(document.TypeName)
	if err != nil {
		return domain.RegistrationNumberTypeRegistration{}, err
	}
	layer, known := domain.RegistrationNumberLayerNamed(document.Layer)
	if !known {
		return domain.RegistrationNumberTypeRegistration{}, fmt.Errorf("快照里的层 %q 不在封闭集内", document.Layer)
	}
	format, err := domain.NewRegistrationNumberFormat(document.FormatPattern)
	if err != nil {
		return domain.RegistrationNumberTypeRegistration{}, err
	}
	basis, err := domain.NewRegistrationNumberTypeBasisReference(document.Basis)
	if err != nil {
		return domain.RegistrationNumberTypeRegistration{}, err
	}
	lifecycle, err := domain.NewRegistrationNumberTypeLifecycle(document.EffectiveFrom)
	if err != nil {
		return domain.RegistrationNumberTypeRegistration{}, err
	}
	if document.DeactivatedAt != nil {
		deactivationBasis, err := domain.NewRegistrationNumberTypeBasisReference(document.DeactivationBasis)
		if err != nil {
			return domain.RegistrationNumberTypeRegistration{}, err
		}
		lifecycle, err = lifecycle.Deactivate(deactivationBasis, *document.DeactivatedAt)
		if err != nil {
			return domain.RegistrationNumberTypeRegistration{}, err
		}
	}
	return domain.NewRegistrationNumberTypeRegistration(tenant, country, code, document.Revision,
		domain.RegistrationNumberTypeSpec{
			Name:   name,
			Layer:  layer,
			Format: format,
			Basis:  basis,
		}, lifecycle)
}
