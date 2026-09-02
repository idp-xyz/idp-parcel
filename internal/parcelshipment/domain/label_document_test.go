package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func submittedForDocuments(t *testing.T, parcels ...string) domain.LabelTransaction {
	t.Helper()

	transaction := establishedLabelTransaction(t, parcels...)
	submitted, err := transaction.SubmitToChannel(labelTransactionAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("提交渠道：%v", err)
	}
	return submitted
}

func documentSpec(t *testing.T, granularity domain.LabelDocumentGranularity, parcels ...string) domain.RecordLabelDocumentSpec {
	t.Helper()

	covered := make([]domain.DeclaredParcelID, 0, len(parcels))
	for _, parcel := range parcels {
		covered = append(covered, mustValue(t, domain.NewDeclaredParcelID, parcel))
	}
	return domain.RecordLabelDocumentSpec{
		Role:           mustValue(t, domain.NewLabelDocumentRole, "shipping-label"),
		Format:         mustValue(t, domain.NewLabelDocumentFormat, "channel-declared-format"),
		Granularity:    granularity,
		CoveredParcels: covered,
		Digest:         mustValue(t, domain.NewLabelDocumentDigest, "digest-1"),
		ObservedAt:     labelTransactionAt.Add(2 * time.Minute),
	}
}

// 载荷只增不改：聚合上没有任何改写或删除的路径，重打与替换各自产生新的一条并保留原条
// （ADR-0092 决定三）。这一条钉的是「原条还在」——顶替最省事的写法是就地改，而那样客户手上
// 那张纸是哪一版事后就无从回答。
func TestAppendingASecondDocumentKeepsTheFirstOne(t *testing.T) {
	t.Parallel()

	transaction := submittedForDocuments(t, "parcel-1")

	first := documentSpec(t, domain.LabelDocumentPerParcel, "parcel-1")
	withFirst, err := transaction.AppendLabelDocument(first)
	if err != nil {
		t.Fatalf("追加第一条：%v", err)
	}

	second := documentSpec(t, domain.LabelDocumentPerParcel, "parcel-1")
	second.Digest = mustValue(t, domain.NewLabelDocumentDigest, "digest-2")
	second.ObservedAt = labelTransactionAt.Add(3 * time.Minute)
	withSecond, err := withFirst.AppendLabelDocument(second)
	if err != nil {
		t.Fatalf("追加第二条：%v", err)
	}

	documents := withSecond.LabelDocuments()
	if len(documents) != 2 {
		t.Fatalf("应有两条载荷记录，实得 %d 条", len(documents))
	}
	if documents[0].Digest().String() != "digest-1" || documents[1].Digest().String() != "digest-2" {
		t.Errorf("顺序应即追加顺序，实得 %q、%q",
			documents[0].Digest().String(), documents[1].Digest().String())
	}

	// 值语义：在派生值上追加不得回头改动原值。两份聚合共享同一底层数组时，这里会看到两条。
	if got := len(withFirst.LabelDocuments()); got != 1 {
		t.Errorf("原聚合值不该被第二次追加改动，实得 %d 条", got)
	}
}

// 逐件恰一件，批至少一件。两格都放开，粒度这个字段就不再约束任何东西，与不写它无异。
func TestGranularityDecidesHowManyParcelsADocumentMayCover(t *testing.T) {
	t.Parallel()

	transaction := submittedForDocuments(t, "parcel-1", "parcel-2")

	perParcelOverTwo := documentSpec(t, domain.LabelDocumentPerParcel, "parcel-1", "parcel-2")
	if _, err := transaction.AppendLabelDocument(perParcelOverTwo); !errors.Is(err, domain.ErrInvalidLabelTransaction) {
		t.Errorf("逐件粒度覆盖两件应被拒，实得 %v", err)
	}

	perBatchOverTwo := documentSpec(t, domain.LabelDocumentPerBatch, "parcel-1", "parcel-2")
	batched, err := transaction.AppendLabelDocument(perBatchOverTwo)
	if err != nil {
		t.Fatalf("批粒度覆盖两件应被接受：%v", err)
	}
	if got := len(batched.LabelDocuments()[0].CoveredParcels()); got != 2 {
		t.Errorf("批粒度那一条应逐件列出覆盖范围，实得 %d 件", got)
	}

	noParcel := documentSpec(t, domain.LabelDocumentPerBatch)
	if _, err := transaction.AppendLabelDocument(noParcel); !errors.Is(err, domain.ErrInvalidLabelTransaction) {
		t.Errorf("不列覆盖范围应被拒，实得 %v", err)
	}

	outside := documentSpec(t, domain.LabelDocumentPerParcel, "parcel-3")
	if _, err := transaction.AppendLabelDocument(outside); !errors.Is(err, domain.ErrInvalidLabelTransaction) {
		t.Errorf("覆盖范围之外的包裹应被拒，实得 %v", err)
	}
}

