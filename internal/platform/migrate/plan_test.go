package migrate_test

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	numberingpg "go.idp.xyz/idp-platform-go/numbering/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

const platformModulePath = "go.idp.xyz/idp-platform-go"

// 平台库 v0.1.0-rc.1 的两串 go.sum 校验和：在全新 GOMODCACHE 与 GOCACHE 里下载取得（票 document-numbering/02 完成
// 记录）。tag 可以被移动或重打，这两串不行——工件的身份由它们认定。
const (
	platformModuleSum = "h1:o+oytSOn+EC5/3iiiSWOdL+CzXTwUeol1Br0ERLH1VQ="
	platformGoModSum  = "h1:GNtiWs3x+t02hdq6QfuTCKebZwGrlcNiWK5QyEhnTks="
)

// TestPlatformVersionIsTheOneGoModPins 证迁移历史记下的平台库版本就是 go.mod 锁定的那一个，且没有 replace 或 go.work
// 把它换成别处、go.sum 记的是登记的那两串（ADR-0355 决定四沿用 ADR-0009 / ADR-0026 的依赖闸门）。迁移历史把
// PlatformVersion 写进已部署的库，那串一旦与实际锁定的版本不符，库就在声称一个它没用过的工件。
//
// 读文件而不用 debug.ReadBuildInfo：测试二进制上它的 Deps 为空。
func TestPlatformVersionIsTheOneGoModPins(t *testing.T) {
	root := repositoryRoot(t)

	pinned := ""
	for _, line := range readLines(t, filepath.Join(root, "go.mod")) {
		fields := strings.Fields(line)
		if strings.HasPrefix(strings.TrimSpace(line), "replace ") && strings.Contains(line, platformModulePath) {
			t.Errorf("平台库被 replace 改写来源：%s", strings.TrimSpace(line))
		}
		for index, field := range fields {
			if field == platformModulePath && index+1 < len(fields) {
				pinned = fields[index+1]
			}
		}
	}
	if pinned != migrate.PlatformVersion {
		t.Fatalf("go.mod 锁定平台库 %q，迁移计划登记的是 %q", pinned, migrate.PlatformVersion)
	}

	want := map[string]string{
		platformModulePath + " " + pinned:             platformModuleSum,
		platformModulePath + " " + pinned + "/go.mod": platformGoModSum,
	}
	for _, line := range readLines(t, filepath.Join(root, "go.sum")) {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == platformModulePath {
			key := fields[0] + " " + fields[1]
			if expected, listed := want[key]; listed {
				if fields[2] != expected {
					t.Errorf("%s 的校验和是 %s，登记的是 %s", key, fields[2], expected)
				}
				delete(want, key)
			}
		}
	}
	for key := range want {
		t.Errorf("go.sum 缺少 %s", key)
	}
	for _, name := range []string{"go.work", "go.work.sum"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			t.Errorf("仓库根存在 %s；它可以在 go.mod 之外改写平台库来源", name)
		}
	}
}

// TestPlatformStepsRenderTheLibraryAssetsVerbatim 证平台步骤就是平台库按本仓平台 schema 渲染的资产：ID 带所有者前缀、
// SQL 逐字不改、校验和是对那份 SQL 算的，并排在框架之后、业务之前。资产一字不改是平台库「只增不改」承诺在本仓这一侧
// 的另一半：本仓要是改写了它，校验和就对不上平台库交出来的那一份。
func TestPlatformStepsRenderTheLibraryAssetsVerbatim(t *testing.T) {
	assets, err := numberingpg.Migrations(migrate.SchemaPlatform)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := migrate.Plan()
	if err != nil {
		t.Fatal(err)
	}

	byID := map[string]migrate.Step{}
	lastFramework, firstPlatform, lastPlatform, firstParcel := -1, -1, -1, -1
	for index, step := range plan {
		byID[step.ID] = step
		switch step.Origin {
		case migrate.OriginFramework:
			lastFramework = index
		case migrate.OriginPlatform:
			if firstPlatform < 0 {
				firstPlatform = index
			}
			lastPlatform = index
		case migrate.OriginParcel:
			if firstParcel < 0 {
				firstParcel = index
			}
		}
	}
	if !(lastFramework < firstPlatform && lastPlatform < firstParcel) {
		t.Fatalf("平台步骤不在框架之后、业务之前：框架止于 %d，平台 %d–%d，业务起于 %d", lastFramework, firstPlatform, lastPlatform, firstParcel)
	}
	for _, asset := range assets {
		step, found := byID["platform/"+asset.ID]
		if !found {
			t.Errorf("计划里没有平台资产 %s", asset.ID)
			continue
		}
		sum := sha256.Sum256([]byte(asset.SQL))
		if step.UpSQL != asset.SQL || step.Checksum != "sha256:"+hex.EncodeToString(sum[:]) {
			t.Errorf("%s 与平台库交出的资产不是同一份", step.ID)
		}
		if step.Schema != migrate.SchemaPlatform || step.PlatformVersion != migrate.PlatformVersion || step.FrameworkVersion != "" {
			t.Errorf("%s 的 schema 或版本记错：%+v", step.ID, step)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("取不到测试文件路径")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("打开 %s：%v", path, err)
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("读 %s：%v", path, err)
	}
	return lines
}
