package domain

import "testing"

// 本文件钉住 CONTEXT 新增的第四个等待态`等待运营登记`在领域层的落点（ADR-0094 Decision 二）。
//
// 它与`等待内部续办`分开的理由不在代码里而在 CONTEXT：查询与重试永远产不出一次登记——权威
// 并不是答不出，是根本没有可答的东西。压成一格会对着一个从未登记过的参数无休止重试。

func TestOperatorRegistrationIsAFourthResumePath(t *testing.T) {
	if !ResumeByOperatorRegistration.valid() {
		t.Fatalf("`等待运营登记`应当是合法的续办路径")
	}
	if got := ResumeByOperatorRegistration.String(); got != "OPERATOR_REGISTRATION" {
		t.Fatalf("续办路径名应为 OPERATOR_REGISTRATION，实际 %q", got)
	}
}

// TestEveryResumePathIsDistinct 守的是「各等待态互不相同」这条 CONTEXT 硬句在代码里的形状：
// 名字撞车会让两条续办路径共用同一份处理尝试记录，而它们要催的人不同。
func TestEveryResumePathIsDistinct(t *testing.T) {
	paths := []ResumePath{
		ResumeByCustomerSupplement,
		ResumeByInternalRetry,
		ResumeByManualReview,
		ResumeByOperatorRegistration,
		ResumeByAuthorizedDisposition,
	}

	seen := make(map[string]ResumePath, len(paths))
	for _, path := range paths {
		if !path.valid() {
			t.Fatalf("路径 %d 未通过 valid()", path)
		}
		name := path.String()
		if name == "" {
			t.Fatalf("路径 %d 没有名字——漏补 String() 会让处理尝试记不进库", path)
		}
		if previous, duplicated := seen[name]; duplicated {
			t.Fatalf("路径 %d 与 %d 共用名字 %q", path, previous, name)
		}
		seen[name] = path
	}
}

// TestInvalidResumePathStaysOutside 钉住上下界：零值与上界外仍须被拒，否则一条没设过续办路径
// 的处理尝试会被当成合法的。
func TestInvalidResumePathStaysOutside(t *testing.T) {
	if ResumePathInvalid.valid() {
		t.Fatalf("零值不得合法")
	}
	if (ResumeByAuthorizedDisposition + 1).valid() {
		t.Fatalf("上界之外不得合法")
	}
	if got := ResumePathInvalid.String(); got != "" {
		t.Fatalf("零值不该有名字，实际 %q", got)
	}
}
