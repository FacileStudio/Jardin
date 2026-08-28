import type { Space } from './backend';

const SPACE_KEY = 'mycelium.space_id';

let activeSpaceId = $state<string | null>(
	typeof window === 'undefined' ? null : localStorage.getItem(SPACE_KEY)
);
let spaces = $state<Space[]>([]);

export function getActiveSpaceId(): string | null {
	return activeSpaceId;
}

export function setActiveSpaceId(id: string | null) {
	activeSpaceId = id;
	if (typeof window === 'undefined') return;
	if (id === null) localStorage.removeItem(SPACE_KEY);
	else localStorage.setItem(SPACE_KEY, id);
}

export function getSpaces(): Space[] {
	return spaces;
}

export function setSpaces(list: Space[]) {
	spaces = list;
	if (activeSpaceId !== null && !list.some((s) => s.id === activeSpaceId)) {
		setActiveSpaceId(null);
	}
}
