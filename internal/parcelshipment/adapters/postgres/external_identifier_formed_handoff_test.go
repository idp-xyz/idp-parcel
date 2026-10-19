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

// 本文件对真实 PostgreSQL 16 证外部标识关联册新形成关联时交出的门铃（ADR-0199 决定三，票 no-reception-source/03）：关联
// 真插入的那一笔同时入队一封只带（租户，号码）的信封；重放不插就不入队；适用终态不交。夹具全为合成引用（S 级）。

// doorbell 是读回的一封门铃：信封的几格与载荷原样。
type doorbell struct {
	eventID, subject, scope, partition string
	payload                            map[string]string
}

func doorbellsOf(t *testing.T, pool *pgxpool.Pool) []doorbell {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT event_id, subject, scope, partition_key, payload FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_type = $1 ORDER BY subject`,
		adapter.ExternalIdentifierRelationFormedEventType)
	if err != nil {
		t.Fatalf("读门铃：%v", err)
	}
	defer rows.Close()
	var found []doorbell
	for rows.Next() {
		var bell doorbell
		var raw []byte
		if err := rows.Scan(&bell.eventID, &bell.subject, &bell.scope, &bell.partition, &raw); err != nil {
			t.Fatalf("扫门铃：%v", err)
		}
		if err := json.Unmarshal(raw, &bell.payload); err != nil {
			t.Fatalf("门铃载荷不是一层字符串对象：%v（%s）", err, raw)
		}
		found = append(found, bell)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读门铃：%v", err)
	}
	return found
}

func numbersRung(bells []doorbell) []string {
	numbers := make([]string, 0, len(bells))
	for _, bell := range bells {
		numbers = append(numbers, bell.payload["number"])
	}
	slices.Sort(numbers)
	return numbers
}

// Covers: ADR-0199 决定三第一个触发与 CONTEXT-MAP `parcel-shipment → node-operations`（外部标识关联已形成的交接）——委托接受
// 那一笔写下两条客户包裹件号关联，同笔入队两封门铃：载荷只有租户与号码（不带包裹、分配方、标识类型或委托），Subject 原样是号码，
// Scope 是租户，信封 ID 是定长指纹、分区键与 ID 不同。同一份已接受委托再保存一次，关联照 ON CONFLICT 不插，门铃也不再入队。
func TestAcceptanceRingsADoorbellPerFormedRelation(t *testing.T) {
	fixture := newExternalIdentifierFixture(t)
	request := numberedRequest(t, "tenant-1", "customer-1", "req-key-1", "request-1",
		[]string{"parcel-1", "parcel-2"}, map[string]string{"parcel-1": "CUST-NRS03-1", "parcel-2": "CUST-NRS03-2"})
	fixture.insertAndAccept(t, request)

	bells := doorbellsOf(t, fixture.pool)
	if got := numbersRung(bells); !slices.Equal(got, []string{"CUST-NRS03-1", "CUST-NRS03-2"}) {
		t.Fatalf("门铃的号 = %v, want 两条关联各一封", got)
	}
	for _, bell := range bells {
		if len(bell.payload) != 2 || bell.payload["tenantId"] != "tenant-1" {
			t.Fatalf("门铃载荷 = %v, want 只有 tenantId 与 number", bell.payload)
		}
		if bell.subject != bell.payload["number"] || bell.scope != "tenant-1" {
			t.Fatalf("Subject %q Scope %q, want 号码原样与租户", bell.subject, bell.scope)
		}
		if !strings.HasPrefix(bell.eventID, "external-identifier-relation/") || len(bell.eventID) > eventing.MaxEventIDLength {
			t.Fatalf("信封 ID = %q, want 口名前缀加定长指纹", bell.eventID)
		}
		if bell.partition == bell.eventID || len(bell.partition) > eventing.MaxPartitionKeyLength {
			t.Fatalf("分区键 = %q, want 与 ID 不同且有界", bell.partition)
		}
	}

	ctx := t.Context()
	identity := request.CurrentSubmissionVersion().SourceSubmission().Identity()
	accepted, found, err := fixture.requests.FindBySourceIdentity(ctx, identity)
	if err != nil || !found {
		t.Fatalf("读回已接受委托：found = %v err = %v", found, err)
	}
	mustSave(t, fixture.transactor, ctx, fixture.requests, identity, accepted)
	if again := doorbellsOf(t, fixture.pool); len(again) != 2 {
		t.Fatalf("再保存一次后门铃 %d 封, want 仍是 2——关联没插，门铃不该再入队", len(again))
	}
}

// Covers: 票 operator-channel/54 的口径落到门铃上——号码进册前只有「存得下」那道门（票 ps-external-mark-relations/06），不拦
// 控制字符：带控制字符的号与册收得下的最长号照样形成关联，接受不因门铃被拒而回滚。Subject 收不下的号换成号码的定长摘要，
// 收得下的原样带，哪一封都不越 Subject 上限；载荷照旧原样带号。
func TestANumberTheSubjectCannotCarryStillRingsByItsDigest(t *testing.T) {
	fixture := newExternalIdentifierFixture(t)
	longest := strings.Repeat("CUST-NRS03-LONG-", 32)
	tabbed := "CUST-NRS03\tTAB"
	fixture.insertAndAccept(t, numberedRequest(t, "tenant-1", "customer-1", "req-key-1", "request-1",
		[]string{"parcel-1", "parcel-2"}, map[string]string{"parcel-1": longest, "parcel-2": tabbed}))

	bells := doorbellsOf(t, fixture.pool)
	if got := numbersRung(bells); !slices.Equal(got, []string{tabbed, longest}) && !slices.Equal(got, []string{longest, tabbed}) {
		t.Fatalf("门铃的号 = %q, want 两个号原样在载荷里", got)
	}
	for _, bell := range bells {
		number := bell.payload["number"]
		digested := strings.HasPrefix(bell.subject, "NUMBER-SHA256/")
		if len(bell.subject) > eventing.MaxSubjectLength || (bell.subject != number && !digested) || (number == tabbed && !digested) {
			t.Fatalf("号 %q 的 Subject = %q, want 收得下的原样、收不下的（带控制字符）换成号码摘要，且不越上限", number, bell.subject)
		}
	}
	if got := fixture.parcelsOf(t, "tenant-1", longest); !slices.Equal(got, []string{"parcel-1"}) {
		t.Fatalf("最长的号查得 %v——接受被门铃带着回滚了", got)
	}
}

// Covers: ADR-0199 决定三「适用终态（作废、替代）不交信封」——同一笔保存里形成又作废的渠道包裹件号不交门铃，另一件的号照交；
// 换单形成的新号交门铃，被替代的旧号不交。
func TestARetiredChannelNumberRingsNoDoorbell(t *testing.T) {
	fixture := newChannelFixture(t)
	ctx := t.Context()
	observed := labelEstablishedAt.Add(2 * time.Minute)
	established := establishedLabelTransactionFixture(t, "tenant-a", "label-txn-1", "parcel-1", "parcel-2")
	mustInsertLabelTransaction(t, fixture.transactor, ctx, fixture.transactions, established)
	submitted, err := fixture.reload(t, "label-txn-1").SubmitToChannel(observed.Add(-time.Minute))
	if err != nil {
		t.Fatalf("提交渠道：%v", err)
	}
	fixture.save(t, submitted)
	recorded, err := fixture.reload(t, "label-txn-1").RecordChannelResult(domain.RecordChannelResultSpec{
		Outcome:       domain.LabelTransactionSucceeded,
		ParcelResults: []domain.LabelTransactionParcelResultSpec{acceptedChannelResult(t, "parcel-1", "CH-NRS03-1"), acceptedChannelResult(t, "parcel-2", "CH-NRS03-2")},
		ObservedAt:    observed,
	})
	if err != nil {
		t.Fatalf("记录渠道结果：%v", err)
	}
	voided, err := recorded.AppendFollowUpAction(domain.FollowUpActionSpec{
		Kind:       domain.ChannelVoidAction,
		Parcels:    []domain.DeclaredParcelID{mustBuild(t, domain.NewDeclaredParcelID, "parcel-1")},
		Reason:     mustBuild(t, domain.NewChannelResultReasonReference, "SYN-REASON"),
		OccurredAt: observed.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("作废：%v", err)
	}
	fixture.save(t, voided)
	if got := numbersRung(doorbellsOf(t, fixture.pool)); !slices.Equal(got, []string{"CH-NRS03-2"}) {
		t.Fatalf("门铃的号 = %v, want 只有没作废的 CH-NRS03-2", got)
	}

	replacement := fixture.replacementOf(t, "label-txn-1", "label-txn-2", "", observed.Add(time.Hour), "parcel-2")
	fixture.recordChannel(t, replacement, observed.Add(2*time.Hour), acceptedChannelResult(t, "parcel-2", "CH-NRS03-2-NEW"))
	if got := numbersRung(doorbellsOf(t, fixture.pool)); !slices.Equal(got, []string{"CH-NRS03-2", "CH-NRS03-2-NEW"}) {
		t.Fatalf("换单后门铃的号 = %v, want 新号多一封、被替代的旧号不再交", got)
	}
}

// Covers: PBC-08 负向证据——门铃与关联同生共死，没有事务就拒，不在自动提交下单独入队一封。
func TestTheDoorbellRefusesToRingOutsideATransaction(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	doorbell, err := adapter.NewExternalIdentifierDoorbell(db)
	if err != nil {
		t.Fatalf("构造门铃：%v", err)
	}
	request := accept(t, numberedRequest(t, "tenant-1", "customer-1", "req-key-1", "request-1",
		[]string{"parcel-1"}, map[string]string{"parcel-1": "CUST-NRS03-1"}))
	relations := request.ExternalIdentifierRelationsFormedByAcceptance(request.CurrentSubmissionVersion().SourceSubmission().Identity())
	if len(relations) != 1 {
		t.Fatalf("接受形成 %d 条关联, want 1", len(relations))
	}
	if err := doorbell.RingFormed(context.Background(), relations[0], time.Now()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务响门铃 err = %v, want ErrTransactionRequired", err)
	}
	if _, err := adapter.NewExternalIdentifierDoorbell(nil); err == nil {
		t.Fatal("缺 db 却构造成功")
	}
}
