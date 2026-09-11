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

To be completed after all retained candidates have been rendered and inspected at 16px, 32px, 180px, 192px, and 512px on both application surfaces and through all target masks.
