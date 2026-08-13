// Package migrations 拥有 Parcel 自有的不可变业务迁移资产。
//
// 它与框架迁移分开：框架的模板不可编辑、只能按 ID 渲染，而这里的 SQL 由 Parcel
// 自己写、自己算校验和。放在仓库根而不是塞进某个上下文包，是因为迁移作业要一次
// 施加全部上下文的业务迁移，而 `//go:embed` 取不到包目录之外的文件。
package migrations

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

// 嵌入清单只列**已随提交落库的** SQL 目录：引用一个尚未提交 SQL 的目录会让干净
// 检出编译不过（`no matching files found`）——本行已两度因跨会话卷带断过远端构建，
// 新模块的目录、本行与模块函数必须同一笔提交一起落。
//
//go:embed all:parcel_shipment all:network_routing all:node_operations all:visibility_exception all:customs_compliance all:settlement_accounting
var assets embed.FS

// Asset 是一份业务迁移。SQL 只读，校验和对文件原始内容计算——一份已施加的迁移
// 事后被改写，会在下一次运行时以校验和不一致暴露，而不是悄悄生效。
type Asset struct {
	// Module 是拥有该迁移的限界上下文目录名，同时也是目标 schema。
	Module   string
	Name     string
	SQL      string
	Checksum string
}

// ParcelShipment 返回 parcel-shipment 的业务迁移，按文件名序排列。
func ParcelShipment() ([]Asset, error) {
	return assetsForModule("parcel_shipment")
}

// NetworkRouting 返回 network-routing 的业务迁移，按文件名序排列。
func NetworkRouting() ([]Asset, error) {
	return assetsForModule("network_routing")
}

// NodeOperations 返回 node-operations 的业务迁移，按文件名序排列。
func NodeOperations() ([]Asset, error) {
	return assetsForModule("node_operations")
}

// VisibilityException 返回 visibility-exception 的业务迁移，按文件名序排列。
func VisibilityException() ([]Asset, error) {
	return assetsForModule("visibility_exception")
}

// SettlementAccounting 返回 settlement-accounting 的业务迁移，按文件名序排列。
func SettlementAccounting() ([]Asset, error) {
	return assetsForModule("settlement_accounting")
}

// CustomsCompliance 返回 customs-compliance 的业务迁移，按文件名序排列。
func CustomsCompliance() ([]Asset, error) {
	return assetsForModule("customs_compliance")
}

func assetsForModule(module string) ([]Asset, error) {
	entries, err := fs.ReadDir(assets, module)
	if err != nil {
		return nil, fmt.Errorf("migrations: 读取 %s 的迁移目录：%w", module, err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		if err := checkSequence(entry.Name()); err != nil {
			return nil, fmt.Errorf("migrations: %s/%s：%w", module, entry.Name(), err)
		}
		names = append(names, entry.Name())
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("migrations: %s 下没有迁移文件；空计划会让 schema 检查空过", module)
	}
	// 按名排序即按序号排序，因为序号是零填充的定长前缀。
	sort.Strings(names)

	result := make([]Asset, 0, len(names))
	for _, name := range names {
		content, err := fs.ReadFile(assets, path.Join(module, name))
		if err != nil {
			return nil, fmt.Errorf("migrations: 读取 %s/%s：%w", module, name, err)
		}
		if strings.TrimSpace(string(content)) == "" {
			return nil, fmt.Errorf("migrations: %s/%s 没有 SQL", module, name)
		}
		result = append(result, Asset{
			Module:   module,
			Name:     strings.TrimSuffix(name, ".sql"),
			SQL:      string(content),
			Checksum: checksumOf(content),
		})
	}
	return result, nil
}

// checkSequence 要求文件名以零填充序号开头。顺序即施加顺序，靠文件名而不是靠
// 目录遍历次序——后者在不同文件系统上不保证一致。
func checkSequence(name string) error {
	prefix, _, found := strings.Cut(name, "_")
	if !found {
		return fmt.Errorf("文件名须以零填充序号开头")
	}
	sequence, err := strconv.Atoi(prefix)
	if err != nil || sequence <= 0 {
		return fmt.Errorf("文件名须以零填充序号开头")
	}
	return nil
}

func checksumOf(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}
