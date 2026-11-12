<script lang="ts">
	import { m } from '../messages/en.ts';

	let { text }: { text: string } = $props();
	let copied = $state(false);

	async function copy() {
		try {
			await navigator.clipboard.writeText(text);
			copied = true;
			setTimeout(() => (copied = false), 1500);
		} catch {
			// clipboard refused (insecure context or permission): the text stays selectable
		}
	}
</script>

<button type="button" onclick={copy}>{copied ? m.addProvider.copied : m.addProvider.copy}</button>
