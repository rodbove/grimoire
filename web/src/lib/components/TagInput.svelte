<script lang="ts">
	import { api, type NameCount } from '#lib/api';

	let {
		values = $bindable([]),
		kind = 'tags',
		placeholder = 'add tag',
		id,
		accent = 'var(--orange)'
	}: { values?: string[]; kind?: 'tags' | 'people'; placeholder?: string; id?: string; accent?: string } = $props();

	let text = $state('');
	let suggestions = $state<NameCount[]>([]);
	let active = $state(-1);
	let open = $state(false);
	let input: HTMLInputElement;
	let timer: ReturnType<typeof setTimeout>;

	function add(v: string) {
		const t = v.trim().replace(/\s+/g, ' ');
		if (!t) return;
		if (!values.some((x) => x.toLowerCase() === t.toLowerCase())) values = [...values, t];
		text = '';
		suggestions = [];
		active = -1;
	}

	function remove(v: string) {
		values = values.filter((x) => x !== v);
	}

	function fetchSuggestions() {
		clearTimeout(timer);
		timer = setTimeout(async () => {
			try {
				const res = await api<NameCount[]>(`/${kind}?prefix=${encodeURIComponent(text)}&limit=8`);
				suggestions = res.filter((s) => !values.some((v) => v.toLowerCase() === s.name.toLowerCase()));
				active = -1;
			} catch {
				suggestions = [];
			}
		}, 120);
	}

	function onkeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' || e.key === ',' || (e.key === 'Tab' && text)) {
			if (e.ctrlKey || e.metaKey) return;
			e.preventDefault();
			add(active >= 0 && suggestions[active] ? suggestions[active].name : text);
		} else if (e.key === 'Backspace' && !text && values.length) {
			values = values.slice(0, -1);
		} else if (e.key === 'ArrowDown' && suggestions.length) {
			e.preventDefault();
			active = (active + 1) % suggestions.length;
		} else if (e.key === 'ArrowUp' && suggestions.length) {
			e.preventDefault();
			active = (active - 1 + suggestions.length) % suggestions.length;
		} else if (e.key === 'Escape') {
			suggestions = [];
		}
	}
</script>

<div class="tags field" style="--accent: {accent}">
	{#each values as v (v)}
		<span class="chip">
			{v}
			<button type="button" aria-label="Remove {v}" onclick={() => remove(v)}>×</button>
		</span>
	{/each}
	<input
		{id}
		bind:this={input}
		bind:value={text}
		{placeholder}
		autocomplete="off"
		oninput={fetchSuggestions}
		onfocus={() => {
			open = true;
			fetchSuggestions();
		}}
		onblur={() => setTimeout(() => (open = false), 150)}
		{onkeydown}
	/>
	{#if open && suggestions.length}
		<ul class="menu" role="listbox">
			{#each suggestions as s, i (s.name)}
				<li role="option" aria-selected={i === active}>
					<button type="button" class:active={i === active} onmousedown={(e) => e.preventDefault()} onclick={() => add(s.name)}>
						<span>{s.name}</span><span class="mute">{s.count}</span>
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.tags {
		position: relative;
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		align-items: center;
		min-height: 40px;
		padding: 5px 6px;
		cursor: text;
	}

	.tags:focus-within {
		border-color: var(--orange);
	}

	input {
		flex: 1;
		min-width: 90px;
		background: transparent;
		border: 0;
		outline: none;
		padding: 4px;
	}

	input::placeholder {
		color: var(--screen-mute);
	}

	.chip {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 3px 4px 3px 9px;
		border-radius: 5px;
		background: var(--accent);
		color: #0d0e0f;
		font-size: 12px;
	}

	.chip button {
		border: 0;
		background: none;
		cursor: pointer;
		padding: 0 4px;
		font-size: 14px;
		line-height: 1;
		color: inherit;
	}

	.menu {
		position: absolute;
		z-index: 30;
		top: calc(100% + 4px);
		left: 0;
		right: 0;
		margin: 0;
		padding: 4px;
		list-style: none;
		background: #151617;
		border: 1px solid #2c2d2f;
		border-radius: 6px;
		box-shadow: 0 10px 30px rgba(0, 0, 0, 0.6);
	}

	.menu button {
		display: flex;
		justify-content: space-between;
		width: 100%;
		padding: 6px 8px;
		border: 0;
		border-radius: 4px;
		background: none;
		cursor: pointer;
		text-align: left;
	}

	.menu button:hover,
	.menu button.active {
		background: var(--screen-faint);
		color: var(--orange);
	}
</style>
