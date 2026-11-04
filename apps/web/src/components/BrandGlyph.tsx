import type { SVGProps } from "react";

// Demo零一 shell: compact brand mark for icon-sized slots.
export function BrandGlyph(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...props} viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <text x="12" y="17.5" textAnchor="middle" fontSize="16" fontWeight={700} fill="currentColor">
        零
      </text>
    </svg>
  );
}
