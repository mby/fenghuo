import type { Location } from "react-router";

export function nodeDetailState(location: Location) {
  if (location.pathname !== "/") return undefined;
  // Keep only the return location, avoiding recursive modal state in browser history.
  return { nodeBackground: { ...location, state: null } };
}

export function nodeDetailBackground(location: Location): Location | undefined {
  if (!/^\/node\/[^/]+$/.test(location.pathname)) return undefined;
  const background = location.state?.nodeBackground;
  if (
    background?.pathname !== "/" ||
    typeof background.search !== "string" ||
    typeof background.hash !== "string" ||
    typeof background.key !== "string"
  )
    return undefined;
  return { ...background, state: null };
}
