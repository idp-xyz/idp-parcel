import { Button } from '@idpxyz/ui-primitives';
import { pageTitleById } from '../../navigation';
import { formatInstant } from '../moment';

// 「我的工作」两页共用的小件（票 admin-web-ux-alignment/02）：按模块筛的 chip 组、时刻格、模块名转写。
// 只放呈现层的小件，不放存储与判读；两页各写一份会在改样式时分叉。

/** 模块 id → 导航里的名字；词表里没有的（导航改过名、旧历史）原样示 id，不编名。 */
export function moduleTitleOf(moduleId: string): string {
  return pageTitleById[moduleId] ?? moduleId;
}

/**
 * 按模块筛的 chip 组：只列列表里出现过的模块（没历史的模块列出来也点不出东西），首个「全部」清筛。
 * 用 Button 而不是 Tag：它是筛选动作不是标签，要能聚焦、要有按下态（aria-pressed）。
 */
export function ModuleChips({
  moduleIds,
  value,
  onChange,
}: {
  moduleIds: string[];
  value: string | null;
  onChange: (moduleId: string | null) => void;
}) {
  if (moduleIds.length === 0) return null;
  return (
    <div className="flex items-center gap-1" role="group" aria-label="按模块筛选">
      <Button
        type="button"
        size="sm"
        variant={value === null ? 'default' : 'outline'}
        aria-pressed={value === null}
        onClick={() => onChange(null)}
      >
        全部
      </Button>
      {moduleIds.map((moduleId) => (
        <Button
          key={moduleId}
          type="button"
          size="sm"
          variant={value === moduleId ? 'default' : 'outline'}
          aria-pressed={value === moduleId}
          onClick={() => onChange(value === moduleId ? null : moduleId)}
        >
          {moduleTitleOf(moduleId)}
        </Button>
      ))}
    </div>
  );
}

/**
 * 时刻格：显本地墙钟带显式偏移（moment.ts 按装配点配置的区算），原 RFC 3339 串放进 dateTime 与 title，悬停给线格式那一份。
 * 与 pages/party/detail-primitives 的 Instant 同形；不从那边 import——「我的工作」不该反向依赖某个业务模块的页面件。
 */
export function InstantCell({ value }: { value: string }) {
  return (
    <time dateTime={value} title={value} className="font-mono text-[12px]">
      {formatInstant(value)}
    </time>
  );
}
