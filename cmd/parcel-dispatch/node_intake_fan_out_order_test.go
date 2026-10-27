package main

import (
	"sort"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// 本文件证生产登记处里节点收寄形成信封上各路的名次（ADR-0299 决定一）。名次只由登记处的 position 定，装配期之外没有别的地方读得到，
// 所以这里按 wireDispatcher 同一份件展开 routeRegistry，读出真正登记的各路。

// registeredLegsOn 以生产装配同一份件展开登记处，交回登在 event 上的各路，按名次排好。
func registeredLegsOn(t *testing.T, event eventing.EventType) []routeLeg {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	outboxStore, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	inboxStore, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	clock := systemClock{}
	projectionDerive, err := newTenantBoundProjectionDerive(db, outboxStore, clock)
	if err != nil {
		t.Fatalf("构造投影派生：%v", err)
	}
	purpose, err := nrdomain.NewServicePurpose("NETWORK_SERVICE")
	if err != nil {
		t.Fatalf("服务目的：%v", err)
	}
	sources := dispatchSources{
		db: db, outbox: outboxStore, inbox: inboxStore, clock: clock, projectionDerive: projectionDerive,
		settings: dispatchSettings{
			purpose: purpose, deliveryTimeout: 5 * time.Second,
			config: dispatch.Config{Limit: 10, LeaseFor: time.Minute, MaxAttempts: 5, RetryAfter: 30 * time.Second},
		},
	}
	var legs []routeLeg
	for _, build := range routeRegistry {
		built, err := build(sources)
		if err != nil {
			t.Fatalf("展开登记处：%v", err)
		}
		for _, leg := range built {
			if leg.event == event {
				legs = append(legs, leg)
			}
		}
	}
	sort.SliceStable(legs, func(i, j int) bool { return legs[i].position < legs[j].position })
	return legs
}

// Covers: AT-VE-221 名次半边（ADR-0299 决定一、越权风险点 7）、AT-TF-366 名次半边（ADR-0331 决定四）——节点收寄形成信封上，移动事实补派生与成员
// 的出发门禁补问都排在 VE 投影与 PS 来源采用之后：两路读量都随候选单元与整袋增长，补问还要问关务，排在前面会吃掉主业务的投递时间；排在后面，
// 主业务先落地，两路是尽力而为的自愈；补问排在补派生之后，是因为它再多一次问关务。
func TestTheMovementFactBackfillAndTheMemberReaskOnNodeIntakeRunAfterTheProjectionAndTheSourceAdoption(t *testing.T) {
	legs := registeredLegsOn(t, veinbox.NodeIntakeFormedEventType)
	var labels []string
	for _, leg := range legs {
		labels = append(labels, leg.label)
	}
	want := []string{"projection", "node intake", "movement fact backfill on node intake", "departure gate reask on node intake"}
	if len(labels) != len(want) {
		t.Fatalf("节点收寄形成信封上的各路 = %v, want %v", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("节点收寄形成信封上的各路次序 = %v, want %v", labels, want)
		}
	}
}
