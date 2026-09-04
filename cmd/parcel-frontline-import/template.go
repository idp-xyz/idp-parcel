package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// importMarkerPrefix 是全部导入来源标记的公共前缀。设备铸造的来源身份不会以它开头，
// 「这条事实的身份与时间不是设备记的」这一点因此在 source_id 上直接可辨（ADR-0023 把
// 身份与发生时间的签发权判给设备，本口是那条规则之外的过渡例外，标记就是例外的印记）。
const importMarkerPrefix = "FTI"

// 两份模板共有的列。前三列是骨架自己要核的格：版本决定认不认、批次一份文件只有一个、
// 行事实号是事实身份且批内唯一；后三列是每条导入事实都要带的记录方与业务时间。
const (
	columnTemplateVersion = "templateVersion"
	columnBatchRef        = "batchRef"
	columnFactRef         = "factRef"
	columnOperator        = "operator"
	columnEvidenceRef     = "evidenceRef"
	columnOccurredAt      = "occurredAt"
)

// templateShape 是一份汇总模板的封闭形状：版本号、列集与整体不合格的根错误。收寄与集运
// 各一份，译装骨架共用——UTF-8、BOM、封闭列集、一份文件一个批次、行事实号批内唯一这
// 几条纪律不因事实类型而异，写两遍就会有一遍先烂。
type templateShape struct {
	version string
	columns []string
	invalid error
}

// templateRecord 是一行通过骨架核验之后交给各口译装的原始格：模板行号（给内勤对表）、
// 批次号、行事实号，以及按列名取格（已去首尾空白）。
type templateRecord struct {
	line     int
	batchRef string
	factRef  string
	cell     func(column string) string
}

// decodeTemplate 跑骨架那一半：整体不合格整批拒；合格的行按模板顺序逐行交 each 译装，
// each 报错同样整批拒——部分行进了库、部分行没进，内勤对着一张表分不出哪几行是哪种。
// 返回批次号。
func decodeTemplate(shape templateShape, raw []byte, each func(templateRecord) error) (string, error) {
	if !utf8.Valid(raw) {
		return "", fmt.Errorf("%w：文件不是合法 UTF-8（Excel 请另存为「CSV UTF-8」）", shape.invalid)
	}
	// Excel 的「CSV UTF-8」会在文件头写 BOM；它不是第一列名的一部分。
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))

	reader := csv.NewReader(bytes.NewReader(raw))
	reader.TrimLeadingSpace = true
	header, err := reader.Read()
	if err != nil {
		return "", fmt.Errorf("%w：读不到表头：%v", shape.invalid, err)
	}
	index, err := templateHeaderIndex(shape, header)
	if err != nil {
		return "", err
	}

	batchRef := ""
	rows := 0
	seenFactRefs := map[string]int{}
	for line := 2; ; line++ {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("%w：第 %d 行读取失败：%v", shape.invalid, line, err)
		}
		cell := func(name string) string { return strings.TrimSpace(record[index[name]]) }

		if version := cell(columnTemplateVersion); version != shape.version {
			return "", fmt.Errorf("%w：第 %d 行 %s=%q，本口只认 %q", shape.invalid, line, columnTemplateVersion, version, shape.version)
		}
		rowBatch := cell(columnBatchRef)
		if rowBatch == "" {
			return "", fmt.Errorf("%w：第 %d 行 %s 为空", shape.invalid, line, columnBatchRef)
		}
		if batchRef == "" {
			batchRef = rowBatch
		} else if rowBatch != batchRef {
			return "", fmt.Errorf("%w：第 %d 行 %s=%q 与首行 %q 不一致——一份文件一个批次", shape.invalid, line, columnBatchRef, rowBatch, batchRef)
		}
		factRef := cell(columnFactRef)
		if factRef == "" {
			return "", fmt.Errorf("%w：第 %d 行 %s 为空", shape.invalid, line, columnFactRef)
		}
		if first, dup := seenFactRefs[factRef]; dup {
			return "", fmt.Errorf("%w：第 %d 行 %s=%q 与第 %d 行重复——行事实号是事实身份，批内不得重复", shape.invalid, line, columnFactRef, factRef, first)
		}
		seenFactRefs[factRef] = line

		if err := each(templateRecord{line: line, batchRef: rowBatch, factRef: factRef, cell: cell}); err != nil {
			return "", fmt.Errorf("%w：第 %d 行：%v", shape.invalid, line, err)
		}
		rows++
	}
	if rows == 0 {
		return "", fmt.Errorf("%w：只有表头没有数据行", shape.invalid)
	}
	return batchRef, nil
}

// templateHeaderIndex 核表头：列集必须恰好等于封闭列集，顺序不限。列名即接口：多一列拒、
// 少一列拒、重复拒——打错的列名静默丢弃会让内勤以为登进去的比实际多。
func templateHeaderIndex(shape templateShape, header []string) (map[string]int, error) {
	index := map[string]int{}
	for position, name := range header {
		name = strings.TrimSpace(name)
		if _, known := index[name]; known {
			return nil, fmt.Errorf("%w：表头列 %q 重复", shape.invalid, name)
		}
		index[name] = position
	}
	var missing, unknown []string
	expected := map[string]bool{}
	for _, name := range shape.columns {
		expected[name] = true
		if _, present := index[name]; !present {
			missing = append(missing, name)
		}
	}
	for name := range index {
		if !expected[name] {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	if len(missing) > 0 || len(unknown) > 0 {
		return nil, fmt.Errorf("%w：表头与模板 %s 不符（缺列 %v，多列 %v）", shape.invalid, shape.version, missing, unknown)
	}
	return index, nil
}

// parseOccurredAt 解析业务发生时间。内勤抄自纸单的现场时刻，带时区；CLI 不用当前时刻
// 顶替——记录时刻另有一格，两者若同源，导入的事实就再也说不出现场什么时候发生的。
func parseOccurredAt(cell func(string) string) (time.Time, error) {
	occurredAt, err := time.Parse(time.RFC3339, cell(columnOccurredAt))
	if err != nil {
		return time.Time{}, fmt.Errorf("%s=%q 不是 RFC3339 时刻（例 2026-09-08T09:15:00+08:00）", columnOccurredAt, cell(columnOccurredAt))
	}
	return occurredAt, nil
}

// importMarker 拼一条导入来源标记：FTI/<模板版本>/<各段>。模板版本进标记，因此换模板列
// 就必须换号——同一个号下两种列形状会让落库的标记说谎。
func importMarker(version string, parts ...string) string {
	return strings.Join(append([]string{importMarkerPrefix, version}, parts...), "/")
}
