package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// DeviceRegistry 实现作业设备册的登记口与装载口（access_identity 0004、ADR-0149 决定二）。撞键从不覆盖：同内容重放
// 答原结果，异内容答冲突，册上已有的那一行原样不动。
type DeviceRegistry struct {
	db *bentopg.DB
}

func NewDeviceRegistry(db *bentopg.DB) (*DeviceRegistry, error) {
	if db == nil {
		return nil, fmt.Errorf("access identity postgres: db is nil")
	}
	return &DeviceRegistry{db: db}, nil
}

var (
	_ accessidentity.DeviceRegistry  = (*DeviceRegistry)(nil)
	_ accessidentity.DeviceRegistrar = (*DeviceRegistry)(nil)
	_ accessidentity.DeviceCatalogue = (*DeviceRegistry)(nil)
)

func (registry *DeviceRegistry) RegisterDevice(
	ctx context.Context,
	registration accessidentity.DeviceRegistration,
) (accessidentity.DeviceRegistrationOutcome, error) {
	const operation = "register device"
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	scope := registration.WorkScope()
	tag, err := executor.Exec(ctx,
		`INSERT INTO access_identity.device (tenant_id, device_id, work_scope_kind, work_scope_ref, basis_ref)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT DO NOTHING`,
		registration.TenantID(), registration.DeviceID(), scope.Kind().String(), scope.Reference(), registration.Basis(),
	)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return accessidentity.DeviceRegistrationRecorded, nil
	}

	var kind, reference, basis string
	err = executor.QueryRow(ctx,
		`SELECT work_scope_kind, work_scope_ref, basis_ref
		   FROM access_identity.device
		  WHERE tenant_id = $1 AND device_id = $2`,
		registration.TenantID(), registration.DeviceID(),
	).Scan(&kind, &reference, &basis)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if kind == scope.Kind().String() && reference == scope.Reference() && basis == registration.Basis() {
		return accessidentity.DeviceRegistrationAlreadyRegistered, nil
	}
	return accessidentity.DeviceRegistrationContentConflict, nil
}

