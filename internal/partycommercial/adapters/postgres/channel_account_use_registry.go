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

// ChannelAccountUseAuthorizations 实现 ports.ChannelAccountUseAuthorizationRegistry：渠道
// 账号使用授权登记册的只增登记与装载（0018 迁移、ADR-0093）。撞键不覆盖：同内容重放，
// 异内容冲突（ADR-0031，判据与 ProductChannelMappings 同款）。
type ChannelAccountUseAuthorizations struct {
	db *bentopg.DB
}

func NewChannelAccountUseAuthorizations(db *bentopg.DB) (*ChannelAccountUseAuthorizations, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &ChannelAccountUseAuthorizations{db: db}, nil
}

var _ ports.ChannelAccountUseAuthorizationRegistry = (*ChannelAccountUseAuthorizations)(nil)

// channelAccountUseDocument 是登记快照的形状。
//
// 技术存续与业务存续两个发布门入参不在里面：聚合不留它们（ADR-0039 决定四），存一份等于
// 把一个本上下文声明过不拥有的事实抄进册子，而没有任何东西会让它跟着现实更新。
type channelAccountUseDocument struct {
	Tenant           string     `json:"tenant"`
	AuthorizationID  string     `json:"authorizationId"`
	Revision         int        `json:"revision"`
	ChannelAccountID string     `json:"channelAccountId"`
	Grantor          string     `json:"grantor"`
	Grantee          string     `json:"grantee"`
	Channel          string     `json:"channel"`
	Scope            string     `json:"scope"`
	StartsAt         time.Time  `json:"startsAt"`
	EndsAt           *time.Time `json:"endsAt,omitempty"`
	Status           string     `json:"status"`
	PublishedAt      time.Time  `json:"publishedAt"`
	RevokedAt        *time.Time `json:"revokedAt,omitempty"`
	RevokedOn        string     `json:"revokedOn,omitempty"`
}

func documentOfChannelAccountUse(
	registration domain.ChannelAccountUseAuthorizationRegistration,
) channelAccountUseDocument {
	authorization := registration.Authorization()
	document := channelAccountUseDocument{
		Tenant:           registration.Tenant().String(),
		AuthorizationID:  registration.ID().String(),
		Revision:         registration.Revision(),
		ChannelAccountID: authorization.Account().String(),
		Grantor:          authorization.Grantor().String(),
		Grantee:          authorization.Grantee().String(),
		Channel:          authorization.Channel().String(),
		Scope:            authorization.Scope().String(),
		StartsAt:         authorization.Effective().StartsAt().UTC(),
		Status:           authorization.Status().String(),
		PublishedAt:      authorization.PublishedAt().UTC(),
	}
	if endsAt, bounded := authorization.Effective().EndsAt(); bounded {
		utc := endsAt.UTC()
		document.EndsAt = &utc
	}
	if revokedAt, revoked := authorization.RevokedAt(); revoked {
		utc := revokedAt.UTC()
		document.RevokedAt = &utc
		document.RevokedOn = authorization.RevokedOn().String()
	}
	return document
}

