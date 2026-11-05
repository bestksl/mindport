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
