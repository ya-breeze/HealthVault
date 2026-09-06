'use client';
import Link from 'next/link';
import type { DataType } from '@/lib/api';
import { metricColorVar } from '@/lib/tokens';
import type { VitalResult } from '@/lib/vitals';
import { useLanguage } from './LanguageContext';
import { interpolate } from '@/lib/i18n';
import TapTarget from './ui/TapTarget';
import { EyeIcon, EyeOffIcon } from './icons';

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

// Calendar-date equality, not timestamp equality: `asOf` is a bucket_start
// (always midnight UTC — see bucketExpr in storage_impl.go), so comparing
// getTime() against "now" would read as "not today" for the rest of every
// day everywhere east of UTC. Comparing the local calendar date each instant
// converts to is what "today" means to the person reading the card.
function isToday(iso: string): boolean {
  return new Date(iso).toDateString() === new Date().toDateString();
}

interface VitalCardProps {
  type: DataType;
  label: string;
  result: VitalResult | null;
  // When set, the card renders move-up/move-down and show/hide controls
  // instead of navigating to the type's detail page — see the "Customizable
  // vitals grid order and visibility" requirement's "Entering edit mode
  // reveals reorder controls" scenario.
  editing?: boolean;
  onMoveUp?: () => void;
  onMoveDown?: () => void;
  moveUpDisabled?: boolean;
  moveDownDisabled?: boolean;
  // Whether the user has hidden this card. Only ever true while `editing`:
  // hidden cards are filtered out of the read-only grid by the dashboard, and
  // rendered here (dimmed) only so they can be found and shown again.
  hidden?: boolean;
  onToggleHidden?: () => void;
  // Set while the Done save is in flight. handleDone closes over the `order`
  // of the render that created it, so an edit made mid-save is never
  // persisted, yet the grid re-renders from the newer local state on success
  // — leaving the screen disagreeing with the server until a reload. Locking
  // the controls for the duration is what keeps the two in step. Found in
  // code review.
  controlsDisabled?: boolean;
}

export default function VitalCard({
  type, label, result, editing, onMoveUp, onMoveDown, moveUpDisabled, moveDownDisabled,
  hidden, onToggleHidden, controlsDisabled,
}: VitalCardProps) {
  const { t } = useLanguage();
  const color = metricColorVar(type);
  const w = 240;
  const h = 30;
  const pad = 3;
  const spark = result && result.sparkline.length > 1 ? sparkPath(result.sparkline, w, h, pad) : null;

  // Fades a hidden card's label and readings in edit mode so it's obvious at a
  // glance which cards won't show on the dashboard. Applied per-section rather
  // than to the whole card, so the show/hide control itself stays legible.
  const dim = editing && hidden ? ' opacity-40' : '';

  const inner = (
    <>
      <div className={`flex items-center justify-between gap-1 mb-2${dim}`}>
        <p className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide flex items-center gap-1.5">
          <span className="w-1.5 h-1.5 rounded-full" style={{ background: color }} />
          {label}
        </p>
      </div>
      {editing && (
        // Own row, not squeezed beside the label: at full 48px (TapTarget's
        // enforced minimum tap target) two buttons don't fit next to a label
        // in a 2-column mobile grid, so they get the width to themselves.
        //
        // flex-wrap because there are three of them now: 3 × 48px plus gaps
        // exceeds a mobile grid cell's inner width (~121px at 360px viewport),
        // so without wrapping the third control would overflow the card.
        //
        // The show/hide toggle comes last precisely because of that wrap: at
        // mobile width the row breaks after two controls, and putting the
        // toggle first split the ↑/↓ pair across two lines. Last, it takes the
        // orphaned second line itself and the arrows stay together.
        <div className="flex flex-wrap items-center justify-end gap-1.5 mb-2">
          <TapTarget
            onClick={onMoveUp}
            disabled={moveUpDisabled || controlsDisabled}
            aria-label={interpolate(t('vitals.moveUp'), { metric: label })}
            className="flex items-center justify-center rounded-md border border-border bg-bg text-text disabled:opacity-30 disabled:cursor-not-allowed"
          >
            ↑
          </TapTarget>
          <TapTarget
            onClick={onMoveDown}
            disabled={moveDownDisabled || controlsDisabled}
            aria-label={interpolate(t('vitals.moveDown'), { metric: label })}
            className="flex items-center justify-center rounded-md border border-border bg-bg text-text disabled:opacity-30 disabled:cursor-not-allowed"
          >
            ↓
          </TapTarget>
          <TapTarget
            onClick={onToggleHidden}
            disabled={controlsDisabled}
            aria-label={interpolate(t(hidden ? 'dashboard.showCard' : 'dashboard.hideCard'), { metric: label })}
            data-testid={`vital-card-${type}-visibility`}
            // No aria-pressed: the label is an action ("Hide Sleep" / "Show
            // Sleep") and already carries the state, so a pressed flag on top
            // of it would read as contradictory ("Show Sleep, pressed").
            className="flex items-center justify-center rounded-md border border-border bg-bg text-text disabled:opacity-30 disabled:cursor-not-allowed"
          >
            {hidden ? <EyeOffIcon className="w-4 h-4" /> : <EyeIcon className="w-4 h-4" />}
          </TapTarget>
        </div>
      )}
      {result ? (
        <div className={dim || undefined}>
          <div className="font-[family-name:var(--font-data)] text-xl font-bold tabular-nums">
            {result.value}
            {result.unit && <span className="text-xs font-medium text-text-muted ml-1">{t(result.unit)}</span>}
          </div>
          <div className="font-[family-name:var(--font-data)] text-[11px] font-bold mt-0.5">
            {TREND_ARROW[result.trend]} {t('vitals.trend7d')}
          </div>
          {result.asOf && !isToday(result.asOf) && (
            <p className="text-[11px] text-text-muted mt-0.5" data-testid={`vital-card-${type}-as-of`}>
              {interpolate(t('vitals.asOf'), { date: new Date(result.asOf).toLocaleDateString() })}
            </p>
          )}
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
        </div>
      ) : (
        <p className={`text-sm text-text-muted py-2${dim}`}>{t('vitals.noData')}</p>
      )}
    </>
  );

  const commonProps = {
    className: 'block bg-bg-elevated border border-border rounded-[10px] px-3.5 py-3 hover:border-accent transition-colors',
    style: { color },
    'data-testid': `vital-card-${type}`,
  };

  if (editing) {
    // `data-hidden` gives e2e a styling-independent way to assert the state;
    // the dimming itself is applied inside `inner` (see `dim`), so that the
    // controls stay at full opacity — fading the button that un-hides the
    // card would work against the reason it's still rendered at all.
    return (
      <div {...commonProps} data-hidden={hidden ? 'true' : 'false'}>
        {inner}
      </div>
    );
  }

  return (
    <Link href={`/data/${type}/`} {...commonProps}>
      {inner}
    </Link>
  );
}
