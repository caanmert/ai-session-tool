import type { ReactNode } from "react";

// Stroke icons on a 24px grid. They inherit the text color.
const paths = {
  folder: (
    <path d="M3 7V5a1 1 0 0 1 1-1h5l2 3h9a1 1 0 0 1 1 1v11a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V7Z" />
  ),
  search: (
    <>
      <circle cx="10.5" cy="10.5" r="6.5" />
      <path d="m16 16 5 5" />
    </>
  ),
  plus: <path d="M12 5v14M5 12h14" />,
  grid: (
    <>
      <rect x="3" y="3" width="7" height="7" rx="1" />
      <rect x="14" y="3" width="7" height="7" rx="1" />
      <rect x="3" y="14" width="7" height="7" rx="1" />
      <rect x="14" y="14" width="7" height="7" rx="1" />
    </>
  ),
  terminal: (
    <>
      <rect x="3" y="4" width="18" height="16" rx="3" />
      <path d="m7 9 3 3-3 3m6 0h4" />
    </>
  ),
  refresh: (
    <>
      <path d="M20 7v5h-5M4 17v-5h5" />
      <path d="M6 7a7 7 0 0 1 12-1l2 3M4 15l2 3a7 7 0 0 0 12-1" />
    </>
  ),
  branch: (
    <>
      <circle cx="6" cy="5" r="2" />
      <circle cx="6" cy="19" r="2" />
      <circle cx="18" cy="5" r="2" />
      <path d="M6 7v10m12-10v2a6 6 0 0 1-6 6H6" />
    </>
  ),
  arrow: <path d="M5 12h14m-5-5 5 5-5 5" />,
  external: (
    <>
      <path d="M14 4h6v6M20 4l-9 9" />
      <path d="M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5" />
    </>
  ),
  issue: (
    <>
      <circle cx="12" cy="12" r="8" />
      <circle cx="12" cy="12" r="1.6" fill="currentColor" />
    </>
  ),
  "arrow-up": <path d="M12 19V5m-6 6 6-6 6 6" />,
  close: <path d="m6 6 12 12M6 18 18 6" />,
  message: <path d="M21 11a8 8 0 0 1-8 8H5l-3 3V11a9 9 0 0 1 19 0Z" />,
  check: <path d="m5 12 4 4L19 6" />,
  pin: <path d="m9 3 6 0-1 6 4 4v2H6v-2l4-4-1-6Zm3 12v6" />,
  bell: (
    <>
      <path d="M6 16v-6a6 6 0 0 1 12 0v6l2 2H4l2-2Z" />
      <path d="M10 21h4" />
    </>
  ),
  pencil: (
    <>
      <path d="m4 20 1-5L16 4l4 4L9 19l-5 1Z" />
      <path d="m14 6 4 4" />
    </>
  ),
  file: (
    <>
      <path d="M6 3h8l5 5v12a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1Z" />
      <path d="M14 3v5h5" />
    </>
  ),
  alert: (
    <>
      <path d="M12 4 2.5 20h19L12 4Z" />
      <path d="M12 10v4m0 3v.01" />
    </>
  ),
  stop: (
    <rect
      x="6"
      y="6"
      width="12"
      height="12"
      rx="2"
      fill="currentColor"
      stroke="none"
    />
  ),
  handoff: (
    <>
      <path d="M4 8h15m-4-4 4 4-4 4" />
      <path d="M20 16H5m4-4-4 4 4 4" />
    </>
  ),
  code: <path d="m9 7-5 5 5 5m6-10 5 5-5 5" />,
  copy: (
    <>
      <rect x="9" y="9" width="11" height="11" rx="2" />
      <path d="M5 15V6a2 2 0 0 1 2-2h8" />
    </>
  ),
  settings: (
    <>
      <circle cx="12" cy="12" r="3.5" />
      <path d="M12 2.5v3m0 13v3m9.5-9.5h-3m-13 0h-3m16.2-6.7-2.1 2.1m-9.2 9.2-2.1 2.1m13.4 0-2.1-2.1M7.4 7.4 5.3 5.3" />
    </>
  ),
  gauge: (
    <>
      <path d="M4 17a8 8 0 1 1 16 0" />
      <path d="m12 17 4-5" />
    </>
  ),
  chevron: <path d="m9 6 6 6-6 6" />,
  clock: (
    <>
      <circle cx="12" cy="12" r="8.5" />
      <path d="M12 7.5V12l3 2" />
    </>
  ),
  sidebar: (
    <>
      <rect x="3" y="4" width="18" height="16" rx="2.5" />
      <path d="M9 4v16" />
    </>
  ),
  filter: <path d="M4 5h16l-6 7v6l-4 2v-8L4 5Z" />,
  image: (
    <>
      <rect x="3" y="4" width="18" height="16" rx="2.5" />
      <circle cx="8.5" cy="9.5" r="1.6" />
      <path d="m4 17 5-5 4 4 3-3 4 4" />
    </>
  ),
  paperclip: (
    <path d="m20 11.5-8 8a5 5 0 0 1-7-7L13.5 4a3.3 3.3 0 0 1 4.7 4.7l-8.4 8.4a1.7 1.7 0 0 1-2.4-2.4L15 7" />
  ),
} satisfies Record<string, ReactNode>;

export type IconName = keyof typeof paths;
export const iconNames = Object.keys(paths) as IconName[];

export default function Icon({
  name,
  size = 18,
}: {
  name: IconName;
  size?: number;
}) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {paths[name]}
    </svg>
  );
}
