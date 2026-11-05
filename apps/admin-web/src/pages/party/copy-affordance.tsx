import { useEffect, useState, type MouseEvent, type ReactNode } from 'react';
import { Copy } from 'lucide-react';
import { COPIED_NOTE, codeRefTitle } from '../../components/CodeRef';

// 公司主体页的复制规则（票 company-subject-ux/14）分三类：号码类整个值是按钮、点击即复制（RefText）；文本类文字照常可选，
// 值末尾悬停出一枚小图标、点图标才整段复制（CopyAside）；标签、状态、日期、说明、计数不复制。
//
// 两件只引 react 与 lucide，不引 @idpxyz/*：那两包只给 import 形的 exports，编成 CommonJS 的 node:test require 不到
// （理由同 components/CodeRef.tsx 文件头），而反馈的话与时序要在 node:test 里钉。detail-primitives.tsx 把两件原样转出，
// 页面仍从那里引；别的页要用时再上提到 components/。
//
// 反馈在件内行内给、不走 toast：一页上号码几十个，连着复制几格时 toast 会叠成一摞，行内那两个字就在手边。
// 复制永远不悄悄发生：成功、被拒、上下文没有剪贴板（非 https）三种结局各有一句，没有反馈的复制等于没复制。
// 「已复制」与「点击复制」两个词取 CodeRef 那一份，全站一处定义。

/** 行内反馈停留的时长。两秒是票面定的：扫一眼看得到，又不挡着连着复制下一格。 */
export const COPIED_NOTE_MS = 2000;
export const COPY_FAILED_NOTE = '复制失败';
export const COPY_UNSUPPORTED_NOTE = '无法复制';

/**
 * 写剪贴板并折成一句反馈。写入的动作从参数进来而不直接摸 navigator：node:test 里没有剪贴板，这样才钉得到三种结局
 * 各对哪句话。`write` 为 undefined 即上下文没有剪贴板。
 */
export function copyFeedback(write: (() => Promise<void>) | undefined): Promise<string> {
  if (!write) return Promise.resolve(COPY_UNSUPPORTED_NOTE);
  return write().then(
    () => COPIED_NOTE,
    () => COPY_FAILED_NOTE,
  );
}

/**
 * 反馈的时序：拿到那句话就显出来，COPIED_NOTE_MS 后清掉；连着复制时上一回的计时作废、从这一回重新数——不然第二次
 * 刚显出来就被第一次的计时清掉。计时器从参数进来（`after` 排一次、返回取消），node:test 用假计时器钉这条时序。
 */
export function copyNoteTimer(
  show: (note: string | null) => void,
  after: (run: () => void, ms: number) => () => void,
): { show: (note: string) => void; cancel: () => void } {
  let cancel: (() => void) | undefined;
  return {
    show: (note) => {
      cancel?.();
      show(note);
      cancel = after(() => show(null), COPIED_NOTE_MS);
    },
    cancel: () => {
      cancel?.();
      cancel = undefined;
    },
  };
}

function clipboardWriter(value: string): (() => Promise<void>) | undefined {
  if (typeof navigator === 'undefined' || !navigator.clipboard) return undefined;
  return () => navigator.clipboard.writeText(value);
}

function windowAfter(run: () => void, ms: number): () => void {
  const id = window.setTimeout(run, ms);
  return () => window.clearTimeout(id);
}

/** 一件复制控件的状态：此刻显的那句反馈（没有就 null）与触发复制的手。卸载时把未到点的计时取消。 */
function useCopyNote(): { note: string | null; copy: (value: string) => void } {
  const [note, setNote] = useState<string | null>(null);
  const [timer] = useState(() => copyNoteTimer(setNote, windowAfter));
  useEffect(() => () => timer.cancel(), [timer]);
  const copy = (value: string) => {
    void copyFeedback(clipboardWriter(value)).then(timer.show);
  };
  return { note, copy };
}

/**
 * 反馈那句话的格。role=status 常驻、没话时只留给读屏器（sr-only）而不卸掉：活动区要先在页面上，读屏器才会播报后来
 * 写进去的话。字号与字体定死：它会落在等宽的号里、也会落在半粗的人名后，哪里都该是一句弱显的小字。
 */
function CopyNote({ note }: { note: string | null }) {
  return (
    <span
      role="status"
      className={note === null ? 'sr-only' : 'ml-1 whitespace-nowrap font-sans type-body font-normal text-idpxyz-breadcrumb'}
    >
      {note ?? ''}
    </span>
  );
}

const copyIconClass = 'h-3.5 w-3.5';

/**
 * 复制图标随悬停显出的样子：显出前等 150ms、淡入淡出 150ms，离开立刻开始淡出。一页几十个可复制的值，鼠标划过时
 * 每经过一个就闪一下图标——用户 2026-11-03 指出「一直闪烁」；停留不到 150ms 的划过不显，停下来才显。
 * Tailwind 要见到完整类名才生成，所以两个组名各写一份字面量，不拼接。
 */
