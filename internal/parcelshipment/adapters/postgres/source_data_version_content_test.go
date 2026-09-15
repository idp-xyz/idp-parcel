package postgres_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件对真实 PostgreSQL 16 证 pp-seams/05 完成判据 (1) / (2)：提交版本与客户原始资料版本携带封闭要素内容并经快照往返——
// 基线格从提交版本的 `elements` 子段读值、已采用格从资料版本的 `content` 子段读值、清空版本被采用后已采用格如实答缺、
// 早于本票的旧形快照（夹具直写抹掉子段）如实答「要素缺席」。零迁移：两段都落在既有 jsonb 快照里。

func deliveryPostalEntry(t *testing.T, postal string) domain.CanonicalContentEntry {
	t.Helper()
	entry, err := domain.NewCanonicalContentEntry(
		domain.AddressElementEntryName(domain.DeliveryPlaceDataGroup(), domain.PostalCodeElement), postal)
	if err != nil {
		t.Fatalf("收件邮编条目：%v", err)
	}
	return entry
}

// submittedRequestWithElements 在 submittedShipmentRequest 的配方上给首个提交版本带上地址要素子段：收件范围报邮编 SYN-100115 与
// 国家 / 地区码 SYN-CC，寄件范围只报邮编。
func submittedRequestWithElements(t *testing.T, key, requestID string) domain.ShipmentRequest {
	t.Helper()
	country, err := domain.NewCanonicalContentEntry(
		domain.AddressElementEntryName(domain.DeliveryPlaceDataGroup(), domain.CountryCodeElement), "SYN-CC")
	if err != nil {
		t.Fatalf("收件国家条目：%v", err)
	}
	senderPostal, err := domain.NewCanonicalContentEntry(
		domain.AddressElementEntryName(domain.SenderPlaceDataGroup(), domain.PostalCodeElement), "SYN-200000")
	if err != nil {
		t.Fatalf("寄件邮编条目：%v", err)
	}
	entries := []domain.CanonicalContentEntry{deliveryPostalEntry(t, "SYN-100115"), country, senderPostal}
	// 要素经 CanonicalizeSubmission 从条目挑出——与接单入口同一条路，而不是在夹具里手拼 AddressElements。
	canonical, err := domain.CanonicalizeSubmission(domain.SubmissionPayloadSpec{
		RequestReference:  mustBuild(t, domain.NewShipmentRequestID, requestID),
		DeclaredParcelIDs: []domain.DeclaredParcelID{mustBuild(t, domain.NewDeclaredParcelID, "parcel-1")},
		Scope:             entries,
	})
	if err != nil {
		t.Fatalf("规范化：%v", err)
	}
	spec := submittedShipmentRequestSpec(t, key, requestID)
	spec.Elements = canonical.DeclaredElements()
	request, err := domain.SubmitShipmentRequest(spec)
	if err != nil {
		t.Fatalf("带要素建单：%v", err)
	}
	return request
}

// acceptedWithElementsChain 插入并真接受带要素子段的夹具委托，再在收件范围上按给定链追加版本：每项 {版本, 前版, 意图, 邮编}，
// 邮编为空即不带内容。
type storedContentLink struct {
	version, prior string
	intent         domain.AmendmentIntent
	postal         string
}

