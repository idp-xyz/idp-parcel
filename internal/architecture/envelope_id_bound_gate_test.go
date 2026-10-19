package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"sort"
	"testing"
)

// 信封 ID 长度门禁：构造 `eventing.Envelope` 并给出 `ID` 的生产文件，必须经 `outboxintent.FingerprintEventID` 铸 ID。
//
// 信封 ID 有上限（eventing.MaxEventIDLength），而租户、委托、包裹这类引用多长归实例半边，本仓给不出上界。原样串接
// 时，短合成值在用例里一律过得去，真实长度的标识一来就被 Envelope.Validate 确定性拒收；交接失败又只化成一条续办
// 引用，重发同一份照样被拒，下游永远等不到信封（票 routing-first-cut/22 实测过初始路由那一口）。规则写在
// FingerprintEventID 的头注里；靠人记不住，所以立成门禁。
//
// 判据按文件，不按信封：一个文件里只要有一处用了指纹就算过。同一文件里一封走指纹、另一封原样串接，本门禁看不出来，
// 那一格靠各口的用例守。

// allowedRawEnvelopeID 是不经指纹铸 ID 的文件，值写理由或去处。只许变短。
//
// 「单一产品铸造标识」那几行的 ID 只取一个由本产品身份工厂铸出的定长标识，长度不随任何实例取值涨；理由里点名工厂，
// 一眼可核。工厂一旦改成收外部来的号，这一行就不再成立。
var allowedRawEnvelopeID = map[string]string{
	"internal/parcelpricing/adapters/postgres/evaluation_handoff.go":                             "单一产品铸造标识：identity.EvaluationIdentities",
	"internal/parcelshipment/adapters/postgres/acceptance_decision_handoff.go":                   "单一产品铸造标识：identity.AcceptanceDecisions",
	"internal/parcelshipment/adapters/postgres/source_data_handoff.go":                           "单一产品铸造标识：identity.SourceDataVersions",
	"internal/visibilityexception/adapters/postgres/customer_view_handoff.go":                    "单一产品铸造标识：identity.CustomerViewVersions",
	"internal/visibilityexception/adapters/postgres/disposition_handoff.go":                      "单一产品铸造标识：identity.DispositionRequests",
	"internal/visibilityexception/adapters/postgres/eta_handoff.go":                              "单一产品铸造标识：identity.ETAVersions",
	"internal/visibilityexception/adapters/postgres/notification_handoff.go":                     "单一产品铸造标识：identity.Notifications",
	"internal/visibilityexception/adapters/postgres/projection_handoff.go":                       "单一产品铸造标识：identity.ProjectionVersions",
	"internal/transportfulfillment/adapters/postgres/capacity_consumption_handoff.go":            "envelope-id-bounds/01",
	"internal/transportfulfillment/adapters/postgres/carrier_first_effective_pickup_handoff.go":  "envelope-id-bounds/01",
	"internal/transportfulfillment/adapters/postgres/disposition_execution_handoff.go":           "envelope-id-bounds/01",
	"internal/transportfulfillment/adapters/postgres/effective_delivery_handoff.go":              "envelope-id-bounds/01",
	"internal/transportfulfillment/adapters/postgres/exception_journey_handoff.go":               "envelope-id-bounds/01",
	"internal/transportfulfillment/adapters/postgres/external_tracking_fact_handoff.go":          "envelope-id-bounds/01",
	"internal/transportfulfillment/adapters/postgres/offsite_pickup_handoff.go":                  "envelope-id-bounds/01",
	"internal/transportfulfillment/adapters/postgres/offsite_pickup_registration_handoff.go":     "envelope-id-bounds/01",
	"internal/transportfulfillment/adapters/postgres/regulatory_acceptance_handoff.go":           "envelope-id-bounds/01",
	"internal/transportfulfillment/adapters/postgres/transport_commission_handoff.go":            "envelope-id-bounds/01",
	"internal/transportfulfillment/adapters/postgres/transport_handover_registration_handoff.go": "envelope-id-bounds/01",
}

// constructsEnvelopeWithID 报告文件里有没有带 ID 的 eventing.Envelope 字面量。
func constructsEnvelopeWithID(syntax *ast.File) bool {
	found := false
	ast.Inspect(syntax, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok || found {
			return !found
		}
		if literal.Type == nil || types.ExprString(literal.Type) != envelopeType {
			return true
		}
		if _, has := envelopeField(literal, "ID"); has {
			found = true
		}
		return !found
	})
	return found
}

// mintsFingerprintEventID 报告文件里有没有调用 outboxintent.FingerprintEventID。
func mintsFingerprintEventID(syntax *ast.File) bool {
	found := false
	ast.Inspect(syntax, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if ok && types.ExprString(call.Fun) == "outboxintent.FingerprintEventID" {
			found = true
		}
		return !found
	})
	return found
}

func TestEveryEnvelopeIDIsMintedAsAFingerprint(t *testing.T) {
	var violations []string
	for _, source := range parseRepositorySources(t) {
		if constructsEnvelopeWithID(source.syntax) && !mintsFingerprintEventID(source.syntax) {
			violations = append(violations, source.path)
		}
	}
	sort.Strings(violations)

	excused := make(map[string]bool, len(allowedRawEnvelopeID))
	for _, path := range violations {
		if destination, allowed := allowedRawEnvelopeID[path]; allowed {
			excused[path] = true
			t.Logf("已知例外 %s：%s", path, destination)
			continue
		}
		t.Errorf("%s 构造了 eventing.Envelope 却没经 outboxintent.FingerprintEventID 铸 ID；原样串接的 ID 遇上真实长度的标识会越过上限", path)
	}
	for path := range allowedRawEnvelopeID {
		if !excused[path] {
			t.Errorf("例外清单里的 %s 已经不违规了，请把它从 allowedRawEnvelopeID 删掉", path)
		}
	}
}

// TestTheEnvelopeIDGateCanActuallyCatchAViolation 证这道门禁真能红。
func TestTheEnvelopeIDGateCanActuallyCatchAViolation(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   bool
	}{
		{
			name:   "原样串接的 ID",
			source: "package p\nfunc f() { _ = eventing.Envelope{ID: eventing.EventID(tenant + \"/\" + parcel)} }\n",
			want:   true,
		},
		{
			name:   "经指纹铸的 ID",
			source: "package p\nfunc f() { _ = eventing.Envelope{ID: outboxintent.FingerprintEventID(\"port\", tenant, parcel)} }\n",
			want:   false,
		},
		{
			name: "ID 来自本文件里经指纹铸 ID 的函数",
			source: "package p\nfunc id() string { return string(outboxintent.FingerprintEventID(\"port\", tenant)) }\n" +
				"func f() { _ = eventing.Envelope{ID: eventing.EventID(id())} }\n",
			want: false,
		},
		{
			name:   "不带 ID 的字面量不算",
			source: "package p\nfunc f() { _ = eventing.Envelope{Type: \"x\"} }\n",
			want:   false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			syntax, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", test.source, 0)
			if err != nil {
				t.Fatalf("解析合成源码：%v", err)
			}
			got := constructsEnvelopeWithID(syntax) && !mintsFingerprintEventID(syntax)
			if got != test.want {
				t.Fatalf("判违规 = %v，want %v", got, test.want)
			}
		})
	}
}
