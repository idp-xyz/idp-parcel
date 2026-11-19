package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 本文件证包材册上「并发提交同一修订号的两笔，至多一笔落册，另一笔按内容答重放或冲突」靠的是库上的主键，与仓、库区、库位类型几册同一个机制：
// 第一笔在自己的事务里插入后先不提交，第二笔随后保存同一修订号、被主键卡在行锁上，观察连接确认它真的卡住了，才放第一笔提交。

// Covers: AT-WF-001（包材的部分）——并发提交同一修订号、内容相同的两笔，一笔 SAVED、一笔 ALREADY_REGISTERED；内容不同则一笔 SAVED、一笔
// CONTENT_CONFLICT。两种情形库里都只有先提交的那一份，登记责任方是先落册者的。内容相同里第二笔换了数值的写法：数值按数值比，仍是重放。
func TestPackagingMaterialRegistryConcurrentSavesOfOneRevisionLandAtMostOnce(t *testing.T) {
	for name, test := range map[string]struct {
		edit func(*domain.PackagingMaterialContentSpec)
		want ports.RegistrySaveOutcome
	}{
		"内容相同（换了数值的写法）": {func(spec *domain.PackagingMaterialContentSpec) {
			spec.Length, spec.Height = measureValue(t, "30.00"), measureValue(t, "015.50")
		}, ports.RegistryRevisionAlreadyRegistered},
		"内容不同": {func(spec *domain.PackagingMaterialContentSpec) {
			spec.Name = valueOf(t, domain.NewPackagingMaterialName, "合成一号纸箱（另一份）")
		}, ports.RegistryRevisionContentConflict},
	} {
		t.Run(name, func(t *testing.T) {
			stores := newPackagingMaterialStores(t)
			observer := newBlockedSessionObserver(t, stores.pool)
			first := packagingMaterialFixture(t, "SYN-TENANT-01", "SYN-BOX-01", 1, nil)
			second := packagingMaterialRegisteredBy(t, packagingMaterialFixture(t, "SYN-TENANT-01", "SYN-BOX-01", 1, test.edit), "operator-2", baseInstant.Add(time.Hour))

			firstOutcome, secondOutcome := raceSaves(t, stores.transactor, stores.packagingMaterials.SavePackagingMaterialRevision, observer, first, second)
			if firstOutcome.err != nil || secondOutcome.err != nil {
				t.Fatalf("两笔的错误：%v、%v", firstOutcome.err, secondOutcome.err)
			}
			if firstOutcome.outcome != ports.RegistryRevisionSaved || secondOutcome.outcome != test.want {
				t.Fatalf("两笔的落点 = %s、%s，want SAVED、%s", firstOutcome.outcome, secondOutcome.outcome, test.want)
			}

			var rows int
			var registrant string
			if err := stores.pool.QueryRow(t.Context(),
				`SELECT count(*), min(registrant) FROM warehouse_fulfillment.packaging_material_revision`).Scan(&rows, &registrant); err != nil {
				t.Fatalf("读册：%v", err)
			}
			if rows != 1 || registrant != "operator-1" {
				t.Fatalf("册里 %d 笔、登记责任方 %q，want 只有先提交的那 1 笔（operator-1）", rows, registrant)
			}
		})
	}
}

// holdingPackagingMaterialRegistry 在第一次保存返回之前停住，让登记用例的事务悬着、不提交。
type holdingPackagingMaterialRegistry struct {
	ports.PackagingMaterialRegistry
	afterSave func()
}

func (registry holdingPackagingMaterialRegistry) SavePackagingMaterialRevision(
	ctx context.Context,
	revision domain.PackagingMaterialRevision,
) (ports.RegistrySaveOutcome, error) {
	outcome, err := registry.PackagingMaterialRegistry.SavePackagingMaterialRevision(ctx, revision)
	if registry.afterSave != nil {
		registry.afterSave()
	}
	return outcome, err
}

