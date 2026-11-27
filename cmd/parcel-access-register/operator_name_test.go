package main

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const operatorNameBatch = `{
  "tenantId": "SYN-TENANT-01",
  "recordedBy": "SYN-NAME-BATCH-01",
  "names": [
    {"issuer": "` + syntheticIssuer + `", "subject": "SYN-OPERATOR-01", "name": " 合成甲 "},
    {"issuer": "` + syntheticIssuer + `", "subject": "SYN-OPERATOR-02", "name": "合成乙"}
  ]
}`

func namesOnRegister(t *testing.T, dsn string) map[string]string {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("开池核对：%v", err)
	}
	defer pool.Close()
	rows, err := pool.Query(t.Context(), `SELECT subject, name || '|' || recorded_by FROM access_identity.operator_name ORDER BY subject`)
	if err != nil {
		t.Fatalf("读姓名：%v", err)
	}
	defer rows.Close()
	names := map[string]string{}
	for rows.Next() {
		var subject, value string
		if err := rows.Scan(&subject, &value); err != nil {
			t.Fatalf("读姓名：%v", err)
		}
		names[subject] = value
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读姓名：%v", err)
	}
	return names
}

// operator-name 在真库上给已登记的主体登姓名：落的是去掉首尾空白的姓名与批头写的「谁登的」；整批重跑照样落定、
// 行不动；不带 name 的项删掉姓名；别的租户登这个主体、给没登记的主体登，各报请人看且册上不变。回显不带姓名本身。
func TestOperatorNameBatchEndToEnd(t *testing.T) {
	dsn := freshMigratedDSN(t)
	if code, _ := runCLI(t, dsn, "operator-register", "-input", batchFile(t, operatorBatch)); code != exitLanded {
		t.Fatalf("登主体：exit = %d", code)
	}

	path := batchFile(t, operatorNameBatch)
	code, out := runCLI(t, dsn, "operator-name", "-input", path)
	if code != exitLanded || strings.Count(out, "OPERATOR_NAME_RECORDED") != 2 {
		t.Fatalf("登姓名：exit = %d, want %d 且两项都回显 OPERATOR_NAME_RECORDED", code, exitLanded)
	}
	if strings.Contains(out, "合成甲") || strings.Contains(out, "合成乙") {
		t.Fatalf("回显里带了姓名：%s", out)
	}
	want := map[string]string{"SYN-OPERATOR-01": "合成甲|SYN-NAME-BATCH-01", "SYN-OPERATOR-02": "合成乙|SYN-NAME-BATCH-01"}
	if got := namesOnRegister(t, dsn); len(got) != 2 || got["SYN-OPERATOR-01"] != want["SYN-OPERATOR-01"] || got["SYN-OPERATOR-02"] != want["SYN-OPERATOR-02"] {
		t.Fatalf("册上的姓名 = %v, want %v", got, want)
	}

	// 换一个「谁登的」重跑同一批姓名：照样落定，行不动。
	replay := strings.Replace(operatorNameBatch, "SYN-NAME-BATCH-01", "SYN-NAME-BATCH-02", 1)
	if code, _ := runCLI(t, dsn, "operator-name", "-input", batchFile(t, replay)); code != exitLanded {
		t.Fatalf("重跑：exit = %d, want %d", code, exitLanded)
	}
	if got := namesOnRegister(t, dsn); got["SYN-OPERATOR-01"] != want["SYN-OPERATOR-01"] {
		t.Fatalf("重跑之后册上的姓名 = %v, want 仍是头一次登的", got)
	}

	for _, refused := range []struct{ name, body string }{
		{"乙租户登甲的主体", `{"tenantId": "SYN-TENANT-02", "recordedBy": "SYN-NAME-BATCH-09", "names": [{"issuer": "` + syntheticIssuer + `", "subject": "SYN-OPERATOR-01", "name": "合成丙"}]}`},
		{"给没登记的主体登", `{"tenantId": "SYN-TENANT-01", "recordedBy": "SYN-NAME-BATCH-09", "names": [{"issuer": "` + syntheticIssuer + `", "subject": "SYN-OPERATOR-GHOST", "name": "合成丙"}]}`},
	} {
		code, out := runCLI(t, dsn, "operator-name", "-input", batchFile(t, refused.body))
		if code != exitAttention || !strings.Contains(out, "OPERATOR_NOT_FOUND") {
			t.Fatalf("%s：exit = %d, want %d 且回显 OPERATOR_NOT_FOUND", refused.name, code, exitAttention)
		}
	}
	if got := namesOnRegister(t, dsn); len(got) != 2 || got["SYN-OPERATOR-01"] != want["SYN-OPERATOR-01"] {
		t.Fatalf("被拒之后册上的姓名 = %v, want 不变", got)
	}

	clearing := `{"tenantId": "SYN-TENANT-01", "recordedBy": "SYN-NAME-BATCH-03", "names": [{"issuer": "` + syntheticIssuer + `", "subject": "SYN-OPERATOR-02"}]}`
	if code, out := runCLI(t, dsn, "operator-name", "-input", batchFile(t, clearing)); code != exitLanded || !strings.Contains(out, "OPERATOR_NAME_CLEARED") {
		t.Fatalf("删姓名：exit = %d, want %d 且回显 OPERATOR_NAME_CLEARED", code, exitLanded)
	}
	if got := namesOnRegister(t, dsn); len(got) != 1 || got["SYN-OPERATOR-01"] != want["SYN-OPERATOR-01"] {
		t.Fatalf("删掉之后册上的姓名 = %v, want 只剩第一位", got)
	}
}