func acceptedWithElementsChain(
	t *testing.T,
	repository *adapter.ShipmentRequests,
	transactor bentoapp.Transactor,
	key, requestID string,
	chain ...storedContentLink,
) {
	t.Helper()
	ctx := t.Context()
	mustInsert(t, transactor, ctx, repository, submittedRequestWithElements(t, key, requestID))
	accepted, err := mustFind(t, repository, ctx, key).Decide(acceptanceDecisionSpec(t))
	if err != nil {
		t.Fatalf("形成接受：%v", err)
	}
	mustSave(t, transactor, ctx, repository, requestIdentity(t, key), accepted)

	scope, err := domain.NewShipmentScopedSourceData(mustBuild(t, domain.NewShipmentRequestID, requestID), domain.DeliveryPlaceDataGroup())
	if err != nil {
		t.Fatalf("资料范围：%v", err)
	}
	for index, link := range chain {
		current := mustFind(t, repository, ctx, key)
		basis := domain.NewSupplementOnAcceptanceBaseline()
		if link.prior != "" {
			if basis, err = domain.NewAmendmentOfVersion(mustBuild(t, domain.NewSourceDataVersionID, link.prior)); err != nil {
				t.Fatalf("前版基准：%v", err)
			}
		}
		spec := domain.CustomerSourceDataVersionSpec{
			VersionID: mustBuild(t, domain.NewSourceDataVersionID, link.version),
			Scope:     scope,
			Basis:     basis,
			Intent:    link.intent,
			Request:   requestFingerprint(t, key+"-amend-"+link.version, "digest-"+link.version),
			Reason:    mustBuild(t, domain.NewAmendmentReasonReference, "ADDRESS_RESTATED"),
			Requester: mustBuild(t, domain.NewRequesterReference, "CUSTOMER-1"),
			Decider:   mustBuild(t, domain.NewDeciderReference, "OPERATOR-1"),
			Authority: mustBuild(t, domain.NewAmendmentAuthoritySnapshot, "AUTH-SNAP-1"),
			FormedAt:  submittedAtFixture.Add(time.Duration(index+2) * time.Hour),
		}
		if link.postal != "" {
			spec.Content = domain.NewAddressElementsContent(domain.AddressElementsOf(domain.DeliveryPlaceDataGroup(),
				[]domain.CanonicalContentEntry{deliveryPostalEntry(t, link.postal)}))
		}
		version, err := domain.FormCustomerSourceDataVersion(spec)
		if err != nil {
			t.Fatalf("形成资料版本 %q：%v", link.version, err)
		}
		amended, err := current.AmendCustomerSourceData(version)
		if err != nil {
			t.Fatalf("追加资料版本 %q：%v", link.version, err)
		}
		mustSave(t, transactor, ctx, repository, requestIdentity(t, key), amended)
	}
}

// 判据 (1)：接单带 DELIVERY_PLACE.POSTAL_CODE 一类条目 → 目的段 ANCHORED_ON_BASELINE 带值；起点段带寄件邮编；值经快照往返原样。
func TestAMemberOfARequestAcceptedWithElementsAnswersThemFromTheSnapshot(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedWithElementsChain(t, repository, transactor, "req-key-1", "request-1")

	answer := loadAddressElements(t, repository, "tenant-1", "parcel-2")
	destination := answer.Destination()
	if destination.Outcome() != domain.AddressElementsAnchoredOnBaseline {
		t.Fatalf("destination outcome = %q, want ANCHORED_ON_BASELINE", destination.Outcome())
	}
	elements, present := destination.Elements()
	if postal, declared := elements.PostalCode(); !present || !declared || postal != "SYN-100115" {
		t.Fatalf("destination postal = %q present = %v declared = %v", postal, present, declared)
	}
	if country, declared := elements.CountryCode(); !declared || country != "SYN-CC" {
		t.Fatalf("destination country = %q declared = %v", country, declared)
	}
	origin := answer.Origin()
	if origin.Outcome() != domain.AddressElementsAnchoredOnBaseline {
		t.Fatalf("origin outcome = %q, want ANCHORED_ON_BASELINE", origin.Outcome())
	}
	originElements, _ := origin.Elements()
	if postal, declared := originElements.PostalCode(); !declared || postal != "SYN-200000" {
		t.Fatalf("origin postal = %q declared = %v", postal, declared)
	}
	if _, declared := originElements.CountryCode(); declared {
		t.Fatal("寄件范围没报国家 / 地区码却在场")
	}
}

// 判据 (1) 第三句：夹具直写旧形快照（抹掉提交版本的 `elements` 子段）→ 读回零值，两段如实答「要素缺席」，不回填、不报错。
func TestAnOldShapeSnapshotWithoutTheElementsSegmentAnswersNotProvided(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	acceptedWithElementsChain(t, repository, transactor, "req-key-1", "request-1")

	if _, err := pool.Exec(t.Context(),
		`UPDATE parcel_shipment.shipment_request
		    SET snapshot = snapshot #- '{currentVersion,elements}'
		  WHERE source_request_key = $1`, "req-key-1"); err != nil {
		t.Fatalf("抹掉 elements 子段：%v", err)
	}

	answer := loadAddressElements(t, repository, "tenant-1", "parcel-1")
	for name, segment := range map[string]domain.AddressElementsResolution{
		"origin": answer.Origin(), "destination": answer.Destination(),
	} {
		if segment.Outcome() != domain.AddressElementsNotProvided {
			t.Fatalf("%s outcome = %q, want NOT_PROVIDED（旧形快照没有子段，如实答缺）", name, segment.Outcome())
		}
	}
}

