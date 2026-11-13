import type { ContextUsage, ModelLimits } from '../api/types.ts';
import type { ServerMsg } from '../api/wire.ts';
import { m } from '../messages/en.ts';

export function reduceContext(current: ContextUsage | null, msg: ServerMsg): ContextUsage | null {
	if (msg.type !== 'context') return current;
	return msg.payload as ContextUsage;
}

export function contextSummarized(current: ContextUsage | null, msg: ServerMsg): boolean {
	return msg.type === 'context' && !!(msg.payload as ContextUsage)?.compacted && (!current?.compacted || current.state === 'compacting');
}

export function hasInputLimit(limits: ModelLimits | undefined): boolean {
	return (limits?.context_window ?? 0) > 0 || (limits?.max_input_tokens ?? 0) > 0;
}

export function validateLimits(limits: ModelLimits): string {
	const values = [limits.context_window, limits.max_input_tokens, limits.max_output_tokens];
	if (values.some((n) => n !== undefined && (!Number.isSafeInteger(n) || n <= 0 || n > 1_073_741_824)) || !values.some((n) => n !== undefined)) return m.context.invalid;
	const window = limits.context_window ?? 0;
	if (window && ((limits.max_input_tokens ?? 0) > window || (limits.max_output_tokens ?? 0) >= window)) return m.context.incompatible;
	return '';
}

export function mergedLimits(endpoint: ModelLimits | undefined, manual: ModelLimits | undefined): ModelLimits {
	return {
		context_window: manual?.context_window || endpoint?.context_window,
		max_input_tokens: manual?.max_input_tokens || endpoint?.max_input_tokens,
		max_output_tokens: manual?.max_output_tokens || endpoint?.max_output_tokens
	};
}
