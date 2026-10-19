package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证声明撤除门铃（ADR-0211 决定三，票 no-reception-identity/07）：一份带客户包裹件号的委托由`已提交`
// 转为`已拒绝`或`已撤回`的那一笔保存，按它声明的每个号各入队一封只带（租户，号码）的信封；接受不交，已终局的委托再保存一次
// 不再交。夹具全为合成引用（S 级）。

// retiredDoorbell 是读回的一封声明撤除门铃：信封的几格与载荷原样。
type retiredDoorbell struct {
	eventID, subject, scope, partition string
	payload                            map[string]string
}

func retiredDoorbellsOf(t *testing.T, pool *pgxpool.Pool) []retiredDoorbell {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT event_id, subject, scope, partition_key, payload FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_type = $1 ORDER BY subject`,
		adapter.CustomerParcelNumberDeclarationRetiredEventType)
	if err != nil {
		t.Fatalf("读声明撤除门铃：%v", err)
	}
	defer rows.Close()
	var found []retiredDoorbell
	for rows.Next() {
		var bell retiredDoorbell
		var raw []byte
		if err := rows.Scan(&bell.eventID, &bell.subject, &bell.scope, &bell.partition, &raw); err != nil {
			t.Fatalf("扫声明撤除门铃：%v", err)
		}
		if err := json.Unmarshal(raw, &bell.payload); err != nil {
			t.Fatalf("门铃载荷不是一层字符串对象：%v（%s）", err, raw)
		}
		found = append(found, bell)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读声明撤除门铃：%v", err)
	}
	return found
}

func retiredNumbers(bells []retiredDoorbell) []string {
	numbers := make([]string, 0, len(bells))
	for _, bell := range bells {
		numbers = append(numbers, bell.payload["number"])
	}
	slices.Sort(numbers)
	return numbers
}

// Covers: ADR-0211 决定三与 CONTEXT-MAP `parcel-shipment → node-operations`（客户包裹件号声明撤除的交接）——委托被拒绝、被撤回
// 那一笔保存，按它声明的每个号各入队一封门铃：两件包裹报了同一个号只交一封；载荷只有租户与号码，不带包裹、委托、客户账户或
// 撤下的原因；Subject 原样是号码，Scope 是租户，信封 ID 是口名前缀加定长指纹、与分区键不同。
func TestARejectionOrWithdrawalRingsADoorbellPerDeclaredNumber(t *testing.T) {
	for _, ending := range []struct {
		name   string
		state  domain.ShipmentRequestState
		decide func(*testing.T) func(domain.ShipmentRequest) (domain.ShipmentRequest, error)
	}{
		{"被拒绝", domain.ShipmentRequestRejected, rejectByContract},
		{"被撤回", domain.ShipmentRequestWithdrawn, withdrawByCustomer},
	} {
		t.Run(ending.name, func(t *testing.T) {
			fixture := newExternalIdentifierFixture(t)
			request := numberedRequest(t, "tenant-1", "customer-1", "req-key-1", "request-1",
				[]string{"parcel-1", "parcel-2", "parcel-3", "parcel-4"},
				map[string]string{"parcel-1": "SYN-NRI07-A", "parcel-2": "SYN-NRI07-B", "parcel-3": "SYN-NRI07-A"})
			mustInsert(t, fixture.transactor, t.Context(), fixture.requests, request)
			if bells := retiredDoorbellsOf(t, fixture.pool); len(bells) != 0 {
				t.Fatalf("提交那一笔就交了 %d 封声明撤除门铃", len(bells))
			}

			fixture.end(t, request, ending.state, ending.decide(t))
			bells := retiredDoorbellsOf(t, fixture.pool)
			if got := retiredNumbers(bells); !slices.Equal(got, []string{"SYN-NRI07-A", "SYN-NRI07-B"}) {
				t.Fatalf("门铃的号 = %v, want 每个声明的号各一封、同号只一封", got)
			}
			for _, bell := range bells {
				if len(bell.payload) != 2 || bell.payload["tenantId"] != "tenant-1" {
					t.Fatalf("门铃载荷 = %v, want 只有 tenantId 与 number", bell.payload)
				}
				if bell.subject != bell.payload["number"] || bell.scope != "tenant-1" {
					t.Fatalf("Subject %q Scope %q, want 号码原样与租户", bell.subject, bell.scope)
				}
				if !strings.HasPrefix(bell.eventID, "customer-parcel-number-declaration-retired/") || len(bell.eventID) > eventing.MaxEventIDLength {
					t.Fatalf("信封 ID = %q, want 口名前缀加定长指纹", bell.eventID)
				}
				if bell.partition == bell.eventID || len(bell.partition) > eventing.MaxPartitionKeyLength {
					t.Fatalf("分区键 = %q, want 与 ID 不同且有界", bell.partition)
				}
			}
		})
	}
}

// Covers: ADR-0211 决定三「委托被接受走的是已有的外部标识关联已形成门铃，本票不另起触发」与「同一号的门铃排一条队」——接受那一笔
// 只交关联已形成门铃、不交声明撤除门铃；同一个号上一份委托被接受、另一份被拒绝，两种门铃的分区键相同。已拒绝的委托读回再保存
// 一次，不是由`已提交`转出，不再交；没给号的委托被拒绝，什么都不交。
func TestOnlyLeavingSubmittedUnacceptedRingsTheRetiredDoorbell(t *testing.T) {
	fixture := newExternalIdentifierFixture(t)
	ctx := t.Context()
	fixture.insertAndAccept(t, numberedRequest(t, "tenant-1", "customer-1", "req-key-a", "request-a",
		[]string{"parcel-a"}, map[string]string{"parcel-a": "SYN-NRI07-SHARED"}))
	if bells := retiredDoorbellsOf(t, fixture.pool); len(bells) != 0 {
		t.Fatalf("接受交了 %d 封声明撤除门铃", len(bells))
	}

	declared := numberedRequest(t, "tenant-1", "customer-2", "req-key-b", "request-b",
		[]string{"parcel-b"}, map[string]string{"parcel-b": "SYN-NRI07-SHARED"})
	mustInsert(t, fixture.transactor, ctx, fixture.requests, declared)
	fixture.end(t, declared, domain.ShipmentRequestRejected, rejectByContract(t))
	retired := retiredDoorbellsOf(t, fixture.pool)
	formed := doorbellsOf(t, fixture.pool)
	if len(retired) != 1 || len(formed) != 1 {
		t.Fatalf("声明撤除门铃 %d 封、关联已形成门铃 %d 封, want 各一封", len(retired), len(formed))
	}
	if retired[0].partition != formed[0].partition {
		t.Fatalf("同一个号的两种门铃分区键 %q / %q, want 相同——排一条队", retired[0].partition, formed[0].partition)
	}

	identity := declared.CurrentSubmissionVersion().SourceSubmission().Identity()
	rejected, found, err := fixture.requests.FindBySourceIdentity(ctx, identity)
	if err != nil || !found || rejected.State() != domain.ShipmentRequestRejected {
		t.Fatalf("读回已拒绝委托：found = %v err = %v", found, err)
	}
	mustSave(t, fixture.transactor, ctx, fixture.requests, identity, rejected)
	if again := retiredDoorbellsOf(t, fixture.pool); len(again) != 1 {
		t.Fatalf("已拒绝的委托再保存一次后门铃 %d 封, want 仍是 1", len(again))
	}

	unnumbered := numberedRequest(t, "tenant-1", "customer-3", "req-key-c", "request-c", []string{"parcel-c"}, nil)
	mustInsert(t, fixture.transactor, ctx, fixture.requests, unnumbered)
	fixture.end(t, unnumbered, domain.ShipmentRequestRejected, rejectByContract(t))
	if again := retiredDoorbellsOf(t, fixture.pool); len(again) != 1 {
		t.Fatalf("没给号的委托被拒绝后门铃 %d 封, want 仍是 1", len(again))
	}
}

// Covers: 门铃与状态变化同生共死——保存撞上版本冲突（另一方先推进了这份委托）时，这一笔什么都不写，也不入队门铃。
func TestARevisionConflictRingsNothing(t *testing.T) {
	fixture := newExternalIdentifierFixture(t)
	ctx := t.Context()
	request := numberedRequest(t, "tenant-1", "customer-1", "req-key-1", "request-1",
		[]string{"parcel-1"}, map[string]string{"parcel-1": "SYN-NRI07-RACE"})
	mustInsert(t, fixture.transactor, ctx, fixture.requests, request)
	identity := request.CurrentSubmissionVersion().SourceSubmission().Identity()
	stale, found, err := fixture.requests.FindBySourceIdentity(ctx, identity)
	if err != nil || !found {
		t.Fatalf("读回：found = %v err = %v", found, err)
	}
	fixture.end(t, request, domain.ShipmentRequestWithdrawn, withdrawByCustomer(t))

	rejected, err := rejectByContract(t)(stale)
	if err != nil {
		t.Fatalf("拒绝：%v", err)
	}
	var outcome string
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		saved, err := fixture.requests.Save(txCtx, identity, rejected)
		outcome = saved.String()
		return err
	}); err != nil {
		t.Fatalf("保存：%v", err)
	}
	if outcome != "REVISION_CONFLICT" {
		t.Fatalf("过时的版本保存答 %q, want REVISION_CONFLICT", outcome)
	}
	if bells := retiredDoorbellsOf(t, fixture.pool); len(bells) != 1 {
		t.Fatalf("门铃 %d 封, want 只有撤回那一封——版本冲突那一笔不入队", len(bells))
	}
}

// Covers: PBC-08 负向证据——声明撤除门铃与状态变化同生共死，没有事务就拒，不在自动提交下单独入队一封。
func TestTheRetiredDoorbellRefusesToRingOutsideATransaction(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	doorbell, err := adapter.NewExternalIdentifierDoorbell(db)
	if err != nil {
		t.Fatalf("构造门铃：%v", err)
	}
	request := numberedRequest(t, "tenant-1", "customer-1", "req-key-1", "request-1",
		[]string{"parcel-1"}, map[string]string{"parcel-1": "SYN-NRI07-1"})
	numbers := []domain.ExternalIdentifierNumber{request.CustomerParcelNumbers()[0].Number()}
	err = doorbell.RingDeclarationsRetired(context.Background(), mustBuild(t, domain.NewTenantID, "tenant-1"), request, numbers, time.Now())
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务响门铃 err = %v, want ErrTransactionRequired", err)
	}
}
