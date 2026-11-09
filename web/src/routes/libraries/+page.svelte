<script lang="ts">
	import { api, type Library } from '#lib/api';
	import { fmtDate } from '#lib/format';
	import { app, loadLibraries, toast } from '#lib/state.svelte';
	import Key from '#lib/components/Key.svelte';

	let path = $state('');
	let name = $state('');
	let upload = $state(true);
	let busy = $state(false);
	let browse = $state<{ path: string; parent: string; dirs: string[] } | null>(null);
	let editing = $state<number | null>(null);
	let editName = $state('');
	let editPath = $state('');

	async function openBrowser(p: string) {
		try {
			browse = await api(`/fs?path=${encodeURIComponent(p)}`);
		} catch (e) {
			toast((e as Error).message, true);
		}
	}

	async function add(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		try {
			await api<Library>('/libraries', { method: 'POST', body: { rootPath: path, name, isUploadTarget: upload } });
			toast('LIBRARY ADDED, SCANNING');
			path = name = '';
			browse = null;
			await loadLibraries();
		} catch (err) {
			toast((err as Error).message, true);
		} finally {
			busy = false;
		}
	}

	async function update(l: Library, body: Record<string, unknown>) {
		try {
			await api(`/libraries/${l.id}`, { method: 'PATCH', body });
			await loadLibraries();
			editing = null;
		} catch (e) {
			toast((e as Error).message, true);
		}
	}

	async function scan(l: Library) {
		try {
			await api(`/libraries/${l.id}/scan`, { method: 'POST' });
			toast('SCAN STARTED');
			setTimeout(() => loadLibraries(), 500);
		} catch (e) {
			toast((e as Error).message, true);
		}
	}

	async function remove(l: Library) {
		if (!confirm(`Remove library "${l.name}"? Files on disk stay. Metadata of classified files stays and comes back if you add the folder again.`)) return;
		try {
			await api(`/libraries/${l.id}`, { method: 'DELETE' });
			await loadLibraries();
			app.refresh++;
		} catch (e) {
			toast((e as Error).message, true);
		}
	}
</script>

<svelte:head><title>library · grimoire</title></svelte:head>

<div class="cards">
	{#each app.libraries as l, i (l.id)}
		<article class="screen card">
			<header class="row">
				<span class="badge">{String.fromCharCode(65 + (i % 26))}</span>
				<h3 class="grow ellipsis">{l.name}</h3>
				<span class="led" class:on={l.status === 'online'} title={l.status}></span>
				<span class="dim">{l.status.toUpperCase()}</span>
			</header>
			<p class="path ellipsis" title={l.rootPath}>{l.rootPath}</p>
			<div class="stats">
				<span><span class="mute">FILES</span> {l.fileCount.toLocaleString()}</span>
				<span><span class="mute">SCAN</span> {l.scanning || app.status.scanning.includes(l.id) ? 'RUNNING' : fmtDate(l.lastScanAt, true)}</span>
				<span><span class="mute">VOLUME</span> {l.volumeId ? 'ID OK' : 'NO ID'}</span>
			</div>
			{#if editing === l.id}
				<div class="edit">
					<input class="field" bind:value={editName} placeholder="name" />
					<input class="field" bind:value={editPath} placeholder="path" />
					<div class="row">
						<Key size="sm" onclick={() => update(l, { name: editName, rootPath: editPath })}>SAVE</Key>
						<Key size="sm" variant="dark" onclick={() => (editing = null)}>CANCEL</Key>
					</div>
				</div>
			{/if}
			<footer class="row wrap">
				<Key size="sm" variant={l.isUploadTarget ? 'orange' : 'dark'} onclick={() => update(l, { isUploadTarget: !l.isUploadTarget })}>
					UPLOAD {l.isUploadTarget ? 'ON' : 'OFF'}
				</Key>
				<Key size="sm" variant="dark" onclick={() => scan(l)}>SCAN</Key>
				<Key size="sm" variant="dark" onclick={() => { editing = l.id; editName = l.name; editPath = l.rootPath; }}>EDIT</Key>
				<span class="grow"></span>
				<Key size="sm" variant="dark" onclick={() => remove(l)}>REMOVE</Key>
			</footer>
		</article>
	{/each}
</div>

<form class="screen add" onsubmit={add}>
	<h3 class="orange">ADD LIBRARY</h3>
	<p class="dim">Type a folder path on the server machine, or browse. Grimoire reads and writes files in this folder.</p>
	<div class="row wrap">
		<input class="field grow" bind:value={path} placeholder={'D:\\Photos  or  /mnt/photos'} required />
		<Key size="sm" variant="dark" onclick={() => openBrowser(path)}>BROWSE</Key>
	</div>
	{#if browse}
		<div class="browser">
			<div class="row">
				<span class="grow ellipsis">{browse.path || 'COMPUTER'}</span>
				{#if browse.path}
					<button type="button" onclick={() => openBrowser(browse!.parent)}>▴ UP</button>
					<button type="button" class="orange" onclick={() => { path = browse!.path; browse = null; }}>USE THIS FOLDER</button>
				{/if}
			</div>
			<ul>
				{#each browse.dirs as d (d)}
					<li><button type="button" onclick={() => openBrowser(d)}>▸ {d}</button></li>
				{:else}
					<li class="mute">no subfolders</li>
				{/each}
			</ul>
		</div>
	{/if}
	<div class="row wrap">
		<input class="field grow" bind:value={name} placeholder="name (optional)" />
		<Key size="sm" variant={upload ? 'orange' : 'dark'} onclick={() => (upload = !upload)}>UPLOAD {upload ? 'ON' : 'OFF'}</Key>
		<Key type="submit" disabled={busy || !path}>ADD</Key>
	</div>
</form>

<style>
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
		gap: 20px;
		margin-bottom: 24px;
	}

	.card {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	h3 {
		margin: 0;
		font-size: 15px;
		font-weight: 500;
	}

	.badge {
		display: grid;
		place-items: center;
		width: 26px;
		height: 26px;
		border-radius: 5px;
		background: var(--white);
		color: var(--ink);
		font-size: 13px;
	}

	.led {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--lcd-off);
	}

	.led.on {
		background: var(--green);
		box-shadow: 0 0 6px var(--green);
	}

	.path {
		margin: 0;
		color: var(--screen-dim);
		font-size: 12px;
	}

	.stats {
		display: flex;
		gap: 16px;
		flex-wrap: wrap;
		font-size: 12px;
	}

	.edit {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	footer {
		margin-top: 4px;
	}

	.add {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.add p {
		margin: 0;
		font-size: 12px;
	}

	.browser {
		border-radius: 6px;
		background: var(--screen-faint);
		padding: 10px;
		font-size: 12px;
	}

	.browser button {
		border: 0;
		background: none;
		cursor: pointer;
		color: var(--screen-ink);
		padding: 3px 6px;
	}

	.browser button:hover {
		color: var(--orange);
	}

	.browser .orange {
		color: var(--orange);
	}

	.browser ul {
		list-style: none;
		margin: 8px 0 0;
		padding: 0;
		max-height: 260px;
		overflow: auto;
	}
</style>
