package registrationjson_test

import (
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

const directNodeRow = `"tenant_id": "SYN-TENANT-01", "code": "SYN-NODE-A", "version": 1,
	"business_timezone": "Asia/Shanghai", "effective_from": "2026-09-01T00:00:00Z"`

// Covers: AT-NR-111（登记形状半边）——直接登记行的显示名称缺席或 null 即没登；给了就随这一版原样登下；不合格整行拒
// （领域构造门的错误原样上抛，不截断不改写），不落成没登。
func TestADirectNodeRowCarriesAnOptionalDisplayName(t *testing.T) {
	for label, extra := range map[string]string{"absent": "", "null": `, "display_name": null`} {
		command, err := registrationjson.NodeVersionFromJSON([]byte(`{` + directNodeRow + extra + `}`))
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if command.Node.DisplayName.Registered() {
			t.Fatalf("%s: 没给名称却登成了 %q", label, command.Node.DisplayName)
		}
	}

	command, err := registrationjson.NodeVersionFromJSON([]byte(`{` + directNodeRow + `, "display_name": "上海枢纽"}`))
	if err != nil {
		t.Fatalf("带名称：%v", err)
	}
	if command.Node.DisplayName.String() != "上海枢纽" || command.Node.Code != "SYN-NODE-A" {
		t.Fatalf("带名称 = %+v", command.Node)
	}

	for _, bad := range []string{`""`, `"  "`, `" 上海枢纽"`, `"上海\n枢纽"`, `"上海‮枢纽"`,
		`"` + strings.Repeat("枢", domain.NodeDisplayNameMaxRunes+1) + `"`} {
		if _, err := registrationjson.NodeVersionFromJSON([]byte(`{` + directNodeRow + `, "display_name": ` + bad + `}`)); !errors.Is(err, domain.ErrNodeDisplayNameMalformed) {
			t.Fatalf("名称 %s：err=%v，想要 ErrNodeDisplayNameMalformed", bad, err)
		}
	}
}

// Covers: AT-NR-111（采用半边，ADR-0353 决定四）——名称是租户取值：节点的采用行可以随行给名称，内容照旧取自参考配置；
// 参考配置不带名称，不给就没登。别的族没有名称这一格：采用行带它按另写内容拒，直接登记行带它按未知键拒。
func TestAnAdoptedNodeMayBeGivenItsDisplayNameByTheAdoptionRow(t *testing.T) {
	adopt := func(extra string) (string, error) {
		command, err := registrationjson.NodeVersionFromJSON([]byte(`{"tenant_id": "SYN-TENANT-01",
			"code": "SYN-NODE-SHA-HUB", "version": 1, "effective_from": "2026-01-01T00:00:00Z",
			"adopt": "` + demoNetwork + `"` + extra + `}`))
		if err != nil {
			return "", err
		}
		if command.Node.BusinessTimezone != "Asia/Shanghai" || !command.Node.Basis.Present() {
			t.Fatalf("采用行的内容与依据应照旧取自参考配置：%+v", command.Node)
		}
		return command.Node.DisplayName.String(), nil
	}
	if name, err := adopt(""); err != nil || name != "" {
		t.Fatalf("不给名称：name=%q err=%v，想要没登", name, err)
	}
	if name, err := adopt(`, "display_name": "上海枢纽"`); err != nil || name != "上海枢纽" {
		t.Fatalf("采用行给名称：name=%q err=%v", name, err)
	}
	if _, err := adopt(`, "display_name": ""`); !errors.Is(err, domain.ErrNodeDisplayNameMalformed) {
		t.Fatalf("采用行给空名称：err=%v，想要 ErrNodeDisplayNameMalformed", err)
	}

	if _, err := registrationjson.ConnectionVersionFromJSON([]byte(`{"tenant_id": "SYN-TENANT-01",
		"code": "SYN-CONN-SHA-SZX", "version": 1, "effective_from": "2026-01-01T00:00:00Z",
		"adopt": "` + demoNetwork + `", "display_name": "沪深干线"}`)); !errors.Is(err, registrationjson.ErrAdoptedContentGiven) {
		t.Fatalf("连接的采用行带名称：err=%v，想要 ErrAdoptedContentGiven", err)
	}
	if _, err := registrationjson.ConnectionVersionFromJSON([]byte(`{"tenant_id": "SYN-TENANT-01",
		"code": "SYN-CONN-A-B", "version": 1, "from_node": "SYN-NODE-A", "to_node": "SYN-NODE-B",
		"business_timezone": "Asia/Shanghai", "effective_from": "2026-01-01T00:00:00Z", "display_name": "沪深干线"}`)); err == nil ||
		!strings.Contains(err.Error(), "display_name") {
		t.Fatalf("连接的直接登记行带名称：err=%v，想要未知键", err)
	}
}
