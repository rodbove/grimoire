<script lang="ts">
	let { cols = 9, rows = 4 }: { cols?: number; rows?: number } = $props();
</script>

<div class="grille" style="grid-template-columns: repeat({cols}, 7px)" aria-hidden="true">
	{#each Array(cols * rows) as _, i (i)}
		<span></span>
	{/each}
</div>

<style>
	.grille {
		display: grid;
		gap: 7px;
	}

	span {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: #55564f;
		box-shadow: inset 0 1px 1px rgba(0, 0, 0, 0.6);
	}
</style>
