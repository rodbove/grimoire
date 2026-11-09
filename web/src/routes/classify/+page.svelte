<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type Asset, type NameCount } from '#lib/api';
	import { draftFrom, patchFrom } from '#lib/draft';
	import { fmtDate, fmtDuration, fmtSize } from '#lib/format';
	import { app, toast } from '#lib/state.svelte';
	import Key from '#lib/components/Key.svelte';
	import Media from '#lib/components/Media.svelte';
	import MetaForm, { type Draft } from '#lib/components/MetaForm.svelte';

	const padKeys = ['1', '2', '3', '4', 'Q', 'W', 'E', 'R', 'A', 'S', 'D', 'F', 'Z', 'X', 'C', 'V'];

	let asset = $state<Asset | null>(null);
	let draft = $state<Draft | null>(null);
	let remaining = $state(0);
	let loading = $state(false);
	let done = $state(false);
	let saving = $state(false);
	let library = $state(0);
	let kind = $state('');
	let order = $state('oldest');
	let skipped = $state<number[]>([]);
	let history = $state<number[]>([]);
	let topTags = $state<NameCount[]>([]);
	let sessionCount = $state(0);

	async function loadTags() {
		try {
			topTags = await api<NameCount[]>('/tags?prefix=&limit=16');
		} catch {
			topTags = [];
		}
	}

	async function next() {
		loading = true;
		try {
			const p = new URLSearchParams({ order });
			if (library) p.set('library', String(library));
			if (kind) p.set('kind', kind);
			if (skipped.length) p.set('exclude', skipped.slice(-400).join(','));
			const r = await api<{ asset: Asset | null; remaining: number }>(`/classify/next?${p}`);
			remaining = r.remaining;
			asset = r.asset;
			draft = r.asset ? draftFrom(r.asset) : null;
			done = !r.asset;
		} catch (e) {
			toast((e as Error).message, true);
		} finally {
			loading = false;
		}
	}

	async function open(id: number) {
		try {
			const a = await api<Asset>(`/assets/${id}`);
			asset = a;
			draft = draftFrom(a);
			done = false;
		} catch (e) {
			toast((e as Error).message, true);
		}
	}

	async function saveNext() {
		if (!asset || !draft || saving) return;
		saving = true;
		try {
			await api(`/assets/${asset.id}`, { method: 'PATCH', body: patchFrom(draft, asset, true) });
			history = [...history, asset.id];
			sessionCount++;
			if (draft.tags.some((t) => !topTags.some((x) => x.name.toLowerCase() === t.toLowerCase()))) loadTags();
			await next();
		} catch (e) {
			toast((e as Error).message, true);
		} finally {
			saving = false;
		}
	}

	async function skip() {
		if (!asset) return;
		skipped.push(asset.id);
		history = [...history, asset.id];
		await next();
	}

	async function back() {
		const id = history.at(-1);
		if (id == null) return;
		history = history.slice(0, -1);
		skipped = skipped.filter((x) => x !== id);
		await open(id);
	}

	function toggleTag(name: string) {
		if (!draft) return;
		const has = draft.tags.some((t) => t.toLowerCase() === name.toLowerCase());
		draft.tags = has ? draft.tags.filter((t) => t.toLowerCase() !== name.toLowerCase()) : [...draft.tags, name];
	}

	function restart() {
		skipped = [];
		history = [];
		next();
	}

	onMount(() => {
		loadTags();
		next();
	});

	function onkeydown(e: KeyboardEvent) {
		const t = e.target as HTMLElement;
		const typing = t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT';
		if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
			e.preventDefault();
			saveNext();
		} else if ((e.ctrlKey || e.metaKey) && e.key === 'ArrowRight') {
			e.preventDefault();
			skip();
		} else if ((e.ctrlKey || e.metaKey) && e.key === 'ArrowLeft') {
			e.preventDefault();
			back();
		} else if (e.key === 'Escape' && typing) {
			t.blur();
		} else if (!typing && !e.ctrlKey && !e.metaKey && !e.altKey) {
			const i = padKeys.indexOf(e.key.toUpperCase());
			if (i >= 0 && topTags[i]) {
				e.preventDefault();
				toggleTag(topTags[i].name);
			} else if (e.key === 't') {
				e.preventDefault();
				document.querySelector<HTMLInputElement>('[id^="tags-"]')?.focus();
			} else if (e.key === 'n') {
				e.preventDefault();
				document.querySelector<HTMLInputElement>('[id^="title-"]')?.focus();
			}
		}
	}
