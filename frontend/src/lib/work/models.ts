import type { WorkClusterResponse } from '$lib/types/api';

export interface ModelUsageSummary {
	model: string;
	count: number;
}

export function summarizeModelUsage(
	clusters: WorkClusterResponse[] | null | undefined
): ModelUsageSummary[] {
	if (!clusters || clusters.length === 0) {
		return [];
	}

	const counts: Record<string, number> = {};

	for (const cluster of clusters) {
		for (const run of [cluster.root, ...(cluster.children ?? [])]) {
			const model = run.model_display.trim();
			if (model === '') {
				continue;
			}

			counts[model] = (counts[model] ?? 0) + 1;
		}
	}

	return Object.entries(counts)
		.map(([model, count]) => ({ model, count }))
		.sort((left, right) => right.count - left.count || left.model.localeCompare(right.model));
}
