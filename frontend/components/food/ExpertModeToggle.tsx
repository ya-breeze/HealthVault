'use client';
import { useLanguage } from '@/components/LanguageContext';

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
    <label className="flex items-center gap-2 text-xs font-medium text-gray-500 dark:text-gray-400 select-none">
      <input
        type="checkbox"
        checked={checked}
        onChange={e => onChange(e.target.checked)}
        className="h-4 w-4"
      />
      {t('expertMode.toggle')}
    </label>
  );
}
