package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// addressEntry 造一条按封闭要素名命名的寄 / 收范围条目（`<资料范围原词>.<要素名>`）。
func addressEntry(t *testing.T, group domain.SourceDataGroupReference, element domain.AddressElementName, value string) domain.CanonicalContentEntry {
	t.Helper()
	return contentEntry(t, domain.AddressElementEntryName(group, element), value)
}

// Covers: pp-seams/05 裁决 3「内容与摘要由同一次规范化产出」——成对结果的摘要与既有 CanonicalizeSubmissionPayload 对同一份
// 输入算出的一字不差（摘要算法零改），要素按寄 / 收两范围从同一份 Scope 条目里挑出，只认封闭名字，其余条目只进摘要。
func TestCanonicalizingASubmissionYieldsTheDigestAndTheClosedElementsTogether(t *testing.T) {
	spec := payloadSpec(t)
	spec.Scope = append(spec.Scope,
		addressEntry(t, domain.DeliveryPlaceDataGroup(), domain.PostalCodeElement, "10115"),
		addressEntry(t, domain.DeliveryPlaceDataGroup(), domain.CountryCodeElement, "DE"),
		addressEntry(t, domain.SenderPlaceDataGroup(), domain.PostalCodeElement, "SYN-200000"),
	)

	canonical, err := domain.CanonicalizeSubmission(spec)
	if err != nil {
		t.Fatalf("canonicalize submission: %v", err)
	}
	if want := canonicalize(t, spec); canonical.Digest() != want {
		t.Fatalf("digest = %q, want %q（成对产出的摘要必须与只算摘要那条路一字不差）", canonical.Digest(), want)
	}

	elements := canonical.DeclaredElements()
	destination := elements.InGroup(domain.DeliveryPlaceDataGroup())
	if postal, declared := destination.PostalCode(); !declared || postal != "10115" {
		t.Fatalf("destination postal code = %q declared = %v, want 10115", postal, declared)
	}
	if country, declared := destination.CountryCode(); !declared || country != "DE" {
		t.Fatalf("destination country code = %q declared = %v, want DE", country, declared)
	}
	origin := elements.InGroup(domain.SenderPlaceDataGroup())
	if postal, declared := origin.PostalCode(); !declared || postal != "SYN-200000" {
		t.Fatalf("origin postal code = %q declared = %v", postal, declared)
	}
	if _, declared := origin.CountryCode(); declared {
		t.Fatal("寄件范围没报国家 / 地区码却在场")
	}
	if elements.Empty() {
		t.Fatal("两段都有要素却报告为空")
	}

	// 不带任何封闭要素的输入照样成对交回：内容那一半为空，摘要与既有路一致。
	plain := payloadSpec(t)
	bare, err := domain.CanonicalizeSubmission(plain)
	if err != nil {
		t.Fatalf("canonicalize submission without elements: %v", err)
	}
	if bare.Digest() != canonicalize(t, plain) || !bare.DeclaredElements().Empty() {
		t.Fatalf("bare = %#v", bare)
	}

	// 摘要算不出来的输入，成对结果同样拒收——两样出自同一道门。
	if _, err := domain.CanonicalizeSubmission(domain.SubmissionPayloadSpec{}); !errors.Is(err, domain.ErrInvalidSubmissionPayload) {
		t.Fatalf("err = %v, want ErrInvalidSubmissionPayload", err)
	}
}

// acceptedWithElements 以带地址要素子段的首个提交版本建单并接受：收件范围报邮编与国家 / 地区码，寄件范围只报邮编。
func acceptedWithElements(t *testing.T) domain.ShipmentRequest {
	t.Helper()
	spec := submitSpec(t, "parcel-1", "parcel-2")
	spec.Elements = domain.NewDeclaredAddressElements(
		domain.AddressElementsOf(domain.SenderPlaceDataGroup(), []domain.CanonicalContentEntry{
			addressEntry(t, domain.SenderPlaceDataGroup(), domain.PostalCodeElement, "SYN-200000"),
		}),
		domain.AddressElementsOf(domain.DeliveryPlaceDataGroup(), []domain.CanonicalContentEntry{
			addressEntry(t, domain.DeliveryPlaceDataGroup(), domain.PostalCodeElement, "10115"),
			addressEntry(t, domain.DeliveryPlaceDataGroup(), domain.CountryCodeElement, "DE"),
		}),
	)
	request, err := domain.SubmitShipmentRequest(spec)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if request, err = request.Decide(decisionSpec(t, allGroupsPassing(t))); err != nil {
		t.Fatalf("decide: %v", err)
	}
	return request
}

