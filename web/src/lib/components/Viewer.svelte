<script lang="ts">
	import { api, originalUrl, type Asset, type Summary } from '#lib/api';
	import { draftFrom, patchFrom, sameDraft } from '#lib/draft';
	import { fmtDate, fmtDuration, fmtSize, pad4 } from '#lib/format';
	import { app, toast } from '#lib/state.svelte';
	import FolderPicker from './FolderPicker.svelte';
	import Key from './Key.svelte';
	import Media from './Media.svelte';
	import MetaForm, { type Draft } from './MetaForm.svelte';

	let {
		items = $bindable(),
		index = $bindable(),
		onclose,
		onchanged
	}: {
		items: Summary[];
		index: number;
		onclose: () => void;
		onchanged: (removedId?: number) => void;
	} = $props();

	let asset = $state<Asset | null>(null);
	let draft = $state<Draft | null>(null);
	let base: Draft | null = null;
	let panel = $state(typeof window !== 'undefined' && window.innerWidth > 900);
	let saving = $state(false);
	let mode = $state<'' | 'rename' | 'move'>('');
	let newName = $state('');
	let moveLib = $state(0);
	let moveFolder = $state('');

	let current = $derived(items[index]);
	let dirty = $derived(!!draft && !!base && !sameDraft(draft, base));
	let file = $derived(asset?.files.find((f) => f.status === 'ok'));

	$effect(() => {
		const id = current?.id;
		if (!id) return;
		mode = '';
		api<Asset>(`/assets/${id}`)
			.then((a) => {
				if (current?.id !== a.id) return;
				asset = a;
				draft = draftFrom(a);
				base = draftFrom(a);
			})
			.catch((e) => toast(e.message, true));
	});

	async function save(classified?: boolean) {
		if (!asset || !draft) return;
		saving = true;
		try {
			const a = await api<Asset>(`/assets/${asset.id}`, { method: 'PATCH', body: patchFrom(draft, asset, classified) });
			asset = a;
			draft = draftFrom(a);
			base = draftFrom(a);
			Object.assign(items[index], { title: a.title, rating: a.rating, classified: a.classified, sortAt: a.sortAt, v: a.v });
			toast('SAVED');
			onchanged();
		} catch (e) {
			toast((e as Error).message, true);
		} finally {
			saving = false;
		}
	}

	function go(d: number) {
		const n = index + d;
		if (n >= 0 && n < items.length) index = n;
	}

	async function doRename() {
		if (!file) return;
		try {
			await api(`/files/${file.id}/rename`, { method: 'POST', body: { name: newName } });
			mode = '';
			toast('RENAMED');
			const a = await api<Asset>(`/assets/${asset!.id}`);
			asset = a;
			items[index].name = a.name;
		} catch (e) {
			toast((e as Error).message, true);
		}
	}

	async function doMove() {
		if (!file) return;
		try {
			await api(`/files/${file.id}/move`, { method: 'POST', body: { libraryId: moveLib, folder: moveFolder } });
			mode = '';
			toast('MOVED');
			asset = await api<Asset>(`/assets/${asset!.id}`);
		} catch (e) {
			toast((e as Error).message, true);
		}
	}

	async function doDelete() {
		if (!asset || !file) return;
		const others = asset.files.filter((f) => f.status === 'ok').length - 1;
		const msg = others > 0 ? `Move this copy to trash? ${others} other copy stays.` : 'Move this file to trash?';
		if (!confirm(msg)) return;
		try {
			await api(`/files/${file.id}`, { method: 'DELETE' });
			toast('MOVED TO TRASH');
			if (others > 0) {
				asset = await api<Asset>(`/assets/${asset.id}`);
				return;
			}
			const id = asset.id;
			items.splice(index, 1);
			onchanged(id);
			if (!items.length) onclose();
			else if (index >= items.length) index = items.length - 1;
		} catch (e) {
			toast((e as Error).message, true);
		}
	}

	function onkeydown(e: KeyboardEvent) {
		const t = e.target as HTMLElement;
		const typing = t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable;
		if ((e.ctrlKey || e.metaKey) && e.key === 's') {
			e.preventDefault();
			save();
			return;
		}
		if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
			e.preventDefault();
			save(true);
			return;
		}
		if (e.key === 'Escape') {
			if (typing) t.blur();
			else onclose();
			return;
		}
		if (typing) return;
		if (e.key === 'ArrowRight') go(1);
		else if (e.key === 'ArrowLeft') go(-1);
		else if (e.key === 'i') panel = !panel;
	}
</script>

<svelte:window {onkeydown} />

