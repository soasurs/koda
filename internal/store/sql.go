package store

// queries contains the SQL statements used by Store. The ADK table prefix is
// fixed during Store initialization, so it is assembled once rather than at
// each call site. Runtime values are always passed as SQL parameters.
type queries struct {
	createSession    string
	getSession       string
	countSessions    string
	listSessions     string
	updateSession    string
	touchSession     string
	deleteSession    string
	sessionExists    string
	countEvents      string
	listEvents       string
	latestUserEvent  string
	deleteTurnEvents string
}

func newQueries(adkTablePrefix string) queries {
	adkEventsTable := adkTablePrefix + "events"
	const eventColumns = `
		event_id,
		session_id,
		turn_id,
		author,
		role,
		text,
		reasoning_text,
		tool_calls,
		tool_result,
		tool_call_id,
		finish_reason,
		parts,
		prompt_tokens,
		completion_tokens,
		total_tokens,
		usage_details,
		created_at,
		updated_at,
		archived_at,
		deleted_at
	`
	sessionSelect := `
		SELECT
			s.id,
			s.title,
			s.workdir,
			s.provider_id,
			s.model_id,
			s.reasoning_effort,
			s.file_access,
			s.shell_access,
			s.created_at,
			s.updated_at,
			COUNT(e.event_id) AS event_count
		FROM koda_sessions AS s
		LEFT JOIN ` + adkEventsTable + ` AS e
			ON e.session_id = s.id
			AND e.deleted_at = 0
			AND e.archived_at = 0
	`
	const sessionGroupBy = `
		s.id,
		s.title,
		s.workdir,
		s.provider_id,
		s.model_id,
		s.reasoning_effort,
		s.file_access,
		s.shell_access,
		s.created_at,
		s.updated_at
	`

	return queries{
		createSession: `
			INSERT INTO koda_sessions (
				id, title, workdir, provider_id, model_id, reasoning_effort,
				file_access, shell_access, created_at, updated_at, deleted_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 0)
		`,
		getSession: sessionSelect + `
			WHERE s.id = $1 AND s.deleted_at = 0
			GROUP BY ` + sessionGroupBy,
		countSessions: `
			SELECT COUNT(*)
			FROM koda_sessions
			WHERE deleted_at = 0
		`,
		listSessions: sessionSelect + `
			WHERE s.deleted_at = 0
			GROUP BY ` + sessionGroupBy + `
			ORDER BY s.updated_at DESC, s.id ASC
			LIMIT $1 OFFSET $2
		`,
		updateSession: `
			UPDATE koda_sessions
			SET title = $1,
				workdir = $2,
				provider_id = $3,
				model_id = $4,
				reasoning_effort = $5,
				file_access = $6,
				shell_access = $7,
				updated_at = $8
			WHERE id = $9 AND deleted_at = 0
		`,
		touchSession: `
			UPDATE koda_sessions
			SET updated_at = $1
			WHERE id = $2 AND deleted_at = 0
		`,
		deleteSession: `
			UPDATE koda_sessions
			SET deleted_at = $1
			WHERE id = $2 AND deleted_at = 0
		`,
		sessionExists: `
			SELECT 1
			FROM koda_sessions
			WHERE id = $1 AND deleted_at = 0
		`,
		countEvents: `
			SELECT COUNT(*)
			FROM ` + adkEventsTable + `
			WHERE session_id = $1
				AND deleted_at = 0
				AND archived_at = 0
		`,
		listEvents: `
			SELECT ` + eventColumns + `
			FROM ` + adkEventsTable + `
			WHERE session_id = $1
				AND deleted_at = 0
				AND archived_at = 0
			ORDER BY created_at ASC, event_id ASC
			LIMIT $2 OFFSET $3
		`,
		latestUserEvent: `
			SELECT ` + eventColumns + `
			FROM ` + adkEventsTable + `
			WHERE session_id = $1
				AND role = $2
				AND deleted_at = 0
				AND archived_at = 0
			ORDER BY created_at DESC, event_id DESC
			LIMIT 1
		`,
		deleteTurnEvents: `
			UPDATE ` + adkEventsTable + `
			SET deleted_at = $1
			WHERE session_id = $2
				AND turn_id = $3
				AND deleted_at = 0
				AND archived_at = 0
		`,
	}
}