// Covers: 完成判据 (1)「接单带 DELIVERY_PLACE.POSTAL_CODE 一类条目 → 目的段 ANCHORED_ON_BASELINE 带值」——提交版本携带
// 要素子段后，锚在基线上的那一段从版本自己的子段读值；同一委托的两个成员答一样（寄收件资料按委托级范围形成）。
func TestAMemberOfARequestAcceptedWithElementsAnswersThemAnchoredOnTheBaseline(t *testing.T) {
	request := acceptedWithElements(t)

	for _, parcel := range []string{"parcel-1", "parcel-2"} {
		answer := resolveAddressElements(t, request, parcel)
		destination := answer.Destination()
		if destination.Outcome() != domain.AddressElementsAnchoredOnBaseline {
			t.Fatalf("%s destination outcome = %q, want ANCHORED_ON_BASELINE", parcel, destination.Outcome())
		}
		elements, present := destination.Elements()
		if postal, declared := elements.PostalCode(); !present || !declared || postal != "10115" {
			t.Fatalf("%s destination postal = %q present = %v declared = %v", parcel, postal, present, declared)
		}
		if country, declared := elements.CountryCode(); !declared || country != "DE" {
			t.Fatalf("%s destination country = %q declared = %v", parcel, country, declared)
		}
		if anchor, anchored := destination.Anchor(); !anchored || !anchor.OnAcceptanceBaseline() {
			t.Fatalf("%s destination anchor = %#v anchored = %v, want the acceptance baseline anchor", parcel, anchor, anchored)
		}

		origin := answer.Origin()
		if origin.Outcome() != domain.AddressElementsAnchoredOnBaseline {
			t.Fatalf("%s origin outcome = %q, want ANCHORED_ON_BASELINE", parcel, origin.Outcome())
		}
		originElements, _ := origin.Elements()
		if postal, declared := originElements.PostalCode(); !declared || postal != "SYN-200000" {
			t.Fatalf("%s origin postal = %q declared = %v", parcel, postal, declared)
		}
		if _, declared := originElements.CountryCode(); declared {
			t.Fatalf("%s 寄件范围没报国家 / 地区码却在场", parcel)
		}
	}
}

// deliveryElementsContent 造一份收件范围上的地址要素内容（只报邮编）。
func deliveryElementsContent(t *testing.T, postal string) domain.SourceDataVersionContent {
	t.Helper()
	return domain.NewAddressElementsContent(domain.AddressElementsOf(domain.DeliveryPlaceDataGroup(),
		[]domain.CanonicalContentEntry{addressEntry(t, domain.DeliveryPlaceDataGroup(), domain.PostalCodeElement, postal)}))
}

// measurementContent 造一份申报测量范围上的内容（只报重量，KG）。
func measurementContent(t *testing.T, weight string) domain.SourceDataVersionContent {
	t.Helper()
	content, err := domain.NewMeasurementContent(measurementOf(t, weight, "KG"))
	if err != nil {
		t.Fatalf("new measurement content: %v", err)
	}
	return content
}

// amendedWithContent 在已接受的夹具委托上按给定范围追加一条版本链，每项 [版本, 前版, 意图, 内容]：前版为空即以接受基线为
// 基准；意图取 "supplement" / "correction" / "clear"；内容按范围造，"" 即不带内容。
type contentLink struct {
	version, prior, intent, value string
}

