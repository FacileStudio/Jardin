/*
 * The device code's alphabet and shape, in one place.
 *
 * The server draws codes from this same alphabet in internal/server/device.go, and the two
 * have to agree: a page that rejects a character the terminal just printed is worse than no
 * validation at all. The omissions are deliberate. No vowels, so a random draw can never
 * spell a word, and no 0, 1, I, L, O or U, so nothing in a code is ambiguous read off one
 * screen and typed into another.
 */
export const DEVICE_CODE_ALPHABET = '23456789BCDFGHJKMNPQRSTVWXYZ';
export const DEVICE_CODE_LENGTH = 8;
export const DEVICE_CODE_GROUP = 4;
export const DEVICE_CODE_EXAMPLE = 'F28H-2J4K';

export type DeviceCodeEntry = {
	code: string;
	digits: number;
	rejected: string[];
};

/*
 * canonicaliseDeviceCode turns whatever was typed or pasted into the one spelling the field
 * will show and the API will receive: upper case, alphabet characters only, eight at most,
 * with a dash after the fourth. Separators the user or their terminal supplied, dashes and
 * whitespace, are dropped without comment, because they are formatting rather than input.
 *
 * Anything else is returned in `rejected` instead of being swallowed. A field that silently
 * eats a keystroke reads as a broken field, so the caller gets what was dropped and can say
 * so in a sentence.
 *
 * The server's normalizeUserCode (internal/server/device.go) keeps every A-Z0-9 and strips
 * the rest, so it is strictly more forgiving than this and nothing canonicalised here can be
 * refused for its spelling. Narrowing on this side is about showing the user what will
 * actually be sent, never about validating on the server's behalf.
 */
export function canonicaliseDeviceCode(raw: string): DeviceCodeEntry {
	const rejected: string[] = [];
	let kept = '';
	for (const char of raw.toUpperCase()) {
		if (kept.length >= DEVICE_CODE_LENGTH) break;
		if (DEVICE_CODE_ALPHABET.includes(char)) kept += char;
		else if (char !== '-' && char.trim() !== '') rejected.push(char);
	}
	const code =
		kept.length > DEVICE_CODE_GROUP
			? `${kept.slice(0, DEVICE_CODE_GROUP)}-${kept.slice(DEVICE_CODE_GROUP)}`
			: kept;
	return { code, digits: kept.length, rejected: [...new Set(rejected)] };
}

/*
 * How many real characters a code holds, ignoring the dash. Counted against the alphabet
 * rather than against a second copy of the separator rule, so the shape stays defined once.
 */
export function deviceCodeDigits(code: string): number {
	return [...code].filter((char) => DEVICE_CODE_ALPHABET.includes(char)).length;
}