function hoverRevealClass(group: 'ref' | 'copy'): string {
  return group === 'ref'
    ? 'opacity-0 transition-opacity duration-150 group-hover/ref:opacity-100 group-hover/ref:delay-150 group-focus-visible/ref:opacity-100 group-focus-visible/ref:delay-0'
    : 'opacity-0 transition-opacity duration-150 group-hover/copy:opacity-100 group-hover/copy:delay-150 focus-visible:opacity-100 focus-visible:delay-0';
}

/**
 * 号码类的值（编码、注册号、监管登记号、账号、SWIFT、来源单据、电话、邮箱、社交账号）：等宽，整个值是按钮，点击即复制；
 * 悬停时值末尾出一枚 14px 复制图标、title 给「点击复制」；Tab 到、Enter / 空格复制是 button 自带的。
 * 读屏名「复制<名> <值>」：名说清复制的是什么，值跟在后面——按钮的内容就是值本身，aria-label 一盖，值就不再被读到。
 * 颜色随外层（text-inherit）：号落在弱显小字里（SWIFT、概要那句「来源单据」）要弱显，落在正文里要正文色，由摆它的格定。
 * 图标与反馈都在按钮里面、跟在值的末字后：按钮按行内盒排，值折行时图标在末行行尾，不悬在半空；反馈显出来时顶替图标的位置。
 * 图标透明而不是不渲染：悬停时显出来不挪动版面。点击不冒泡：它常落在工作台的行里，冒上去会把行选中或收起。
 */
export function RefText({ value, name, className }: { value: string; name: string; className?: string }) {
  const { note, copy } = useCopyNote();
  return (
    <button
      type="button"
      aria-label={`复制${name} ${value}`}
      title={codeRefTitle(name, value)}
      onClick={(event: MouseEvent<HTMLButtonElement>) => {
        event.stopPropagation();
        copy(value);
      }}
      className={`group/ref max-w-full rounded text-left font-mono text-inherit transition-colors duration-150 hover:text-idpxyz-accent focus-visible:outline focus-visible:outline-2 focus-visible:outline-idpxyz-accent ${className ?? ''}`.trim()}
    >
      <span className="[overflow-wrap:anywhere]">{value}</span>
      {note === null ? (
        <Copy
          aria-hidden="true"
          className={`ml-1 inline-block ${copyIconClass} align-[-2px] text-idpxyz-breadcrumb ${hoverRevealClass('ref')}`}
        />
      ) : null}
      <CopyNote note={note} />
    </button>
  );
}

/**
 * 文本类的值（公司名、开票抬头、地址、户名、开户行、联系人姓名）：文字照常可选可拖，值末尾挂一枚小复制图标，悬停到值上
 * 才显，点图标才整段复制、同样行内反馈两秒。`value` 是复制出去的整段（地址按行拼接换行），`children` 是排好版的显示形——
 * 两者可以不一样（显示形把括号组粘住、把国家译成名）。图标常驻 DOM、只是透明：Tab 能到它，聚焦时也显出来；触屏没有
 * 悬停，点值末尾那一小块同样能复制。读屏名「复制<名>」：值本身是旁边的普通文字，读屏器自己会读到，不重复。
 */
export function CopyAside({
  value,
  name,
  children,
  className,
}: {
  value: string;
  name: string;
  children: ReactNode;
  className?: string;
}) {
  const { note, copy } = useCopyNote();
  // 悬停区是值的整块矩形（inline-block），不是逐个字形：多行地址的行间空隙、末字到图标之间的空隙都还在区里，鼠标从文字
  // 挪向图标的路上图标不会消失（用户 2026-11-03 指出「点中这个图标很麻烦」）。按钮四周各垫 4px、负外边距抵回，可点
  // 范围 22px 而版面不变。
  return (
    <span className={`group/copy inline-block max-w-full align-top ${className ?? ''}`.trim()}>
      {children}
      <button
        type="button"
        aria-label={`复制${name}`}
        title={`复制${name}`}
        onClick={(event: MouseEvent<HTMLButtonElement>) => {
          event.stopPropagation();
          copy(value);
        }}
        className={`-my-1 ml-0.5 inline-flex rounded p-1 align-[-6px] text-idpxyz-breadcrumb hover:bg-idpxyz-hover hover:text-idpxyz-accent focus-visible:outline focus-visible:outline-2 focus-visible:outline-idpxyz-accent ${hoverRevealClass('copy')}`}
      >
        <Copy aria-hidden="true" className={copyIconClass} />
      </button>
      <CopyNote note={note} />
    </span>
  );
}
