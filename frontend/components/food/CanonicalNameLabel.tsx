'use client';
import { useLanguage } from '@/components/LanguageContext';

// Expert Mode's canonical-name sub-label — shared by MealItemRow and the
// custom-food catalog page so the two don't hand-roll independent copies
// that can drift (they already had, before this: one had `break-words`, the
// other didn't). Renders nothing when Expert Mode is off or there's no
// Canonical Name to show, matching how both call sites previously guarded
// this inline.
export default function CanonicalNameLabel({
  expertMode,
  canonicalName,
}: {
  expertMode: boolean;
  canonicalName?: string | null;
}) {
  const { t } = useLanguage();
  if (!expertMode || !canonicalName) return null;
  // A block-displayed <span>, not a <p>: ItemResolver renders this inside a
  // candidate row that is itself a <button>, whose content model is phrasing
  // content only — a <p> there is invalid nesting the parser would restructure
  // on hydration. `block` reproduces exactly what the <p> looked like, since
  // Tailwind's preflight already zeroes paragraph margins.
  return (
    <span className="block text-xs text-gray-400 dark:text-gray-500 italic break-words">
      {t('expertMode.canonicalNamePrefix')} {canonicalName}
    </span>
  );
}
