<script lang="ts">
	import { api } from '#lib/api';
	import { toast } from '#lib/state.svelte';
	import Key from './Key.svelte';
	import TagInput from './TagInput.svelte';

	let {
		selected = $bindable(),
		onselectevent,
		onselectall,
		ondone
	}: {
		selected: Set<number>;
		onselectevent: () => void;
		onselectall: () => void;
		ondone: () => void;
	} = $props();

	let addTags = $state<string[]>([]);
	let addPeople = $state<string[]>([]);
	let location = $state('');
	let description = $state('');
	let busy = $state(false);

	async function apply(classified?: boolean) {
		busy = true;
		try {
			const patch: Record<string, unknown> = { addTags, addPeople };
			if (location.trim()) patch.locationText = location;
			if (description.trim()) patch.description = description;
			if (classified !== undefined) patch.classified = classified;
			await api('/assets/bulk', { method: 'POST', body: { ids: [...selected], patch } });
			toast(`UPDATED ${selected.size}`);
			addTags = [];
			addPeople = [];
			location = '';
			description = '';
			ondone();
		} catch (e) {
			toast((e as Error).message, true);
		} finally {
			busy = false;
		}
	}

	function download() {
		window.location.href = `/api/download?ids=${[...selected].join(',')}`;
	}
</script>

<div class="bulk screen">
	<div class="row wrap head">
		<span class="orange count">{selected.size} SELECTED</span>
		<button type="button" class="link" onclick={onselectevent}>+ EVENT</button>
		<button type="button" class="link" onclick={onselectall}>+ ALL LOADED</button>
		<button type="button" class="link" onclick={() => (selected = new Set())}>CLEAR</button>
		<span class="grow"></span>
		<Key size="sm" variant="dark" onclick={download}>ZIP</Key>
	</div>
	<div class="fields">
		<TagInput bind:values={addTags} kind="tags" placeholder="add tags to all" />
		<TagInput bind:values={addPeople} kind="people" placeholder="add people to all" accent="var(--blue)" />
		<input class="field" bind:value={location} placeholder="set location" />
		<input class="field" bind:value={description} placeholder="set description" />
	</div>
	<div class="row wrap">
		<Key size="sm" onclick={() => apply()} disabled={busy}>APPLY</Key>
		<Key size="sm" variant="orange" onclick={() => apply(true)} disabled={busy}>APPLY + CLASSIFY</Key>
		<Key size="sm" variant="dark" onclick={() => apply(false)} disabled={busy}>UNMARK</Key>
	</div>
</div>

<style>
	.bulk {
		position: sticky;
		bottom: 12px;
		z-index: 20;
		display: flex;
		flex-direction: column;
		gap: 10px;
		margin-top: 16px;
		box-shadow:
			inset 0 0 0 6px #050506,
			0 12px 40px rgba(0, 0, 0, 0.5);
	}

	.count {
		font-size: 13px;
	}

	.link {
		border: 0;
		background: none;
		color: var(--screen-dim);
		cursor: pointer;
		font-size: 12px;
	}

	.link:hover {
		color: var(--orange);
	}

	.fields {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 8px;
	}

	@media (max-width: 700px) {
		.fields {
			grid-template-columns: 1fr;
		}
	}
</style>
