// Inspector items context — pages write live data; layout reads it for AppShell.
import { getContext, setContext } from 'svelte';

const KEY = 'gc-inspector';

export type InspectorItem = {
	label: string;
	value: string;
	tone?: 'default' | 'accent' | 'warning';
};

export function setInspectorItems(items: () => InspectorItem[]): void {
	setContext(KEY, items);
}

export function getInspectorItems(): (() => InspectorItem[]) | undefined {
	return getContext<(() => InspectorItem[]) | undefined>(KEY);
}