// 姓名批的缺件与形状错在触库之前拒收：谁登的不代填，空白姓名不当成删掉。
func TestOperatorNameBatchRefusesBeforeTouchingTheDatabase(t *testing.T) {
	if _, err := operatorNameBatchFromJSON([]byte(operatorNameBatch)); err != nil {
		t.Fatalf("基准姓名批应译得出：%v", err)
	}
	item := func(rest string) string {
		return `{"tenantId": "SYN-TENANT-01", "recordedBy": "SYN-NAME-BATCH-01", "names": [{"issuer": "` + syntheticIssuer + `", "subject": "SYN-OPERATOR-01"` + rest + `}]}`
	}
	if _, err := operatorNameBatchFromJSON([]byte(item(``))); err != nil {
		t.Fatalf("不带 name 的项（删掉）应译得出：%v", err)
	}
	for label, body := range map[string]string{
		"缺谁登的":  `{"tenantId": "SYN-TENANT-01", "names": [{"issuer": "i", "subject": "s", "name": "合成甲"}]}`,
		"谁登的空白": `{"tenantId": "SYN-TENANT-01", "recordedBy": "  ", "names": [{"issuer": "i", "subject": "s", "name": "合成甲"}]}`,
		"缺租户":   `{"recordedBy": "SYN-NAME-BATCH-01", "names": [{"issuer": "i", "subject": "s", "name": "合成甲"}]}`,
		"空数组":   `{"tenantId": "SYN-TENANT-01", "recordedBy": "SYN-NAME-BATCH-01", "names": []}`,
		"缺 sub": `{"tenantId": "SYN-TENANT-01", "recordedBy": "SYN-NAME-BATCH-01", "names": [{"issuer": "i", "name": "合成甲"}]}`,
		"空白姓名":  item(`, "name": "  "`),
		"超长姓名":  item(`, "name": "` + strings.Repeat("甲", 61) + `"`),
		"带换行":   item(`, "name": "合成\n甲"`),
		"未知字段":  item(`, "name": "合成甲", "phone": "x"`),
	} {
		if _, err := operatorNameBatchFromJSON([]byte(body)); err == nil {
			t.Fatalf("%s：翻译应在触库前拒收", label)
		}
	}
}
