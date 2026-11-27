import { useEffect, useRef, useState, type FormEvent } from 'react';
import { Button, Dialog, DialogBody, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../../components/primary-action';
import { statusToneText } from '../../../components/status-tone';
import { shortPickLines, type ShortPickPlan } from './pick-flow';
import type { PickCell } from './pick-queue';

// 拣货工位里弹出的两个框：改数量、确认少拣。两个框开着时扫描框不抢焦点（由 PickStation 传 suspended），所以关框时不让 Radix
// 把焦点还给原来的按钮（onCloseAutoFocus），焦点交给扫描框自己放回去。

const footerButton = 'h-11 px-4 type-body';

export function QuantityDialog({
  open,
  cell,
  counted,
  onClose,
  onSubmit,
}: {
  open: boolean;
  cell: PickCell | null;
  counted: number;
  onClose: () => void;
  /** 交回判定：不收的把原因说回来，框留着。 */
  onSubmit: (text: string) => { ok: true } | { ok: false; reason: string };
}) {
  const [text, setText] = useState('');
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open && cell !== null) {
      setText(String(counted > 0 ? counted : cell.quantity));
      setError(null);
    }
  }, [open, counted, cell]);

  if (cell === null) return null;

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const result = onSubmit(text);
    if (result.ok) onClose();
    else setError(result.reason);
  };

  return (
    <Dialog open={open} onOpenChange={(next) => (next ? undefined : onClose())}>
      <DialogContent size="sm" onCloseAutoFocus={(event) => event.preventDefault()}>
        <form onSubmit={submit}>
          <DialogHeader>
            <div>
              <DialogTitle>改数量</DialogTitle>
              <DialogDescription className="mt-1">
                这一格要拣 {cell.quantity} 件 {cell.goodsCode}。把已扫的数直接改成实数，1 到 {cell.quantity} 之间；改到 {cell.quantity} 就自动提交。
              </DialogDescription>
            </div>
          </DialogHeader>
          <DialogBody>
            <label className="flex flex-col gap-1.5">
              <span className="type-body text-idpxyz-textMuted">已扫的件数</span>
              <input
                inputMode="numeric"
                autoComplete="off"
                value={text}
                aria-invalid={error !== null}
                onChange={(event) => setText(event.target.value)}
                onFocus={(event) => event.currentTarget.select()}
                className="h-12 rounded border border-idpxyz-border bg-idpxyz-inputBg px-3 font-mono type-value text-idpxyz-textBright outline-none focus:border-idpxyz-accent"
              />
            </label>
            {error ? (
              <p role="alert" className={`mt-2 type-body ${statusToneText.danger}`}>
                {error}
              </p>
            ) : null}
          </DialogBody>
          <DialogFooter>
            <Button type="button" variant="outline" className={footerButton} onClick={onClose}>
              取消
            </Button>
            <Button type="submit" className={`${footerButton} ${primaryActionClass}`}>
              确定
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function ShortPickDialog({
  open,
  plan,
  onClose,
  onConfirm,
}: {
  open: boolean;
  plan: ShortPickPlan | null;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const cancel = useRef<HTMLButtonElement>(null);
  if (plan === null) return null;

  return (
    <Dialog open={open} onOpenChange={(next) => (next ? undefined : onClose())}>
      <DialogContent
        size="md"
        // 默认把焦点放在「再扫一扫」上：扫描枪结尾的回车若落进框里，按下去的是取消，不是确认
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          cancel.current?.focus();
        }}
        onCloseAutoFocus={(event) => event.preventDefault()}
      >
        <DialogHeader>
          <div>
            <DialogTitle>{plan.quantity === 0 ? '这一格一件都没有？' : '少拣这一格？'}</DialogTitle>
            <DialogDescription className="mt-1">
              {plan.quantity === 0 ? '确认之后按 0 件提交，这一格记成短拣，提交了就不能改。' : '确认之后按已扫的数提交，提交了就不能改。'}
            </DialogDescription>
          </div>
        </DialogHeader>
        <DialogBody>
          <ul className="flex flex-col gap-2 type-body text-idpxyz-text">
            {shortPickLines(plan).map((line) => (
              <li key={line}>{line}</li>
            ))}
          </ul>
        </DialogBody>
        <DialogFooter>
          <Button ref={cancel} type="button" variant="outline" className={footerButton} onClick={onClose}>
            再扫一扫
          </Button>
          <Button
            type="button"
            className={`${footerButton} ${primaryActionClass}`}
            onClick={() => {
              onConfirm();
              onClose();
            }}
          >
            确认少拣并提交
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
