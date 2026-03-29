import { describe, expect, it, vi } from 'vitest';
import { editKnowledgeItem, forgetKnowledgeItem } from './actions';

describe('knowledge action helpers', () => {
	it('posts knowledge edits to the edit endpoint', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			return new Response(
				JSON.stringify({
					id: 'mem-1',
					agent_id: 'assistant',
					scope: 'local',
					content: 'updated operator preference',
					source: 'human',
					provenance: 'Edited from config',
					confidence: 1,
					created_at_label: '2026-03-29 09:00',
					updated_at_label: '2026-03-29 11:00'
				}),
				{
					status: 200,
					headers: { 'content-type': 'application/json' }
				}
			);
		});

		await editKnowledgeItem(fetcher, 'mem-1', 'updated operator preference');

		expect(fetcher).toHaveBeenCalledWith('/api/knowledge/mem-1/edit', {
			method: 'POST',
			headers: {
				accept: 'application/json',
				'content-type': 'application/json'
			},
			body: JSON.stringify({
				content: 'updated operator preference'
			})
		});
	});

	it('posts forget actions to the forget endpoint', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			return new Response(JSON.stringify({ id: 'mem-1', forgotten: true }), {
				status: 200,
				headers: { 'content-type': 'application/json' }
			});
		});

		await forgetKnowledgeItem(fetcher, 'mem-1');

		expect(fetcher).toHaveBeenCalledWith('/api/knowledge/mem-1/forget', {
			method: 'POST',
			headers: {
				accept: 'application/json'
			}
		});
	});
});
