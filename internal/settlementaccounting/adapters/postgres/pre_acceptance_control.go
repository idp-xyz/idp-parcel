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
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// PreAcceptanceControlScopeLocks 实现 ports.PreAcceptanceControlScopeLock。
type PreAcceptanceControlScopeLocks struct {
	db *bentopg.DB
}

func NewPreAcceptanceControlScopeLocks(db *bentopg.DB) (*PreAcceptanceControlScopeLocks, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &PreAcceptanceControlScopeLocks{db: db}, nil
}

var _ ports.PreAcceptanceControlScopeLock = (*PreAcceptanceControlScopeLocks)(nil)

// LockControlScope 取（租户，结算作用域）的事务级咨询锁，事务结束即放。走 RequireExecutor：在连接池上取，锁随那条语句的自动提交
// 当场放掉，挡不住任何一路。不对账本行 FOR UPDATE：一笔新冻结或新暴露还没有行可锁，两路首次占用照样各自判够。
//
// 键照本仓其余事务级锁的双 int 形式：第一键是锁空间名的散列，第二键是四维以 chr(31) 拼接后的散列；撞上只让两个不相干的作用域多等
// 一会儿。同一事务里再取一次是重入，组合策略在两本账上各取一次不会自己等自己。
func (locks *PreAcceptanceControlScopeLocks) LockControlScope(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
) error {
	executor, err := locks.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("lock pre-acceptance control scope: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('settlement_accounting.pre_acceptance_control.scope'),
		        hashtext($1 || chr(31) || $2 || chr(31) || $3 || chr(31) || $4))`,
		tenant.String(),
		scope.LegalEntity().String(),
		scope.Account().String(),
		scope.Currency().String(),
	); err != nil {
		return fmt.Errorf("lock pre-acceptance control scope: %w", err)
	}
	return nil
}

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
// UPSERT——只增不删由行模型保证：键上 DO UPDATE 只把占着推进到已释放，原金额与原
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
		        request_digest, frozen_at, released_at, converted_minor, converted_at
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
		var releasedAt, convertedAt *time.Time
		var convertedMinor int64
		if err := rows.Scan(&requestID, &freezeID, &status, &amountMinor,
			&association, &digest, &frozenAt, &releasedAt, &convertedMinor, &convertedAt); err != nil {
			return nil, fmt.Errorf("load freeze ledger: %w", err)
		}
		decoded, err := decodeDigest(digest)
		if err != nil {
			return nil, fmt.Errorf("load freeze ledger: %w", err)
		}
		spec, err := freezeSpecOf(requestID, freezeID, status, amountMinor,
			association, decoded, frozenAt, releasedAt, scope)
		if err == nil {
			spec.ConvertedMinor = convertedMinor
			if convertedAt != nil {
				spec.ConvertedAt = *convertedAt
			}
		}
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
//
// DO UPDATE 只放行「库里占着、这一册已释放」那一种推进，别的行撞上键就原样留着、一行不写。整册里绝大多数行这一次并没有变，
// 无条件 SET 就是拿读到的快照覆盖库里的现状：快照读得比库旧，库里已释放的那一行会被改回占着、释放时间清空，钱被静默占回去。
// 编排在作用域锁下读写，这里不靠它，绕过编排的写入同样压不回一次释放。
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
		var releasedAt, convertedAt *time.Time
		if freeze.Status() == domain.FreezeReleased {
			at := freeze.ReleasedAt().UTC()
			releasedAt = &at
		}
		if !freeze.ConvertedAt().IsZero() {
			at := freeze.ConvertedAt().UTC()
			convertedAt = &at
		}
		if _, err := executor.Exec(ctx,
			`INSERT INTO settlement_accounting.funds_freeze
				(tenant_id, legal_entity, account_id, currency, control_request_id,
				 freeze_id, status, amount_minor, association, request_digest,
				 frozen_at, released_at, converted_minor, converted_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $15, $16)
			 ON CONFLICT ON CONSTRAINT funds_freeze_pkey DO UPDATE
			    SET status = EXCLUDED.status,
			        released_at = EXCLUDED.released_at,
			        converted_minor = EXCLUDED.converted_minor,
			        converted_at = EXCLUDED.converted_at,
			        saved_at = now()
			  WHERE funds_freeze.status = $13
			    AND EXCLUDED.converted_minor >= funds_freeze.converted_minor
			    AND (EXCLUDED.status = $14
			         OR EXCLUDED.converted_minor > funds_freeze.converted_minor)`,
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
			uint8(domain.FreezeHeld),
			uint8(domain.FreezeReleased),
			freeze.ConvertedMinor(),
			convertedAt,
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
		        request_digest, exposed_at, released_at, credit_policy_ref,
		        converted_minor, converted_at
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
		var releasedAt, convertedAt *time.Time
		var policyRef *string
		var convertedMinor int64
		if err := rows.Scan(&requestID, &exposureID, &status, &amountMinor,
			&association, &digest, &exposedAt, &releasedAt, &policyRef,
			&convertedMinor, &convertedAt); err != nil {
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

			ConvertedMinor: convertedMinor,
		}
		if releasedAt != nil {
			spec.ReleasedAt = *releasedAt
		}
		if convertedAt != nil {
			spec.ConvertedAt = *convertedAt
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
//
// DO UPDATE 只放行「库里已记、这一册已释放」那一种推进，理由同冻结账本：旧快照写回压不回库里的释放。
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
		var releasedAt, convertedAt *time.Time
		if exposure.Status() == domain.ExposureReleased {
			at := exposure.ReleasedAt().UTC()
			releasedAt = &at
		}
		if !exposure.ConvertedAt().IsZero() {
			at := exposure.ConvertedAt().UTC()
			convertedAt = &at
		}
		// 零值出处只属于重建回来的存量行，原样写回 NULL；新形成的暴露一律带出处（Expose 不收没换上
		// 授权额度的状况，领域门保证）。
		var policyRef *string
		if !exposure.Policy().IsZero() {
			reference := exposure.Policy().String()
			policyRef = &reference
		}
		if _, err := executor.Exec(ctx,
			`INSERT INTO settlement_accounting.credit_exposure
				(tenant_id, legal_entity, account_id, currency, control_request_id,
				 exposure_id, status, amount_minor, association, request_digest,
				 exposed_at, released_at, credit_policy_ref, converted_minor, converted_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $16, $17)
			 ON CONFLICT ON CONSTRAINT credit_exposure_pkey DO UPDATE
			    SET status = EXCLUDED.status,
			        released_at = EXCLUDED.released_at,
			        converted_minor = EXCLUDED.converted_minor,
			        converted_at = EXCLUDED.converted_at,
			        saved_at = now()
			  WHERE credit_exposure.status = $14
			    AND EXCLUDED.converted_minor >= credit_exposure.converted_minor
			    AND (EXCLUDED.status = $15
			         OR EXCLUDED.converted_minor > credit_exposure.converted_minor)`,
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
			uint8(domain.ExposureRecorded),
			uint8(domain.ExposureReleased),
			exposure.ConvertedMinor(),
			convertedAt,
		); err != nil {
			return fmt.Errorf("save credit exposure ledger: %w", err)
		}
	}
	return nil
}
