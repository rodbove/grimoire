import type { Asset, Patch } from './api';
import type { Draft } from './components/MetaForm.svelte';
import { fromLocalInput, toLocalInput } from './format';

export function draftFrom(a: Asset): Draft {
	return {
		title: a.title ?? '',
		description: a.description ?? '',
		locationText: a.locationText ?? '',
		tags: [...a.tags],
		people: [...a.people],
		rating: a.rating ?? 0,
		takenAt: toLocalInput(a.takenAt)
	};
}

export function patchFrom(d: Draft, a: Asset, classified?: boolean): Patch {
	const p: Patch = {
		title: d.title,
		description: d.description,
		locationText: d.locationText,
		tags: d.tags,
		people: d.people,
		rating: d.rating
	};
	const t = fromLocalInput(d.takenAt);
	if (t && toLocalInput(t) !== toLocalInput(a.takenAt)) p.takenAt = t;
	if (classified !== undefined) p.classified = classified;
	return p;
}

export function sameDraft(a: Draft, b: Draft) {
	return JSON.stringify(a) === JSON.stringify(b);
}