// 判据 (2)：收件范围上带内容的修订被采用后，已采用格第二返回值为真且值是那一版的（不是基线的 SYN-100115，也不是首版的）。
func TestAnAdoptedDeliveryPlaceCorrectionAnswersItsOwnElementsFromTheSnapshot(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedWithElementsChain(t, repository, transactor, "req-key-1", "request-1",
		storedContentLink{"dp-v1", "", domain.SupplementIntent, "SYN-200095"},
		storedContentLink{"dp-v2", "dp-v1", domain.CorrectionIntent, "SYN-200097"},
	)

	destination := loadAddressElements(t, repository, "tenant-1", "parcel-1").Destination()
	if destination.Outcome() != domain.AddressElementsAnchoredOnAdoptedVersion {
		t.Fatalf("destination outcome = %q, want ANCHORED_ON_ADOPTED_VERSION", destination.Outcome())
	}
	elements, present := destination.Elements()
	if postal, declared := elements.PostalCode(); !present || !declared || postal != "SYN-200097" {
		t.Fatalf("postal = %q present = %v declared = %v, want SYN-200097", postal, present, declared)
	}
	if _, declared := elements.CountryCode(); declared {
		t.Fatal("修订版本没报国家 / 地区码，基线的 SYN-CC 却被顶了上来")
	}
	anchor, anchored := destination.Anchor()
	if adopted, onVersion := anchor.AdoptedVersion(); !anchored || !onVersion || adopted.String() != "dp-v2" {
		t.Fatalf("anchor = %q, want dp-v2", adopted)
	}
}

// 判据 (2)：显式清空的版本被采用后，已采用格带锚不带值，不回退到基线值。
func TestAnAdoptedExplicitClearAnswersTheAdoptedAnchorWithoutAValueFromTheSnapshot(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedWithElementsChain(t, repository, transactor, "req-key-1", "request-1",
		storedContentLink{"dp-v1", "", domain.SupplementIntent, "SYN-200095"},
		storedContentLink{"dp-v2", "dp-v1", domain.ExplicitClearIntent, ""},
	)

	destination := loadAddressElements(t, repository, "tenant-1", "parcel-2").Destination()
	if destination.Outcome() != domain.AddressElementsAnchoredOnAdoptedVersion {
		t.Fatalf("destination outcome = %q, want ANCHORED_ON_ADOPTED_VERSION", destination.Outcome())
	}
	if elements, present := destination.Elements(); present {
		t.Fatalf("清空版本被采用后交出了要素 %#v", elements)
	}
	if anchor, anchored := destination.Anchor(); !anchored {
		t.Fatal("已采用版本格没带锚")
	} else if adopted, _ := anchor.AdoptedVersion(); adopted.String() != "dp-v2" {
		t.Fatalf("anchor = %q, want dp-v2", adopted)
	}
}

// 判据 (2)：申报测量范围上带内容的修订被采用后，DeclaredMeasurementView 已采用格 Measurement() 第二返回值为真且值是那一版的。
func TestAnAdoptedMeasurementCorrectionAnswersItsOwnMeasurementFromTheSnapshot(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()
	mustInsert(t, transactor, ctx, repository, submittedShipmentRequest(t, "req-key-1", "request-1"))
	accepted, err := mustFind(t, repository, ctx, "req-key-1").Decide(acceptanceDecisionSpec(t))
	if err != nil {
		t.Fatalf("形成接受：%v", err)
	}
	mustSave(t, transactor, ctx, repository, requestIdentity(t, "req-key-1"), accepted)

	scope, err := domain.NewParcelScopedSourceData(
		mustBuild(t, domain.NewShipmentRequestID, "request-1"),
		mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"),
		domain.DeclaredMeasurementDataGroup(),
	)
	if err != nil {
		t.Fatalf("申报测量资料范围：%v", err)
	}
	weight, err := domain.NewDeclaredWeight(
		mustBuild(t, domain.NewMeasurementValue, "3.25"), mustBuild(t, domain.NewMeasurementUnitReference, "kg"))
	if err != nil {
		t.Fatalf("申报重量：%v", err)
	}
	measurement, err := domain.NewDeclaredMeasurement(weight, domain.DeclaredDimensions{})
	if err != nil {
		t.Fatalf("申报测量：%v", err)
	}
	content, err := domain.NewMeasurementContent(measurement)
	if err != nil {
		t.Fatalf("测量内容：%v", err)
	}
	version, err := domain.FormCustomerSourceDataVersion(domain.CustomerSourceDataVersionSpec{
		VersionID: mustBuild(t, domain.NewSourceDataVersionID, "measure-v1"),
		Scope:     scope,
		Basis:     domain.NewSupplementOnAcceptanceBaseline(),
		Intent:    domain.SupplementIntent,
		Request:   requestFingerprint(t, "req-key-1-amend-measure-v1", "digest-measure-v1"),
		Reason:    mustBuild(t, domain.NewAmendmentReasonReference, "WEIGHT_RESTATED"),
		Requester: mustBuild(t, domain.NewRequesterReference, "CUSTOMER-1"),
		Decider:   mustBuild(t, domain.NewDeciderReference, "OPERATOR-1"),
		Authority: mustBuild(t, domain.NewAmendmentAuthoritySnapshot, "AUTH-SNAP-1"),
		FormedAt:  submittedAtFixture.Add(2 * time.Hour),
		Content:   content,
	})
	if err != nil {
		t.Fatalf("形成资料版本：%v", err)
	}
	amended, err := mustFind(t, repository, ctx, "req-key-1").AmendCustomerSourceData(version)
	if err != nil {
		t.Fatalf("追加资料版本：%v", err)
	}
	mustSave(t, transactor, ctx, repository, requestIdentity(t, "req-key-1"), amended)

	resolution := loadDeclaredMeasurement(t, repository, "tenant-1", "parcel-1")
	if resolution.Outcome() != domain.DeclaredMeasurementAnchoredOnAdoptedVersion {
		t.Fatalf("outcome = %q, want ANCHORED_ON_ADOPTED_VERSION", resolution.Outcome())
	}
	got, declared := resolution.Measurement()
	if !declared || got.Weight().Value().String() != "3.25" || got.Weight().Unit().String() != "kg" {
		t.Fatalf("measurement = %#v declared = %v, want 3.25 kg（那一版自己的值，不是基线的 2.5）", got, declared)
	}
	if _, present := got.Dimensions(); present {
		t.Fatal("修订版本没报外廓，基线的 30×20×10 却被顶了上来")
	}
}

