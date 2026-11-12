// Limits mirror server/workspace/chat.go: checked here so the user hears about
// them before uploading, and again by mani.
export const MAX_IMAGES = 4;
export const MAX_IMAGE_BYTES = 5 * 1024 * 1024;
export const IMAGE_TYPES = ['image/png', 'image/jpeg', 'image/gif', 'image/webp'];

export type Attachment = { mediaType: string; data: string; url: string; name: string };

/** Why a file cannot be attached, or null. */
export function checkImage(file: { type: string; size: number; name: string }, already: number): string | null {
	if (already >= MAX_IMAGES) return `At most ${MAX_IMAGES} images per message.`;
	if (!IMAGE_TYPES.includes(file.type)) return `${file.name}: only PNG, JPEG, GIF and WebP images.`;
	if (file.size > MAX_IMAGE_BYTES) return `${file.name} is ${(file.size / 1048576).toFixed(1)} MB: the limit is 5 MB.`;
	return null;
}

export function readImage(file: File): Promise<Attachment> {
	return new Promise((resolve, reject) => {
		const r = new FileReader();
		r.onload = () => {
			const url = String(r.result);
			resolve({ mediaType: file.type, data: url.slice(url.indexOf(',') + 1), url, name: file.name || 'pasted image' });
		};
		r.onerror = () => reject(r.error);
		r.readAsDataURL(file);
	});
}
