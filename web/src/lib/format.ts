export function fmtDate(ms: number | null | undefined, withTime = false) {
	if (!ms) return '--';
	const d = new Date(ms);
	const pad = (n: number) => String(n).padStart(2, '0');
	const date = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
	return withTime ? `${date} ${pad(d.getHours())}:${pad(d.getMinutes())}` : date;
}

export function fmtSize(n: number) {
	if (n < 1024) return `${n} B`;
	const u = ['KB', 'MB', 'GB', 'TB'];
	let i = -1;
	do {
		n /= 1024;
		i++;
	} while (n >= 1024 && i < u.length - 1);
	return `${n.toFixed(n < 10 ? 1 : 0)} ${u[i]}`;
}

export function fmtDuration(ms: number | null) {
	if (!ms) return '';
	const s = Math.round(ms / 1000);
	const m = Math.floor(s / 60);
	return `${m}:${String(s % 60).padStart(2, '0')}`;
}

export function toLocalInput(ms: number | null) {
	if (!ms) return '';
	const d = new Date(ms - new Date(ms).getTimezoneOffset() * 60000);
	return d.toISOString().slice(0, 16);
}

export function fromLocalInput(s: string) {
	if (!s) return undefined;
	const t = new Date(s).getTime();
	return Number.isNaN(t) ? undefined : t;
}

export function pad4(n: number) {
	return String(n % 10000).padStart(4, '0');
}

export function kindLabel(k: string, motion = false) {
	if (k === 'live_photo' || motion) return 'LIVE';
	if (k === 'video') return 'VID';
	if (k === 'document') return 'DOC';
	return 'IMG';
}
