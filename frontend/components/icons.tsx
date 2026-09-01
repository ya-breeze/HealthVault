import type { SVGProps } from 'react';

/**
 * Instrument Panel icon set — inline SVG line icons replacing emoji, so
 * rendering doesn't depend on the OS/browser's emoji font (see dashboard-ui's
 * "Icon set replaces emoji" requirement). Consistent 1.75 stroke weight,
 * round caps/joins, 24x24 viewBox; size via className (e.g. "w-4 h-4").
 */

function base(props: SVGProps<SVGSVGElement>) {
  return {
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: 1.75,
    strokeLinecap: 'round' as const,
    strokeLinejoin: 'round' as const,
    ...props,
  };
}

/**
 * Props for the five icons the mobile bottom navigation bar uses. `active`
 * swaps the outline drawing for a solid one, so the active destination is
 * distinguishable without relying on its accent color — see
 * mobile-navigation's "Active destination is indicated" ("SHALL NOT be
 * carried by color alone"). The filled drawings knock their detail out with
 * `fillRule="evenodd"` rather than painting it in the surface color, so they
 * stay correct on whatever background they are placed on.
 */
type IconProps = SVGProps<SVGSVGElement> & { active?: boolean };

// Solid drawings share these: no stroke, so the shape is entirely fill, and
// `evenodd` so an inner subpath (a camera lens, a clock's hands) reads as a
// hole rather than as more ink.
function solid(props: SVGProps<SVGSVGElement>) {
  return {
    viewBox: '0 0 24 24',
    fill: 'currentColor',
    fillRule: 'evenodd' as const,
    clipRule: 'evenodd' as const,
    stroke: 'none',
    ...props,
  };
}

export function CameraIcon({ active, ...props }: IconProps) {
  if (active) {
    return (
      <svg {...solid(props)}>
        <path d="M4 8h3l1.5-2h7L17 8h3a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V9a1 1 0 0 1 1-1Zm8 8.25a3.25 3.25 0 1 0 0-6.5 3.25 3.25 0 0 0 0 6.5Z" />
      </svg>
    );
  }
  return (
    <svg {...base(props)}>
      <path d="M4 8h3l1.5-2h7L17 8h3a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V9a1 1 0 0 1 1-1Z" />
      <circle cx="12" cy="13" r="3.25" />
    </svg>
  );
}

export function PencilIcon({ active, ...props }: IconProps) {
  if (active) {
    return (
      <svg {...solid(props)}>
        <path d="M4 20h4l10.5-10.5a2 2 0 0 0 0-2.83l-1.17-1.17a2 2 0 0 0-2.83 0L4 16v4Z" />
      </svg>
    );
  }
  return (
    <svg {...base(props)}>
      <path d="M4 20h4l10.5-10.5a2 2 0 0 0 0-2.83l-1.17-1.17a2 2 0 0 0-2.83 0L4 16v4Z" />
      <path d="m13.5 6.5 4 4" />
    </svg>
  );
}

export function HistoryIcon({ active, ...props }: IconProps) {
  if (active) {
    // A filled disc with the clock's hands knocked out of it. The outline
    // drawing below is three open arcs, which cannot simply be filled — an
    // open path fills to its chord and reads as a wedge, not a clock.
    return (
      <svg {...solid(props)}>
        <path d="M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18Zm.9 4.9a.9.9 0 0 0-1.8 0v4.6c0 .3.15.58.4.75l3 2a.9.9 0 0 0 1-1.5l-2.6-1.73V7.9Z" />
      </svg>
    );
  }
  return (
    <svg {...base(props)}>
      <path d="M3 12a9 9 0 1 0 3-6.7" />
      <path d="M3 4v4h4" />
      <path d="M12 8v4l3 2" />
    </svg>
  );
}

export function HomeIcon({ active, ...props }: IconProps) {
  if (active) {
    return (
      <svg {...solid(props)}>
        <path d="M11.36 3.27a1 1 0 0 1 1.28 0l8 6.67a1 1 0 0 1 .36.77V20a1 1 0 0 1-1 1h-5v-6h-6v6H4a1 1 0 0 1-1-1v-9.29a1 1 0 0 1 .36-.77l8-6.67Z" />
      </svg>
    );
  }
  return (
    <svg {...base(props)}>
      <path d="M3.5 10.7 12 3.6l8.5 7.1V20a1 1 0 0 1-1 1h-4.75v-6h-5.5v6H4.5a1 1 0 0 1-1-1v-9.3Z" />
    </svg>
  );
}

export function MoreIcon({ active, ...props }: IconProps) {
  if (active) {
    return (
      <svg {...solid(props)}>
        <circle cx="5.5" cy="12" r="1.85" />
        <circle cx="12" cy="12" r="1.85" />
        <circle cx="18.5" cy="12" r="1.85" />
      </svg>
    );
  }
  return (
    <svg {...base(props)}>
      <circle cx="5.5" cy="12" r="1.5" />
      <circle cx="12" cy="12" r="1.5" />
      <circle cx="18.5" cy="12" r="1.5" />
    </svg>
  );
}

export function EyeIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base(props)}>
      <path d="M2 12s3.5-6.5 10-6.5S22 12 22 12s-3.5 6.5-10 6.5S2 12 2 12Z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  );
}

export function EyeOffIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base(props)}>
      <path d="M10.6 6.1A9.7 9.7 0 0 1 12 5.5c6.5 0 10 6.5 10 6.5a17.8 17.8 0 0 1-3.1 4.1" />
      <path d="M6.2 8A17.6 17.6 0 0 0 2 12s3.5 6.5 10 6.5a9.9 9.9 0 0 0 4.1-.85" />
      <path d="M9.9 9.9a3 3 0 0 0 4.2 4.2" />
      <path d="m3 3 18 18" />
    </svg>
  );
}

export function LinkIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base(props)}>
      <path d="M9.5 14.5 14.5 9.5" />
      <path d="M11 6.5 12.5 5a3.54 3.54 0 0 1 5 5L16 11.5" />
      <path d="M13 17.5 11.5 19a3.54 3.54 0 0 1-5-5L8 12.5" />
    </svg>
  );
}

export function ImportIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base(props)}>
      <path d="M12 3v11" />
      <path d="m7.5 10 4.5 4.5 4.5-4.5" />
      <path d="M4 17v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2" />
    </svg>
  );
}

export function LogoutIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base(props)}>
      <path d="M9 4H6a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h3" />
      <path d="M16 16.5 21 12l-5-4.5" />
      <path d="M21 12H9" />
    </svg>
  );
}

export function SettingsIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base(props)}>
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09a1.65 1.65 0 0 0-1.08-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09a1.65 1.65 0 0 0 1.51-1.08 1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1Z" />
    </svg>
  );
}

// Drawn at the same 24x24 as the rest of the set but rendered small (14px on
// the nutrition card), so the stem and the dot are separate subpaths with a
// visible gap rather than a single glyph that would fill in at that size.
export function InfoIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base(props)}>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 16v-4" />
      <path d="M12 8h.01" />
    </svg>
  );
}
