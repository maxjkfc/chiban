"use client";

import { useEffect } from "react";

/**
 * Syncs the browser's visual viewport height into `--app-height`.
 *
 * iOS Safari never resizes `dvh` for the on-screen keyboard, and does not
 * implement `interactive-widget=resizes-content` (WebKit bug 259770).
 * Mirroring `visualViewport.height` into `--app-height` lets the app shell
 * naturally shrink to the visible area when the keyboard opens, keeping the
 * in-flow flex composer pinned above the keyboard without manual fixed-position
 * compensations or scroll overrides.
 */
export function ViewportHeight() {
  useEffect(() => {
    const vv = window.visualViewport;
    if (!vv) return;

    function update() {
      // If user is pinch-zooming, revert to native viewport rather than
      // retaining an invalid fixed height.
      if (vv!.scale !== 1) {
        document.documentElement.style.removeProperty("--app-height");
        return;
      }
      document.documentElement.style.setProperty(
        "--app-height",
        `${vv!.height}px`,
      );
      // If iOS panned the visual viewport during focus, snap layout back.
      if (vv!.offsetTop !== 0) {
        window.scrollTo(0, 0);
      }
    }

    update();
    vv.addEventListener("resize", update);
    vv.addEventListener("scroll", update);
    return () => {
      vv.removeEventListener("resize", update);
      vv.removeEventListener("scroll", update);
    };
  }, []);

  return null;
}
