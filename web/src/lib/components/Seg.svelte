<script lang="ts">
	let { value = '', digits = 6, color = 'var(--orange)' }: { value?: string | number; digits?: number; color?: string } = $props();

	const map: Record<string, string> = {
		'0': 'abcdef',
		'1': 'bc',
		'2': 'abdeg',
		'3': 'abcdg',
		'4': 'bcfg',
		'5': 'acdfg',
		'6': 'acdefg',
		'7': 'abc',
		'8': 'abcdefg',
		'9': 'abcdfg',
		'-': 'g',
		' ': '',
		E: 'adefg',
		r: 'eg',
		o: 'cdeg',
		n: 'ceg',
		d: 'bcdeg',
		F: 'aefg',
		L: 'def'
	};

	const segs: Record<string, string> = {
		a: '3,1 5,0 15,0 17,1 15,3 5,3',
		b: '18,2 19,4 19,13 18,15 16,13 16,4',
		c: '18,17 19,19 19,28 18,30 16,28 16,19',
		d: '3,31 5,29 15,29 17,31 15,32 5,32',
		e: '2,17 4,19 4,28 2,30 1,28 1,19',
		f: '2,2 4,4 4,13 2,15 1,13 1,4',
		g: '3,16 5,14.5 15,14.5 17,16 15,17.5 5,17.5'
	};

	let chars = $derived(String(value).slice(-digits).padStart(digits, ' ').split(''));
</script>

<span class="seg" aria-label={String(value)} role="img">
	{#each chars as ch, i (i)}
		<svg viewBox="-1 -1 22 35" width="22" height="34" aria-hidden="true">
			<g transform="skewX(-6) translate(3 0)">
				{#each Object.entries(segs) as [name, pts] (name)}
					<polygon points={pts} fill={(map[ch] ?? '').includes(name) ? color : 'var(--lcd-off)'} class:lit={(map[ch] ?? '').includes(name)} />
				{/each}
			</g>
		</svg>
	{/each}
</span>

<style>
	.seg {
		display: inline-flex;
		gap: 4px;
	}

	.lit {
		filter: drop-shadow(0 0 3px rgba(255, 91, 20, 0.55));
	}
</style>
