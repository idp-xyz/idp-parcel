package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件对真实 PostgreSQL 16 证 ports.DeclaredMeasurementView 的封闭五格（pp-seams/02 完成判据 2 与裁决 6）：基线锚带测量 /
// 已采用版本锚只带锚 / `待复核`未定 / 成员无画像答未申报 / 非委托对象答无。夹具与收件地点引用的同款：真接受（Decide →
// Save）而不是只翻 state 列，读口要走的是接受基线的成员集合与基线所指那一版上的画像，只翻列的行没有基线也没有画像。
//
// submittedShipmentRequest 给 parcel-1 一张 2.5 kg / 30×20×10 cm 的画像、parcel-2 无画像——「真库一正一缺」两格由同一份委托
// 的两个成员各占一格。

var _ ports.DeclaredMeasurementView = (*adapter.ShipmentRequests)(nil)

// acceptedRequestWithMeasurementChain 插入并真接受夹具委托，再按给定链在 **parcel-1 的申报测量范围**（按包裹指名、
// DeclaredMeasurementDataGroup）上逐份追加版本并保存：每项 [版本, 前版]，前版为空即以接受基线为基准的补充。
func acceptedRequestWithMeasurementChain(
	t *testing.T,
	repository *adapter.ShipmentRequests,
	transactor bentoapp.Transactor,
	key, requestID string,
	chain ...[2]string,
) {
	t.Helper()
	ctx := t.Context()
	mustInsert(t, transactor, ctx, repository, submittedShipmentRequest(t, key, requestID))
	accepted, err := mustFind(t, repository, ctx, key).Decide(acceptanceDecisionSpec(t))
	if err != nil {
		t.Fatalf("形成接受：%v", err)
	}
	mustSave(t, transactor, ctx, repository, requestIdentity(t, key), accepted)

	scope, err := domain.NewParcelScopedSourceData(
		mustBuild(t, domain.NewShipmentRequestID, requestID),
		mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"),
		domain.DeclaredMeasurementDataGroup(),
	)
	if err != nil {
		t.Fatalf("申报测量资料范围：%v", err)
	}
	for index, link := range chain {
		current := mustFind(t, repository, ctx, key)
		basis := domain.NewSupplementOnAcceptanceBaseline()
		intent := domain.SupplementIntent
		if link[1] != "" {
			if basis, err = domain.NewAmendmentOfVersion(mustBuild(t, domain.NewSourceDataVersionID, link[1])); err != nil {
				t.Fatalf("前版基准：%v", err)
			}
			intent = domain.CorrectionIntent
		}
		version, err := domain.FormCustomerSourceDataVersion(domain.CustomerSourceDataVersionSpec{
			VersionID: mustBuild(t, domain.NewSourceDataVersionID, link[0]),
			Scope:     scope,
			Basis:     basis,
			Intent:    intent,
			Request:   requestFingerprint(t, key+"-amend-"+link[0], "digest-"+link[0]),
			Reason:    mustBuild(t, domain.NewAmendmentReasonReference, "WEIGHT_RESTATED"),
			Requester: mustBuild(t, domain.NewRequesterReference, "CUSTOMER-1"),
			Decider:   mustBuild(t, domain.NewDeciderReference, "OPERATOR-1"),
			Authority: mustBuild(t, domain.NewAmendmentAuthoritySnapshot, "AUTH-SNAP-1"),
			FormedAt:  submittedAtFixture.Add(time.Duration(index+2) * time.Hour),
		})
		if err != nil {
			t.Fatalf("形成资料版本 %q：%v", link[0], err)
		}
		amended, err := current.AmendCustomerSourceData(version)
		if err != nil {
			t.Fatalf("追加资料版本 %q：%v", link[0], err)
		}
		mustSave(t, transactor, ctx, repository, requestIdentity(t, key), amended)
	}
}

func loadDeclaredMeasurement(
	t *testing.T,
	view ports.DeclaredMeasurementView,
	tenant, parcel string,
) domain.DeclaredMeasurementResolution {
	t.Helper()
	resolution, err := view.LoadDeclaredMeasurement(t.Context(),
		mustBuild(t, domain.NewTenantID, tenant), mustBuild(t, domain.NewDeclaredParcelID, parcel))
	if err != nil {
		t.Fatalf("取申报测量（%s / %s）：%v", tenant, parcel, err)
	}
	return resolution
}

