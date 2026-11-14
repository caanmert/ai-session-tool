// Runs in the fixture browser. Check visible text against its actual CSS layers.
export function contrastFailures() {
  const rgba = (value) => {
    const n = value.match(/[\d.]+/g)?.map(Number) || [0, 0, 0];
    return [n[0], n[1], n[2], n[3] ?? 1];
  };
  const blend = (front, back) =>
    front.slice(0, 3).map((v, i) => v * front[3] + back[i] * (1 - front[3]));
  function background(el) {
    if (!el) return [255, 255, 255];
    const color = rgba(getComputedStyle(el).backgroundColor);
    return color[3] === 1 ? color : blend(color, background(el.parentElement));
  }
  const luminance = (color) => {
    const linear = color
      .slice(0, 3)
      .map((value) => value / 255)
      .map((v) => (v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
    return 0.2126 * linear[0] + 0.7152 * linear[1] + 0.0722 * linear[2];
  };
  const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  const failures = [],
    modal = document.querySelector("dialog[open]");
  let node;
  while ((node = walker.nextNode())) {
    const el = node.parentElement;
    if (
      !node.textContent.trim() ||
      !el ||
      el.closest("script,style,button:disabled") ||
      (modal && !modal.contains(el))
    )
      continue;
    const style = getComputedStyle(el),
      bounds = el.getBoundingClientRect();
    if (
      !bounds.width ||
      !bounds.height ||
      style.visibility !== "visible" ||
      bounds.bottom < 0 ||
      bounds.top > innerHeight
    )
      continue;
    const bg = background(el),
      fg = rgba(style.color);
    for (let current = el; current; current = current.parentElement)
      fg[3] *= Number(getComputedStyle(current).opacity);
    if (fg[3] === 0) continue;
    const a = luminance(blend(fg, bg)),
      b = luminance(bg),
      ratio = (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
    const size = parseFloat(style.fontSize),
      large = size >= 24 || (size >= 18.66 && Number(style.fontWeight) >= 700);
    if (ratio + 0.03 < (large ? 3 : 4.5))
      failures.push({
        text: node.textContent.trim().slice(0, 60),
        ratio: Number(ratio.toFixed(2)),
        className: el.className,
      });
  }
  return failures;
}
