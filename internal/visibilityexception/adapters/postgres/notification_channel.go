package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// NotificationChannels 是通知渠道登记册：实现 ports.NotificationChannelDirectory，并给离线登记口一个写入方法。
type NotificationChannels struct {
	db *bentopg.DB
}

func NewNotificationChannels(db *bentopg.DB) (*NotificationChannels, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &NotificationChannels{db: db}, nil
}

var _ ports.NotificationChannelDirectory = (*NotificationChannels)(nil)

// RegisterChannel 登记或换掉一个渠道的推送端点。重登即换：改的是往后的发送。
func (repository *NotificationChannels) RegisterChannel(
	ctx context.Context,
	tenant domain.TenantID,
	channel domain.NotificationChannelEndpoint,
	registeredAt time.Time,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("register notification channel: %w", err)
	}
	if tenant.String() == "" || channel.Channel().String() == "" || registeredAt.IsZero() {
		return fmt.Errorf("register notification channel: tenant, channel and time are required")
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.notification_channel (tenant_id, channel_ref, endpoint, credential_ref, registered_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (tenant_id, channel_ref)
		 DO UPDATE SET endpoint = EXCLUDED.endpoint, credential_ref = EXCLUDED.credential_ref, registered_at = EXCLUDED.registered_at`,
		tenant.String(), channel.Channel().String(), channel.Endpoint(), channel.Credential(), registeredAt.UTC(),
	); err != nil {
		return fmt.Errorf("register notification channel: %w", err)
	}
	return nil
}

func (repository *NotificationChannels) FindChannel(
	ctx context.Context,
	tenant domain.TenantID,
	channel domain.NotificationChannelReference,
) (domain.NotificationChannelEndpoint, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.NotificationChannelEndpoint{}, false, fmt.Errorf("find notification channel: %w", err)
	}
	var endpoint, credential string
	err = querier.QueryRow(ctx,
		`SELECT endpoint, credential_ref FROM visibility_exception.notification_channel WHERE tenant_id = $1 AND channel_ref = $2`,
		tenant.String(), channel.String(),
	).Scan(&endpoint, &credential)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NotificationChannelEndpoint{}, false, nil
	}
	if err != nil {
		return domain.NotificationChannelEndpoint{}, false, fmt.Errorf("find notification channel: %w", err)
	}
	registered, err := domain.NewNotificationChannelEndpoint(channel, endpoint, credential)
	if err != nil {
		return domain.NotificationChannelEndpoint{}, false, fmt.Errorf("find notification channel: %w", err)
	}
	return registered, true, nil
}

var _ ports.NotificationSendLock = (*CustomerNotifications)(nil)

// LockDisclosure 取一把随事务释放的咨询锁，键是（租户，发作期，客户）。通知行在首发之前还不存在，没有行可锁。
func (repository *CustomerNotifications) LockDisclosure(
	ctx context.Context,
	tenant domain.TenantID,
	episode domain.EpisodeID,
	customer domain.CustomerAccountReference,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("lock disclosure for notification: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('visibility_exception.customer_notification.disclosure'),
		        hashtext($1 || chr(31) || $2 || chr(31) || $3))`,
		tenant.String(), episode.String(), customer.String(),
	); err != nil {
		return fmt.Errorf("lock disclosure for notification: %w", err)
	}
	return nil
}