func amendedWithContent(t *testing.T, request domain.ShipmentRequest, scope domain.SourceDataScope, links ...contentLink) domain.ShipmentRequest {
	t.Helper()
	for _, link := range links {
		spec := amendmentSpec(t)
		spec.Scope = scope
		spec.VersionID = mustValue(t, domain.NewSourceDataVersionID, link.version)
		spec.Basis = domain.NewSupplementOnAcceptanceBaseline()
		if link.prior != "" {
			var err error
			if spec.Basis, err = domain.NewAmendmentOfVersion(mustValue(t, domain.NewSourceDataVersionID, link.prior)); err != nil {
				t.Fatalf("new amendment basis: %v", err)
			}
		}
		switch link.intent {
		case "supplement":
			spec.Intent = domain.SupplementIntent
		case "correction":
			spec.Intent = domain.CorrectionIntent
		case "clear":
			spec.Intent = domain.ExplicitClearIntent
		default:
			t.Fatalf("unknown intent %q", link.intent)
		}
		if link.value != "" {
			if scope.DataGroup() == domain.DeclaredMeasurementDataGroup() {
				spec.Content = measurementContent(t, link.value)
			} else {
				spec.Content = deliveryElementsContent(t, link.value)
			}
		}
		version, err := domain.FormCustomerSourceDataVersion(spec)
		if err != nil {
			t.Fatalf("form version %q: %v", link.version, err)
		}
		if request, err = request.AmendCustomerSourceData(version); err != nil {
			t.Fatalf("amend with version %q: %v", link.version, err)
		}
	}
	return request
}

// Covers: 完成判据 (2)「收件范围上的修订被采用后，已采用格第二返回值为真且值是那一版的」——值取链尾那一版自己的内容，
// 不是基线值也不是首版；锚仍是已采用版本锚，格名不变。
func TestAnAdoptedDeliveryPlaceCorrectionAnswersItsOwnElementsOnTheAdoptedVersion(t *testing.T) {
	request := amendedWithContent(t, acceptedWithElements(t), deliveryPlaceScope(t, "request-1"),
		contentLink{"dp-v1", "", "supplement", "20095"},
		contentLink{"dp-v2", "dp-v1", "correction", "20097"},
	)

	destination := resolveAddressElements(t, request, "parcel-1").Destination()
	if destination.Outcome() != domain.AddressElementsAnchoredOnAdoptedVersion {
		t.Fatalf("destination outcome = %q, want ANCHORED_ON_ADOPTED_VERSION", destination.Outcome())
	}
	elements, present := destination.Elements()
	if postal, declared := elements.PostalCode(); !present || !declared || postal != "20097" {
		t.Fatalf("postal = %q present = %v declared = %v, want 20097（链尾那一版的值，不是基线的 10115 也不是首版的 20095）", postal, present, declared)
	}
	if _, declared := elements.CountryCode(); declared {
		t.Fatal("修订版本没报国家 / 地区码，基线的 DE 却被顶了上来")
	}
	if anchor, anchored := destination.Anchor(); !anchored {
		t.Fatal("已采用版本格没带锚")
	} else if adopted, onVersion := anchor.AdoptedVersion(); !onVersion || adopted.String() != "dp-v2" {
		t.Fatalf("anchor = %q, want dp-v2", adopted)
	}
}

// Covers: 完成判据 (2)「测量范围的修订被采用后 Measurement() 第二返回值为真且值是那一版的」。
func TestAnAdoptedMeasurementCorrectionAnswersItsOwnMeasurementOnTheAdoptedVersion(t *testing.T) {
	request := amendedWithContent(t, acceptedWithMeasurementVersions(t), measurementScope(t, "request-1", "parcel-1"),
		contentLink{"measure-v1", "", "supplement", "3.10"},
		contentLink{"measure-v2", "measure-v1", "correction", "3.25"},
	)

	resolution := resolveMeasurement(t, request, "parcel-1")
	if resolution.Outcome() != domain.DeclaredMeasurementAnchoredOnAdoptedVersion {
		t.Fatalf("outcome = %q, want ANCHORED_ON_ADOPTED_VERSION", resolution.Outcome())
	}
	measurement, declared := resolution.Measurement()
	if !declared || measurement.Weight().Value().String() != "3.25" {
		t.Fatalf("measurement = %#v declared = %v, want 3.25 KG（链尾那一版）", measurement, declared)
	}
	if _, present := measurement.Dimensions(); present {
		t.Fatal("修订版本没报外廓却在场")
	}
}

