import { field } from '../card-form/fields';
import { SectionTitle, TextField, hintClass } from './controls';
import { useEditor } from './EditorContext';

export function StepRefs() {
  const { state, update } = useEditor();
  const { refs } = state;
  const set = (change: Partial<typeof refs>) => update((draft) => ({ ...draft, refs: { ...draft.refs, ...change } }));
  const fuel = [...new Set(state.fees.filter((fee) => fee.kind === 'PERCENT' && fee.percentMode === 'SERIES' && fee.seriesId.trim() !== '').map((fee) => fee.seriesId.trim()))];
  const needRemote = state.fees.some((fee) => fee.kind === 'TABLE_REMOTE');

  return (
    <div className="space-y-6">
      <section>
        <SectionTitle hint="这些是价卡引用的外部清单，不在这里维护，只在这里说明用哪一份。">引用的燃油费率序列</SectionTitle>
        {fuel.length === 0 ? (
          <p className={hintClass}>这张价卡没有按燃油费率序列计算的费用项。</p>
        ) : (
          <ul className="list-disc pl-5 text-[13px] text-idpxyz-text">
            {fuel.map((id) => (
              <li key={id} className="font-mono">
                {id}
              </li>
            ))}
          </ul>
        )}
        <p className={`${hintClass} mt-1`}>来自第四步里的费用项；要换序列回到第四步改。</p>
      </section>

      <section>
        <SectionTitle hint="目录决定收件地址属于哪个分区、哪个偏远档位。目前没有目录清单可选，请直接填目录标识。">地址目录</SectionTitle>
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          <TextField
            id={field.refs('zoneCatalogue')}
            label="分区目录"
            mono
            value={refs.zoneCatalogue}
            onChange={(zoneCatalogue) => set({ zoneCatalogue })}
            hint="按收件邮编查分区用；价表里的分区名要在这份目录里。没有可不填"
          />
          <TextField
            id={field.refs('remoteTierCatalogue')}
            label="偏远档位目录"
            required={needRemote}
            mono
            value={refs.remoteTierCatalogue}
            onChange={(remoteTierCatalogue) => set({ remoteTierCatalogue })}
            hint={needRemote ? '第四步有按偏远档位查价表的费用项，必须填' : '按收件邮编查偏远档位用；没有按偏远档位收费的费用项可不填'}
          />
        </div>
      </section>
    </div>
  );
}
