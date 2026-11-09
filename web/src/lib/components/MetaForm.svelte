<script lang="ts" module>
	export interface Draft {
		title: string;
		description: string;
		locationText: string;
		tags: string[];
		people: string[];
		rating: number;
		takenAt: string;
	}
</script>

<script lang="ts">
	import TagInput from './TagInput.svelte';

	let { draft = $bindable(), compact = false }: { draft: Draft; compact?: boolean } = $props();
	const uid = Math.random().toString(36).slice(2, 8);
</script>

<div class="form" class:compact>
	<label class="full">
		<span class="label">Title</span>
		<input class="field" id="title-{uid}" bind:value={draft.title} placeholder="untitled" autocomplete="off" />
	</label>
	<label class="full">
		<span class="label">Description</span>
		<textarea class="field" bind:value={draft.description} rows={compact ? 3 : 4} placeholder="what is in this file?"></textarea>
	</label>
	<div class="full">
		<label class="label" for="tags-{uid}">Tags</label>
		<TagInput id="tags-{uid}" bind:values={draft.tags} kind="tags" placeholder="add tag, Enter" />
	</div>
	<div class="full">
		<label class="label" for="people-{uid}">People</label>
		<TagInput id="people-{uid}" bind:values={draft.people} kind="people" placeholder="add person, Enter" accent="var(--blue)" />
	</div>
	<label>
		<span class="label">Location</span>
		<input class="field" bind:value={draft.locationText} placeholder="place" autocomplete="off" />
	</label>
	<label>
		<span class="label">Date</span>
		<input class="field" type="datetime-local" bind:value={draft.takenAt} />
	</label>
	<div class="full">
		<span class="label">Rating</span>
		<div class="stars" role="radiogroup" aria-label="Rating">
			{#each [1, 2, 3, 4, 5] as n (n)}
				<button
					type="button"
					role="radio"
					aria-checked={draft.rating === n}
					aria-label="{n} of 5"
					class:on={draft.rating >= n}
					onclick={() => (draft.rating = draft.rating === n ? 0 : n)}
				>{n}</button>
			{/each}
		</div>
	</div>
</div>

<style>
	.form {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 12px 12px;
	}

	.full {
		grid-column: 1 / -1;
	}

	label,
	.full {
		display: flex;
		flex-direction: column;
		gap: 5px;
		min-width: 0;
	}

	.stars {
		display: flex;
		gap: 8px;
	}

	.stars button {
		flex: 1;
		height: 34px;
		border: 1px solid #000;
		border-radius: 6px;
		background: var(--screen-faint);
		color: var(--screen-mute);
		cursor: pointer;
		box-shadow: inset 0 2px 5px rgba(0, 0, 0, 0.6);
	}

	.stars button.on {
		background: var(--orange);
		color: #1a0b03;
		box-shadow: 0 0 10px rgba(255, 91, 20, 0.4);
	}

	@media (max-width: 520px) {
		.form {
			grid-template-columns: 1fr;
		}
	}
</style>
