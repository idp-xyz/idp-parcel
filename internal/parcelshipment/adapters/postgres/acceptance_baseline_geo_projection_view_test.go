package postgres_test

import (
	"errors"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件对真实 PostgreSQL 16 证 ports.AcceptanceBaselineGeoProjectionView（ADR-0301，票 demo-chain-gaps/143）：按（租户，包裹）
// 答接受基线那一版提交的地理投影，答案与在整份聚合上取（接受基线 → 基线所指那一版 → GeoProjection）一字不差，而读口
// 不经手聚合的其余部分。夹具是真接受（Decide → Save），不只翻 state 列；自相矛盾的几格由夹具直写快照造出。

var _ ports.AcceptanceBaselineGeoProjectionView = (*adapter.ShipmentRequests)(nil)

func loadBaselineGeo(
	t *testing.T,
	repository *adapter.ShipmentRequests,
	tenant, parcel string,
) (domain.AcceptanceBaselineGeoProjection, bool, error) {
	t.Helper()
	return repository.LoadAcceptanceBaselineGeoProjection(t.Context(),
		mustBuild(t, domain.NewTenantID, tenant), mustBuild(t, domain.NewDeclaredParcelID, parcel))
}

// 读口的答案就是整份聚合会给出的那一份：委托、客户账户、基线版本、基线固定时刻与基线那一版的投影逐格相同，
// 两个成员各问一次答同一份。值经快照往返原样：收件国家与邮编、寄件只报了邮编。
func TestTheBaselineGeoProjectionIsWhatTheWholeAggregateWouldGive(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedWithElementsChain(t, repository, transactor, "req-key-1", "request-1")

	request := mustFind(t, repository, t.Context(), "req-key-1")
	baseline, present := request.AcceptanceBaseline()
	if !present {
		t.Fatal("夹具委托没有接受基线")
	}
	submission, present := request.SubmissionVersionByID(baseline.SubmissionVersionID())
	if !present {
		t.Fatal("夹具委托找不到基线那一版")
	}

	for _, parcel := range []string{"parcel-1", "parcel-2"} {
		got, found, err := loadBaselineGeo(t, repository, "tenant-1", parcel)
		if err != nil || !found {
			t.Fatalf("%s: found=%v err=%v, want 当前已接受包裹", parcel, found, err)
		}
		if got.ShipmentRequestID() != request.ShipmentRequestID() ||
			got.CustomerAccountID().String() != "customer-1" ||
			got.BaselineVersion() != baseline.SubmissionVersionID() ||
			!got.FixedAt().Equal(baseline.FixedAt()) {
			t.Fatalf("%s: 引用与时刻 = %s / %s / %s / %s，want 聚合上的 %s / customer-1 / %s / %s", parcel,
				got.ShipmentRequestID(), got.CustomerAccountID(), got.BaselineVersion(), got.FixedAt(),
				request.ShipmentRequestID(), baseline.SubmissionVersionID(), baseline.FixedAt())
		}
		if got.Geo() != submission.GeoProjection() {
			t.Fatalf("%s: 投影 = %+v，want 基线那一版的 %+v", parcel, got.Geo(), submission.GeoProjection())
		}
	}

	got, _, _ := loadBaselineGeo(t, repository, "tenant-1", "parcel-1")
	if country, has := got.Geo().Delivery().Country(); !has || country != "SYN-CC" {
		t.Fatalf("收件国家 = %q/%v", country, has)
	}
	if postal, has := got.Geo().Delivery().PostalCode(); !has || postal != "SYN-100115" {
		t.Fatalf("收件邮编 = %q/%v", postal, has)
	}
	if postal, has := got.Geo().Sender().PostalCode(); !has || postal != "SYN-200000" {
		t.Fatalf("寄件邮编 = %q/%v", postal, has)
	}
	if _, has := got.Geo().Sender().Country(); has {
		t.Fatal("寄件没报国家却在场")
	}
}

// 接受之后经资料修订采用的新收件邮编不进这份投影：读的是接受基线那一版（ADR-0215 越权风险点 5 的同一处取舍）。
func TestAnAdoptedAmendmentAfterAcceptanceDoesNotMoveTheBaselineGeo(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedWithElementsChain(t, repository, transactor, "req-key-1", "request-1",
		storedContentLink{"dp-v1", "", domain.SupplementIntent, "SYN-200095"},
		storedContentLink{"dp-v2", "dp-v1", domain.CorrectionIntent, "SYN-200097"},
	)

	got, found, err := loadBaselineGeo(t, repository, "tenant-1", "parcel-1")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if postal, _ := got.Geo().Delivery().PostalCode(); postal != "SYN-100115" {
		t.Fatalf("收件邮编 = %q，want 基线那一版的 SYN-100115，不是修订采用的", postal)
	}
}

// 基线固定在历史版本上（夹具直写把首版挪进历史、另立一版当前版本；领域不让已接受委托再提交，这一形只能直写造出）：
// 重建门拒这份委托（「接受基线固定在另一个提交版本上」），读口不交历史那一版的投影，两个成员都答接受基线自相矛盾。
func TestABaselineFixedOnAPriorVersionIsInconsistent(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	acceptedWithElementsChain(t, repository, transactor, "req-key-1", "request-1")
	if _, err := pool.Exec(t.Context(),
		`UPDATE parcel_shipment.shipment_request
		    SET snapshot = jsonb_set(
		          jsonb_set(snapshot, '{priorVersions}', jsonb_build_array(snapshot -> 'currentVersion')),
		          '{currentVersion}',
		          (snapshot -> 'currentVersion')
		            || '{"versionId":"version-2","elements":{"delivery":{"postalCode":"SYN-999999"}}}'::jsonb)
		  WHERE source_request_key = $1`, "req-key-1"); err != nil {
		t.Fatalf("另立当前版本：%v", err)
	}

	assertGateRefusesAndViewNamesInconsistency(t, repository)
}

// assertGateRefusesAndViewNamesInconsistency 钉住同一件事的两面：仓储的重建门拒这份委托，读口对它的每个成员都答接受基线
// 自相矛盾——读口不把重建门会拒的委托当好答案交出。
func assertGateRefusesAndViewNamesInconsistency(t *testing.T, repository *adapter.ShipmentRequests) {
	t.Helper()
	if _, _, err := repository.FindBySourceIdentity(t.Context(), requestIdentity(t, "req-key-1")); err == nil {
		t.Fatal("重建门放行了这份委托，本用例证不了什么")
	}
	for _, parcel := range []string{"parcel-1", "parcel-2"} {
		got, found, err := loadBaselineGeo(t, repository, "tenant-1", parcel)
		if found || !errors.Is(err, domain.ErrAcceptanceBaselineInconsistent) {
			t.Fatalf("%s: found=%v err=%v, want ErrAcceptanceBaselineInconsistent", parcel, found, err)
		}
		if got != (domain.AcceptanceBaselineGeoProjection{}) {
			t.Fatalf("%s: 答矛盾还交回了 %+v", parcel, got)
		}
	}
}

// 基线那一版一格地址要素都没报：投影照样是「带了」、各格缺席，与 PS 信封交出的同一形状。
func TestABaselineVersionWithoutElementsStillCarriesAProjection(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithAddressChain(t, repository, transactor, "req-key-1", "request-1")

	got, found, err := loadBaselineGeo(t, repository, "tenant-1", "parcel-1")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if !got.Geo().Carried() {
		t.Fatal("没报地址要素被读成了「没带投影」")
	}
	if _, has := got.Geo().Delivery().PostalCode(); has {
		t.Fatal("没报的收件邮编冒了出来")
	}
}

// 读口只取接受基线、当前提交版本的标识、成员与地址要素子段、接受决定的时刻：聚合别处坏到仓储读不回（这里弄坏的是申报
// 测量画像），读口照答。重建门里关于接受基线之外的各条不归本口（ADR-0301 越权风险点 1）。
func TestTheProjectionDoesNotReadTheRestOfTheAggregate(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	acceptedWithElementsChain(t, repository, transactor, "req-key-1", "request-1")
	if _, err := pool.Exec(t.Context(),
		`UPDATE parcel_shipment.shipment_request
		    SET snapshot = jsonb_set(snapshot, '{currentVersion,profiles}', '"not-a-list"')
		  WHERE source_request_key = $1`, "req-key-1"); err != nil {
		t.Fatalf("弄坏画像：%v", err)
	}
	if _, _, err := repository.FindBySourceIdentity(t.Context(), requestIdentity(t, "req-key-1")); err == nil {
		t.Fatal("画像坏了仓储还读得回，本用例证不了什么")
	}

	got, found, err := loadBaselineGeo(t, repository, "tenant-1", "parcel-1")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v，聚合别处坏了不该挡住只取两段的读口", found, err)
	}
	if postal, _ := got.Geo().Delivery().PostalCode(); postal != "SYN-100115" {
		t.Fatalf("收件邮编 = %q", postal)
	}
}

