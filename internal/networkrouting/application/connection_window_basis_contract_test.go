package application_test

import (
	"strconv"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件守 NR 对 TF 的两条契约（ADR-0201 决定二；复核记录 R4，票 demo-chain-gaps/142）：
//
//   - 计划段（节点间段）的窗口依据里恰有一格 `CONNECTION/<代码>@<版本>`，指名这一段所走的那一版连接；
//   - 同一连接代码跨版本只改起讫，含义变就换代码。
//
// TF 的运输准备靠第一条从计划段认连接：按 `+` 切格、认 `CONNECTION/` 前缀、在最后一个 `@` 处切掉版本（TF 侧的
// `connectionOf`，在 internal/transportfulfillment/adapters/networkrouting），只留不带版本的代码，再与计划段起讫一起配
// 班次方向。这里不导入 TF，按同一读法在 NR 自己的产出上读一遍：NR 改了拼写，先红的是这里，而不是 TF 每封信封上的
// 响亮失败（ADR-0201 越权风险点 25）。
//
// 班次方向不带版本，出发门禁的读数也按它相认，所以要靠第二条：代码不变而含义变了，按代码相认的班次就悄悄配给了另一种
// 连接（ADR-0201 越权风险点 6）。第二条只有一半落在 NR 的产出上：同一代码的新一版，计划段上那一格只有版本号跟着走、
// 代码一字不变，段的起讫取当版的起讫，TF 的起讫比对因此看得见起讫的变化。另一半「含义变就换代码」是登记方的纪律：
// 目录的连接行除起讫外没有承载含义的格，NR 的产出与登记口都判不出含义变没变，这里守不了。
//
// 产出点是目录证据视图折出的候选路径：初始路由与复核改路都原样取被选候选的 PlanLegs 成计划段链，计划段的窗口依据
// 只在这里拼。

// connectionContractCatalog 是一条两段线路 A → B → C：两条连接各有一版，一条故意在代码里带 `@` 与 `/`——TF 按最后一个
// `@` 切版本、按前缀认格，这种代码也得读回原样。A → B 那条另登衔接缓冲，候选的投影依据因此多出一格 CONNECTION，计划段
// 不该跟着多。末个节点登了派送时长，路径接一段交付段。
func connectionContractCatalog(t *testing.T, connections ...ports.ConnectionDefinitionVersion) *catalogReadDouble {
	t.Helper()
	processing := 60
	delivery := 180
	buffer := 30
	nodeCalendar := func(code string) ports.ServiceCalendarDefinitionVersion {
		return ports.ServiceCalendarDefinitionVersion{
			TargetKind: ports.TargetNode, TargetCode: code, Version: 1, EffectiveFrom: catalogEffectiveFrom,
			ProcessingMinutes: &processing, DeliveryMinutes: &delivery,
		}
	}
	return &catalogReadDouble{configured: true, snapshot: ports.NetworkCatalogSnapshot{
		Nodes: []ports.NodeDefinitionVersion{
			{Code: "SYN-NODE-A", Version: 1, BusinessTimezone: "Asia/Shanghai", EffectiveFrom: catalogEffectiveFrom},
			{Code: "SYN-NODE-B", Version: 1, BusinessTimezone: "Asia/Shanghai", EffectiveFrom: catalogEffectiveFrom},
			{Code: "SYN-NODE-C", Version: 1, BusinessTimezone: "Asia/Singapore", EffectiveFrom: catalogEffectiveFrom},
			{Code: "SYN-NODE-C2", Version: 1, BusinessTimezone: "Asia/Singapore", EffectiveFrom: catalogEffectiveFrom},
		},
		Connections: connections,
		Lines: []ports.LineDefinitionVersion{
			{Code: "SYN-LINE-A-C", Version: 1, Segments: []string{"SYN-CONN-A-B", "SYN@CONN/B-C"},
				BusinessTimezone: "Asia/Shanghai", ApplicableScope: catalogPurpose, EffectiveFrom: catalogEffectiveFrom},
		},
		ServiceAreas: []ports.ServiceAreaDefinitionVersion{
			{Code: "SYN-AREA-XA", Version: 1, EffectiveFrom: catalogEffectiveFrom,
				HasCoverage: true, CoverageCountry: "CN", OriginNodes: []string{"SYN-NODE-A"}},
			{Code: "SYN-AREA-XB-10", Version: 1, EffectiveFrom: catalogEffectiveFrom,
				HasCoverage: true, CoverageCountry: "SG", PostalPrefixes: []string{"10"},
				DestinationNodes: []string{"SYN-NODE-C", "SYN-NODE-C2"}},
		},
		Strategies: []ports.RouteStrategyDefinitionVersion{
			{Code: "SYN-STRATEGY-1", Version: 1, ApplicableScope: catalogPurpose, EffectiveFrom: catalogEffectiveFrom},
		},
		Calendars: []ports.ServiceCalendarDefinitionVersion{
			nodeCalendar("SYN-NODE-A"), nodeCalendar("SYN-NODE-B"), nodeCalendar("SYN-NODE-C"), nodeCalendar("SYN-NODE-C2"),
			{TargetKind: ports.TargetConnection, TargetCode: "SYN-CONN-A-B", Version: 2, EffectiveFrom: catalogEffectiveFrom,
				BufferMinutes: &buffer},
		},
		Revision: value(t, domain.NewNetworkViewRevision, "7"),
	}}
}

func contractConnection(code string, version int32, from, to string) ports.ConnectionDefinitionVersion {
	return ports.ConnectionDefinitionVersion{
		Code: code, Version: version, FromNode: from, ToNode: to,
		BusinessTimezone: "Asia/Shanghai", EffectiveFrom: catalogEffectiveFrom,
	}
}

// pathOverCatalog 经真目录证据视图折出唯一那条候选路径。
func pathOverCatalog(t *testing.T, catalog *catalogReadDouble) ports.CandidatePath {
	t.Helper()
	view, err := application.NewCatalogInitialRouteEvidence(catalog, satisfiedCustoms(), fixedClock{at: judgedAt})
	if err != nil {
		t.Fatalf("构造目录证据视图：%v", err)
	}
	evidence, configured, err := view.LoadInitialRouteEvidence(t.Context(), catalogInitialRouteKey(t),
		carriedGeo("CN", "", "SG", "100200"))
	if err != nil || !configured {
		t.Fatalf("configured=%v err=%v", configured, err)
	}
	if len(evidence.Paths) != 1 {
		t.Fatalf("候选路径 = %d 条，想要恰一条", len(evidence.Paths))
	}
	return evidence.Paths[0]
}

// connectionCells 按 TF 的读法从窗口依据里挑出指名连接的各格。
func connectionCells(basis domain.WindowBasisReference) []string {
	var cells []string
	for _, cell := range strings.Split(basis.String(), "+") {
		if strings.HasPrefix(cell, "CONNECTION/") {
			cells = append(cells, cell)
		}
	}
	return cells
}

// tfReading 按 TF 的读法把一格 `CONNECTION/<代码>@<版本>` 切成代码与版本：版本在最后一个 `@` 之后。
func tfReading(t *testing.T, cell string) (string, string) {
	t.Helper()
	code, version, read := tfRead(cell)
	if !read {
		t.Fatalf("%q 不是带版本的连接拼写", cell)
	}
	return code, version
}

// tfRead 是 tfReading 不失败的那一半：读不出带版本的连接就答 false。
func tfRead(cell string) (string, string, bool) {
	reference := strings.TrimPrefix(cell, "CONNECTION/")
	cut := strings.LastIndex(reference, "@")
	if cut <= 0 {
		return "", "", false
	}
	return reference[:cut], reference[cut+1:], true
}

// Covers: R4 第一条——每段节点间段的窗口依据恰有一格 `CONNECTION/<代码>@<版本>`，指名该段走的那一版连接，按 TF 的读法读回
// 原代码与版本；段的起讫就是那一版连接的起讫。连接的衔接缓冲只进候选的投影依据，不给计划段多添一格；交付段的依据指
// 节点日历、一格连接都不指（ADR-0217）。
func TestEachLegBetweenNodesNamesExactlyOneVersionedConnection(t *testing.T) {
	connections := []ports.ConnectionDefinitionVersion{
		contractConnection("SYN-CONN-A-B", 3, "SYN-NODE-A", "SYN-NODE-B"),
		contractConnection("SYN@CONN/B-C", 1, "SYN-NODE-B", "SYN-NODE-C"),
	}
	path := pathOverCatalog(t, connectionContractCatalog(t, connections...))

	if len(path.Legs) != len(connections) {
		t.Fatalf("节点间段 = %d，想要每条连接一段", len(path.Legs))
	}
	for index, leg := range path.Legs {
		connection := connections[index]
		cells := connectionCells(leg.Window().Basis())
		if len(cells) != 1 {
			t.Fatalf("第 %d 段窗口依据 %q 指了 %d 格连接，契约是恰一格", index+1, leg.Window().Basis(), len(cells))
		}
		if want := "CONNECTION/" + connection.Code + "@" + strconv.Itoa(int(connection.Version)); cells[0] != want {
			t.Fatalf("第 %d 段连接格 = %q，want %q", index+1, cells[0], want)
		}
		code, version := tfReading(t, cells[0])
		if code != connection.Code || version != strconv.Itoa(int(connection.Version)) {
			t.Fatalf("第 %d 段按 TF 读法读回 %q@%q，want %q@%d", index+1, code, version, connection.Code, connection.Version)
		}
		if leg.Kind() != domain.BetweenNodesLeg ||
			leg.From().String() != connection.FromNode || leg.To().String() != connection.ToNode {
			t.Fatalf("第 %d 段 = %s %s → %s，want 节点间段 %s → %s", index+1, leg.Kind(), leg.From(), leg.To(),
				connection.FromNode, connection.ToNode)
		}
	}

	if !path.HasDelivery {
		t.Fatal("末个节点登了派送时长、收件地有交付范围，路径却没接交付段")
	}
	if cells := connectionCells(path.Delivery.Window().Basis()); len(cells) != 0 {
		t.Fatalf("交付段依据 %q 指了连接 %v", path.Delivery.Window().Basis(), cells)
	}
}

// Covers: R4 第二条落在 NR 产出上的那一半——同一连接代码出了新一版，计划段上那一格只有版本号跟着走，代码一字不变；
// 段的起讫取当版的起讫，于是 TF 按（代码，起点，终点）配班次时，起讫变了的那一种配不上、看得见。
func TestANewVersionOfAConnectionKeepsItsCodeAndMovesOnlyTheVersionAndEnds(t *testing.T) {
	before := pathOverCatalog(t, connectionContractCatalog(t,
		contractConnection("SYN-CONN-A-B", 3, "SYN-NODE-A", "SYN-NODE-B"),
		contractConnection("SYN@CONN/B-C", 1, "SYN-NODE-B", "SYN-NODE-C"),
	))
	after := pathOverCatalog(t, connectionContractCatalog(t,
		contractConnection("SYN-CONN-A-B", 4, "SYN-NODE-A", "SYN-NODE-B"),
		contractConnection("SYN@CONN/B-C", 2, "SYN-NODE-B", "SYN-NODE-C2"),
	))

	wantVersions := [][2]string{{"3", "4"}, {"1", "2"}}
	for index := range before.Legs {
		beforeCode, beforeVersion := tfReading(t, connectionCells(before.Legs[index].Window().Basis())[0])
		afterCode, afterVersion := tfReading(t, connectionCells(after.Legs[index].Window().Basis())[0])
		if beforeCode != afterCode {
			t.Fatalf("第 %d 段同一条连接换版后代码 %q → %q，TF 按代码相认的班次会全部失配", index+1, beforeCode, afterCode)
		}
		if beforeVersion != wantVersions[index][0] || afterVersion != wantVersions[index][1] {
			t.Fatalf("第 %d 段版本 %s → %s，want %s → %s", index+1, beforeVersion, afterVersion,
				wantVersions[index][0], wantVersions[index][1])
		}
	}
	if to := after.Legs[1].To().String(); to != "SYN-NODE-C2" {
		t.Fatalf("换版改了终点，计划段终点 = %s，want 当版的 SYN-NODE-C2", to)
	}
	if from := after.Legs[1].From().String(); from != before.Legs[1].From().String() {
		t.Fatalf("起点没改却变了：%s → %s", before.Legs[1].From(), from)
	}
}

// Covers: AT-NR-094——登记口拒的连接代码，恰是第一条契约读不回的那些（票 demo-chain-gaps/176）。逐个代码先经真证据视图
// 折成计划段、按 TF 的读法读一遍，再交登记用例：读不回原代码的（带 `+`，不论在头、在尾、在中间，后半截拼成另一格
// `CONNECTION/`，还是前半截自带版本、被读成另一条连接）受理门指名拒、写入口零调用；读得回的（带 `@`、`/`，或本身以 `CONNECTION/` 开头）照常登记、原样到达
// 写入口。两侧对着同一份读法放在一条用例里：受理门多拒一种或少拒一种，这里先红；读法与预期不符（例如 TF 那一侧换了
// 切法、这里跟着改了读法），同样先红。
func TestTheRegistrationGateRefusesExactlyTheConnectionCodesTheContractCannotReadBack(t *testing.T) {
	cases := []struct {
		code     string
		readable bool
	}{
		{"SYN-CONN-B-C", true},
		{"SYN@CONN/B-C", true},
		{"SYN-CONN-B-C@", true},
		{"CONNECTION/SYN-B-C", true},
		{"SYN-CONN-B+C", false},
		{"+SYN-CONN-B-C", false},
		{"SYN-CONN-B-C+", false},
		{"SYN-B+CONNECTION/C", false},
		// 不报错，读成另一条连接 `SYN-CONN-A`：TF 那一侧是静默答错，比读不回更该挡。
		{"SYN-CONN-A@1+B", false},
	}
	for _, test := range cases {
		t.Run(test.code, func(t *testing.T) {
			catalog := connectionContractCatalog(t,
				contractConnection("SYN-CONN-A-B", 1, "SYN-NODE-A", "SYN-NODE-B"),
				contractConnection(test.code, 1, "SYN-NODE-B", "SYN-NODE-C"),
			)
			catalog.snapshot.Lines[0].Segments[1] = test.code
			leg := pathOverCatalog(t, catalog).Legs[1]
			cells := connectionCells(leg.Window().Basis())
			readable := false
			if len(cells) == 1 {
				code, version, read := tfRead(cells[0])
				readable = read && code == test.code && version == "1"
			}
			if readable != test.readable {
				t.Fatalf("窗口依据 %q 按 TF 读法读回原代码 = %v，预期 %v", leg.Window().Basis(), readable, test.readable)
			}

			double := &catalogRegistryDouble{}
			row := validConnection()
			row.Code = test.code
			result, err := newCatalogRegistration(t, double).RegisterConnectionVersion(t.Context(),
				application.RegisterConnectionVersionCommand{TenantID: catalogTenant(t), Connection: row})
			if err != nil {
				t.Fatalf("受理门的答案不是错误：%v", err)
			}
			if test.readable {
				if result.Outcome() != application.CatalogRegistered || double.calls != 1 || double.connection.Code != test.code {
					t.Fatalf("读得回的代码 %q：outcome=%s refusal=%s calls=%d 到达=%q，想要原样登记",
						test.code, result.Outcome(), result.RefusalReason(), double.calls, double.connection.Code)
				}
				return
			}
			if result.Outcome() != application.CatalogRegistrationRefused ||
				result.RefusalReason() != application.CatalogConnectionCodeCarriesBasisSeparator || double.calls != 0 {
				t.Fatalf("读不回的代码 %q：outcome=%s refusal=%s calls=%d，想要 REFUSED(%s) 且写入口零调用",
					test.code, result.Outcome(), result.RefusalReason(), double.calls,
					application.CatalogConnectionCodeCarriesBasisSeparator)
			}
		})
	}
}