// 资料版本登记册（SourceDataVersions）自己的往返：Append 落下的内容 FindVersion 原样读回——委托快照与登记册两处存的是同一份
// 文档形状，任一处丢内容都会让两口在一侧有值、一侧没值。
func TestASourceDataVersionRegistryRoundTripsTheContent(t *testing.T) {
	registry, transactor := newSourceDataVersions(t)
	ctx := t.Context()
	scope, err := domain.NewShipmentScopedSourceData(mustBuild(t, domain.NewShipmentRequestID, "request-1"), domain.DeliveryPlaceDataGroup())
	if err != nil {
		t.Fatalf("资料范围：%v", err)
	}
	version, err := domain.FormCustomerSourceDataVersion(domain.CustomerSourceDataVersionSpec{
		VersionID: mustBuild(t, domain.NewSourceDataVersionID, "dp-v1"),
		Scope:     scope,
		Basis:     domain.NewSupplementOnAcceptanceBaseline(),
		Intent:    domain.SupplementIntent,
		Request:   requestFingerprint(t, "req-key-1-amend-dp-v1", "digest-dp-v1"),
		Reason:    mustBuild(t, domain.NewAmendmentReasonReference, "ADDRESS_RESTATED"),
		Requester: mustBuild(t, domain.NewRequesterReference, "CUSTOMER-1"),
		Decider:   mustBuild(t, domain.NewDeciderReference, "OPERATOR-1"),
		Authority: mustBuild(t, domain.NewAmendmentAuthoritySnapshot, "AUTH-SNAP-1"),
		FormedAt:  submittedAtFixture.Add(2 * time.Hour),
		Content: domain.NewAddressElementsContent(domain.AddressElementsOf(domain.DeliveryPlaceDataGroup(),
			[]domain.CanonicalContentEntry{deliveryPostalEntry(t, "SYN-200095")})),
	})
	if err != nil {
		t.Fatalf("形成资料版本：%v", err)
	}
	identity := requestIdentity(t, "req-key-1")
	var outcome ports.SourceDataVersionAppendOutcome
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = registry.Append(txCtx, identity, version)
		return err
	}); err != nil {
		t.Fatalf("追加：%v", err)
	}
	if outcome != ports.SourceDataVersionAppended {
		t.Fatalf("append outcome = %v, want APPENDED", outcome)
	}

	found, present, err := registry.FindVersion(ctx, identity, version.VersionID())
	if err != nil || !present {
		t.Fatalf("读回：present = %v err = %v", present, err)
	}
	if found.Content() != version.Content() {
		t.Fatalf("content = %#v, want %#v（登记册往返丢了内容）", found.Content(), version.Content())
	}
}
