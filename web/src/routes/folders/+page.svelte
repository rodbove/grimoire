<script lang="ts">
	import { api, type Summary } from '#lib/api';
	import { emptyFilters, toParams } from '#lib/filters';
	import { app } from '#lib/state.svelte';
	import BulkBar from '#lib/components/BulkBar.svelte';
	import Grid from '#lib/components/Grid.svelte';
	import Key from '#lib/components/Key.svelte';
	import Viewer from '#lib/components/Viewer.svelte';

	let library = $state(0);
	let folder = $state('');
	let subs = $state<string[]>([]);
	let items = $state<Summary[]>([]);
	let total = $state(0);
	let selected = $state(new Set<number>());
	let viewIndex = $state(-1);
	let grid = $state<Grid>()!;

	let online = $derived(app.libraries.filter((l) => l.status === 'online'));

	$effect(() => {
		if (!library && online.length) library = online[0].id;
	});

	$effect(() => {
		const lib = library;
		const f = folder;
		app.refresh;
		if (!lib) return;
		api<{ folders: string[] }>(`/browse?library=${lib}&path=${encodeURIComponent(f)}`)
			.then((r) => (subs = r.folders))
			.catch(() => (subs = []));
	});

	let query = $derived.by(() => {
		const f = emptyFilters();
		f.library = library;
		f.folder = folder;
		f.direct = true;
		f.order = 'asc';
		return toParams(f).toString();
	});

	let crumbs = $derived(folder ? folder.split('/') : []);
</script>

<svelte:head><title>folders · grimoire</title></svelte:head>

<div class="libs">
	<span class="label">LIBRARY</span>
	<div class="row wrap">
		{#each app.libraries as l (l.id)}
			<Key variant="light" led={l.id === library} active={l.id === library} disabled={l.status !== 'online'} onclick={() => { library = l.id; folder = ''; }}>
				{l.name}
			</Key>
		{/each}
	</div>
</div>

<div class="screen path">
	<div class="crumbs">
		<button type="button" onclick={() => (folder = '')}>{app.libraries.find((l) => l.id === library)?.name ?? '/'}</button>
		{#each crumbs as c, i (i)}
			<span class="mute">/</span>
			<button type="button" onclick={() => (folder = crumbs.slice(0, i + 1).join('/'))}>{c}</button>
		{/each}
		<span class="grow"></span>
		<span class="mute">{total} HERE</span>
	</div>
	{#if subs.length}
		<div class="subs">
			{#each subs as s (s)}
				<button type="button" class="folder" onclick={() => (folder = folder ? `${folder}/${s}` : s)}>
					<span class="tab"></span>
					<span class="ellipsis">{s}</span>
				</button>
			{/each}
		</div>
	{/if}
</div>

{#if library}
	<Grid bind:this={grid} {query} bind:items bind:total bind:selected refresh={app.refresh} onopen={(i) => (viewIndex = i)} minTile={150} />
{/if}

{#if selected.size}
	<BulkBar bind:selected onselectevent={() => grid.selectEvent()} onselectall={() => grid.selectLoaded()} ondone={() => grid.reload()} />
{/if}

{#if viewIndex >= 0 && items[viewIndex]}
	<Viewer bind:items bind:index={viewIndex} onclose={() => (viewIndex = -1)} onchanged={(r) => r && total--} />
{/if}

<style>
	.libs {
		margin-bottom: 18px;
	}

	.libs .row {
		margin-top: 6px;
		gap: 12px;
	}

	.path {
		margin-bottom: 22px;
	}

	.crumbs {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
		font-size: 14px;
	}

	.crumbs button {
		border: 0;
		background: none;
		color: var(--screen-ink);
		cursor: pointer;
		padding: 2px 4px;
	}

	.crumbs button:hover {
		color: var(--orange);
	}

	.subs {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(170px, 1fr));
		gap: 10px;
		margin-top: 14px;
	}

	.folder {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 10px 12px;
		border: 1px solid #2c2d2f;
		border-radius: 6px;
		background: var(--screen-faint);
		color: var(--screen-ink);
		cursor: pointer;
		text-align: left;
		font-size: 13px;
	}

	.folder:hover {
		border-color: var(--orange);
	}

	.tab {
		flex: none;
		width: 16px;
		height: 12px;
		border-radius: 2px;
		background: var(--orange);
		box-shadow: inset 0 3px 0 rgba(0, 0, 0, 0.25);
	}
</style>
