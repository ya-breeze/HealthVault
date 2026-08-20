'use client';
import { useLanguage } from '@/components/LanguageContext';
import TapTarget from '@/components/ui/TapTarget';

interface Props {
  checked: boolean;
  onChange: (checked: boolean) => void;
}

// Per-screen, non-persisted toggle that reveals a Food Item's or Custom
// Food's Canonical Name alongside its Display Name. Deliberately plain
// React state owned by each screen (see design.md decision 7) — reloading
// the page resets it to off, and it is never written to UserSettings or a
// URL param. See openspec/specs/expert-mode "Per-Screen Expert Mode Toggle".
export default function ExpertModeToggle({ checked, onChange }: Props) {
  const { t } = useLanguage();
  return (
    // TapTarget as a <label>, not a bare one: this toggle ships on the review
    // and custom-foods screens, both covered by
    // openspec/specs/mobile-touch-targets "Minimum Tap Target Size", and a
    // 16px checkbox inside a text-height label is far under the 48x48
    // minimum. Clicking anywhere in a label toggles its checkbox, so sizing
    // the label is what actually enlarges the tap target. Found in code
    // review.
    <TapTarget
      as="label"
      className="inline-flex items-center gap-2 text-xs font-medium text-gray-500 dark:text-gray-400 select-none cursor-pointer"
    >
      <input
        type="checkbox"
        checked={checked}
        onChange={e => onChange(e.target.checked)}
        className="h-4 w-4"
        // Stable, wording-independent hook for e2e tests: this toggle's
        // accessible name is free text (see the en.ts translation's doc
        // comment) and has already collided once with another field's
        // getByLabel('Name') locator on the same page. A future label
        // change should not have to worry about re-colliding with something
        // else — tests should target this by data-testid, not by label
        // text.
        data-testid="expert-mode-toggle"
      />
      {t('expertMode.toggle')}
    </TapTarget>
  );
}
