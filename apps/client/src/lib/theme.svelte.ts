export type ThemeMode = 'system' | 'light' | 'dark';

const KEY = 'jardin-theme';

function stored(): ThemeMode {
	if (typeof window === 'undefined') return 'system';
	const raw = localStorage.getItem(KEY);
	return raw === 'light' || raw === 'dark' || raw === 'system' ? raw : 'system';
}

export const theme = $state({ mode: stored() });

/*
 * Both classes are written, and `system` writes neither. muse's tokens flip on
 * `prefers-color-scheme` scoped to `:root:not(.light)`, so the `.light` class is the only
 * thing that lets someone force light on a dark OS.
 */
export function setTheme(mode: ThemeMode) {
	theme.mode = mode;
	if (typeof document === 'undefined') return;
	const root = document.documentElement;
	root.classList.toggle('dark', mode === 'dark');
	root.classList.toggle('light', mode === 'light');
	localStorage.setItem(KEY, mode);
}

export function applyStoredTheme() {
	setTheme(theme.mode);
}