// Covers: 裁决 3「清空就是那一版上要素缺席，两口的已采用格随之如实答缺」——显式清空的版本被采用后，格仍是已采用版本格
// （带锚），值缺席；不回退到基线值。
func TestAnAdoptedExplicitClearLeavesTheAdoptedCellWithoutAValue(t *testing.T) {
	request := amendedWithContent(t, acceptedWithElements(t), deliveryPlaceScope(t, "request-1"),
		contentLink{"dp-v1", "", "supplement", "20095"},
		contentLink{"dp-v2", "dp-v1", "clear", ""},
	)

	destination := resolveAddressElements(t, request, "parcel-2").Destination()
	if destination.Outcome() != domain.AddressElementsAnchoredOnAdoptedVersion {
		t.Fatalf("destination outcome = %q, want ANCHORED_ON_ADOPTED_VERSION", destination.Outcome())
	}
	if elements, present := destination.Elements(); present {
		t.Fatalf("清空版本被采用后交出了要素 %#v", elements)
	}
	if anchor, anchored := destination.Anchor(); !anchored {
		t.Fatal("清空版本被采用后没带锚")
	} else if adopted, _ := anchor.AdoptedVersion(); adopted.String() != "dp-v2" {
		t.Fatalf("anchor = %q, want dp-v2", adopted)
	}

	measured := amendedWithContent(t, acceptedWithMeasurementVersions(t), measurementScope(t, "request-1", "parcel-1"),
		contentLink{"measure-v1", "", "clear", ""},
	)
	resolution := resolveMeasurement(t, measured, "parcel-1")
	if resolution.Outcome() != domain.DeclaredMeasurementAnchoredOnAdoptedVersion {
		t.Fatalf("outcome = %q, want ANCHORED_ON_ADOPTED_VERSION", resolution.Outcome())
	}
	if measurement, declared := resolution.Measurement(); declared {
		t.Fatalf("清空版本被采用后交出了测量 %#v", measurement)
	}
}

// Covers: 判据 (2)「`待复核`仍不给值」——带内容的两条修订分叉，读口既不给锚也不给值，不替客户挑一版。
func TestAForkedChainWithContentStillWithholdsBothAnchorAndValue(t *testing.T) {
	request := amendedWithContent(t, acceptedWithElements(t), deliveryPlaceScope(t, "request-1"),
		contentLink{"dp-v1", "", "supplement", "20095"},
		contentLink{"dp-v2", "dp-v1", "correction", "20097"},
		contentLink{"dp-v3", "dp-v1", "correction", "20099"},
	)

	destination := resolveAddressElements(t, request, "parcel-1").Destination()
	if destination.Outcome() != domain.AddressElementsUndetermined {
		t.Fatalf("destination outcome = %q, want UNDETERMINED", destination.Outcome())
	}
	if _, present := destination.Elements(); present {
		t.Fatal("`待复核`交出了要素")
	}
}

// Covers: 裁决 3 的构造门「形与范围不配 → 拒」「显式清空带内容 → 拒」：测量内容不能落在收件范围、要素内容不能落在测量
// 范围、也不能落在两个地址范围之外的开放范围；清空版本带任何内容都拒；不带内容的补充 / 更正照旧合法（该范围上的其余
// 条目只进摘要，本上下文对它们没有词条）。
func TestASourceDataVersionOnlyCarriesTheContentShapeOfItsOwnGroup(t *testing.T) {
	cases := map[string]struct {
		scope   domain.SourceDataScope
		intent  domain.AmendmentIntent
		content domain.SourceDataVersionContent
		want    error
	}{
		"收件范围带测量内容": {deliveryPlaceScope(t, "request-1"), domain.SupplementIntent, measurementContent(t, "1.00"), domain.ErrInvalidCustomerSourceDataVersion},
		"测量范围带要素内容": {measurementScope(t, "request-1", "parcel-1"), domain.SupplementIntent, deliveryElementsContent(t, "10115"), domain.ErrInvalidCustomerSourceDataVersion},
		"开放范围带要素内容": {consigneeScope(t), domain.SupplementIntent, deliveryElementsContent(t, "10115"), domain.ErrInvalidCustomerSourceDataVersion},
		"显式清空带要素内容": {deliveryPlaceScope(t, "request-1"), domain.ExplicitClearIntent, deliveryElementsContent(t, "10115"), domain.ErrInvalidCustomerSourceDataVersion},
		"显式清空带测量内容": {measurementScope(t, "request-1", "parcel-1"), domain.ExplicitClearIntent, measurementContent(t, "1.00"), domain.ErrInvalidCustomerSourceDataVersion},
		"收件范围带要素内容": {deliveryPlaceScope(t, "request-1"), domain.SupplementIntent, deliveryElementsContent(t, "10115"), nil},
		"测量范围带测量内容": {measurementScope(t, "request-1", "parcel-1"), domain.SupplementIntent, measurementContent(t, "1.00"), nil},
		"收件范围不带内容":  {deliveryPlaceScope(t, "request-1"), domain.SupplementIntent, domain.SourceDataVersionContent{}, nil},
		"开放范围不带内容":  {consigneeScope(t), domain.SupplementIntent, domain.SourceDataVersionContent{}, nil},
		"显式清空不带内容":  {deliveryPlaceScope(t, "request-1"), domain.ExplicitClearIntent, domain.SourceDataVersionContent{}, nil},
	}
	for name, item := range cases {
		t.Run(name, func(t *testing.T) {
			spec := amendmentSpec(t)
			spec.Scope = item.scope
			spec.Intent = item.intent
			spec.Content = item.content
			version, err := domain.FormCustomerSourceDataVersion(spec)
			if !errors.Is(err, item.want) {
				t.Fatalf("err = %v, want %v", err, item.want)
			}
			if err == nil && version.Content() != item.content {
				t.Fatalf("content = %#v, want %#v（版本要把内容原样带走）", version.Content(), item.content)
			}
			if fits := item.content.FitsAmendment(item.scope, item.intent); fits != (item.want == nil) {
				t.Fatalf("FitsAmendment = %v, want %v（同一道门要能在形成版本之前先问）", fits, item.want == nil)
			}
		})
	}
}

