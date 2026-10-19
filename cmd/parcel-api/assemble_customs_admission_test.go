package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	customsdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// unwiredNodeCustomsZoneRegistration 是端点表装配测试里节点关务区域那一格的占位：未配置与渠道换口两条用例都钉在
// 编排之前，走到它就是 Intake 漏了一道。
type unwiredNodeCustomsZoneRegistration struct{}

func (unwiredNodeCustomsZoneRegistration) Handle(
	context.Context,
	customsapp.RegisterNodeCustomsZoneCommand,
) (customsapp.CaseConfigurationOutcome, error) {
	return customsapp.CaseConfigurationOutcomeInvalid, errOrchestrationNotWired
}

// 节点关务区域登记口挂操作者渠道的登记册 Intake：没令牌 401；有授予而批文自报租户 400——租户只从信封来。
func TestNodeCustomsZoneRegistrationFaceAnswersFromTheOperatorChannel(t *testing.T) {
	minter, err := buildOperatorMinter(acceptingVerifier{}, registryWriterRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	registries, err := buildOperatorRegistryIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnwiredBusinessEndpointsWithOperatorIntakes(
		unconfiguredOperatorDecisions(), registries, unconfiguredIntegrationClientIntakes(), nil, nil, nil, nil))
	for name, testCase := range map[string]struct {
		token  string
		status int
		code   string
	}{
		"no bearer token":               {"", http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		"granted, tenant self-reported": {"presented.operator.token", http.StatusBadRequest, "MALFORMED_REQUEST"},
	} {
		request := httptest.NewRequest(http.MethodPost, "/customs-node-zone-registrations", strings.NewReader(`{"tenantId": "SYN-TENANT-02"}`))
		if testCase.token != "" {
			request.Header.Set("Authorization", "Bearer "+testCase.token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != testCase.status || problemCode(t, response) != testCase.code {
			t.Fatalf("%s: %d %q, want %d %q", name, response.Code, problemCode(t, response), testCase.status, testCase.code)
		}
	}
}

// 生产装配那两格对真库走通（隔离合成 S）：区域登记在自己的事务里落地并能重放；准入判断不靠调用方的事务，自己开一笔、
// 记下判断、重放回到同一份。
func TestCustomsAdmissionOrchestrationRunsOnRealPostgres(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	orchestration, err := buildCustomsAdmissionOrchestration(db)
	if err != nil {
		t.Fatalf("装配：%v", err)
	}
	ctx := t.Context()
	tenant, _ := customsdomain.NewTenantID("SYN-TENANT-01")
	node := func(code string) customsdomain.NetworkNodeReference {
		built, err := customsdomain.NewNetworkNodeReference(code)
		if err != nil {
			t.Fatalf("构造节点：%v", err)
		}
		return built
	}

	for _, zone := range []struct{ node, zone string }{
		{"SYN-NODE-SHA-HUB", "SYN-ZONE-CN"}, {"SYN-NODE-SZX-GATE", "SYN-ZONE-CN"},
	} {
		zoneRef, _ := customsdomain.NewCustomsZoneReference(zone.zone)
		command := customsapp.RegisterNodeCustomsZoneCommand{
			TenantID: tenant, Node: node(zone.node), Zone: zoneRef, AppliesFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		for _, want := range []customsapp.CaseConfigurationOutcome{customsapp.ConfigurationRegistered, customsapp.ConfigurationExisting} {
			outcome, err := orchestration.nodeZoneRegistration.Handle(ctx, command)
			if err != nil || outcome != want {
				t.Fatalf("登 %s：%s / %v，要 %s", zone.node, outcome, err, want)
			}
		}
	}

	intent, err := customsdomain.NewDepartureIntent(node("SYN-NODE-SHA-HUB"), node("SYN-NODE-SZX-GATE"))
	if err != nil {
		t.Fatalf("构造出发意图：%v", err)
	}
	object, _ := customsdomain.NewDeclaredParcelReference("SYN-PCLREF-ASSEMBLY")
	command := customsapp.JudgeActionAdmissionCommand{
		TenantID: tenant, Object: object, Intent: intent, At: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC),
	}
	first, err := orchestration.actionAdmission.Handle(ctx, command)
	if err != nil || first.Outcome() != customsapp.ActionAdmissionJudged {
		t.Fatalf("首判：%s / %v", first.Outcome(), err)
	}
	judgement, id, _ := first.Judgement()
	if judgement.Answer() != customsdomain.AdmissionNotGuarded {
		t.Fatalf("同区出发、对象不在册：%s", judgement.Answer())
	}
	replay, err := orchestration.actionAdmission.Handle(ctx, command)
	_, replayID, _ := replay.Judgement()
	if err != nil || replay.Outcome() != customsapp.ActionAdmissionExisting || replayID != id {
		t.Fatalf("重放：%s / %s / %v", replay.Outcome(), replayID, err)
	}
}
