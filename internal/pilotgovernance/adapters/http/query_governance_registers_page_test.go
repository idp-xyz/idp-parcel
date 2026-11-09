package governancehttp_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	governancehttp "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 四册各自的可排维；缺省序都是它的倒序。
var registerSortDimensions = map[string]string{
	"authority-interval": "insertedAt",
	"suspension":         "occurredAt",
	"resumption":         "decidedAt",
	"takeover":           "effectiveAt",
}

func registryRequest(target string) *http.Request {
	return httptest.NewRequest(http.MethodGet, "/governance-registers"+target, nil)
}

// Covers: ADR-0144 决定五、六——四册的答复都带 page（size 取 Intake 给的页大小、next 与 total 原样带出，末页 next 为 null）；
// register 选中的那一册的声明解出查询对象，原样交给读口；不带 sort 时是该册的缺省序（倒序）。
func TestGovernanceRegistersCarryThePageAndPassTheDecodedQuery(t *testing.T) {
	for register, dimension := range registerSortDimensions {
		t.Run(register, func(t *testing.T) {
			reader := &stubRegistryReader{next: "SYN-cursor", total: 7}
			endpoint := governancehttp.NewQueryGovernanceRegistersEndpoint(grantedIntake(t, 25), reader)
			response := httptest.NewRecorder()
			endpoint.ServeHTTP(response, registryRequest("?register="+register+"&q=%20SYN-GOV%20&sort="+dimension))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d；body = %s", response.Code, response.Body)
			}
			page := pageOf(t, response)
			if string(page["size"]) != "25" || string(page["next"]) != `"SYN-cursor"` || string(page["total"]) != "7" {
				t.Fatalf("page 不对：%s", response.Body.Bytes())
			}
			if reader.gotLimit != 25 || reader.gotQuery.Sort != (cataloguepage.Sort{Field: dimension}) ||
				reader.gotQuery.Keyword != "SYN-GOV" || reader.gotQuery.After != nil {
				t.Fatalf("读口收到的查询不对：limit %d %+v", reader.gotLimit, reader.gotQuery)
			}

			last := &stubRegistryReader{total: 0}
			response = httptest.NewRecorder()
			governancehttp.NewQueryGovernanceRegistersEndpoint(grantedIntake(t, 25), last).
				ServeHTTP(response, registryRequest("?register="+register))
			if page := pageOf(t, response); string(page["next"]) != "null" || string(page["total"]) != "0" {
				t.Fatalf("末页的 page 不对：%s", response.Body.Bytes())
			}
			if last.gotQuery.Sort != (cataloguepage.Sort{Field: dimension, Descending: true}) {
				t.Fatalf("不带 sort 时应是缺省序 -%s，得 %+v", dimension, last.gotQuery.Sort)
			}
		})
	}
}

// Covers: ADR-0144 决定四「有状态的册至少放出状态一维」——权威区间的 open、暂停的 resumed 照选中那一册的声明解成查询对象的
// 筛选，维内为或、去重升序，原样交给读口。
func TestGovernanceRegistersHandTheStatusDimensionToTheReader(t *testing.T) {
	for _, testCase := range []struct {
		query     string
		dimension string
		want      string
	}{
		{"?register=authority-interval&open=true&open=false", "open", "false,true"},
		{"?register=suspension&resumed=false&resumed=false", "resumed", "false"},
	} {
		t.Run(testCase.dimension, func(t *testing.T) {
			reader := &stubRegistryReader{}
			response := httptest.NewRecorder()
			governancehttp.NewQueryGovernanceRegistersEndpoint(grantedIntake(t, 25), reader).
				ServeHTTP(response, registryRequest(testCase.query))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d；body = %s", response.Code, response.Body)
			}
			if got := reader.gotQuery.Filters; len(got) != 1 || strings.Join(got[testCase.dimension], ",") != testCase.want {
				t.Fatalf("读口收到的筛选 = %v，应只有 %s=%s", got, testCase.dimension, testCase.want)
			}
		})
	}
}

