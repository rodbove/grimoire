<script lang="ts">
	import { originalUrl, previewUrl, type Asset } from '#lib/api';

	let { asset }: { asset: Asset } = $props();

	let imgFailed = $state(false);
	let videoSrc = $state('');
	let transcoding = $state(false);
	let live = $state(false);
	let liveEl = $state<HTMLVideoElement>();

	$effect(() => {
		asset.id;
		imgFailed = false;
		live = false;
		transcoding = false;
		videoSrc = originalUrl(asset.id);
	});

	function videoError() {
		if (videoSrc.startsWith(originalUrl(asset.id))) {
			transcoding = true;
			videoSrc = `/api/assets/${asset.id}/video`;
		} else {
			transcoding = false;
		}
	}

	let liveSrc = $derived(
		asset.liveVideoId ? originalUrl(asset.liveVideoId) : asset.motion ? `/api/assets/${asset.id}/motion` : ''
	);

	function startLive() {
		if (!liveSrc) return;
		live = true;
		queueMicrotask(() => liveEl?.play().catch(() => {}));
	}

	let isPhoto = $derived(asset.kind === 'photo' || asset.kind === 'live_photo');
	let isVideo = $derived(asset.kind === 'video' || asset.kind === 'live_video');
	let isPdf = $derived(asset.mime === 'application/pdf');
	let browserImage = $derived(['image/jpeg', 'image/png', 'image/webp', 'image/gif', 'image/avif'].includes(asset.mime));
</script>

<div class="media">
	{#if isPhoto}
		{#if live && liveSrc}
			<!-- svelte-ignore a11y_media_has_caption -->
			<video bind:this={liveEl} src={liveSrc} autoplay playsinline onended={() => (live = false)}></video>
		{:else if !imgFailed}
			<img src={previewUrl(asset)} alt={asset.title ?? asset.name} onerror={() => (imgFailed = true)} />
		{:else if browserImage}
			<img src={originalUrl(asset.id)} alt={asset.title ?? asset.name} />
		{:else}
			<p class="msg">NO PREVIEW</p>
		{/if}
		{#if liveSrc}
			<button
				type="button"
				class="live"
				class:on={live}
				onpointerdown={startLive}
				onpointerup={() => (live = false)}
				onpointerleave={() => (live = false)}
				onkeydown={(e) => e.key === ' ' && startLive()}
				onkeyup={() => (live = false)}
			>
				<span class="led"></span>LIVE
			</button>
		{/if}
	{:else if isVideo}
		{#key videoSrc}
			<!-- svelte-ignore a11y_media_has_caption -->
			<video src={videoSrc} controls playsinline preload="metadata" poster={previewUrl(asset)} onerror={videoError}></video>
		{/key}
		{#if transcoding}<p class="note">CONVERTING VIDEO FOR THIS BROWSER...</p>{/if}
	{:else if isPdf}
		<iframe src={originalUrl(asset.id)} title={asset.name}></iframe>
	{:else}
		<div class="doc">
			<p class="ext">{(asset.name.split('.').pop() ?? '').toUpperCase()}</p>
			<a href={originalUrl(asset.id, true)}>DOWNLOAD {asset.name}</a>
		</div>
	{/if}
</div>

<style>
	.media {
		position: relative;
		display: grid;
		place-items: center;
		width: 100%;
		height: 100%;
		min-height: 0;
	}

	img,
	video,
	iframe {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: contain;
		display: block;
	}

	video {
		background: #000;
	}

	iframe {
		border: 0;
		background: #fff;
		border-radius: 4px;
	}

	.live {
		position: absolute;
		left: 10px;
		top: 10px;
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 5px 10px;
		border: 1px solid #333;
		border-radius: 5px;
		background: rgba(13, 14, 15, 0.8);
		color: var(--screen-ink);
		font-size: 11px;
		letter-spacing: 0.08em;
		cursor: pointer;
		touch-action: none;
	}

	.live .led {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--screen-mute);
	}

	.live.on .led {
		background: var(--orange);
		box-shadow: 0 0 6px var(--orange);
	}

	.msg,
	.note {
		color: var(--screen-dim);
		letter-spacing: 0.08em;
	}

	.note {
		position: absolute;
		bottom: 8px;
		left: 0;
		right: 0;
		text-align: center;
		font-size: 11px;
		color: var(--orange);
	}

	.doc {
		text-align: center;
	}

	.ext {
		font-size: 48px;
		color: var(--screen-mute);
		margin: 0 0 16px;
	}

	.doc a {
		color: var(--orange);
	}
</style>