func (repository *ChannelAccountUseAuthorizations) SaveChannelAccountUse(
	ctx context.Context,
	registration domain.ChannelAccountUseAuthorizationRegistration,
) (ports.ChannelAccountUseSaveOutcome, error) {
	const operation = "save channel account use authorization"
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ChannelAccountUseSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}

	document := documentOfChannelAccountUse(registration)
	raw, err := json.Marshal(document)
	if err != nil {
		return ports.ChannelAccountUseSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	digest := sha256.Sum256(raw)
	contentDigest := hex.EncodeToString(digest[:])

	var revokedOn *string
	if document.RevokedOn != "" {
		value := document.RevokedOn
		revokedOn = &value
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.channel_account_use_authorization
			(tenant_id, authorization_id, revision,
			 channel_account_id, grantor_party_id, grantee_party_id, channel_ref, scope_ref,
			 effective_starts_at, effective_ends_at,
			 status, published_at, revoked_at, revoked_on,
			 content_digest, snapshot)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		 ON CONFLICT DO NOTHING`,
		document.Tenant, document.AuthorizationID, document.Revision,
		document.ChannelAccountID, document.Grantor, document.Grantee, document.Channel, document.Scope,
		document.StartsAt, document.EndsAt,
		document.Status, document.PublishedAt, document.RevokedAt, revokedOn,
		contentDigest, raw,
	)
	if err != nil {
		return ports.ChannelAccountUseSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return ports.ChannelAccountUseSaved, nil
	}

	var existingDigest string
	err = executor.QueryRow(ctx,
		`SELECT content_digest
		   FROM party_commercial.channel_account_use_authorization
		  WHERE tenant_id = $1 AND authorization_id = $2 AND revision = $3`,
		document.Tenant, document.AuthorizationID, document.Revision,
	).Scan(&existingDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ChannelAccountUseSaveOutcomeInvalid, fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return ports.ChannelAccountUseSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if existingDigest == contentDigest {
		return ports.ChannelAccountUseAlreadyRegistered, nil
	}
	return ports.ChannelAccountUseContentConflict, nil
}

func (repository *ChannelAccountUseAuthorizations) LoadLatest(
	ctx context.Context,
	tenant domain.TenantID,
	authorization domain.ChannelAccountUseAuthorizationID,
) (domain.ChannelAccountUseAuthorizationRegistration, bool, error) {
	const operation = "load latest channel account use authorization"
	if tenant.String() == "" || authorization.String() == "" {
		return domain.ChannelAccountUseAuthorizationRegistration{}, false,
			fmt.Errorf("%s: tenant and identifier are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ChannelAccountUseAuthorizationRegistration{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	var raw []byte
	err = querier.QueryRow(ctx,
		`SELECT snapshot
		   FROM party_commercial.channel_account_use_authorization
		  WHERE tenant_id = $1 AND authorization_id = $2
		  ORDER BY revision DESC
		  LIMIT 1`,
		tenant.String(), authorization.String(),
	).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ChannelAccountUseAuthorizationRegistration{}, false, nil
	}
	if err != nil {
		return domain.ChannelAccountUseAuthorizationRegistration{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	registration, err := channelAccountUseFromSnapshot(raw)
	if err != nil {
		return domain.ChannelAccountUseAuthorizationRegistration{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return registration, true, nil
}

// LoadAuthorizedAccountUse 按渠道账号取该账号名下**每一笔**授权的最新修订。
//
// 按登记标识分组再各取最新，而不是整体取最新一行：同一个账号可以先后授给不同的被授权人，
// 取一行会让其中一笔在册上消失，而调用方正是要拿它们逐一对时点判 AllowsUseAt。
// 判「此刻许不许用」不在这里做——那是一份会过期的推导，固化进读口就等于替调用方答了。
func (repository *ChannelAccountUseAuthorizations) LoadAuthorizedAccountUse(
	ctx context.Context,
	tenant domain.TenantID,
	account domain.ChannelAccountID,
) ([]domain.ChannelAccountUseAuthorizationRegistration, error) {
	const operation = "load channel account use authorizations by account"
	if tenant.String() == "" || account.String() == "" {
		return nil, fmt.Errorf("%s: tenant and account are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := querier.Query(ctx,
		`SELECT DISTINCT ON (authorization_id) snapshot
		   FROM party_commercial.channel_account_use_authorization
		  WHERE tenant_id = $1 AND channel_account_id = $2
		  ORDER BY authorization_id, revision DESC`,
		tenant.String(), account.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()

	registrations := make([]domain.ChannelAccountUseAuthorizationRegistration, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		registration, err := channelAccountUseFromSnapshot(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		registrations = append(registrations, registration)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return registrations, nil
}

func channelAccountUseFromSnapshot(raw []byte) (domain.ChannelAccountUseAuthorizationRegistration, error) {
	var document channelAccountUseDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return domain.ChannelAccountUseAuthorizationRegistration{},
			fmt.Errorf("快照不是本适配器写下的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.Tenant)
	if err != nil {
		return domain.ChannelAccountUseAuthorizationRegistration{}, err
	}
	id, err := domain.NewChannelAccountUseAuthorizationID(document.AuthorizationID)
	if err != nil {
		return domain.ChannelAccountUseAuthorizationRegistration{}, err
	}
	account, err := domain.NewChannelAccountID(document.ChannelAccountID)
	if err != nil {
		return domain.ChannelAccountUseAuthorizationRegistration{}, err
	}
	grantor, err := domain.NewPartyID(document.Grantor)
	if err != nil {
		return domain.ChannelAccountUseAuthorizationRegistration{}, err
	}
	grantee, err := domain.NewPartyID(document.Grantee)
	if err != nil {
		return domain.ChannelAccountUseAuthorizationRegistration{}, err
	}
	channel, err := domain.NewChannelProductReference(document.Channel)
	if err != nil {
		return domain.ChannelAccountUseAuthorizationRegistration{}, err
	}
	scope, err := domain.NewCommercialScopeReference(document.Scope)
	if err != nil {
		return domain.ChannelAccountUseAuthorizationRegistration{}, err
	}
	endsAt := time.Time{}
	if document.EndsAt != nil {
		endsAt = *document.EndsAt
	}
	effective, err := domain.NewEffectiveInterval(document.StartsAt, endsAt)
	if err != nil {
		return domain.ChannelAccountUseAuthorizationRegistration{}, err
	}

	revokedAt := time.Time{}
	revokedOn := domain.ChannelAccountRevocationBasisReference{}
	if document.RevokedAt != nil {
		revokedAt = *document.RevokedAt
		revokedOn, err = domain.NewChannelAccountRevocationBasisReference(document.RevokedOn)
		if err != nil {
			return domain.ChannelAccountUseAuthorizationRegistration{}, err
		}
	}

	authorization, err := domain.RehydrateChannelAccountUseAuthorization(
		account, grantor, grantee, channel, scope, effective,
		document.PublishedAt, revokedAt, revokedOn,
	)
	if err != nil {
		return domain.ChannelAccountUseAuthorizationRegistration{}, err
	}
	return domain.NewChannelAccountUseAuthorizationRegistration(
		tenant, id, document.Revision, authorization,
	)
}
