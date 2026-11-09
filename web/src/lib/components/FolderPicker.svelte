<script lang="ts">
	import { api } from '#lib/api';
	import { app } from '#lib/state.svelte';

	let {
		library = $bindable(0),
		folder = $bindable(''),
		onlyUploadTargets = false
	}: { library?: number; folder?: string; onlyUploadTargets?: boolean } = $props();

	let subs = $state<string[]>([]);
	let libs = $derived(app.libraries.filter((l) => l.status === 'online' && (!onlyUploadTargets || l.isUploadTarget)));

	$effect(() => {
		if (!library && libs.length) library = libs[0].id;
	});

	$effect(() => {
		const lib = library;
		const f = folder;
		if (!lib) return;
		api<string[]>(`/libraries/${lib}/folders?path=${encodeURIComponent(f)}`)
			.then((r) => (subs = r))
			.catch(() => (subs = []));
	});

	let crumbs = $derived(folder ? folder.split('/') : []);
</script>

<div class="picker">
	<select class="field" bind:value={library} onchange={() => (folder = '')}>
		{#each libs as l (l.id)}
			<option value={l.id}>{l.name}</option>
		{/each}
	</select>
	<div class="crumbs">
		<button type="button" onclick={() => (folder = '')}>/</button>
		{#each crumbs as c, i (i)}
			<button type="button" onclick={() => (folder = crumbs.slice(0, i + 1).join('/'))}>{c}/</button>
		{/each}
	</div>
	<div class="subs">
		{#each subs as s (s)}
			<button type="button" onclick={() => (folder = folder ? folder + '/' + s : s)}>▸ {s}</button>
		{:else}
			<span class="mute">no subfolders</span>
		{/each}
	</div>
	<input class="field" bind:value={folder} placeholder="folder (empty = auto YYYY/MM)" />
</div>

<style>
	.picker {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.crumbs {
		display: flex;
		flex-wrap: wrap;
		gap: 2px;
		font-size: 12px;
	}

	.crumbs button,
	.subs button {
		border: 0;
		background: none;
		cursor: pointer;
		color: var(--orange);
		padding: 2px 4px;
	}

	.subs {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		max-height: 160px;
		overflow: auto;
		padding: 6px;
		border-radius: 6px;
		background: var(--screen-faint);
		font-size: 12px;
	}

	.subs button {
		color: var(--screen-ink);
	}

	.subs button:hover {
		color: var(--orange);
	}
</style>
