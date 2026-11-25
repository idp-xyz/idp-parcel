// 把一张要打的单收成自包含的 HTML，交给浏览器打印。不引样式包。
// 模拟的 USPS 面单另有自己的版式，走 openPrintHtml；它不是承运商交来的面单文件。
//
// 格子里的字都先转义。单上的号、库位、商品码来自租户数据，不转义的话一个「<」就能把这页拆开。

export interface PrintSection {
  heading: string;
  lines: ReadonlyArray<{ label: string; value: string }>;
  headers: readonly string[];
  rows: ReadonlyArray<readonly string[]>;
}

export interface PrintSheet {
  title: string;
  sections: readonly PrintSection[];
}

export function escapePrint(value: string): string {
  return value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function sectionHtml(section: PrintSection): string {
  const lines = section.lines.map((line) => `<p><span>${escapePrint(line.label)}</span> ${escapePrint(line.value)}</p>`).join('');
  const head = section.headers.map((header) => `<th>${escapePrint(header)}</th>`).join('');
  const body = section.rows
    .map((row) => `<tr>${row.map((cell) => `<td>${escapePrint(cell)}</td>`).join('')}</tr>`)
    .join('');
  const table = section.headers.length === 0 ? '' : `<table><thead><tr>${head}</tr></thead><tbody>${body}</tbody></table>`;
  return `<section><h2>${escapePrint(section.heading)}</h2>${lines}${table}</section>`;
}

/** 一页能打印的 HTML。各段之间分页，最后一段不分。 */
export function printHtml(sheet: PrintSheet): string {
  return `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8"><title>${escapePrint(sheet.title)}</title><style>
body{font-family:"Source Han Sans SC","Noto Sans SC","Microsoft YaHei",sans-serif;color:#111;margin:24px}
h1{font-size:22px;margin:0 0 16px}h2{font-size:16px;margin:0 0 8px}
p{margin:0 0 4px}span{color:#555}
table{border-collapse:collapse;width:100%;margin-top:8px}th,td{border:1px solid #ccc;padding:6px 8px;text-align:left;font-size:13px}
section{break-after:page}section:last-child{break-after:auto}
</style></head><body><h1>${escapePrint(sheet.title)}</h1>${sheet.sections.map(sectionHtml).join('')}</body></html>`;
}

/** 拉起打印。 */
export function openPrint(sheet: PrintSheet): { ok: true } | { ok: false; reason: string } {
  return openPrintHtml(printHtml(sheet));
}

/**
 * 在当前页里拉起打印。
 * 新开窗口会被弹窗拦截；带 noopener 时即便开了，open 也返回空，页面分不出打没打成。
 * 点击这一下里写进同一页的框再 print，不另求弹窗许可。
 */
export function openPrintHtml(html: string): { ok: true } | { ok: false; reason: string } {
  const frame = document.createElement('iframe');
  frame.title = '打印';
  frame.style.position = 'fixed';
  frame.style.left = '-10000px';
  frame.style.top = '0';
  frame.style.width = '8in';
  frame.style.height = '11in';
  frame.style.border = '0';
  document.body.appendChild(frame);
  const page = frame.contentWindow;
  const doc = frame.contentDocument;
  if (page === null || doc === null) {
    frame.remove();
    return { ok: false, reason: '这一页打不开打印内容。' };
  }
  doc.open();
  doc.write(html);
  doc.close();
  const remove = () => frame.remove();
  page.addEventListener('afterprint', remove);
  window.setTimeout(remove, 60_000);
  page.focus();
  page.print();
  return { ok: true };
}
