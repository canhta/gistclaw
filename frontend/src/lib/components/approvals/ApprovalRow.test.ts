import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import type { RecoverApprovalResponse } from '$lib/types/api';
import ApprovalRow from './ApprovalRow.svelte';

const pendingApproval: RecoverApprovalResponse = {
	id: 'appr-1',
	run_id: 'run-abc',
	tool_name: 'bash',
	binding_summary: 'rm -rf /tmp/scratch',
	status: 'pending',
	status_label: 'Pending',
	status_class: 'is-active'
};

const resolvedApproval: RecoverApprovalResponse = {
	id: 'appr-2',
	run_id: 'run-def',
	tool_name: 'read_file',
	binding_summary: '/etc/hosts',
	status: 'approved',
	status_label: 'Approved',
	status_class: 'is-success',
	resolved_by: 'admin',
	resolved_at_label: '2 min ago'
};

describe('ApprovalRow', () => {
	it('renders the tool name', () => {
		const { body } = render(ApprovalRow, { props: { approval: pendingApproval } });
		expect(body).toContain('bash');
	});

	it('renders the binding summary', () => {
		const { body } = render(ApprovalRow, { props: { approval: pendingApproval } });
		expect(body).toContain('rm -rf /tmp/scratch');
	});

	it('renders the status label', () => {
		const { body } = render(ApprovalRow, { props: { approval: pendingApproval } });
		expect(body).toContain('Pending');
	});

	it('renders approve and deny buttons for pending approval', () => {
		const { body } = render(ApprovalRow, { props: { approval: pendingApproval } });
		expect(body).toContain('APPROVE');
		expect(body).toContain('DENY');
	});

	it('does not render action buttons for resolved approval', () => {
		const { body } = render(ApprovalRow, { props: { approval: resolvedApproval } });
		expect(body).not.toContain('APPROVE');
		expect(body).not.toContain('DENY');
	});

	it('renders resolved_at_label when present', () => {
		const { body } = render(ApprovalRow, { props: { approval: resolvedApproval } });
		expect(body).toContain('2 min ago');
	});
});
