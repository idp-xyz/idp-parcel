package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// 本文件证生产装配（newSYNVerticalFixture 里的 wiredBeat）上的申报发送真把 HTTPSender 的线格式发出去。
// 投影用例不登通道，发送停在通道未登记，这一路上从来看不到正文，尤其是成版时固定的 memberWeights。

const (
	declarationWireChannel    = "SYN-CHANNEL-WIRE-01"
	declarationWireCredential = "SYN_DECLARATION_WIRE_TOKEN"
	declarationWireToken      = "syn-secret-01"
	declarationWireProcedure  = "SYN-PROCEDURE-WIRE-01"
	declarationWireMemberOne  = "SYN-PARCEL-WIRE-01"
	declarationWireMemberTwo  = "SYN-PARCEL-WIRE-02"
	declarationWireMessage    = "SYN-BROKER-MSG-01"
	declarationWireWeigh      = "SYN-WEIGH-WIRE-01"
)

func TestTheWiredDeclarationSendSpeaksTheProductWireFormat(t *testing.T) {
	t.Setenv(declarationWireCredential, declarationWireToken)
	measuredAt := time.Date(2026, 10, 21, 9, 15, 0, 123456000, time.UTC)

	t.Run("有成员实测毛重", func(t *testing.T) {
		weight, err := ccdomain.NewMemberMeasuredWeight(
			mustCC(t, ccdomain.NewDeclaredParcelReference, declarationWireMemberOne),
			"2.10", "KG", declarationWireWeigh, measuredAt,
		)
		if err != nil {
			t.Fatalf("实测毛重：%v", err)
		}
		hit := dispatchWiredDeclaration(t, "SYN-DECL-UNIT-WIRE-01", "SYN-DECL-VER-WIRE-01", []ccdomain.MemberMeasuredWeight{weight})
		if hit.method != http.MethodPost || hit.contentType != "application/json" ||
			hit.idempotency != "SYN-TENANT-01/SYN-DECL-VER-WIRE-01/1" ||
			hit.authorization != "Bearer "+declarationWireToken {
			t.Fatalf("请求 %s %s %s %s", hit.method, hit.contentType, hit.idempotency, hit.authorization)
		}
		var body map[string]any
		if err := json.Unmarshal(hit.body, &body); err != nil {
			t.Fatalf("正文：%v %s", err, hit.body)
		}
		assertDeclarationWireKeys(t, body)
		if body["tenant"] != "SYN-TENANT-01" || body["version"] != "SYN-DECL-VER-WIRE-01" || body["sequence"] != float64(1) ||
			body["unit"] != "SYN-DECL-UNIT-WIRE-01" || body["procedure"] != declarationWireProcedure ||
			body["dossier"] != "SYN-DOSSIER-WIRE-01" || body["roles"] != "SYN-ROLES-WIRE-01" ||
			body["target"] != declarationWireChannel {
			t.Fatalf("顶层 = %#v", body)
		}
		members, _ := body["members"].([]any)
		if len(members) != 2 || members[0] != declarationWireMemberOne || members[1] != declarationWireMemberTwo {
			t.Fatalf("成员 = %#v", body["members"])
		}
		weights, _ := body["memberWeights"].([]any)
		if len(weights) != 1 {
			t.Fatalf("memberWeights = %#v", body["memberWeights"])
		}
		item, _ := weights[0].(map[string]any)
		if item["parcel"] != declarationWireMemberOne || item["grossWeight"] != "2.10" || item["grossWeightUnit"] != "KG" ||
			item["measurementSource"] != declarationWireWeigh || item["measuredAt"] != "2026-10-21T09:15:00.123456Z" {
			t.Fatalf("毛重 = %#v", item)
		}
	})

	t.Run("成版时无任何成员实测", func(t *testing.T) {
		hit := dispatchWiredDeclaration(t, "SYN-DECL-UNIT-WIRE-02", "SYN-DECL-VER-WIRE-02", nil)
		if !bytes.Contains(hit.body, []byte(`"memberWeights":[]`)) {
			t.Fatalf("正文 = %s，want 字面空数组", hit.body)
		}
	})
}

