# Give HealthVault its chosen web icon

## Why

`frontend/app/favicon.ico` is still the generic icon the Next.js scaffold left behind, and
`frontend/public/` still holds that scaffold's unreferenced `file.svg`, `globe.svg`, `next.svg`,
`vercel.svg` and `window.svg`. Nothing in `frontend/app/layout.tsx` declares an Apple touch icon
or a web manifest, so an installed shortcut gets no name, no colours, and a cropped placeholder.

An earlier attempt (closed pull request #67) generated three candidate marks and scored them
against a rubric it also wrote. The owner rejected all three. The artwork this spec ships was
supplied by the owner: a heart whose right half is a leaf, in two greens. It arrived as a
transparent PNG and is used as it is — no redraw, no vector conversion.

## How

Use the App Router's file conventions rather than hand-written `<link>` tags. `app/icon.png`,
`app/apple-icon.png` and `app/manifest.ts` are picked up by the framework, which emits the markup
and the hashed URLs itself; hand-written tags would duplicate that and drift from it. The existing
`favicon.ico` keeps its name and place, so the root `/favicon.ico` probe keeps working.

The Apple icon is opaque white. iOS ignores transparency and composites on black, which would put
the dark-green mark on black and lose its outer edge. The manifest's maskable entry carries the
mark at 78% of the canvas for the same class of reason on Android, where the launcher crops the
icon to its own shape.

`export const viewport` already opts the document into painting behind the safe areas, and the
whole app depends on that. Theme colour goes in the manifest, not into that export, so the
existing `viewportFit: "cover"` is left exactly as it is.

Removing the scaffold's five unused SVGs is in scope only after confirming nothing references
them, since they are the other half of the same "this is still a scaffold" problem.

Excluded deliberately: a header or login-page logo, any change to the app's palette or fonts,
service workers, offline behaviour, install prompts, and the two other projects' icons, which
have their own branches.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

### Task 1: Ship the assets through the App Router conventions

- [x] Replace `frontend/app/favicon.ico` with the 16/32/48 ICO built from the supplied artwork.
- [x] Add `frontend/app/icon.png` (512, transparent) and `frontend/app/apple-icon.png`
      (180, opaque white) so Next.js emits the icon and Apple touch metadata itself.
- [x] Add `frontend/public/icon-192.png`, `frontend/public/icon-512.png` and
      `frontend/public/icon-512-maskable.png` for the manifest to reference.
- [x] Mark completed

### Task 2: Declare the manifest

- [x] Add `frontend/app/manifest.ts` exporting a `MetadataRoute.Manifest` with HealthVault's name,
      short name, `/` start URL, standalone display, background and theme colours drawn from the
      app's existing palette, and the 192, 512 and maskable 512 entries with correct sizes, types
      and purposes.
- [x] Leave `export const viewport` untouched, so the safe-area behaviour the layout depends on
      is unchanged.
- [x] Mark completed

### Task 3: Clear out the scaffold leftovers

- [x] Confirm by search that `file.svg`, `globe.svg`, `next.svg`, `vercel.svg` and `window.svg`
      are referenced nowhere in the app, the tests or the e2e suite.
- [x] Remove them only if that search comes back empty; otherwise leave them and say so here.
- [x] Mark completed

### Task 4: Cover it and validate

- [ ] Add an e2e check that the served document advertises an icon and a manifest, that the
      manifest responds with a JSON body naming icons that themselves respond, and that the
      Apple touch icon is reachable.
- [ ] Run `make lint`, `make test` and `make test-e2e` against the deployed WIP stack and fix
      what they report.
- [ ] Mark completed
