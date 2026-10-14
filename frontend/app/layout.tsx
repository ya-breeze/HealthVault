import type { Metadata, Viewport } from "next";
import { Manrope, IBM_Plex_Mono } from "next/font/google";
import { ToastProvider } from "@/components/Toast";
import { BottomActionBarProvider } from "@/components/ui/BottomActionBar";
import { LanguageProvider } from "@/components/LanguageContext";
import "./globals.css";

// Manrope matches the approved design and supplies both Latin and Cyrillic
// directly, replacing the previous two-face Barlow/Inter fallback arrangement.
const bodyFont = Manrope({variable: '--font-body', subsets: ['latin', 'cyrillic'], display: 'swap'});
const dataFont = IBM_Plex_Mono({variable: '--font-data', weight: ['400', '500', '600', '700'], subsets: ['latin', 'cyrillic'], display: 'swap'});

export const metadata: Metadata = {
  title: "HealthVault",
  description: "Android health data dashboard",
};

// Opts the document into painting behind the device's safe areas, which is
// what makes `env(safe-area-inset-*)` resolve to anything other than 0.
// Without it every inset rule in the app is dead CSS on *every* device, a
// notched phone included — not merely in a headless test browser. The bottom
// navigation bar's own inset padding depends on this, and so do the two
// submit bars' pre-existing `env()` padding, which was inert until now.
// See mobile-navigation's "The document opts into safe-area insets".
export const viewport: Viewport = {
  viewportFit: "cover",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      className={`${bodyFont.variable} ${dataFont.variable} h-full antialiased`}
    >
      <body className="min-h-full flex flex-col">
        <LanguageProvider>
          <BottomActionBarProvider>
            <ToastProvider>{children}</ToastProvider>
          </BottomActionBarProvider>
        </LanguageProvider>
      </body>
    </html>
  );
}
