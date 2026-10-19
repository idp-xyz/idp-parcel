package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件证送站预报与预报有效时窗册的领域形状（ADR-0212 决定二；NO CONTEXT「送站预报」）：号码集合非空、不重复、逐字，撤回是
// 另一份事实，时窗的区间与重叠。夹具全为合成（S 级）。

var (
	preAdviceExpectedAt = time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	preAdviceFormedAt   = time.Date(2026, 10, 19, 18, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
)

func preAdviceSpec(t *testing.T, numbers ...string) domain.DeliveryPreAdviceSpec {
	t.Helper()
	listed := make([]domain.PreAdvisedParcelNumber, 0, len(numbers))
	for _, number := range numbers {
		listed = append(listed, mustBuild(t, domain.NewPreAdvisedParcelNumber, number))
	}
	return domain.DeliveryPreAdviceSpec{
		TenantID:          mustBuild(t, domain.NewTenantID, "SYN-TENANT-01"),
		ID:                mustBuild(t, domain.NewDeliveryPreAdviceID, "SYN-PREADV-PSXM08-01"),
		CustomerAccount:   mustBuild(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-01"),
		Node:              mustBuild(t, domain.NewNodeReference, "SYN-NODE-SHA-HUB"),
		ExpectedArrivalAt: preAdviceExpectedAt,
		Numbers:           listed,
		Source: domain.CustomerRequestSource{
			Channel:       "https://id.syn.example/dex#syn-customer-client-01",
			RequestKey:    "SYN-REQ-PSXM08-01",
			ContentDigest: "sha256:SYN-DIGEST-PSXM08-01",
		},
		FormedAt: preAdviceFormedAt,
	}
}

// Covers: ADR-0212 决定二「客户包裹件号集合（非空、不重复、逐字保存）」——空集合与逐字重复立不起来；只差空白或大小写的两个号
// 是两个号，照原样保存；交付方线索可以不给；时刻按 UTC 存。
func TestAPreAdviceListsANonEmptySetOfVerbatimNumbers(t *testing.T) {
	formed, err := domain.FormDeliveryPreAdvice(preAdviceSpec(t, "SYN-CPN-A1", " SYN-CPN-A1", "syn-cpn-a1"))
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	numbers := formed.Numbers()
	if len(numbers) != 3 || numbers[0].String() != "SYN-CPN-A1" || numbers[1].String() != " SYN-CPN-A1" || numbers[2].String() != "syn-cpn-a1" {
		t.Fatalf("numbers = %v, want the three verbatim in the given order", numbers)
	}
	if _, present := formed.DeliveringParty(); present {
		t.Fatal("没给交付方线索却在场")
	}
	if formed.FormedAt().Location() != time.UTC || !formed.FormedAt().Equal(preAdviceFormedAt) {
		t.Fatalf("formedAt = %v, want the same instant in UTC", formed.FormedAt())
	}
	if !formed.Lists(" SYN-CPN-A1") || formed.Lists("SYN-CPN-A1 ") || formed.Lists("SYN-CPN-a1") {
		t.Fatal("Lists 应逐字比对")
	}

	for name, spec := range map[string]domain.DeliveryPreAdviceSpec{
		"号码集合为空":  preAdviceSpec(t),
		"同一个号报两次": preAdviceSpec(t, "SYN-CPN-A1", "SYN-CPN-B1", "SYN-CPN-A1"),
	} {
		if _, err := domain.FormDeliveryPreAdvice(spec); !errors.Is(err, domain.ErrInvalidDeliveryPreAdvice) {
			t.Fatalf("%s: err = %v, want ErrInvalidDeliveryPreAdvice", name, err)
		}
	}

	missing := map[string]func(*domain.DeliveryPreAdviceSpec){
		"缺租户":     func(spec *domain.DeliveryPreAdviceSpec) { spec.TenantID = domain.TenantID{} },
		"缺标识":     func(spec *domain.DeliveryPreAdviceSpec) { spec.ID = domain.DeliveryPreAdviceID{} },
		"缺客户账户":   func(spec *domain.DeliveryPreAdviceSpec) { spec.CustomerAccount = domain.CustomerAccountReference{} },
		"缺节点":     func(spec *domain.DeliveryPreAdviceSpec) { spec.Node = domain.NodeReference{} },
		"缺预计送达时刻": func(spec *domain.DeliveryPreAdviceSpec) { spec.ExpectedArrivalAt = time.Time{} },
		"缺来源请求键":  func(spec *domain.DeliveryPreAdviceSpec) { spec.Source.RequestKey = " " },
		"缺来源":     func(spec *domain.DeliveryPreAdviceSpec) { spec.Source.Channel = "" },
		"缺内容摘要":   func(spec *domain.DeliveryPreAdviceSpec) { spec.Source.ContentDigest = "" },
		"缺形成时刻":   func(spec *domain.DeliveryPreAdviceSpec) { spec.FormedAt = time.Time{} },
		"号码是零值": func(spec *domain.DeliveryPreAdviceSpec) {
			spec.Numbers = append(spec.Numbers, domain.PreAdvisedParcelNumber{})
		},
	}
	for name, mutate := range missing {
		spec := preAdviceSpec(t, "SYN-CPN-A1")
		mutate(&spec)
		if _, err := domain.FormDeliveryPreAdvice(spec); !errors.Is(err, domain.ErrInvalidDeliveryPreAdvice) {
			t.Fatalf("%s: err = %v, want ErrInvalidDeliveryPreAdvice", name, err)
		}
	}

	withClue := preAdviceSpec(t, "SYN-CPN-A1")
	withClue.DeliveringParty = mustBuild(t, domain.NewDeliveringPartyReference, "SYN-COURIER-PSXM08")
	clued, err := domain.FormDeliveryPreAdvice(withClue)
	if err != nil {
		t.Fatalf("form with clue: %v", err)
	}
	if clue, present := clued.DeliveringParty(); !present || clue.String() != "SYN-COURIER-PSXM08" {
		t.Fatalf("clue = %q %v", clue.String(), present)
	}
}

// Covers: NO CONTEXT 生命周期「送站预报」——已登记 → 已撤回是另一份事实：撤回指名预报与撤回它的客户账户，带自己的来源请求键；
// 证据状态据它答已撤回。
func TestAWithdrawalIsItsOwnFactAboutAPreAdvice(t *testing.T) {
	preAdvice, err := domain.FormDeliveryPreAdvice(preAdviceSpec(t, "SYN-CPN-A1"))
	if err != nil {
		t.Fatal(err)
	}
	withdrawal, err := domain.FormDeliveryPreAdviceWithdrawal(domain.DeliveryPreAdviceWithdrawalSpec{
		TenantID:        preAdvice.TenantID(),
		PreAdvice:       preAdvice.ID(),
		CustomerAccount: preAdvice.CustomerAccount(),
		Source: domain.CustomerRequestSource{
			Channel: "https://id.syn.example/dex#syn-customer-client-01", RequestKey: "SYN-REQ-PSXM08-W1", ContentDigest: "sha256:SYN-W1",
		},
		WithdrawnAt: preAdviceExpectedAt,
	})
	if err != nil {
		t.Fatalf("form withdrawal: %v", err)
	}
	if (domain.DeliveryPreAdviceState{PreAdvice: preAdvice}).Withdrawn() {
		t.Fatal("没撤回却答已撤回")
	}
	state := domain.DeliveryPreAdviceState{PreAdvice: preAdvice, Withdrawal: withdrawal}
	if !state.Withdrawn() || !state.Registered() {
		t.Fatalf("state = %+v, want registered and withdrawn", state)
	}
	if (domain.DeliveryPreAdviceState{}).Registered() {
		t.Fatal("零值的证据状态应答不在册")
	}
	if _, err := domain.FormDeliveryPreAdviceWithdrawal(domain.DeliveryPreAdviceWithdrawalSpec{
		TenantID: preAdvice.TenantID(), PreAdvice: preAdvice.ID(), CustomerAccount: preAdvice.CustomerAccount(),
		WithdrawnAt: preAdviceExpectedAt,
	}); !errors.Is(err, domain.ErrInvalidDeliveryPreAdvice) {
		t.Fatalf("缺来源的撤回: err = %v", err)
	}
}

func windowSpec(t *testing.T, from, until time.Time) domain.PreAdviceValidityWindowSpec {
	t.Helper()
	return domain.PreAdviceValidityWindowSpec{
		TenantID:       mustBuild(t, domain.NewTenantID, "SYN-TENANT-01"),
		Lead:           24 * time.Hour,
		Grace:          6 * time.Hour,
		EffectiveFrom:  from,
		EffectiveUntil: until,
		Basis:          "SYN-BASIS-PSXM08-WINDOW",
		RegisteredAt:   preAdviceFormedAt,
	}
}

// Covers: ADR-0212 决定二末项与 PAR-NET-18「区间是〔预计送达时刻减提前量，预计送达时刻加宽限量〕」——两端都算在内；生效区间
// 半开，终点可以没有；提前量与宽限量不得为负、以分钟为粒度；登记依据必备。都没有默认。
func TestAValidityWindowAdmitsTheClosedSpanAroundTheExpectedArrival(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	window, err := domain.FormPreAdviceValidityWindow(windowSpec(t, from, time.Time{}))
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if !window.Registered() {
		t.Fatal("formed window should be registered")
	}
	if _, bounded := window.EffectiveUntil(); bounded {
		t.Fatal("没给终点却有终点")
	}
	cases := map[string]struct {
		receivedAt time.Time
		admitted   bool
	}{
		"提前量那一端":   {preAdviceExpectedAt.Add(-24 * time.Hour), true},
		"再早一秒":     {preAdviceExpectedAt.Add(-24*time.Hour - time.Second), false},
		"宽限量那一端":   {preAdviceExpectedAt.Add(6 * time.Hour), true},
		"再晚一秒":     {preAdviceExpectedAt.Add(6*time.Hour + time.Second), false},
		"恰在预计送达时刻": {preAdviceExpectedAt, true},
	}
	for name, testCase := range cases {
		if got := window.Admits(preAdviceExpectedAt, testCase.receivedAt); got != testCase.admitted {
			t.Fatalf("%s: admits = %v, want %v", name, got, testCase.admitted)
		}
	}
	if !window.EffectiveAt(from) || window.EffectiveAt(from.Add(-time.Nanosecond)) {
		t.Fatal("生效区间应含起点、不含起点之前")
	}

	bounded, err := domain.FormPreAdviceValidityWindow(windowSpec(t, from, from.AddDate(0, 6, 0)))
	if err != nil {
		t.Fatal(err)
	}
	if bounded.EffectiveAt(from.AddDate(0, 6, 0)) || !bounded.EffectiveAt(from.AddDate(0, 6, 0).Add(-time.Second)) {
		t.Fatal("生效区间应不含终点")
	}

	refused := map[string]func(*domain.PreAdviceValidityWindowSpec){
		"提前量为负":   func(spec *domain.PreAdviceValidityWindowSpec) { spec.Lead = -time.Minute },
		"宽限量为负":   func(spec *domain.PreAdviceValidityWindowSpec) { spec.Grace = -time.Minute },
		"提前量不足整分": func(spec *domain.PreAdviceValidityWindowSpec) { spec.Lead = 90 * time.Second },
		"缺起点":     func(spec *domain.PreAdviceValidityWindowSpec) { spec.EffectiveFrom = time.Time{} },
		"终点不晚于起点": func(spec *domain.PreAdviceValidityWindowSpec) { spec.EffectiveUntil = spec.EffectiveFrom },
		"缺登记依据":   func(spec *domain.PreAdviceValidityWindowSpec) { spec.Basis = " \t" },
		"缺租户":     func(spec *domain.PreAdviceValidityWindowSpec) { spec.TenantID = domain.TenantID{} },
		"缺登记时刻":   func(spec *domain.PreAdviceValidityWindowSpec) { spec.RegisteredAt = time.Time{} },
	}
	for name, mutate := range refused {
		spec := windowSpec(t, from, time.Time{})
		mutate(&spec)
		if _, err := domain.FormPreAdviceValidityWindow(spec); !errors.Is(err, domain.ErrInvalidPreAdviceValidityWindow) {
			t.Fatalf("%s: err = %v, want ErrInvalidPreAdviceValidityWindow", name, err)
		}
	}
}

// Covers: ADR-0212 决定二「同租户任一时刻至多一行生效，重叠在登记时拒」的判据——半开区间首尾相接不算重叠，开放终点与任何更晚
// 起点都重叠；内容比对不看登记时刻（重放判据）。
func TestValidityWindowsOverlapOnTheirHalfOpenEffectiveIntervals(t *testing.T) {
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	jul := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	form := func(from, until time.Time) domain.PreAdviceValidityWindow {
		t.Helper()
		window, err := domain.FormPreAdviceValidityWindow(windowSpec(t, from, until))
		if err != nil {
			t.Fatal(err)
		}
		return window
	}
	firstHalf := form(jan, jul)
	secondHalfOpen := form(jul, time.Time{})
	allYearOpen := form(jan, time.Time{})
	if firstHalf.Overlaps(secondHalfOpen) || secondHalfOpen.Overlaps(firstHalf) {
		t.Fatal("首尾相接的两段不该重叠")
	}
	if !allYearOpen.Overlaps(secondHalfOpen) || !secondHalfOpen.Overlaps(allYearOpen) || !allYearOpen.Overlaps(firstHalf) {
		t.Fatal("开放终点应与任何更晚起点重叠")
	}

	again := windowSpec(t, jan, jul)
	again.RegisteredAt = again.RegisteredAt.Add(time.Hour)
	replayed, err := domain.FormPreAdviceValidityWindow(again)
	if err != nil {
		t.Fatal(err)
	}
	if !firstHalf.SameContent(replayed) {
		t.Fatal("只差登记时刻应算同一内容")
	}
	changed := windowSpec(t, jan, jul)
	changed.Grace = 7 * time.Hour
	other, err := domain.FormPreAdviceValidityWindow(changed)
	if err != nil {
		t.Fatal(err)
	}
	if firstHalf.SameContent(other) {
		t.Fatal("宽限量不同不该算同一内容")
	}
}
