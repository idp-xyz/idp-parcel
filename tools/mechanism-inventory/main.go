// mechanism-inventory 清点「机制半边现状」里那些能由代码算出来的数：逐上下文的生产/测试
// 文件、应用编排、PostgreSQL 与 HTTP 适配器、Outbox 投递适配器、跨上下文消费缝、迁移份数，
// 接线面的接入面端点、消费适配器与直投路由表条目，以及端口的两口径实现缺口。
//
// 它**只清点，不定级**。「达标 / 部分 / 未开始」与「显式留待已认可」是判断与裁定，代码算不
// 出来，也不该由一个脚本的启发式覆盖——那两栏留在开发主线正文里由人维护。
//
// 本工具自成一个模块：端口清点要 golang.org/x/tools，而主模块的直接依赖只有三个且这件事本
// 身被开发主线当作「参数显式未配置」的佐证在引用。把工具依赖并进去会让那句话变假。
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	dir := flag.String("dir", ".", "仓库根")
	withPorts := flag.Bool("ports", true, "是否做端口两口径清点（需要加载全部包，较慢）")
	// 落盘由本程序自己做而不交给 shell 重定向：Windows PowerShell 的默认重定向按 ANSI 代码页
	// 写，中文报告一出去就是乱码，而乱码与「文件真写坏了」在屏幕上分不出来。
	out := flag.String("out", "", "报告落盘路径；留空写标准输出")
	flag.Parse()

	if err := run(*dir, *withPorts, *out); err != nil {
		fmt.Fprintln(os.Stderr, "mechanism-inventory:", err)
		os.Exit(1)
	}
}

func run(dir string, withPorts bool, outPath string) error {
	census, err := CensusFiles(os.DirFS(dir))
	if err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("# 机制半边清点（生成物，勿手改）\n\n")
	b.WriteString("由 `tools/mechanism-inventory` 生成。本文只有数，没有定级——")
	b.WriteString("「达标／部分／未开始」与留待裁定在开发主线正文里。\n\n")

	b.WriteString("## 逐上下文文件面\n\n")
	b.WriteString("| 上下文 | 生产 | 测试 | 应用编排 | postgres 适配器 | 其中 Outbox 投递 | http 适配器 |\n")
	b.WriteString("|---|---|---|---|---|---|---|\n")
	for _, ctx := range census.Contexts {
		name := ctx.Name
		if !ctx.Business {
			name += "（非业务）"
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d | %d |\n",
			name, ctx.Production, ctx.Test, ctx.Application, ctx.Postgres, ctx.OutboxHandoff, ctx.HTTP)
	}
	total := census.Totals()
	fmt.Fprintf(&b, "| **合计** | %d | %d | %d | %d | %d | %d |\n",
		total.Production, total.Test, total.Application, total.Postgres, total.OutboxHandoff, total.HTTP)
	fmt.Fprintf(&b, "\n业务上下文 %d 个，非业务目录 %d 个。`cmd/` 生产 %d、测试 %d。\n\n",
		len(census.BusinessContexts()), len(census.Contexts)-len(census.BusinessContexts()),
		census.CmdProd, census.CmdTest)

	groups, seamFiles := census.CrossSeamTotals()
	fmt.Fprintf(&b, "## 跨上下文消费缝：%d 组，%d 个生产文件\n\n", groups, seamFiles)
	b.WriteString("| 消费方 | 提供方 | 文件 |\n|---|---|---|\n")
	for _, seam := range census.CrossSeams {
		fmt.Fprintf(&b, "| %s | %s | %d |\n", seam.Consumer, seam.Provider, seam.Files)
	}

	migrationTotal := 0
	for _, m := range census.Migrations {
		migrationTotal += m.Files
	}
	fmt.Fprintf(&b, "\n## 迁移：%d 个模块共 %d 份 SQL\n\n", len(census.Migrations), migrationTotal)
	b.WriteString("| 模块 | 份数 |\n|---|---|\n")
	for _, m := range census.Migrations {
		fmt.Fprintf(&b, "| %s | %d |\n", m.Name, m.Files)
	}

	wiring, err := CensusWiring(os.DirFS(dir))
	if err != nil {
		return err
	}
	writeWiring(&b, wiring)

	if withPorts {
		ports, err := CensusPorts(dir)
		if err != nil {
			return err
		}
		missA, missB := ports.MissingA(), ports.MissingB()
		fmt.Fprintf(&b, "\n## 端口：声明 %d 个；基线口径缺 %d，精确口径缺 %d\n\n",
			len(ports.Ports), len(missA), len(missB))
		if len(ports.LoadErrors) > 0 {
			fmt.Fprintf(&b, "**包加载报错 %d 条，下面的数不可信**：\n\n", len(ports.LoadErrors))
			for _, e := range ports.LoadErrors {
				fmt.Fprintf(&b, "- %s\n", e)
			}
			b.WriteString("\n")
		}
		b.WriteString("基线口径缺（名字未在任何适配器/平台生产文件出现）：\n\n")
		writePortList(&b, missA, func(p PortEntry) string {
			if p.ImplementedB {
				return "（虚低：精确口径已实现，实现者 " + p.ImplementerB + "）"
			}
			return ""
		})
		b.WriteString("\n精确口径缺（无具体类型完整实现）：\n\n")
		writePortList(&b, missB, func(p PortEntry) string {
			if p.ImplementedA {
				return "（虚高：名字出现过，但无人实现）"
			}
			return ""
		})
	}

	if outPath == "" {
		_, err = os.Stdout.WriteString(b.String())
		return err
	}
	return os.WriteFile(outPath, []byte(b.String()), 0o644)
}

