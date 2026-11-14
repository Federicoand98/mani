<script lang="ts">
	import '@fontsource-variable/plus-jakarta-sans';
	import '@fontsource-variable/jetbrains-mono';
	import '#lib/styles/tokens.css';
	import '#lib/styles/base.css';

	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import Icon from '#lib/components/Icon.svelte';
	import type { IconName } from '#lib/components/icons.ts';
	import Toasts from '#lib/components/Toasts.svelte';
	import { m } from '#lib/messages/en.ts';
	import { app, checkHealth, loadProviders } from '#lib/state.svelte.ts';

	let { children } = $props();
	let menuOpen = $state(false);

	type Item = { href: string; icon: IconName; label: string };
	const top: Item[] = [{ href: '/', icon: 'home', label: m.nav.home }];
	const build: Item[] = [
		{ href: '/chat', icon: 'chat', label: m.nav.chat },
		{ href: '/agents', icon: 'agent', label: m.nav.agents }
	];
	const observe: Item[] = [{ href: '/runs', icon: 'runs', label: m.nav.runs }];
	const system: Item[] = [
		{ href: '/providers', icon: 'cube', label: m.nav.providers },
		{ href: '/settings', icon: 'sliders', label: m.nav.settings }
	];
	const current = (href: string) =>
		href === '/' ? page.url.pathname === '/' : page.url.pathname.startsWith(href);

	onMount(() => {
		checkHealth();
		const id = setInterval(checkHealth, 15000);
		return () => clearInterval(id);
	});

	// Providers load once mani answers, and again when it comes back.
	$effect(() => {
		if (app.connection === 'ok') loadProviders();
	});

	$effect(() => {
		page.url.pathname;
		menuOpen = false;
	});
</script>

