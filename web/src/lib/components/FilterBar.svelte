<script lang="ts">
	import { api, type NameCount } from '#lib/api';
	import type { Filters } from '#lib/filters';
	import { app } from '#lib/state.svelte';
	import Key from './Key.svelte';
	import TagInput from './TagInput.svelte';

	let {
		filters = $bindable(),
		total = 0,
		showClassified = true,
		showOrder = true
	}: { filters: Filters; total?: number; showClassified?: boolean; showOrder?: boolean } = $props();

	let open = $state(false);
	let cameras = $state<NameCount[]>([]);
	let qText = $state(filters.q);
	let timer: ReturnType<typeof setTimeout>;

	$effect(() => {
		if (open && !cameras.length) api<NameCount[]>('/cameras').then((c) => (cameras = c)).catch(() => {});
	});

	function onq() {
		clearTimeout(timer);
		timer = setTimeout(() => (filters.q = qText.trim()), 250);
	}

	const kinds = [
		['', 'ALL'],
		['photo', 'PHOTO'],
		['video', 'VIDEO'],
		['live', 'LIVE'],
		['document', 'DOCS']
	];

	let active = $derived(
		[filters.library, filters.classified, filters.tags.length, filters.people.length, filters.camera, filters.rating, filters.duplicates, filters.offline, filters.from, filters.to].filter(Boolean).length
	);
</script>

<div class="screen bar">
	<div class="tabs" role="tablist">
		{#each kinds as [k, label] (k)}
			<button type="button" role="tab" aria-selected={filters.kind === k} class:on={filters.kind === k} onclick={() => (filters.kind = k)}>{label}</button>
		{/each}
		<span class="grow"></span>
		<span class="count"><span class="mute">MATCH</span> {total.toLocaleString()}</span>
	</div>
	<div class="row search">
		<span class="orange">&gt;</span>
		<input class="q" type="search" bind:value={qText} oninput={onq} placeholder="search title, description, tags, people, file name..." aria-label="Search" />
		<button type="button" class="flt" class:on={open || active > 0} onclick={() => (open = !open)}>
			FILTER{active ? ` ${active}` : ''}
		</button>
	</div>
	{#if open}
		<div class="grid">
			<label>
				<span class="label">LIBRARY</span>
				<select class="field" bind:value={filters.library}>
					<option value={0}>all libraries</option>
					{#each app.libraries as l (l.id)}<option value={l.id}>{l.name}</option>{/each}
				</select>
			</label>
			{#if showClassified}
				<label>
					<span class="label">STATE</span>
					<select class="field" bind:value={filters.classified}>
						<option value="">all</option>
						<option value="yes">classified</option>
						<option value="no">not classified</option>
					</select>
				</label>
			{/if}
			<label>
				<span class="label">CAMERA</span>
				<select class="field" bind:value={filters.camera}>
					<option value="">any</option>
					{#each cameras as c (c.name)}<option value={c.name}>{c.name} ({c.count})</option>{/each}
				</select>
			</label>
			<label>
				<span class="label">MIN RATING</span>
				<select class="field" bind:value={filters.rating}>
					{#each [0, 1, 2, 3, 4, 5] as r (r)}<option value={r}>{r ? '★'.repeat(r) : 'any'}</option>{/each}
				</select>
			</label>
			<label>
				<span class="label">FROM</span>
				<input class="field" type="date" bind:value={filters.from} />
			</label>
			<label>
				<span class="label">TO</span>
				<input class="field" type="date" bind:value={filters.to} />
			</label>
			<div class="wide">
				<span class="label">TAGS ({filters.tagMode === 'all' ? 'ALL OF' : 'ANY OF'})
					<button type="button" class="mini" onclick={() => (filters.tagMode = filters.tagMode === 'all' ? 'any' : 'all')}>SWITCH</button>
				</span>
				<TagInput bind:values={filters.tags} kind="tags" placeholder="tag" />
			</div>
			<div class="wide">
				<span class="label">PEOPLE</span>
				<TagInput bind:values={filters.people} kind="people" placeholder="person" accent="var(--blue)" />
			</div>
			<div class="wide row wrap">
				<Key size="sm" variant={filters.duplicates ? 'orange' : 'dark'} onclick={() => (filters.duplicates = !filters.duplicates)}>DUPLICATES</Key>
				<Key size="sm" variant={filters.offline ? 'orange' : 'dark'} onclick={() => (filters.offline = !filters.offline)}>OFFLINE</Key>
				{#if showOrder}
					<Key size="sm" variant="dark" onclick={() => (filters.order = filters.order === 'desc' ? 'asc' : 'desc')}>
						{filters.order === 'desc' ? 'NEWEST FIRST' : 'OLDEST FIRST'}
					</Key>
				{/if}
				<span class="grow"></span>
				<Key size="sm" variant="dark" onclick={() => {
					Object.assign(filters, { library: 0, classified: '', tags: [], people: [], camera: '', rating: 0, duplicates: false, offline: false, from: '', to: '' });
				}}>RESET</Key>
			</div>
		</div>
	{/if}
</div>

<style>
	.bar {
		display: flex;
		flex-direction: column;
		gap: 12px;
		margin-bottom: 22px;
	}

	.tabs {
		display: flex;
		align-items: center;
		gap: 4px;
		flex-wrap: wrap;
		font-size: 13px;
	}

	.tabs button {
		border: 0;
		background: none;
		color: var(--screen-mute);
		padding: 4px 10px;
		border-radius: 4px;
		cursor: pointer;
	}

	.tabs button:hover {
		color: var(--screen-ink);
	}

	.tabs button.on {
		background: var(--white);
		color: var(--ink);
	}

	.count {
		font-size: 13px;
	}

	.search {
		gap: 10px;
		font-size: 18px;
	}

	.q {
		flex: 1;
		min-width: 0;
		background: transparent;
		border: 0;
		outline: none;
		font-size: 17px;
		color: var(--screen-ink);
		padding: 4px 0;
	}

	.q::placeholder {
		color: var(--screen-mute);
	}

	.flt {
		border: 1px solid #2c2d2f;
		background: var(--screen-faint);
		color: var(--screen-dim);
		border-radius: 5px;
		padding: 5px 12px;
		font-size: 12px;
		cursor: pointer;
	}

	.flt.on {
		color: var(--orange);
		border-color: var(--orange-dim);
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(190px, 1fr));
		gap: 12px;
		padding-top: 12px;
		border-top: 1px solid var(--screen-faint);
	}

	label,
	.wide {
		display: flex;
		flex-direction: column;
		gap: 5px;
	}

	.wide {
		grid-column: 1 / -1;
	}

	.wide.row {
		flex-direction: row;
	}

	.mini {
		border: 0;
		background: none;
		color: var(--orange);
		font-size: 10px;
		cursor: pointer;
	}
</style>
