'use client';
import { useCallback, useEffect, useRef, useState } from 'react';

const COPIED_RESET_MS = 2000;

/**
 * Copy-to-clipboard with a transient "copied" flag.
 *
 * Lifted out of Header.tsx when the More sheet gained a second copy control
 * for the same webhook URL: this is ~20 lines of branching (async Clipboard
 * API, `execCommand` fallback for the insecure-origin case, timed flag
 * reset) that must behave identically on both surfaces, and two copies of it
 * would be free to drift.
 *
 * The fallback matters here rather than being defensive boilerplate:
 * `navigator.clipboard` is undefined on a non-secure origin, and this app is
 * reached over plain HTTP on the LAN.
 */
export function useCopyToClipboard(): {
  copied: boolean;
  copy: (text: string) => void;
} {
  const [copied, setCopied] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Clearing on unmount keeps a copy immediately before a navigation from
  // calling setState on an unmounted component.
  useEffect(() => () => {
    if (timerRef.current) clearTimeout(timerRef.current);
  }, []);

  const flagCopied = useCallback(() => {
    setCopied(true);
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => setCopied(false), COPIED_RESET_MS);
  }, []);

  const copy = useCallback((text: string) => {
    const execCommandCopy = () => {
      const el = document.createElement('input');
      el.value = text;
      el.style.cssText = 'position:fixed;opacity:0;top:0;left:0';
      document.body.appendChild(el);
      el.focus();
      el.select();
      document.execCommand('copy');
      document.body.removeChild(el);
      flagCopied();
    };

    if (navigator.clipboard) {
      navigator.clipboard.writeText(text)
        .then(flagCopied)
        .catch(() => execCommandCopy());
    } else {
      execCommandCopy();
    }
  }, [flagCopied]);

  return { copied, copy };
}
