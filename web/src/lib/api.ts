import { app } from './state.svelte';

export class ApiError extends Error {
	status: number;
	constructor(status: number, message: string) {
		super(message);
		this.status = status;
	}
}

type Options = { method?: string; body?: unknown; signal?: AbortSignal };

export async function api<T>(path: string, opts: Options = {}): Promise<T> {
	const headers: Record<string, string> = { 'X-Grimoire': '1' };
	if (opts.body !== undefined) headers['Content-Type'] = 'application/json';
	const res = await fetch('/api' + path, {
		method: opts.method ?? 'GET',
		headers,
		body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
		signal: opts.signal,
		credentials: 'same-origin'
	});
	const text = await res.text();
	let data: any = null;
	try {
		data = text ? JSON.parse(text) : null;
	} catch {
		data = null;
	}
	if (res.status === 401 && !path.startsWith('/auth/')) app.authenticated = false;
	if (!res.ok) throw new ApiError(res.status, data?.error ?? res.statusText);
	return data as T;
}

export type Kind = 'photo' | 'video' | 'live_photo' | 'live_video' | 'document';

export interface Summary {
	id: number;
	kind: Kind;
	mime: string;
	sortAt: number;
	takenAt: number | null;
	width: number | null;
	height: number | null;
	durationMs: number | null;
	title: string | null;
	rating: number | null;
	classified: boolean;
	motion: boolean;
	thumb: 'ok' | 'pending' | 'failed' | 'none';
	name: string;
	v: number;
}

export interface FileInfo {
	id: number;
	libraryId: number;
	library: string;
	path: string;
	fullPath: string;
	size: number;
	status: string;
	online: boolean;
}

export interface Asset extends Summary {
	size: number;
	videoCodec: string | null;
	takenAtSource: string | null;
	cameraMake: string | null;
	cameraModel: string | null;
	lat: number | null;
	lon: number | null;
	description: string | null;
	locationText: string | null;
	classifiedAt: number | null;
	liveVideoId: number | null;
	hash: string;
	tags: string[];
	people: string[];
	files: FileInfo[];
}

export interface Page {
	items: Summary[];
	next: string;
	total?: number;
}

export interface Library {
	id: number;
	name: string;
	rootPath: string;
	volumeId: string;
	isUploadTarget: boolean;
	status: 'online' | 'offline';
	lastScanAt: number | null;
	scanning: boolean;
	fileCount: number;
}

export interface Status {
	assets: number;
	unclassified: number;
	jobs: Record<string, number>;
	failed: number;
	scanning: number[];
}

export interface NameCount {
	name: string;
	count: number;
}

export interface Patch {
	title?: string | null;
	description?: string | null;
	locationText?: string | null;
	rating?: number;
	takenAt?: number;
	tags?: string[];
	people?: string[];
	addTags?: string[];
	removeTags?: string[];
	addPeople?: string[];
	removePeople?: string[];
	classified?: boolean;
}

export const thumbUrl = (a: { id: number; v: number }) => `/api/assets/${a.id}/thumb?v=${a.v}`;
export const previewUrl = (a: { id: number; v: number }) => `/api/assets/${a.id}/preview?v=${a.v}`;
export const originalUrl = (id: number, download = false) => `/api/assets/${id}/original${download ? '?download=1' : ''}`;