// RegisterDeactivation 先查本租户册上有没有这台设备再写，理由同 OperatorRegistry.RegisterGrant：让外键报错会把整个事务
// 打成失败态，调用方就没法在同一事务里拿到一个「未登记」的答复。按（租户、设备标识）查，别的租户拿同一个标识停不动。
func (registry *DeviceRegistry) RegisterDeactivation(
	ctx context.Context,
	deactivation accessidentity.DeviceDeactivation,
) (accessidentity.DeviceRegistrationOutcome, error) {
	const operation = "register device deactivation"
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	var registered bool
	if err := executor.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM access_identity.device
			 WHERE tenant_id = $1 AND device_id = $2)`,
		deactivation.TenantID(), deactivation.DeviceID(),
	).Scan(&registered); err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if !registered {
		return accessidentity.DeviceNotRegistered, nil
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO access_identity.device_deactivation (tenant_id, device_id, deactivated_at, basis_ref)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		deactivation.TenantID(), deactivation.DeviceID(), deactivation.DeactivatedAt(), deactivation.Basis(),
	)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return accessidentity.DeviceRegistrationRecorded, nil
	}

	var (
		storedAt    time.Time
		storedBasis string
	)
	err = executor.QueryRow(ctx,
		`SELECT deactivated_at, basis_ref
		   FROM access_identity.device_deactivation
		  WHERE tenant_id = $1 AND device_id = $2`,
		deactivation.TenantID(), deactivation.DeviceID(),
	).Scan(&storedAt, &storedBasis)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if sameInstant(storedAt, deactivation.DeactivatedAt()) && storedBasis == deactivation.Basis() {
		return accessidentity.DeviceRegistrationAlreadyRegistered, nil
	}
	return accessidentity.DeviceRegistrationContentConflict, nil
}

// FindDevice 用一条查询取设备与它的停用。此刻在不在用不在这里判，理由同 FindOperator：调用方拿 RecordedDevice 对自己的
// 时点判。
func (registry *DeviceRegistry) FindDevice(
	ctx context.Context,
	tenantID, deviceID string,
) (accessidentity.RecordedDevice, bool, error) {
	const operation = "find device"
	querier, err := registry.db.ReadExecutor(ctx)
	if err != nil {
		return accessidentity.RecordedDevice{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	var (
		kind, reference, basis string
		deactivatedAt          *time.Time
		deactivationBasis      *string
	)
	err = querier.QueryRow(ctx,
		`SELECT d.work_scope_kind, d.work_scope_ref, d.basis_ref, x.deactivated_at, x.basis_ref
		   FROM access_identity.device d
		   LEFT JOIN access_identity.device_deactivation x
		     ON x.tenant_id = d.tenant_id AND x.device_id = d.device_id
		  WHERE d.tenant_id = $1 AND d.device_id = $2`,
		tenantID, deviceID,
	).Scan(&kind, &reference, &basis, &deactivatedAt, &deactivationBasis)
	if errors.Is(err, pgx.ErrNoRows) {
		return accessidentity.RecordedDevice{}, false, nil
	}
	if err != nil {
		return accessidentity.RecordedDevice{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	device, err := recordedDeviceFromRow(tenantID, deviceID, kind, reference, basis, deactivatedAt, deactivationBasis)
	if err != nil {
		return accessidentity.RecordedDevice{}, false, fmt.Errorf("%s: 设备 %s：%w", operation, deviceID, err)
	}
	return device, true, nil
}

// deviceCatalogueStatus 由停用导出查阅行上的状态词，词取 accessidentity.DeviceCatalogueStatus：登记与停用不另存一列状态。
var deviceCatalogueStatus = fmt.Sprintf("CASE WHEN x.device_id IS NULL THEN '%s' ELSE '%s' END",
	accessidentity.DeviceCatalogueRegistered, accessidentity.DeviceCatalogueDeactivated)

// deviceCataloguePages 把 accessidentity.DeviceCataloguePages 落到设备表 d 与停用表 x 的列上。停用表以（租户、设备标识）
// 为主键，左连接不放大行数，总数数的仍是设备。
var deviceCataloguePages = accessidentity.DeviceCataloguePages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"deviceId": "d.device_id"},
	Identity: []string{"d.device_id"},
	Filters:  map[string]string{"status": deviceCatalogueStatus},
	Keyword:  []string{"d.device_id", "d.work_scope_ref"},
})

// scannedDevice 是一台设备扫描回来的设备标识列与建好的查阅行：查阅行上的标识经构造门修剪过，游标位置取列上原样的值。
type scannedDevice struct {
	deviceID string
	row      accessidentity.DeviceCatalogueRow
}

// ListDevices 按租户列出已登记设备，含已停用的，取页按 accessidentity.DeviceCataloguePages。空册答空页。别的租户的行不在
// 结果里。一台设备连同它的停用出自同一条查询；总数另起一句，与本页同一组条件。limit 非正是调用方编程错误。
//
// 查询仍取出依据，只为经领域构造门重建登记行；查阅行再把依据留在登记行上，不带出去。缺省次序按设备标识，
// 两次读回才对得上，不表示业务优先级。
func (registry *DeviceRegistry) ListDevices(
	ctx context.Context,
	tenantID string,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[accessidentity.DeviceCatalogueRow], error) {
	const operation = "list devices"
	fail := func(err error) (cataloguepage.Result[accessidentity.DeviceCatalogueRow], error) {
		return cataloguepage.Result[accessidentity.DeviceCatalogueRow]{}, fmt.Errorf("%s: %w", operation, err)
	}
	tenant := strings.TrimSpace(tenantID)
	if tenant == "" {
		return fail(accessidentity.ErrDeviceCatalogueTenantRequired)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := deviceCataloguePages.Plan(query, "d.tenant_id = $1", []any{tenant}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := registry.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	const source = `access_identity.device d
		   LEFT JOIN access_identity.device_deactivation x
		     ON x.tenant_id = d.tenant_id AND x.device_id = d.device_id`
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(source), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx,
		plan.PageSQL("d.device_id, d.work_scope_kind, d.work_scope_ref, d.basis_ref, x.deactivated_at, x.basis_ref", source),
		plan.PageArgs...,
	)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	var fetched []scannedDevice
	for rows.Next() {
		var (
			deviceID, kind, reference, basis string
			deactivatedAt                    *time.Time
			deactivationBasis                *string
		)
		if err := rows.Scan(&deviceID, &kind, &reference, &basis, &deactivatedAt, &deactivationBasis); err != nil {
			return fail(err)
		}
		device, err := recordedDeviceFromRow(tenant, deviceID, kind, reference, basis, deactivatedAt, deactivationBasis)
		if err != nil {
			return fail(fmt.Errorf("设备 %s：%w", deviceID, err))
		}
		row, err := device.CatalogueRow()
		if err != nil {
			return fail(fmt.Errorf("设备 %s：%w", deviceID, err))
		}
		fetched = append(fetched, scannedDevice{deviceID: deviceID, row: row})
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}

	page, err := cataloguepage.Finish(query, fetched, limit, total, devicePosition)
	if err != nil {
		return fail(err)
	}
	listed := make([]accessidentity.DeviceCatalogueRow, 0, len(page.Rows))
	for _, scanned := range page.Rows {
		listed = append(listed, scanned.row)
	}
	return cataloguepage.Result[accessidentity.DeviceCatalogueRow]{Rows: listed, Next: page.Next, Total: page.Total}, nil
}

// devicePosition 取本页末台设备扫描回来的设备标识：排序维与行标识是同一列。
func devicePosition(row scannedDevice, _ string) cataloguepage.Position {
	return cataloguepage.Position{Value: row.deviceID, Identity: []string{row.deviceID}}
}

// recordedDeviceFromRow 经领域构造门重建一台设备，理由同 recordedGrantFromRow。
func recordedDeviceFromRow(
	tenantID, deviceID, rawKind, reference, basis string,
	deactivatedAt *time.Time,
	deactivationBasis *string,
) (accessidentity.RecordedDevice, error) {
	scope, err := workScopeFromRow(rawKind, reference)
	if err != nil {
		return accessidentity.RecordedDevice{}, err
	}
	registration, err := accessidentity.NewDeviceRegistration(tenantID, deviceID, scope, basis)
	if err != nil {
		return accessidentity.RecordedDevice{}, err
	}
	if deactivatedAt == nil {
		return accessidentity.NewRecordedDevice(registration, nil)
	}
	off := ""
	if deactivationBasis != nil {
		off = *deactivationBasis
	}
	deactivation, err := accessidentity.NewDeviceDeactivation(tenantID, deviceID, *deactivatedAt, off)
	if err != nil {
		return accessidentity.RecordedDevice{}, err
	}
	return accessidentity.NewRecordedDevice(registration, &deactivation)
}

func workScopeFromRow(rawKind, reference string) (accessidentity.WorkScope, error) {
	kind, err := accessidentity.ParseWorkScopeKind(rawKind)
	if err != nil {
		return accessidentity.WorkScope{}, err
	}
	return accessidentity.NewWorkScope(kind, reference)
}
