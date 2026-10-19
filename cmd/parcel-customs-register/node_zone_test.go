package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

type nodeZoneClock struct{}

func (nodeZoneClock) Now() time.Time { return time.Date(2026, 8, 20, 10, 1, 0, 0, time.UTC) }

// 演示种子的四个节点区域照原样灌进真库（证据 S），再拿生产那只准入编排判演示动线两段：SHA→SZX 只适用装载出发，
// SZX→SIN 另加跨关务区域移动（票 demo-chain-gaps/31 演示判据）。读的是 scripts/demo-seeds 里的文件本身——种子改了
// 形状或节点代码，这里先红。
// Covers: AT-CC-411、AT-CC-412 的演示种子半边——区域读数取自随产品发出的种子文件。
func TestDemoNodeZoneSeedsDriveTheAdmissionApplicability(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registrar, err := buildRegistrar(db)
	if err != nil {
		t.Fatalf("装配登记口：%v", err)
	}
	ctx := t.Context()

	seeds, err := filepath.Glob(filepath.Join("..", "..", "scripts", "demo-seeds", "data", "customs", "*-node-zone-*.json"))
	if err != nil || len(seeds) == 0 {
		t.Fatalf("找不到节点区域种子：%v", err)
	}
	for _, seed := range seeds {
		raw, err := os.ReadFile(seed)
		if err != nil {
			t.Fatalf("读种子 %s：%v", seed, err)
		}
		for _, want := range []string{"REGISTERED", "EXISTING"} {
			message, code := execute(ctx, commandNodeZone, raw, registrar)
			if code != exitRegistered || !strings.HasSuffix(message, want) {
				t.Fatalf("%s：%s（退出码 %d），要 %s", filepath.Base(seed), message, code, want)
			}
		}
	}

	handler := admissionHandlerOn(t, db)
	for _, leg := range []struct {
		from, to string
		want     []domain.GuardedAction
	}{
		{"SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE", []domain.GuardedAction{domain.LoadingDeparture}},
		{"SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB", []domain.GuardedAction{domain.LoadingDeparture, domain.CrossCustomsMovement}},
	} {
		intent, err := domain.NewDepartureIntent(mustNode(t, leg.from), mustNode(t, leg.to))
		if err != nil {
			t.Fatalf("构造出发意图：%v", err)
		}
		tenant, _ := domain.NewTenantID("SYN-TENANT-01")
		object, _ := domain.NewDeclaredParcelReference("SYN-PCLREF-DEMO")
		var result application.JudgeActionAdmissionResult
		err = db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var handleErr error
			result, handleErr = handler.Handle(txCtx, application.JudgeActionAdmissionCommand{
				TenantID: tenant, Object: object, Intent: intent, At: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC),
			})
			return handleErr
		})
		if err != nil {
			t.Fatalf("%s→%s 判：%v", leg.from, leg.to, err)
		}
		judgement, _, ok := result.Judgement()
		if !ok {
			t.Fatalf("%s→%s 没形成判断：%s", leg.from, leg.to, result.UnreadableReason())
		}
		var actions []domain.GuardedAction
		for _, finding := range judgement.Findings() {
			actions = append(actions, finding.Action)
		}
		if len(actions) != len(leg.want) {
			t.Fatalf("%s→%s 适用 %v，要 %v", leg.from, leg.to, actions, leg.want)
		}
		for index := range actions {
			if actions[index] != leg.want[index] {
				t.Fatalf("%s→%s 适用 %v，要 %v", leg.from, leg.to, actions, leg.want)
			}
		}
	}
}

func mustNode(t *testing.T, code string) domain.NetworkNodeReference {
	t.Helper()
	node, err := domain.NewNetworkNodeReference(code)
	if err != nil {
		t.Fatalf("构造节点：%v", err)
	}
	return node
}

func admissionHandlerOn(t *testing.T, db *bentopg.DB) *application.JudgeActionAdmissionHandler {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("构造适配器：%v", err)
		}
	}
	zones, err := adapter.NewNodeCustomsZonePointView(db)
	must(err)
	scopes, err := adapter.NewObjectCustomsScopeView(db)
	must(err)
	restrictions, err := adapter.NewRestrictions(db)
	must(err)
	gates, err := adapter.NewGateConditionView(db)
	must(err)
	duty, err := adapter.NewDutyPaymentReconciliation(db)
	must(err)
	releases, err := adapter.NewReleaseOutcomeView(db)
	must(err)
	store, err := adapter.NewActionAdmissionJudgements(db)
	must(err)
	handler, err := application.NewJudgeActionAdmissionHandler(application.JudgeActionAdmissionDeps{
		Zones: zones, Scopes: scopes, Restrictions: restrictions, Conditions: gates, DutyRules: gates,
		DutyVerifications: duty, Releases: releases, Store: store, Clock: nodeZoneClock{},
	})
	must(err)
	return handler
}
