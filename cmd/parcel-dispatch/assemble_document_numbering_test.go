package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// 本文件证派发进程装上了编号组件（ADR-0355 决定四、六）：读的是与 parcel-api 同一项加扰密钥配置，配错时启动即退；
// 装出的取号编排在本进程的库上、在本进程开的事务里出得了号。本进程要开立的单据（自动截单的对账单、自动立案的异常
// 案件等）由各自的实现票去接，这里不接任何一类的开立。

func envWithScrambleSecrets(dsn, path string) func(string) string {
	base := startupEnv(dsn)
	return func(name string) string {
		if name == docnumber.ScrambleSecretsFileEnv {
			return path
		}
		return base(name)
	}
}

func writeSecrets(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "document-number-secrets.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Covers: 加扰密钥那一项实例配置配了而不合规时，派发进程在开池之前带原因退出——与 parcel-api 读同一项、同一个判法。
// 不需要真实 PostgreSQL：要证的就是还没连库就停下。
func TestAssemblyFailsWhenTheScrambleSecretsAreInvalid(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	env := envWithScrambleSecrets("postgres://parcel:parcel@127.0.0.1:1/postgres?sslmode=disable", writeSecrets(t, `{"SYN-TENANT-01":"short"}`))
	beat, cleanup, err := assembleDispatcher(ctx, env, discardLogger())
	if err == nil {
		if cleanup != nil {
			cleanup()
		}
		t.Fatalf("加扰密钥不合规却装配成功了：%v", beat)
	}
	if !errors.Is(err, docnumber.ErrInvalidScrambleSecrets) {
		t.Fatalf("错误未指向加扰密钥，装配点没在开池之前读那一项：%v", err)
	}
}

// Covers: ADR-0355 决定三、四——派发进程装出的取号编排在本进程的库上出号：对账单按开立方（结算账户）交来的业务时区，
// 异常案件按租户的单据编号时区；取号在本进程开的事务里，事务外即拒。配了合规的密钥文件，整个派发器照样装得起来。
func TestDispatchAssemblesDocumentNumbering(t *testing.T) {
	dsn := migratedDatabase(t)

	beat, cleanup, err := assembleDispatcher(t.Context(),
		envWithScrambleSecrets(dsn, writeSecrets(t, `{"SYN-TENANT-01":"syn-scramble-secret-0123456789abcdef"}`)), discardLogger())
	if err != nil {
		t.Fatalf("配了合规密钥文件的派发器装配失败：%v", err)
	}
	t.Cleanup(cleanup)
	if beat == nil {
		t.Fatal("装配成功却没有交回一拍")
	}

	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	numbers, err := buildDocumentNumbers(db, dispatchSettings{})
	if err != nil {
		t.Fatalf("装编号组件：%v", err)
	}
	if _, err := numbers.Registrations.RegisterTimeZone(t.Context(), docnumber.RegisterTimeZoneCommand{
		Tenant: "SYN-TENANT-01", TimeZone: "Asia/Shanghai", EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN-TEST",
	}); err != nil {
		t.Fatalf("登单据编号时区：%v", err)
	}
	if _, err := numbers.Assigner.Assign(t.Context(), docnumber.IssueRequest{
		Tenant: "SYN-TENANT-01", Type: "EXCEPTION_CASE", IsUsed: func(context.Context, string) (bool, error) { return false, nil },
	}); !errors.Is(err, docnumber.ErrTransactionRequired) {
		t.Fatalf("事务外取号：%v", err)
	}

	var statement, exceptionCase docnumber.Assignment
	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		unused := func(context.Context, string) (bool, error) { return false, nil }
		var err error
		if statement, err = numbers.Assigner.Assign(ctx, docnumber.IssueRequest{
			Tenant: "SYN-TENANT-01", Type: "CUSTOMER_STATEMENT", IssuerTimeZone: "Asia/Singapore", IsUsed: unused,
		}); err != nil {
			return err
		}
		exceptionCase, err = numbers.Assigner.Assign(ctx, docnumber.IssueRequest{Tenant: "SYN-TENANT-01", Type: "EXCEPTION_CASE", IsUsed: unused})
		return err
	})
	if err != nil {
		t.Fatalf("在本进程的事务里取号：%v", err)
	}
	if statement.Outcome != docnumber.NumberAssigned || statement.TimeZone != "Asia/Singapore" ||
		!strings.HasPrefix(statement.Number, "STMT-") || !strings.HasSuffix(statement.Number, "-00001") {
		t.Fatalf("对账单：%+v", statement)
	}
	if exceptionCase.Outcome != docnumber.NumberAssigned || exceptionCase.TimeZone != "Asia/Shanghai" ||
		!strings.HasPrefix(exceptionCase.Number, "EX-") || !strings.HasSuffix(exceptionCase.Number, "-0001") {
		t.Fatalf("异常案件：%+v", exceptionCase)
	}
}
