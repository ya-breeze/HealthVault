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
});

// Cyrillic companion to Barlow, supplying the glyphs Barlow has none of. It
// sits *after* --font-body in the stack (see globals.css), which is what makes
// CSS's per-glyph fallback reach it for Russian text and nothing else: Barlow
// covers Latin, so every English character still resolves to Barlow before
// this face is ever consulted. Inter is a neutral low-contrast grotesque, the
// same genre as Barlow, so the seam between the two scripts reads as an
// ordinary mixed-script page rather than as two unrelated fonts.
//
// Verified against the built CSS rather than assumed: next/font emits an
// @font-face per unicode-range here (Latin ranges included, despite the
// cyrillic subset request), but an @font-face is a declaration, not a
// download — a browser fetches a range's file only when it actually needs a
// glyph from it. An English-only session therefore never requests an Inter
// file at all, and a Russian one fetches only the Cyrillic ranges.
//
// Found in code review, after three rounds of the Russian UI — the audience
// this whole feature exists for — rendering in the system font.
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