{#if app.connection === 'signed-out'}
	<main class="gate" id="main">
		<div class="card stack">
			<span class="brand-mark" aria-hidden="true">m</span>
			<h1>{m.signedOut.title}</h1>
			<p class="mu">{m.signedOut.body}</p>
			<div class="command"><span>mani web</span></div>
		</div>
	</main>
{:else}
	<a class="skip" href="#main">{m.app.skip}</a>
	<div class="app">
		<nav class="side" class:open={menuOpen} aria-label="Main">
			<a class="brand" href="/"><span class="brand-mark" aria-hidden="true">m</span>mani</a>
			{#each top as it (it.href)}
				<a class="nav" href={it.href} aria-current={current(it.href) ? 'page' : undefined}><Icon name={it.icon} />{it.label}</a>
			{/each}
			<div class="group">{m.nav.groupBuild}</div>
			{#each build as it (it.href)}
				<a class="nav" href={it.href} aria-current={current(it.href) ? 'page' : undefined}><Icon name={it.icon} />{it.label}</a>
			{/each}
			<div class="group">{m.nav.groupObserve}</div>
			{#each observe as it (it.href)}
				<a class="nav" href={it.href} aria-current={current(it.href) ? 'page' : undefined}><Icon name={it.icon} />{it.label}</a>
			{/each}
			<div class="group">{m.nav.groupSystem}</div>
			{#each system as it (it.href)}
				<a class="nav" href={it.href} aria-current={current(it.href) ? 'page' : undefined}><Icon name={it.icon} />{it.label}</a>
			{/each}
			<div class="server" class:off={app.connection === 'offline'}>
				<b>
					<span class="dot"></span>
					{app.connection === 'offline' ? m.status.offline : app.connection === 'ok' ? m.status.connected : m.status.checking}
				</b>
				<span class="mu">{location.host}{app.health ? ` - mani ${app.health.version}` : ''}</span>
			</div>
		</nav>
		{#if menuOpen}<button class="side-scrim" aria-label="Close the menu" onclick={() => (menuOpen = false)}></button>{/if}

		<div class="main">
			<header class="top">
				<button class="icon-btn menu-btn" aria-label={m.app.menu} onclick={() => (menuOpen = !menuOpen)}><Icon name="menu" /></button>
				{#if app.health}
					<span class="ws" title={m.app.workspace}><Icon name="folder" size={16} />{app.health.dir}</span>
				{/if}
			</header>
			<main id="main" tabindex="-1" class="content" class:full={page.url.pathname.startsWith('/chat') || page.url.pathname === '/agents/new'}>
				{#if app.connection === 'offline'}
					<div class="banner error" role="alert">
						<Icon name="warn" />
						<div class="stack" style="gap:10px;flex:1;min-width:0">
							<span><b>{m.offline.title}</b> {m.offline.body}</span>
							<div class="command"><span>{m.offline.command}</span></div>
						</div>
						<button class="btn secondary small" onclick={checkHealth}>{m.offline.retry}</button>
					</div>
				{/if}
				{@render children()}
			</main>
		</div>
	</div>
	<Toasts />
{/if}

<style>
	.skip {
		position: absolute;
		left: -999px;
		top: 8px;
		z-index: 100;
		background: var(--pr);
		color: var(--onpr);
		padding: 8px 12px;
		border-radius: 8px;
	}
	.skip:focus {
		left: 8px;
	}
	.app {
		display: grid;
		grid-template-columns: 248px minmax(0, 1fr);
		min-height: 100vh;
	}
	.side {
		position: sticky;
		top: 0;
		height: 100vh;
		overflow-y: auto;
		background: var(--sf);
		border-right: 1px solid var(--bd);
		display: flex;
		flex-direction: column;
		padding: 16px 12px;
		gap: 4px;
	}
	.brand {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 4px 8px 16px;
		font-weight: 800;
		font-size: 18px;
		color: var(--tx);
		text-decoration: none;
	}
	.brand-mark {
		display: grid;
		place-items: center;
		width: 32px;
		height: 32px;
		border-radius: 9px;
		background: var(--pr);
		color: var(--onpr);
		font-weight: 800;
	}
	.group {
		font-size: 11px;
		line-height: 16px;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--mu);
		padding: 16px 10px 6px;
	}
	.nav {
		display: flex;
		align-items: center;
		gap: 10px;
		height: 38px;
		padding: 0 10px;
		border-radius: 8px;
		color: var(--tx);
		text-decoration: none;
		font-size: 14px;
		font-weight: 600;
	}
	.nav:hover {
		background: var(--sk);
		color: var(--tx);
	}
	.nav[aria-current='page'] {
		background: var(--prt);
		color: var(--pr);
	}
	.server {
		margin-top: auto;
		border: 1px solid var(--bd);
		border-radius: 10px;
		padding: 10px 12px;
		font-size: 13px;
		line-height: 18px;
		display: flex;
		flex-direction: column;
	}
	.server b {
		display: flex;
		align-items: center;
		gap: 6px;
		color: var(--se);
	}
	.server.off b {
		color: var(--er);
	}
	.main {
		min-width: 0;
		display: flex;
		flex-direction: column;
	}
	.top {
		position: sticky;
		top: 0;
		z-index: 20;
		height: 64px;
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 0 32px;
		background: var(--bg);
		border-bottom: 1px solid var(--bd);
	}
	.ws {
		display: flex;
		align-items: center;
		gap: 8px;
		font-family: var(--f-mono);
		font-size: 13px;
		color: var(--mu);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		min-width: 0;
	}
	.icon-btn {
		width: 36px;
		height: 36px;
		border-radius: 8px;
		border: 1px solid var(--bd);
		background: var(--sf);
		display: inline-grid;
		place-items: center;
		cursor: pointer;
		flex: none;
	}
	.menu-btn,
	.side-scrim {
		display: none;
	}
	.content {
		padding: 32px;
		width: 100%;
		max-width: 1264px;
		margin: 0 auto;
		display: flex;
		flex-direction: column;
		gap: 24px;
		outline: none;
	}
	.content.full {
		padding: 0;
		max-width: none;
		gap: 0;
	}
	.content.full > :global(.banner) {
		margin: 16px;
	}
	.gate {
		min-height: 100vh;
		display: grid;
		place-items: center;
		padding: 16px;
	}
	.gate .card {
		max-width: 480px;
		padding: 32px;
	}
	@media (max-width: 1023px) {
		.app {
			grid-template-columns: minmax(0, 1fr);
		}
		.side {
			position: fixed;
			left: 0;
			top: 0;
			bottom: 0;
			width: 272px;
			z-index: 70;
			transform: translateX(-100%);
			transition: transform 0.2s;
			box-shadow: var(--sh3);
		}
		.side.open {
			transform: none;
		}
		.side-scrim {
			display: block;
			position: fixed;
			inset: 0;
			z-index: 60;
			border: 0;
			background: var(--scrim);
		}
		.menu-btn {
			display: inline-grid;
		}
		.top {
			padding: 0 16px;
		}
		.content {
			padding: 24px 16px;
		}
		.content.full {
			padding: 0;
		}
	}
</style>
