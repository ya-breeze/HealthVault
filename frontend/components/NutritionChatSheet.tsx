'use client';
import { useEffect, useRef, useState } from 'react';
import Link from 'next/link';
import {
  api,
  type NutritionAdviceWindow,
  type NutritionChatSignal,
  type NutritionChatSource,
  type NutritionChatTurn,
} from '@/lib/api';
import type { HealthinessResult } from '@/lib/healthiness';
import TapTarget from '@/components/ui/TapTarget';
import { useLanguage } from '@/components/LanguageContext';
import { dateLocaleFor, interpolate, macroSourceLabel, numberLocaleFor } from '@/lib/i18n';

/** The server's own limits, restated so the input can stop the user at them. */
const QUESTION_MAX_LENGTH = 500;
const MAX_TURNS = 8;

type DisplayTurn = NutritionChatTurn & { sources?: NutritionChatSource[] };

/**
 * The sheet the nutrition card's discuss control opens: the advice's measured
 * basis, and a conversation about it.
 *
 * Every turn lives in this component's state and nowhere else — no
 * `localStorage`, no `sessionStorage`, no persistence call. Closing the sheet
 * unmounts it and the conversation is gone, which is the whole storage model
 * (docs/specs/nutrition-chat.md, "The history lives in the tab and nowhere
 * else"). Nothing here may be changed to remember a turn without revisiting
 * that decision: these are medical conversations.
 *
 * Mounted only while open, the way MoreSheet is, so its controls stay out of
 * the accessibility tree and out of test locators while the card is idle.
 */
