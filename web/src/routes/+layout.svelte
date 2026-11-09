<script lang="ts">
	import '../app.css';
	import { onMount, type Snippet } from 'svelte';
	import { page } from '$app/state';
	import { api } from '#lib/api';
	import { app, connectEvents, loadLibraries, pendingJobs, toasts } from '#lib/state.svelte';
	import Grille from '#lib/components/Grille.svelte';
	import Key from '#lib/components/Key.svelte';
	import Seg from '#lib/components/Seg.svelte';
	import Upload from '#lib/components/Upload.svelte';

	let { children }: { children: Snippet } = $props();

	let password = $state('');
	let password2 = $state('');
	let authError = $state('');
	let dragging = $state(false);
	let dragDepth = 0;

	async function checkAuth() {
		const s = await api<{ setUp: boolean; authenticated: boolean }>('/auth/state');
		app.setUp = s.setUp;
		app.authenticated = s.authenticated;
		app.ready = true;
	}

	onMount(() => {
		checkAuth().catch(() => (app.ready = true));
	});

	$effect(() => {
		if (!app.authenticated) return;
		loadLibraries().catch(() => {});
		return connectEvents();
	});

	async function submitAuth(e: SubmitEvent) {
		e.preventDefault();
		authError = '';
		try {
			if (!app.setUp) {
				if (password !== password2) {
					authError = 'PASSWORDS DO NOT MATCH';
					return;
				}
				await api('/auth/setup', { method: 'POST', body: { password } });
			} else {
				await api('/auth/login', { method: 'POST', body: { password } });
			}
			password = password2 = '';
			await checkAuth();
		} catch (err) {
			authError = (err as Error).message.toUpperCase();
		}
	}

	let path = $derived(page.url.pathname);
	let jobs = $derived(pendingJobs(app.status));
	let lcdValue = $derived(path.startsWith('/classify') ? app.status.unclassified : app.status.assets);
	let lcdLabel = $derived(path.startsWith('/classify') ? 'TO CLASSIFY' : 'FILES');

	function hasFiles(e: DragEvent) {
		return e.dataTransfer?.types.includes('Files');
	}

	function ondragenter(e: DragEvent) {
		if (!hasFiles(e) || !app.authenticated) return;
		dragDepth++;
		dragging = true;
	}

	function ondragleave() {
		dragDepth = Math.max(0, dragDepth - 1);
		if (!dragDepth) dragging = false;
	}

	function ondrop(e: DragEvent) {
		if (!hasFiles(e) || !app.authenticated) return;
		e.preventDefault();
		dragDepth = 0;
		dragging = false;
		app.droppedFiles = Array.from(e.dataTransfer?.files ?? []);
		app.uploadOpen = true;
	}
</script>

<svelte:window {ondragenter} {ondragleave} ondragover={(e) => hasFiles(e) && e.preventDefault()} {ondrop} />

