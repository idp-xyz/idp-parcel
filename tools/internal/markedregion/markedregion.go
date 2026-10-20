// Package markedregion 把生成物落进一份手写文件里的一对标记之间，并提供「写回」与「只核对」两种落法。
//
// 生成物与手写正文同住一份文件时，工具只许碰标记之内：标记外是人写的说明与追记，
// 工具重写它们就等于替人改了文字，而那种改动在 diff 里与正常重生成混在一起，看不出来。
package markedregion

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrStale 表示文件里标记之间的内容与这一次生成的不同。只核对模式据此非零退出。
var ErrStale = errors.New("生成物与文件不一致")

// Replace 交回把 content 里 begin 行与 end 行之间的部分换成 body 之后的全文。
//
// 两个标记都必须独占一行、各恰出现一次、begin 在前。宽松匹配（行内出现、出现多次取第一个）
// 会让一处手误把生成区悄悄挪到别处，而工具照样「成功」——那正是生成物要防的那一类无声错。
// body 非空时以换行结尾，这样 end 标记总在自己的一行上。
func Replace(content, begin, end, body string) (string, error) {
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	lines := strings.SplitAfter(content, "\n")
	beginAt, endAt := -1, -1
	for i, line := range lines {
		bare := strings.TrimRight(line, "\r\n")
		switch bare {
		case begin:
			if beginAt >= 0 {
				return "", fmt.Errorf("开始标记出现了不止一次：%s", begin)
			}
			beginAt = i
		case end:
			if endAt >= 0 {
				return "", fmt.Errorf("结束标记出现了不止一次：%s", end)
			}
			endAt = i
		}
	}
	switch {
	case beginAt < 0:
		return "", fmt.Errorf("找不到独占一行的开始标记：%s", begin)
	case endAt < 0:
		return "", fmt.Errorf("找不到独占一行的结束标记：%s", end)
	case endAt < beginAt:
		return "", fmt.Errorf("结束标记在开始标记之前：%s", end)
	}
	var b strings.Builder
	for _, line := range lines[:beginAt+1] {
		b.WriteString(line)
	}
	if !strings.HasSuffix(lines[beginAt], "\n") {
		b.WriteString("\n")
	}
	b.WriteString(body)
	for _, line := range lines[endAt:] {
		b.WriteString(line)
	}
	return b.String(), nil
}

// Sync 让 path 的内容等于 want。check 为真时只比较、不写：不同即交回包了 ErrStale 的错误。
// 内容已经相同时不写盘，免得只改了修改时间。
//
// 落盘由程序自己做而不交给 shell 重定向，理由同 tools/mechanism-inventory 的 -out：
// Windows PowerShell 的默认重定向按 ANSI 代码页写，中文一出去就坏。
func Sync(path, want string, check bool) error {
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(current) == want {
		return nil
	}
	if check {
		return fmt.Errorf("%s：%w，重跑生成器（不带 -check）后提交", path, ErrStale)
	}
	return os.WriteFile(path, []byte(want), 0o644)
}