// Covers: AT-WF-001（包材的部分）端到端——登记用例在真库上并发提交同一修订号、内容相同的两笔：一笔 REGISTERED、一笔 ALREADY_REGISTERED，答重放的那一笔
// 回显先落册者的登记责任方；换内容则另一笔答 CONTENT_CONFLICT。每笔各起自己的事务，本租户已登记过仓。
func TestRegisterPackagingMaterialConcurrentRegistrationsOnTheRealRegisterLandAtMostOnce(t *testing.T) {
	for name, test := range map[string]struct {
		secondName string
		want       application.RegistrationOutcome
	}{
		"内容相同": {"合成一号纸箱", application.RegistrationAlreadyRegistered},
		"内容不同": {"合成一号纸箱（另一份）", application.RegistrationContentConflict},
	} {
		t.Run(name, func(t *testing.T) {
			stores := newPackagingMaterialStores(t)
			mustSave(t, stores.transactor, stores.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 1, nil))
			observer := newBlockedSessionObserver(t, stores.pool)
			ctx := t.Context()
			clock := fixedClock{at: baseInstant}

			commandFor := func(materialName, registrant string) application.RegisterPackagingMaterialCommand {
				return application.RegisterPackagingMaterialCommand{
					Tenant:   valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
					Code:     valueOf(t, domain.NewPackagingMaterialCode, "SYN-BOX-01"),
					Revision: 1,
					Content: packagingMaterialContentWith(t, func(spec *domain.PackagingMaterialContentSpec) {
						spec.Name = valueOf(t, domain.NewPackagingMaterialName, materialName)
					}),
					Registrant: valueOf(t, domain.NewRegistrant, registrant),
				}
			}
			// 命令在起 goroutine 之前造好：valueOf 失败走 t.Fatalf，不该发生在别的 goroutine 里。
			firstCommand := commandFor("合成一号纸箱", "operator-1")
			secondCommand := commandFor(test.secondName, "operator-2")

			inserted := make(chan struct{})
			release := make(chan struct{})
			holding := holdingPackagingMaterialRegistry{PackagingMaterialRegistry: stores.packagingMaterials, afterSave: func() {
				close(inserted)
				<-release
			}}
			var firstResult, secondResult application.RegisterPackagingMaterialResult
			var firstErr, secondErr error
			var wait sync.WaitGroup

			wait.Add(1)
			go func() {
				defer wait.Done()
				handler := application.NewRegisterPackagingMaterialHandler(holding, stores.warehouses, clock)
				firstErr = stores.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
					var registerErr error
					firstResult, registerErr = handler.Register(txCtx, firstCommand)
					return registerErr
				})
			}()
			select {
			case <-inserted:
			case <-time.After(10 * time.Second):
				t.Fatalf("第一笔没有走到保存")
			}

			wait.Add(1)
			go func() {
				defer wait.Done()
				handler := application.NewRegisterPackagingMaterialHandler(stores.packagingMaterials, stores.warehouses, clock)
				secondErr = stores.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
					var registerErr error
					secondResult, registerErr = handler.Register(txCtx, secondCommand)
					return registerErr
				})
			}()
			blocked := observer.waitForBlockedSession(ctx, 10*time.Second)
			close(release)
			wait.Wait()

			if !blocked {
				t.Errorf("第二笔没有卡在主键上，没有走到「等锁」那条路")
			}
			if firstErr != nil || secondErr != nil {
				t.Fatalf("两笔的错误：%v、%v", firstErr, secondErr)
			}
			if firstResult.Outcome() != application.RegistrationRegistered || secondResult.Outcome() != test.want {
				t.Fatalf("两笔的结果 = %s、%s，want REGISTERED、%s", firstResult.Outcome(), secondResult.Outcome(), test.want)
			}
			if test.want == application.RegistrationAlreadyRegistered {
				echoed, has := secondResult.Revision()
				if !has || echoed.Registrant().String() != "operator-1" {
					t.Fatalf("答重放的一笔回显的登记责任方 = %q（%v），want 先落册者的 operator-1", echoed.Registrant(), has)
				}
			}
			if rows := countPackagingMaterialRows(t, stores); rows != 1 {
				t.Fatalf("册里 %d 笔，want 1", rows)
			}
		})
	}
}
