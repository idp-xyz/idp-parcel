package domain

import (
	"testing"
	"time"
)

func TestPreAdviceClosedSetsNameOnlyTheirWords(t *testing.T) {
	if ArrivalModeInvalid.String() != "" || InboundModeInvalid.String() != "" || PreAdviceStatusInvalid.String() != "" ||
		NumberingAbsenceNone.String() != "" || PreAdviceContentIssueKindInvalid.String() != "" {
		t.Fatal("零值不该有名字")
	}
	for word, want := range map[string]ArrivalMode{
		"FULL_CONTAINER": ArrivalFullContainer, "PALLET": ArrivalPallet, "LOOSE_CARTON": ArrivalLooseCarton,
	} {
		got, err := NewArrivalMode(word)
		if err != nil || got != want || got.String() != word {
			t.Fatalf("到仓方式 %s = %s %v", word, got, err)
		}
	}
	for word, want := range map[string]InboundMode{"BY_GOODS": InboundByGoods, "BY_CARTON": InboundByCarton} {
		got, err := NewInboundMode(word)
		if err != nil || got != want || got.String() != word {
			t.Fatalf("入库方式 %s = %s %v", word, got, err)
		}
	}
	for word, want := range map[string]PreAdviceStatus{
		"ADVISED": PreAdviceAdvised, "RECEIVING": PreAdviceReceiving, "CLOSED": PreAdviceClosed, "CANCELLED": PreAdviceCancelled,
	} {
		got, err := NewPreAdviceStatus(word)
		if err != nil || got != want || got.String() != word {
			t.Fatalf("状态 %s = %s %v", word, got, err)
		}
	}
	if _, err := NewArrivalMode("TRUCK"); err == nil {
		t.Fatal("集外的到仓方式不该认得")
	}
	if NumberingTimeZoneNotConfigured.String() != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" {
		t.Fatalf("无号原因 = %s", NumberingTimeZoneNotConfigured)
	}
}

func TestPreAdviceSubmissionKeepsTheTwoPathsApart(t *testing.T) {
	owner, err := NewOwnerPreAdviceSubmission(mustValue(t, NewSubmissionSource, "SYN-CHANNEL"), mustValue(t, NewRequestKey, "REQ-1"))
	if err != nil || owner.Path() != SubmissionByOwner {
		t.Fatalf("货主口 = %s %v", owner.Path(), err)
	}
	if _, has := owner.EvidenceReference(); has {
		t.Fatal("货主口不带依据引用")
	}
	if _, err := NewOwnerPreAdviceSubmission(SubmissionSource{}, mustValue(t, NewRequestKey, "REQ-1")); err == nil {
		t.Fatal("货主口缺来源不该成立")
	}
	onBehalf, err := NewOnBehalfPreAdviceSubmission(mustValue(t, NewRequestKey, "DEC-1"), mustValue(t, NewEvidenceReference, "AUTH-1"))
	if err != nil || onBehalf.Path() != SubmissionOnBehalf {
		t.Fatalf("代录 = %s %v", onBehalf.Path(), err)
	}
	if _, has := onBehalf.Source(); has {
		t.Fatal("代录没有来源")
	}
	if !owner.SameSlotAs(owner) || owner.SameSlotAs(onBehalf) {
		t.Fatal("判重位置只看途径、来源与键")
	}
}

func TestPreAdviceContentIssuesCollectEveryDefect(t *testing.T) {
	known := RegisteredPreAdviceGoods([]GoodsUnitName{mustValue(t, NewGoodsUnitName, "件")}, false, true)
	lines := []GoodsInboundLine{
		mustGoodsLine(t, "SYN-MISSING", "件", 0, "", "", 0, true),
		mustGoodsLine(t, "SYN-GOODS", "托", 0, "LOT-1", "", 0, false),
		mustGoodsLine(t, "SYN-GOODS", "件", 2, "", "", 0, false),
	}
	issues := CollectGoodsInboundIssues(lines, map[string]PreAdviceGoodsFacts{"SYN-GOODS": known})
	gotGoods := []string{
		"0:-1:GOODS_NOT_REGISTERED",
		"0:-1:QUANTITY_NOT_POSITIVE",
		"0:-1:CARTON_COUNT_NOT_POSITIVE",
		"1:-1:UNIT_NOT_A_PACKAGING_LEVEL",
		"1:-1:QUANTITY_NOT_POSITIVE",
		"1:-1:BATCH_NOT_TRACKED",
	}
	if got := issueTexts(issues); len(got) != len(gotGoods) {
		t.Fatalf("商品入库的不合 = %v，want %v", got, gotGoods)
	} else {
		for index := range gotGoods {
			if got[index] != gotGoods[index] {
				t.Fatalf("商品入库的不合 = %v，want %v", got, gotGoods)
			}
		}
	}
	for _, text := range issueTexts(issues) {
		if text == "2:-1:EXPIRY_NOT_TRACKED" || text == "2:-1:EXPIRY_REQUIRED" {
			t.Fatalf("跟踪效期却没带不该拒：%v", issueTexts(issues))
		}
	}

	cartons := []Carton{
		mustCarton(t, "C-1", mustCartonLine(t, "SYN-MISSING", 1)),
		mustCarton(t, "C-2"),
		mustCarton(t, "C-1", mustCartonLine(t, "SYN-GOODS", 0), mustCartonLine(t, "SYN-GOODS", 3)),
	}
	cartonIssues := CollectCartonIssues(cartons, func(code GoodsCode) bool { return code.String() == "SYN-GOODS" })
	got := issueTexts(cartonIssues)
	wantCarton := []string{
		"0:-1:DUPLICATE_CARTON_NUMBER",
		"0:0:GOODS_NOT_REGISTERED",
		"1:-1:EMPTY_CARTON",
		"2:-1:DUPLICATE_CARTON_NUMBER",
		"0:2:QUANTITY_NOT_POSITIVE",
	}
	if len(got) != len(wantCarton) {
		t.Fatalf("整箱的不合 = %v，want %v", got, wantCarton)
	}
	for index := range wantCarton {
		if got[index] != wantCarton[index] {
			t.Fatalf("整箱的不合 = %v，want %v", got, wantCarton)
		}
	}
	if whole := issueTexts(CollectGoodsInboundIssues(nil, nil)); len(whole) != 1 || whole[0] != "-1:-1:NO_LINES" {
		t.Fatalf("没有明细 = %v", whole)
	}
}

