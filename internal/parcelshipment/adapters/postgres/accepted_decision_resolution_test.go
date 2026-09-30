package postgres_test

import (
	"errors"
	"testing"

	pscommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件对真实 PostgreSQL 证生产用的接受时解析回指：NewAcceptedDecisionResolution 经委托仓储
// 读已接受决定上的解析标识。空查询、非成员、查无此委托都答未形成；查询租户与来源身份不一致是写坏的查询。

func TestTheAcceptedDecisionResolutionSourceReadsTheStoredDecision(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()
	key := "req-key-resolution"
	mustInsert(t, transactor, ctx, repository, submittedShipmentRequest(t, key, "request-resolution"))
	accepted, err := mustFind(t, repository, ctx, key).Decide(acceptanceDecisionSpec(t))
	if err != nil {
		t.Fatalf("形成接受：%v", err)
	}
	identity := requestIdentity(t, key)
	mustSave(t, transactor, ctx, repository, identity, accepted)

	source, err := pscommercial.NewAcceptedDecisionResolution(repository)
	if err != nil {
		t.Fatalf("装配回指：%v", err)
	}
	member := mustBuild(t, domain.NewDeclaredParcelID, "parcel-1")
	tenant := mustBuild(t, domain.NewTenantID, "tenant-1")

	got, configured, err := source.ResolutionFor(ctx, ports.ChannelSelectionQuery{
		Tenant: tenant, Shipment: identity, Parcel: member,
	})
	if err != nil || !configured || got.String() != "RES-1" {
		t.Fatalf("回指 = %q configured=%v err=%v，want RES-1", got, configured, err)
	}

	_, _, err = source.ResolutionFor(ctx, ports.ChannelSelectionQuery{
		Tenant: tenant, Shipment: identity, Parcel: mustBuild(t, domain.NewDeclaredParcelID, "parcel-9"),
	})
	if !errors.Is(err, pscommercial.ErrAcceptanceResolutionNotFormed) {
		t.Fatalf("非成员 = %v，want 未形成", err)
	}

	_, _, err = source.ResolutionFor(ctx, ports.ChannelSelectionQuery{})
	if !errors.Is(err, pscommercial.ErrAcceptanceResolutionNotFormed) {
		t.Fatalf("空查询 = %v，want 未形成", err)
	}

	_, _, err = source.ResolutionFor(ctx, ports.ChannelSelectionQuery{
		Tenant: tenant, Shipment: requestIdentity(t, "req-key-absent"), Parcel: member,
	})
	if !errors.Is(err, pscommercial.ErrAcceptanceResolutionNotFormed) {
		t.Fatalf("查无委托 = %v，want 未形成", err)
	}

	_, _, err = source.ResolutionFor(ctx, ports.ChannelSelectionQuery{
		Tenant:   mustBuild(t, domain.NewTenantID, "tenant-other"),
		Shipment: identity,
		Parcel:   member,
	})
	if !errors.Is(err, pscommercial.ErrAcceptanceResolutionTenantMismatch) {
		t.Fatalf("租户不一致 = %v，want ErrAcceptanceResolutionTenantMismatch", err)
	}
}
