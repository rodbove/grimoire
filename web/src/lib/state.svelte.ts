import type { Library, Status } from './api';
import { api } from './api';

export const app = $state({
	ready: false,
	setUp: true,
	authenticated: false,
	libraries: [] as Library[],
	status: { assets: 0, unclassified: 0, jobs: {}, failed: 0, scanning: [] } as Status,
	uploadOpen: false,
	droppedFiles: [] as File[],
	refresh: 0
});

export const toasts = $state({ items: [] as { id: number; text: string; error: boolean }[] });

let toastId = 0;

export function toast(text: string, error = false) {
	const id = ++toastId;
	toasts.items.push({ id, text, error });
	setTimeout(() => {
		toasts.items = toasts.items.filter((t) => t.id !== id);
	}, error ? 6000 : 3000);
}

export async function loadLibraries() {
	app.libraries = await api<Library[]>('/libraries');
}

export function connectEvents() {
	let es: EventSource | null = null;
	let closed = false;
	let lastAssets = -1;
	const open = () => {
		es = new EventSource('/api/events');
		es.onmessage = (e) => {
			const s = JSON.parse(e.data) as Status;
			const wasScanning = app.status.scanning.length;
			app.status = s;
			if (wasScanning !== s.scanning.length) loadLibraries().catch(() => {});
			if (lastAssets >= 0 && s.assets !== lastAssets && pendingJobs(s) === 0) app.refresh++;
			lastAssets = s.assets;
		};
		es.onerror = () => {
			es?.close();
			if (!closed) setTimeout(open, 3000);
		};
	};
	open();
	return () => {
		closed = true;
		es?.close();
	};
}

export function pendingJobs(s: Status) {
	return Object.values(s.jobs).reduce((a, b) => a + b, 0);
}
