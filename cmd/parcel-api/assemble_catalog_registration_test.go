package main

import (
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	ccregistrationjson "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/registrationjson"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	networkapp "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	networkdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	networkports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	veregistrationjson "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/registrationjson"
	visibilityapp "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

// Covers: 网络七族、关务四类与 VE 六类登记端点的第二参是真编排——三个 build* 在真实
// PostgreSQL 上装得起来，且各自的事务壳**确实提交**。
//
// 各族的委托接没接对不在本文件证：每个内层方法收的命令类型互不相同，把一族的编排接到
// 另一族的方法上编译期就红。这里要证的是编译器看不见的那一半——事务壳把用例答案带出来
// 的同时有没有把写入留在库里。三处各取一条链，取法随各自答案代数的差别而不同：
//
//   - 网络目录没有重放格（重复版本号由主键挡，用例把它当依赖故障上抛），所以提交与否
//     只能靠读回同一只适配器的列面来证。
//   - 关务口岸册的重放格 `EXISTING` 本身就要读回首行才答得出来，重放即提交证据。
//   - VE 目录的重放答 `REFUSED`/`VERSION_NOT_OVERWRITABLE`，同理要读回首行。
//
// 测试输入是隔离合成，只记 `S`，不进生产装配。
func TestTheWiredCatalogRegistrationsRecordAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()

	t.Run("网络目录", func(t *testing.T) {
		registration, err := buildNetworkCatalogRegistrationOrchestration(db)
		if err != nil {
			t.Fatalf("装配网络目录登记编排：%v", err)
		}
		tenant, err := networkdomain.NewTenantID("SYN-TENANT-API-NR")
		if err != nil {
			t.Fatalf("构造租户：%v", err)
		}
		node := networkports.NodeDefinitionVersion{
			Code:             "SYN-API-NODE-1",
			Version:          1,
			BusinessTimezone: "Asia/Shanghai",
			EffectiveFrom:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		result, err := registration.RegisterNodeVersion(ctx, networkapp.RegisterNodeVersionCommand{
			TenantID: tenant,
			Node:     node,
		})
		if err != nil {
			t.Fatalf("节点版本登记：%v", err)
		}
		if result.Outcome() != networkapp.CatalogRegistered {
			t.Fatalf("节点版本登记 outcome = %s（refusal %s），想要 REGISTERED",
				result.Outcome(), result.RefusalReason())
		}

		catalog, err := nrpostgres.NewNetworkCatalog(db)
		if err != nil {
			t.Fatalf("构造网络目录读面：%v", err)
		}
		rows, err := catalog.ListNodeVersions(ctx, tenant, 50)
		if err != nil {
			t.Fatalf("读回节点版本：%v", err)
		}
		var found bool
		for _, row := range rows {
			if row.Code == node.Code && row.Version == node.Version {
				found = true
			}
		}
		if !found {
			t.Fatalf("列面里没有 %s/v%d——事务壳没把这一笔提交", node.Code, node.Version)
		}
	})

	t.Run("关务候选口岸", func(t *testing.T) {
		registration, err := buildCustomsRegistrationOrchestration(db)
		if err != nil {
			t.Fatalf("装配关务登记编排：%v", err)
		}
		command, err := ccregistrationjson.CandidatePortFromJSON([]byte(
			`{"tenantId":"SYN-TENANT-API-CC","portRef":"SYN-API-PORT-1",` +
				`"appliesFrom":"2026-01-01T00:00:00Z"}`))
		if err != nil {
			t.Fatalf("译装候选口岸登记：%v", err)
		}

		registered, err := registration.candidatePort.Handle(ctx, command)
		if err != nil {
			t.Fatalf("候选口岸首登：%v", err)
		}
		if registered != customsapp.ConfigurationRegistered {
			t.Fatalf("候选口岸首登 outcome = %s，想要 REGISTERED", registered)
		}
		replayed, err := registration.candidatePort.Handle(ctx, command)
		if err != nil {
			t.Fatalf("候选口岸重放：%v", err)
		}
		if replayed != customsapp.ConfigurationExisting {
			t.Fatalf("候选口岸重放 outcome = %s，想要 EXISTING——读不到首行说明首登事务没提交", replayed)
		}
	})

	t.Run("VE 里程碑映射", func(t *testing.T) {
		registration, err := buildVERegistrationOrchestration(db)
		if err != nil {
			t.Fatalf("装配 VE 登记编排：%v", err)
		}
		command, err := veregistrationjson.MilestoneMappingFromJSON([]byte(
			`{"tenantId":"SYN-TENANT-API-VE","version":"SYN-API-MAP-V1",` +
				`"approvedBy":"SYN-approver-1","effectiveFrom":"2026-09-01T00:00:00Z",` +
				`"entries":[{"source":"PARCEL_SHIPMENT","factKind":"SYN_KIND_DELIVERED",` +
				`"milestone":"SYN-MILESTONE-DELIVERED"}]}`))
		if err != nil {
			t.Fatalf("译装里程碑映射登记：%v", err)
		}

		registered, err := registration.milestoneMapping.Handle(ctx, command)
		if err != nil {
			t.Fatalf("里程碑映射首登：%v", err)
		}
		if registered.Outcome() != visibilityapp.CatalogRegistered {
			t.Fatalf("里程碑映射首登 outcome = %s（refusal %s），想要 REGISTERED",
				registered.Outcome(), registered.RefusalReason())
		}
		replayed, err := registration.milestoneMapping.Handle(ctx, command)
		if err != nil {
			t.Fatalf("里程碑映射重放：%v", err)
		}
		if replayed.Outcome() != visibilityapp.CatalogRegistrationRefused ||
			replayed.RefusalReason() != visibilityapp.CatalogVersionNotOverwritable {
			t.Fatalf("里程碑映射重放 outcome = %s / refusal = %s，想要 REFUSED / VERSION_NOT_OVERWRITABLE"+
				"——读不到首行说明首登事务没提交", replayed.Outcome(), replayed.RefusalReason())
		}
	})
}