// 摘要必备、定位符可缺，两者不对称是本记录的关键一格：摘要证明我们收到过这份件，定位符
// 回答它此刻在哪。合成一格，「收到了但没处放」就表达不出来——而那是今天唯一走得到的分支。
func TestADocumentNeedsItsDigestButNotYetAPlaceToLive(t *testing.T) {
	t.Parallel()

	transaction := submittedForDocuments(t, "parcel-1")

	withoutDigest := documentSpec(t, domain.LabelDocumentPerParcel, "parcel-1")
	withoutDigest.Digest = domain.LabelDocumentDigest{}
	if _, err := transaction.AppendLabelDocument(withoutDigest); !errors.Is(err, domain.ErrInvalidLabelTransaction) {
		t.Errorf("没有摘要应被拒，实得 %v", err)
	}

	withoutLocator := documentSpec(t, domain.LabelDocumentPerParcel, "parcel-1")
	stored, err := transaction.AppendLabelDocument(withoutLocator)
	if err != nil {
		t.Fatalf("没有定位符应被接受：%v", err)
	}
	record := stored.LabelDocuments()[0]
	if record.BodyStored() {
		t.Error("没有定位符时本体不该报告已存放")
	}
	if record.Digest().String() == "" {
		t.Error("摘要应留下——它是「我们确实收到过这份件」的全部证据")
	}

	withLocator := documentSpec(t, domain.LabelDocumentPerParcel, "parcel-1")
	withLocator.Locator = mustValue(t, domain.NewLabelDocumentLocator, "store://label/1")
	placed, err := transaction.AppendLabelDocument(withLocator)
	if err != nil {
		t.Fatalf("带定位符应被接受：%v", err)
	}
	if !placed.LabelDocuments()[0].BodyStored() {
		t.Error("带定位符时本体应报告已存放")
	}
}

// `已建立`还没提交渠道，没有渠道会交回件；`失败`整笔未受理，一份面单也不会产生。
func TestNoDocumentBeforeSubmissionOrOnAWhollyFailedTransaction(t *testing.T) {
	t.Parallel()

	established := establishedLabelTransaction(t, "parcel-1")
	if _, err := established.AppendLabelDocument(documentSpec(t, domain.LabelDocumentPerParcel, "parcel-1")); !errors.Is(err, domain.ErrLabelTransactionStateNotAdmitted) {
		t.Errorf("未提交渠道时应拒，实得 %v", err)
	}

	failed, err := submittedForDocuments(t, "parcel-1").RecordChannelResult(domain.RecordChannelResultSpec{
		Outcome: domain.LabelTransactionFailed,
		ParcelResults: []domain.LabelTransactionParcelResultSpec{{
			Parcel:   mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
			Accepted: false,
			Reason:   mustValue(t, domain.NewChannelResultReasonReference, "reason-rejected"),
		}},
		ObservedAt: labelTransactionAt.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("记失败结果：%v", err)
	}
	if _, err := failed.AppendLabelDocument(documentSpec(t, domain.LabelDocumentPerParcel, "parcel-1")); !errors.Is(err, domain.ErrLabelTransactionStateNotAdmitted) {
		t.Errorf("整笔失败时应拒，实得 %v", err)
	}
}

// 件可能随提交应答就回来，那时还没有任何结果痕迹。这一条钉的是那个中间态成立——把载荷绑到
// 「有结果之后」会让同次回件的渠道无处落。
func TestADocumentMayArriveWithTheSubmissionBeforeAnyResult(t *testing.T) {
	t.Parallel()

	transaction := submittedForDocuments(t, "parcel-1")
	withDocument, err := transaction.AppendLabelDocument(documentSpec(t, domain.LabelDocumentPerParcel, "parcel-1"))
	if err != nil {
		t.Fatalf("提交后未出结果时追加载荷：%v", err)
	}
	if got := withDocument.State(); got != domain.LabelTransactionSubmitted {
		t.Errorf("追加载荷不该改变状态，实得 %q", got)
	}
	if withDocument.Finalized() {
		t.Error("追加载荷不该使交易定案")
	}
}
