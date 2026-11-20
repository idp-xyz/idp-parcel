import type { PriceCardOverlap } from './api';
import { priceCardOverlapLines } from './price-card-overlap';

/** 琥珀色提示：适用期重叠不阻止保存、批准或发布。没有重叠就不占一行。 */
export function PriceCardOverlapNotice({ overlaps }: { overlaps?: readonly PriceCardOverlap[] }) {
  const lines = priceCardOverlapLines(overlaps);
  if (lines.length === 0) return null;
  return (
    <div
      role="note"
      aria-label="适用期重叠"
      className="rounded border border-idpxyz-border border-l-2 border-l-[color:var(--ds-status-warning-fg)] px-3 py-2 type-body text-idpxyz-text"
    >
      <ul className="space-y-1">
        {lines.map((line) => (
          <li key={line.key}>{line.text}</li>
        ))}
      </ul>
    </div>
  );
}
