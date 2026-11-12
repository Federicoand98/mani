import { describe, expect, it } from 'vitest';
import { checkImage, MAX_IMAGE_BYTES } from './images.ts';

describe('checkImage', () => {
	const png = { type: 'image/png', size: 1000, name: 'shot.png' };
	it('accepts a small PNG', () => expect(checkImage(png, 0)).toBeNull());
	it('refuses a fifth image', () => expect(checkImage(png, 4)).toContain('At most 4'));
	it('refuses other types', () => expect(checkImage({ ...png, type: 'image/bmp', name: 'a.bmp' }, 0)).toContain('only PNG'));
	it('refuses a file over 5 MB, saying how big it is', () =>
		expect(checkImage({ ...png, size: MAX_IMAGE_BYTES + 1 }, 0)).toContain('5.0 MB: the limit is 5 MB'));
});
