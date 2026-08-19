'use client';
import Link from 'next/link';
import type { DataType } from '@/lib/api';
import { metricColorVar } from '@/lib/tokens';
import type { VitalResult } from '@/lib/vitals';
import TapTarget from './ui/TapTarget';

function sparkPath(data: number[], w: number, h: number, pad: number) {
  const min = Math.min(...data);
  const max = Math.max(...data);
  const range = max - min || 1;
  const step = data.length > 1 ? (w - pad * 2) / (data.length - 1) : 0;
  const pts = data.map((v, i) => [pad + i * step, pad + (h - pad * 2) * (1 - (v - min) / range)] as const);
  const d = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${p[0].toFixed(1)},${p[1].toFixed(1)}`).join(' ');
  return { d, last: pts[pts.length - 1] };
}

const TREND_ARROW: Record<VitalResult['trend'], string> = { up: '↑', down: '↓', flat: '→' };

interface VitalCardProps {
  type: DataType;
  label: string;
  result: VitalResult | null;
  // When set, the card renders move-up/move-down controls instead of
  // navigating to the type's detail page — see the "Customizable vitals grid
  // order" requirement's "Entering edit mode reveals reorder controls" scenario.
  editing?: boolean;
  onMoveUp?: () => void;
  onMoveDown?: () => void;
  moveUpDisabled?: boolean;
  moveDownDisabled?: boolean;
}

export default function VitalCard({
  type, label, result, editing, onMoveUp, onMoveDown, moveUpDisabled, moveDownDisabled,
}: VitalCardProps) {
  const color = metricColorVar(type);
  const w = 240;
  const h = 30;
  const pad = 3;
  const spark = result && result.sparkline.length > 1 ? sparkPath(result.sparkline, w, h, pad) : null;

  const inner = (
    <>
      <div className="flex items-center justify-between gap-1 mb-2">
        <p className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide flex items-center gap-1.5">
          <span className="w-1.5 h-1.5 rounded-full" style={{ background: color }} />
          {label}
        </p>
      </div>
      {editing && (
        // Own row, not squeezed beside the label: at full 48px (TapTarget's
        // enforced minimum tap target) two buttons don't fit next to a label
        // in a 2-column mobile grid, so they get the width to themselves.
        <div className="flex items-center justify-end gap-1.5 mb-2">
          <TapTarget
            onClick={onMoveUp}
            disabled={moveUpDisabled}
            aria-label={`Move ${label} up`}
            className="flex items-center justify-center rounded-md border border-border bg-bg text-text disabled:opacity-30 disabled:cursor-not-allowed"
          >
            ↑
          </TapTarget>
          <TapTarget
            onClick={onMoveDown}
            disabled={moveDownDisabled}
            aria-label={`Move ${label} down`}
            className="flex items-center justify-center rounded-md border border-border bg-bg text-text disabled:opacity-30 disabled:cursor-not-allowed"
          >
            ↓
          </TapTarget>
        </div>
      )}
      {result ? (
        <>
          <div className="font-[family-name:var(--font-data)] text-xl font-bold tabular-nums">
            {result.value}
            {result.unit && <span className="text-xs font-medium text-text-muted ml-1">{result.unit}</span>}
          </div>
          <div className="font-[family-name:var(--font-data)] text-[11px] font-bold mt-0.5">
            {TREND_ARROW[result.trend]} 7d trend
          </div>
          {spark && (
            <svg viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none" className="w-full h-[30px] mt-2 block">
              <path
                d={`${spark.d} L${(w - pad).toFixed(1)},${(h - pad).toFixed(1)} L${pad},${(h - pad).toFixed(1)} Z`}
                fill={color}
                fillOpacity={0.18}
                stroke="none"
              />
              <path d={spark.d} fill="none" stroke={color} strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" />
              <circle cx={spark.last[0].toFixed(1)} cy={spark.last[1].toFixed(1)} r={2.6} fill={color} />
            </svg>
          )}
        </>
      ) : (
        <p className="text-sm text-text-muted py-2">No data</p>
      )}
    </>
  );

  const commonProps = {
    className: 'block bg-bg-elevated border border-border rounded-[10px] px-3.5 py-3 hover:border-accent transition-colors',
    style: { color },
    'data-testid': `vital-card-${type}`,
  };

  if (editing) {
    return <div {...commonProps}>{inner}</div>;
  }

  return (
    <Link href={`/data/${type}/`} {...commonProps}>
      {inner}
    </Link>
  );
}
