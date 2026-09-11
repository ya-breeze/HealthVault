# HealthVault web icon exploration

Idea: [ya-breeze/idea-forge#383](https://github.com/ya-breeze/idea-forge/issues/383)

## Decision status

This document defines the icon brief and evaluation method. The candidate comparison and final selection are recorded below once the artwork has been tested at its actual output sizes.

## Current-state audit

- [`frontend/app/`](../../frontend/app/) contains only [`favicon.ico`](../../frontend/app/favicon.ico) as browser icon artwork. It is the generic image retained from the original Next.js scaffold; there is no Apple touch icon or application-icon family in the app directory.
- [`frontend/public/`](../../frontend/public/) still contains the scaffold illustrations `file.svg`, `globe.svg`, `next.svg`, `vercel.svg`, and `window.svg`. They do not represent HealthVault and are not source material for this exploration.
- [`frontend/app/layout.tsx`](../../frontend/app/layout.tsx) exports the product title and description through `metadata`, but declares no icon metadata. Its separate `viewport` export sets `viewportFit: "cover"` so the application can paint into device safe areas; this exploration does not alter either export.
- [`frontend/app/globals.css`](../../frontend/app/globals.css) names the visual language “Instrument Panel” and defines paired neutral surfaces and teal accents for light and dark color schemes. Those tokens are the complete color source for the candidate marks.

This split is design groundwork only. It retains editable sources and a reviewable render but does not replace the favicon, add production files to `frontend/public/`, change metadata or viewport behavior, remove starter files, introduce a service worker, or add install UI.

## Visual philosophy: Clinical Aperture

Clinical Aperture treats information as a signal protected by structure. Broad geometric walls create a quiet chamber; a single interruption or pulse makes the chamber feel active and personal. Meaning should emerge from the relationship between enclosure and signal, not from a pile of familiar healthcare symbols. Every contour is deliberately economical and meticulously tuned.

Space is functional material. A conservative outer margin protects the shape from home-screen masks, while generous interior voids keep the mark open at sixteen pixels. The composition should feel machined but not cold: controlled asymmetry and a living rhythm soften the vault geometry. Optical weight, rather than mathematical density alone, determines the final balance.

Color behaves like an instrument indicator. Teal is the live signal and the neutral container provides stable contrast; there are no decorative hues, simulated materials, or gradients. The light and dark variants preserve the same hierarchy while adapting the accent to the surrounding application surface. This restraint should look painstakingly calibrated rather than merely sparse.

Scale changes detail, not identity. The smallest render may consolidate narrow separations, while the large home-screen render reveals the exact same enclosure and health gesture with no added ornament. Repetition in the comparison grid exposes weak geometry honestly. The final silhouette must feel as if it has been refined through countless reductions, not designed only as an enlarged logo.

Typography is absent from the mark and quiet in the comparison artifact. Labels act as clinical annotations around the work; geometry carries the meaning. Alignment, spacing, mask placement, and edge quality should show master-level care so the package reads as a considered identity study rather than a contact sheet of drafts.

## Palette contract

Candidate artwork is limited to the established application tokens, transparency, and a necessary neutral container:

| Role | Light application surface | Dark application surface |
| --- | --- | --- |
| Background | `#f7f8fa` | `#10141a` |
| Accent | `#0f9c8c` | `#4fd8c4` |

The stable icon container uses the dark application surface (`#10141a`) and the live symbol uses the dark-scheme accent (`#4fd8c4`). This pairing provides one reproducible source artwork that remains identifiable against both light and dark browser chrome. The other two documented colors appear in the comparison backgrounds and may be used when deriving an explicitly theme-aware variant; no additional brand color is introduced.

## Geometry and safe-area contract

- Every source is a square SVG with `viewBox="0 0 64 64"`.
- Critical geometry stays inside the central `48 × 48` unit region from `(8, 8)` to `(56, 56)`. The outer 8-unit margin is clear space, not a crop allowance.
- A circular mask must retain the complete identifying silhouette. Rounded-square masks may not be used to conceal a corner or compensate for off-center artwork.
- Interior negative spaces are at least 4 SVG units wide wherever they carry meaning. At 16px, this is one physical pixel before antialiasing.
- The artwork must remain recognizable by silhouette and voids if reduced to one color. Color can reinforce the live signal, but cannot be the only distinction between its parts.

## Evaluation rubric

Each candidate is scored from 1 (fails) to 5 (excellent) on eight equally weighted criteria, for a maximum of 40. The displayed comparison—not an enlarged source view—is the scoring evidence.

| Criterion | A score of 5 requires |
| --- | --- |
| Recognizability | A memorable mark that can be picked out again without a label. |
| Health + vault meaning | One integrated gesture communicates both protected storage and personal health data without reading as two unrelated badges. |
| Distinctive silhouette | The outer shape and principal void do not collapse into a stock app-category icon. |
| 16px / 32px legibility | The structure, gaps, and signal remain separate in favicon-like tabs at both sizes. |
| Light / dark contrast | The same source artwork has clear edges and stable hierarchy on `#f7f8fa` and `#10141a`. |
| Mask safety | Critical features survive centered square, rounded-square, and circular home-screen masks with comfortable breathing room. |
| Derivative readiness | Geometry can be exported cleanly to SVG, multi-size ICO, 180px Apple touch, and 192/512 application PNGs. |
| Monochrome readiness | A one-color reduction preserves the identifying silhouette and negative-space logic. |

## Explicit exclusions

The exploration does not use a standalone or dominant heart, medical cross, shield, padlock, or house. Those stock symbols advertise broad categories and would make HealthVault harder, not easier, to distinguish. In particular, no proposal may echo the generic house used by the application’s `HomeIcon`.

The mark also excludes letters, words, embedded fonts, gradients, shadows, hairlines, and fine-detail decoration. An `H` or `HV` may emerge only as geometric negative space rather than typeset text. Every candidate must be original artwork, must reference no external resource, and must work without a color-only distinction.

## Candidate comparison and decision

The self-contained [comparison page](../assets/healthvault-icon/comparison.html) and its committed [PNG capture](../assets/healthvault-icon/comparison.png) render all retained sources in light and dark favicon tabs at 16px and 32px. They also render every source at 180px, 192px, and 512px in square, rounded-square, and circular masks on both application surfaces. The active-tab rules use `#0f9c8c` on light and `#4fd8c4` on dark; the source artwork does not change between contexts.

### Scores

| Candidate | Recognizable | Health + vault | Silhouette | 16 / 32 | Contrast | Masks | Derivatives | Monochrome | Total / 40 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `pulse-dial.svg` | 5 | 5 | 4 | 5 | 5 | 5 | 5 | 5 | **39** |
| `record-chamber.svg` | 4 | 5 | 4 | 3 | 5 | 5 | 4 | 4 | **34** |
| `hv-aperture.svg` | 4 | 3 | 5 | 3 | 5 | 5 | 4 | 4 | **33** |

### Selected source

**Selected default: [`pulse-dial.svg`](../assets/healthvault-icon/pulse-dial.svg).**

Pulse Dial wins because one economical relationship carries the whole idea: the circular vault rim establishes secure storage, while the dial spindle becomes the health-data pulse. The identifying rim, central void, and pulse remain separate at 16px and become cleaner rather than busier at home-screen sizes. The circular construction is naturally mask-safe, and the teal rim remains visible when the dark chamber blends into dark browser chrome. A monochrome derivative can render the teal geometry as one foreground shape and the dark chamber as a knockout without redrawing the mark.

The main trade-off is that an isolated pulse is a familiar health gesture and a circular dial can suggest other instrument categories. Their integration and the unusually heavy rim make the combination specific enough for this product, but future brand use should not separate the pulse from its dial or decorate the circle with extra tick marks.

### Rejected alternatives

- [`record-chamber.svg`](../assets/healthvault-icon/record-chamber.svg) communicates stored records most explicitly. Its octagonal perimeter is sturdy, optically even in all three masks, and visibly different from the dial. It loses because the two record bands and the pulse channel compress into a denser, less immediate glyph at 16px. Removing another band would improve reduction but weaken the protected-record-chamber idea that distinguishes this proposal.
- [`hv-aperture.svg`](../assets/healthvault-icon/hv-aperture.svg) has the most distinctive outer silhouette and the strongest single-color poster quality. The continuous geometric H-to-V construction survives home-screen masks cleanly. It loses because the negative spaces can read as a face, gem, or directional marker before they read as personal health data in a vault; at 16px the lower counter also becomes less obvious than the Pulse Dial’s open waveform.

## Production contract for the selected mark

### Color

- The source and default full-color derivatives use `#4fd8c4` for the live rim and pulse and `#10141a` for the vault chamber. The transparent area outside the dial is not a third color.
- Favicon and ICO sizes retain transparency outside the 48-unit dial. Opaque Apple touch and 192/512 application canvases use `#10141a` behind the unchanged mark; the teal rim and pulse remain the defining silhouette on that surface.
- `#0f9c8c` is the light-interface accent and may replace the teal only in a deliberately theme-aware, one-color UI treatment. It must not be mixed with `#4fd8c4` inside one mark. The committed default source stays stable across browser themes.

### Clear space and mask safety

- Preserve the 8-unit clear-space margin on all four sides of the 64-unit canvas: 12.5% of the output width, or 2px at 16px, 4px at 32px, 22.5px at 180px, 24px at 192px, and 64px at 512px.
- Keep the dial centered at `(32, 32)` with radius 24. Do not enlarge it to fill a rounded-square or circular mask; the clear area is what makes the same source safe in both.
- Do not place badges, borders, or status dots inside the `(8, 8)` to `(56, 56)` critical-geometry box. Any product-owned container belongs outside the source mark and may not consume its clear space.

### Favicon behavior

- At 16px and 32px, use the source exactly as drawn: a 5-unit rim and a 5-unit round-joined pulse. Do not add dial ticks or restore detail from a larger concept.
- Rasterize directly from SVG at each target size with antialiasing; do not downsample the 512px application raster or upscale the 16px favicon.
- If a one-color favicon is required, keep the ring and pulse as foreground and knock out the chamber. The pulse endpoints, peak, trough, and round joins must remain visible.

### Invariants across derivatives

The centered 24-unit-radius dial, 5-unit rim, five-segment pulse path, round endpoints and joins, 8-unit canvas margin, and relative alignment of the pulse to the dial are invariant. Raster derivatives may change file format, canvas opacity, and pixel-snapped antialiasing only. They may not rotate, crop, skew, outline, shadow, relabel, or separate the pulse from its vault ring.
