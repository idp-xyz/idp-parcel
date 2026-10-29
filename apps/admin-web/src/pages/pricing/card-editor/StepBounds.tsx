import { amountRoundingPointLabels, roundingModeOptionLabels } from '../../../vocabulary/price-card';
import { field } from '../card-form/fields';
import { allowedValues, sheetNames } from '../card-form/spec';
import { FieldBox, SelectField, SectionTitle, TextField, hintClass } from './controls';
import { useEditor } from './EditorContext';

export function StepBounds() {
  const { state, update, spec } = useEditor();
  const { bounds, amountRounding } = state;
  const currency = state.basics.currency || '所选币种';
  const setBounds = (change: Partial<typeof bounds>) => update((draft) => ({ ...draft, bounds: { ...draft.bounds, ...change } }));
  const setRounding = (change: Partial<typeof amountRounding>) => update((draft) => ({ ...draft, amountRounding: { ...draft.amountRounding, ...change } }));
  const points = allowedValues(spec, sheetNames.card, 'amountRoundingPoints');

  return (
    <div className="space-y-6">
      <section>
        <SectionTitle hint="对整票所有费用的合计设一个下限和上限，可以只填一端；不设就都留空。">总额上下限</SectionTitle>
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          <TextField
            id={field.bounds('minimum')}
            label={`最低收费（${currency}）`}
            mono
            inputMode="decimal"
            value={bounds.minimum}
            onChange={(minimum) => setBounds({ minimum })}
            hint="合计低于它，就补一笔差额，补足到最低收费"
          />
          <TextField
            id={field.bounds('maximum')}
            label={`最高收费（${currency}）`}
            mono
            inputMode="decimal"
            value={bounds.maximum}
            onChange={(maximum) => setBounds({ maximum })}
            hint="合计高于它，就扣掉超出的部分，封顶到最高收费"
          />
        </div>
      </section>

      <section>
        <SectionTitle hint="金额算出来后按多大的单位进位或四舍五入，比如精确到 0.01。不取整就不勾。">金额取整</SectionTitle>
        <label className="mb-3 flex items-center gap-2 text-[13px] text-idpxyz-text">
          <input type="checkbox" checked={amountRounding.enabled} onChange={(event) => setRounding({ enabled: event.target.checked })} />
          对算出的金额取整
        </label>
        {amountRounding.enabled ? (
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
            <SelectField
              id={field.rounding('mode')}
              label="取整方式"
              required
              value={amountRounding.mode}
              options={allowedValues(spec, sheetNames.card, 'amountRoundingMode').map((code) => ({ value: code, label: roundingModeOptionLabels[code] ?? code }))}
              onChange={(mode) => setRounding({ mode })}
            />
            <TextField
              id={field.rounding('increment')}
              label={`进位单位（${currency}）`}
              required
              mono
              inputMode="decimal"
              value={amountRounding.increment}
              onChange={(increment) => setRounding({ increment })}
              hint="如 0.01"
            />
            <FieldBox id={field.rounding('points')} label="在哪些环节取整" required labelsControl={false} className="md:col-span-2" hint="每一项费用算出来就取整，还是只对合计取整，可以多选。">
              <div className="flex flex-wrap gap-4" data-field={field.rounding('points')} tabIndex={-1}>
                {points.map((code) => (
                  <label key={code} className="flex items-center gap-1.5 text-[13px] text-idpxyz-text">
                    <input
                      type="checkbox"
                      checked={amountRounding.points.includes(code)}
                      onChange={(event) =>
                        setRounding({ points: event.target.checked ? [...amountRounding.points, code] : amountRounding.points.filter((point) => point !== code) })
                      }
                    />
                    {amountRoundingPointLabels[code] ?? code}
                  </label>
                ))}
              </div>
            </FieldBox>
          </div>
        ) : null}
        <p className={`${hintClass} mt-2`}>卡上写了金额取整时，最低 / 最高收费要落在进位单位的整数倍上，否则预览会提示。</p>
      </section>
    </div>
  );
}
