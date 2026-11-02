package docnumbering

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: 各取号与登记进程读同一项加扰密钥配置——不设即没配，设了而不合规交回错误；启动日志只报配没配与租户数，
// 不出现租户名与密钥。
func TestLoadScrambleSecretsReadsOneConfigurationItem(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	table, err := LoadScrambleSecrets(func(string) string { return "" }, nil, logger)
	if err != nil || table.Tenants() != 0 || !strings.Contains(logs.String(), "not configured") {
		t.Fatalf("unset: %v %d %s", err, table.Tenants(), logs.String())
	}

	logs.Reset()
	getenv := func(name string) string {
		if name == docnumber.ScrambleSecretsFileEnv {
			return "/secrets/numbers.json"
		}
		return ""
	}
	const secret = "syn-scramble-secret-0123456789abcdef"
	table, err = LoadScrambleSecrets(getenv, func(string) ([]byte, error) {
		return []byte(`{"SYN-TENANT-01":"` + secret + `"}`), nil
	}, logger)
	if err != nil || table.Tenants() != 1 {
		t.Fatalf("configured: %v %d", err, table.Tenants())
	}
	if strings.Contains(logs.String(), secret) || strings.Contains(logs.String(), "SYN-TENANT-01") || !strings.Contains(logs.String(), "tenants=1") {
		t.Fatalf("startup log leaks or misses: %s", logs.String())
	}

	if _, err := LoadScrambleSecrets(getenv, func(string) ([]byte, error) { return []byte(`{"SYN-TENANT-01":"short"}`), nil }, nil); !errors.Is(err, docnumber.ErrInvalidScrambleSecrets) {
		t.Fatalf("invalid table: %v", err)
	}
}

// Covers: ADR-0355 决定三、四、六——共享装配在真库上装出的一套：登记包在事务里（同内容重放答已存在）、读口看得见
// 登记、取号在调用方事务里出号，撞号时调用方回滚，取过的流水一并释放。
func TestBuildAssemblesOneSetOnARealDatabase(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	numbers, err := Build(db, docnumber.SecretTable{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Build(nil, docnumber.SecretTable{}); err == nil {
		t.Fatal("nil db was accepted")
	}

	command := docnumber.RegisterTimeZoneCommand{
		Tenant: "SYN-TENANT-01", TimeZone: "Asia/Shanghai", EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN-TEST",
	}
	first, err := numbers.Registrations.RegisterTimeZone(t.Context(), command)
	if err != nil || first.Outcome != docnumber.VersionRegistered {
		t.Fatalf("register: %+v %v", first, err)
	}
	if replay, err := numbers.Registrations.RegisterTimeZone(t.Context(), command); err != nil || replay.Outcome != docnumber.VersionExisting {
		t.Fatalf("replay: %+v %v", replay, err)
	}
	view, err := numbers.Reader.Read(t.Context(), "SYN-TENANT-01")
	if err != nil || view.NumberingTimeZone == nil || len(view.Types) == 0 {
		t.Fatalf("read: %+v %v", view, err)
	}

	issue := func(isUsed func(context.Context, string) (bool, error), rollBack bool) (docnumber.Assignment, error) {
		var assignment docnumber.Assignment
		rolledBack := errors.New("the issuing transaction rolls back")
		err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
			assigned, err := numbers.Assigner.Assign(ctx, docnumber.IssueRequest{Tenant: "SYN-TENANT-01", Type: "BOOKING", IsUsed: isUsed})
			assignment = assigned
			if err == nil && rollBack {
				return rolledBack
			}
			return err
		})
		if errors.Is(err, rolledBack) {
			err = nil
		}
		return assignment, err
	}
	collision, err := issue(func(context.Context, string) (bool, error) { return true, nil }, true)
	if err != nil || collision.Outcome != docnumber.NumberingRuleCollision || collision.Skipped != docnumber.MaxSkippedNumbers {
		t.Fatalf("collision: %+v %v", collision, err)
	}
	assigned, err := issue(func(context.Context, string) (bool, error) { return false, nil }, false)
	if err != nil || assigned.Outcome != docnumber.NumberAssigned || !strings.HasSuffix(assigned.Number, "-00001") {
		t.Fatalf("after the rolled-back collision the first value should be free again: %+v %v", assigned, err)
	}
}