<div class="device">
	<header class="head">
		<a class="brand" href="/">
			<span class="logo">grimoire</span>
			<span class="label">MEDIA LIBRARY G-1</span>
		</a>
		<div class="grille"><Grille cols={10} rows={4} /></div>
		<div class="lcd">
			<Seg value={app.ready && app.authenticated ? lcdValue : '----'} digits={6} />
			<div class="lcd-info">
				<span class="orange">{lcdLabel}</span>
				<span>{jobs ? `JOBS ${jobs}` : 'IDLE'}</span>
				<span class="leds">
					<i class:on={app.status.scanning.length > 0} title="Scanning"></i>
					<i class:on={jobs > 0} title="Processing"></i>
					<i class:on={app.status.failed > 0} class="warn" title="Failed jobs"></i>
				</span>
			</div>
		</div>
		{#if app.authenticated}
			<nav class="top">
				<Key onclick={() => (app.uploadOpen = true)}>UPLOAD</Key>
				<Key href="/libraries" active={path.startsWith('/libraries')}>LIBRARY</Key>
				<Key href="/trash" active={path.startsWith('/trash')}>TRASH</Key>
				<Key href="/setup" active={path.startsWith('/setup')} led={app.status.failed > 0}>SETUP</Key>
			</nav>
		{/if}
	</header>

	{#if !app.ready}
		<p class="boot">BOOTING...</p>
	{:else if !app.authenticated}
		<form class="auth screen" onsubmit={submitAuth}>
			<p class="orange">{app.setUp ? 'ENTER PASSWORD' : 'FIRST START: SET A PASSWORD'}</p>
			<input class="field" type="password" bind:value={password} placeholder="password" autocomplete={app.setUp ? 'current-password' : 'new-password'} />
			{#if !app.setUp}
				<input class="field" type="password" bind:value={password2} placeholder="repeat password" autocomplete="new-password" />
			{/if}
			{#if authError}<p class="err">{authError}</p>{/if}
			<Key type="submit" variant="orange" wide>{app.setUp ? 'UNLOCK' : 'SET PASSWORD'}</Key>
		</form>
	{:else}
		<nav class="modes">
			<span class="label">MODE</span>
			<div class="mode-keys">
				<Key variant="dark" href="/" active={path === '/'} size="lg">GRID</Key>
				<Key variant="dark" href="/classify" active={path.startsWith('/classify')} size="lg" led={app.status.unclassified > 0}>CLASSIFY</Key>
				<Key variant="dark" href="/folders" active={path.startsWith('/folders')} size="lg">FOLDERS</Key>
			</div>
		</nav>
		<main>
			{@render children()}
		</main>
	{/if}
</div>

{#if app.uploadOpen}
	<Upload />
{/if}

{#if dragging}
	<div class="dropzone"><span>DROP TO UPLOAD</span></div>
{/if}

<div class="toasts" aria-live="polite">
	{#each toasts.items as t (t.id)}
		<div class="toast" class:error={t.error}>{t.text}</div>
	{/each}
</div>

<style>
	.device {
		min-height: 100vh;
		max-width: 1680px;
		margin: 0 auto;
		padding: 22px 28px 40px;
		background: var(--body);
		box-shadow:
			inset 0 0 0 2px var(--edge),
			0 0 0 1px rgba(0, 0, 0, 0.25);
	}

	@media (min-width: 1740px) {
		.device {
			margin: 24px auto;
			min-height: calc(100vh - 48px);
			border-radius: 28px;
		}
	}

	.head {
		display: flex;
		align-items: center;
		gap: 26px;
		flex-wrap: wrap;
	}

	.brand {
		display: flex;
		flex-direction: column;
		text-decoration: none;
		color: var(--ink);
	}

	.logo {
		font-family: var(--sans);
		font-weight: 300;
		font-size: 46px;
		line-height: 1;
		letter-spacing: -0.02em;
	}

	.brand .label {
		margin-top: 10px;
	}

	.lcd {
		display: flex;
		align-items: center;
		gap: 16px;
		padding: 10px 18px;
		border-radius: 10px;
		background: var(--screen);
		box-shadow:
			inset 0 0 0 4px #050506,
			inset 0 2px 10px rgba(0, 0, 0, 0.9),
			0 1px 0 rgba(255, 255, 255, 0.3);
	}

	.lcd-info {
		display: flex;
		flex-direction: column;
		gap: 3px;
		font-size: 11px;
		color: var(--screen-ink);
		min-width: 96px;
	}

	.leds {
		display: flex;
		gap: 6px;
		margin-top: 2px;
	}

	.leds i {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--lcd-off);
	}

	.leds i.on {
		background: var(--orange);
		box-shadow: 0 0 6px var(--orange);
	}

	.top {
		margin-left: auto;
		display: flex;
		gap: 12px;
		flex-wrap: wrap;
	}

	.modes {
		margin: 26px 0 18px;
	}

	.mode-keys {
		display: flex;
		gap: 12px;
		margin-top: 6px;
		flex-wrap: wrap;
	}

	.boot {
		padding: 80px 0;
		text-align: center;
		color: var(--dim-ink);
	}

	.auth {
		max-width: 380px;
		margin: 80px auto;
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 24px;
	}

	.auth p {
		margin: 0;
	}

	.err {
		color: var(--orange);
		font-size: 12px;
	}

	.dropzone {
		position: fixed;
		inset: 0;
		z-index: 70;
		display: grid;
		place-items: center;
		background: rgba(13, 14, 15, 0.7);
		pointer-events: none;
	}

	.dropzone span {
		padding: 30px 50px;
		border: 2px dashed var(--orange);
		border-radius: 14px;
		color: var(--orange);
		font-size: 20px;
		letter-spacing: 0.1em;
	}

	.toasts {
		position: fixed;
		right: 16px;
		bottom: 16px;
		z-index: 80;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.toast {
		padding: 10px 14px;
		border-radius: 8px;
		background: var(--screen);
		color: var(--screen-ink);
		font-size: 12px;
		letter-spacing: 0.04em;
		box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
		border-left: 3px solid var(--green);
	}

	.toast.error {
		border-left-color: var(--orange);
	}

	@media (max-width: 1100px) {
		.grille {
			display: none;
		}
	}

	@media (max-width: 720px) {
		.device {
			padding: 14px 16px 30px;
		}

		.head {
			gap: 14px;
		}

		.logo {
			font-size: 34px;
		}

		.top {
			margin-left: 0;
			width: 100%;
		}

		.top :global(.key) {
			flex: 1;
			min-width: 0;
			padding: 0 6px;
		}

		.mode-keys :global(.key) {
			flex: 1;
			min-width: 0;
		}
	}
</style>