func TestAMemberWithABaselineProfileAnswersItsDeclaredMeasurementFromTheSnapshot(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithMeasurementChain(t, repository, transactor, "req-key-1", "request-1")

	resolution := loadDeclaredMeasurement(t, repository, "tenant-1", "parcel-1")
	if resolution.Outcome() != domain.DeclaredMeasurementAnchoredOnBaseline {
		t.Fatalf("outcome = %q, want ANCHORED_ON_BASELINE", resolution.Outcome())
	}
	measurement, declared := resolution.Measurement()
	if !declared {
		t.Fatal("基线锚那一格没带测量")
	}
	if got := measurement.Weight(); got.Value().String() != "2.5" || got.Unit().String() != "kg" {
		t.Fatalf("weight = %s %s, want 2.5 kg（客户原样，单位保持 PS 自由串）", got.Value(), got.Unit())
	}
	dimensions, present := measurement.Dimensions()
	if !present || dimensions.Length().String() != "30" || dimensions.Width().String() != "20" ||
		dimensions.Height().String() != "10" || dimensions.Unit().String() != "cm" {
		t.Fatalf("dimensions = %#v present = %v, want 30×20×10 cm", dimensions, present)
	}
	anchor, anchored := resolution.Anchor()
	if !anchored || !anchor.OnAcceptanceBaseline() {
		t.Fatalf("anchor = %#v anchored = %v, want the acceptance baseline anchor", anchor, anchored)
	}
}

// 「真库一正一缺」的缺：同一份委托的第二个成员在基线上没有画像，如实答未申报，不拿 parcel-1 的测量顶。
func TestAMemberWithoutABaselineProfileAnswersNotDeclaredFromTheSnapshot(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithMeasurementChain(t, repository, transactor, "req-key-1", "request-1")

	resolution := loadDeclaredMeasurement(t, repository, "tenant-1", "parcel-2")
	if resolution.Outcome() != domain.DeclaredMeasurementNotDeclared {
		t.Fatalf("outcome = %q, want NOT_DECLARED", resolution.Outcome())
	}
	if _, declared := resolution.Measurement(); declared {
		t.Fatal("没申报的成员凭空长出了测量")
	}
}

func TestAnAdoptedMeasurementAmendmentMovesTheAnchorAndWithholdsTheValue(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithMeasurementChain(t, repository, transactor, "req-key-1", "request-1",
		[2]string{"measure-v1", ""},
		[2]string{"measure-v2", "measure-v1"},
	)

	resolution := loadDeclaredMeasurement(t, repository, "tenant-1", "parcel-1")
	if resolution.Outcome() != domain.DeclaredMeasurementAnchoredOnAdoptedVersion {
		t.Fatalf("outcome = %q, want ANCHORED_ON_ADOPTED_VERSION", resolution.Outcome())
	}
	anchor, anchored := resolution.Anchor()
	adopted, onVersion := anchor.AdoptedVersion()
	if !anchored || !onVersion || adopted.String() != "measure-v2" {
		t.Fatalf("anchor = %q, want measure-v2（链尾那一版）", adopted)
	}
	if measurement, declared := resolution.Measurement(); declared {
		t.Fatalf("已采用版本锚交出了测量 %#v——那是基线值，客户已经更正过它", measurement)
	}

	// 同一委托的另一成员不受 parcel-1 范围上修订的影响：范围按包裹指名。
	other := loadDeclaredMeasurement(t, repository, "tenant-1", "parcel-2")
	if other.Outcome() != domain.DeclaredMeasurementNotDeclared {
		t.Fatalf("parcel-2 outcome = %q, want NOT_DECLARED（parcel-1 的修订动不到它）", other.Outcome())
	}
}

