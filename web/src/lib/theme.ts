import { useSyncExternalStore } from "react";

export type ThemeChoice = "light" | "dark" | "system";

const KEY = "fenghuo-theme";
const media = window.matchMedia("(prefers-color-scheme: dark)");
const listeners = new Set<() => void>();

function stored(): ThemeChoice {
  const v = localStorage.getItem(KEY);
  return v === "light" || v === "dark" ? v : "system";
}

function resolve(choice: ThemeChoice): "light" | "dark" {
  if (choice === "system") return media.matches ? "dark" : "light";
  return choice;
}

function apply() {
  const dark = resolve(stored()) === "dark";
  document.documentElement.classList.toggle("dark", dark);
  // Mobile browsers tint their toolbar with this; keep it in step with --background.
  document.querySelector('meta[name="theme-color"]')?.setAttribute("content", dark ? "#131a2b" : "#f3f5f9");
  for (const l of listeners) l();
}

media.addEventListener("change", apply);
apply();

function subscribe(l: () => void) {
  listeners.add(l);
  return () => listeners.delete(l);
}

export function setTheme(choice: ThemeChoice) {
  if (choice === "system") localStorage.removeItem(KEY);
  else localStorage.setItem(KEY, choice);
  apply();
}

export function useTheme() {
  const choice = useSyncExternalStore(subscribe, stored);
  const resolved = useSyncExternalStore(subscribe, () => resolve(stored()));
  return { choice, resolved };
}