<div class="overlay" role="dialog" aria-modal="true" aria-label="Viewer">
	<div class="stage">
		<header class="bar">
			<span class="orange">{pad4(current?.id ?? 0)}</span>
			<span class="ellipsis grow">{asset?.title || current?.name}</span>
			<span class="dim">{index + 1}/{items.length}</span>
			<Key size="sm" variant="dark" onclick={() => (panel = !panel)} active={panel}>INFO</Key>
			<Key size="sm" variant="dark" onclick={onclose}>ESC</Key>
		</header>
		<div class="view">
			{#if asset && asset.id === current?.id}
				<Media {asset} />
			{/if}
			{#if index > 0}
				<button type="button" class="nav prev" aria-label="Previous" onclick={() => go(-1)}>‹</button>
			{/if}
			{#if index < items.length - 1}
				<button type="button" class="nav next" aria-label="Next" onclick={() => go(1)}>›</button>
			{/if}
		</div>
	</div>

	{#if panel && asset && draft}
		<aside class="panel">
			<div class="screen info">
				<div class="row"><span class="label">FILE</span><span class="ellipsis">{asset.name}</span></div>
				<div class="row">
					<span class="label">DATE</span><span>{fmtDate(asset.takenAt ?? asset.sortAt, true)}</span>
					{#if !asset.takenAt}<span class="mute">(file time)</span>{/if}
				</div>
				<div class="row">
					<span class="label">SIZE</span><span>{fmtSize(asset.size)}</span>
					{#if asset.width}<span class="dim">{asset.width}×{asset.height}</span>{/if}
					{#if asset.durationMs}<span class="dim">{fmtDuration(asset.durationMs)}</span>{/if}
					{#if asset.videoCodec}<span class="dim">{asset.videoCodec}</span>{/if}
				</div>
				{#if asset.cameraModel}
					<div class="row"><span class="label">CAM</span><span class="ellipsis">{asset.cameraMake ?? ''} {asset.cameraModel}</span></div>
				{/if}
				{#if asset.lat != null}
					<div class="row">
						<span class="label">GPS</span>
						<a class="orange" target="_blank" rel="noreferrer" href="https://www.openstreetmap.org/?mlat={asset.lat}&mlon={asset.lon}#map=15/{asset.lat}/{asset.lon}">
							{asset.lat.toFixed(5)}, {asset.lon?.toFixed(5)}
						</a>
					</div>
				{/if}
				{#each asset.files as f (f.id)}
					<div class="row path">
						<span class="label">{f.status === 'ok' ? (f.online ? 'PATH' : 'OFF') : 'GONE'}</span>
						<span class="ellipsis" title={f.fullPath}>{f.library}/{f.path}</span>
					</div>
				{/each}
			</div>

			<div class="screen">
				<MetaForm bind:draft compact />
				<div class="row actions">
					<Key variant="light" onclick={() => save()} disabled={saving || !dirty} led={dirty}>SAVE</Key>
					<Key variant="orange" onclick={() => save(true)} disabled={saving} title="Ctrl+Enter">
						{asset.classified ? 'SAVE' : 'CLASSIFY'}
					</Key>
					{#if asset.classified}
						<Key variant="dark" size="sm" onclick={() => save(false)}>UNMARK</Key>
					{/if}
				</div>
			</div>

			{#if mode === 'rename'}
				<div class="screen col">
					<span class="label">NEW NAME</span>
					<input class="field" bind:value={newName} onkeydown={(e) => e.key === 'Enter' && doRename()} />
					<div class="row"><Key size="sm" onclick={doRename}>OK</Key><Key size="sm" variant="dark" onclick={() => (mode = '')}>CANCEL</Key></div>
				</div>
			{:else if mode === 'move'}
				<div class="screen col">
					<span class="label">MOVE TO</span>
					<FolderPicker bind:library={moveLib} bind:folder={moveFolder} />
					<div class="row"><Key size="sm" onclick={doMove}>MOVE</Key><Key size="sm" variant="dark" onclick={() => (mode = '')}>CANCEL</Key></div>
				</div>
			{/if}

			<div class="tools">
				<Key size="sm" href={originalUrl(asset.id, true)}>DOWNLOAD</Key>
				<Key size="sm" disabled={!file?.online} onclick={() => { newName = asset?.name ?? ''; mode = 'rename'; }}>RENAME</Key>
				<Key size="sm" disabled={!file?.online || !app.libraries.length} onclick={() => { moveLib = file?.libraryId ?? 0; moveFolder = ''; mode = 'move'; }}>MOVE</Key>
				<Key size="sm" variant="dark" disabled={!file?.online} onclick={doDelete}>DELETE</Key>
			</div>
		</aside>
	{/if}
</div>

<style>
	.overlay {
		position: fixed;
		inset: 0;
		z-index: 50;
		display: flex;
		background: var(--screen);
		color: var(--screen-ink);
	}

	.stage {
		flex: 1;
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.bar {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 10px 14px;
		border-bottom: 1px solid var(--screen-faint);
		font-size: 13px;
	}

	.view {
		position: relative;
		flex: 1;
		min-height: 0;
		padding: 12px;
	}

	.nav {
		position: absolute;
		top: 50%;
		transform: translateY(-50%);
		width: 44px;
		height: 72px;
		border: 0;
		border-radius: 8px;
		background: rgba(34, 35, 37, 0.7);
		color: var(--screen-ink);
		font-size: 30px;
		cursor: pointer;
	}

	.nav:hover {
		color: var(--orange);
	}

	.prev {
		left: 10px;
	}

	.next {
		right: 10px;
	}

	.panel {
		width: 400px;
		flex: none;
		overflow: auto;
		padding: 14px;
		display: flex;
		flex-direction: column;
		gap: 14px;
		background: var(--body);
		color: var(--ink);
		box-shadow: inset 3px 0 0 var(--edge);
	}

	.info {
		display: flex;
		flex-direction: column;
		gap: 6px;
		font-size: 12px;
	}

	.info .label {
		width: 40px;
		flex: none;
	}

	.col {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.actions {
		margin-top: 14px;
	}

	.tools {
		display: flex;
		flex-wrap: wrap;
		gap: 10px;
		padding-bottom: 6px;
	}

	@media (max-width: 900px) {
		.overlay {
			flex-direction: column;
			overflow: auto;
		}

		.stage {
			min-height: 70vh;
		}

		.panel {
			width: 100%;
			box-shadow: inset 0 3px 0 var(--edge);
		}
	}
</style>