func assertDeclarationWireKeys(t *testing.T, body map[string]any) {
	t.Helper()
	want := []string{"tenant", "version", "sequence", "unit", "procedure", "members", "dossier", "roles", "target", "memberWeights"}
	if len(body) != len(want) {
		t.Fatalf("顶层键 %d 个：%v", len(body), body)
	}
	for _, key := range want {
		if _, ok := body[key]; !ok {
			t.Fatalf("缺 %s，正文键 = %v", key, body)
		}
	}
}

type declarationWireHit struct {
	method, path, contentType, idempotency, authorization string
	body                                                  []byte
}

type declarationWireCapture struct {
	mu   sync.Mutex
	hits []declarationWireHit
}

func (capture *declarationWireCapture) record(hit declarationWireHit) {
	capture.mu.Lock()
	capture.hits = append(capture.hits, hit)
	capture.mu.Unlock()
}

func (capture *declarationWireCapture) only() (declarationWireHit, int) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if len(capture.hits) != 1 {
		return declarationWireHit{}, len(capture.hits)
	}
	return capture.hits[0], 1
}

func newDeclarationWireServer(t *testing.T) (*httptest.Server, *declarationWireCapture) {
	t.Helper()
	capture := &declarationWireCapture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		capture.record(declarationWireHit{
			method: r.Method, path: r.URL.Path, contentType: r.Header.Get("Content-Type"),
			idempotency: r.Header.Get("Idempotency-Key"), authorization: r.Header.Get("Authorization"),
			body: body,
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"outcome":"ACCEPTED","messageId":"` + declarationWireMessage + `"}`))
	}))
	t.Cleanup(server.Close)
	return server, capture
}

func dispatchWiredDeclaration(t *testing.T, unit, version string, weights []ccdomain.MemberMeasuredWeight) declarationWireHit {
	t.Helper()
	server, capture := newDeclarationWireServer(t)
	fixture := newSYNVerticalFixture(t)
	eventID := enqueueWiredDeclaration(t, fixture, server.URL, unit, version, weights)
	published, err := fixture.beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("派发：%v", err)
	}
	if status := outboxStatus(t, fixture.db, eventID); status != "PUBLISHED" {
		t.Fatalf("信封 %s published=%d status=%s 失败码=%q",
			eventID, published, status, recordedFailureCode(t, fixture.db, eventID))
	}
	// 尝试行登记之后不改写。越过发送边界的判断记在发送事实上，同步受理就是 ACKNOWLEDGED。
	attempt, outcome, message := latestDeclarationAttempt(t, fixture, version)
	if message != declarationWireMessage || outcome != "ACKNOWLEDGED" || attempt != "REGISTERED_PENDING_SEND" {
		t.Fatalf("尝试 %s 发送事实 %s %s", attempt, outcome, message)
	}
	hit, n := capture.only()
	if n != 1 {
		t.Fatalf("收到 %d 次，want 1 次 POST", n)
	}
	if hit.method != http.MethodPost {
		t.Fatalf("方法 %s 路径 %s", hit.method, hit.path)
	}
	return hit
}

func enqueueWiredDeclaration(
	t *testing.T,
	fixture *synVerticalFixture,
	endpoint, unitID, versionID string,
	weights []ccdomain.MemberMeasuredWeight,
) string {
	t.Helper()
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	submissions, err := ccpostgres.NewDeclarationSubmissions(fixture.db)
	if err != nil {
		t.Fatalf("构造申报链库：%v", err)
	}
	handoff, err := ccpostgres.NewOutboxDeclarationSubmissionHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造提交交接：%v", err)
	}
	sends, err := ccpostgres.NewSubmissionSends(fixture.db)
	if err != nil {
		t.Fatalf("构造发送册：%v", err)
	}
	readinessBook, err := ccpostgres.NewReadinessRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造就绪册：%v", err)
	}
	authorityBook, err := ccpostgres.NewSubmissionAuthorityRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造授权册：%v", err)
	}

	tenant := mustCC(t, ccdomain.NewTenantID, fixture.identity.TenantID().String())
	procedure := mustCC(t, ccdomain.NewCustomsProcedureReference, declarationWireProcedure)
	unit := mustCC(t, ccdomain.NewDeclarationUnitID, unitID)
	fixedAt := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	customsCase := mustCC(t, ccdomain.NewCustomsCaseID, "SYN-CASE-WIRE-01")
	formed, err := ccdomain.FormDeclarationUnit(unit, customsCase, procedure, []ccdomain.DeclaredParcelReference{
		mustCC(t, ccdomain.NewDeclaredParcelReference, declarationWireMemberOne),
		mustCC(t, ccdomain.NewDeclaredParcelReference, declarationWireMemberTwo),
	})
	if err != nil {
		t.Fatalf("构造申报单元：%v", err)
	}
	judgment, err := ccdomain.JudgeReady(unit, mustCC(t, ccdomain.NewReadinessBasisReference, "SYN-READINESS-WIRE-01"), fixedAt.Add(-time.Hour))
	if err != nil {
		t.Fatalf("构造就绪判断：%v", err)
	}
	authorization, err := ccdomain.GrantSubmissionAuthority(unit, mustCC(t, ccdomain.NewSubmissionAuthorityReference, "SYN-AUTHORITY-WIRE-01"), fixedAt.Add(-time.Hour))
	if err != nil {
		t.Fatalf("构造提交授权：%v", err)
	}
	version, err := ccdomain.FixSubmissionVersion(ccdomain.CustomsSubmissionVersionSpec{
		ID:            mustCC(t, ccdomain.NewSubmissionVersionID, versionID),
		Unit:          formed,
		Dossier:       mustCC(t, ccdomain.NewDossierSnapshotReference, "SYN-DOSSIER-WIRE-01"),
		Roles:         mustCC(t, ccdomain.NewRoleSnapshotReference, "SYN-ROLES-WIRE-01"),
		Readiness:     judgment,
		Authorization: authorization,
		FixedAt:       fixedAt,
		MemberWeights: weights,
	})
	if err != nil {
		t.Fatalf("固定提交版本：%v", err)
	}
	attempt, err := ccdomain.InitialAttempt(version, declarationWireChannel, ccdomain.AttemptRegisteredPendingSend, time.Time{})
	if err != nil {
		t.Fatalf("构造首次尝试：%v", err)
	}
	channel, err := ccdomain.NewDeclarationChannel(declarationWireChannel, endpoint, declarationWireCredential)
	if err != nil {
		t.Fatalf("构造通道：%v", err)
	}
	record := ccports.DeclarationSubmissionRecord{
		Key: ccports.DeclarationSubmissionKey{
			TenantID: tenant, Unit: unit, Procedure: procedure,
		},
		ContentDigest: "SYN-DECL-DIGEST-WIRE-01",
		Version:       version,
		Attempt:       attempt,
		RecordedAt:    fixedAt,
	}
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		if saved, err := readinessBook.RegisterReadiness(txCtx, tenant, judgment); err != nil || saved != ccports.CaseConfigurationRegistered {
			return fmt.Errorf("readiness %v %w", saved, err)
		}
		if saved, err := authorityBook.GrantSubmissionAuthority(txCtx, tenant, authorization); err != nil || saved != ccports.CaseConfigurationRegistered {
			return fmt.Errorf("authority %v %w", saved, err)
		}
		if effect, err := sends.SaveDeclarationChannel(txCtx, tenant, channel, fixedAt); err != nil || effect != ccports.SubmissionSendRegistered {
			return fmt.Errorf("channel %v %w", effect, err)
		}
		outcome, err := submissions.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ccports.DeclarationSubmissionSaved {
			return fmt.Errorf("save outcome %v", outcome)
		}
		return handoff.HandOffDeclarationSubmission(txCtx, ccports.DeclarationSubmissionHandoffIntent{Record: record, Case: customsCase})
	})
	return string(outboxintent.FingerprintEventID("declaration-submission",
		tenant.String(), unitID, declarationWireProcedure, versionID))
}

func latestDeclarationAttempt(t *testing.T, fixture *synVerticalFixture, version string) (attempt, outcome, message string) {
	t.Helper()
	err := fixture.pool.QueryRow(t.Context(),
		`SELECT a.result, COALESCE(f.outcome, ''), COALESCE(f.channel_message, '')
		   FROM customs_compliance.submission_attempt AS a
		   LEFT JOIN customs_compliance.submission_send_fact AS f
		     ON f.tenant_id = a.tenant_id AND f.version_id = a.version_id AND f.sequence = a.sequence
		  WHERE a.tenant_id = $1 AND a.version_id = $2
		  ORDER BY a.sequence DESC
		  LIMIT 1`,
		fixture.identity.TenantID().String(), version,
	).Scan(&attempt, &outcome, &message)
	if err != nil {
		t.Fatalf("读最新尝试：%v", err)
	}
	return attempt, outcome, message
}
