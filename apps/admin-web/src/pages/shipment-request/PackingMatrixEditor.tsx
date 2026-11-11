import { useState } from 'react';
import { nonBlankGoodsLineIndexes, type GoodsLineForm } from './goods-form';
import { inputClass } from './controls';
import { decimalShape } from './form-shapes';
import {
  fillSameEachBox,
  packingCell,
  packingCheckOf,
  packingProblemsOf,
  withPackingCell,
  type PackingMatrix,
} from './packing-form';

// 装箱矩阵的录入(ADR-0373 决定三,票 declared-goods-lines/05):一箱一行、一种货一列,格里填这一箱装了这种货多少。小包下单页的
// 多件委托与平台货件页共用。表下逐行写各箱之和与这一行报的数量,对不上的点出来;服务端照样核,这里只是方便录入。
//
// 格的问题随填随显,不等提交:矩阵大,滚到第 27 箱才发现一格填错,比提交后对着一句「有 1 处没填好」去找要省事。

const cellClass =
  'w-[72px] rounded border bg-idpxyz-inputBg px-1.5 py-1 type-value text-idpxyz-text font-mono ' +
  'focus:outline-none focus:border-idpxyz-accent';

function lineNameOf(line: GoodsLineForm, index: number): string {
  return line.descriptionZh.trim() || line.descriptionEn.trim() || line.sku.trim() || `第 ${index + 1} 行`;
}

