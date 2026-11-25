import { useRef, type KeyboardEvent } from 'react';
import { scanFromKey } from './scan-input';
import { edgeClass, type ScanTone } from './ScanFeedback';
import { useScanFocus } from './use-scan-focus';

// 扫描框：一个一直有焦点的输入。扫描枪当键盘用，打一串字加回车；没有枪就手敲加回车，粘贴也算。
// 回车算不算一次扫码、码规范化成什么在 scan-input.ts；焦点怎么一直留在这里在 use-scan-focus.ts。
//
// 输入不受控：枪一秒能打十几个字符，受控的输入每个字符都要过一次 React 状态再写回，不受控的只在回车那一刻读一次值。
// 不收码的时候（提交中、认码中、等点按钮）用只读而不是禁用：禁用的输入会丢焦点，只读的还拿着焦点，枪的字符落不进去、
// 回车也提交不出空码，等能收码了不用再抢一次。

export interface ScanBoxProps {
  /** 一次扫码，码已规范化、不为空。 */
  onScan: (code: string) => void;
  placeholder: string;
  /** 这一刻不收码。 */
  busy: boolean;
  /** 弹出的数量输入、确认框、换波次的下拉开着：焦点该在它们上面，不抢。 */
  suspended: boolean;
  tone: ScanTone;
  /** 刚扫到的码，写在框下面。 */
  scanned?: string;
}

export function ScanBox({ onScan, placeholder, busy, suspended, tone, scanned }: ScanBoxProps) {
  const input = useRef<HTMLInputElement>(null);
  useScanFocus(input, suspended);

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key !== 'Enter') return;
    event.preventDefault();
    const field = event.currentTarget;
    const code = scanFromKey({ key: event.key, isComposing: event.nativeEvent.isComposing }, field.value);
    field.value = '';
    if (code !== null) onScan(code);
  };

  return (
    <div>
      <div className={`rounded-md border-2 bg-idpxyz-inputBg px-3 ${edgeClass[tone]}`}>
        <input
          ref={input}
          type="text"
          aria-label="扫描框"
          aria-busy={busy}
          autoComplete="off"
          autoCorrect="off"
          autoCapitalize="off"
          spellCheck={false}
          readOnly={busy}
          placeholder={placeholder}
          onKeyDown={onKeyDown}
          className="h-12 w-full bg-transparent font-mono type-value text-idpxyz-textBright outline-none placeholder:text-idpxyz-textMuted"
        />
      </div>
      {scanned ? <p className="mt-1 break-all font-mono type-caption text-idpxyz-textMuted">刚才扫到：{scanned}</p> : null}
    </div>
  );
}