// 「不是」是业务答案：从未声明过、他租户、仅`已提交`的委托成员，以及投影列声明了它而接受基线里没有它，都答 found=false、
// 无错。最后一形只能是投影列与快照不一致（基线成员与当前提交版本不一致已在前面答矛盾）：成员归属以基线为准，答法与
// 其余按包裹读口相同。
func TestAnObjectThatIsNotACurrentAcceptedMemberHasNoBaselineGeo(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	acceptedWithElementsChain(t, repository, transactor, "req-key-1", "request-1")
	mustInsert(t, transactor, t.Context(), repository, submittedShipmentRequest(t, "req-key-2", "request-2"))
	acceptedWithElementsChain(t, repository, transactor, "req-key-3", "request-3")
	// request-3 的快照不动，投影列多声明一件 parcel-3。
	if _, err := pool.Exec(t.Context(),
		`UPDATE parcel_shipment.shipment_request
		    SET declared_parcel_ids = ARRAY['parcel-1', 'parcel-2', 'parcel-3']
		  WHERE source_request_key = $1`, "req-key-3"); err != nil {
		t.Fatalf("改投影列：%v", err)
	}

	cases := map[string][2]string{
		"从未声明过的包裹":        {"tenant-1", "parcel-9"},
		"他租户问本租户已接受委托的成员": {"tenant-b", "parcel-2"},
		"投影列声明了而接受基线里没有它": {"tenant-1", "parcel-3"},
	}
	for name, item := range cases {
		t.Run(name, func(t *testing.T) {
			got, found, err := loadBaselineGeo(t, repository, item[0], item[1])
			if err != nil || found {
				t.Fatalf("found=%v err=%v, want 不是、无错", found, err)
			}
			if got != (domain.AcceptanceBaselineGeoProjection{}) {
				t.Fatalf("答「不是」还交回了 %+v", got)
			}
		})
	}
}

