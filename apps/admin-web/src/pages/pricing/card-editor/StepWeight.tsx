import { Button } from '@idpxyz/ui-primitives';
import { Plus, Trash2 } from 'lucide-react';
import { roundingModeOptionLabels, weightMethodLabels } from '../../../vocabulary/price-card';
import { field } from '../card-form/fields';
import { newSegment, type RoundingSegment } from '../card-form/model';
import { allowedValues, sheetNames } from '../card-form/spec';
import { CellInput, GroupErrors, SelectControl, SelectField, SectionTitle, TextField, hintClass, useFieldErrors } from './controls';
import { useEditor } from './EditorContext';
import { labelled, lengthUnitOptions, unitWord } from './options';

function Segments({ policy, title, hint }: { policy: 'chargeable' | 'volumetric'; title: string; hint: string }) {
  const { state, update, spec, env } = useEditor();
  const segments = state.weight[policy];
  const modes = labelled(allowedValues(spec, sheetNames.weightRounding, 'mode'), roundingModeOptionLabels);
  const unit = state.basics.weightUnit === '' ? '所选重量单位' : unitWord(state.basics.weightUnit, env);
  const setSegments = (change: (list: RoundingSegment[]) => RoundingSegment[]) =>
    update((draft) => ({ ...draft, weight: { ...draft.weight, [policy]: change(draft.weight[policy]) } }));
  const patch = (uid: string, change: Partial<RoundingSegment>) =>
    setSegments((list) => list.map((segment) => (segment.uid === uid ? { ...segment, ...change } : segment)));
  const ids = segments.flatMap((segment) => [
    field.segment(policy, segment.uid, 'mode'),
    field.segment(policy, segment.uid, 'increment'),
    field.segment(policy, segment.uid, 'maximum'),
  ]);

  return (
    <section>
      <SectionTitle hint={hint}>{title}</SectionTitle>
      <div className="overflow-x-auto rounded border border-idpxyz-border">
        <table className="w-full min-w-[480px] text-[12px]">
          <thead className="bg-idpxyz-sidebar text-left text-idpxyz-breadcrumb">
            <tr>
              <th className="w-10 px-2 py-1.5 font-medium">段</th>
              <th className="px-2 py-1.5 font-medium">进位方式</th>
              <th className="px-2 py-1.5 font-medium">进位单位（{unit}）</th>
              <th className="px-2 py-1.5 font-medium">本段上界（{unit}，最后一段留空）</th>
              <th className="w-10 px-2 py-1.5" />
            </tr>
          </thead>
          <tbody>
            {segments.map((segment, index) => (
              <tr key={segment.uid} className="border-t border-idpxyz-border">
                <td className="px-2 py-1 text-idpxyz-breadcrumb">{index + 1}</td>
                <td className="px-1 py-1">
                  <SegmentMode id={field.segment(policy, segment.uid, 'mode')} value={segment.mode} options={modes} onChange={(mode) => patch(segment.uid, { mode })} />
                </td>
                <td className="px-1 py-1">
                  <CellInput id={field.segment(policy, segment.uid, 'increment')} label={`第 ${index + 1} 段进位单位`} value={segment.increment} onChange={(increment) => patch(segment.uid, { increment })} />
                </td>
                <td className="px-1 py-1">
                  <CellInput id={field.segment(policy, segment.uid, 'maximum')} label={`第 ${index + 1} 段上界`} value={segment.maximum} placeholder={index === segments.length - 1 ? '不封顶' : ''} onChange={(maximum) => patch(segment.uid, { maximum })} />
                </td>
                <td className="px-1 py-1 text-center">
                  <Button variant="ghost" size="icon" aria-label={`删除第 ${index + 1} 段`} title="删除这一段" disabled={segments.length <= 1} onClick={() => setSegments((list) => list.filter((item) => item.uid !== segment.uid))}>
                    <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <GroupErrors ids={ids} />
      <Button variant="outline" size="sm" className="mt-2" onClick={() => setSegments((list) => [...list, newSegment()])}>
        <Plus className="mr-1 h-3.5 w-3.5" aria-hidden="true" />
        添加一段
      </Button>
    </section>
  );
}

function SegmentMode({ id, value, options, onChange }: { id: string; value: string; options: ReturnType<typeof labelled>; onChange: (next: string) => void }) {
  const errors = useFieldErrors(id);
  return <SelectControl id={id} value={value} options={options} label="进位方式" invalid={errors.length > 0} onChange={onChange} />;
}

export function StepWeight() {
  const { state, update, spec, env } = useEditor();
  const { weight } = state;
  const patch = (change: Partial<typeof weight>) => update((draft) => ({ ...draft, weight: { ...draft.weight, ...change } }));

  return (
    <div className="space-y-6">
      <section>
        <SectionTitle hint="用来算运费的重量怎么得出：只看包裹实际称重，还是把体积折算成重量后取较大的那个。">计重方法</SectionTitle>
        <SelectField
          id={field.weight('method')}
          label="计重方法"
          required
          className="max-w-sm"
          value={weight.method}
          options={labelled(allowedValues(spec, sheetNames.card, 'weightMethod'), weightMethodLabels)}
          onChange={(method) => patch({ method })}
        />
      </section>

      {weight.method === 'MAX' ? (
        <section>
          <SectionTitle hint="体积重 = 长 × 宽 × 高 ÷ 除数。">体积重</SectionTitle>
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
            <TextField id={field.weight('divisor')} label="体积重除数" required mono inputMode="decimal" value={weight.divisor} onChange={(divisor) => patch({ divisor })} hint="如 5000" />
            <SelectField id={field.weight('lengthUnit')} label="长宽高的长度单位" required value={weight.lengthUnit} options={lengthUnitOptions(spec, env)} onChange={(lengthUnit) => patch({ lengthUnit })} />
          </div>
        </section>
      ) : null}

      <Segments
        policy="chargeable"
        title="计价重量的进位"
        hint="得出的重量再按这里的规则进位后才用来查价表。只有一段就是全程同一个规则；分多段时，前面各段填上界，最后一段不填。"
      />
      {weight.method === 'MAX' ? (
        <Segments policy="volumetric" title="体积重的进位" hint="体积重折算出来后先按这里进位，再与实重比较取较大者。" />
      ) : null}
      <p className={hintClass}>重量规则里的所有数字都用第一步选的重量单位。</p>
    </div>
  );
}
