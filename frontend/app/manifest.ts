import type { MetadataRoute } from "next";

// Served by the App Router at /manifest.webmanifest, with the <link> emitted into every
// document. Written as a route rather than a static file in `public/` so the names and colours
// live beside the rest of the app's metadata instead of in a JSON file nothing type-checks.
//
// `icon.png` and `apple-icon.png` are NOT listed here. Those are App Router file conventions:
// Next.js emits their <link> tags itself, with hashed URLs, and repeating them in the manifest
// would pin a second, unhashed copy of the same asset.
export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "HealthVault",
    short_name: "HealthVault",
    description: "Android health data dashboard",
    start_url: "/",
    display: "standalone",
    // The app's own light surface and teal accent, from `globals.css`. The installed splash
    // should agree with the interface it opens into.
    background_color: "#f7f8fa",
    theme_color: "#0f9c8c",
    icons: [
      { src: "/icon-192.png", sizes: "192x192", type: "image/png", purpose: "any" },
      { src: "/icon-512.png", sizes: "512x512", type: "image/png", purpose: "any" },
      // Android crops an installed icon to the launcher's shape. Without a maskable entry it
      // crops the plain one, taking the outer edge of the mark with it.
      {
        src: "/icon-512-maskable.png",
        sizes: "512x512",
        type: "image/png",
        purpose: "maskable",
      },
    ],
  };
}
