export interface Filters {
	q: string;
	kind: string;
	library: number;
	folder: string;
	direct: boolean;
	classified: string;
	tags: string[];
	tagMode: 'all' | 'any';
	people: string[];
	camera: string;
	rating: number;
	duplicates: boolean;
	offline: boolean;
	from: string;
	to: string;
	order: 'desc' | 'asc';
}

export const emptyFilters = (): Filters => ({
	q: '',
	kind: '',
	library: 0,
	folder: '',
	direct: false,
	classified: '',
	tags: [],
	tagMode: 'all',
	people: [],
	camera: '',
	rating: 0,
	duplicates: false,
	offline: false,
	from: '',
	to: '',
	order: 'desc'
});

export function toParams(f: Filters): URLSearchParams {
	const p = new URLSearchParams();
	if (f.q) p.set('q', f.q);
	if (f.kind) p.set('kind', f.kind);
	if (f.library) p.set('library', String(f.library));
	if (f.folder) p.set('folder', f.folder);
	if (f.direct) p.set('direct', '1');
	if (f.classified) p.set('classified', f.classified);
	if (f.tags.length) p.set('tags', f.tags.join(','));
	if (f.tagMode === 'any') p.set('tagMode', 'any');
	if (f.people.length) p.set('people', f.people.join(','));
	if (f.camera) p.set('camera', f.camera);
	if (f.rating) p.set('rating', String(f.rating));
	if (f.duplicates) p.set('duplicates', '1');
	if (f.offline) p.set('offline', '1');
	if (f.from) p.set('from', String(new Date(f.from + 'T00:00').getTime()));
	if (f.to) p.set('to', String(new Date(f.to + 'T00:00').getTime() + 86400000));
	if (f.order === 'asc') p.set('order', 'asc');
	return p;
}

export function fromUrl(u: { get(name: string): string | null }): Filters {
	const f = emptyFilters();
	f.q = u.get('q') ?? '';
	f.kind = u.get('kind') ?? '';
	f.library = Number(u.get('library') ?? 0) || 0;
	f.folder = u.get('folder') ?? '';
	f.direct = u.get('direct') === '1';
	f.classified = u.get('classified') ?? '';
	f.tags = (u.get('tags') ?? '').split(',').filter(Boolean);
	f.tagMode = u.get('tagMode') === 'any' ? 'any' : 'all';
	f.people = (u.get('people') ?? '').split(',').filter(Boolean);
	f.camera = u.get('camera') ?? '';
	f.rating = Number(u.get('rating') ?? 0) || 0;
	f.duplicates = u.get('duplicates') === '1';
	f.offline = u.get('offline') === '1';
	f.from = u.get('fromDate') ?? '';
	f.to = u.get('toDate') ?? '';
	f.order = u.get('order') === 'asc' ? 'asc' : 'desc';
	return f;
}

export function toUrl(f: Filters): string {
	const p = toParams(f);
	p.delete('from');
	p.delete('to');
	if (f.from) p.set('fromDate', f.from);
	if (f.to) p.set('toDate', f.to);
	const s = p.toString();
	return s ? '?' + s : '';
}
