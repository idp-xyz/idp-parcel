package nodeopshttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件证节点作业查阅页的节点控制转移册（registry=physical-control，ADR-0214 决定三）的传输面，并给既有的几只查阅目录替身补上
// 这一问。

// controlTransitionsCatalogue 在查阅目录替身外面补上转移册那一问。
type controlTransitionsCatalogue struct {
	*stubReviewCatalogue
	transitions []ports.PhysicalControlTransitionCatalogueRow
}

func (catalogue controlTransitionsCatalogue) ListPhysicalControlTransitions(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.PhysicalControlTransitionCatalogueRow, error) {
	catalogue.gotTenant, catalogue.gotLimit = tenant.String(), limit
	return catalogue.transitions, catalogue.err
}

// ListPhysicalControlTransitions 让既有的查阅目录替身也满足读口：它们只测别的册子，这一问交回空。
func (stub *stubReviewCatalogue) ListPhysicalControlTransitions(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.PhysicalControlTransitionCatalogueRow, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return nil, stub.err
}

func (stub unreachableReviewCatalogue) ListPhysicalControlTransitions(
	context.Context, domain.TenantID, int,
) ([]ports.PhysicalControlTransitionCatalogueRow, error) {
	stub.refuse("physical control transition")
	return nil, nil
}

func (catalogue reviewUnitsCatalogue) ListPhysicalControlTransitions(
	ctx context.Context, tenant domain.TenantID, limit int,
) ([]ports.PhysicalControlTransitionCatalogueRow, error) {
	return catalogue.stubReviewCatalogue.ListPhysicalControlTransitions(ctx, tenant, limit)
}

// Covers: `AT-NO-112` 的传输面（ADR-0214 决定三、五）——registry=physical-control 照行转写：转出那一行带控制四件与转出两件，没推进
// 的那一行带原因；不在场的格整键不出现，不以空串或 null 兼表。读口收到的租户与页大小来自 Intake。
func TestPhysicalControlTransitionsAreListedRowByRow(t *testing.T) {
	established := catalogueBaseAt
	transferred := catalogueBaseAt.Add(3 * time.Hour)
	catalogue := controlTransitionsCatalogue{stubReviewCatalogue: &stubReviewCatalogue{}, transitions: []ports.PhysicalControlTransitionCatalogueRow{
		{
			HandoverObject: "SYN-PARCEL-61", HandoverScope: "SYN-SCOPE-61", HandoverVersion: "v2",
			Direction: "OUT", Outcome: "NOT_ADVANCED", Reason: "CONTROL_SOURCE_CHAIN_PENDING_RECOMPUTE",
			Unit: "SYN-HU-61", Node: "SYN-NODE-SHA-HUB", ControlKind: "NODE_INTAKE", ControlBasis: "NODE-INTAKE/v-61",
			ControlEstablishedAt: &established, TransferReference: "TRANSPORT-HANDOVER/v1", TransferredAt: &transferred,
			OccurredAt: transferred, RecordedAt: transferred.Add(2 * time.Hour),
		},
		{
			HandoverObject: "SYN-PARCEL-61-X", HandoverScope: "SYN-SCOPE-61", HandoverVersion: "v1",
			Direction: "OUT", Outcome: "NOT_ADVANCED", Reason: "NO_ASSOCIATED_UNIT",
			OccurredAt: transferred, RecordedAt: transferred.Add(time.Hour),
		},
	}}
	response := serveGet(nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(grantedIntake(), catalogue),
		"/node-operations-records?registry=physical-control")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome     string                       `json:"outcome"`
		Transitions []map[string]json.RawMessage `json:"transitions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.String(), err)
	}
	if body.Outcome != "PHYSICAL_CONTROL_TRANSITIONS_LISTED" || len(body.Transitions) != 2 {
		t.Fatalf("答复 = %s", response.Body.String())
	}
	if catalogue.gotTenant != "TENANT-1" || catalogue.gotLimit != 25 {
		t.Fatalf("读口收到 %q / %d, want Intake 给的租户与页大小", catalogue.gotTenant, catalogue.gotLimit)
	}
	recompute, unresolved := body.Transitions[0], body.Transitions[1]
	for key, want := range map[string]string{
		"handoverVersion": `"v2"`, "direction": `"OUT"`, "outcome": `"NOT_ADVANCED"`,
		"reason": `"CONTROL_SOURCE_CHAIN_PENDING_RECOMPUTE"`, "unit": `"SYN-HU-61"`, "node": `"SYN-NODE-SHA-HUB"`,
		"controlKind": `"NODE_INTAKE"`, "controlBasis": `"NODE-INTAKE/v-61"`, "transferReference": `"TRANSPORT-HANDOVER/v1"`,
		"controlEstablishedAt": `"2026-08-28T09:00:00Z"`, "transferredAt": `"2026-08-28T12:00:00Z"`,
		"occurredAt": `"2026-08-28T12:00:00Z"`, "recordedAt": `"2026-08-28T14:00:00Z"`,
	} {
		if got := string(recompute[key]); got != want {
			t.Fatalf("待重算那一行 %s = %s, want %s", key, got, want)
		}
	}
	if string(unresolved["reason"]) != `"NO_ASSOCIATED_UNIT"` {
		t.Fatalf("对不上实物那一行 reason = %s", unresolved["reason"])
	}
	for _, absent := range []string{"unit", "node", "controlKind", "controlBasis", "controlEstablishedAt", "transferReference", "transferredAt"} {
		if _, present := unresolved[absent]; present {
			t.Fatalf("对不上实物那一行不该带 %s：%s", absent, response.Body.String())
		}
	}
}

// Covers: ADR-0022 与分派参数的封闭集——转移册一样经 Intake 认证（未配置 403、读口不被触到），读库失败答 500 不带业务结果格。
func TestPhysicalControlTransitionsKeepTheEndpointsGrades(t *testing.T) {
	refused := serveGet(nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(
		nodeopshttp.UnconfiguredIntake{}, unreachableReviewCatalogue{t: t}), "/node-operations-records?registry=physical-control")
	if refused.Code != http.StatusForbidden || problemCode(t, refused) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("未配置：%d %s", refused.Code, refused.Body.String())
	}
	assertNoOutcome(t, refused)

	failing := serveGet(nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(
		grantedIntake(), &stubReviewCatalogue{err: errors.New("register is down")}), "/node-operations-records?registry=physical-control")
	if failing.Code != http.StatusInternalServerError || problemCode(t, failing) != "NO_ANSWER_FORMED" {
		t.Fatalf("读库失败：%d %s", failing.Code, failing.Body.String())
	}
	assertNoOutcome(t, failing)
}
