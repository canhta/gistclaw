export type AutomateEditorKind = 'at' | 'every' | 'cron';

export interface AutomateEditorState {
	name: string;
	objective: string;
	kind: AutomateEditorKind;
	anchorAt: string;
	everyHours: string;
	cronExpr: string;
	timezone: string;
}

export interface AutomateCreateRequest {
	name: string;
	objective: string;
	kind: AutomateEditorKind;
	anchor_at?: string;
	every_hours?: number;
	cron_expr?: string;
	timezone?: string;
}

export interface AutomateEditorErrors {
	name?: string;
	objective?: string;
	anchorAt?: string;
	everyHours?: string;
	cronExpr?: string;
}

type BuildAutomateCreateSuccess = {
	ok: true;
	request: AutomateCreateRequest;
};

type BuildAutomateCreateFailure = {
	ok: false;
	errors: AutomateEditorErrors;
};

export function defaultAutomateEditorState(): AutomateEditorState {
	return {
		name: '',
		objective: '',
		kind: 'cron',
		anchorAt: '',
		everyHours: '24',
		cronExpr: '',
		timezone: ''
	};
}

export function buildAutomateCreateRequest(
	state: AutomateEditorState
): BuildAutomateCreateSuccess | BuildAutomateCreateFailure {
	const name = state.name.trim();
	const objective = state.objective.trim();
	const anchorAt = state.anchorAt.trim();
	const cronExpr = state.cronExpr.trim();
	const timezone = state.timezone.trim();
	const errors: AutomateEditorErrors = {};

	if (name === '') {
		errors.name = 'Name is required.';
	}
	if (objective === '') {
		errors.objective = 'Objective is required.';
	}

	switch (state.kind) {
		case 'at': {
			const anchor = toRFC3339(anchorAt);
			if (anchorAt === '') {
				errors.anchorAt = 'Start time is required.';
			} else if (anchor == null) {
				errors.anchorAt = 'Start time must be a valid date and time.';
			}

			if (Object.keys(errors).length > 0) {
				return { ok: false, errors };
			}

			return {
				ok: true,
				request: {
					name,
					objective,
					kind: 'at',
					anchor_at: anchor ?? undefined
				}
			};
		}

		case 'every': {
			const anchor = toRFC3339(anchorAt);
			const everyHours = Number.parseInt(state.everyHours.trim(), 10);

			if (anchorAt === '') {
				errors.anchorAt = 'Start time is required.';
			} else if (anchor == null) {
				errors.anchorAt = 'Start time must be a valid date and time.';
			}
			if (!Number.isFinite(everyHours) || everyHours <= 0) {
				errors.everyHours = 'Repeat every hours must be greater than zero.';
			}

			if (Object.keys(errors).length > 0) {
				return { ok: false, errors };
			}

			return {
				ok: true,
				request: {
					name,
					objective,
					kind: 'every',
					anchor_at: anchor ?? undefined,
					every_hours: everyHours
				}
			};
		}

		case 'cron':
		default: {
			if (cronExpr === '') {
				errors.cronExpr = 'Cron expression is required.';
			}

			if (Object.keys(errors).length > 0) {
				return { ok: false, errors };
			}

			return {
				ok: true,
				request: {
					name,
					objective,
					kind: 'cron',
					cron_expr: cronExpr,
					...(timezone === '' ? {} : { timezone })
				}
			};
		}
	}
}

function toRFC3339(value: string): string | null {
	if (value === '') {
		return null;
	}

	const date = new Date(value);
	if (Number.isNaN(date.getTime())) {
		return null;
	}
	return date.toISOString();
}
