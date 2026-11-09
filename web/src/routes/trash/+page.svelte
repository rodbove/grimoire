<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '#lib/api';
	import { fmtDate } from '#lib/format';
	import { app, toast } from '#lib/state.svelte';
	import Key from '#lib/components/Key.svelte';

	type Item = { id: number; fileId: number; assetId: number | null; library: string; path: string; deletedAt: number; kind: string };

	let items = $state<Item[]>([]);
	let loaded = $state(false);

	async function load() {
		try {
			items = await api<Item[]>('/trash');
		} catch (e) {
			toast((e as Error).message, true);
		}
		loaded = true;
	}

	async function restore(t: Item) {
		try {
			await api(`/trash/${t.id}/restore`, { method: 'POST' });
			items = items.filter((x) => x.id !== t.id);
			app.refresh++;
			toast('RESTORED');
		} catch (e) {
			toast((e as Error).message, true);
		}
	}

	async function purge(t: Item) {
		if (!confirm(`Delete "${t.path}" for good? This cannot be undone.`)) return;
		try {
			await api(`/trash/${t.id}`, { method: 'DELETE' });
			items = items.filter((x) => x.id !== t.id);
		} catch (e) {
			toast((e as Error).message, true);
		}
	}

	async function purgeAll() {
		if (!confirm(`Delete all ${items.length} files in trash for good? This cannot be undone.`)) return;
		for (const t of [...items]) {
			try {
				await api(`/trash/${t.id}`, { method: 'DELETE' });
				items = items.filter((x) => x.id !== t.id);
			} catch (e) {
				toast((e as Error).message, true);
				break;
			}
		}
	}

	onMount(load);
</script>

<svelte:head><title>trash · grimoire</title></svelte:head>

<div class="screen">
	<div class="row head">
		<span class="grow"><span class="orange">TRASH</span> <span class="dim">{items.length} FILES · REMOVED FOR GOOD AFTER 30 DAYS</span></span>
		{#if items.length}<Key size="sm" variant="dark" onclick={purgeAll}>EMPTY</Key>{/if}
	</div>
	{#if loaded && !items.length}
		<p class="dim empty">TRASH IS EMPTY</p>
	{/if}
	<ul>
		{#each items as t (t.id)}
			<li>
				{#if t.assetId}
					<img src="/api/assets/{t.assetId}/thumb" alt="" onerror={(e) => ((e.currentTarget as HTMLImageElement).style.visibility = 'hidden')} />
				{:else}
					<span class="ph"></span>
				{/if}
				<div class="grow info">
					<span class="ellipsis">{t.path}</span>
					<span class="dim">{t.library} · {fmtDate(t.deletedAt, true)}</span>
				</div>
				<Key size="sm" onclick={() => restore(t)}>RESTORE</Key>
				<Key size="sm" variant="dark" onclick={() => purge(t)}>DELETE</Key>
			</li>
		{/each}
	</ul>
</div>

<style>
	.head {
		margin-bottom: 10px;
	}

	ul {
		list-style: none;
		margin: 0;
		padding: 0;
	}

	li {
		display: flex;
		align-items: center;
		gap: 14px;
		padding: 10px 0;
		border-top: 1px solid var(--screen-faint);
	}

	img,
	.ph {
		width: 52px;
		height: 52px;
		border-radius: 5px;
		object-fit: cover;
		background: var(--screen-faint);
		flex: none;
	}

	.info {
		display: flex;
		flex-direction: column;
		gap: 3px;
		font-size: 13px;
	}

	.info .dim {
		font-size: 11px;
	}

	.empty {
		text-align: center;
		padding: 30px;
	}
</style>