</script>

<svelte:window {onkeydown} />
<svelte:head><title>classify · grimoire</title></svelte:head>

<div class="screen top">
	<div class="tabs">
		{#each [['', 'ALL'], ['photo', 'PHOTO'], ['video', 'VIDEO'], ['live', 'LIVE'], ['document', 'DOCS']] as [k, label] (k)}
			<button type="button" class:on={kind === k} onclick={() => { kind = k; restart(); }}>{label}</button>
		{/each}
	</div>
	<span class="grow"></span>
	<select class="field small" bind:value={library} onchange={restart} aria-label="Library">
		<option value={0}>all libraries</option>
		{#each app.libraries as l (l.id)}<option value={l.id}>{l.name}</option>{/each}
	</select>
	<select class="field small" bind:value={order} onchange={restart} aria-label="Order">
		<option value="oldest">oldest first</option>
		<option value="newest">newest first</option>
		<option value="folder">by folder</option>
		<option value="random">random</option>
	</select>
	<span class="stat"><span class="mute">LEFT</span> <span class="orange">{remaining.toLocaleString()}</span></span>
	<span class="stat"><span class="mute">DONE</span> {sessionCount}</span>
</div>

{#if done}
	<div class="screen empty">
		<p class="orange">ALL CLEAR</p>
		<p>No file without metadata matches this filter.</p>
		{#if skipped.length}<Key size="sm" onclick={restart}>SHOW {skipped.length} SKIPPED AGAIN</Key>{/if}
	</div>
{:else if asset && draft}
	<div class="layout">
		<section class="left">
			<div class="screen stage">
				<div class="meta row">
					<span class="ellipsis grow">{asset.files[0]?.library}/{asset.files[0]?.path}</span>
					{#if asset.classified}<span class="orange">CLASSIFIED</span>{/if}
				</div>
				<div class="view"><Media {asset} /></div>
				<div class="meta row wrap">
					<span>{fmtDate(asset.takenAt ?? asset.sortAt, true)}</span>
					{#if !asset.takenAt}<span class="mute">FILE TIME</span>{/if}
					{#if asset.width}<span class="dim">{asset.width}×{asset.height}</span>{/if}
					{#if asset.durationMs}<span class="dim">{fmtDuration(asset.durationMs)}</span>{/if}
					<span class="dim">{fmtSize(asset.size)}</span>
					{#if asset.cameraModel}<span class="dim">{asset.cameraModel}</span>{/if}
					{#if asset.lat != null}<span class="dim">GPS {asset.lat.toFixed(3)}, {asset.lon?.toFixed(3)}</span>{/if}
				</div>
			</div>
			<div class="transport">
				<Key variant="dark" size="lg" onclick={back} disabled={!history.length} title="Ctrl+Left">◂ BACK</Key>
				<Key variant="dark" size="lg" onclick={skip} disabled={loading} title="Ctrl+Right">SKIP ▸</Key>
				<Key variant="orange" size="lg" onclick={saveNext} disabled={saving || loading} title="Ctrl+Enter" wide>SAVE + NEXT</Key>
			</div>
		</section>
		<section class="right">
			<div class="screen">
				<MetaForm bind:draft />
			</div>
			{#if topTags.length}
				<div>
					<span class="label">QUICK TAGS</span>
					<div class="pads">
						{#each topTags as t, i (t.name)}
							{@const on = draft.tags.some((x) => x.toLowerCase() === t.name.toLowerCase())}
							<button type="button" class="pad" class:on onclick={() => toggleTag(t.name)}>
								<span class="row"><span class="grow">{String(i + 1).padStart(2, '0')}</span><span class="k">{padKeys[i]}</span></span>
								<span class="ellipsis">{t.name}</span>
								{#if on}<i class="led"></i>{/if}
							</button>
						{/each}
					</div>
				</div>
			{/if}
			<p class="keys label">CTRL+ENTER SAVE · CTRL+→ SKIP · CTRL+← BACK · T TAGS · N TITLE · PAD KEYS TOGGLE TAGS</p>
		</section>
	</div>
{:else}
	<p class="loading">LOADING...</p>
{/if}

<style>
	.top {
		display: flex;
		align-items: center;
		gap: 14px;
		flex-wrap: wrap;
		margin-bottom: 20px;
		padding: 10px 14px;
	}

	.tabs {
		display: flex;
		gap: 4px;
		flex-wrap: wrap;
	}

	.tabs button {
		border: 0;
		background: none;
		color: var(--screen-mute);
		padding: 4px 10px;
		border-radius: 4px;
		cursor: pointer;
		font-size: 13px;
	}

	.tabs button.on {
		background: var(--white);
		color: var(--ink);
	}

	.small {
		width: auto;
		padding: 5px 30px 5px 9px;
		font-size: 12px;
	}

	.stat {
		font-size: 13px;
	}

	.layout {
		display: grid;
		grid-template-columns: minmax(0, 1.5fr) minmax(340px, 1fr);
		gap: 24px;
		align-items: start;
	}

	.left {
		display: flex;
		flex-direction: column;
		gap: 18px;
		position: sticky;
		top: 12px;
	}

	.stage {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	.view {
		height: max(300px, calc(100vh - 470px));
	}

	.meta {
		font-size: 12px;
		gap: 14px;
	}

	.transport {
		display: grid;
		grid-template-columns: auto auto 1fr;
		gap: 12px;
	}

	.right {
		display: flex;
		flex-direction: column;
		gap: 18px;
	}

	.pads {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 10px;
		margin-top: 6px;
	}

	.pad {
		position: relative;
		display: flex;
		flex-direction: column;
		justify-content: space-between;
		gap: 10px;
		height: 66px;
		padding: 7px 9px;
		border: 0;
		border-radius: 8px;
		background: linear-gradient(180deg, var(--pad-hi), var(--pad) 55%);
		box-shadow:
			0 3px 0 var(--pad-shadow),
			0 5px 8px rgba(0, 0, 0, 0.25),
			inset 0 1px 0 rgba(255, 255, 255, 0.8);
		cursor: pointer;
		text-align: left;
		font-size: 12px;
	}

	.pad.on {
		outline: 3px solid var(--orange);
		outline-offset: -1px;
	}

	.pad:active {
		transform: translateY(2px);
	}

	.k {
		color: var(--dim-ink);
		font-size: 11px;
	}

	.led {
		position: absolute;
		right: 9px;
		bottom: 9px;
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: var(--orange);
		box-shadow: 0 0 6px var(--orange);
	}

	.keys {
		line-height: 1.7;
	}

	.empty,
	.loading {
		text-align: center;
		padding: 50px 20px;
	}

	.loading {
		color: var(--dim-ink);
	}

	@media (max-width: 1000px) {
		.layout {
			grid-template-columns: 1fr;
		}

		.left {
			position: static;
		}

		.view {
			height: 52vh;
		}
	}

	@media (max-width: 520px) {
		.transport {
			grid-template-columns: 1fr 1fr;
		}

		.transport :global(.key:last-child) {
			grid-column: 1 / -1;
		}

		.pads {
			grid-template-columns: repeat(3, 1fr);
		}
	}
</style>
