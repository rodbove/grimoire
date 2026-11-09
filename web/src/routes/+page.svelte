<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import type { Summary } from '#lib/api';
	import { fromUrl, toParams, toUrl, type Filters } from '#lib/filters';
	import { app } from '#lib/state.svelte';
	import BulkBar from '#lib/components/BulkBar.svelte';
	import FilterBar from '#lib/components/FilterBar.svelte';
	import Grid from '#lib/components/Grid.svelte';
	import Viewer from '#lib/components/Viewer.svelte';

	let filters = $state<Filters>(fromUrl(page.url.searchParams));
	let items = $state<Summary[]>([]);
	let total = $state(0);
	let selected = $state(new Set<number>());
	let viewIndex = $state(-1);
	let grid = $state<Grid>()!;

	let query = $derived(toParams(filters).toString());

	$effect(() => {
		const url = toUrl(filters);
		if (url !== page.url.search) goto(url || '/', { replace: true, shallow: true, reset: false });
	});
</script>

<svelte:head><title>grimoire</title></svelte:head>

<FilterBar bind:filters {total} />

{#if !app.libraries.length}
	<div class="screen hint">
		<p class="orange">NO LIBRARY YET</p>
		<p>Open <a href="/libraries">LIBRARY</a> and add a folder. Grimoire indexes the files in it.</p>
	</div>
{/if}

<Grid bind:this={grid} {query} bind:items bind:total bind:selected refresh={app.refresh} onopen={(i) => (viewIndex = i)} />

{#if selected.size}
	<BulkBar bind:selected onselectevent={() => grid.selectEvent()} onselectall={() => grid.selectLoaded()} ondone={() => grid.reload()} />
{/if}

{#if viewIndex >= 0 && items[viewIndex]}
	<Viewer
		bind:items
		bind:index={viewIndex}
		onclose={() => (viewIndex = -1)}
		onchanged={(removed) => {
			if (removed) total--;
		}}
	/>
{/if}

<style>
	.hint {
		margin-bottom: 20px;
	}

	.hint a {
		color: var(--orange);
	}
</style>
