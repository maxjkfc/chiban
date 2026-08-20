"use client";

import { useEffect } from "react";

// iOS Safari never shrinks `dvh` for the on-screen keyboard, and it does not
// implement `interactive-widget` either (WebKit bug 259770, still open) - the
// only thing that actually reports the space left after the keyboard is
// `visualViewport`. Mirroring its height into a CSS variable lets the app
// shell size itself to what's really visible instead of the pre-keyboard
// layout height, so nothing below the composer goes blank and nothing needs
// to auto-scroll to keep the focused input in view.
//
// Closing the keyboard fires the same `resize` event back to the full
// window height. iOS leaves the app shell scrolled wherever it auto-scrolled
// the focused input to; nothing else undoes that once the keyboard is gone,
// so the space it made room for keeps looking blank. Snapping the shell back
// to the top when the full height returns clears it.
export function ViewportHeight() {
  useEffect(() => {
    const vv = window.visualViewport;
    if (!vv) return;

    function apply() {
      const height = vv!.height;
      document.documentElement.style.setProperty("--app-height", `${height}px`);
      if (Math.abs(height - window.innerHeight) < 1) {
        document.getElementById("app-shell")?.scrollTo(0, 0);
      }
    }

    apply();
    vv.addEventListener("resize", apply);
    vv.addEventListener("scroll", apply);
    return () => {
      vv.removeEventListener("resize", apply);
      vv.removeEventListener("scroll", apply);
    };
  }, []);

  return null;
}
