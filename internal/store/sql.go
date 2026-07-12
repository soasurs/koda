package store

// queries contains the SQL statements used by Store. The ADK table prefix is
// fixed during Store initialization, so it is assembled once rather than at
// each call site. Runtime values are always passed as SQL parameters.
type queries struct {
	createSession string
	getSession    string
	countSessions string
	listSessions  string
	updateSession string
	touchSession  string
	deleteSession string
	sessionExists string
}

func newQueries(adkTablePrefix string) queries {
	adkEventsTable := adkTablePrefix + "events"
	sessionSelect := `
		SELECT
			s.id,
			s.title,
			s.workdir,
			s.provider_id,
			s.model_id,
			s.reasoning_effort,
			s.safe_mode,
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
		s.safe_mode,
		s.created_at,
		s.updated_at
	`

	return queries{
		createSession: `
			INSERT INTO koda_sessions (
				id, title, workdir, provider_id, model_id, reasoning_effort,
				safe_mode, created_at, updated_at, deleted_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 0)
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
				safe_mode = $6,
				updated_at = $7
			WHERE id = $8 AND deleted_at = 0
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
	}
}
