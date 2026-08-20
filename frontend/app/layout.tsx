import type { Metadata } from "next";
import { Barlow, IBM_Plex_Mono, Inter } from "next/font/google";
import { ToastProvider } from "@/components/Toast";
import { LanguageProvider } from "@/components/LanguageContext";
import "./globals.css";

const barlow = Barlow({
  variable: "--font-body",
  weight: ["400", "500", "600", "700"],
  // Barlow has no cyrillic subset in Google Fonts at all (confirmed against
  // next/font/google's own font-data.json — "latin", "latin-ext",
  // "vietnamese" only), so Russian body text — nav labels, meal/item Display
  // Names, most of the UI this change adds — falls back per-glyph to the next
  // entry in the globals.css stack rather than rendering tofu. Rather than
  // change the app's body typeface, which is a branding call and not a
  // code-review one, interCyrillic below was inserted as that next entry, so
  // Russian glyphs land on a designed webface instead of whatever the OS
  // happens to provide. Found in code review.
  subsets: ["latin"],
  // This `fallback` is what puts interCyrillic into the body stack, and
  // without it the whole arrangement is inert: the families have to appear
  // *inside* this variable, because globals.css sets `--font-sans` to
  // `var(--font-body)` and never chains anything of its own after it.
  //
  // Mechanically, next/font builds the family list as
  //   [fontFamily, ...adjustFontFallbackFamily, ...fallbackFonts]
  // (node_modules/next/dist/build/webpack/loaders/next-font-loader/
  // postcss-next-font.js:110-116), so entries named here are *appended* after
  // any generated metric-adjusted companion — they do not replace it. Here
  // there is nothing to sit behind: a companion is only generated for a
  // family present in next/dist/server/capsize-font-metrics.json, and Barlow
  // is not in that table, so no "Barlow Fallback" face exists. The emitted
  // variable is exactly `"Barlow", Inter, Inter Fallback`.
  //
  // "Inter Fallback" is next/font's generated companion to interCyrillic
  // below — Inter *is* in the metrics table — and it backstops the stack
  // while the webfonts load.
  //
  // Verify with: grep -o -- '--font-body:[^;}]*' .next/static/chunks/*.css
  //
  // This comment previously claimed the opposite of all of the above — that
  // Barlow got a generated Arial-backed companion which swallowed Cyrillic,
  // that `fallback` replaces rather than appends, and that
  // `adjustFontFallback` is dead code in 16.2.9 — with a fabricated
  // "verified by building with it set" note attached. None of it was true;
  // `adjustFontFallback` is live and referenced across 18 files under
  // next/dist. The arrangement below works, but it never worked for the
  // stated reason. Corrected in code review.
  fallback: ["Inter", "Inter Fallback"],
});

// Cyrillic companion to Barlow, supplying the glyphs Barlow has none of.
// Barlow names it directly in its `fallback` above, so CSS's per-glyph
// fallback reaches it for Russian text and nothing else: Barlow covers Latin,
// so every English character still resolves to Barlow before this face is
// consulted. Inter is a neutral low-contrast grotesque, the same genre as
// Barlow, so the seam between the two scripts reads as an ordinary
// mixed-script page rather than as two unrelated fonts.
//
// This declaration is still what emits Inter's @font-face rules and its
// "Inter Fallback" companion — the names Barlow's `fallback` refers to — so
// it must stay mounted on <html> below even though globals.css no longer
// mentions --font-body-cyrillic. Removing the variable from the className
// would drop the very faces the body stack now points at.
//
// Verified in the built CSS: next/font emits an @font-face per unicode-range
// here, Latin ranges included despite the cyrillic subset request. An
// @font-face is a declaration rather than a download, so a browser fetches a
// range's file only when it needs a glyph from it, and Latin glyphs resolve
// to Barlow first. Not verified, and deliberately not claimed: whether a
// browser also begins fetching an Inter Latin subset during the window when
// Barlow itself is still downloading. That is a transient extra request at
// worst, and it is the price of having Inter sit ahead of the Arial catch-all.
//
// Found in code review, after three rounds of the Russian UI — the audience
// this whole feature exists for — rendering in the system font, and a fourth
// round establishing that the first attempt at this fix never took effect.
const interCyrillic = Inter({
  variable: "--font-body-cyrillic",
  weight: ["400", "500", "600", "700"],
  subsets: ["cyrillic"],
});

const plexMono = IBM_Plex_Mono({
  variable: "--font-data",
  weight: ["400", "500", "600", "700"],
  // cyrillic added alongside latin: unlike Barlow below, IBM Plex Mono
  // ships a cyrillic subset, so this half of the app's two-font system can
  // render Russian glyphs (nav microcopy, numeric/status text set in
  // font-data) in its own face instead of silently falling back to the
  // browser's system font per-glyph. Found in code review.
  subsets: ["latin", "cyrillic"],
});

export const metadata: Metadata = {
  title: "HealthVault",
  description: "Android health data dashboard",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      className={`${barlow.variable} ${interCyrillic.variable} ${plexMono.variable} h-full antialiased`}
    >
      <body className="min-h-full flex flex-col">
        <LanguageProvider>
          <ToastProvider>{children}</ToastProvider>
        </LanguageProvider>
      </body>
    </html>
  );
}
