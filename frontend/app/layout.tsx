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
  // "Inter Fallback" is next/font's generated metric-adjusted companion to
  // interCyrillic below, and it backstops the stack while the webfonts load.
  //
  // Verify with: grep -o -- '--font-body:[^;}]*' .next/static/chunks/*.css
  fallback: ["Inter", "Inter Fallback"],
  // Suppresses the metric-adjusted companion next/font would otherwise
  // generate for Barlow itself. That companion is
  //   @font-face{font-family:Barlow Fallback;src:local("Arial");…}
  // with **no unicode-range**, so it matches every codepoint there is,
  // Cyrillic included — and the webpack loader inserts it *ahead* of the
  // families named in `fallback`:
  //   [fontFamily, ...adjustFontFallbackFamily, ...fallbackFonts]
  // (node_modules/next/dist/build/webpack/loaders/next-font-loader/
  // postcss-next-font.js:110-116). Russian text then resolves to Arial before
  // Inter is ever consulted, which is the exact bug this whole arrangement
  // exists to fix. Confirmed by building both ways:
  //   next build            → --font-body:"Barlow", Inter, Inter Fallback
  //   next build --webpack  → --font-body:"Barlow","Barlow Fallback",Inter,…
  // Turbopack, this project's default, happens to replace the companion when
  // an explicit `fallback` is given rather than prepending to it, so the bug
  // was latent behind a build flag rather than visible. This flag removes it
  // structurally instead of relying on which bundler ran.
  //
  // The cost is that Barlow keeps no metric-adjusted backstop of its own, so
  // Latin text is backstopped by Inter Fallback, whose overrides are computed
  // for Inter (107.12% ascent vs Barlow's 103.43%) — a small layout shift
  // while Barlow loads. Accepted deliberately: that is already exactly what
  // ships today under Turbopack, and Cyrillic rendering in the right typeface
  // is what this change is for.
  //
  // This comment has been wrong twice, in opposite directions, before this
  // note. It first claimed Barlow *did* get an Arial companion but that
  // `fallback` replaced it — with a fabricated "verified by building" claim
  // attached. It was then "corrected" to say Barlow is absent from
  // next/dist/server/capsize-font-metrics.json so no companion exists at all;
  // that is also false — the table keys are camelCased, and `barlow` is right
  // there beside `inter`. Both readings were guesses at a mechanism that a
  // two-line build would have settled. Corrected in code review.
  adjustFontFallback: false,
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
