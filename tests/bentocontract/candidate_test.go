package bentocontract

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// 本文件证 `PBC-01` 的「从正式 module path 与精确候选版本编译」。
//
// 编译这一半由 `candidate.go` 的 import 承担：那几条 import 走的是正式 module
// path，解析不到候选，本包连编译都过不去，测试根本无从运行。所以下面的用例不再
// 重复证「能不能编译」，只证编译所依据的锁是不是登记的那一个。
//
// 不用 `debug.ReadBuildInfo`：测试二进制上它的 `Deps` 为空，据此判断会得出
// 「候选不存在」的错误结论，或者反过来被人改成宽松匹配而变成永远通过的空门禁。

// TestGoModPinsTheExactCandidateWithoutReplace 证 go.mod 钉的是登记的精确版本，
// 且没有任何 `replace` 把它换成别处。`replace` 之下版本号与 module path 都还
// 好看，实际取的却是本地目录或另一份源码——`P-07` 与 `PBC-01` 都明禁。
func TestGoModPinsTheExactCandidateWithoutReplace(t *testing.T) {
	t.Parallel()

	var pinned string
	var replaced []string

	for _, line := range readLines(t, filepath.Join(repositoryRoot(t), "go.mod")) {
		trimmed := strings.TrimSpace(line)
		fields := strings.Fields(trimmed)

		if strings.HasPrefix(trimmed, "replace ") && strings.Contains(trimmed, ModulePath) {
			replaced = append(replaced, trimmed)
			continue
		}
		// require 块内的行只有 "<path> <version>" 两列，块外则是
		// "require <path> <version>"，两种形状都要认。
		for index, field := range fields {
			if field != ModulePath || index+1 >= len(fields) {
				continue
			}
			pinned = fields[index+1]
		}
	}

	if pinned == "" {
		t.Fatalf("go.mod 没有钉住 %s；候选依赖不存在", ModulePath)
	}
	if pinned != Version {
		t.Errorf("go.mod 钉的候选版本是 %q，登记的是 %q", pinned, Version)
	}
	for _, directive := range replaced {
		t.Errorf("候选被 replace 改写来源：%s", directive)
	}
}

// TestGoSumRecordsTheRegisteredChecksums 证锁到的工件就是登记的那一个。
// tag 可以被移动或重打，`go.sum` 里这两串不行——候选的不可变性由它认定。
func TestGoSumRecordsTheRegisteredChecksums(t *testing.T) {
	t.Parallel()

	expected := map[string]string{
		ModulePath + " " + Version:             ModuleSum,
		ModulePath + " " + Version + "/go.mod": GoModSum,
	}
	found := make(map[string]string, len(expected))

	for _, line := range readLines(t, filepath.Join(repositoryRoot(t), "go.sum")) {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[0] != ModulePath {
			continue
		}
		found[fields[0]+" "+fields[1]] = fields[2]
	}

	for key, want := range expected {
		got, ok := found[key]
		if !ok {
			t.Errorf("go.sum 缺少 %q 的记录；无法认定工件身份", key)
			continue
		}
		if got != want {
			t.Errorf("%q 的 checksum 是 %q，登记的是 %q；工件已不是同一个", key, got, want)
		}
	}
}

// TestNoWorkspaceFileOverridesTheCandidate 补上 go.mod 判不到的旁路：`go.work`
// 能在 go.mod 之外把 module 指向本地目录，而 go.mod 与 go.sum 看上去毫无变化。
func TestNoWorkspaceFileOverridesTheCandidate(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	for _, name := range []string{"go.work", "go.work.sum"} {
		switch _, err := os.Stat(filepath.Join(root, name)); {
		case err == nil:
			t.Errorf("仓库根存在 %s；它可以在 go.mod 之外改写候选来源", name)
		case !os.IsNotExist(err):
			t.Fatalf("检查 %s：%v", name, err)
		}
	}
}

// TestConsumersAgreeOnTheCandidateVersion 证仓内其他地方登记的框架版本与本包这份
// 候选身份是同一个。迁移历史会把框架版本写进已部署的数据库，那串一旦与实际锁定的
// 候选不符，库就在声称一个它没用过的工件。
//
// 校验方向由本包发起：候选身份归合同包所有，生产代码不反向 import 它。
func TestConsumersAgreeOnTheCandidateVersion(t *testing.T) {
	t.Parallel()

	if migrate.FrameworkVersion != Version {
		t.Errorf("迁移计划登记的框架版本为 %q，锁定的候选为 %q",
			migrate.FrameworkVersion, Version)
	}
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
		t.Fatalf("读取 %s：%v", path, err)
	}
	if len(lines) == 0 {
		t.Fatalf("%s 为空；据此判断会得到永远通过的空门禁", path)
	}
	return lines
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("取工作目录：%v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("测试工作目录之上找不到 go.mod")
		}
		directory = parent
	}
}
