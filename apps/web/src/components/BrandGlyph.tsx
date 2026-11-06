import type { SVGProps } from "react";

// Demo零一 shell: compact brand mark for icon-sized slots.
// Lighthouse translated from MindPort's pixel mark into the lucide-style
// stroke language so it sits with the rest of the icon set at 16px.
export function BrandGlyph(props: SVGProps<SVGSVGElement>) {
  return (
    <svg
      {...props}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.75}
      strokeLinecap="round"
      strokeLinejoin="round"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path d="M9.4 4.6 7.4 3.5" />
      <path d="m14.6 4.6 2-1.1" />
      <path d="M10.6 2.8h2.8" />
      <rect x="10.1" y="3.4" width="3.8" height="2.5" rx="0.7" />
      <circle cx="12" cy="4.65" r="0.65" fill="currentColor" stroke="none" />
      <path d="M9.5 6.4h5" />
      <path d="M10.7 6.4h2.6l1.7 11.6H9.0l1.7-11.6Z" />
      <path d="M10 10.8h4" />
      <path d="M9.5 14.6h5" />
      <path d="M7.6 18h8.8l0.9 2.2H6.7l0.9-2.2Z" />
      <path d="M4.2 21.6c1.2-.9 2.3-.9 3.5 0 1.2.9 2.3.9 3.5 0 1.2-.9 2.3-.9 3.5 0 1.2.9 2.3.9 3.5 0" />
    </svg>
  );
}

// Full-colour night-harbour tile for the persistent brand area above Search.
// Palette mirrors scripts/gen-brand-icons.py so it matches the favicon set.
// Colours are exposed as local CSS custom properties to keep the SVG on
// theme tokens rather than hardcoded attributes.
const HARBOR_TILE_COLORS = {
  "--harbor-sea-top": "#2b4a55",
  "--harbor-sea-bottom": "#121f25",
  "--harbor-lamp": "#ffd668",
  "--harbor-tower": "#c06a45",
  "--harbor-tower-cap": "#9c4632",
  "--harbor-stripe": "#f0e4cd",
  "--harbor-sea": "#3a5a63",
  "--harbor-sea-deep": "#243944",
} as const;

export function HarborLighthouseTile(props: SVGProps<SVGSVGElement>) {
  return (
    <svg
      {...props}
      viewBox="0 0 24 24"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden
      style={{ ...HARBOR_TILE_COLORS, ...props.style }}
    >
      <defs>
        <linearGradient id="harbor-tile-sea" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="var(--harbor-sea-top)" />
          <stop offset="1" stopColor="var(--harbor-sea-bottom)" />
        </linearGradient>
        <clipPath id="harbor-tile-clip">
          <rect width="24" height="24" rx="5.5" />
        </clipPath>
      </defs>
      <rect width="24" height="24" rx="5.5" fill="url(#harbor-tile-sea)" />
      <g clipPath="url(#harbor-tile-clip)">
        {/* beam lines */}
        <path
          d="M10.2 5.2 7.6 4"
          stroke="var(--harbor-lamp)"
          strokeWidth="0.55"
          strokeLinecap="round"
          opacity="0.85"
        />
        <path
          d="M13.8 5.2 16.4 4"
          stroke="var(--harbor-lamp)"
          strokeWidth="0.55"
          strokeLinecap="round"
          opacity="0.85"
        />
        {/* lamp */}
        <circle cx="12" cy="5.4" r="1.15" fill="var(--harbor-lamp)" />
        {/* tower */}
        <path d="M10.3 6.6h3.4l1.35 12.1H8.95L10.3 6.6Z" fill="var(--harbor-tower)" />
        <path d="M10.3 6.6h3.4l-0.12 1.05h-3.16L10.3 6.6Z" fill="var(--harbor-tower-cap)" />
        {/* cream stripes */}
        <rect x="10.05" y="9.9" width="3.9" height="0.8" fill="var(--harbor-stripe)" />
        <rect x="9.8" y="13.6" width="4.4" height="0.85" fill="var(--harbor-stripe)" />
        {/* sea */}
        <rect x="0" y="18.7" width="24" height="5.3" fill="var(--harbor-sea)" />
        <path
          d="M0 20.6h24M0 22.3h24"
          stroke="var(--harbor-sea-deep)"
          strokeWidth="0.9"
          strokeLinecap="round"
          opacity="0.9"
        />
        <rect x="0" y="23.0" width="24" height="1" rx="0" fill="var(--harbor-sea-deep)" />
      </g>
    </svg>
  );
}

// Companion mark for larger brand slots (onboarding header): the container
// ship from MindPort's harbour scene, same stroke language as the lighthouse.
export function HarborBoatMark(props: SVGProps<SVGSVGElement>) {
  return (
    <svg
      {...props}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.75}
      strokeLinecap="round"
      strokeLinejoin="round"
      xmlns="http://www.w3.org/2000/svg"
    >
      <circle cx="7.4" cy="7.6" r="0.65" fill="currentColor" stroke="none" />
      <path d="m8.4 10.6-.7-2.2" />
      <path d="M7 15v-3.2a1 1 0 0 1 1-1h3.2a1 1 0 0 1 1 1V15" />
      <rect x="12.6" y="11.8" width="2.8" height="3.2" rx="0.4" />
      <rect x="15.9" y="11.8" width="2.8" height="3.2" rx="0.4" />
      <path d="M3 15h18l-2 3.5a2 2 0 0 1-1.8 1.1H6.8A2 2 0 0 1 5 18.5L3 15Z" />
      <path d="M4 21.4c1.2-.9 2.3-.9 3.5 0 1.2.9 2.3.9 3.5 0 1.2-.9 2.3-.9 3.5 0 1.2.9 2.3.9 3.5 0" />
    </svg>
  );
}
