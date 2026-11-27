package postgres_test

import (
	"net/url"
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func TestPreAdviceCatalogueListsOneOwnerAndFindsByDocumentNumber(t *testing.T) {
	world := newPreAdviceWorld(t, preAdviceNumberingInstant)
	world.seedWarehouse(t, nil)
	world.seedGoods(t)
	accepted := world.accept(t, preAdviceAcceptCommand(t, "REQ-1", func(command *application.AcceptPreAdviceCommand) {
		command.Submission = onBehalfPreAdviceSubmission(t, "SYN-DECISION-01", "SYN-EVIDENCE-01")
	}))
	advice, has := accepted.PreAdvice()
	number, numbered := advice.DocumentNumber()
	if accepted.Outcome() != application.PreAdviceOutcomeAdvised || !has || !numbered {
		t.Fatalf("受理 = %s", accepted.Outcome())
	}
	catalogue, err := postgres.NewPreAdviceCatalogue(world.db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	owner := valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01")
	query, err := ports.PreAdvicePages.Decode(nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := catalogue.ListPreAdvices(t.Context(), tenant, owner, 20, query)
	if err != nil || page.Total != 1 || len(page.Rows) != 1 || page.Rows[0].ID() != advice.ID() {
		t.Fatalf("列表 total=%d rows=%d err=%v", page.Total, len(page.Rows), err)
	}
	if len(page.Rows[0].Notices()) != 1 || page.Rows[0].CurrentNotice().Version() != 1 {
		t.Fatalf("告知版本 = %d", len(page.Rows[0].Notices()))
	}
	advised, err := ports.PreAdvicePages.Decode(url.Values{"status": {"ADVISED"}})
	if err != nil {
		t.Fatal(err)
	}
	narrowed, err := catalogue.ListPreAdvices(t.Context(), tenant, owner, 20, advised)
	if err != nil || narrowed.Total != 1 || len(narrowed.Rows) != 1 || narrowed.Rows[0].Status() != domain.PreAdviceAdvised {
		t.Fatalf("已预报 total=%d rows=%d err=%v", narrowed.Total, len(narrowed.Rows), err)
	}
	closed, err := ports.PreAdvicePages.Decode(url.Values{"status": {"CLOSED"}})
	if err != nil {
		t.Fatal(err)
	}
	none, err := catalogue.ListPreAdvices(t.Context(), tenant, owner, 20, closed)
	if err != nil || none.Total != 0 || len(none.Rows) != 0 {
		t.Fatalf("已完结 total=%d err=%v", none.Total, err)
	}
	other, err := catalogue.ListPreAdvices(t.Context(), tenant, valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-02"), 20, query)
	if err != nil || other.Total != 0 || len(other.Rows) != 0 {
		t.Fatalf("别的货主 total=%d err=%v", other.Total, err)
	}
	found, ok, err := catalogue.PreAdviceByDocumentNumber(t.Context(), tenant, number)
	if err != nil || !ok || found.ID() != advice.ID() {
		t.Fatalf("按号找到 %v %v %v", ok, found.ID(), err)
	}
	_, ok, err = catalogue.PreAdviceByDocumentNumber(t.Context(), tenant, valueOf(t, domain.NewDocumentNumber, "IB-NOT-A-NUMBER"))
	if err != nil || ok {
		t.Fatalf("不存在的号 ok=%v err=%v", ok, err)
	}
	loaded, ok, err := catalogue.PreAdviceForOwner(t.Context(), tenant, owner, advice.ID())
	if err != nil || !ok || loaded.Submission().Path() != domain.SubmissionOnBehalf {
		t.Fatalf("按货主读 %v %s %v", ok, loaded.Submission().Path(), err)
	}
	evidence, given := loaded.Submission().EvidenceReference()
	if !given || evidence.String() != "SYN-EVIDENCE-01" {
		t.Fatalf("依据 = %s %v", evidence, given)
	}
}

func onBehalfPreAdviceSubmission(t *testing.T, key, evidence string) domain.PreAdviceSubmission {
	t.Helper()
	submission, err := domain.NewOnBehalfPreAdviceSubmission(
		valueOf(t, domain.NewRequestKey, key), valueOf(t, domain.NewEvidenceReference, evidence))
	if err != nil {
		t.Fatal(err)
	}
	return submission
}
