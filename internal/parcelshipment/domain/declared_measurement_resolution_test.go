package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// measurementScope 是读口要查的那一个范围：按包裹指名的申报测量资料组（测量逐成员申报，ADR-0048 画像按成员）。
func measurementScope(t *testing.T, requestID, parcel string) domain.SourceDataScope {
	t.Helper()
	scope, err := domain.NewParcelScopedSourceData(
		mustValue(t, domain.NewShipmentRequestID, requestID),
		mustValue(t, domain.NewDeclaredParcelID, parcel),
		domain.DeclaredMeasurementDataGroup(),
	)
	if err != nil {
		t.Fatalf("new measurement scope: %v", err)
	}
	return scope
}

// acceptedWithMeasurementVersions 提交一份两成员委托（parcel-1 带重量 2.50 KG、无外廓的画像；parcel-2 无画像），
// 真接受，再按给定链在 **parcel-1 的申报测量范围**上追加版本：每项 [版本, 前版]，前版为空即以接受基线为基准
// 的补充。与 acceptedWithDeliveryPlaceVersions 同一配方，只是范围换成本读口要查的那一个。
func acceptedWithMeasurementVersions(t *testing.T, chain ...[2]string) domain.ShipmentRequest {
	t.Helper()
	spec := submitSpec(t, "parcel-1", "parcel-2")
	spec.Profiles = []domain.DeclaredParcelProfile{profileOf(t, "parcel-1", "2.50")}
	request, err := domain.SubmitShipmentRequest(spec)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if request, err = request.Decide(decisionSpec(t, allGroupsPassing(t))); err != nil {
		t.Fatalf("decide: %v", err)
	}
	for _, link := range chain {
		amendment := amendmentSpec(t)
		amendment.Scope = measurementScope(t, "request-1", "parcel-1")
		amendment.VersionID = mustValue(t, domain.NewSourceDataVersionID, link[0])
		if link[1] == "" {
			amendment.Basis = domain.NewSupplementOnAcceptanceBaseline()
			amendment.Intent = domain.SupplementIntent
		} else {
			if amendment.Basis, err = domain.NewAmendmentOfVersion(
				mustValue(t, domain.NewSourceDataVersionID, link[1]),
			); err != nil {
				t.Fatalf("new amendment basis: %v", err)
			}
			amendment.Intent = domain.CorrectionIntent
		}
		version, err := domain.FormCustomerSourceDataVersion(amendment)
		if err != nil {
			t.Fatalf("form version %q: %v", link[0], err)
		}
		if request, err = request.AmendCustomerSourceData(version); err != nil {
			t.Fatalf("amend with version %q: %v", link[0], err)
		}
	}
	return request
}

func resolveMeasurement(t *testing.T, request domain.ShipmentRequest, parcel string) domain.DeclaredMeasurementResolution {
	t.Helper()
	resolution, err := request.DeclaredMeasurementFor(mustValue(t, domain.NewDeclaredParcelID, parcel))
	if err != nil {
		t.Fatalf("declared measurement for %q: %v", parcel, err)
	}
	return resolution
}

// Covers: pp-seams/02 裁决 3「接受基线（该范围上尚无修订）→ 基线锚」与裁决 5「答 DeclaredMeasurement（毛重必备、
// 尺寸可缺如实）+ 资料版本锚」——值取接受基线所指那一版提交版本上的成员画像，客户的数字原样（"2.50" 不规范化），
// 没报外廓就如实答缺，不填默认尺寸。
func TestAMemberWithABaselineProfileAnswersItsMeasurementAnchoredOnTheBaseline(t *testing.T) {
	request := acceptedWithMeasurementVersions(t)

	resolution := resolveMeasurement(t, request, "parcel-1")
	if resolution.Outcome() != domain.DeclaredMeasurementAnchoredOnBaseline {
		t.Fatalf("outcome = %q, want ANCHORED_ON_BASELINE", resolution.Outcome())
	}
	measurement, declared := resolution.Measurement()
	if !declared {
		t.Fatal("a baseline anchored answer carries no measurement")
	}
	if measurement.Weight().Value().String() != "2.50" || measurement.Weight().Unit().String() != "KG" {
		t.Fatalf("weight = %s %s; 客户的申报被改写了", measurement.Weight().Value(), measurement.Weight().Unit())
	}
	if _, present := measurement.Dimensions(); present {
		t.Fatal("没申报外廓却报告在场——缺席要如实")
	}
	anchor, anchored := resolution.Anchor()
	if !anchored || !anchor.OnAcceptanceBaseline() {
		t.Fatalf("anchor = %#v anchored = %v, want the acceptance baseline anchor", anchor, anchored)
	}
}

// Covers: 裁决 5 与「缺尺寸如实答缺」的另一半——成员在接受基线里，但基线那一版上没有它的画像（测量必填与否由
// 真实产品定，ADR-0048）。答「未申报」，不拿同委托别的成员的测量顶，也不算作「无」：它是委托成员，只是没报。
func TestAMemberWithoutABaselineProfileAnswersNotDeclared(t *testing.T) {
	resolution := resolveMeasurement(t, acceptedWithMeasurementVersions(t), "parcel-2")
	if resolution.Outcome() != domain.DeclaredMeasurementNotDeclared {
		t.Fatalf("outcome = %q, want NOT_DECLARED", resolution.Outcome())
	}
	if _, declared := resolution.Measurement(); declared {
		t.Fatal("没申报的成员凭空长出了测量")
	}
	if _, anchored := resolution.Anchor(); anchored {
		t.Fatal("「未申报」那一格带了锚")
	}
}

