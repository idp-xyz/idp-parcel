// Package postgres 是 settlement-accounting 自有语义端口的 PostgreSQL 适配器。
//
// 显式 SQL、行模型与状态推进都留在这里，不进领域对象。所有语句显式携带租户与结算
// 作用域条件：作用域不是过滤器而是身份的一部分（与 parcel-shipment 侧同一条纪律）。
package postgres

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 幂等指纹是账本用 NUL 分隔符拼出的不透明字符串，而 PostgreSQL 的 text 存不了
// `0x00`。适配器以十六进制携带它：编码保持等值关系（同指纹同编码），账本拿回的
// 与当初记下的一字不差——绝不在这里改指纹的拼法，那是账本自己的判定口径。
func encodeDigest(digest string) string {
	return hex.EncodeToString([]byte(digest))
}

func decodeDigest(encoded string) (string, error) {
	raw, err := hex.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("幂等指纹不是本适配器写下的形状：%w", err)
	}
	return string(raw), nil
}

// FreezeLedgers 实现 ports.FreezeLedgerRepository：整册取回、整册保存。
//
// 一行一条冻结：LoadForScope 读全作用域重建账本（幂等与冲突判定要整册），Save 逐条
// UPSERT——只增不删由行模型保证：键上 DO UPDATE 只推进状态与释放时间，原金额与原
// 冻结时间不在 SET 里，写不动。
type FreezeLedgers struct {
	db *bentopg.DB
}

func NewFreezeLedgers(db *bentopg.DB) (*FreezeLedgers, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &FreezeLedgers{db: db}, nil
}

