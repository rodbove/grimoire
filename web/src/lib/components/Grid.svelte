<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { api, thumbUrl, type Page, type Summary } from '#lib/api';
	import { fmtDate, fmtDuration, kindLabel, pad4 } from '#lib/format';
	import { toast } from '#lib/state.svelte';

	let {
		query,
		items = $bindable([]),
		total = $bindable(0),
		selected = $bindable(new Set<number>()),
		onopen,
		minTile = 168,
		refresh = 0
	}: {
		query: string;
		items?: Summary[];
		total?: number;
		selected?: Set<number>;
		onopen: (index: number) => void;
		minTile?: number;
		refresh?: number;
	} = $props();

	const gap = 14;
	const labelH = 28;

	let el: HTMLDivElement;
	let width = $state(0);
	let scrollY = $state(0);
	let viewH = $state(800);
	let top = $state(0);
	let next = '';
	let loading = $state(false);
	let generation = 0;
	let lastClicked = -1;

	let cols = $derived(Math.max(2, Math.floor((width + gap) / (minTile + gap))));
	let tile = $derived(width ? (width - gap * (cols - 1)) / cols : minTile);
	let rowH = $derived(tile + labelH + gap);
	let rows = $derived(Math.ceil(items.length / cols));
	let firstRow = $derived(Math.max(0, Math.floor((scrollY - top) / rowH) - 3));
	let lastRow = $derived(Math.min(rows, Math.ceil((scrollY - top + viewH) / rowH) + 3));
	let visible = $derived(items.slice(firstRow * cols, lastRow * cols).map((it, i) => ({ it, i: firstRow * cols + i })));

	async function load(reset: boolean) {
		const gen = reset ? ++generation : generation;
		if (!reset && (!next || loading)) return;
		loading = true;
		try {
			const sep = query ? query + '&' : '';
			const page = await api<Page>(`/assets?${sep}limit=240${!reset && next ? '&cursor=' + encodeURIComponent(next) : ''}`);
			if (gen !== generation) return;
			if (reset) {
				items = page.items;
				total = page.total ?? page.items.length;
				selected = new Set();
				lastClicked = -1;
			} else {
				items = [...items, ...page.items];
			}
			next = page.next;
		} catch (e) {
			if (gen === generation) toast((e as Error).message, true);
		} finally {
			if (gen === generation) loading = false;
		}
	}

	$effect(() => {
		query;
		refresh;
		untrack(() => {
			next = '';
			loading = false;
			load(true);
		});
	});

	$effect(() => {
		if (lastRow >= rows - 4 && !loading) untrack(() => load(false));
	});

	function measure() {
		if (!el) return;
		top = el.getBoundingClientRect().top + window.scrollY;
		width = el.clientWidth;
		viewH = window.innerHeight;
	}

	onMount(() => {
		measure();
		const ro = new ResizeObserver(measure);
		ro.observe(el);
		const onScroll = () => {
			scrollY = window.scrollY;
			top = el.getBoundingClientRect().top + window.scrollY;
		};
		onScroll();
		window.addEventListener('scroll', onScroll, { passive: true });
		window.addEventListener('resize', measure);
		return () => {
			ro.disconnect();
			window.removeEventListener('scroll', onScroll);
			window.removeEventListener('resize', measure);
		};
	});

	function toggle(i: number, shift: boolean) {
		const s = new Set(selected);
		if (shift && lastClicked >= 0) {
			const [a, b] = [Math.min(lastClicked, i), Math.max(lastClicked, i)];
			for (let k = a; k <= b; k++) s.add(items[k].id);
		} else if (s.has(items[i].id)) {
			s.delete(items[i].id);
		} else {
			s.add(items[i].id);
		}
		lastClicked = i;
		selected = s;
	}

	function click(e: MouseEvent, i: number) {
		if (e.shiftKey || e.ctrlKey || e.metaKey || selected.size > 0) {
			e.preventDefault();
			toggle(i, e.shiftKey);
			return;
		}
		lastClicked = i;
		onopen(i);
	}

	export function selectEvent() {
		const i = lastClicked >= 0 ? lastClicked : 0;
		if (!items.length) return;
		const gapMs = 2 * 3600 * 1000;
		let a = i;
		let b = i;
		while (a > 0 && Math.abs(items[a - 1].sortAt - items[a].sortAt) <= gapMs) a--;
		while (b < items.length - 1 && Math.abs(items[b + 1].sortAt - items[b].sortAt) <= gapMs) b++;
		const s = new Set(selected);
		for (let k = a; k <= b; k++) s.add(items[k].id);
		selected = s;
	}

	export function selectLoaded() {
		selected = new Set(items.map((x) => x.id));
	}

	export function reload() {
		next = '';
		load(true);
	}