// Covers: 内容的形——零值即「缺席」，测量内容必须带一份有重量的测量，要素内容两格都缺也算缺席。
func TestSourceDataVersionContentIsOneShapeOrAbsent(t *testing.T) {
	if !(domain.SourceDataVersionContent{}).Empty() {
		t.Fatal("零值内容不算缺席")
	}
	if _, err := domain.NewMeasurementContent(domain.DeclaredMeasurement{}); !errors.Is(err, domain.ErrInvalidDeclaredMeasurement) {
		t.Fatalf("err = %v, want ErrInvalidDeclaredMeasurement（没有重量的测量不是内容）", err)
	}
	measured := measurementContent(t, "2.00")
	if measurement, present := measured.Measurement(); !present || measurement.Weight().Value().String() != "2.00" {
		t.Fatalf("measurement = %#v present = %v", measurement, present)
	}
	if _, present := measured.AddressElements(); present || measured.Empty() {
		t.Fatal("测量内容长出了要素，或被判成缺席")
	}
	if domain.NewAddressElementsContent(domain.AddressElements{}) != (domain.SourceDataVersionContent{}) {
		t.Fatal("两格都缺的要素内容不是零值")
	}
	elements := deliveryElementsContent(t, "10115")
	if got, present := elements.AddressElements(); !present {
		t.Fatal("要素内容没报告在场")
	} else if postal, declared := got.PostalCode(); !declared || postal != "10115" {
		t.Fatalf("postal = %q declared = %v", postal, declared)
	}
	if _, present := elements.Measurement(); present {
		t.Fatal("要素内容长出了测量")
	}
}

// Covers: 完成判据 (1)「接单不带 → NOT_PROVIDED」的另一半与「一段有一段无是常态」——只报了收件邮编的提交版本，起点段仍答
// 「要素缺席」，不拿收件的值顶寄件。
func TestARequestDeclaringOnlyTheDestinationLeavesTheOriginNotProvided(t *testing.T) {
	spec := submitSpec(t, "parcel-1")
	spec.Elements = domain.NewDeclaredAddressElements(domain.AddressElements{},
		domain.AddressElementsOf(domain.DeliveryPlaceDataGroup(), []domain.CanonicalContentEntry{
			addressEntry(t, domain.DeliveryPlaceDataGroup(), domain.PostalCodeElement, "10115"),
		}))
	request, err := domain.SubmitShipmentRequest(spec)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if request, err = request.Decide(decisionSpec(t, allGroupsPassing(t))); err != nil {
		t.Fatalf("decide: %v", err)
	}

	answer := resolveAddressElements(t, request, "parcel-1")
	if answer.Destination().Outcome() != domain.AddressElementsAnchoredOnBaseline {
		t.Fatalf("destination outcome = %q, want ANCHORED_ON_BASELINE", answer.Destination().Outcome())
	}
	if answer.Origin().Outcome() != domain.AddressElementsNotProvided {
		t.Fatalf("origin outcome = %q, want NOT_PROVIDED", answer.Origin().Outcome())
	}
}
