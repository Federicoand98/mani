// 20×20 stroke icons of the design system, drawn with currentColor.
export const paths = {
	home: '<path d="M3 9l7-6 7 6v8H3z"/>',
	cube: '<path d="M10 2l7 4v8l-7 4-7-4V6zM3 6l7 4 7-4M10 10v8"/>',
	sliders: '<path d="M3 6h8M15 6h2M3 14h2M9 14h8"/><circle cx="13" cy="6" r="2"/><circle cx="7" cy="14" r="2"/>',
	plus: '<path d="M10 4v12M4 10h12"/>',
	check: '<circle cx="10" cy="10" r="8"/><path d="M6 10.5l3 3 5-6"/>',
	circle: '<circle cx="10" cy="10" r="8"/>',
	warn: '<path d="M10 3l8 14H2z"/><path d="M10 8v4M10 14.5h.01"/>',
	x: '<path d="M5 5l10 10M15 5L5 15"/>',
	back: '<path d="M12 4l-6 6 6 6"/>',
	menu: '<path d="M3 5h14M3 10h14M3 15h14"/>',
	folder: '<path d="M2 5.5a1 1 0 0 1 1-1h4l2 2h8a1 1 0 0 1 1 1V15a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1z"/>',
	net: '<circle cx="10" cy="10" r="7"/><path d="M3 10h14M10 3c2.5 2.5 2.5 11.5 0 14M10 3c-2.5 2.5-2.5 11.5 0 14"/>',
	lock: '<rect x="4" y="9" width="12" height="8" rx="2"/><path d="M7 9V6a3 3 0 0 1 6 0v3"/>',
	chat: '<path d="M4 4h12a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1H9l-4 3v-3H4a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1z"/>',
	agent: '<rect x="3" y="5" width="14" height="11" rx="3"/><path d="M10 2v3M7 10v1M13 10v1"/>',
	runs: '<path d="M4 5h12M4 10h12M4 15h8"/>',
	flow: '<circle cx="4.5" cy="10" r="2"/><circle cx="15.5" cy="5" r="2"/><circle cx="15.5" cy="15" r="2"/><path d="M6.5 10h3l4-4M9.5 10l4 4"/>'
} as const;

export type IconName = keyof typeof paths;
