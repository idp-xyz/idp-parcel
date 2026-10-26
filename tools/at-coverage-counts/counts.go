package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Bucket 是盘点里的一个分桶节：标题以带圈数字开头的二级标题，到下一个二级标题为止。
type Bucket struct {
	Label    string // 标题去掉全角括号说明之后的部分，如「① 已钉住」
	Mark     string // 带圈数字本身，作前缀表的列名
	ByPrefix map[string]int
	Total    int
}

// Census 是一次重算的全部结果，桶按在文件里出现的顺序。
type Census struct {
	Buckets  []Bucket
	Prefixes []string          // 出现过的 AT 前缀，按字母序
	Rows     map[string]string // 每个有表行的 AT → 所在桶的 Label，供与 UC 的立号对照
}

var atRow = regexp.MustCompile("^\\| `AT-([A-Z]+)-(\\d+)`")

// isCircledDigit 认 ①–⑳。分桶靠这个认，而不是把四个桶名写死在这里：桶名在文件里那一处定义，
// 这里再写一份，改名或加桶时两处就会各说各的。
func isCircledDigit(r rune) bool { return r >= '①' && r <= '⑳' }

// Count 按分桶节里的表行重算。同一个 AT 出现两次（同桶或跨桶）是错：票本该「移行」，
// 抄了一份没删就会让两张表都多算一条，而多算的那条看上去和真的进展一模一样。
func Count(content string) (Census, error) {
	var census Census
	seen := map[string]string{}
	prefixes := map[string]bool{}
	current := -1
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "## ") {
			current = -1
			title := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			first := []rune(title)
			if len(first) > 0 && isCircledDigit(first[0]) {
				label := title
				if at := strings.Index(label, "（"); at >= 0 {
					label = strings.TrimSpace(label[:at])
				}
				census.Buckets = append(census.Buckets, Bucket{Label: label, Mark: string(first[0]), ByPrefix: map[string]int{}})
				current = len(census.Buckets) - 1
			}
			continue
		}
		if current < 0 {
			continue
		}
		m := atRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		id := "AT-" + m[1] + "-" + m[2]
		bucket := &census.Buckets[current]
		if where, dup := seen[id]; dup {
			return Census{}, fmt.Errorf("%s 出现了两次：%s 与 %s", id, where, bucket.Label)
		}
		seen[id] = bucket.Label
		bucket.ByPrefix[m[1]]++
		bucket.Total++
		prefixes[m[1]] = true
	}
	if len(census.Buckets) == 0 {
		return Census{}, fmt.Errorf("没有找到以带圈数字开头的二级标题，认不出分桶")
	}
	for prefix := range prefixes {
		census.Prefixes = append(census.Prefixes, prefix)
	}
	sort.Strings(census.Prefixes)
	census.Rows = seen
	return census, nil
}

// Render 交回总览两张表：按桶的数量与合计，按前缀的总数与逐桶数。
func Render(census Census) string {
	var b strings.Builder
	b.WriteString("| 桶 | 数量 |\n|---|---|\n")
	total := 0
	for _, bucket := range census.Buckets {
		fmt.Fprintf(&b, "| %s | %d |\n", bucket.Label, bucket.Total)
		total += bucket.Total
	}
	fmt.Fprintf(&b, "| 合计 | %d |\n\n", total)

	b.WriteString("| 前缀 | 总数 |")
	for _, bucket := range census.Buckets {
		fmt.Fprintf(&b, " %s |", bucket.Mark)
	}
	b.WriteString("\n|---|---|")
	b.WriteString(strings.Repeat("---|", len(census.Buckets)))
	b.WriteString("\n")
	for _, prefix := range census.Prefixes {
		sum := 0
		var cells strings.Builder
		for _, bucket := range census.Buckets {
			n := bucket.ByPrefix[prefix]
			sum += n
			fmt.Fprintf(&cells, " %d |", n)
		}
		fmt.Fprintf(&b, "| %s | %d |%s\n", prefix, sum, cells.String())
	}
	return b.String()
}
