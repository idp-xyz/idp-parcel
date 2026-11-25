import type { KeyboardEvent } from 'react';
import type { ParcelPatch } from './pack-flow';
import type { PackOrder } from './pack-order';
import { judgeParcel, type ParcelDraft } from './pack-parcels';
import { stationButtonClass } from './StationFrame';

// 打包的一个包裹：装什么、装几件、用什么包材、称多重、多大。填什么、对不对由 pack-flow.ts 与 pack-parcels.ts 判，这里只摆。
//
// 称重与尺寸的输入框拿着焦点时，扫描框不抢它（use-scan-focus.ts 对「焦点在别的输入里」本来就让位）；在框里按回车，把焦点交回
// 扫描框，点按钮、点空白处也回到扫描框。所以填完重量接着扫下一个包材码，不用再点扫描框。

const fieldClass =
  'h-11 w-full min-w-0 rounded border border-idpxyz-border bg-idpxyz-inputBg px-3 font-mono type-body text-idpxyz-textBright outline-none focus:border-idpxyz-accent read-only:opacity-60';

/** 回车算填完这一格：交回焦点，扫描框接着收码。 */
function leaveOnEnter(event: KeyboardEvent<HTMLInputElement>) {
  if (event.key !== 'Enter') return;
  event.preventDefault();
  event.currentTarget.blur();
}

function TextField({
  label,
  value,
  readOnly,
  inputMode,
  onChange,
}: {
  label: string;
  value: string;
  readOnly: boolean;
  inputMode: 'decimal' | 'numeric' | 'text';
  onChange: (value: string) => void;
}) {
  return (
    <label className="flex min-w-0 flex-col gap-1">
      <span className="type-caption text-idpxyz-textMuted">{label}</span>
      <input
        type="text"
        inputMode={inputMode}
        autoComplete="off"
        autoCorrect="off"
        autoCapitalize="off"
        spellCheck={false}
        enterKeyHint="done"
        readOnly={readOnly}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        onKeyDown={leaveOnEnter}
        onFocus={(event) => event.currentTarget.select()}
        className={fieldClass}
      />
    </label>
  );
}

export interface ParcelCardProps {
  parcel: ParcelDraft;
  /** 第几个包裹，从 1 数。 */
  position: number;
  order: PackOrder;
  /** 只有打包这一步（packing）能改；提交中、没送到这些阶段只看。 */
  editable: boolean;
  onEdit: (patch: ParcelPatch) => void;
  onRemove: () => void;
  onPickMaterial: () => void;
}

export function ParcelCard({ parcel, position, order, editable, onEdit, onRemove, onPickMaterial }: ParcelCardProps) {
  const problem = judgeParcel(order, parcel, position);
  const unit = parcel.unit;
  const dimsFromMaterial = !parcel.dimsEdited && parcel.packaging !== '' && parcel.length !== '';
  const dimsMissingFromMaterial = !parcel.dimsEdited && parcel.packaging !== '' && parcel.length === '';

  return (
    <section aria-label={`包裹 ${position}`} className="flex flex-col gap-3 rounded-lg border border-idpxyz-border bg-idpxyz-editor p-3">
      <div className="flex items-center justify-between gap-2">
        <h3 className="type-section font-semibold text-idpxyz-textBright">包裹 {position}</h3>
        <button type="button" className={stationButtonClass} disabled={!editable} onClick={onRemove}>
          删除这个包裹
        </button>
      </div>

      <div className="flex flex-col gap-2">
        <span className="type-caption text-idpxyz-textMuted">装什么</span>
        {order.goods.length > 1 ? (
          <div className="flex flex-wrap gap-2">
            {order.goods.map((goods) => {
              const picked = goods.goodsCode === parcel.goodsCode && goods.unit === parcel.unit;
              return (
                <button
                  key={`${goods.goodsCode}\u0000${goods.unit}`}
                  type="button"
                  aria-pressed={picked}
                  disabled={!editable}
                  className={`${stationButtonClass} break-all font-mono ${picked ? 'border-idpxyz-accent bg-idpxyz-activeItem' : ''}`}
                  onClick={() => onEdit({ goodsCode: goods.goodsCode, unit: goods.unit })}
                >
                  {goods.goodsCode}
                </button>
              );
            })}
          </div>
        ) : (
          <p className="break-all font-mono type-body text-idpxyz-text">{parcel.goodsCode}</p>
        )}
        <div className="max-w-[12rem]">
          <TextField label={`装几件（${unit}）`} value={parcel.quantity} readOnly={!editable} inputMode="numeric" onChange={(quantity) => onEdit({ quantity })} />
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <span className="type-caption text-idpxyz-textMuted">包材</span>
        {parcel.packaging === '' ? (
          <span className="type-body text-idpxyz-text">还没选，扫包材码或点选</span>
        ) : (
          <span className="break-all type-body text-idpxyz-text">
            <span className="font-mono">{parcel.packaging}</span>
            {parcel.packagingName ? `（${parcel.packagingName}）` : ''}
          </span>
        )}
        <button type="button" className={stationButtonClass} disabled={!editable} onClick={onPickMaterial}>
          {parcel.packaging === '' ? '选包材' : '换包材'}
        </button>
      </div>

      <div className="grid grid-cols-2 gap-2">
        <TextField label="重量" value={parcel.weight} readOnly={!editable} inputMode="decimal" onChange={(weight) => onEdit({ weight })} />
        <TextField label="重量单位" value={parcel.weightUnit} readOnly={!editable} inputMode="text" onChange={(weightUnit) => onEdit({ weightUnit })} />
      </div>
      {editable && parcel.packagingWeightUnit !== '' && parcel.weightUnit === '' ? (
        <div>
          <button type="button" className={stationButtonClass} onClick={() => onEdit({ weightUnit: parcel.packagingWeightUnit })}>
            重量单位用包材登记的 {parcel.packagingWeightUnit}
          </button>
        </div>
      ) : null}

      <div className="grid grid-cols-3 gap-2">
        <TextField label="长" value={parcel.length} readOnly={!editable} inputMode="decimal" onChange={(length) => onEdit({ length })} />
        <TextField label="宽" value={parcel.width} readOnly={!editable} inputMode="decimal" onChange={(width) => onEdit({ width })} />
        <TextField label="高" value={parcel.height} readOnly={!editable} inputMode="decimal" onChange={(height) => onEdit({ height })} />
      </div>
      <div className="max-w-[12rem]">
        <TextField
          label="尺寸单位"
          value={parcel.dimensionUnit}
          readOnly={!editable}
          inputMode="text"
          onChange={(dimensionUnit) => onEdit({ dimensionUnit })}
        />
      </div>
      {dimsFromMaterial ? <p className="type-caption text-idpxyz-textMuted">长宽高取了包材登记的外廓，可以改。</p> : null}
      {dimsMissingFromMaterial ? <p className="type-caption text-idpxyz-textMuted">这个包材没有可用的外廓登记，长宽高与尺寸单位要自己填。</p> : null}

      {editable && problem !== null ? <p className="type-caption text-idpxyz-textMuted">还差：{problem.text}</p> : null}
    </section>
  );
}
