import type { Metadata } from "next";
import { Barlow, IBM_Plex_Mono } from "next/font/google";
import { ToastProvider } from "@/components/Toast";
import { LanguageProvider } from "@/components/LanguageContext";
import "./globals.css";

const barlow = Barlow({
  variable: "--font-body",
  weight: ["400", "500", "600", "700"],
  // Barlow has no cyrillic subset in Google Fonts at all (confirmed against
  // next/font/google's own font-data.json — "latin", "latin-ext",
  // "vietnamese" only), so Russian body text — nav labels, meal/item Display
  // Names, most of the UI this change adds — falls back per-glyph to the
  // globals.css stack's next entry (ui-sans-serif) rather than rendering
  // tofu: readable, just a visibly different typeface than the surrounding
  // English UI. Swapping the app's body typeface to something with cyrillic
  // coverage is a branding call outside a code-review fix's scope — flagged
  // to the user rather than decided here. Found in code review.
  subsets: ["latin"],
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
      className={`${barlow.variable} ${plexMono.variable} h-full antialiased`}
    >
      <body className="min-h-full flex flex-col">
        <LanguageProvider>
          <ToastProvider>{children}</ToastProvider>
        </LanguageProvider>
      </body>
    </html>
  );
}
