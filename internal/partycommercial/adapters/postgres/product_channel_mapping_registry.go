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

// ProductChannelMappings 实现 ports.ProductChannelMappingRegistry：产品—渠道映射
// 登记册的只增登记与最新修订装载（0016 迁移）。撞键不覆盖：同内容重放，异内容冲突
// （ADR-0031，判据与 PartyIdentityRegistrations 同款）。
type ProductChannelMappings struct {
	db *bentopg.DB
}

func NewProductChannelMappings(db *bentopg.DB) (*ProductChannelMappings, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &ProductChannelMappings{db: db}, nil
}

var _ ports.ProductChannelMappingRegistry = (*ProductChannelMappings)(nil)

// productChannelMappingDocument 是登记快照的形状。Channels 永远是非 nil 数组——
// 空数组即显式“未配置”绑定（0016 表注：进得来全靠领域门的显式声明），marshal 成
// JSON null 会过不了库上的 array CHECK，那正是要的：nil 与空在这里必须不可混。
type productChannelMappingDocument struct {
	Tenant              string     `json:"tenant"`
	MappingID           string     `json:"mappingId"`
	Revision            int        `json:"revision"`
	ProductObjectID     string     `json:"productObjectId"`
	ProductVersionLabel string     `json:"productVersionLabel"`
	Channels            []string   `json:"channels"`
	Basis               string     `json:"basis"`
	StartsAt            time.Time  `json:"startsAt"`
	EndsAt              *time.Time `json:"endsAt,omitempty"`
}

func documentOfMapping(registration domain.ProductChannelMappingRegistration) productChannelMappingDocument {
	binding := registration.Binding().Channels()
	channels := make([]string, 0, len(binding))
	for _, channel := range binding {
		channels = append(channels, channel.String())
	}
	document := productChannelMappingDocument{
		Tenant:              registration.Tenant().String(),
		MappingID:           registration.ID().String(),
		Revision:            registration.Revision(),
		ProductObjectID:     registration.Product().String(),
		ProductVersionLabel: registration.ProductVersion().String(),
		Channels:            channels,
		Basis:               registration.Basis().String(),
		StartsAt:            registration.Effective().StartsAt().UTC(),
	}
	if endsAt, bounded := registration.Effective().EndsAt(); bounded {
		utc := endsAt.UTC()
		document.EndsAt = &utc
	}
	return document
}

func (repository *ProductChannelMappings) SaveMapping(
	ctx context.Context,
	registration domain.ProductChannelMappingRegistration,
) (ports.MappingSaveOutcome, error) {
	const operation = "save product channel mapping"
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.MappingSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}

	document := documentOfMapping(registration)
	raw, err := json.Marshal(document)
	if err != nil {
		return ports.MappingSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	channelRefs, err := json.Marshal(document.Channels)
	if err != nil {
		return ports.MappingSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	digest := sha256.Sum256(raw)
	contentDigest := hex.EncodeToString(digest[:])

	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.product_channel_mapping_registration
			(tenant_id, mapping_id, revision,
			 product_object_id, product_version_label, channel_refs, basis_ref,
			 effective_starts_at, effective_ends_at, content_digest, snapshot)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT DO NOTHING`,
		document.Tenant, document.MappingID, document.Revision,
		document.ProductObjectID, document.ProductVersionLabel, channelRefs, document.Basis,
		document.StartsAt, document.EndsAt, contentDigest, raw,
	)
	if err != nil {
		return ports.MappingSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return ports.MappingSaved, nil
	}

	var existingDigest string
	err = executor.QueryRow(ctx,
		`SELECT content_digest
		   FROM party_commercial.product_channel_mapping_registration
		  WHERE tenant_id = $1 AND mapping_id = $2 AND revision = $3`,
		document.Tenant, document.MappingID, document.Revision,
	).Scan(&existingDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.MappingSaveOutcomeInvalid, fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return ports.MappingSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if existingDigest == contentDigest {
		return ports.MappingAlreadyRegistered, nil
	}
	return ports.MappingContentConflict, nil
}

func (repository *ProductChannelMappings) LoadLatestMapping(
	ctx context.Context,
	tenant domain.TenantID,
	mapping domain.ProductChannelMappingID,
) (domain.ProductChannelMappingRegistration, bool, error) {
	const operation = "load latest product channel mapping"
	if tenant.String() == "" || mapping.String() == "" {
		return domain.ProductChannelMappingRegistration{}, false,
			fmt.Errorf("%s: tenant and identifier are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ProductChannelMappingRegistration{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	var raw []byte
	err = querier.QueryRow(ctx,
		`SELECT snapshot
		   FROM party_commercial.product_channel_mapping_registration
		  WHERE tenant_id = $1 AND mapping_id = $2
		  ORDER BY revision DESC
		  LIMIT 1`,
		tenant.String(), mapping.String(),
	).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ProductChannelMappingRegistration{}, false, nil
	}
	if err != nil {
		return domain.ProductChannelMappingRegistration{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	registration, err := mappingFromSnapshot(raw)
	if err != nil {
		return domain.ProductChannelMappingRegistration{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return registration, true, nil
}

func mappingFromSnapshot(raw []byte) (domain.ProductChannelMappingRegistration, error) {
	var document productChannelMappingDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return domain.ProductChannelMappingRegistration{}, fmt.Errorf("快照不是本适配器写下的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.Tenant)
	if err != nil {
		return domain.ProductChannelMappingRegistration{}, err
	}
	id, err := domain.NewProductChannelMappingID(document.MappingID)
	if err != nil {
		return domain.ProductChannelMappingRegistration{}, err
	}
	product, err := domain.NewCommercialObjectID(document.ProductObjectID)
	if err != nil {
		return domain.ProductChannelMappingRegistration{}, err
	}
	productVersion, err := domain.NewCommercialVersionLabel(document.ProductVersionLabel)
	if err != nil {
		return domain.ProductChannelMappingRegistration{}, err
	}
	basis, err := domain.NewMappingBasisReference(document.Basis)
	if err != nil {
		return domain.ProductChannelMappingRegistration{}, err
	}
	endsAt := time.Time{}
	if document.EndsAt != nil {
		endsAt = *document.EndsAt
	}
	effective, err := domain.NewEffectiveInterval(document.StartsAt, endsAt)
	if err != nil {
		return domain.ProductChannelMappingRegistration{}, err
	}

	// 空数组重建成显式“未配置”：写入只有 UnconfiguredChannelBinding 一条路能落下
	// 空数组，读回走同一格；非空经真构造门重建，重复或空白引用即坏数据，上抛不吞。
	binding := domain.UnconfiguredChannelBinding()
	if len(document.Channels) > 0 {
		channels := make([]domain.ChannelProductReference, 0, len(document.Channels))
		for _, value := range document.Channels {
			channel, err := domain.NewChannelProductReference(value)
			if err != nil {
				return domain.ProductChannelMappingRegistration{}, err
			}
			channels = append(channels, channel)
		}
		binding, err = domain.NewConfiguredChannelBinding(channels)
		if err != nil {
			return domain.ProductChannelMappingRegistration{}, err
		}
	}

	return domain.NewProductChannelMappingRegistration(tenant, id, document.Revision,
		domain.ProductChannelMappingSpec{
			Product:        product,
			ProductVersion: productVersion,
			Binding:        binding,
			Effective:      effective,
			Basis:          basis,
		})
}
