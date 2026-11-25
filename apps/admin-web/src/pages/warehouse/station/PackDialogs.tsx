import { useEffect, useMemo, useRef, useState, type FormEvent } from 'react';
import { Button, Dialog, DialogBody, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../../components/primary-action';
import { statusToneBanner, statusToneText } from '../../../components/status-tone';
import { mismatchLines, type MismatchPlan } from './pack-flow';
import { goodsKey, type PackOrder } from './pack-order';
import type { PackagingChoices } from './use-pack-session';
import type { PackagingMaterial } from './pack-parcels';

// 复核打包工位里弹出的三个框：改已扫的数量、确认数量不符照实提交、点选包材。框开着时扫描框不抢焦点（由 PackStation 传 suspended），
// 所以关框时不让 Radix 把焦点还给原来的按钮（onCloseAutoFocus），焦点交给扫描框自己放回去。

const footerButton = 'h-11 px-4 type-body';

const numberInput =
  'h-12 w-full min-w-0 rounded border border-idpxyz-border bg-idpxyz-inputBg px-3 font-mono type-value text-idpxyz-textBright outline-none focus:border-idpxyz-accent';

// —— 改数量 ——

export function CountsDialog({
  open,
  order,
  counted,
  onClose,
  onSubmit,
}: {
  open: boolean;
  order: PackOrder | null;
  counted: Readonly<Record<string, number>>;
  onClose: () => void;
  /** 交回判定：不收的把原因说回来，框留着。 */
  onSubmit: (texts: Readonly<Record<string, string>>) => { ok: true } | { ok: false; reason: string };
}) {
  const [texts, setTexts] = useState<Record<string, string>>({});
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open || order === null) return;
    setTexts(Object.fromEntries(order.goods.map((goods) => [goodsKey(goods), String(counted[goodsKey(goods)] ?? 0)])));
    setError(null);
  }, [open, order, counted]);

  if (order === null) return null;

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const result = onSubmit(texts);
    if (result.ok) onClose();
    else setError(result.reason);
  };

  return (
    <Dialog open={open} onOpenChange={(next) => (next ? undefined : onClose())}>
      <DialogContent size="sm" onCloseAutoFocus={(event) => event.preventDefault()}>
        <form onSubmit={submit}>
          <DialogHeader>
            <div>
              <DialogTitle>改已扫的数量</DialogTitle>
              <DialogDescription className="mt-1">
                把每种货已扫的件数直接改成实数。改到合计等于该有的，就自动提交复核；数对不上的，用「数量不符，照实提交」。
              </DialogDescription>
            </div>
          </DialogHeader>
          <DialogBody>
            <div className="flex flex-col gap-3">
              {order.goods.map((goods) => (
                <label key={goodsKey(goods)} className="flex flex-col gap-1.5">
                  <span className="break-all type-body text-idpxyz-textMuted">
                    <span className="font-mono text-idpxyz-text">{goods.goodsCode}</span>，该有 {goods.quantity} {goods.unit}
                  </span>
                  <input
                    inputMode="numeric"
                    autoComplete="off"
                    value={texts[goodsKey(goods)] ?? ''}
                    aria-invalid={error !== null}
                    onChange={(event) => setTexts((previous) => ({ ...previous, [goodsKey(goods)]: event.target.value }))}
                    onFocus={(event) => event.currentTarget.select()}
                    className={numberInput}
                  />
                </label>
              ))}
            </div>
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

// —— 数量不符，照实提交 ——

export function MismatchDialog({
  open,
  plan,
  onClose,
  onConfirm,
}: {
  open: boolean;
  plan: MismatchPlan | null;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const cancel = useRef<HTMLButtonElement>(null);
  if (plan === null) return null;

  return (
    <Dialog open={open} onOpenChange={(next) => (next ? undefined : onClose())}>
      <DialogContent
        size="md"
        // 默认把焦点放在「再点一点」上：扫描枪结尾的回车若落进框里，按下去的是取消，不是确认
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          cancel.current?.focus();
        }}
        onCloseAutoFocus={(event) => event.preventDefault()}
      >
        <DialogHeader>
          <div>
            <DialogTitle>数量不符，照实提交？</DialogTitle>
            <DialogDescription className="mt-1">确认之后按点到的数提交，提交了就不能撤回。</DialogDescription>
          </div>
        </DialogHeader>
        <DialogBody>
          <ul className="flex flex-col gap-2 type-body text-idpxyz-text">
            {mismatchLines(plan).map((line) => (
              <li key={line}>{line}</li>
            ))}
          </ul>
        </DialogBody>
        <DialogFooter>
          <Button ref={cancel} type="button" variant="outline" className={footerButton} onClick={onClose}>
            再点一点
          </Button>
          <Button
            type="button"
            className={`${footerButton} ${primaryActionClass}`}
            onClick={() => {
              onConfirm();
              onClose();
            }}
          >
            照实提交
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// —— 点选包材 ——

/** 列表最多摆几行：再多的靠搜索框缩小，或者直接扫码。 */
const listLimit = 100;

function sizeText(material: PackagingMaterial): string {
  if (material.length === '' || material.width === '' || material.height === '') return '没登记外廓';
  return `${material.length} × ${material.width} × ${material.height} ${material.dimensionUnit}`;
}

export function PackagingDialog({
  open,
  position,
  choices,
  onClose,
  onLoad,
  onPick,
}: {
  open: boolean;
  /** 给第几个包裹选；没在选时是 null。 */
  position: number | null;
  choices: PackagingChoices;
  onClose: () => void;
  /** 开框时读一遍名册；读不成点「再读一次」。 */
  onLoad: () => void;
  onPick: (material: PackagingMaterial) => void;
}) {
  const [filter, setFilter] = useState('');
  // 只在开框那一下读：读的函数每次渲染可能是新的，用 ref 拿最新的一份，不放进依赖里免得它一变就重读
  const load = useRef(onLoad);
  load.current = onLoad;
  const search = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!open) return;
    setFilter('');
    load.current();
  }, [open]);

  // 名册读出来、搜索框摆出来之后把焦点给它：开框那一刻框里只有「关闭」可落焦，而人要找的是列表里的包材
  const ready = choices.phase === 'ready';
  useEffect(() => {
    if (open && ready) search.current?.focus();
  }, [open, ready]);

  const shown = useMemo(() => {
    if (choices.phase !== 'ready') return [];
    const needle = filter.trim().toLowerCase();
    const rows = needle === '' ? choices.list.items : choices.list.items.filter((item) => `${item.code} ${item.name}`.toLowerCase().includes(needle));
    return rows;
  }, [choices, filter]);

  return (
    <Dialog open={open} onOpenChange={(next) => (next ? undefined : onClose())}>
      <DialogContent size="md" onCloseAutoFocus={(event) => event.preventDefault()}>
        <DialogHeader>
          <div>
            <DialogTitle>选包材{position === null ? '' : `（包裹 ${position}）`}</DialogTitle>
            <DialogDescription className="mt-1">只列在用的包材。点一个就用它；也可以关掉这个框，直接扫包材码。</DialogDescription>
          </div>
        </DialogHeader>
        <DialogBody>
          {choices.phase === 'loading' || choices.phase === 'idle' ? (
            <p role="status" className="type-body text-idpxyz-textMuted">
              正在读包材名册…
            </p>
          ) : choices.phase === 'error' ? (
            <div className="flex flex-col gap-3">
              <p role="alert" className={`rounded border px-3 py-2 type-body ${statusToneBanner.danger}`}>
                没读成包材名册：{choices.message}。
              </p>
              <div>
                <Button type="button" variant="outline" className={footerButton} onClick={onLoad}>
                  再读一次
                </Button>
              </div>
            </div>
          ) : (
            <div className="flex flex-col gap-3">
              <input
                ref={search}
                type="text"
                aria-label="按编码或名称找包材"
                placeholder="按编码或名称找"
                autoComplete="off"
                value={filter}
                onChange={(event) => setFilter(event.target.value)}
                className="h-11 w-full rounded border border-idpxyz-border bg-idpxyz-inputBg px-3 type-body text-idpxyz-textBright outline-none focus:border-idpxyz-accent"
              />
              {choices.list.truncated ? (
                <p className={`rounded border px-3 py-2 type-caption ${statusToneBanner.warning}`}>
                  只列了前 {choices.list.items.length} 个在用的包材，后面还有，列不全：找不到的请关掉这个框直接扫包材码。
                </p>
              ) : null}
              {shown.length === 0 ? (
                <p className="type-body text-idpxyz-textMuted">
                  {choices.list.items.length === 0 ? '名册里没有在用的包材。' : '没有名称或编码含这个词的在用包材。'}
                </p>
              ) : (
                <ul className="flex max-h-[50vh] flex-col gap-2 overflow-y-auto">
                  {shown.slice(0, listLimit).map((item) => (
                    <li key={item.code}>
                      <button
                        type="button"
                        className="flex w-full flex-col items-start gap-0.5 rounded border border-idpxyz-border bg-idpxyz-editor px-3 py-2 text-left type-body hover:bg-idpxyz-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-idpxyz-accent"
                        onClick={() => {
                          onPick(item);
                          onClose();
                        }}
                      >
                        <span className="break-all font-mono text-idpxyz-textBright">{item.code}</span>
                        <span className="text-idpxyz-text">{item.name}</span>
                        <span className="type-caption text-idpxyz-textMuted">{sizeText(item)}</span>
                      </button>
                    </li>
                  ))}
                </ul>
              )}
              {shown.length > listLimit ? <p className="type-caption text-idpxyz-textMuted">另有 {shown.length - listLimit} 个没列出，输入关键词缩小。</p> : null}
            </div>
          )}
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="outline" className={footerButton} onClick={onClose}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