</script>

<div class="grid" bind:this={el} style="height: {rows * rowH}px">
	{#each visible as { it, i } (it.id)}
		{@const x = (i % cols) * (tile + gap)}
		{@const y = Math.floor(i / cols) * rowH}
		<div class="cell" style="transform: translate({x}px, {y}px); width: {tile}px">
			<button
				type="button"
				class="pad"
				class:sel={selected.has(it.id)}
				style="height: {tile + labelH}px"
				title="{it.name} · {fmtDate(it.sortAt, true)}"
				onclick={(e) => click(e, i)}
			>
				<span class="top">
					<span>{pad4(it.id)}</span>
					<span class="kind">{kindLabel(it.kind, it.motion)}{it.durationMs ? ' ' + fmtDuration(it.durationMs) : ''}</span>
				</span>
				<span class="img" style="height: {tile - 40}px">
					{#if it.thumb === 'ok'}
						<img src={thumbUrl(it)} alt="" loading="lazy" decoding="async" draggable="false" />
					{:else}
						<span class="ph">{it.thumb === 'pending' ? '...' : (it.name.split('.').pop() ?? '').toUpperCase()}</span>
					{/if}
				</span>
				<span class="name">
					<span class="ellipsis">{it.title || it.name}</span>
					{#if !it.classified}<span class="dot" title="Not classified"></span>{/if}
				</span>
			</button>
			<button
				type="button"
				class="check"
				class:on={selected.has(it.id)}
				aria-label="Select {it.name}"
				onclick={(e) => {
					e.stopPropagation();
					toggle(i, e.shiftKey);
				}}
			></button>
		</div>
	{/each}
</div>

{#if loading && !items.length}
	<p class="empty">LOADING...</p>
{:else if !items.length}
	<p class="empty">NO FILES MATCH</p>
{/if}

<style>
	.grid {
		position: relative;
		width: 100%;
	}

	.cell {
		position: absolute;
		top: 0;
		left: 0;
	}

	.pad {
		display: flex;
		flex-direction: column;
		width: 100%;
		padding: 8px 9px 7px;
		border: 0;
		border-radius: 9px;
		background: linear-gradient(180deg, var(--pad-hi), var(--pad) 55%);
		box-shadow:
			0 4px 0 var(--pad-shadow),
			0 6px 10px rgba(0, 0, 0, 0.25),
			inset 0 1px 0 rgba(255, 255, 255, 0.8);
		cursor: pointer;
		text-align: left;
		transition: transform 60ms ease;
	}

	.pad:active {
		transform: translateY(2px);
		box-shadow:
			0 2px 0 var(--pad-shadow),
			0 3px 5px rgba(0, 0, 0, 0.25);
	}

	.pad.sel {
		outline: 3px solid var(--orange);
		outline-offset: -1px;
	}

	.top {
		display: flex;
		justify-content: space-between;
		font-size: 12px;
		height: 18px;
		padding-right: 18px;
	}

	.kind {
		color: var(--dim-ink);
		font-size: 10px;
		line-height: 18px;
	}

	.img {
		display: block;
		width: 100%;
		border-radius: 5px;
		overflow: hidden;
		background: var(--screen);
		box-shadow: inset 0 1px 3px rgba(0, 0, 0, 0.6);
	}

	img {
		width: 100%;
		height: 100%;
		object-fit: cover;
		display: block;
	}

	.ph {
		display: grid;
		place-items: center;
		height: 100%;
		color: var(--screen-mute);
		font-size: 13px;
	}

	.name {
		display: flex;
		align-items: center;
		gap: 6px;
		margin-top: 6px;
		font-size: 12px;
		height: 16px;
	}

	.name .ellipsis {
		flex: 1;
	}

	.dot {
		flex: none;
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: var(--ink);
	}

	.check {
		position: absolute;
		top: 8px;
		right: 8px;
		width: 18px;
		height: 18px;
		border: 0;
		background: none;
		cursor: pointer;
		padding: 0;
	}

	.check::after {
		content: '';
		position: absolute;
		inset: 5px;
		border-radius: 50%;
		background: #b9b8b2;
		box-shadow: inset 0 1px 1px rgba(0, 0, 0, 0.4);
	}

	.check.on::after {
		background: var(--orange);
		box-shadow: 0 0 6px var(--orange);
	}

	.empty {
		text-align: center;
		padding: 60px 0;
		color: var(--dim-ink);
		letter-spacing: 0.08em;
	}
</style>
