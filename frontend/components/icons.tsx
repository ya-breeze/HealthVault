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

export function CameraIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base(props)}>
      <path d="M4 8h3l1.5-2h7L17 8h3a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V9a1 1 0 0 1 1-1Z" />
      <circle cx="12" cy="13" r="3.25" />
    </svg>
  );
}

export function PencilIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base(props)}>
      <path d="M4 20h4l10.5-10.5a2 2 0 0 0 0-2.83l-1.17-1.17a2 2 0 0 0-2.83 0L4 16v4Z" />
      <path d="m13.5 6.5 4 4" />
    </svg>
  );
}

export function HistoryIcon(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base(props)}>
      <path d="M3 12a9 9 0 1 0 3-6.7" />
      <path d="M3 4v4h4" />
      <path d="M12 8v4l3 2" />
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
