/*
 * The runnable check for the device-code canonicaliser, run by scripts/check.sh.
 *
 * It imports nothing but the module under test on purpose. The client has no test runner and
 * does not need one for this: a plain script that throws is the smallest thing that fails
 * when the logic breaks, and it costs no dependency and no types package.
 *
 *   bun src/lib/deviceCode.check.ts
 */
import {
	DEVICE_CODE_ALPHABET,
	DEVICE_CODE_LENGTH,
	canonicaliseDeviceCode,
	deviceCodeDigits
} from './deviceCode';

function want(label: string, got: unknown, expected: unknown) {
	const same = JSON.stringify(got) === JSON.stringify(expected);
	if (!same) throw new Error(`${label}: got ${JSON.stringify(got)}, want ${JSON.stringify(expected)}`);
}

/*
 * What the user sees has to equal what the API receives, or the page is back to refusing a
 * code the terminal just printed without saying why.
 */
want('lower case', canonicaliseDeviceCode('f28h2j4k').code, 'F28H-2J4K');
want('separators', canonicaliseDeviceCode('  f28h - 2j4k \n').code, 'F28H-2J4K');
want('overlong', canonicaliseDeviceCode('F28H-2J4K-EXTRA').code, 'F28H-2J4K');
want('half typed', canonicaliseDeviceCode('F28').digits, 3);
want('digits ignore the dash', deviceCodeDigits('F28H-2J4K'), DEVICE_CODE_LENGTH);

/*
 * A dropped keystroke has to be reportable, or the field reads as broken. These five are the
 * characters the alphabet leaves out precisely because they are misread.
 */
want('rejected are reported', canonicaliseDeviceCode('F0O1').rejected, ['0', 'O', '1']);
want('rejected are not kept', canonicaliseDeviceCode('F0O1').code, 'F');

/*
 * The alphabet is the server's, from internal/server/device.go. Every character it can
 * generate has to survive the field, or a valid code is unenterable on the page that
 * approves it.
 */
const generated = canonicaliseDeviceCode(DEVICE_CODE_ALPHABET.slice(0, DEVICE_CODE_LENGTH));
want('server alphabet survives', generated.rejected, []);
want('server alphabet is kept whole', generated.digits, DEVICE_CODE_LENGTH);

console.log('deviceCode: ok');