func TestPreAdviceKeepsTheFirstNoticeWhenALaterOneIsAppended(t *testing.T) {
	advice := mustAdvised(t, "")
	if _, err := NewAdvisedPreAdvice(
		advice.Tenant(), advice.ID(), advice.Owner(), advice.Warehouse(), advice.Arrival(), advice.Inbound(),
		[]GoodsInboundLine{mustGoodsLine(t, "SYN-GOODS", "件", 0, "", "", 0, false)}, nil,
		mustValue(t, NewDocumentNumber, "IB-1"), NumberingAbsenceNone,
		advice.Submission(), advice.Registrant(), advice.AcceptedAt(), advice.Notices()[0],
	); err == nil {
		t.Fatal("数量不为正的明细不该落成预报")
	}
	later, err := NewPreAdviceNotice(2, advice.CurrentNotice().ExpectedAt(), ArrivalClue{}, OwnerReference{},
		mustOwnerPreAdviceSubmission(t, "REQ-2"), mustValue(t, NewRegistrant, "operator-2"), advice.AcceptedAt().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	updated, err := advice.AppendNotice(later)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status() != advice.Status() || len(updated.Notices()) != 2 || updated.CurrentNotice().Version() != 2 {
		t.Fatalf("告知更新后状态 %s 版本 %d", updated.Status(), updated.CurrentNotice().Version())
	}
	number, has := updated.DocumentNumber()
	original, _ := advice.DocumentNumber()
	if !has || number != original || !slicesEqualLines(updated.GoodsLines(), advice.GoodsLines()) {
		t.Fatal("告知更新动了单据号或明细")
	}
	if !updated.SameSubmissionAs(
		advice.Owner(), advice.Warehouse(), advice.Arrival(), advice.Inbound(), advice.GoodsLines(), nil,
		advice.Notices()[0].ExpectedAt(), advice.Notices()[0].Clue(), advice.Notices()[0].Reference(), advice.Submission(),
	) {
		t.Fatal("后一版告知不该让当初的提交对不上")
	}
	if updated.CurrentNotice().Clue().Given() {
		t.Fatal("没带的线索不该还在新的一版上")
	}
	withdrawal, err := NewPreAdviceWithdrawal(mustOwnerPreAdviceSubmission(t, "REQ-W"), advice.Registrant(), advice.AcceptedAt().Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := updated.Cancel(withdrawal)
	if err != nil {
		t.Fatal(err)
	}
	kept, has := cancelled.DocumentNumber()
	if cancelled.Status() != PreAdviceCancelled || !has || kept != original {
		t.Fatalf("撤回后 %s 单据号 %s", cancelled.Status(), kept)
	}
	if _, err := cancelled.Cancel(withdrawal); err != ErrPreAdviceStatus {
		t.Fatal("已取消不能再撤")
	}
	receiving, err := advice.StartReceiving()
	if err != nil || receiving.Status() != PreAdviceReceiving {
		t.Fatal(err)
	}
	if _, err := receiving.Cancel(withdrawal); err != ErrPreAdviceStatus {
		t.Fatal("收货中不能撤")
	}
	closed, err := receiving.CloseOut()
	if err != nil || closed.Status() != PreAdviceClosed {
		t.Fatal(err)
	}
	if _, has := closed.DocumentNumber(); !has {
		t.Fatal("完结不放单据号")
	}
}

func TestPreAdviceNumberAndAbsenceAreMutuallyExclusive(t *testing.T) {
	advice := mustAdvised(t, "")
	if _, err := RehydratePreAdvice(
		advice.Tenant(), advice.ID(), advice.Owner(), advice.Warehouse(), advice.Arrival(), advice.Inbound(),
		advice.GoodsLines(), nil, advice.Status(), advice.mustNumber(t), NumberingTimeZoneNotConfigured,
		advice.Submission(), advice.Registrant(), advice.AcceptedAt(), advice.Notices(), PreAdviceWithdrawal{},
	); err == nil {
		t.Fatal("有号又记无号原因不该装得回来")
	}
	without, err := RehydratePreAdvice(
		advice.Tenant(), advice.ID(), advice.Owner(), advice.Warehouse(), advice.Arrival(), advice.Inbound(),
		advice.GoodsLines(), nil, advice.Status(), DocumentNumber{}, NumberingTimeZoneNotConfigured,
		advice.Submission(), advice.Registrant(), advice.AcceptedAt(), advice.Notices(), PreAdviceWithdrawal{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, has := without.DocumentNumber(); has || without.NumberingAbsence() != NumberingTimeZoneNotConfigured {
		t.Fatalf("无号 = %s", without.NumberingAbsence())
	}
}

func (advice PreAdvice) mustNumber(t *testing.T) DocumentNumber {
	t.Helper()
	number, has := advice.DocumentNumber()
	if !has {
		t.Fatal("这份预报没有单据号")
	}
	return number
}

func mustAdvised(t *testing.T, clue string) PreAdvice {
	t.Helper()
	at := time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC)
	var arrivalClue ArrivalClue
	if clue != "" {
		arrivalClue = mustValue(t, NewArrivalClue, clue)
	}
	submission := mustOwnerPreAdviceSubmission(t, "REQ-1")
	registrant := mustValue(t, NewRegistrant, "operator-1")
	notice, err := NewPreAdviceNotice(1, at.Add(24*time.Hour), arrivalClue, OwnerReference{}, submission, registrant, at)
	if err != nil {
		t.Fatal(err)
	}
	number := mustValue(t, NewDocumentNumber, "IB-20261120-00001")
	advice, err := NewAdvisedPreAdvice(
		mustValue(t, NewTenantID, "SYN-TENANT-01"), mustValue(t, NewPreAdviceID, "IPA-1"),
		mustValue(t, NewOwnerAccountID, "SYN-OWNER-01"), mustValue(t, NewWarehouseCode, "SYN-WH-01"),
		ArrivalPallet, InboundByGoods,
		[]GoodsInboundLine{mustGoodsLine(t, "SYN-GOODS", "件", 2, "", "", 0, false)}, nil,
		number, NumberingAbsenceNone, submission, registrant, at, notice,
	)
	if err != nil {
		t.Fatal(err)
	}
	return advice
}

func mustOwnerPreAdviceSubmission(t *testing.T, key string) PreAdviceSubmission {
	t.Helper()
	submission, err := NewOwnerPreAdviceSubmission(mustValue(t, NewSubmissionSource, "SYN-CHANNEL"), mustValue(t, NewRequestKey, key))
	if err != nil {
		t.Fatal(err)
	}
	return submission
}

func mustGoodsLine(t *testing.T, code, unit string, quantity int64, batch, expiry string, cartons int64, cartonsGiven bool) GoodsInboundLine {
	t.Helper()
	var batchNumber BatchNumber
	var expiryDate ExpiryDate
	var err error
	if batch != "" {
		batchNumber, err = NewBatchNumber(batch)
		if err != nil {
			t.Fatal(err)
		}
	}
	if expiry != "" {
		expiryDate, err = NewExpiryDate(expiry)
		if err != nil {
			t.Fatal(err)
		}
	}
	var count CartonCount
	if cartonsGiven {
		count = GivenCartonCount(cartons)
	}
	line, err := NewGoodsInboundLine(mustValue(t, NewGoodsCode, code), mustValue(t, NewGoodsUnitName, unit), quantity, batchNumber, expiryDate, count)
	if err != nil {
		t.Fatal(err)
	}
	return line
}

func mustCarton(t *testing.T, number string, lines ...CartonContentLine) Carton {
	t.Helper()
	carton, err := NewCarton(mustValue(t, NewCartonNumber, number), lines)
	if err != nil {
		t.Fatal(err)
	}
	return carton
}

func mustCartonLine(t *testing.T, code string, quantity int64) CartonContentLine {
	t.Helper()
	line, err := NewCartonContentLine(mustValue(t, NewGoodsCode, code), quantity)
	if err != nil {
		t.Fatal(err)
	}
	return line
}

func mustValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("%q: %v", raw, err)
	}
	return value
}

func issueTexts(issues []PreAdviceContentIssue) []string {
	texts := make([]string, len(issues))
	for index, issue := range issues {
		texts[index] = issue.String()
	}
	return texts
}

func slicesEqualLines(left, right []GoodsInboundLine) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
