# 命令面板的提示是英文，还写着可以搜对象编号

Category: bug
Status: ready-for-agent

出自第 1 站 1.7。验收卡见 [stations/01-entry.md](../stations/01-entry.md)。

## 看到的

工作台顶栏「搜索或跳转…」打开的面板，输入「价格表」能滤出「打开 价格表」并跳到 `#/price-card-catalog`。输入框的占位是 `Type a command or search object ID...`。页签列表按钮的可访问名称是 `Open tabs list`，鼠标界面上看不到这串字。

## 要改成

占位用中文，只说按页面名称跳转。今天没有跨对象搜索，提示里不写搜对象编号。`Open tabs list` 换成中文可访问名称。

## 不做

不在这一票接真的对象搜索。
