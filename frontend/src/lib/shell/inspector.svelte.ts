// Inspector context — pages set content; RightInspector reads it.
import { getContext, setContext } from 'svelte';

const KEY = 'gc:inspector';

export type InspectorItem = {
	label: string;
	value: string;
	tone?: 'default' | 'primary' | 'signal';
};

export type InspectorAction = {
	label: string;
	onclick: () => void;
	primary?: boolean;
};

export type InspectorContent = {
	eyebrow?: string;
	title?: string;
	items?: InspectorItem[];
	actions?: InspectorAction[];
};

export function setInspectorContent(fn: () => InspectorContent | null): void {
	setContext(KEY, fn);
}

export function getInspectorContent(): (() => InspectorContent | null) | undefined {
	return getContext<(() => InspectorContent | null) | undefined>(KEY);
}

// Legacy shim — used by existing pages during transition
export type InspectorLegacyItem = {
	label: string;
	value: string;
	tone?: 'default' | 'accent' | 'warning';
};

export function setInspectorItems(items: () => InspectorLegacyItem[]): void {
	setContext(KEY, () => ({
		items: items().map((i) => ({
			label: i.label,
			value: i.value,
			tone: i.tone === 'accent' ? 'signal' : i.tone === 'warning' ? 'primary' : 'default'
		}))
	}));
}

export function getInspectorItems(): (() => InspectorLegacyItem[]) | undefined {
	return undefined; // deprecated — use getInspectorContent
}
