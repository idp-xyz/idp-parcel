// Package postgres 是 parcel-shipment 自有语义端口的 PostgreSQL 适配器。
//
// 显式 SQL、行模型与 SQLSTATE 翻译都留在这里，不进领域对象。所有语句显式携带租户
// 与客户账户条件：作用域不是过滤器而是身份的一部分，缺了它，另一个租户的同名外部
// 键就会被当成同一条记录。
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// ErrAlreadyPreserved 表示同一来源身份已经保全过。
//
// 应用编排先 FindPreserved 再 Preserve，所以它只会在并发竞态下出现：两个同键请求
// 在这两步之间交错。此时报错而不是覆盖——覆盖会毁掉先到者已保全的原始事实，而
// 「原始内容一经保全不可改写」正是来源保全这一步存在的理由。调用方重试即可读到
// 已保全记录并走重放那一支。
var ErrAlreadyPreserved = errors.New("parcel shipment postgres: source submission already preserved")

// SourceSubmissions 实现 ports.SourceSubmissionRepository。
type SourceSubmissions struct {
	db *bentopg.DB
}

func NewSourceSubmissions(db *bentopg.DB) (*SourceSubmissions, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &SourceSubmissions{db: db}, nil
}

// FindPreserved 按完整来源身份取回已保全事实。
//
// 走 ReadExecutor 而非 RequireExecutor：它在事务 context 上加入该事务、无事务时用
// 显式注入的连接池，因此同一个方法既能在来源保全事务里读到本事务刚写的行，也能被
// 事务外的查询复用。否定结果只回 false，不区分「不存在」与「属于另一个租户或客户
// 账户」——区分它们等于泄露其他作用域是否存在该对象。
func (repository *SourceSubmissions) FindPreserved(
	ctx context.Context,
	identity domain.SourceIdentity,
) (domain.SourceSubmissionFingerprint, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.SourceSubmissionFingerprint{}, false, fmt.Errorf("find preserved source: %w", err)
	}

	var digest string
	var occurredAt, receivedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT payload_digest, occurred_at, received_at
		   FROM parcel_shipment.source_submission
		  WHERE tenant_id = $1
		    AND customer_account_id = $2
		    AND source = $3
		    AND source_request_key = $4`,
		identity.TenantID().String(),
		identity.CustomerAccountID().String(),
		identity.Source().String(),
		identity.RequestKey().String(),
	).Scan(&digest, &occurredAt, &receivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SourceSubmissionFingerprint{}, false, nil
	}
	if err != nil {
		return domain.SourceSubmissionFingerprint{}, false, fmt.Errorf("find preserved source: %w", err)
	}

	payloadDigest, err := domain.NewPayloadDigest(digest)
	if err != nil {
		return domain.SourceSubmissionFingerprint{}, false, fmt.Errorf("find preserved source: %w", err)
	}
	// 经领域构造函数重建而不是直接组装：库里读回来的东西同样要过一遍不变量，
	// 否则一次坏写入会在此处变成一个看起来合法的领域对象。
	preserved, err := domain.NewSourceSubmissionFingerprint(
		identity, payloadDigest, occurredAt.UTC(), receivedAt.UTC())
	if err != nil {
		return domain.SourceSubmissionFingerprint{}, false, fmt.Errorf("find preserved source: %w", err)
	}
	return preserved, true, nil
}

// Preserve 写下不可改写的来源事实。
//
// 走 RequireExecutor：无事务时它返回 ErrTransactionRequired 而不是改用连接池。
// 这条正是框架合同要的保证——来源保全必须落在一个明确的事务里，否则它与同一步
// 里的其他写入不再同生共死。
//
// 「已保全」用 ON CONFLICT DO NOTHING 加零行判定翻译，不捕 23505——同一事务里撞
// 唯一约束会把事务打进中止态，调用方在保全之后的业务判断会被连带回滚（本仓真库
// 纪律，本文件曾是捕码译法的孤例）。零行时仍返回 ErrAlreadyPreserved：并发竞态下
// 后到者要读到的是错误而不是覆盖，语义与此前一致，事务保持可用。
func (repository *SourceSubmissions) Preserve(
	ctx context.Context,
	submission domain.SourceSubmissionFingerprint,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("preserve source: %w", err)
	}

	identity := submission.Identity()
	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_shipment.source_submission
			(tenant_id, customer_account_id, source, source_request_key,
			 payload_digest, occurred_at, received_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (tenant_id, customer_account_id, source, source_request_key) DO NOTHING`,
		identity.TenantID().String(),
		identity.CustomerAccountID().String(),
		identity.Source().String(),
		identity.RequestKey().String(),
		submission.Digest().String(),
		submission.OccurredAt().UTC(),
		submission.ReceivedAt().UTC(),
	)
	if err != nil {
		return fmt.Errorf("preserve source: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w", ErrAlreadyPreserved)
	}
	return nil
}

// AppendObservation 记录同一逻辑请求再次被观察到。
//
// 它只追加，绝不触碰已保全那一行：不同的 occurredAt/receivedAt 是新的观察，不是对
// 历史的更正。做成一行一身份再 UPSERT，第二次观察就会把第一次的时间抹掉。
func (repository *SourceSubmissions) AppendObservation(
	ctx context.Context,
	observed domain.SourceSubmissionFingerprint,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("append source observation: %w", err)
	}

	identity := observed.Identity()
	_, err = executor.Exec(ctx,
		`INSERT INTO parcel_shipment.source_submission_observation
			(tenant_id, customer_account_id, source, source_request_key,
			 payload_digest, occurred_at, received_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		identity.TenantID().String(),
		identity.CustomerAccountID().String(),
		identity.Source().String(),
		identity.RequestKey().String(),
		observed.Digest().String(),
		observed.OccurredAt().UTC(),
		observed.ReceivedAt().UTC(),
	)
	if err != nil {
		return fmt.Errorf("append source observation: %w", err)
	}
	return nil
}
