// Package buildinfo 暴露由构建流水线注入的不可变构建元数据。
package buildinfo

// 这几项做成变量而非常量，是为了让发布构建用 -ldflags -X 注入。
var (
	version = "dev"
	commit  = "unknown"
	builtAt = "unknown"
)

// Info 描述一个运行中二进制的来源与构建身份。
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"built_at"`
}

// Current 返回当前二进制内嵌的构建身份。
func Current() Info {
	return Info{
		Version: version,
		Commit:  commit,
		BuiltAt: builtAt,
	}
}