export function PackingMatrixEditor({
  lines,
  boxNames,
  matrix,
  onChange,
}: {
  lines: GoodsLineForm[];
  /** 每箱在左列的叫法,按箱的次序;长度就是箱数。 */
  boxNames: string[];
  matrix: PackingMatrix;
  onChange: (matrix: PackingMatrix) => void;
}) {
  const used = nonBlankGoodsLineIndexes(lines);
  const boxCount = boxNames.length;
  const check = packingCheckOf(lines, matrix, boxCount);
  const problems = packingProblemsOf(lines, matrix, boxCount, check);

  if (used.length === 0) {
    return <p className="type-body text-idpxyz-textMuted">先在上面填货物行,再填每箱装了哪几行。</p>;
  }

  return (
    <div className="space-y-3">
      <QuickFill
        lines={lines}
        used={used}
        boxCount={boxCount}
        onFill={(line, perBox, from, to) => onChange(fillSameEachBox(matrix, line, perBox, from, to))}
      />
      <div className="max-h-[480px] overflow-auto rounded border border-idpxyz-border">
        <table className="border-collapse">
          <thead className="sticky top-0 z-20 bg-idpxyz-editor">
            <tr>
              <th scope="col" className="sticky left-0 z-30 bg-idpxyz-editor px-2 py-1.5 text-left type-caption text-idpxyz-textMuted">
                箱
              </th>
              {used.map((line) => (
                <th
                  key={line}
                  scope="col"
                  title={lineNameOf(lines[line], line)}
                  className="max-w-[160px] truncate px-2 py-1.5 text-left type-caption text-idpxyz-textMuted"
                >
                  第 {line + 1} 行 · {lineNameOf(lines[line], line)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {boxNames.map((name, box) => (
              <tr key={box} className="border-t border-idpxyz-border">
                <th
                  scope="row"
                  className="sticky left-0 z-10 bg-idpxyz-editor px-2 py-1 text-left type-body font-normal text-idpxyz-text whitespace-nowrap"
                >
                  {name}
                </th>
                {used.map((line) => {
                  const value = packingCell(matrix, box, line);
                  const malformed = value.trim() !== '' && !decimalShape(value.trim());
                  return (
                    <td key={line} className="px-2 py-1">
                      <input
                        className={`${cellClass} ${malformed ? 'border-idpxyz-textBright' : 'border-idpxyz-border'}`}
                        inputMode="decimal"
                        aria-label={`${name} 第 ${line + 1} 行数量`}
                        aria-invalid={malformed || undefined}
                        value={value}
                        onChange={(event) => onChange(withPackingCell(matrix, box, line, event.target.value))}
                      />
                    </td>
                  );
                })}
              </tr>
            ))}
          </tbody>
          <tfoot className="sticky bottom-0 z-20 bg-idpxyz-editor">
            <tr className="border-t border-idpxyz-border">
              <th
                scope="row"
                className="sticky left-0 z-30 bg-idpxyz-editor px-2 py-1.5 text-left type-caption font-normal text-idpxyz-textMuted"
              >
                各箱合计 / 本行数量
              </th>
              {check.lines.map((line) => (
                <td
                  key={line.line}
                  className={`px-2 py-1.5 type-caption font-mono ${line.balanced ? 'text-idpxyz-textMuted' : 'text-idpxyz-textBright font-bold'}`}
                >
                  {line.packed} / {line.declared || '未填'}
                </td>
              ))}
            </tr>
          </tfoot>
        </table>
      </div>
      {problems.length > 0 ? (
        <ul className="list-disc pl-5 type-caption leading-4 text-idpxyz-textBright space-y-0.5">
          {problems.map((problem) => (
            <li key={problem}>{problem}</li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

function QuickFill({
  lines,
  used,
  boxCount,
  onFill,
}: {
  lines: GoodsLineForm[];
  used: number[];
  boxCount: number;
  onFill: (line: number, perBox: string, fromBox: number, toBox: number) => void;
}) {
  const [line, setLine] = useState<number | null>(null);
  const [perBox, setPerBox] = useState('');
  const [from, setFrom] = useState('1');
  const [to, setTo] = useState('');
  // 选中的那一行被去掉后回到第一个非空行,不让下拉框显示一行、按钮按另一行。
  const chosen = line !== null && used.includes(line) ? line : used[0];
  const unit = lines[chosen]?.unit.trim() || '件';
  const fromBox = Number(from);
  const toBox = to.trim() === '' ? boxCount : Number(to);
  const ready =
    decimalShape(perBox.trim()) &&
    from.trim() !== '' &&
    Number.isInteger(fromBox) &&
    Number.isInteger(toBox) &&
    fromBox >= 1 &&
    toBox >= fromBox &&
    toBox <= boxCount;

  return (
    <div className="flex flex-wrap items-end gap-2 type-body text-idpxyz-textMuted">
      <span>同款每箱</span>
      <select
        className={`${inputClass} w-auto`}
        aria-label="快捷填哪一行货"
        value={String(chosen)}
        onChange={(event) => setLine(Number(event.target.value))}
      >
        {used.map((index) => (
          <option key={index} value={index}>
            第 {index + 1} 行 · {lineNameOf(lines[index], index)}
          </option>
        ))}
      </select>
      <span>装</span>
      <input
        className={`${inputClass} w-[72px]`}
        inputMode="decimal"
        aria-label="每箱数量"
        value={perBox}
        onChange={(event) => setPerBox(event.target.value)}
      />
      <span>{unit},从第</span>
      <input className={`${inputClass} w-[64px]`} inputMode="numeric" aria-label="从第几箱" value={from} onChange={(event) => setFrom(event.target.value)} />
      <span>箱到第</span>
      <input
        className={`${inputClass} w-[64px]`}
        inputMode="numeric"
        aria-label="到第几箱"
        placeholder={String(boxCount)}
        value={to}
        onChange={(event) => setTo(event.target.value)}
      />
      <span>箱</span>
      <button
        type="button"
        disabled={!ready}
        onClick={() => onFill(chosen, perBox.trim(), fromBox - 1, toBox - 1)}
        className="rounded border border-idpxyz-border px-3 py-1.5 type-body text-idpxyz-text hover:bg-idpxyz-hover disabled:opacity-40"
      >
        填入
      </button>
    </div>
  );
}
