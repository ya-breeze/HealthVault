'use client';
import type { CSSProperties } from 'react';
import type { TodaySummary } from '@/lib/api';
import { useLanguage } from './LanguageContext';
import { interpolate, numberLocaleFor } from '@/lib/i18n';
import TapTarget from './ui/TapTarget';
import { InfoIcon } from './icons';

/** Read the same computed summary as Android. Unknown fiber stays unknown. */
export default function TodayNutritionSummary({summary, onToggleHint, hintOpen, hintId}: {
  summary: TodaySummary; onToggleHint?: () => void; hintOpen?: boolean; hintId?: string;
}) {
  const {t, language} = useLanguage();
  const target = summary.target;
  const number = (value: number) => value.toLocaleString(numberLocaleFor(language), {maximumFractionDigits: 1});
  const progress = (consumed: number, goal: number) => goal > 0 ? Math.min(100, Math.max(0, consumed / goal * 100)) : 0;
  const calorieText = target.available
    ? interpolate(t('loggingGap.todayCalories'), {consumed: String(Math.round(summary.calories_consumed)), target: String(Math.round(target.calories))})
    : `${number(summary.calories_consumed)} ${t('unit.kcal')}`;
  const fiber = summary.dietary_fiber_grams_consumed;
  const fiberKnown = typeof fiber === 'number' && Number.isFinite(fiber);
  const fiberGoal = target.dietary_fiber_grams;
  const fiberGoalKnown = typeof fiberGoal === 'number' && Number.isFinite(fiberGoal) && fiberGoal > 0;
  const macros = [
    {key: 'protein' as const, consumed: summary.protein_grams_consumed, goal: target.available ? target.protein_grams : null},
    {key: 'fat' as const, consumed: summary.fat_grams_consumed, goal: target.available ? target.fat_grams : null},
    {key: 'carbs' as const, consumed: summary.carbs_grams_consumed, goal: target.available ? target.carbs_grams : null},
  ];
  return <div data-testid="nutrition-today">
    <div className="hv-nutrition-energy">
      <div className="hv-calorie-ring" style={{'--intake-progress': `${target.available ? progress(summary.calories_consumed,target.calories) : 0}%`} as CSSProperties} aria-hidden="true"><div><strong>{Math.round(summary.calories_consumed).toLocaleString(numberLocaleFor(language))}</strong><small>{t('unit.kcal')}</small></div></div>
      <div className="hv-nutrition-target">
        {onToggleHint ? <TapTarget compactOnMouse onClick={onToggleHint} aria-expanded={hintOpen} aria-controls={hintId} data-testid="nutrition-today-hint-toggle" className="flex items-center justify-between gap-2 text-left"><span className="text-xl font-bold" data-testid="nutrition-today-calories">{calorieText}</span><span className="sr-only">{t('loggingGap.hintToggle')}</span><InfoIcon className="w-4 h-4 shrink-0" aria-hidden="true"/></TapTarget> : <p className="text-xl font-bold" data-testid="nutrition-today-calories">{calorieText}</p>}
        <p className="hv-nutrition-caption">{t('redesign.nutrition.confirmedOnly')}</p>
      </div>
    </div>
    <div className="hv-nutrients" data-testid="nutrition-today-macros">
      {macros.map(({key,consumed,goal})=><div key={key} className="hv-nutrient"><span>{goal!==null?interpolate(t(`loggingGap.${key}`),{consumed:String(Math.round(consumed)),target:String(Math.round(goal))}):`${t(`redesign.nutrition.${key}`)} ${number(consumed)} ${t('unit.grams')}`}</span>{goal!==null&&<div className="hv-nutrient-bar" aria-hidden="true"><span style={{width:`${progress(consumed,goal)}%`}}/></div>}</div>)}
      <div className="hv-nutrient fiber" data-testid="nutrition-today-fiber"><span>{t('item.fiber')} {fiberKnown?number(fiber):'—'}{fiberGoalKnown?` / ${number(fiberGoal)}`:''} {t('unit.grams')}</span>{fiberKnown&&fiberGoalKnown&&<div className="hv-nutrient-bar" aria-hidden="true"><span style={{width:`${progress(fiber,fiberGoal)}%`}}/></div>}</div>
    </div>
  </div>;
}
