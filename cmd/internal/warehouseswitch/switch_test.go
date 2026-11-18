package warehouseswitch

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// Covers: ADR-0375 决定四，开关语义照 ADR-0352 决定二——去首尾空白后恰为 `true` 才开；缺省、空、大小写不同、`1`、`yes`、
// `false` 都按关读。
func TestTheSwitchOpensOnlyOnTrue(t *testing.T) {
	cases := map[string]bool{
		"":         false,
		"true":     true,
		" true ":   true,
		"\ttrue\n": true,
		"TRUE":     false,
		"True":     false,
		"1":        false,
		"yes":      false,
		"false":    false,
	}
	for value, want := range cases {
		getenv := func(name string) string {
			if name == EnabledEnv {
				return value
			}
			return ""
		}
		if got := Enabled(getenv); got != want {
			t.Fatalf("%q 读成 %v，want %v", value, got, want)
		}
	}
}

// Covers: ADR-0375 决定四——环境变量名是运维文档与部署清单照着写的外部契约，改名要连同它们一起改。
func TestTheSwitchNameIsPinned(t *testing.T) {
	if EnabledEnv != "IDP_PARCEL_WAREHOUSE_FULFILLMENT_ENABLED" {
		t.Fatalf("环境变量名 = %s：改名要同改部署清单与运维文档", EnabledEnv)
	}
}

// Covers: ADR-0375 决定四——缺省是关：这一项一个别的变量都不认，环境里没有它时按关读。
func TestTheSwitchReadsNothingButItsOwnVariable(t *testing.T) {
	getenv := func(name string) string {
		if name != EnabledEnv {
			return "true"
		}
		return ""
	}
	if Enabled(getenv) {
		t.Fatal("别的变量为 true 而本开关没设，却读成了开")
	}
}

type loggedRecord struct {
	level string
	msg   string
	env   string
}

func recordsOf(t *testing.T, raw []byte) []loggedRecord {
	t.Helper()
	var records []loggedRecord
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("日志行不是 JSON：%q：%v", line, err)
		}
		level, _ := decoded["level"].(string)
		msg, _ := decoded["msg"].(string)
		env, _ := decoded["env"].(string)
		records = append(records, loggedRecord{level: level, msg: msg, env: env})
	}
	return records
}

// Covers: ADR-0375 决定四「各进程在启动时读一次、带启动日志」——开与关都出声，各一条 Info，带环境变量名；两条正文不同，
// 事后查日志分得出这一次启动是开是关。
func TestStartupLogSpeaksOnceWhetherOnOrOff(t *testing.T) {
	messages := map[bool]string{}
	for _, enabled := range []bool{true, false} {
		var buffer bytes.Buffer
		LogStartup(slog.New(slog.NewJSONHandler(&buffer, nil)), enabled)

		records := recordsOf(t, buffer.Bytes())
		if len(records) != 1 {
			t.Fatalf("enabled=%v 写了 %d 条日志，want 1", enabled, len(records))
		}
		record := records[0]
		if record.level != slog.LevelInfo.String() {
			t.Fatalf("enabled=%v 的日志级别 = %s，want INFO", enabled, record.level)
		}
		if record.env != EnabledEnv {
			t.Fatalf("enabled=%v 的日志没带环境变量名：env = %q", enabled, record.env)
		}
		if strings.TrimSpace(record.msg) == "" {
			t.Fatalf("enabled=%v 的日志正文为空", enabled)
		}
		messages[enabled] = record.msg
	}
	if messages[true] == messages[false] {
		t.Fatalf("开与关写了同一句正文 %q，事后分不出这次启动是哪一种", messages[true])
	}
}

// Covers: 与 docnumbering 的启动日志同一约定——logger 为 nil 即不出声，调用方不必先判空。
func TestStartupLogWithANilLoggerDoesNothing(t *testing.T) {
	LogStartup(nil, true)
	LogStartup(nil, false)
}