// 两份已接受委托同时声明同一件包裹是歧义（ADR-0060），不挑一份。
func TestTwoAcceptedRequestsClaimingOneParcelAreAmbiguousForTheBaselineGeo(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedWithElementsChain(t, repository, transactor, "req-key-1", "request-1")
	acceptedWithElementsChain(t, repository, transactor, "req-key-2", "request-2")

	_, found, err := loadBaselineGeo(t, repository, "tenant-1", "parcel-1")
	if found || !errors.Is(err, domain.ErrAmbiguousParcelTarget) {
		t.Fatalf("found=%v err=%v, want ErrAmbiguousParcelTarget", found, err)
	}
	if errors.Is(err, domain.ErrAcceptanceBaselineInconsistent) {
		t.Fatal("歧义被读成了基线自相矛盾")
	}
}

// 重建门里关于接受基线的各条，哪一条不成立都答 ErrAcceptanceBaselineInconsistent，与读不动分开；每一形都先证重建门确实拒
// 这份委托，两个成员都问。基线固定在历史版本上那一形另见 TestABaselineFixedOnAPriorVersionIsInconsistent。
func TestAnInconsistentAcceptanceBaselineIsNamed(t *testing.T) {
	cases := map[string]string{
		"没有接受基线":        `snapshot - 'baseline'`,
		"基线不指当前提交版本":    `jsonb_set(snapshot, '{baseline,submissionVersion}', '"version-9"')`,
		"基线没有固定时刻":      `jsonb_set(snapshot, '{baseline,fixedAt}', '"0001-01-01T00:00:00Z"')`,
		"基线成员收窄":        `jsonb_set(snapshot, '{baseline,declaredParcelIds}', '["parcel-1"]')`,
		"基线时刻与接受决定时刻不一": `jsonb_set(snapshot, '{baseline,fixedAt}', '"2001-01-01T00:00:00Z"')`,
		"接受决定没有时刻":      `snapshot #- '{decision,decidedAt}'`,
	}
	for name, expression := range cases {
		t.Run(name, func(t *testing.T) {
			repository, transactor, pool := newShipmentRequests(t)
			acceptedWithElementsChain(t, repository, transactor, "req-key-1", "request-1")
			if _, err := pool.Exec(t.Context(),
				`UPDATE parcel_shipment.shipment_request SET snapshot = `+expression+` WHERE source_request_key = $1`,
				"req-key-1"); err != nil {
				t.Fatalf("改快照：%v", err)
			}

			assertGateRefusesAndViewNamesInconsistency(t, repository)
		})
	}
}

func TestTheBaselineGeoLookupRequiresTenantAndParcel(t *testing.T) {
	repository, _, _ := newShipmentRequests(t)
	if _, _, err := repository.LoadAcceptanceBaselineGeoProjection(t.Context(),
		domain.TenantID{}, mustBuild(t, domain.NewDeclaredParcelID, "parcel-1")); err == nil {
		t.Fatal("缺租户却放行了")
	}
	if _, _, err := repository.LoadAcceptanceBaselineGeoProjection(t.Context(),
		mustBuild(t, domain.NewTenantID, "tenant-1"), domain.DeclaredParcelID{}); err == nil {
		t.Fatal("缺包裹却放行了")
	}
}