// Covers: ADR-0144 决定一、三、四——集外的键、排序维与筛选键，解不开的游标、换了条件或换了一册还拿来的旧游标，都答 400
// 带理由散文，读口不被调用。register 是选择器、取值进游标摘要：暂停册的游标拿去翻恢复册即拒。register 与解码都在 Intake
// 之后：渠道未配置时参数写对写错、register 缺席或写错都答 403，答复不随请求内容变（ADR-0055 决定三）。
func TestGovernanceRegistersRefuseQueriesOutsideTheDeclaration(t *testing.T) {
	selected := url.Values{"register": {"suspension"}, "q": {"SYN-A"}}
	keyed, err := ports.SuspensionRegistryPages.Decode(selected)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := keyed.CursorAfter(cataloguepage.Position{
		Value:    cataloguepage.FormatInstant(registryEndpointAt),
		Identity: []string{"SYN-GOV-SUS-0001"},
	})
	if err != nil {
		t.Fatal(err)
	}

	refusals := map[string]struct {
		query  string
		points string
	}{
		"未知键":       {"?register=suspension&tenant=SYN-TENANT-02", "tenant"},
		"页大小不由调用方给": {"?register=authority-interval&limit=9999", "limit"},
		"筛选维不在声明里":  {"?register=takeover&authority=SYN-AUTH/successor", "authority"},
		"集外排序维":     {"?register=authority-interval&sort=fromAt", "fromAt"},
		"别册的排序维":    {"?register=resumption&sort=-occurredAt", "occurredAt"},
		"解不开的游标":    {"?register=suspension&after=SYN-not-a-cursor", "游标"},
		"换了条件的旧游标":  {"?register=suspension&q=SYN-B&after=" + stale, "游标"},
		"换了一册的旧游标":  {"?register=resumption&q=SYN-A&after=" + stale, "游标"},
		"坏转义":       {"?register=suspension&q=%zz", "解不开的一段"},
		"区间状态词表外":   {"?register=authority-interval&open=maybe", "open"},
		"暂停状态词表外":   {"?register=suspension&resumed=yes", "resumed"},
		"恢复册没有状态维":  {"?register=resumption&resumed=true", "resumed"},
	}
	for name, refusal := range refusals {
		t.Run(name, func(t *testing.T) {
			endpoint := governancehttp.NewQueryGovernanceRegistersEndpoint(grantedIntake(t, 25), unreachableRegistryReader{t: t})
			response := httptest.NewRecorder()
			endpoint.ServeHTTP(response, registryRequest(refusal.query))
			if detail := assertPageQueryRefused(t, response); !strings.Contains(detail, refusal.points) {
				t.Fatalf("理由散文应点到 %s：%q", refusal.points, detail)
			}
			if _, present := registryBody(t, response)["outcome"]; present {
				t.Fatalf("拒绝带了 outcome：%s", response.Body)
			}
		})
	}

	// 同一个旧游标在原条件、原册下照常解得开：上面两格拒的是换了条件与换了册，不是游标本身。
	selected.Set("after", stale)
	if _, err := ports.SuspensionRegistryPages.Decode(selected); err != nil {
		t.Fatalf("原条件下的游标解不开：%v", err)
	}

	unconfigured := governancehttp.NewQueryGovernanceRegistersEndpoint(
		governancehttp.UnconfiguredIntake{}, unreachableRegistryReader{t: t})
	baseline := httptest.NewRecorder()
	unconfigured.ServeHTTP(baseline, registryRequest("?register=suspension"))
	if baseline.Code != http.StatusForbidden {
		t.Fatalf("渠道未配置：%d %s", baseline.Code, baseline.Body)
	}
	for _, query := range []string{
		"?register=suspension&sort=fromAt",
		"?register=suspension&sort=-occurredAt&q=SYN",
		"?register=suspension&after=SYN-not-a-cursor",
		"?register=suspension&q=%zz",
		"?register=suspension&tenant=SYN-TENANT-02",
		"?register=takeover&authority=SYN-AUTH/successor",
		"?register=suspension&resumed=yes",
		"?register=authority-interval&open=maybe",
		"",
		"?register=",
		"?register=stage-review&q=SYN",
	} {
		response := httptest.NewRecorder()
		unconfigured.ServeHTTP(response, registryRequest(query))
		if response.Code != http.StatusForbidden || response.Body.String() != baseline.Body.String() {
			t.Fatalf("%q 渠道未配置：%d %s，want 与写对的一问同答 403 %s", query, response.Code, response.Body, baseline.Body)
		}
	}
}
