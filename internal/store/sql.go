package store

// queries contains the SQL statements used by Store. The ADK table prefix is
// fixed during Store initialization, so it is assembled once rather than at
// each call site. Runtime values are always passed as SQL parameters.
type queries struct {
	createSession           string
	getSession              string
	countSessions           string
	listSessions            string
	updateSession           string
	touchSession            string
	restoreSession          string
	deleteSession           string
	deleteADKSession        string
	deleteEvents            string
	sessionExists           string
	listEvents              string
	listHistoryEvents       string
	latestUserEvent         string
	deleteTurnEvents        string
	deleteCompactions       string
	recordCompactionFailure string
}

func newQueries(adkTablePrefix string) queries {
	adkEventsTable := adkTablePrefix + "events"
	adkSessionsTable := adkTablePrefix + "sessions"
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
			s.archived_at,
			s.current_compaction_id,
			s.compaction_generation,
			s.last_compaction_attempt_usage,
			s.consecutive_compaction_failures,
			COALESCE((
				SELECT usage.prompt_tokens + usage.completion_tokens
				FROM ` + adkEventsTable + ` AS usage
				WHERE usage.session_id = s.id
					AND (usage.prompt_tokens > 0 OR usage.completion_tokens > 0)
					AND usage.deleted_at = 0
					AND usage.archived_at = 0
					AND usage.event_id > s.context_usage_min_event_id
				ORDER BY usage.created_at DESC, usage.event_id DESC
				LIMIT 1
			), 0) AS context_tokens,
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
		s.updated_at,
		s.archived_at,
		s.current_compaction_id,
		s.compaction_generation,
		s.last_compaction_attempt_usage,
		s.consecutive_compaction_failures,
		s.context_usage_min_event_id
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
				AND (($1 AND archived_at > 0) OR (NOT $1 AND archived_at = 0))
		`,
		listSessions: sessionSelect + `
			WHERE s.deleted_at = 0
				AND (($1 AND s.archived_at > 0) OR (NOT $1 AND s.archived_at = 0))
			GROUP BY ` + sessionGroupBy + `
			ORDER BY s.updated_at DESC, s.id ASC
			LIMIT $2 OFFSET $3
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
				archived_at = $8,
				updated_at = $9
			WHERE id = $10 AND deleted_at = 0
		`,
		touchSession: `
			UPDATE koda_sessions
			SET updated_at = $1
			WHERE id = $2 AND deleted_at = 0
		`,
		restoreSession: `
			UPDATE koda_sessions
			SET title = $1,
				updated_at = $2
			WHERE id = $3 AND deleted_at = 0
		`,
		deleteSession: `
			UPDATE koda_sessions
			SET deleted_at = $1
			WHERE id = $2 AND deleted_at = 0
		`,
		deleteADKSession: `
			UPDATE ` + adkSessionsTable + `
			SET deleted_at = $1
			WHERE session_id = $2 AND deleted_at = 0
		`,
		deleteEvents: `
			UPDATE ` + adkEventsTable + `
			SET deleted_at = $1
			WHERE session_id = $2 AND deleted_at = 0
		`,
		sessionExists: `
			SELECT 1
			FROM koda_sessions
			WHERE id = $1 AND deleted_at = 0
		`,
		listEvents: `
			SELECT ` + eventColumns + `
			FROM ` + adkEventsTable + `
			WHERE session_id = $1
				AND deleted_at = 0
				AND archived_at = 0
			ORDER BY created_at ASC, event_id ASC
		`,
		listHistoryEvents: `
			SELECT ` + eventColumns + `
			FROM ` + adkEventsTable + `
			WHERE session_id = $1
				AND deleted_at = 0
			ORDER BY created_at ASC, event_id ASC
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
		deleteCompactions: `
			UPDATE koda_session_compactions
			SET deleted_at = $1
			WHERE session_id = $2 AND deleted_at = 0
		`,
		recordCompactionFailure: `
			UPDATE koda_sessions
			SET last_compaction_attempt_usage = $1,
				consecutive_compaction_failures = consecutive_compaction_failures + 1
			WHERE id = $2 AND deleted_at = 0 AND compaction_generation = $3
		`,
	}
}
