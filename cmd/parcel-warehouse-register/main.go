// parcel-warehouse-register 是仓、库区、库位类型、库位、包材五册的受控批量登记口。
// 运营在数据库网络内手跑，不监听端口。每一行调与在线登记口相同的登记器；租户取自行里的 tenantId，
// 登记人记为本口。开关关着时整批拒绝，一行都不落。
//
// 一行一笔事务：坏行不回滚已经落上的行。整批再跑时，已经落上的行答 ALREADY_REGISTERED。
//
// 退出码：0 每一行都是 REGISTERED 或 ALREADY_REGISTERED；1 有坏输入、未受理或未配置；
// 2 有内容冲突；3 依赖故障，那一行登记与否未知。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/cmd/internal/warehouseswitch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	whpostgres "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

const (
	exitSettled      = 0
	exitRefused      = 1
	exitConflict     = 2
	exitUndecided    = 3
	registeredBy     = "CONTROLLED-CLI/parcel-warehouse-register"
	kindWarehouse    = "warehouse"
	kindZone         = "zone"
	kindLocationType = "location-type"
	kindLocation     = "location"
	kindPackaging    = "packaging"
)

var supportedKinds = []string{kindWarehouse, kindZone, kindLocationType, kindLocation, kindPackaging}

type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now().UTC() }

type catalogues struct {
	transactor    bentoapp.Transactor
	warehouses    *application.RegisterWarehouseHandler
	zones         *application.RegisterZoneHandler
	locationTypes *application.RegisterLocationTypeHandler
	locations     *application.RegisterLocationHandler
	packaging     *application.RegisterPackagingMaterialHandler
	registrant    domain.Registrant
}

type batchRow struct {
	TenantID string          `json:"tenantId"`
	Revision json.RawMessage `json:"revision"`
}

type rowReport struct {
	Index     int
	Outcome   string
	Refusal   string
	Detail    string
	Technical bool
	Settled   bool
}

func main() {
	kind := flag.String("kind", "", "登记册："+strings.Join(supportedKinds, " / "))
	file := flag.String("file", "", "批量登记 JSON 路径，数组，每行含 tenantId 与 revision")
	flag.Parse()

	if !warehouseswitch.Enabled(os.Getenv) {
		fmt.Fprintf(os.Stderr, "仓配开关没开（%s 不是 true），批量登记拒绝\n", warehouseswitch.EnabledEnv)
		os.Exit(exitRefused)
	}
	if !knownKind(*kind) {
		fmt.Fprintf(os.Stderr, "-kind 必须是 %s\n", strings.Join(supportedKinds, " / "))
		os.Exit(exitRefused)
	}
	dsn := os.Getenv("IDP_PARCEL_POSTGRES_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "IDP_PARCEL_POSTGRES_DSN 未设置——登记口不猜连接串")
		os.Exit(exitRefused)
	}
	if *file == "" {
		fmt.Fprintln(os.Stderr, "-file 未指定")
		os.Exit(exitRefused)
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取批量登记：%v\n", err)
		os.Exit(exitRefused)
	}
	rows, err := decodeBatch(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "批量登记：%v\n", err)
		os.Exit(exitRefused)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接数据库：%v\n", err)
		os.Exit(exitUndecided)
	}
	defer pool.Close()
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		fmt.Fprintf(os.Stderr, "构造框架 DB：%v\n", err)
		os.Exit(exitUndecided)
	}
	built, err := buildCatalogues(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "装配登记器：%v\n", err)
		os.Exit(exitUndecided)
	}
	reports := applyBatch(ctx, built.transactor, rows, func(txCtx context.Context, row batchRow) (string, string, error) {
		return built.register(txCtx, *kind, row)
	})
	for _, report := range reports {
		fmt.Println(report.line())
	}
	os.Exit(exitCode(reports))
}

func knownKind(kind string) bool {
	for _, supported := range supportedKinds {
		if kind == supported {
			return true
		}
	}
	return false
}

func buildCatalogues(db *bentopg.DB) (catalogues, error) {
	var none catalogues
	warehouses, err := whpostgres.NewWarehouses(db)
	if err != nil {
		return none, err
	}
	zones, err := whpostgres.NewZones(db)
	if err != nil {
		return none, err
	}
	locationTypes, err := whpostgres.NewLocationTypes(db)
	if err != nil {
		return none, err
	}
	locations, err := whpostgres.NewLocations(db)
	if err != nil {
		return none, err
	}
	packaging, err := whpostgres.NewPackagingMaterials(db)
	if err != nil {
		return none, err
	}
	registrant, err := domain.NewRegistrant(registeredBy)
	if err != nil {
		return none, err
	}
	clock := wallClock{}
	return catalogues{
		transactor:    db.Transactor(),
		warehouses:    application.NewRegisterWarehouseHandler(warehouses, clock),
		zones:         application.NewRegisterZoneHandler(zones, warehouses, clock),
		locationTypes: application.NewRegisterLocationTypeHandler(locationTypes, warehouses, clock),
		locations:     application.NewRegisterLocationHandler(locations, warehouses, zones, locationTypes, clock),
		packaging:     application.NewRegisterPackagingMaterialHandler(packaging, warehouses, clock),
		registrant:    registrant,
	}, nil
}

func (catalogues catalogues) register(ctx context.Context, kind string, row batchRow) (string, string, error) {
	tenant, err := domain.NewTenantID(row.TenantID)
	if err != nil {
		return "", "", errUsage("tenantId is required")
	}
	if len(row.Revision) == 0 || string(row.Revision) == "null" {
		return "", "", errUsage("revision is required")
	}
	body := bytes.NewReader(row.Revision)
	switch kind {
	case kindWarehouse:
		command, err := registrationjson.WarehouseCommand(body, tenant, catalogues.registrant)
		if err != nil {
			return "", "", err
		}
		result, err := catalogues.warehouses.Register(ctx, command)
		return result.Outcome().String(), result.Refusal().String(), err
	case kindZone:
		command, err := registrationjson.ZoneCommand(body, tenant, catalogues.registrant)
		if err != nil {
			return "", "", err
		}
		result, err := catalogues.zones.Register(ctx, command)
		return result.Outcome().String(), result.Refusal().String(), err
	case kindLocationType:
		command, err := registrationjson.LocationTypeCommand(body, tenant, catalogues.registrant)
		if err != nil {
			return "", "", err
		}
		result, err := catalogues.locationTypes.Register(ctx, command)
		return result.Outcome().String(), result.Refusal().String(), err
	case kindLocation:
		command, err := registrationjson.LocationCommand(body, tenant, catalogues.registrant)
		if err != nil {
			return "", "", err
		}
		result, err := catalogues.locations.Register(ctx, command)
		return result.Outcome().String(), result.Refusal().String(), err
	case kindPackaging:
		command, err := registrationjson.PackagingMaterialCommand(body, tenant, catalogues.registrant)
		if err != nil {
			return "", "", err
		}
		result, err := catalogues.packaging.Register(ctx, command)
		return result.Outcome().String(), result.Refusal().String(), err
	default:
		return "", "", errUsage("kind is not one of the five catalogues")
	}
}
