package postgres_test

import (
	"fmt"
	"net/url"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件对真实 PostgreSQL 证两册查阅迁到 ADR-0144 之后的读口一侧（票 catalogue-read-pagination-rollout/13）：按游标翻页期间
// 有新行登进来，已翻过的页不重出、未翻的页不漏行；同序值按行标识与排序同向决胜；total 随筛选与 q 变；空册答空页。

// pageEpoch 带到微秒：库里 timestamptz 只到微秒，游标里的登记时刻要与库里那一格分毫不差。
var pageEpoch = time.Date(2026, 3, 1, 8, 0, 0, 123456000, time.UTC)

func pageSubject(index int) string { return fmt.Sprintf("SYN-OPERATOR-PAGE-P%02d", index) }

func pageGrant(index int) string { return fmt.Sprintf("SYN-PAGE-GRANT-P%02d", index) }

func pageAt(index int) time.Time { return pageEpoch.Add(time.Duration(index) * time.Minute) }

// seedOperator 直接落一行操作者，登记时刻由用例给：翻页用例要把新行落进已翻过的那一段、要几个操作者同刻，登记口只会落成此刻。
func seedOperator(t *testing.T, register operatorRegister, issuer, subject, basis string, recordedAt time.Time) accessidentity.OperatorSubject {
	t.Helper()
	if _, err := register.pool.Exec(t.Context(),
		`INSERT INTO access_identity.operator (issuer, subject, tenant_id, basis_ref, recorded_at) VALUES ($1, $2, $3, $4, $5)`,
		issuer, subject, tenantOne, basis, recordedAt); err != nil {
		t.Fatalf("落操作者 %s：%v", subject, err)
	}
	return subjectOf(t, issuer, subject)
}

func operatorPage(t *testing.T, register operatorRegister, size int, query cataloguepage.Query) cataloguepage.Result[accessidentity.OperatorCatalogueRow] {
	t.Helper()
	page, err := register.registry.ListOperators(t.Context(), tenantOne, size, query)
	if err != nil {
		t.Fatalf("ListOperators：%v", err)
	}
	return page
}

// operatorKeys 把一页写成「发行方 主体」，同一主体名挂在两个发行方下也分得开。
func operatorKeys(rows []accessidentity.OperatorCatalogueRow) []string {
	keys := make([]string, len(rows))
	for index, row := range rows {
		subject := row.Binding().Subject()
		keys[index] = subject.Issuer() + " " + subject.Subject()
	}
	return keys
}

func assertOperatorPage(t *testing.T, label string, page cataloguepage.Result[accessidentity.OperatorCatalogueRow], subjects []int, total int64) {
	t.Helper()
	want := make([]string, len(subjects))
	for index, subject := range subjects {
		want[index] = syntheticIssuer + " " + pageSubject(subject)
	}
	if got := operatorKeys(page.Rows); !slices.Equal(got, want) || page.Total != total {
		t.Fatalf("%s = %v（total %d），want %v（total %d）", label, got, page.Total, want, total)
	}
}

// Covers: ADR-0144 决定一、五与 ADR-0363 决定二——操作者册按登记时刻正序翻页：翻页期间在已翻过的那一段与未翻的那一段各落一个
// 操作者，已翻过的页不重出、未翻的页不漏行，total 随之变；每个操作者的授予与撤销随本页一起回来，各在各的操作者名下；空册答
// 空页、total 为 0；页大小非正拒。
func TestOperatorCataloguePagesByCursorWithoutRepeatsOrGapsWhileOperatorsRegister(t *testing.T) {
	register := newOperatorRegister(t)
	pages := accessidentity.OperatorCataloguePages

	empty := operatorPage(t, register, 2, firstPage(t, pages))
	if empty.Rows == nil || len(empty.Rows) != 0 || empty.Next != "" || empty.Total != 0 {
		t.Fatalf("空册 = %+v，want 空页、total 0", empty)
	}

	for index := 1; index <= 5; index++ {
		subject := seedOperator(t, register, syntheticIssuer, pageSubject(index), "SYN-BINDING-BASIS", pageAt(index))
		expectOutcome(t, "授予 "+pageSubject(index),
			register.grant(t, grantOf(t, tenantOne, pageGrant(index), subject, accessidentity.CapabilityMasterDataAndOperationsRead, grantStartsAt, time.Time{})),
			accessidentity.OperatorRegistrationRecorded)
	}
	expectOutcome(t, "撤销 P03 的授予",
		register.revoke(t, revocationOf(t, tenantOne, pageGrant(3), grantStartsAt.Add(time.Hour), "SYN-REVOKE-BASIS-P03")),
		accessidentity.OperatorRegistrationRecorded)

	first := operatorPage(t, register, 2, firstPage(t, pages))
	assertOperatorPage(t, "第一页", first, []int{1, 2}, 5)

	seedOperator(t, register, syntheticIssuer, pageSubject(0), "SYN-BINDING-BASIS", pageAt(0))
	seedOperator(t, register, syntheticIssuer, pageSubject(6), "SYN-BINDING-BASIS", pageAt(6))

	second := operatorPage(t, register, 2, nextPage(t, pages, url.Values{}, first.Next))
	assertOperatorPage(t, "第二页", second, []int{3, 4}, 7)
	third := operatorPage(t, register, 2, nextPage(t, pages, url.Values{}, second.Next))
	assertOperatorPage(t, "末页", third, []int{5, 6}, 7)
	if third.Next != "" {
		t.Fatalf("末页还给了游标 %q", third.Next)
	}

	for _, page := range [][]accessidentity.OperatorCatalogueRow{first.Rows, second.Rows, {third.Rows[0]}} {
		for _, row := range page {
			grants := row.Grants()
			want := "SYN-PAGE-GRANT-" + row.Binding().Subject().Subject()[len("SYN-OPERATOR-PAGE-"):]
			if len(grants) != 1 || grants[0].Recorded().Grant().GrantID() != want {
				t.Fatalf("%s 的授予 = %d 笔，want 只有 %s", row.Binding().Subject().Subject(), len(grants), want)
			}
			_, revoked := grants[0].Recorded().Revocation()
			if revoked != (want == pageGrant(3)) {
				t.Fatalf("%s 的撤销 = %v", want, revoked)
			}
		}
	}
	if grants := third.Rows[1].Grants(); len(grants) != 0 {
		t.Fatalf("没授予的 P06 = %d 笔授予", len(grants))
	}

	if _, err := register.registry.ListOperators(t.Context(), tenantOne, 0, firstPage(t, pages)); err == nil {
		t.Fatal("页大小 0 应拒")
	}
}

// Covers: ADR-0144 决定一、三——同一登记时刻的几个操作者按（发行方、主体）决胜、与排序同向：每页一行地翻过同刻的一组，正序与倒序
// 各走完全册，每个操作者恰好出现一次；同刻的一组跨了发行方，决胜先看发行方。
func TestOperatorCatalogueBreaksTiesOnIssuerAndSubjectInTheSortDirection(t *testing.T) {
	register := newOperatorRegister(t)
	pages := accessidentity.OperatorCataloguePages
	// 两个发行方同形、只差末一字，次序不随库的排序规则变。
	const issuerA, issuerB = "SYN-ISSUER-A", "SYN-ISSUER-B"
	seedOperator(t, register, issuerB, "SYN-OPERATOR-TIE-A", "SYN-BINDING-BASIS", pageAt(1))
	seedOperator(t, register, issuerA, "SYN-OPERATOR-TIE-B", "SYN-BINDING-BASIS", pageAt(1))
	seedOperator(t, register, issuerA, "SYN-OPERATOR-LATER", "SYN-BINDING-BASIS", pageAt(2))
	seedOperator(t, register, issuerA, "SYN-OPERATOR-TIE-A", "SYN-BINDING-BASIS", pageAt(1))
	seedOperator(t, register, issuerB, "SYN-OPERATOR-TIE-B", "SYN-BINDING-BASIS", pageAt(1))
	ascending := []string{
		issuerA + " SYN-OPERATOR-TIE-A",
		issuerA + " SYN-OPERATOR-TIE-B",
		issuerB + " SYN-OPERATOR-TIE-A",
		issuerB + " SYN-OPERATOR-TIE-B",
		issuerA + " SYN-OPERATOR-LATER",
	}
	descending := slices.Clone(ascending)
	slices.Reverse(descending)

	for label, walk := range map[string]struct {
		values url.Values
		want   []string
	}{
		"正序": {url.Values{}, ascending},
		"倒序": {url.Values{"sort": {"-recordedAt"}}, descending},
	} {
		var walked []string
		page := operatorPage(t, register, 1, pageQuery(t, pages, walk.values))
		for {
			walked = append(walked, operatorKeys(page.Rows)...)
			if page.Total != 5 {
				t.Fatalf("%s total = %d", label, page.Total)
			}
			if page.Next == "" {
				break
			}
			page = operatorPage(t, register, 1, nextPage(t, pages, walk.values, page.Next))
		}
		if !slices.Equal(walked, walk.want) {
			t.Fatalf("%s 走出 %v，want %v", label, walked, walk.want)
		}
	}
}

// Covers: ADR-0144 决定四、五——q 在主体标识与绑定依据上做字面包含、不分大小写，total 随之变；% 与 _ 按字面；带 q 照样翻得动。
func TestOperatorCatalogueTotalFollowsTheKeyword(t *testing.T) {
	register := newOperatorRegister(t)
	pages := accessidentity.OperatorCataloguePages
	for index, basis := range []string{"SYN-BINDING-ALPHA", "SYN-BINDING-BETA", "SYN-BINDING-GAMMA"} {
		seedOperator(t, register, syntheticIssuer, pageSubject(index+1), basis, pageAt(index+1))
	}

	for keyword, want := range map[string][]int{
		"":      {1, 2, 3},
		"p02":   {2},
		"beta":  {2},
		"Gamma": {3},
		"SYN":   {1, 2, 3},
		"%":     {},
		"P_2":   {},
	} {
		values := url.Values{"q": {keyword}}
		page := operatorPage(t, register, 2, pageQuery(t, pages, values))
		rows := page.Rows
		if page.Next != "" {
			rows = append(rows, operatorPage(t, register, 2, nextPage(t, pages, values, page.Next)).Rows...)
		}
		assertOperatorPage(t, "q="+keyword, cataloguepage.Result[accessidentity.OperatorCatalogueRow]{Rows: rows, Total: page.Total}, want, int64(len(want)))
	}
}

func devicePage(t *testing.T, register deviceRegister, size int, query cataloguepage.Query) cataloguepage.Result[accessidentity.DeviceCatalogueRow] {
	t.Helper()
	page, err := register.registry.ListDevices(t.Context(), tenantOne, size, query)
	if err != nil {
		t.Fatalf("ListDevices：%v", err)
	}
	return page
}

func pageDevice(index int) string { return fmt.Sprintf("SYN-DEVICE-P%02d", index) }

// walkDevices 按同一组条件从第一页翻到末页，交回走过的设备标识与每页的 total。
func walkDevices(t *testing.T, register deviceRegister, size int, values url.Values) ([]string, []int64) {
	t.Helper()
	pages := accessidentity.DeviceCataloguePages
	var walked []string
	var totals []int64
	page := devicePage(t, register, size, pageQuery(t, pages, values))
	for {
		walked = append(walked, deviceIDs(page.Rows)...)
		totals = append(totals, page.Total)
		if page.Next == "" {
			return walked, totals
		}
		page = devicePage(t, register, size, nextPage(t, pages, values, page.Next))
	}
}

func pageDevices(indexes ...int) []string {
	ids := make([]string, len(indexes))
	for position, index := range indexes {
		ids[position] = pageDevice(index)
	}
	return ids
}

// Covers: ADR-0144 决定一、四、五——作业设备册按设备标识正序翻页：翻页期间在已翻过的那一段与未翻的那一段各登一台，已翻过的页
// 不重出、未翻的页不漏行，total 随之变；倒序同样走得完；状态筛选与 q 下 total 随条件变、筛选下照样翻得动；% 与 _ 按字面；
// 空册答空页、total 为 0；页大小非正拒。
func TestDeviceCataloguePagesByCursorWithoutRepeatsOrGapsWhileDevicesRegister(t *testing.T) {
	register := newDeviceRegister(t)
	pages := accessidentity.DeviceCataloguePages
	registerDevice := func(index int) {
		scope := scopeOf(t, accessidentity.WorkScopeNode, fmt.Sprintf("SYN-NODE-%02d", index))
		expectDeviceOutcome(t, "登 "+pageDevice(index),
			register.register(t, deviceOf(t, tenantOne, pageDevice(index), scope, "SYN-DEVICE-BASIS")),
			accessidentity.DeviceRegistrationRecorded)
	}

	empty := devicePage(t, register, 2, firstPage(t, pages))
	if empty.Rows == nil || len(empty.Rows) != 0 || empty.Next != "" || empty.Total != 0 {
		t.Fatalf("空册 = %+v，want 空页、total 0", empty)
	}

	for index := 1; index <= 5; index++ {
		registerDevice(index)
	}
	for _, index := range []int{2, 4} {
		expectDeviceOutcome(t, "停用 "+pageDevice(index),
			register.deactivate(t, deactivationOf(t, tenantOne, pageDevice(index), grantStartsAt.Add(time.Hour), "SYN-DEVICE-OFF")),
			accessidentity.DeviceRegistrationRecorded)
	}

	first := devicePage(t, register, 2, firstPage(t, pages))
	if got := deviceIDs(first.Rows); !slices.Equal(got, pageDevices(1, 2)) || first.Total != 5 {
		t.Fatalf("第一页 = %v（total %d）", got, first.Total)
	}
	registerDevice(0)
	registerDevice(6)
	second := devicePage(t, register, 2, nextPage(t, pages, url.Values{}, first.Next))
	if got := deviceIDs(second.Rows); !slices.Equal(got, pageDevices(3, 4)) || second.Total != 7 {
		t.Fatalf("第二页 = %v（total %d）", got, second.Total)
	}
	third := devicePage(t, register, 2, nextPage(t, pages, url.Values{}, second.Next))
	if got := deviceIDs(third.Rows); !slices.Equal(got, pageDevices(5, 6)) || third.Next != "" || third.Total != 7 {
		t.Fatalf("末页 = %v（total %d，next %q）", got, third.Total, third.Next)
	}
	if second.Rows[1].Status() != accessidentity.DeviceCatalogueDeactivated || second.Rows[0].Status() != accessidentity.DeviceCatalogueRegistered {
		t.Fatalf("第二页的状态 = %s / %s", second.Rows[0].Status(), second.Rows[1].Status())
	}

	for label, walk := range map[string]struct {
		values url.Values
		want   []string
	}{
		"倒序":     {url.Values{"sort": {"-deviceId"}}, pageDevices(6, 5, 4, 3, 2, 1, 0)},
		"已停用":    {url.Values{"status": {"DEACTIVATED"}}, pageDevices(2, 4)},
		"在册":     {url.Values{"status": {"REGISTERED"}}, pageDevices(0, 1, 3, 5, 6)},
		"两种状态":   {url.Values{"status": {"REGISTERED", "DEACTIVATED"}}, pageDevices(0, 1, 2, 3, 4, 5, 6)},
		"已停用倒序":  {url.Values{"status": {"DEACTIVATED"}, "sort": {"-deviceId"}}, pageDevices(4, 2)},
		"设备标识":   {url.Values{"q": {"p03"}}, pageDevices(3)},
		"作业范围引用": {url.Values{"q": {"node-05"}}, pageDevices(5)},
		"筛选与 q":  {url.Values{"q": {"SYN-DEVICE"}, "status": {"DEACTIVATED"}}, pageDevices(2, 4)},
		"百分号按字面": {url.Values{"q": {"%"}}, pageDevices()},
		"下划线按字面": {url.Values{"q": {"P_3"}}, pageDevices()},
	} {
		walked, totals := walkDevices(t, register, 1, walk.values)
		if !slices.Equal(walked, walk.want) {
			t.Fatalf("%s 走出 %v，want %v", label, walked, walk.want)
		}
		for _, total := range totals {
			if total != int64(len(walk.want)) {
				t.Fatalf("%s 的 total = %v，want 每页都是 %d", label, totals, len(walk.want))
			}
		}
	}

	if _, err := register.registry.ListDevices(t.Context(), tenantOne, 0, firstPage(t, pages)); err == nil {
		t.Fatal("页大小 0 应拒")
	}
}
