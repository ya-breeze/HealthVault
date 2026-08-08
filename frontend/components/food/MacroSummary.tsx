import { FoodMeal } from '@/lib/api';

// Displays the meal's own aggregate columns. Confirmed meals show a real
// total; pending_review meals show all-zero (or partial, once items are
// resolved by the user) since the aggregate is computed only on confirm —
// see FoodMeal.Aggregate, called from PUT .../confirm, not from analysis.
export default function MacroSummary({ meal }: { meal: FoodMeal }) {
  const isFinal = meal.status === 'confirmed';
  return (
    <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-100 dark:border-gray-700 p-4">
      <p className="text-xs font-medium text-gray-500 dark:text-gray-400 mb-2">
        {isFinal ? 'Logged totals' : 'Totals will be calculated when you confirm'}
      </p>
      <div className="grid grid-cols-4 gap-3 text-center">
        <div>
          <p className="text-lg font-bold text-gray-900 dark:text-white">
            {isFinal ? Math.round(meal.calories) : '—'}
          </p>
          <p className="text-[11px] text-gray-400">kcal</p>
        </div>
        <div>
          <p className="text-lg font-bold text-gray-900 dark:text-white">
            {isFinal ? meal.protein_grams.toFixed(0) : '—'}
          </p>
          <p className="text-[11px] text-gray-400">protein g</p>
        </div>
        <div>
          <p className="text-lg font-bold text-gray-900 dark:text-white">
            {isFinal ? meal.carbs_grams.toFixed(0) : '—'}
          </p>
          <p className="text-[11px] text-gray-400">carbs g</p>
        </div>
        <div>
          <p className="text-lg font-bold text-gray-900 dark:text-white">
            {isFinal ? meal.fat_grams.toFixed(0) : '—'}
          </p>
          <p className="text-[11px] text-gray-400">fat g</p>
        </div>
      </div>
    </div>
  );
}
