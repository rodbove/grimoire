<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		variant = 'light',
		led = false,
		active = false,
		size = 'md',
		href,
		type = 'button',
		disabled = false,
		title,
		wide = false,
		onclick,
		children
	}: {
		variant?: 'light' | 'dark' | 'orange';
		led?: boolean;
		active?: boolean;
		size?: 'sm' | 'md' | 'lg';
		href?: string;
		type?: 'button' | 'submit';
		disabled?: boolean;
		title?: string;
		wide?: boolean;
		onclick?: (e: MouseEvent) => void;
		children: Snippet;
	} = $props();
</script>

{#if href}
	<a {href} {title} class="key {variant} {size}" class:active class:wide aria-current={active ? 'page' : undefined}>
		{#if led}<span class="led"></span>{/if}
		{@render children()}
	</a>
{:else}
	<button {type} {title} {disabled} class="key {variant} {size}" class:active class:wide aria-pressed={active} {onclick}>
		{#if led}<span class="led"></span>{/if}
		{@render children()}
	</button>
{/if}

<style>
	.key {
		position: relative;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 6px;
		min-width: 64px;
		height: 42px;
		padding: 0 16px;
		border: 0;
		border-radius: var(--r);
		font-family: var(--mono);
		font-size: 14px;
		letter-spacing: 0.02em;
		text-transform: uppercase;
		text-decoration: none;
		white-space: nowrap;
		cursor: pointer;
		user-select: none;
		transition:
			transform 60ms ease,
			box-shadow 60ms ease;
	}

	.sm {
		height: 32px;
		min-width: 44px;
		padding: 0 10px;
		font-size: 12px;
	}

	.lg {
		height: 54px;
		min-width: 92px;
		font-size: 15px;
	}

	.wide {
		width: 100%;
	}

	.light {
		background: linear-gradient(180deg, var(--pad-hi), var(--pad) 60%);
		color: var(--ink);
		box-shadow:
			0 3px 0 var(--pad-shadow),
			0 5px 8px rgba(0, 0, 0, 0.28),
			inset 0 1px 0 rgba(255, 255, 255, 0.8);
	}

	.dark {
		background: linear-gradient(180deg, var(--key-hi), var(--key) 60%);
		color: var(--key-ink);
		box-shadow:
			0 3px 0 var(--key-shadow),
			0 5px 8px rgba(0, 0, 0, 0.35),
			inset 0 1px 0 rgba(255, 255, 255, 0.08);
	}

	.orange {
		background: linear-gradient(180deg, #ff7535, var(--orange) 60%);
		color: #1a0b03;
		box-shadow:
			0 3px 0 #a33608,
			0 5px 8px rgba(0, 0, 0, 0.3),
			inset 0 1px 0 rgba(255, 255, 255, 0.35);
	}

	.key:hover:not(:disabled) {
		filter: brightness(1.04);
	}

	.key:active:not(:disabled),
	.key.active {
		transform: translateY(2px);
	}

	.light:active:not(:disabled),
	.light.active {
		box-shadow:
			0 1px 0 var(--pad-shadow),
			0 2px 3px rgba(0, 0, 0, 0.25),
			inset 0 1px 0 rgba(255, 255, 255, 0.6);
	}

	.dark:active:not(:disabled),
	.dark.active {
		box-shadow:
			0 1px 0 var(--key-shadow),
			0 2px 3px rgba(0, 0, 0, 0.3);
	}

	.dark.active {
		background: linear-gradient(180deg, #1d1d1d, #151515);
	}

	.orange:active:not(:disabled) {
		box-shadow: 0 1px 0 #a33608;
	}

	.key:disabled {
		opacity: 0.45;
		cursor: default;
	}

	.led {
		position: absolute;
		top: 6px;
		left: 7px;
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--orange);
		box-shadow: 0 0 6px var(--orange);
	}
</style>