// writeWiring 落接线面三栏。数据源各写一句在表前：三栏数的是「装了多少」不是「写了多少文件」，
// 而这正是它们此前只能活在叙述里、并在叙述里烂掉的原因——文件数一眼数得出，装配条目要读装配点。
func writeWiring(b *strings.Builder, wiring WiringCensus) {
	fmt.Fprintf(b, "\n## 接线面：接入面端点 %d 个，消费适配器 %d 个生产文件，直投路由表 %d 条\n\n",
		wiring.EndpointTotal, wiring.ConsumerTotal, wiring.RouteTotal)

	b.WriteString("接入面端点按 `cmd/` 生产文件里 `[]httpapi.BusinessEndpoint` 字面量的条目数，")
	b.WriteString("按端点构造函数所在的 `internal/<上下文>/adapters/http` 归属；不按 `adapters/http/` 的文件数——")
	b.WriteString("一个处理器可挂多个端点。\n\n")
	b.WriteString("| 上下文 | 端点 |\n|---|---|\n")
	for _, tally := range wiring.Endpoints {
		fmt.Fprintf(b, "| %s | %d |\n", tally.Context, tally.Count)
	}
	fmt.Fprintf(b, "| **合计** | %d |\n\n", wiring.EndpointTotal)

	b.WriteString("消费适配器按 `internal/<消费方>/adapters/` 下 `")
	b.WriteString(strings.Join(consumerDirs, "`、`"))
	b.WriteString("` 四类目录的生产文件数。它与上面的「跨上下文消费缝」是两种东西：那一栏数的是消费方为某个提供方写的防腐层，这一栏数的是接进程内直投信封的消费门。\n\n")
	b.WriteString("| 消费方 | " + strings.Join(consumerDirs, " | ") + " | 合计 |\n|---|")
	b.WriteString(strings.Repeat("---|", len(consumerDirs)+1))
	b.WriteString("\n")
	for _, consumer := range wiring.Consumers {
		fmt.Fprintf(b, "| %s |", consumer.Consumer)
		for _, dir := range consumerDirs {
			fmt.Fprintf(b, " %d |", consumer.ByDir[dir])
		}
		fmt.Fprintf(b, " %d |\n", consumer.Total)
	}
	b.WriteString("| **合计** |")
	for _, total := range wiring.ConsumerDirTotals() {
		fmt.Fprintf(b, " %d |", total)
	}
	fmt.Fprintf(b, " %d |\n\n", wiring.ConsumerTotal)

	b.WriteString("直投路由表按 `cmd/` 生产文件里 `map[eventing.EventType]dispatch.Consumer` 字面量的条目数，")
	b.WriteString("按条目键（事件类型常量）所属的消费门包归属。路由表只随消费者一起长（ADR-0049 第三条），本表只报它此刻多长。\n\n")
	b.WriteString("| 事件类型所属消费方 | 条目 |\n|---|---|\n")
	for _, tally := range wiring.Routes {
		fmt.Fprintf(b, "| %s | %d |\n", tally.Context, tally.Count)
	}
	fmt.Fprintf(b, "| **合计** | %d |\n", wiring.RouteTotal)
}

func writePortList(b *strings.Builder, ports []PortEntry, note func(PortEntry) string) {
	if len(ports) == 0 {
		b.WriteString("- 无\n")
		return
	}
	for _, p := range ports {
		fmt.Fprintf(b, "- `%s.%s` %s\n", p.Context, p.Name, note(p))
	}
}