// LoadForScope 按租户+结算作用域读回整册。空册交回新账本而不是错误：这个作用域还没
// 冻结过任何东西是常态。
func (repository *FreezeLedgers) LoadForScope(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
) (*domain.FreezeLedger, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("load freeze ledger: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT control_request_id, freeze_id, status, amount_minor, association,
		        request_digest, frozen_at, released_at
		   FROM settlement_accounting.funds_freeze
		  WHERE tenant_id = $1
		    AND legal_entity = $2
		    AND account_id = $3
		    AND currency = $4
		  ORDER BY freeze_id`,
		tenant.String(),
		scope.LegalEntity().String(),
		scope.Account().String(),
		scope.Currency().String(),
	)
	if err != nil {
		return nil, fmt.Errorf("load freeze ledger: %w", err)
	}
	defer rows.Close()

	specs := make([]domain.RehydrateFundsFreezeSpec, 0)
	for rows.Next() {
		var requestID, freezeID, association, digest string
		var status uint8
		var amountMinor int64
		var frozenAt time.Time
		var releasedAt *time.Time
		if err := rows.Scan(&requestID, &freezeID, &status, &amountMinor,
			&association, &digest, &frozenAt, &releasedAt); err != nil {
			return nil, fmt.Errorf("load freeze ledger: %w", err)
		}
		decoded, err := decodeDigest(digest)
		if err != nil {
			return nil, fmt.Errorf("load freeze ledger: %w", err)
		}
		spec, err := freezeSpecOf(requestID, freezeID, status, amountMinor,
			association, decoded, frozenAt, releasedAt, scope)
		if err != nil {
			return nil, fmt.Errorf("load freeze ledger: %w", err)
		}
		specs = append(specs, spec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load freeze ledger: %w", err)
	}

	ledger, err := domain.RehydrateFreezeLedger(specs)
	if err != nil {
		return nil, fmt.Errorf("load freeze ledger: %w", err)
	}
	return ledger, nil
}

// Save 把整册写回。UPSERT 的 SET 只有状态与释放时间——冻结不可改写，释放是唯一的
// 状态推进；重复保存同一册是幂等。
func (repository *FreezeLedgers) Save(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
	ledger *domain.FreezeLedger,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save freeze ledger: %w", err)
	}

	for _, freeze := range ledger.Entries() {
		digest, found := ledger.DigestFor(freeze.RequestID())
		if !found {
			// 账本自己的不变量：入册的冻结必有指纹。走到这里是领域或适配器的 bug。
			return fmt.Errorf("save freeze ledger: 冻结 %s 没有幂等指纹", freeze.FreezeID())
		}
		var releasedAt *time.Time
		if freeze.Status() == domain.FreezeReleased {
			at := freeze.ReleasedAt().UTC()
			releasedAt = &at
		}
		if _, err := executor.Exec(ctx,
			`INSERT INTO settlement_accounting.funds_freeze
				(tenant_id, legal_entity, account_id, currency, control_request_id,
				 freeze_id, status, amount_minor, association, request_digest,
				 frozen_at, released_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			 ON CONFLICT ON CONSTRAINT funds_freeze_pkey DO UPDATE
			    SET status = EXCLUDED.status,
			        released_at = EXCLUDED.released_at,
			        saved_at = now()`,
			tenant.String(),
			scope.LegalEntity().String(),
			scope.Account().String(),
			scope.Currency().String(),
			freeze.RequestID().String(),
			freeze.FreezeID().String(),
			uint8(freeze.Status()),
			freeze.AmountMinor(),
			freeze.Association().String(),
			encodeDigest(digest),
			freeze.FrozenAt().UTC(),
			releasedAt,
		); err != nil {
			return fmt.Errorf("save freeze ledger: %w", err)
		}
	}
	return nil
}

func freezeSpecOf(
	requestID, freezeID string,
	status uint8,
	amountMinor int64,
	association, digest string,
	frozenAt time.Time,
	releasedAt *time.Time,
	scope domain.SettlementScope,
) (domain.RehydrateFundsFreezeSpec, error) {
	controlRequest, err := domain.NewControlRequestID(requestID)
	if err != nil {
		return domain.RehydrateFundsFreezeSpec{}, err
	}
	businessAssociation, err := domain.NewBusinessAssociationReference(association)
	if err != nil {
		return domain.RehydrateFundsFreezeSpec{}, err
	}
	spec := domain.RehydrateFundsFreezeSpec{
		FreezeID:    freezeID,
		RequestID:   controlRequest,
		Scope:       scope,
		AmountMinor: amountMinor,
		Association: businessAssociation,
		Status:      domain.FreezeStatus(status),
		FrozenAt:    frozenAt,
		Digest:      digest,
	}
	if releasedAt != nil {
		spec.ReleasedAt = *releasedAt
	}
	return spec, nil
}

// CreditExposureLedgers 实现 ports.CreditExposureLedgerRepository。形状与冻结账本
// 一致，但它是另一本账（ADR-0047 两轨）：表、类型与重建入口都分立，不共用实现——
// 共用会让两本账在实现里合流，而那正是 SET-03 要拦的事。
type CreditExposureLedgers struct {
	db *bentopg.DB
}

func NewCreditExposureLedgers(db *bentopg.DB) (*CreditExposureLedgers, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &CreditExposureLedgers{db: db}, nil
}

// LoadForScope 按租户+结算作用域读回整册暴露。
func (repository *CreditExposureLedgers) LoadForScope(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
) (*domain.CreditExposureLedger, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("load credit exposure ledger: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT control_request_id, exposure_id, status, amount_minor, association,
		        request_digest, exposed_at, released_at, credit_policy_ref
		   FROM settlement_accounting.credit_exposure
		  WHERE tenant_id = $1
		    AND legal_entity = $2
		    AND account_id = $3
		    AND currency = $4
		  ORDER BY exposure_id`,
		tenant.String(),
		scope.LegalEntity().String(),
		scope.Account().String(),
		scope.Currency().String(),
	)
	if err != nil {
		return nil, fmt.Errorf("load credit exposure ledger: %w", err)
	}
	defer rows.Close()

	specs := make([]domain.RehydrateCreditExposureSpec, 0)
	for rows.Next() {
		var requestID, exposureID, association, digest string
		var status uint8
		var amountMinor int64
		var exposedAt time.Time
		var releasedAt *time.Time
		var policyRef *string
		if err := rows.Scan(&requestID, &exposureID, &status, &amountMinor,
			&association, &digest, &exposedAt, &releasedAt, &policyRef); err != nil {
			return nil, fmt.Errorf("load credit exposure ledger: %w", err)
		}
		controlRequest, err := domain.NewControlRequestID(requestID)
		if err != nil {
			return nil, fmt.Errorf("load credit exposure ledger: %w", err)
		}
		businessAssociation, err := domain.NewBusinessAssociationReference(association)
		if err != nil {
			return nil, fmt.Errorf("load credit exposure ledger: %w", err)
		}
		decoded, err := decodeDigest(digest)
		if err != nil {
			return nil, fmt.Errorf("load credit exposure ledger: %w", err)
		}
		spec := domain.RehydrateCreditExposureSpec{
			ExposureID:  exposureID,
			RequestID:   controlRequest,
			Scope:       scope,
			AmountMinor: amountMinor,
			Association: businessAssociation,
			Status:      domain.ExposureStatus(status),
			ExposedAt:   exposedAt,
			Digest:      decoded,
		}
		if releasedAt != nil {
			spec.ReleasedAt = *releasedAt
		}
		if policyRef != nil {
			// NULL 是政策引用列落地之前的存量行，如实读回为零值；非 NULL 的值 CHECK 已保证非空白，
			// 构造门在这里拒的只能是列被绕过 CHECK 改写——那是库里那一行的问题，上抛。
			policy, err := domain.NewCreditPolicyReference(*policyRef)
			if err != nil {
				return nil, fmt.Errorf("load credit exposure ledger: %w", err)
			}
			spec.Policy = policy
		}
		specs = append(specs, spec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load credit exposure ledger: %w", err)
	}

	ledger, err := domain.RehydrateCreditExposureLedger(specs)
	if err != nil {
		return nil, fmt.Errorf("load credit exposure ledger: %w", err)
	}
	return ledger, nil
}

// Save 把整册暴露写回。SET 只有状态与释放时间，纪律同冻结账本：政策引用与金额、暴露时间一样是
// 形成时的事实，不在 SET 里，写不动——存量行释放时也不会被补上一个它当初没有的出处。
func (repository *CreditExposureLedgers) Save(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
	ledger *domain.CreditExposureLedger,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save credit exposure ledger: %w", err)
	}

	for _, exposure := range ledger.Entries() {
		digest, found := ledger.DigestFor(exposure.RequestID())
		if !found {
			return fmt.Errorf("save credit exposure ledger: 暴露 %s 没有幂等指纹", exposure.ExposureID())
		}
		var releasedAt *time.Time
		if exposure.Status() == domain.ExposureReleased {
			at := exposure.ReleasedAt().UTC()
			releasedAt = &at
		}
		// 零值出处只属于重建回来的存量行，原样写回 NULL；新形成的暴露一律带出处（Expose 不收没换上
		// 授权额度的状况，领域门保证）。
		var policyRef *string
		if reference := exposure.Policy().String(); reference != "" {
			policyRef = &reference
		}
		if _, err := executor.Exec(ctx,
			`INSERT INTO settlement_accounting.credit_exposure
				(tenant_id, legal_entity, account_id, currency, control_request_id,
				 exposure_id, status, amount_minor, association, request_digest,
				 exposed_at, released_at, credit_policy_ref)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			 ON CONFLICT ON CONSTRAINT credit_exposure_pkey DO UPDATE
			    SET status = EXCLUDED.status,
			        released_at = EXCLUDED.released_at,
			        saved_at = now()`,
			tenant.String(),
			scope.LegalEntity().String(),
			scope.Account().String(),
			scope.Currency().String(),
			exposure.RequestID().String(),
			exposure.ExposureID().String(),
			uint8(exposure.Status()),
			exposure.AmountMinor(),
			exposure.Association().String(),
			encodeDigest(digest),
			exposure.ExposedAt().UTC(),
			releasedAt,
			policyRef,
		); err != nil {
			return fmt.Errorf("save credit exposure ledger: %w", err)
		}
	}
	return nil
}
