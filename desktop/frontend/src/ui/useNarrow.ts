import { useEffect, useState } from "react";

// True below the width where the details panel leaves the layout.
export function useNarrow() {
  const query = "(max-width: 1199px)";
  const [narrow, setNarrow] = useState(
    () => window.matchMedia?.(query).matches ?? false,
  );
  useEffect(() => {
    const media = window.matchMedia?.(query);
    if (!media) return;
    const update = () => setNarrow(media.matches);
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);
  return narrow;
}
