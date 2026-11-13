<script lang="ts">
	import Icon from '#lib/components/Icon.svelte';
	import { m } from '#lib/messages/en.ts';
	import { app, enabledProviders } from '#lib/state.svelte.ts';

	const t = m.home;
	import { agentsApi, api } from '#lib/api/client.ts';

	const connected = $derived(enabledProviders().length);
	let chats = $state(0);
	let agentCount = $state(0);
	agentsApi.list().then((r) => (agentCount = r.agents.length), () => {});
	api.conversations().then((r) => (chats = r.conversations.length), () => {});
</script>

<svelte:head><title>{t.title} - mani</title></svelte:head>

<div>
	<h1>{t.title}</h1>
	{#if app.health}<p class="sm mu">{t.subtitle(app.health.dir)}</p>{/if}
</div>

<section class="card start">
	<h2>{t.getStarted}</h2>
	<a class="step" href="/providers">
		<span class:done={connected > 0} class="tick"><Icon name={connected > 0 ? 'check' : 'circle'} /></span>
		<span class="grow">
			<b>{t.connect}</b>
			<span class="sm mu">{connected > 0 ? t.connectDone(connected) : t.connectTodo}</span>
		</span>
		{#if connected > 0}<span class="cap">{t.done}</span>{/if}
	</a>
	<a class="step" href="/chat">
		<span class:done={chats > 0} class="tick"><Icon name={chats > 0 ? 'check' : 'circle'} /></span>
		<span class="grow">
			<b>{t.chat}</b>
			<span class="sm mu">{chats > 0 ? t.chatDone(chats) : t.chatTodo}</span>
		</span>
		{#if chats > 0}<span class="cap">{t.done}</span>{/if}
	</a>
	<a class="step" href="/agents">
		<span class:done={agentCount > 0} class="tick"><Icon name={agentCount > 0 ? 'check' : 'circle'} /></span>
		<span class="grow">
			<b>{t.agents}</b>
			<span class="sm mu">{agentCount > 0 ? t.agentsDone(agentCount) : t.agentsTodo}</span>
		</span>
	</a>
	<div class="next">
		<span class="cap">{t.next}</span>
		<ul>
			{#each t.nextItems as item (item)}<li class="sm mu">{item}</li>{/each}
		</ul>
	</div>
</section>

<style>
	.start {
		display: flex;
		flex-direction: column;
		gap: 12px;
		max-width: 640px;
	}
	.step {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 12px;
		margin: 0 -12px;
		border-radius: 10px;
		color: var(--tx);
		text-decoration: none;
	}
	.step:hover {
		background: var(--sk);
		color: var(--tx);
	}
	.step b {
		display: block;
		font-size: 15px;
	}
	.tick {
		color: var(--bs);
		display: grid;
	}
	.tick.done {
		color: var(--ok);
	}
	.grow {
		flex: 1;
		min-width: 0;
	}
	.next {
		border-top: 1px solid var(--bd);
		padding-top: 12px;
	}
	.next ul {
		margin: 6px 0 0;
		padding-left: 20px;
	}
</style>