// Covers: 裁决 3「当前采用的客户原始资料版本 → 已采用版本锚」。锚是链尾那一版；这两版都不带内容，所以这一格只交锚
// 不交测量——**绝不回退到基线值**：交基线值等于把一份已被客户更正的申报当现行申报送出去。带内容的那一路见
// TestAnAdoptedMeasurementCorrectionAnswersItsOwnMeasurementOnTheAdoptedVersion。
func TestAnAdoptedAmendmentMovesTheMeasurementAnchorAndWithholdsTheBaselineValue(t *testing.T) {
	request := acceptedWithMeasurementVersions(t,
		[2]string{"measure-v1", ""},
		[2]string{"measure-v2", "measure-v1"},
	)

	resolution := resolveMeasurement(t, request, "parcel-1")
	if resolution.Outcome() != domain.DeclaredMeasurementAnchoredOnAdoptedVersion {
		t.Fatalf("outcome = %q, want ANCHORED_ON_ADOPTED_VERSION", resolution.Outcome())
	}
	anchor, anchored := resolution.Anchor()
	if !anchored {
		t.Fatal("an adopted version answer carries no anchor")
	}
	adopted, onVersion := anchor.AdoptedVersion()
	if !onVersion || adopted.String() != "measure-v2" {
		t.Fatalf("anchor = %q on version = %v, want measure-v2（链尾那一版，不是首版也不是基线）", adopted, onVersion)
	}
	if measurement, declared := resolution.Measurement(); declared {
		t.Fatalf("已采用版本锚那一格交出了测量 %#v——那是基线值，客户已经更正过它", measurement)
	}
}

// Covers: 裁决 6「`待复核` 答未定」：两条修订分叉未并，本上下文此刻说不出该按哪一版，不给锚也不给值。
func TestAForkedMeasurementAmendmentChainAnswersUndetermined(t *testing.T) {
	request := acceptedWithMeasurementVersions(t,
		[2]string{"measure-v1", ""},
		[2]string{"measure-v2", "measure-v1"},
		[2]string{"measure-v3", "measure-v1"},
	)

	resolution := resolveMeasurement(t, request, "parcel-1")
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

// Covers: 裁决 4「集运单元 found=false」与裁决 6「非委托对象答无」：不在任何已接受委托成员集合里的对象按统一
// 不可见结果答「无」；仅`已提交`的委托没有基线，其成员同样答无——责任起点在接受之后。
func TestAnObjectOutsideEveryAcceptedBaselineHasNoDeclaredMeasurement(t *testing.T) {
	stranger := resolveMeasurement(t, acceptedWithMeasurementVersions(t), "parcel-9")
	if stranger.Outcome() != domain.NoDeclaredMeasurement {
		t.Fatalf("outcome for a non-member = %q, want NO_DECLARED_MEASUREMENT", stranger.Outcome())
	}
	if _, declared := stranger.Measurement(); declared {
		t.Fatal("a non-member got a measurement")
	}

	spec := submitSpec(t, "parcel-1")
	spec.Profiles = []domain.DeclaredParcelProfile{profileOf(t, "parcel-1", "1.00")}
	pending, err := domain.SubmitShipmentRequest(spec)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	resolution := resolveMeasurement(t, pending, "parcel-1")
	if resolution.Outcome() != domain.NoDeclaredMeasurement {
		t.Fatalf("outcome for a member of a merely submitted request = %q, want NO_DECLARED_MEASUREMENT", resolution.Outcome())
	}
}

// Covers: 读口按本上下文的原词查申报测量范围——收件范围上的修订不动测量的锚（两口各查各的范围，互不知对方）。
func TestAmendmentsOnTheDeliveryPlaceGroupDoNotMoveTheMeasurementAnchor(t *testing.T) {
	spec := submitSpec(t, "parcel-1", "parcel-2")
	spec.Profiles = []domain.DeclaredParcelProfile{profileOf(t, "parcel-1", "2.50")}
	request, err := domain.SubmitShipmentRequest(spec)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if request, err = request.Decide(decisionSpec(t, allGroupsPassing(t))); err != nil {
		t.Fatalf("decide: %v", err)
	}
	amendment := amendmentSpec(t)
	amendment.Scope = deliveryPlaceScope(t, "request-1")
	version, err := domain.FormCustomerSourceDataVersion(amendment)
	if err != nil {
		t.Fatalf("form version: %v", err)
	}
	if request, err = request.AmendCustomerSourceData(version); err != nil {
		t.Fatalf("amend: %v", err)
	}

	resolution := resolveMeasurement(t, request, "parcel-1")
	if resolution.Outcome() != domain.DeclaredMeasurementAnchoredOnBaseline {
		t.Fatalf("outcome = %q, want ANCHORED_ON_BASELINE", resolution.Outcome())
	}
}

// Covers: 封闭答格的名字与零值——零值没有名字，五格各有各的名字，读面与传输层原样透出不另造词。
func TestDeclaredMeasurementOutcomesAreNamed(t *testing.T) {
	names := map[domain.DeclaredMeasurementOutcome]string{
		domain.DeclaredMeasurementAnchoredOnBaseline:       "ANCHORED_ON_BASELINE",
		domain.DeclaredMeasurementAnchoredOnAdoptedVersion: "ANCHORED_ON_ADOPTED_VERSION",
		domain.DeclaredMeasurementUndetermined:             "UNDETERMINED",
		domain.DeclaredMeasurementNotDeclared:              "NOT_DECLARED",
		domain.NoDeclaredMeasurement:                       "NO_DECLARED_MEASUREMENT",
	}
	for outcome, want := range names {
		if outcome.String() != want {
			t.Fatalf("%d.String() = %q, want %q", outcome, outcome.String(), want)
		}
	}
	if (domain.DeclaredMeasurementResolution{}).Outcome().String() != "" {
		t.Fatal("the zero outcome has a name")
	}
}
