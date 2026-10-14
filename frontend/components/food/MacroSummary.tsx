import { FoodMeal } from '@/lib/api';
import { useLanguage } from '@/components/LanguageContext';

// Pending meals do not contribute to logged totals. Unknown fiber stays unknown.
export default function MacroSummary({ meal }: { meal: FoodMeal }) {
  const { t } = useLanguage();
  const isFinal = meal.status === 'confirmed';
  const nutrients = [
    [t('unit.kcal'), meal.calories],
    [t('resolver.protein'), meal.protein_grams],
    [t('resolver.carbs'), meal.carbs_grams],
    [t('resolver.fat'), meal.fat_grams],
    [t('resolver.fiber'), meal.dietary_fiber_grams],
  ] as const;
  return (
    <section className="hv-card" data-testid="meal-nutrition-summary">
      <h2 className="text-lg font-semibold text-text">{t('redesign.review.mealNutrition')}</h2>
      <p className="text-xs text-text-muted mt-2 mb-5">
        {isFinal
          ? t('redesign.review.loggedTotalsUpdateAsYouEditItems')
          : t('redesign.review.totalsWillBeCalculatedWhenYouConfirm')}
      </p>
      <div className="grid grid-cols-2 gap-3">
        {nutrients.map(([label, value], index) => (
          <div key={label} className={`rounded-xl p-4 ${index === 4 ? 'bg-accent/10 col-span-2' : 'bg-bg'}`}>
            <p className="text-xs text-text-muted">{label}</p>
            <p className="text-2xl font-semibold text-text tabular-nums mt-2" data-testid={index === 4 ? 'meal-total-fiber' : undefined}>
              {isFinal && value != null ? (index === 4 ? value.toFixed(1) : Math.round(value)) : '—'}
            </p>
          </div>
        ))}
      </div>
    </section>
  );
}
