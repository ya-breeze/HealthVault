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
  // This `fallback` is what actually lets interCyrillic below be reached, and
  // without it the whole arrangement is inert. Left to itself, next/font
  // appends a generated face to the variable — `--font-body` becomes
  // `"Barlow", "Barlow Fallback"` — and that face is `src: local(Arial)` with
  // *no* unicode-range, so it claims every glyph Barlow itself lacks. Arial
  // ships full Cyrillic on Windows and macOS, so Russian text stopped there
  // and Inter was never consulted, no matter what came after it in
  // globals.css. Supplying `fallback` replaces that generated entry rather
  // than appending to it, which is what gets Inter ahead of the Arial catch-all.
  //
  // The families are named literally because they have to appear *inside*
  // this variable; chaining them after `var(--font-body)` in globals.css is
  // too late, since "Barlow Fallback" is already inside it. "Inter Fallback"
  // is next/font's generated companion to interCyrillic below — verified
  // present in the built CSS, and it is what backstops the stack while the
  // webfonts load.
  //
  // Note that `adjustFontFallback: false` does *not* work here, despite being
  // the documented switch for this and still being declared in next/font's
  // .d.ts: nothing under next/dist references it in 16.2.9, so it is accepted
  // and silently ignored. Verified by building with it set and finding
  // "Barlow Fallback" still in the emitted variable.
  //
  // One cost is accepted deliberately: Barlow's generated face carried its
  // metric overrides (size-adjust 96.68%), which reduce layout shift while
  // Barlow downloads. Latin text now backstops to "Inter Fallback" during
  // that window instead — also Arial-based, but adjusted to Inter's metrics
  // (107.12%). Swap-window metrics are therefore ~10% off Barlow's rather
  // than matched to it, which is a smaller defect than Russian text rendering
  // in Arial permanently. Found in code review.
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
