<script lang="ts">
	import * as tus from 'tus-js-client';
	import { api } from '#lib/api';
	import { fmtSize } from '#lib/format';
	import { app, toast } from '#lib/state.svelte';
	import FolderPicker from './FolderPicker.svelte';
	import Key from './Key.svelte';

	type Item = { file: File; progress: number; status: 'queued' | 'uploading' | 'done' | 'duplicate' | 'error'; error?: string; upload?: tus.Upload };

	let library = $state(0);
	let folder = $state('');
	let dup = $state<'skip' | 'keep'>('skip');
	let queue = $state<Item[]>([]);
	let running = $state(0);
	let picker = $state<HTMLInputElement>()!;
	let targets = $derived(app.libraries.filter((l) => l.isUploadTarget && l.status === 'online'));

	$effect(() => {
		if (app.droppedFiles.length) {
			addFiles(app.droppedFiles);
			app.droppedFiles = [];
		}
	});

	function addFiles(files: File[] | FileList) {
		for (const f of Array.from(files)) queue.push({ file: f, progress: 0, status: 'queued' });
	}

	function start() {
		if (!library) {
			toast('SELECT A LIBRARY', true);
			return;
		}
		pump();
	}

	function pump() {
		while (running < 3) {
			const item = queue.find((q) => q.status === 'queued');
			if (!item) break;
			run(item);
		}
	}

	function run(item: Item) {
		running++;
		item.status = 'uploading';
		const up = new tus.Upload(item.file, {
			endpoint: '/api/uploads',
			headers: { 'X-Grimoire': '1' },
			chunkSize: 32 * 1024 * 1024,
			retryDelays: [0, 1000, 3000, 5000, 10000],
			removeFingerprintOnSuccess: true,
			metadata: { filename: item.file.name, library: String(library), folder, dup },
			onProgress: (sent, total) => {
				item.progress = total ? sent / total : 1;
			},
			onError: (err) => {
				item.status = 'error';
				const m = /response text: (.*?)(,|$)/.exec(String(err));
				item.error = m?.[1] ?? 'upload failed';
				running--;
				pump();
			},
			onSuccess: async () => {
				try {
					const res = await api<{ status: string }>(`${(up.url ?? '').replace(/^.*\/api/, '')}/result`);
					item.status = res.status === 'duplicate' ? 'duplicate' : 'done';
				} catch {
					item.status = 'done';
				}
				item.progress = 1;
				running--;
				if (!queue.some((q) => q.status === 'queued' || q.status === 'uploading')) {
					app.refresh++;
					toast('UPLOAD DONE');
				}
				pump();
			}
		});
		item.upload = up;
		up.findPreviousUploads().then((prev) => {
			if (prev.length) up.resumeFromPreviousUpload(prev[0]);
			up.start();
		});
	}

	function clearDone() {
		queue = queue.filter((q) => q.status === 'queued' || q.status === 'uploading' || q.status === 'error');
	}

	function close() {
		app.uploadOpen = false;
	}

	let doneCount = $derived(queue.filter((q) => q.status === 'done').length);
	let dupCount = $derived(queue.filter((q) => q.status === 'duplicate').length);
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && close()} />

<div class="overlay" role="dialog" aria-modal="true" aria-label="Upload">
	<div class="box">
		<header class="row">
			<h2 class="grow">UPLOAD</h2>
			<Key size="sm" variant="dark" onclick={close}>CLOSE</Key>
		</header>
		{#if !targets.length}
			<p>No library is an upload target. Open <a href="/libraries" onclick={close}>LIBRARIES</a> and turn on UPLOAD for one.</p>
		{:else}
			<div class="screen cols">
				<div>
					<span class="label">TARGET</span>
					<FolderPicker bind:library bind:folder onlyUploadTargets />
				</div>
				<div class="side">
					<span class="label">DUPLICATES</span>
					<div class="row">
						<Key size="sm" variant={dup === 'skip' ? 'orange' : 'dark'} onclick={() => (dup = 'skip')}>SKIP</Key>
						<Key size="sm" variant={dup === 'keep' ? 'orange' : 'dark'} onclick={() => (dup = 'keep')}>KEEP BOTH</Key>
					</div>
					<button type="button" class="drop" onclick={() => picker.click()}>
						DROP FILES HERE<br /><span class="mute">or click to select</span>
					</button>
					<input bind:this={picker} type="file" multiple hidden onchange={(e) => addFiles((e.target as HTMLInputElement).files ?? [])} />
				</div>
			</div>
			{#if queue.length}
				<div class="screen list">
					<div class="row head">
						<span class="label grow">{queue.length} FILES · {doneCount} DONE{dupCount ? ` · ${dupCount} DUPLICATE` : ''}</span>
						<Key size="sm" variant="dark" onclick={clearDone}>CLEAR</Key>
						<Key size="sm" variant="orange" onclick={start} disabled={!queue.some((q) => q.status === 'queued')}>START</Key>
					</div>
					<ul>
						{#each queue as q, i (i)}
							<li>
								<span class="ellipsis grow">{q.file.name}</span>
								<span class="mute">{fmtSize(q.file.size)}</span>
								<span class="st {q.status}">{q.status === 'uploading' ? Math.round(q.progress * 100) + '%' : q.status.toUpperCase()}</span>
								<span class="bar" style="--p: {q.progress}"></span>
								{#if q.error}<span class="err">{q.error}</span>{/if}
							</li>
						{/each}
					</ul>
				</div>
			{/if}
		{/if}
	</div>
</div>

<style>
	.overlay {
		position: fixed;
		inset: 0;
		z-index: 60;
		display: grid;
		place-items: center;
		background: rgba(0, 0, 0, 0.55);
		padding: 16px;
	}

	.box {
		width: min(760px, 100%);
		max-height: 100%;
		overflow: auto;
		background: var(--body);
		border-radius: 18px;
		padding: 18px;
		display: flex;
		flex-direction: column;
		gap: 14px;
		box-shadow:
			inset 0 0 0 2px var(--edge),
			0 20px 60px rgba(0, 0, 0, 0.5);
	}

	h2 {
		margin: 0;
		font-size: 15px;
		letter-spacing: 0.08em;
	}

	.cols {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 16px;
	}

	.side {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	.drop {
		flex: 1;
		min-height: 110px;
		border: 1px dashed var(--screen-mute);
		border-radius: 8px;
		background: var(--screen-faint);
		color: var(--screen-ink);
		cursor: pointer;
		line-height: 1.7;
	}

	.drop:hover {
		border-color: var(--orange);
	}

	.list ul {
		list-style: none;
		margin: 10px 0 0;
		padding: 0;
		max-height: 320px;
		overflow: auto;
	}

	li {
		position: relative;
		display: flex;
		flex-wrap: wrap;
		gap: 10px;
		padding: 7px 0 9px;
		font-size: 12px;
		border-bottom: 1px solid var(--screen-faint);
	}

	.bar {
		position: absolute;
		left: 0;
		bottom: 2px;
		height: 2px;
		width: calc(var(--p) * 100%);
		background: var(--orange);
	}

	.st.done {
		color: var(--green);
	}

	.st.duplicate {
		color: var(--screen-dim);
	}

	.st.error,
	.err {
		color: var(--orange);
	}

	.err {
		width: 100%;
	}

	@media (max-width: 640px) {
		.cols {
			grid-template-columns: 1fr;
		}
	}
</style>
