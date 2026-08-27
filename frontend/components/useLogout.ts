'use client';
import { useRouter } from 'next/navigation';
import { api } from '@/lib/api';
import { clearSession } from '@/lib/session';
import { useLanguage } from '@/components/LanguageContext';
import { useToast } from '@/components/Toast';

/**
 * The logout action, shared by the header's control and the More sheet's so
 * the two cannot diverge — the same reasoning that put the copy control's
 * behaviour in `useCopyToClipboard`.
 *
 * Reports failure rather than letting it become an unhandled rejection.
 * `api.logout` throws `ApiError` on any non-2xx, and both call sites await it
 * before routing, so a 500 or a dropped connection used to leave the user on
 * the page with no indication that anything had happened — and, in the
 * sheet's case, still looking at the button they had just pressed. `useToast`
 * is how every other user-action failure in this app is surfaced.
 */
export function useLogout(): () => Promise<void> {
  const router = useRouter();
  const { t } = useLanguage();
  const { showToast } = useToast();

  return async () => {
    try {
      await api.logout();
    } catch {
      showToast(t('header.logoutFailed'), 'error');
      return;
    }
    // Only after the server has actually ended the session: leaving the cache
    // populated on a failed logout keeps the chrome rendering from the
    // session the user still has.
    clearSession();
    router.push('/login');
  };
}
