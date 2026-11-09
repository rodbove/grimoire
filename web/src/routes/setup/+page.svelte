<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '#lib/api';
	import { app, pendingJobs, toast } from '#lib/state.svelte';
	import Key from '#lib/components/Key.svelte';

	type Failed = { id: number; type: string; assetId: number; fileId: number; error: string };

	let system = $state<{ tools: Record<string, boolean>; dataDir: string; workers: number } | null>(null);
	let failed = $state<Failed[]>([]);
	let current = $state('');
	let next = $state('');

	async function load() {
		try {
			[system, failed] = await Promise.all([api<typeof system>('/system'), api<Failed[]>('/jobs/failed')]);
		} catch (e) {
			toast((e as Error).message, true);
		}
	}

	async function retry() {
		await api('/jobs/retry', { method: 'POST' });
		toast('RETRYING');
		load();
	}

	async function changePassword(e: SubmitEvent) {
		e.preventDefault();
		try {
			await api('/auth/password', { method: 'POST', body: { password: current, newPassword: next } });
			current = next = '';
			toast('PASSWORD CHANGED');
		} catch (err) {
			toast((err as Error).message, true);
		}
	}

	async function logout() {
		await api('/auth/logout', { method: 'POST' });
		app.authenticated = false;
	}

	onMount(load);
</script>

<svelte:head><title>setup · grimoire</title></svelte:head>

<div class="cols">
	<section class="screen">
		<h3 class="orange">SYSTEM</h3>
		{#if system}
			<dl>
				<dt>DATA</dt><dd class="ellipsis">{system.dataDir}</dd>
				<dt>WORKERS</dt><dd>{system.workers}</dd>
				<dt>FILES</dt><dd>{app.status.assets.toLocaleString()}</dd>
				<dt>QUEUE</dt><dd>{pendingJobs(app.status)} {Object.entries(app.status.jobs).map(([k, v]) => `${k}:${v}`).join(' ')}</dd>
			</dl>
			<h4 class="label">TOOLS</h4>
			<ul class="tools">
				{#each Object.entries(system.tools) as [name, ok] (name)}
					<li><span class="led" class:on={ok}></span>{name}<span class="dim">{ok ? 'FOUND' : 'MISSING'}</span></li>
				{/each}
			</ul>
			{#if !system.tools.exiftool}
				<p class="dim note">Without exiftool, HEIC dates, Live Photo pairs and RAW previews are limited. Put exiftool in the bin folder next to grimoire.</p>
			{/if}
		{/if}
	</section>

	<section class="screen">
		<div class="row">
			<h3 class="orange grow">FAILED JOBS {failed.length}</h3>
			{#if failed.length}<Key size="sm" onclick={retry}>RETRY ALL</Key>{/if}
		</div>
		<ul class="failed">
			{#each failed as f (f.id)}
				<li>
					<span>{f.type.toUpperCase()} {f.assetId ? `#${f.assetId}` : `file ${f.fileId}`}</span>
					<span class="dim">{f.error}</span>
				</li>
			{:else}
				<li class="dim">NONE</li>
			{/each}
		</ul>
	</section>

	<section class="screen">
		<h3 class="orange">PASSWORD</h3>
		<form class="pw" onsubmit={changePassword}>
			<input class="field" type="password" bind:value={current} placeholder="current password" autocomplete="current-password" />
			<input class="field" type="password" bind:value={next} placeholder="new password" autocomplete="new-password" />
			<div class="row">
				<Key size="sm" type="submit" disabled={!current || next.length < 6}>CHANGE</Key>
				<span class="grow"></span>
				<Key size="sm" variant="dark" onclick={logout}>LOG OUT</Key>
			</div>
		</form>
	</section>
</div>

<style>
	.cols {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
		gap: 20px;
		align-items: start;
	}

	h3 {
		margin: 0 0 12px;
		font-size: 14px;
		font-weight: 500;
	}

	h4 {
		margin: 16px 0 8px;
		font-weight: normal;
	}

	dl {
		display: grid;
		grid-template-columns: 80px 1fr;
		gap: 6px;
		margin: 0;
		font-size: 12px;
	}

	dt {
		color: var(--screen-mute);
	}

	dd {
		margin: 0;
		min-width: 0;
	}

	ul {
		list-style: none;
		margin: 0;
		padding: 0;
		font-size: 12px;
	}

	.tools li {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 4px 0;
	}

	.tools .dim {
		margin-left: auto;
	}

	.led {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: var(--orange);
	}

	.led.on {
		background: var(--green);
		box-shadow: 0 0 6px var(--green);
	}

	.note {
		font-size: 11px;
		line-height: 1.5;
	}

	.failed {
		max-height: 340px;
		overflow: auto;
	}

	.failed li {
		display: flex;
		flex-direction: column;
		gap: 3px;
		padding: 7px 0;
		border-top: 1px solid var(--screen-faint);
		word-break: break-word;
	}

	.pw {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
</style>
