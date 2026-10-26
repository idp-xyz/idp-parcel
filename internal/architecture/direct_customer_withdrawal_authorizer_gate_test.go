package architecture

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// DirectCustomerWithdrawalAuthorizer 不问任何外部权威、对任何撤回都放行（授权已在铸造首方撤回信封时判过）：它只在首方直客 1:1 的
// 撤回装配里成立。别处构造它，就等于给代理人、运营侧撤回装了一个默认放行的授权器（UC-PS-005 明禁默认任何角色有撤回权）。
// 所以全仓非测试源码里只许 cmd/parcel-api/assemble_withdrawal.go 构造它。
func TestTheDirectCustomerWithdrawalAuthorizerIsBuiltOnlyByTheFirstPartyWiring(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	const allowed = "cmd/parcel-api/assemble_withdrawal.go"
	const defining = "internal/parcelshipment/adapters/accessidentity/direct_customer_withdrawal_authorization.go"
	constructed := false
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if name := entry.Name(); name == ".git" || name == "node_modules" || name == ".scratch" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, _ := filepath.Rel(root, path)
		relative = filepath.ToSlash(relative)
		if relative == defining {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(source), "DirectCustomerWithdrawalAuthorizer{") {
			return nil
		}
		if relative != allowed {
			t.Errorf("%s 构造了 DirectCustomerWithdrawalAuthorizer；它只许首方撤回装配（%s）构造", relative, allowed)
		} else {
			constructed = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !constructed {
		t.Errorf("%s 不再构造 DirectCustomerWithdrawalAuthorizer：门禁指错了位置，或首方撤回装配换了写法", allowed)
	}
}