export default function NutritionChatSheet({
  healthiness,
  window: adviceWindow,
  onClose,
}: {
  healthiness: HealthinessResult;
  window: NutritionAdviceWindow;
  onClose: () => void;
}) {
  const { t, language } = useLanguage();
  const panelRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const logRef = useRef<HTMLDivElement>(null);
  const pressedBackdrop = useRef(false);
  const [turns, setTurns] = useState<DisplayTurn[]>([]);
  const [question, setQuestion] = useState('');
  const [pending, setPending] = useState(false);
  const [failed, setFailed] = useState(false);

  // Escape closes; Tab is confined to the panel. Both live on the document
  // rather than the panel because focus can legitimately be on the body at
  // the moment the sheet mounts — the same reasoning MoreSheet documents.
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
        return;
      }
      if (e.key !== 'Tab' || !panelRef.current) return;
      const focusable = panelRef.current.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), input:not([disabled]), [tabindex]:not([tabindex="-1"])'
      );
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (e.shiftKey && (document.activeElement === first || !panelRef.current.contains(document.activeElement))) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && (document.activeElement === last || !panelRef.current.contains(document.activeElement))) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [onClose]);

  useEffect(() => {
    const previous = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = previous;
    };
  }, []);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  // Keep the newest turn in view as the conversation grows.
  useEffect(() => {
    if (logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight;
  }, [turns, pending]);

  const atTurnLimit = turns.length >= MAX_TURNS;

  async function send() {
    const asked = question.trim();
    if (!asked || pending || atTurnLimit) return;
    // The question joins the log before the answer arrives, so the user can
    // see what they asked while they wait.
    const nextTurns: NutritionChatTurn[] = [...turns, { role: 'user', text: asked }];
    setTurns(nextTurns);
    setQuestion('');
    setPending(true);
    setFailed(false);
    try {
      const response = await api.postNutritionChat({
        label: healthiness.label,
        reasons: healthiness.reasons,
        window: adviceWindow,
        signals: healthiness.signals.map(
          (signal): NutritionChatSignal => ({
            code: signal.code,
            value: signal.value,
            unit: signal.unit,
            verdict: signal.verdict,
            ...(signal.reason ? { reason: signal.reason } : {}),
            off_boundary: signal.offBoundary,
            ...(signal.farBoundary !== null ? { far_boundary: signal.farBoundary } : {}),
          })
        ),
        eligible_days: healthiness.eligibleDays,
        // Sources are display-only evidence from prior server tool calls. The
        // model replays only the words that were already on screen.
        turns: turns.map(({ role, text }) => ({ role, text })),
        question: asked,
      });
      if (response.available) {
        setTurns([
          ...nextTurns,
          { role: 'assistant', text: response.answer, sources: response.sources ?? [] },
        ]);
      } else {
        setFailed(true);
      }
    } catch {
      setFailed(true);
    } finally {
      setPending(false);
    }
  }

  function formatValue(signal: HealthinessResult['signals'][number], value: number) {
    return signal.unit === 'share'
      ? `${Math.round(value * 100)}%`
      : interpolate(t('nutritionChat.gramsPerDay'), { value: value.toFixed(1) });
  }

  // Which side of its band the signal fell on. A two-sided signal can be
  // flagged for being too low as well as too high, and saying "above" for a
  // low-protein finding states the opposite of what was measured. The stored
  // boundary is already the pair on the side the value fell, so comparing
  // against it answers the question for one-sided signals too.
  function direction(signal: HealthinessResult['signals'][number]) {
    return signal.value < signal.offBoundary ? 'below' : 'above';
  }

  function sourceSignalLabel(signal: string) {
    switch (signal) {
      case 'protein': return t('nutritionChat.signal.protein');
      case 'carbs': return t('nutritionChat.signal.carbs');
      case 'fat': return t('nutritionChat.signal.fat');
      case 'sugar': return t('nutritionChat.signal.sugar');
      case 'sodium': return t('nutritionChat.signal.sodium');
      case 'fiber': return t('nutritionChat.signal.fiber');
      default: return signal;
    }
  }

  function sourceDate(date: string) {
    return new Date(`${date}T12:00:00Z`).toLocaleDateString(dateLocaleFor(language), {
      day: 'numeric',
      month: 'short',
      timeZone: 'UTC',
    });
  }

  const flagged = healthiness.signals.filter(signal => signal.reason !== null);

  return (
    <div
      data-testid="nutrition-chat-backdrop"
      className="fixed inset-0 z-50 bg-black/50 flex items-end"
      // Dismiss only for a press *and* release both on the backdrop, so a
      // drag that starts on selected answer text and ends outside the panel
      // cannot close the sheet mid-selection. MoreSheet documents the case.
      onPointerDown={e => {
        pressedBackdrop.current = e.target === e.currentTarget;
      }}
      onClick={e => {
        if (pressedBackdrop.current && e.target === e.currentTarget) onClose();
      }}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-label={t('nutritionChat.title')}
        data-testid="nutrition-chat-sheet"
        className="w-full bg-bg-elevated border-t border-border rounded-t-2xl px-4 pt-4 pb-[calc(1rem+env(safe-area-inset-bottom))] max-h-[85vh] overflow-y-auto flex flex-col gap-3"
      >
        <div className="flex items-center justify-between">
          <p className="text-sm font-semibold text-text">{t('nutritionChat.title')}</p>
          <TapTarget
            onClick={onClose}
            aria-label={t('nav.close')}
            data-testid="nutrition-chat-close"
            className="flex items-center justify-center text-text-muted hover:text-text text-xl leading-none"
          >
            &times;
          </TapTarget>
        </div>

        <div className="space-y-1" data-testid="nutrition-chat-basis">
          <p className="text-xs font-medium text-text">{t('nutritionChat.basisTitle')}</p>
          {flagged.length === 0 ? (
            <p className="text-xs text-text-muted" data-testid="nutrition-chat-basis-none">
              {t('nutritionChat.basisNothingFlagged')}
            </p>
          ) : (
            flagged.map(signal => (
              <p key={signal.code} className="text-xs text-text-muted" data-testid="nutrition-chat-basis-row">
                {interpolate(t(`nutritionChat.basis.${signal.verdict === 'far' ? 'far' : 'off'}.${direction(signal)}`), {
                  signal: t(`nutritionChat.signal.${signal.code}`),
                  value: formatValue(signal, signal.value),
                  // The guideline is where the signal stopped being ok, and
                  // farThreshold is the boundary a `far` verdict itself
                  // crossed. A far row names both, so the number that produced
                  // its verdict is on screen rather than only in the request.
                  threshold: formatValue(signal, signal.offBoundary),
                  farThreshold: signal.farBoundary === null ? '' : formatValue(signal, signal.farBoundary),
                })}
              </p>
            ))
          )}
          <p className="text-xs text-text-muted" data-testid="nutrition-chat-basis-days">
            {interpolate(t('nutritionChat.basisDays'), { days: healthiness.eligibleDays })}
          </p>
        </div>

        <div
          ref={logRef}
          className="flex flex-col gap-2 max-h-[40vh] overflow-y-auto"
          data-testid="nutrition-chat-log"
        >
          {turns.map((turn, index) => (
            <div
              key={`${index}:${turn.role}`}
              className={
                turn.role === 'user'
                  ? 'self-end max-w-[85%]'
                  : 'self-start w-full max-w-[92%]'
              }
            >
              <p
                data-testid={`nutrition-chat-${turn.role}`}
                className={
                  turn.role === 'user'
                    ? 'rounded-lg bg-accent/15 px-3 py-2 text-sm text-text'
                    : 'rounded-lg bg-bg px-3 py-2 text-sm text-text'
                }
              >
                {turn.text}
              </p>
              {turn.role === 'assistant' && turn.sources && turn.sources.length > 0 && (
                <div className="mt-2 space-y-1" data-testid="nutrition-chat-sources">
                  <p className="px-1 text-[11px] font-medium text-text-muted">
                    {t('nutritionChat.sourcesTitle')}
                  </p>
                  {turn.sources.map((source, sourceIndex) => (
                    <Link
                      key={`${source.meal_id}:${source.signal}:${sourceIndex}`}
                      href={`/food/review/?meal=${encodeURIComponent(source.meal_id)}`}
                      aria-label={interpolate(t('nutritionChat.openMeal'), {
                        food: source.food,
                        date: sourceDate(source.date),
                      })}
                      data-testid="nutrition-chat-source"
                      className="flex min-h-12 w-full items-center justify-between gap-3 rounded-lg border border-border bg-bg-elevated px-3 py-2 text-left hover:border-accent"
                    >
                      <span className="min-w-0">
                        <span className="block truncate text-xs font-medium text-text">{source.food}</span>
                        <span className="block text-[11px] text-text-muted tabular-nums">
                          {interpolate(t('nutritionChat.sourceContribution'), {
                            signal: sourceSignalLabel(source.signal),
                            value: source.nutrient_grams.toLocaleString(numberLocaleFor(language), {
                              minimumFractionDigits: 0,
                              maximumFractionDigits: 2,
                            }),
                            date: sourceDate(source.date),
                          })}
                        </span>
                      </span>
                      <span className="flex shrink-0 flex-col items-end gap-0.5">
                        <span className="rounded bg-bg px-1.5 py-0.5 text-[10px] text-text-muted">
                          {macroSourceLabel(t, source.macro_source)}
                        </span>
                        {source.macro_source === 'estimated' && source.confidence > 0 && (
                          <span className="text-[10px] text-text-muted">
                            {interpolate(t('nutritionChat.sourceConfidence'), {
                              value: Math.round(source.confidence * 100),
                            })}
                          </span>
                        )}
                      </span>
                    </Link>
                  ))}
                </div>
              )}
            </div>
          ))}
          {pending && (
            <p className="self-start text-xs text-text-muted" data-testid="nutrition-chat-pending">
              {t('nutritionChat.thinking')}
            </p>
          )}
        </div>

        {failed && (
          <p className="text-xs text-text-muted" data-testid="nutrition-chat-error">
            {t('nutritionChat.unavailable')}
          </p>
        )}
        {atTurnLimit && (
          <p className="text-xs text-text-muted" data-testid="nutrition-chat-turn-limit">
            {t('nutritionChat.turnLimit')}
          </p>
        )}

        <div className="flex items-center gap-2">
          <input
            ref={inputRef}
            type="text"
            value={question}
            maxLength={QUESTION_MAX_LENGTH}
            disabled={pending || atTurnLimit}
            onChange={e => setQuestion(e.target.value)}
            onKeyDown={e => {
              if (e.key === 'Enter') void send();
            }}
            placeholder={t('nutritionChat.placeholder')}
            aria-label={t('nutritionChat.inputLabel')}
            data-testid="nutrition-chat-input"
            className="flex-1 min-w-0 rounded-lg border border-border bg-bg px-3 py-2 text-sm text-text disabled:opacity-50"
          />
          <TapTarget
            onClick={() => void send()}
            disabled={pending || atTurnLimit || question.trim() === ''}
            data-testid="nutrition-chat-send"
            className="rounded-lg bg-accent px-3 text-sm font-medium text-bg-elevated disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {t('nutritionChat.send')}
          </TapTarget>
        </div>
        <p className="text-xs text-text-muted" data-testid="nutrition-chat-ephemeral-note">
          {t('nutritionChat.ephemeralNote')}
        </p>
      </div>
    </div>
  );
}