func TestAForkedMeasurementAmendmentChainAnswersUndeterminedWithoutAValue(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithMeasurementChain(t, repository, transactor, "req-key-1", "request-1",
		[2]string{"measure-v1", ""},
		[2]string{"measure-v2", "measure-v1"},
		[2]string{"measure-v3", "measure-v1"},
	)

	resolution := loadDeclaredMeasurement(t, repository, "tenant-1", "parcel-1")
	if resolution.Outcome() != domain.DeclaredMeasurementUndetermined {
		t.Fatalf("outcome = %q, want UNDETERMINED", resolution.Outcome())
	}
	if _, declared := resolution.Measurement(); declared {
		t.Fatal("`待复核`交出了测量——任选一版就是在读口上偷做了那次合并")
	}
	if _, anchored := resolution.Anchor(); anchored {
		t.Fatal("`待复核`交出了锚")
	}
}

func TestAnObjectOutsideEveryAcceptedBaselineHasNoDeclaredMeasurementInTheStore(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithMeasurementChain(t, repository, transactor, "req-key-1", "request-1")

	cases := map[string][2]string{
		"从未声明过的包裹":        {"tenant-1", "parcel-9"},
		"他租户问本租户已接受委托的成员": {"tenant-b", "parcel-1"},
	}
	for name, item := range cases {
		t.Run(name, func(t *testing.T) {
			resolution := loadDeclaredMeasurement(t, repository, item[0], item[1])
			if resolution.Outcome() != domain.NoDeclaredMeasurement {
				t.Fatalf("outcome = %q, want NO_DECLARED_MEASUREMENT", resolution.Outcome())
			}
			if _, declared := resolution.Measurement(); declared {
				t.Fatal("「无」那一格带了测量")
			}
		})
	}
}

// 责任起点在接受之后：仅`已提交`的委托没有基线，它的成员今天没有可交出的申报测量——即便画像已在快照里。
func TestAMemberOfAMerelySubmittedRequestHasNoDeclaredMeasurement(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	mustInsert(t, transactor, t.Context(), repository, submittedShipmentRequest(t, "req-key-1", "request-1"))

	resolution := loadDeclaredMeasurement(t, repository, "tenant-1", "parcel-1")
	if resolution.Outcome() != domain.NoDeclaredMeasurement {
		t.Fatalf("outcome = %q, want NO_DECLARED_MEASUREMENT", resolution.Outcome())
	}
}

// 两份已接受委托同时声明同一件包裹是读面坏了（ADR-0060 的歧义），不是五格里的任何一格。
func TestTwoAcceptedRequestsClaimingOneParcelAreAReadFaceFailureForMeasurement(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithMeasurementChain(t, repository, transactor, "req-key-1", "request-1")
	acceptedRequestWithMeasurementChain(t, repository, transactor, "req-key-2", "request-2")

	resolution, err := repository.LoadDeclaredMeasurement(t.Context(),
		mustBuild(t, domain.NewTenantID, "tenant-1"), mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"))
	if !errors.Is(err, domain.ErrAmbiguousParcelTarget) {
		t.Fatalf("err = %v, want ErrAmbiguousParcelTarget", err)
	}
	if resolution != (domain.DeclaredMeasurementResolution{}) {
		t.Fatalf("上抛的同时还交回了答复 %#v", resolution)
	}
}

func TestDeclaredMeasurementLookupRequiresTenantAndParcel(t *testing.T) {
	repository, _, _ := newShipmentRequests(t)
	cases := []struct {
		name   string
		tenant domain.TenantID
		parcel domain.DeclaredParcelID
	}{
		{"缺租户", domain.TenantID{}, mustBuild(t, domain.NewDeclaredParcelID, "parcel-1")},
		{"缺包裹", mustBuild(t, domain.NewTenantID, "tenant-1"), domain.DeclaredParcelID{}},
	}
	for _, item := range cases {
		resolution, err := repository.LoadDeclaredMeasurement(context.Background(), item.tenant, item.parcel)
		if err == nil {
			t.Fatalf("%s 没有上抛，而是拿残缺的键去查了：%#v", item.name, resolution)
		}
		if resolution != (domain.DeclaredMeasurementResolution{}) {
			t.Fatalf("%s 上抛的同时还交回了答复 %#v", item.name, resolution)
		}
	}
}
