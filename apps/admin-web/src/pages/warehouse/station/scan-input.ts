// 扫描框的纯判断（纯 TypeScript，node:test 直接测）：一次键入算不算一次扫码、规范化成什么，焦点丢了要不要抢回来。
//
// 扫描枪当键盘用：打一串字加回车。没有枪就手敲加回车，粘贴也算——所以这里不区分「枪」与「手」，只看有没有回车、
// 去掉控制字符与首尾空白之后还剩不剩东西。

/**
 * 去掉控制字符，再去首尾空白。枪常在码前后夹着 STX、CR、LF、TAB 这类控制字符；码本身逐字保存（中间的空白不动，
 * 大小写不折），所以只动两头与控制字符。
 */
export function normalizeScan(raw: string): string {
  return raw.replace(/[\u0000-\u001f\u007f-\u009f]/g, '').trim();
}

export interface ScanKeyFacts {
  key: string;
  /** 输入法正在组字：回车是用来确认候选词的，不是提交。 */
  isComposing?: boolean;
}

/**
 * 这一下键入算不算一次扫码：回车、不在组字、规范化之后不是空的才算；算就交回规范化后的码，不算交回 null。
 * 空的不算——枪偶尔单独发一个回车，把它当成一次「扫到了空码」会在屏上闪出一条扫错。
 */
export function scanFromKey(event: ScanKeyFacts, raw: string): string | null {
  if (event.key !== 'Enter' || event.isComposing === true) return null;
  const code = normalizeScan(raw);
  return code === '' ? null : code;
}

export interface FocusFacts {
  /** 弹出的数量输入、确认框开着：这时焦点该在它们上面，不抢。 */
  suspended: boolean;
  /** 此刻焦点在哪个元素上；落在页面上、没有元素拿着为 null。 */
  active: { editable: boolean } | null;
  /** 刚才这次失焦是不是键盘上的 Tab 走的：人是有意走到那个按钮上的。 */
  byTab: boolean;
}

/**
 * 失焦之后要不要把焦点抢回扫描框。
 *
 * 点了按钮、点了空白处都该回来：用完一个控件就回到扫描框，下一枪才扫得进去。不抢的三种：弹层开着；焦点在别的输入或
 * 下拉里（换设备的下拉、数量输入，人正在用它，抢回去会把下拉的选项面板直接收掉）；用 Tab 走到按钮上（键盘用户要能走到
 * 按钮，否则每按一下 Tab 就被拽回来；他一动手打字，shouldRedirectKey 会把字送回扫描框）。
 */
export function shouldReclaimFocus(facts: FocusFacts): boolean {
  if (facts.suspended) return false;
  if (facts.active === null) return true;
  if (facts.active.editable) return false;
  return !facts.byTab;
}

export interface KeyFacts {
  key: string;
  ctrlKey?: boolean;
  metaKey?: boolean;
  altKey?: boolean;
}

/**
 * 焦点不在扫描框、也不在别的输入里时，来了一个键：要不要把焦点送回扫描框让这个键落进去。枪在焦点落在按钮上时扫码，
 * 第一个字符就会落空；在按键按下那一刻把焦点送回去，这个字符与后面的字符（含结尾的回车）都落进扫描框。
 * 只认可打印字符。回车与空格不认：焦点在按钮上时它们是「按下去」，键盘用户要靠它们用按钮，而一次扫码不会以它们开头；
 * Tab、方向键、Esc、带 Ctrl / Alt / ⌘ 的组合键同样不动，留给浏览器与页面自己的快捷键。
 */
export function shouldRedirectKey(event: KeyFacts, active: { editable: boolean } | null, suspended: boolean): boolean {
  if (suspended) return false;
  if (active !== null && active.editable) return false;
  if (event.ctrlKey === true || event.metaKey === true || event.altKey === true) return false;
  // 单个字符是可打印的；Shift、F5、ArrowDown 这类键名都长于一个字符
  return event.key.length === 1 && event.key !== ' ';
}
